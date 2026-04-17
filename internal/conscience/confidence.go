package conscience

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/marcus-ai/marcus/internal/memory"
	"github.com/marcus-ai/marcus/internal/outcome"
	"github.com/marcus-ai/marcus/internal/provider"
)

// Scorer calculates confidence levels for decisions and actions
type Scorer struct {
	mu             sync.RWMutex
	memory         *memory.Manager
	outcomeTracker *outcome.Tracker
	provider       provider.Provider
	history        []ConfidenceRecord
	maxHistory     int
}

// ConfidenceRecord tracks a confidence assessment
type ConfidenceRecord struct {
	Timestamp       time.Time  `json:"timestamp"`
	Task            string     `json:"task"`
	PredictedConf   float64    `json:"predicted_confidence"`
	ActualSuccess   bool       `json:"actual_success"`
	Factors         []string   `json:"factors"`
	CalibrationUsed bool       `json:"calibration_used"`
}

// ConfidenceFactors are the factors affecting confidence
type ConfidenceFactors struct {
	// Historical success rate for similar tasks (0-1)
	HistoricalRate float64

	// Context completeness (0-1) - how much context is available
	ContextComplete float64

	// Task complexity (0-1) - lower is better
	Complexity float64

	// Tool availability (0-1) - are required tools available
	ToolAvailability float64

	// Time pressure (0-1) - lower is better
	TimePressure float64

	// Ambiguity (0-1) - lower is better
	Ambiguity float64
}

// ConfidenceAssessment is the result of confidence calculation
type ConfidenceAssessment struct {
	Confidence    float64              `json:"confidence"` // 0-1
	Level         ConfidenceLevel      `json:"level"`
	Factors       ConfidenceFactors    `json:"factors"`
	Recommendation ConfidenceRecommendation `json:"recommendation"`
	Reasoning     []string             `json:"reasoning"`
}

// ConfidenceRecommendation is what MARCUS should do
type ConfidenceRecommendation string

const (
	RecommendProceed     ConfidenceRecommendation = "proceed"
	RecommendVerify      ConfidenceRecommendation = "verify_first"
	RecommendAsk         ConfidenceRecommendation = "ask_user"
	RecommendResearch    ConfidenceRecommendation = "research_first"
	RecommendDecline     ConfidenceRecommendation = "decline"
)

// NewScorer creates a new confidence scorer
func NewScorer(mem *memory.Manager, tracker *outcome.Tracker) *Scorer {
	return &Scorer{
		memory:         mem,
		outcomeTracker: tracker,
		history:        make([]ConfidenceRecord, 0),
		maxHistory:     1000,
	}
}

// SetProvider sets the LLM provider for complex assessments
func (s *Scorer) SetProvider(p provider.Provider) {
	s.provider = p
}

// Assess calculates confidence for a task
func (s *Scorer) Assess(ctx context.Context, task string, factors ConfidenceFactors) ConfidenceAssessment {
	s.mu.Lock()
	defer s.mu.Unlock()

	assessment := ConfidenceAssessment{
		Factors: factors,
		Reasoning: make([]string, 0),
	}

	// Calculate base confidence from factors
	assessment.Confidence = s.calculateBaseConfidence(factors)
	assessment.Reasoning = append(assessment.Reasoning,
		fmt.Sprintf("Base confidence from factors: %.2f", assessment.Confidence))

	// Adjust based on historical performance
	historicalAdjustment := s.getHistoricalAdjustment(task)
	assessment.Confidence += historicalAdjustment
	assessment.Confidence = clamp(assessment.Confidence, 0, 1)

	if historicalAdjustment != 0 {
		assessment.Reasoning = append(assessment.Reasoning,
			fmt.Sprintf("Historical adjustment: %.2f", historicalAdjustment))
	}

	// Determine level
	assessment.Level = confidenceFromScore(assessment.Confidence)

	// Determine recommendation
	assessment.Recommendation = s.getRecommendation(assessment)

	// Record assessment
	s.history = append(s.history, ConfidenceRecord{
		Timestamp:      time.Now(),
		Task:           task,
		PredictedConf:  assessment.Confidence,
		Factors:        s.factorsToStrings(factors),
		CalibrationUsed: historicalAdjustment != 0,
	})

	if len(s.history) > s.maxHistory {
		s.history = s.history[1:]
	}

	return assessment
}

