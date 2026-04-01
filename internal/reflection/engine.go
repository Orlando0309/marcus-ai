package reflection

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/marcus-ai/marcus/internal/memory"
	"github.com/marcus-ai/marcus/internal/outcome"
	"github.com/marcus-ai/marcus/internal/provider"
)

// Reflection represents a single reflection entry
type Reflection struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Context     string    `json:"context"`     // What was the goal/situation
	Decision    string    `json:"decision"`    // What decision was made
	Outcome     string    `json:"outcome"`     // What was the result
	Learned     string    `json:"learned"`     // What was learned
	Confidence  float64   `json:"confidence"`  // Confidence in this lesson (0-1)
	Applicable  []string  `json:"applicable"`  // Tags/contexts where this applies
}

// Heuristic represents a learned rule or pattern
type Heuristic struct {
	ID          string    `json:"id"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
	Condition   string    `json:"condition"`   // When this applies
	Recommendation string `json:"recommendation"` // What to do
	Avoid       string   `json:"avoid,omitempty"` // What to avoid
	SuccessCount int     `json:"success_count"`
	FailureCount int     `json:"failure_count"`
	Confidence  float64 `json:"confidence"`    // Derived from success/failure ratio
	Tags        []string `json:"tags,omitempty"`
}

// Engine performs reflection on past decisions and outcomes
type Engine struct {
	mu           sync.RWMutex
	dataDir      string
	reflections  []Reflection
	heuristics   map[string]*Heuristic
	memory       *memory.Manager
	outcomeTracker *outcome.Tracker
	provider     provider.Provider
	maxReflections int
}

// NewEngine creates a new reflection engine
func NewEngine(dataDir string, mem *memory.Manager, tracker *outcome.Tracker) *Engine {
	e := &Engine{
		dataDir:        filepath.Join(dataDir, "reflection"),
		reflections:    make([]Reflection, 0),
		heuristics:     make(map[string]*Heuristic),
		memory:         mem,
		outcomeTracker: tracker,
		maxReflections: 1000,
	}
	e.loadHeuristics()
	return e
}

// SetProvider sets the LLM provider for deep reflection
func (e *Engine) SetProvider(p provider.Provider) {
	e.provider = p
}

// Reflect records a reflection on a completed action
func (e *Engine) Reflect(ctx context.Context, context, decision, outcome string, learned string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	reflection := Reflection{
		ID:         generateID(),
		Timestamp:  time.Now(),
		Context:    context,
		Decision:   decision,
		Outcome:    outcome,
		Learned:    learned,
		Confidence: 0.5, // Start with neutral confidence
	}

	e.reflections = append(e.reflections, reflection)
	if len(e.reflections) > e.maxReflections {
		e.reflections = e.reflections[1:]
	}

	// Extract potential heuristics from this reflection
	e.extractHeuristics(reflection)

	// Persist periodically
	if len(e.reflections)%10 == 0 {
		go e.saveHeuristics()
	}
}

// extractHeuristics analyzes a reflection to extract generalizable rules
func (e *Engine) extractHeuristics(reflection Reflection) {
	// Simple pattern extraction - can be enhanced with LLM
	keywords := extractKeywords(reflection.Learned)

	for _, keyword := range keywords {
		key := heuristicKey(keyword, reflection.Decision)

		if _, ok := e.heuristics[key]; !ok {
			e.heuristics[key] = &Heuristic{
				ID:           key,
				Created:      time.Now(),
				Condition:    keyword,
				Recommendation: reflection.Decision,
				SuccessCount: 1,
				Confidence:   0.5,
			}
		} else {
			h := e.heuristics[key]
			if strings.Contains(strings.ToLower(reflection.Outcome), "success") {
				h.SuccessCount++
			} else {
				h.FailureCount++
			}
			h.Updated = time.Now()
			h.Confidence = float64(h.SuccessCount) / float64(h.SuccessCount + h.FailureCount)
		}
	}
}

// GetHeuristics returns applicable heuristics for a given context
func (e *Engine) GetHeuristics(context string) []Heuristic {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var applicable []Heuristic
	contextLower := strings.ToLower(context)

	for _, h := range e.heuristics {
		if strings.Contains(contextLower, strings.ToLower(h.Condition)) {
			applicable = append(applicable, *h)
		}
	}

	// Sort by confidence descending
	sort.Slice(applicable, func(i, j int) bool {
		return applicable[i].Confidence > applicable[j].Confidence
	})

	return applicable
}

// GetSimilarDecisions finds similar past decisions
func (e *Engine) GetSimilarDecisions(context string, limit int) []Reflection {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var similar []Reflection
	contextLower := strings.ToLower(context)

	for _, r := range e.reflections {
		if strings.Contains(contextLower, strings.ToLower(r.Context)) ||
		   strings.Contains(contextLower, strings.ToLower(r.Decision)) {
			similar = append(similar, r)
			if len(similar) >= limit {
				break
			}
		}
	}

	return similar
}

// DeepReflect uses LLM to analyze patterns and generate insights
func (e *Engine) DeepReflect(ctx context.Context) ([]Insight, error) {
	if e.provider == nil {
		return nil, fmt.Errorf("no provider configured for deep reflection")
	}

	e.mu.RLock()
	patterns := e.outcomeTracker.GetPatterns()
	recentFailures := e.outcomeTracker.GetRecentFailures(20)
	e.mu.RUnlock()

	if len(patterns) == 0 && len(recentFailures) == 0 {
		return nil, nil
	}

	// Build analysis prompt
	prompt := e.buildReflectionPrompt(patterns, recentFailures)

	resp, err := e.provider.Complete(ctx, prompt, provider.CompletionOptions{
		Temperature: 0.3,
		MaxTokens:   2000,
		JSON:        true,
	})
	if err != nil {
		return nil, fmt.Errorf("deep reflection request failed: %w", err)
	}

	insights, err := parseInsights(resp.Text)
	if err != nil {
		return nil, fmt.Errorf("parse insights: %w", err)
	}

	// Store insights as durable memory
	for _, insight := range insights {
		e.memory.Remember("decisions", "insight", insight.Title, insight.Content, "reflection-engine", insight.Tags...)
	}

	return insights, nil
}

// Insight represents a deep analytical insight
type Insight struct {
	Title       string   `json:"title"`
	Content     string   `json:"content"`
	Category    string   `json:"category"` // pattern, anti-pattern, recommendation
	Confidence  float64  `json:"confidence"`
	Tags        []string `json:"tags"`
}

func (e *Engine) buildReflectionPrompt(patterns []outcome.Pattern, failures []outcome.ActionOutcome) string {
	var sb strings.Builder

	sb.WriteString("Analyze the following patterns and failures to extract actionable insights.\n\n")

	sb.WriteString("## Recurring Failure Patterns\n")
	for _, p := range patterns {
		sb.WriteString(fmt.Sprintf("- %s (%s): %d occurrences\n", p.ErrorType, p.ActionType, p.Occurrences))
	}

	sb.WriteString("\n## Recent Failures\n")
	for _, f := range failures {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", f.ActionType, f.Error))
	}

	sb.WriteString("\nProvide a JSON response with insights in this format:\n")
	sb.WriteString(`{
  "insights": [
    {
      "title": "Brief title",
      "content": "Detailed insight",
      "category": "pattern|anti-pattern|recommendation",
      "confidence": 0.8,
      "tags": ["tag1", "tag2"]
    }
  ]
}`)

	return sb.String()
}

func parseInsights(text string) ([]Insight, error) {
	var response struct {
		Insights []Insight `json:"insights"`
	}

	// Extract JSON from response
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("no JSON found in response")
	}

	if err := json.Unmarshal([]byte(text[start:end+1]), &response); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	return response.Insights, nil
}

// GetReflectionSummary returns a summary of learned patterns
func (e *Engine) GetReflectionSummary() string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if len(e.heuristics) == 0 {
		return "No learned heuristics yet"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Learned %d heuristics from %d reflections:\n\n", len(e.heuristics), len(e.reflections)))

	// Show top heuristics by confidence
	var sortedHeuristics []*Heuristic
	for _, h := range e.heuristics {
		sortedHeuristics = append(sortedHeuristics, h)
	}
	sort.Slice(sortedHeuristics, func(i, j int) bool {
		return sortedHeuristics[i].Confidence > sortedHeuristics[j].Confidence
	})

	for i, h := range sortedHeuristics {
		if i >= 5 {
			break
		}
		sb.WriteString(fmt.Sprintf("- [%s] When %s: %s (%.0f%% confidence)\n",
			h.Condition, h.Condition, h.Recommendation, h.Confidence*100))
	}

	return sb.String()
}

// saveHeuristics persists heuristics to disk
func (e *Engine) saveHeuristics() error {
	if e.dataDir == "" {
		return nil
	}

	if err := os.MkdirAll(e.dataDir, 0755); err != nil {
		return err
	}

	path := filepath.Join(e.dataDir, "heuristics.json")
	data, _ := json.MarshalIndent(e.heuristics, "", "  ")
	return os.WriteFile(path, data, 0644)
}

// loadHeuristics loads heuristics from disk
func (e *Engine) loadHeuristics() error {
	if e.dataDir == "" {
		return nil
	}

	path := filepath.Join(e.dataDir, "heuristics.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &e.heuristics)
}

// extractKeywords extracts key terms from text for pattern matching
func extractKeywords(text string) []string {
	// Simple keyword extraction - can be enhanced
	words := strings.Fields(strings.ToLower(text))
	keywords := make([]string, 0)
	seen := make(map[string]bool)

	for _, word := range words {
		word = strings.Trim(word, ".,;:()[]{}\"'")
		if len(word) >= 4 && !seen[word] && !isStopWord(word) {
			keywords = append(keywords, word)
			seen[word] = true
		}
	}

	return keywords
}

func isStopWord(word string) bool {
	stopWords := map[string]bool{
		"this": true, "that": true, "with": true, "have": true,
		"been": true, "were": true, "they": true, "their": true,
		"what": true, "when": true, "where": true, "which": true,
	}
	return stopWords[word]
}

func heuristicKey(keyword, decision string) string {
	// Create a simple hash-based key
	return fmt.Sprintf("%s_%s", keyword, decision[:min(20, len(decision))])
}

func generateID() string {
	return fmt.Sprintf("ref_%d", time.Now().UnixNano())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
