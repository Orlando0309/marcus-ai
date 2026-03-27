package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/marcus-ai/marcus/internal/provider"
)

// ResultSynthesizer combines multiple agent results into a coherent output
type ResultSynthesizer struct {
	provider provider.Provider
	config   SynthesizerConfig
}

// SynthesizerConfig configures the synthesizer
type SynthesizerConfig struct {
	MaxResults     int     `json:"max_results"`
	MinConfidence  float64 `json:"min_confidence"`
	UseLLM         bool    `json:"use_llm"`
	IncludeMetrics bool    `json:"include_metrics"`
}

// DefaultSynthesizerConfig returns default config
func DefaultSynthesizerConfig() SynthesizerConfig {
	return SynthesizerConfig{
		MaxResults:     10,
		MinConfidence:  0.5,
		UseLLM:         true,
		IncludeMetrics: true,
	}
}

// NewResultSynthesizer creates a new result synthesizer
func NewResultSynthesizer(p provider.Provider, config SynthesizerConfig) *ResultSynthesizer {
	return &ResultSynthesizer{
		provider: p,
		config:   config,
	}
}

// SynthesisInput represents input for synthesis
type SynthesisInput struct {
	Query       string        `json:"query"`
	Results     []AgentResult `json:"results"`
	Context     string        `json:"context,omitempty"`
	Constraints []string      `json:"constraints,omitempty"`
}

// SynthesisOutput is the synthesized output
type SynthesisOutput struct {
	Summary       string           `json:"summary"`
	KeyFindings   []string         `json:"key_findings"`
	Conflicts     []string         `json:"conflicts,omitempty"`
	Confidence    float64          `json:"confidence"`
	Recommendations []string       `json:"recommendations,omitempty"`
	Metrics       SynthesisMetrics `json:"metrics,omitempty"`
}

// SynthesisMetrics holds synthesis quality metrics
type SynthesisMetrics struct {
	InputCount      int     `json:"input_count"`
	UsedCount       int     `json:"used_count"`
	AvgConfidence   float64 `json:"avg_confidence"`
	ConflictCount   int     `json:"conflict_count"`
	ProcessingTime  string  `json:"processing_time"`
}

// Synthesize combines multiple agent results
func (s *ResultSynthesizer) Synthesize(ctx context.Context, input SynthesisInput) (SynthesisOutput, error) {
	start := time.Now()
	output := SynthesisOutput{}

	// Filter results by confidence and limit
	filteredResults := s.filterResults(input.Results)

	if len(filteredResults) == 0 {
		return SynthesisOutput{
			Summary:    "No results met the minimum confidence threshold",
			Confidence: 0,
		}, nil
	}

	// Calculate metrics
	metrics := SynthesisMetrics{
		InputCount: len(input.Results),
		UsedCount:  len(filteredResults),
	}

	// Extract key findings
	output.KeyFindings = s.extractKeyFindings(filteredResults)

	// Detect conflicts
	output.Conflicts = s.detectConflicts(filteredResults)
	metrics.ConflictCount = len(output.Conflicts)

	// Calculate average confidence
	metrics.AvgConfidence = 0.8 // Default confidence for successful results
	output.Confidence = metrics.AvgConfidence

	// Use LLM for synthesis if enabled and provider available
	if s.config.UseLLM && s.provider != nil {
		llmOutput, err := s.synthesizeWithLLM(ctx, input, filteredResults)
		if err != nil {
			// Fall back to rule-based synthesis
			output.Summary = s.ruleBasedSummary(filteredResults)
		} else {
			output = llmOutput
			output.Metrics = metrics
		}
	} else {
		output.Summary = s.ruleBasedSummary(filteredResults)
		output.Recommendations = s.generateRecommendations(filteredResults)
	}

	metrics.ProcessingTime = time.Since(start).String()
	output.Metrics = metrics

	return output, nil
}

// filterResults filters results by confidence and limit
func (s *ResultSynthesizer) filterResults(results []AgentResult) []AgentResult {
	var filtered []AgentResult

	for _, r := range results {
		if r.Success {
			filtered = append(filtered, r)
			if len(filtered) >= s.config.MaxResults {
				break
			}
		}
	}

	return filtered
}

// extractKeyFindings extracts key findings from results
func (s *ResultSynthesizer) extractKeyFindings(results []AgentResult) []string {
	findings := make([]string, 0, len(results))

	for _, r := range results {
		if r.Success && r.Summary != "" {
			// Extract first sentence as key finding
			summary := r.Summary
			if idx := strings.Index(summary, "."); idx > 0 && idx < 200 {
				summary = summary[:idx+1]
			}
			findings = append(findings, summary)
		}
	}

	return findings
}