// RecordOutcome records the actual outcome for calibration
func (s *Scorer) RecordOutcome(task string, predictedConf float64, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update last record for this task
	for i := len(s.history) - 1; i >= 0; i-- {
		if s.history[i].Task == task {
			s.history[i].ActualSuccess = success
			break
		}
	}
}

// GetCalibrationStats returns calibration statistics
func (s *Scorer) GetCalibrationStats() CalibrationStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := CalibrationStats{
		TotalAssessments: len(s.history),
	}

	if len(s.history) == 0 {
		return stats
	}

	// Calculate Brier score (lower is better)
	var brierScore float64
	correct := 0

	for _, record := range s.history {
		if record.ActualSuccess {
			correct++
			// Brier: (predicted - actual)^2, where actual=1 for success
			brierScore += math.Pow(record.PredictedConf-1, 2)
		} else {
			// Brier: (predicted - actual)^2, where actual=0 for failure
			brierScore += math.Pow(record.PredictedConf-0, 2)
		}
	}

	stats.BrierScore = brierScore / float64(len(s.history))
	stats.SuccessRate = float64(correct) / float64(len(s.history))

	// Calculate calibration by confidence buckets
	buckets := map[string]struct {
		total   int
		success int
	}{
		"high":     {0, 0},
		"medium":   {0, 0},
		"low":      {0, 0},
	}

	for _, record := range s.history {
		if record.PredictedConf >= 0.7 {
			b := buckets["high"]
			b.total++
			if record.ActualSuccess {
				b.success++
			}
			buckets["high"] = b
		} else if record.PredictedConf >= 0.4 {
			b := buckets["medium"]
			b.total++
			if record.ActualSuccess {
				b.success++
			}
			buckets["medium"] = b
		} else {
			b := buckets["low"]
			b.total++
			if record.ActualSuccess {
				b.success++
			}
			buckets["low"] = b
		}
	}

	for name, b := range buckets {
		if b.total > 0 {
			stats.Calibration[name] = float64(b.success) / float64(b.total)
		}
	}

	return stats
}

// CalibrationStats holds calibration statistics
type CalibrationStats struct {
	TotalAssessments int                `json:"total_assessments"`
	BrierScore       float64            `json:"brier_score"`
	SuccessRate      float64            `json:"success_rate"`
	Calibration      map[string]float64 `json:"calibration_by_bucket"`
}

func (s *Scorer) calculateBaseConfidence(factors ConfidenceFactors) float64 {
	// Weighted combination of factors
	weights := map[string]float64{
		"historical":      0.30,
		"context":         0.20,
		"complexity":      0.20,
		"tools":           0.15,
		"time":            0.05,
		"ambiguity":       0.10,
	}

	confidence := 0.0

	// Historical rate (direct contribution)
	confidence += factors.HistoricalRate * weights["historical"]

	// Context completeness (direct contribution)
	confidence += factors.ContextComplete * weights["context"]

	// Complexity (inverse - lower complexity = higher confidence)
	confidence += (1 - factors.Complexity) * weights["complexity"]

	// Tool availability (direct contribution)
	confidence += factors.ToolAvailability * weights["tools"]

	// Time pressure (inverse - lower pressure = higher confidence)
	confidence += (1 - factors.TimePressure) * weights["time"]

	// Ambiguity (inverse - lower ambiguity = higher confidence)
	confidence += (1 - factors.Ambiguity) * weights["ambiguity"]

	return confidence
}

