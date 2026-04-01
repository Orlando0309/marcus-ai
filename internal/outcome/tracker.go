package outcome

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ActionType represents a category of action
type ActionType string

const (
	ActionReadFile      ActionType = "read_file"
	ActionWriteFile     ActionType = "write_file"
	ActionEditFile      ActionType = "edit_file"
	ActionRunCommand    ActionType = "run_command"
	ActionSearchCode    ActionType = "search_code"
	ActionGlobFiles     ActionType = "glob_files"
	ActionFetchURL      ActionType = "fetch_url"
	ActionCreateTask    ActionType = "create_task"
	ActionUpdateTask    ActionType = "update_task"
	ActionGitOperation  ActionType = "git_operation"
)

// ActionOutcome records the result of a single action
type ActionOutcome struct {
	ID          string            `json:"id"`
	Timestamp   time.Time         `json:"timestamp"`
	ActionType  ActionType        `json:"action_type"`
	Context     string            `json:"context,omitempty"` // Goal or task context
	Input       map[string]any    `json:"input,omitempty"`
	Success     bool              `json:"success"`
	Error       string            `json:"error,omitempty"`
	Duration    time.Duration     `json:"duration_ms"`
	RetryCount  int               `json:"retry_count"`
	SelfCorrect bool              `json:"self_corrected"`
	Tags        []string          `json:"tags,omitempty"` // e.g., ["file-write", "test", "refactor"]
}

// ToolStats tracks statistics for a single tool/action type
type ToolStats struct {
	ActionType       ActionType `json:"action_type"`
	TotalAttempts    int        `json:"total_attempts"`
	Successful       int        `json:"successful"`
	Failed           int        `json:"failed"`
	SelfCorrected    int        `json:"self_corrected"`
	TotalDuration    int64      `json:"total_duration_ms"` // milliseconds
	LastUsed         time.Time  `json:"last_used"`
	RecentFailures   int        `json:"recent_failures"` // failures in last 10 attempts
}

// SuccessRate returns the success rate as a percentage (0-100)
func (s *ToolStats) SuccessRate() float64 {
	if s.TotalAttempts == 0 {
		return 0
	}
	return float64(s.Successful) / float64(s.TotalAttempts) * 100
}

// AverageDuration returns average execution duration
func (s *ToolStats) AverageDuration() time.Duration {
	if s.TotalAttempts == 0 {
		return 0
	}
	return time.Duration(s.TotalDuration/int64(s.TotalAttempts)) * time.Millisecond
}

// PerformanceScore returns a composite score (0-100) considering success rate, recency, and self-correction
func (s *ToolStats) PerformanceScore() float64 {
	if s.TotalAttempts == 0 {
		return 50 // Neutral score for untested tools
	}

	// Base score from success rate (0-60 points)
	successScore := s.SuccessRate() * 0.6

	// Recency bonus (0-20 points) - tools used recently get boost
	recencyScore := 0.0
	if time.Since(s.LastUsed) < 24*time.Hour {
		recencyScore = 20
	} else if time.Since(s.LastUsed) < 7*24*time.Hour {
		recencyScore = 10
	}

	// Self-correction penalty (0-20 points) - tools that need correction often are less reliable
	correctionPenalty := 0.0
	if s.TotalAttempts > 0 {
		correctionRate := float64(s.SelfCorrected) / float64(s.TotalAttempts)
		correctionPenalty = correctionRate * 20
	}

	// Recent failure penalty (0-20 points)
	recentFailurePenalty := float64(s.RecentFailures) * 2
	if recentFailurePenalty > 20 {
		recentFailurePenalty = 20
	}

	return successScore + recencyScore - correctionPenalty - recentFailurePenalty
}

// Tracker tracks action outcomes and provides performance insights
type Tracker struct {
	mu          sync.RWMutex
	dataDir     string
	outcomes    []ActionOutcome
	stats       map[ActionType]*ToolStats
	maxOutcomes int // Keep last N outcomes in memory
}

// NewTracker creates a new outcome tracker
func NewTracker(dataDir string) *Tracker {
	t := &Tracker{
		dataDir:     filepath.Join(dataDir, "outcomes"),
		outcomes:    make([]ActionOutcome, 0),
		stats:       make(map[ActionType]*ToolStats),
		maxOutcomes: 10000,
	}
	t.loadStats()
	return t
}

// RecordOutcome records the outcome of an action
func (t *Tracker) RecordOutcome(outcome ActionOutcome) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Initialize stats if needed
	if _, ok := t.stats[outcome.ActionType]; !ok {
		t.stats[outcome.ActionType] = &ToolStats{
			ActionType: outcome.ActionType,
		}
	}

	// Update stats
	stats := t.stats[outcome.ActionType]
	stats.TotalAttempts++
	if outcome.Success {
		stats.Successful++
	} else {
		stats.Failed++
	}
	if outcome.SelfCorrect {
		stats.SelfCorrected++
	}
	stats.TotalDuration += int64(outcome.Duration / time.Millisecond)
	stats.LastUsed = outcome.Timestamp

	// Track recent failures (sliding window of 10)
	if !outcome.Success {
		stats.RecentFailures++
	}
	if stats.TotalAttempts%10 == 0 && stats.TotalAttempts > 0 {
		// Reset recent failures every 10 attempts to keep sliding window
		stats.RecentFailures = 0
	}

	// Store outcome
	t.outcomes = append(t.outcomes, outcome)
	if len(t.outcomes) > t.maxOutcomes {
		t.outcomes = t.outcomes[1:]
	}

	// Persist periodically
	if len(t.outcomes)%100 == 0 {
		go t.saveStats()
	}
}