// detectConflicts detects conflicts between results
func (s *ResultSynthesizer) detectConflicts(results []AgentResult) []string {
	var conflicts []string

	// Simple conflict detection: look for contradictory statements
	// In production, use NLP-based contradiction detection
	seenTopics := make(map[string][]string)

	for _, r := range results {
		if r.Success {
			// Extract topic from first few words
			topic := strings.Fields(r.Summary)
			if len(topic) > 3 {
				topic = topic[:3]
			}
			topicKey := strings.Join(topic, " ")
			seenTopics[topicKey] = append(seenTopics[topicKey], r.Summary)
		}
	}

	// Check for conflicting statements on same topic
	for topic, statements := range seenTopics {
		if len(statements) > 1 {
			// Check if statements differ significantly
			if !s.statementsSimilar(statements) {
				conflicts = append(conflicts, fmt.Sprintf("Conflicting information on '%s'", topic))
			}
		}
	}

	return conflicts
}

// statementsSimilar checks if statements are semantically similar
func (s *ResultSynthesizer) statementsSimilar(statements []string) bool {
	if len(statements) < 2 {
		return true
	}

	// Simple heuristic: check word overlap
	words1 := strings.ToLower(strings.Join(strings.Fields(statements[0]), " "))
	words2 := strings.ToLower(strings.Join(strings.Fields(statements[1]), " "))

	if len(words1) < 10 || len(words2) < 10 {
		return words1 == words2
	}

	// Check for significant word overlap
	commonWords := 0
	wordSet := make(map[string]bool)
	for _, w := range strings.Fields(words1) {
		wordSet[w] = true
	}
	for _, w := range strings.Fields(words2) {
		if wordSet[w] {
			commonWords++
		}
	}

	// More than 50% common words = similar
	return float64(commonWords)/float64(len(wordSet)) > 0.5
}

// ruleBasedSummary generates a summary without LLM
func (s *ResultSynthesizer) ruleBasedSummary(results []AgentResult) string {
	if len(results) == 0 {
		return "No results available"
	}

	if len(results) == 1 {
		return results[0].Summary
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Analysis of %d sources:\n\n", len(results)))

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, r.Summary))
	}

	return sb.String()
}

// generateRecommendations generates recommendations from results
func (s *ResultSynthesizer) generateRecommendations(results []AgentResult) []string {
	recommendations := make([]string, 0)

	for _, r := range results {
		if r.Success && len(r.Actions) > 0 {
			for _, action := range r.Actions {
				recommendations = append(recommendations, action)
			}
		}
	}

	return recommendations
}

// synthesizeWithLLM uses LLM to synthesize results
func (s *ResultSynthesizer) synthesizeWithLLM(ctx context.Context, input SynthesisInput, results []AgentResult) (SynthesisOutput, error) {
	// Build synthesis prompt
	prompt := s.buildSynthesisPrompt(input, results)

	// Call LLM
	resp, err := s.provider.Complete(ctx, prompt, provider.CompletionOptions{
		Temperature: 0.3,
		MaxTokens:   2000,
		JSON:        true,
	})
	if err != nil {
		return SynthesisOutput{}, fmt.Errorf("LLM synthesis failed: %w", err)
	}

	// Parse response
	var output SynthesisOutput
	if err := json.Unmarshal([]byte(resp.Text), &output); err != nil {
		return SynthesisOutput{}, fmt.Errorf("parse synthesis: %w", err)
	}

	return output, nil
}

// buildSynthesisPrompt builds the LLM prompt for synthesis
func (s *ResultSynthesizer) buildSynthesisPrompt(input SynthesisInput, results []AgentResult) string {
	var sb strings.Builder

	sb.WriteString("Synthesize the following agent research results into a coherent answer.\n\n")

	if input.Context != "" {
		sb.WriteString("Context: ")
		sb.WriteString(input.Context)
		sb.WriteString("\n\n")
	}

	if len(input.Constraints) > 0 {
		sb.WriteString("Constraints:\n")
		for _, c := range input.Constraints {
			sb.WriteString("- ")
			sb.WriteString(c)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Agent Results:\n\n")
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, r.Summary))
		if len(r.Actions) > 0 {
			sb.WriteString("   Actions: ")
			for j, a := range r.Actions {
				if j > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(a)
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\nProvide a JSON response with this structure:\n")
	sb.WriteString(`{
  "summary": "Brief overall summary",
  "key_findings": ["Finding 1", "Finding 2"],
  "conflicts": ["Any conflicts detected"],
  "confidence": 0.8,
  "recommendations": ["Recommendation 1"]
}`)

	return sb.String()
}
