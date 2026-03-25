package flow

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/marcus-ai/marcus/internal/tool"
)

// VerificationResult represents the result of verifying an action
type VerificationResult struct {
	Action      tool.ActionProposal
	Success     bool
	Errors      []string
	Warnings    []string
	Suggestions []string
	Confidence  float64
	Retryable   bool
}

// Verifier checks tool outputs for correctness and common issues
type Verifier struct {
	// ErrorPatterns maps error patterns to their severity
	ErrorPatterns []ErrorPattern
	// WarningPatterns identifies potential issues
	WarningPatterns []WarningPattern
}

// ErrorPattern defines a pattern to detect errors
type ErrorPattern struct {
	Name        string
	Pattern     *regexp.Regexp
	Severity    string // "error", "critical"
	Message     string
	Suggestion  string
}

// WarningPattern defines a pattern to detect warnings
type WarningPattern struct {
	Name        string
	Pattern     *regexp.Regexp
	Message     string
	Suggestion  string
}

// NewVerifier creates a new verifier with default patterns
func NewVerifier() *Verifier {
	v := &Verifier{
		ErrorPatterns:   make([]ErrorPattern, 0),
		WarningPatterns: make([]WarningPattern, 0),
	}
	v.registerDefaultPatterns()
	return v
}

// registerDefaultPatterns sets up common error detection patterns
func (v *Verifier) registerDefaultPatterns() {
	// Build error patterns
	v.ErrorPatterns = append(v.ErrorPatterns, ErrorPattern{
		Name:       "compile_error",
		Pattern:    regexp.MustCompile(`(?i)(syntax error|compile error|build failed|cannot compile)`),
		Severity:   "error",
		Message:    "Compilation error detected",
		Suggestion: "Check syntax and dependencies, then retry",
	})
	v.ErrorPatterns = append(v.ErrorPatterns, ErrorPattern{
		Name:       "test_failure",
		Pattern:    regexp.MustCompile(`(?i)(test.*failed|fail.*test|test.*error)`),
		Severity:   "error",
		Message:    "Test failure detected",
		Suggestion: "Review test output and fix the underlying issue",
	})
	v.ErrorPatterns = append(v.ErrorPatterns, ErrorPattern{
		Name:       "import_error",
		Pattern:    regexp.MustCompile(`(?i)(cannot find package|import error|no such package)`),
		Severity:   "error",
		Message:    "Import or package error detected",
		Suggestion: "Check package names and ensure dependencies are installed",
	})
	v.ErrorPatterns = append(v.ErrorPatterns, ErrorPattern{
		Name:       "permission_error",
		Pattern:    regexp.MustCompile(`(?i)(permission denied|access denied|unauthorized)`),
		Severity:   "critical",
		Message:    "Permission denied",
		Suggestion: "This may require elevated permissions or configuration changes",
	})
	v.ErrorPatterns = append(v.ErrorPatterns, ErrorPattern{
		Name:       "not_found",
		Pattern:    regexp.MustCompile(`(?i)(file not found|no such file|does not exist)`),
		Severity:   "error",
		Message:    "File or resource not found",
		Suggestion: "Verify the path exists and is accessible",
	})
	v.ErrorPatterns = append(v.ErrorPatterns, ErrorPattern{
		Name:       "connection_error",
		Pattern:    regexp.MustCompile(`(?i)(connection refused|timeout|network error|no connection)`),
		Severity:   "error",
		Message:    "Network or connection error",
		Suggestion: "Check network connectivity and retry",
	})
	v.ErrorPatterns = append(v.ErrorPatterns, ErrorPattern{
		Name:       "resource_exhausted",
		Pattern:    regexp.MustCompile(`(?i)(out of memory|disk full|resource exhausted|quota exceeded)`),
		Severity:   "critical",
		Message:    "Resource exhausted",
		Suggestion: "Free resources or increase limits before retrying",
	})

	// Warning patterns
	v.WarningPatterns = append(v.WarningPatterns, WarningPattern{
		Name:       "deprecated",
		Pattern:    regexp.MustCompile(`(?i)(deprecated|obsolete|legacy)`),
		Message:    "Deprecated functionality used",
		Suggestion: "Consider updating to newer alternatives",
	})
	v.WarningPatterns = append(v.WarningPatterns, WarningPattern{
		Name:       "lint_warning",
		Pattern:    regexp.MustCompile(`(?i)(warning|linter|style issue)`),
		Message:    "Lint or style warning detected",
		Suggestion: "Consider fixing style issues for consistency",
	})
	v.WarningPatterns = append(v.WarningPatterns, WarningPattern{
		Name:       "inefficient",
		Pattern:    regexp.MustCompile(`(?i)(inefficient|slow|performance|optimize)`),
		Message:    "Performance warning detected",
		Suggestion: "Consider optimizing for better performance",
	})
}