func (s *Scorer) getHistoricalAdjustment(task string) float64 {
	if s.outcomeTracker == nil {
		return 0
	}

	// Get stats for relevant action types
	stats := s.outcomeTracker.GetAllStats()
	if len(stats) == 0 {
		return 0
	}

	// Find relevant stats based on task keywords
	taskLower := strings.ToLower(task)
	var relevantSuccessRate float64
	var relevantCount int

	for actionType, actionStats := range stats {
		if strings.Contains(taskLower, string(actionType)) {
			relevantSuccessRate += actionStats.SuccessRate() / 100 // Convert to 0-1
			relevantCount++
		}
	}

	if relevantCount == 0 {
		return 0
	}

	avgSuccessRate := relevantSuccessRate / float64(relevantCount)

	// Adjustment: if historical rate is below 50%, reduce confidence
	if avgSuccessRate < 0.5 {
		return (avgSuccessRate - 0.5) * 0.4 // Max adjustment of -0.2
	} else if avgSuccessRate > 0.8 {
		return (avgSuccessRate - 0.8) * 0.2 // Max adjustment of +0.04
	}

	return 0
}

func (s *Scorer) getRecommendation(assessment ConfidenceAssessment) ConfidenceRecommendation {
	switch assessment.Level {
	case ConfidenceLevelHigh:
		return RecommendProceed
	case ConfidenceLevelMedium:
		if assessment.Factors.Ambiguity > 0.5 {
			return RecommendVerify
		}
		return RecommendProceed
	case ConfidenceLevelLow:
		if assessment.Factors.HistoricalRate < 0.4 {
			return RecommendAsk
		}
		return RecommendResearch
	case ConfidenceLevelVeryLow:
		return RecommendDecline
	default:
		return RecommendResearch
	}
}

func (s *Scorer) factorsToStrings(factors ConfidenceFactors) []string {
	var result []string

	if factors.HistoricalRate > 0 {
		result = append(result, fmt.Sprintf("historical_rate=%.2f", factors.HistoricalRate))
	}
	if factors.ContextComplete > 0 {
		result = append(result, fmt.Sprintf("context=%.2f", factors.ContextComplete))
	}
	if factors.Complexity > 0 {
		result = append(result, fmt.Sprintf("complexity=%.2f", factors.Complexity))
	}
	if factors.ToolAvailability > 0 {
		result = append(result, fmt.Sprintf("tools=%.2f", factors.ToolAvailability))
	}

	return result
}

func confidenceFromScore(score float64) ConfidenceLevel {
	if score >= 0.8 {
		return ConfidenceLevelHigh
	} else if score >= 0.6 {
		return ConfidenceLevelMedium
	} else if score >= 0.4 {
		return ConfidenceLevelLow
	}
	return ConfidenceLevelVeryLow
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// GetConfidenceSummary returns a summary of confidence assessments
func (s *Scorer) GetConfidenceSummary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.GetCalibrationStats()

	var sb strings.Builder
	sb.WriteString("Confidence Scorer Summary:\n\n")
	sb.WriteString(fmt.Sprintf("Total assessments: %d\n", stats.TotalAssessments))
	sb.WriteString(fmt.Sprintf("Overall success rate: %.1f%%\n", stats.SuccessRate*100))
	sb.WriteString(fmt.Sprintf("Brier score: %.3f (lower is better)\n\n", stats.BrierScore))

	if len(stats.Calibration) > 0 {
		sb.WriteString("Calibration by confidence level:\n")

		// Sort bucket names for consistent output
		bucketNames := make([]string, 0, len(stats.Calibration))
		for name := range stats.Calibration {
			bucketNames = append(bucketNames, name)
		}
		sort.Strings(bucketNames)

		for _, name := range bucketNames {
			if cal, ok := stats.Calibration[name]; ok {
				sb.WriteString(fmt.Sprintf("  %s confidence: %.1f%% actual success\n",
					strings.Title(name), cal*100))
			}
		}
	}

	return sb.String()
}