// GetStats returns stats for a specific action type
func (t *Tracker) GetStats(actionType ActionType) *ToolStats {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.stats[actionType]
}

// GetAllStats returns stats for all action types
func (t *Tracker) GetAllStats() map[ActionType]*ToolStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[ActionType]*ToolStats, len(t.stats))
	for k, v := range t.stats {
		result[k] = v
	}
	return result
}

// GetRankedTools returns tools ranked by performance score
func (t *Tracker) GetRankedTools() []ToolStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var tools []ToolStats
	for _, stats := range t.stats {
		tools = append(tools, *stats)
	}

	// Sort by performance score descending
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].PerformanceScore() > tools[j].PerformanceScore()
	})

	return tools
}

// GetRecentFailures returns recent failed outcomes for analysis
func (t *Tracker) GetRecentFailures(limit int) []ActionOutcome {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var failures []ActionOutcome
	for i := len(t.outcomes) - 1; i >= 0 && len(failures) < limit; i-- {
		if !t.outcomes[i].Success {
			failures = append(failures, t.outcomes[i])
		}
	}

	return failures
}

// GetPatterns identifies common failure patterns
func (t *Tracker) GetPatterns() []Pattern {
	t.mu.RLock()
	defer t.mu.RUnlock()

	patterns := make(map[string]*Pattern)

	for _, outcome := range t.outcomes {
		if !outcome.Success && outcome.Error != "" {
			// Group by error type
			errorKey := string(outcome.ActionType) + ":" + truncateString(outcome.Error, 50)
			if _, ok := patterns[errorKey]; !ok {
				patterns[errorKey] = &Pattern{
					ErrorType:   truncateString(outcome.Error, 50),
					ActionType:  outcome.ActionType,
					Occurrences: 0,
					Examples:    make([]string, 0),
				}
			}
			patterns[errorKey].Occurrences++
			if len(patterns[errorKey].Examples) < 3 {
				patterns[errorKey].Examples = append(patterns[errorKey].Examples, outcome.Context)
			}
		}
	}

	var result []Pattern
	for _, p := range patterns {
		if p.Occurrences >= 2 { // Only patterns that occur multiple times
			result = append(result, *p)
		}
	}

	// Sort by occurrence count
	sort.Slice(result, func(i, j int) bool {
		return result[i].Occurrences > result[j].Occurrences
	})

	return result
}

// Pattern represents a recurring failure pattern
type Pattern struct {
	ErrorType   string     `json:"error_type"`
	ActionType  ActionType `json:"action_type"`
	Occurrences int        `json:"occurrences"`
	Examples    []string   `json:"examples,omitempty"`
}

// GetSuccessRateForContext returns success rate for a specific context/tag
func (t *Tracker) GetSuccessRateForContext(tag string) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var total, success int
	for _, outcome := range t.outcomes {
		for _, t := range outcome.Tags {
			if t == tag {
				total++
				if outcome.Success {
					success++
				}
			}
		}
	}

	if total == 0 {
		return 0
	}
	return float64(success) / float64(total) * 100
}

// saveStats persists stats to disk
func (t *Tracker) saveStats() error {
	if t.dataDir == "" {
		return nil
	}

	if err := os.MkdirAll(t.dataDir, 0755); err != nil {
		return err
	}

	// Save stats
	statsData := make(map[string]*ToolStats)
	for k, v := range t.stats {
		statsData[string(k)] = v
	}

	statsPath := filepath.Join(t.dataDir, "stats.json")
	data, _ := json.MarshalIndent(statsData, "", "  ")
	if err := os.WriteFile(statsPath, data, 0644); err != nil {
		return err
	}

	return nil
}

// loadStats loads stats from disk
func (t *Tracker) loadStats() error {
	if t.dataDir == "" {
		return nil
	}

	statsPath := filepath.Join(t.dataDir, "stats.json")
	data, err := os.ReadFile(statsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var statsData map[string]*ToolStats
	if err := json.Unmarshal(data, &statsData); err != nil {
		return err
	}

	for k, v := range statsData {
		t.stats[ActionType(k)] = v
	}

	return nil
}

// GetSummary returns a human-readable summary of tool performance
func (t *Tracker) GetSummary() string {
	stats := t.GetRankedTools()
	if len(stats) == 0 {
		return "No outcome data collected yet"
	}

	var summary strings.Builder
	for _, s := range stats {
		summary.WriteString(string(s.ActionType))
		summary.WriteString(": ")
		summary.WriteString(fmt.Sprintf("%.1f%% success", s.SuccessRate()))
		if s.SelfCorrected > 0 {
			summary.WriteString(fmt.Sprintf(" (%d self-corrections)", s.SelfCorrected))
		}
		summary.WriteString("\n")
	}

	return summary.String()
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