// VerifyAction checks if an action's result indicates success or failure
func (v *Verifier) VerifyAction(action tool.ActionProposal, output string, err error) VerificationResult {
	result := VerificationResult{
		Action:     action,
		Success:    true,
		Confidence: 1.0,
		Retryable:  false,
	}

	// Check for execution error
	if err != nil {
		result.Success = false
		result.Errors = append(result.Errors, fmt.Sprintf("Execution error: %v", err))
		result.Retryable = v.isRetryableError(err)
		result.Confidence = 0.0
		return result
	}

	// Check output for error patterns
	outputLower := strings.ToLower(output)
	for _, pattern := range v.ErrorPatterns {
		if pattern.Pattern.MatchString(outputLower) {
			result.Success = false
			result.Errors = append(result.Errors, pattern.Message)
			if pattern.Suggestion != "" {
				result.Suggestions = append(result.Suggestions, pattern.Suggestion)
			}
			result.Retryable = pattern.Severity != "critical"
			result.Confidence = 0.1
		}
	}

	// Check for warnings (don't mark as failure but note them)
	for _, pattern := range v.WarningPatterns {
		if pattern.Pattern.MatchString(outputLower) {
			result.Warnings = append(result.Warnings, pattern.Message)
			if pattern.Suggestion != "" {
				result.Suggestions = append(result.Suggestions, pattern.Suggestion)
			}
		}
	}

	// Calculate confidence based on output quality
	result.Confidence = v.calculateConfidence(output, result.Errors)

	return result
}

// isRetryableError determines if an error can be retried
func (v *Verifier) isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())

	// Non-retryable errors
	nonRetryable := []string{
		"permission denied",
		"unauthorized",
		"invalid configuration",
		"bad request",
		"validation failed",
	}

	for _, pattern := range nonRetryable {
		if strings.Contains(errStr, pattern) {
			return false
		}
	}

	// Retryable errors
	retryable := []string{
		"timeout",
		"connection",
		"temporary",
		"rate limit",
		"unavailable",
	}

	for _, pattern := range retryable {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return true // Default to retryable
}

// calculateConfidence computes a confidence score based on output quality
func (v *Verifier) calculateConfidence(output string, errors []string) float64 {
	base := 1.0

	// Reduce confidence for errors
	for _, err := range errors {
		if strings.Contains(err, "critical") {
			base -= 0.5
		} else {
			base -= 0.3
		}
	}

	// Check output quality indicators
	if len(output) == 0 {
		base -= 0.2
	}
	if strings.Contains(strings.ToLower(output), "error") {
		base -= 0.1
	}
	if strings.Contains(strings.ToLower(output), "success") ||
		strings.Contains(strings.ToLower(output), "completed") {
		base += 0.1
	}

	// Clamp between 0 and 1
	if base < 0 {
		base = 0
	}
	if base > 1 {
		base = 1
	}

	return base
}

// VerifyActionBatch checks a batch of actions
func (v *Verifier) VerifyActionBatch(actions []tool.ActionProposal, outputs []string, errs []error) []VerificationResult {
	results := make([]VerificationResult, len(actions))
	for i := range actions {
		var err error
		if i < len(errs) {
			err = errs[i]
		}
		var output string
		if i < len(outputs) {
			output = outputs[i]
		}
		results[i] = v.VerifyAction(actions[i], output, err)
	}
	return results
}

// ShouldRetry determines if an action should be retried based on verification
func (v *Verifier) ShouldRetry(result VerificationResult) bool {
	return !result.Success && result.Retryable
}

// SuggestFix generates a suggestion for fixing a failed action
func (v *Verifier) SuggestFix(result VerificationResult) string {
	if result.Success {
		return ""
	}

	if len(result.Suggestions) > 0 {
		return result.Suggestions[0]
	}

	// Generate suggestion based on action type
	switch result.Action.Type {
	case "write_file", "create_file":
		return "Check file permissions and path existence before writing"
	case "run_command":
		return "Verify the command syntax and required dependencies"
	case "read_file":
		return "Ensure the file path is correct and accessible"
	case "patch_file":
		return "Verify the patch can be applied cleanly to the file"
	default:
		return "Review the error message and address the root cause"
	}
}
