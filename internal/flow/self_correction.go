package flow

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/marcus-ai/marcus/internal/provider"
	"github.com/marcus-ai/marcus/internal/tool"
)

// SelfCorrectionEngine detects and fixes errors in tool outputs
type SelfCorrectionEngine struct {
	verifier             *Verifier
	maxRetries           int
	confidenceThreshold  float64
	backoffStrategy      BackoffStrategy
	provider             provider.Provider
	correctionHistory    []CorrectionRecord
}

// BackoffStrategy defines retry backoff behavior
type BackoffStrategy struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

// CorrectionRecord tracks a correction attempt
type CorrectionRecord struct {
	Timestamp   time.Time
	Action      tool.ActionProposal
	Attempt     int
	Error       string
	FixApplied  string
	Success     bool
}

// CorrectionResult contains the outcome of self-correction
type CorrectionResult struct {
	OriginalAction tool.ActionProposal
	FixedAction    *tool.ActionProposal
	Success        bool
	Attempts       int
	FinalError     string
	Confidence     float64
	Records        []CorrectionRecord
}

// DefaultBackoff returns the default backoff strategy
func DefaultBackoff() BackoffStrategy {
	return BackoffStrategy{
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
}

// NewSelfCorrectionEngine creates a new self-correction engine
func NewSelfCorrectionEngine(opts CorrectionOptions) *SelfCorrectionEngine {
	return &SelfCorrectionEngine{
		verifier:            NewVerifier(),
		maxRetries:          opts.MaxRetries,
		confidenceThreshold: opts.ConfidenceThreshold,
		backoffStrategy:     opts.Backoff,
		provider:            nil, // LLM-based corrections optional
		correctionHistory:   make([]CorrectionRecord, 0),
	}
}

// SetProvider sets the LLM provider for LLM-based corrections
func (s *SelfCorrectionEngine) SetProvider(provider provider.Provider) {
	s.provider = provider
}

// CorrectionOptions configures the self-correction engine
type CorrectionOptions struct {
	MaxRetries          int
	ConfidenceThreshold float64
	Backoff             BackoffStrategy
}

// DefaultCorrectionOptions returns default options
func DefaultCorrectionOptions() CorrectionOptions {
	return CorrectionOptions{
		MaxRetries:          3,
		ConfidenceThreshold: 0.7,
		Backoff:             DefaultBackoff(),
	}
}

// CorrectAction attempts to fix a failed action
func (s *SelfCorrectionEngine) CorrectAction(ctx context.Context, action tool.ActionProposal, output string, err error) CorrectionResult {
	result := CorrectionResult{
		OriginalAction: action,
		Attempts:       0,
		Records:        make([]CorrectionRecord, 0),
	}

	// Verify the action result
	verification := s.verifier.VerifyAction(action, output, err)

	// If successful, no correction needed
	if verification.Success {
		result.Success = true
		result.Confidence = verification.Confidence
		return result
	}

	// Attempt corrections
	currentAction := action
	for attempt := 1; attempt <= s.maxRetries; attempt++ {
		result.Attempts = attempt

		// Wait before retry (exponential backoff)
		if attempt > 1 {
			delay := s.calculateBackoff(attempt)
			time.Sleep(delay)
		}

		// Generate fixed action
		fixedAction, fixDescription := s.generateFix(ctx, currentAction, output, verification)

		record := CorrectionRecord{
			Timestamp:  time.Now(),
			Action:     currentAction,
			Attempt:    attempt,
			Error:      fmt.Sprintf("%v", verification.Errors),
			FixApplied: fixDescription,
			Success:    false,
		}

		if fixedAction == nil {
			// No fix possible
			record.Success = false
			result.Records = append(result.Records, record)
			result.FinalError = fmt.Sprintf("Could not generate fix after %d attempts", attempt)
			result.Success = false
			return result
		}

		result.FixedAction = fixedAction
		currentAction = *fixedAction

		// Record the attempt
		s.correctionHistory = append(s.correctionHistory, record)
		result.Records = append(result.Records, record)
	}

	result.Success = false
	result.FinalError = fmt.Sprintf("Failed to correct after %d attempts", result.Attempts)
	return result
}

// CorrectActionBatch attempts to fix a batch of failed actions
func (s *SelfCorrectionEngine) CorrectActionBatch(ctx context.Context, actions []tool.ActionProposal, outputs []string, errs []error) []CorrectionResult {
	results := make([]CorrectionResult, len(actions))
	for i := range actions {
		var err error
		if i < len(errs) {
			err = errs[i]
		}
		var output string
		if i < len(outputs) {
			output = outputs[i]
		}
		results[i] = s.CorrectAction(ctx, actions[i], output, err)
	}
	return results
}

// generateFix creates a fixed version of an action
func (s *SelfCorrectionEngine) generateFix(ctx context.Context, action tool.ActionProposal, output string, verification VerificationResult) (*tool.ActionProposal, string) {
	if !verification.Retryable {
		return nil, "Error is not retryable"
	}

	// Generate fix based on action type and error
	switch action.Type {
	case "write_file", "create_file":
		return s.fixFileWrite(action, verification)
	case "run_command":
		return s.fixCommand(action, verification)
	case "read_file":
		return s.fixFileRead(action, verification)
	case "patch_file":
		return s.fixPatch(action, verification)
	case "edit_file":
		return s.fixEdit(action, verification)
	default:
		return s.fixWithLLM(ctx, action, output, verification)
	}
}

// fixFileWrite fixes file write errors
func (s *SelfCorrectionEngine) fixFileWrite(action tool.ActionProposal, verification VerificationResult) (*tool.ActionProposal, string) {
	fixed := action

	// Check for path issues
	for _, err := range verification.Errors {
		if contains(err, "not found") || contains(err, "does not exist") {
			// Ensure directory exists by modifying the action
			// This is a hint - actual implementation may vary
			return &fixed, "Ensure directory exists before writing"
		}
		if contains(err, "permission") {
			return nil, "Permission error - cannot auto-fix"
		}
	}

	return &fixed, "Retry file write"
}

// fixCommand fixes command execution errors
func (s *SelfCorrectionEngine) fixCommand(action tool.ActionProposal, verification VerificationResult) (*tool.ActionProposal, string) {
	fixed := action
	cmd := action.Command

	// Common command fixes
	for _, err := range verification.Errors {
		if contains(err, "not found") || contains(err, "not recognized") {
			// Try alternative commands
			if contains(cmd, "python") {
				fixed.Command = replacePrefix(cmd, "python", "python3")
				return &fixed, "Try python3 instead of python"
			}
			if contains(cmd, "python3") {
				fixed.Command = replacePrefix(cmd, "python3", "python")
				return &fixed, "Try python instead of python3"
			}
		}

		if contains(err, "permission") {
			// Add sudo or check if sudo is appropriate
			if !contains(cmd, "sudo") {
				fixed.Command = "sudo " + cmd
				return &fixed, "Try with elevated permissions"
			}
		}
	}

	return &fixed, "Retry command"
}

// fixFileRead fixes file read errors
func (s *SelfCorrectionEngine) fixFileRead(action tool.ActionProposal, verification VerificationResult) (*tool.ActionProposal, string) {
	fixed := action

	for _, err := range verification.Errors {
		if contains(err, "not found") || contains(err, "does not exist") {
			// Try alternative paths
			path := action.Path
			if contains(path, "/") {
				// Try relative path
				fixed.Path = "." + path[strings.LastIndex(path, "/"):]
				return &fixed, "Try relative path"
			}
		}
	}

	return &fixed, "Retry file read"
}

// fixPatch fixes patch application errors
func (s *SelfCorrectionEngine) fixPatch(action tool.ActionProposal, verification VerificationResult) (*tool.ActionProposal, string) {
	// Patch errors typically require LLM intervention
	return s.fixWithLLM(nil, action, "", verification)
}

// fixEdit fixes edit operation errors
func (s *SelfCorrectionEngine) fixEdit(action tool.ActionProposal, verification VerificationResult) (*tool.ActionProposal, string) {
	// Edit errors may require content adjustment
	return s.fixWithLLM(nil, action, "", verification)
}

// fixWithLLM uses the LLM to generate a fix
func (s *SelfCorrectionEngine) fixWithLLM(ctx context.Context, action tool.ActionProposal, output string, verification VerificationResult) (*tool.ActionProposal, string) {
	if s.provider == nil || ctx == nil {
		return nil, "No provider available for LLM-based correction"
	}

	// Build correction prompt
	prompt := s.buildCorrectionPrompt(action, output, verification)

	// Call LLM for fix
	resp, err := s.provider.Complete(ctx, prompt, provider.CompletionOptions{
		Temperature: 0.3,
		MaxTokens:   500,
		JSON:        true,
	})

	if err != nil || resp == nil {
		return nil, "LLM correction failed"
	}

	// Parse and apply the fix
	fixed, err := parseCorrectionResponse(action, resp.Text)
	if err != nil {
		return nil, "Failed to parse LLM correction"
	}

	return fixed, "LLM-generated fix"
}

// buildCorrectionPrompt creates a prompt for LLM-based correction
func (s *SelfCorrectionEngine) buildCorrectionPrompt(action tool.ActionProposal, output string, verification VerificationResult) string {
	return fmt.Sprintf(`The following action failed. Please provide a corrected version.

Action Type: %s
Action Details: %+v

Error Output: %s
Verification Errors: %v

Please provide a JSON response with the corrected action. Only change what is necessary to fix the error.`,
		action.Type,
		action,
		output,
		verification.Errors,
	)
}

// parseCorrectionResponse parses an LLM correction response
func parseCorrectionResponse(original tool.ActionProposal, response string) (*tool.ActionProposal, error) {
	// For now, return a modified version of the original
	// In a full implementation, this would parse structured JSON
	return &original, nil
}

// calculateBackoff computes the delay before retry
func (s *SelfCorrectionEngine) calculateBackoff(attempt int) time.Duration {
	delay := float64(s.backoffStrategy.InitialDelay) * math.Pow(s.backoffStrategy.Multiplier, float64(attempt-1))
	if delay > float64(s.backoffStrategy.MaxDelay) {
		delay = float64(s.backoffStrategy.MaxDelay)
	}
	return time.Duration(delay)
}

// ShouldSelfCorrect determines if self-correction should be attempted
func (s *SelfCorrectionEngine) ShouldSelfCorrect(result VerificationResult) bool {
	return !result.Success && result.Retryable && result.Confidence < s.confidenceThreshold
}

// GetCorrectionStats returns statistics about corrections
func (s *SelfCorrectionEngine) GetCorrectionStats() CorrectionStats {
	stats := CorrectionStats{
		TotalAttempts:   len(s.correctionHistory),
		SuccessfulFixes: 0,
		FailedFixes:     0,
	}

	for _, record := range s.correctionHistory {
		if record.Success {
			stats.SuccessfulFixes++
		} else {
			stats.FailedFixes++
		}
	}

	if stats.TotalAttempts > 0 {
		stats.SuccessRate = float64(stats.SuccessfulFixes) / float64(stats.TotalAttempts)
	}

	return stats
}

// CorrectionStats tracks correction performance
type CorrectionStats struct {
	TotalAttempts   int
	SuccessfulFixes int
	FailedFixes     int
	SuccessRate     float64
}

// ResetHistory clears the correction history
func (s *SelfCorrectionEngine) ResetHistory() {
	s.correctionHistory = make([]CorrectionRecord, 0)
}

// Helper functions
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func replacePrefix(s, old, new string) string {
	if len(s) >= len(old) && s[:len(old)] == old {
		return new + s[len(old):]
	}
	return s
}
