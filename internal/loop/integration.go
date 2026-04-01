package loop

import (
	"context"
	"fmt"
	"time"

	"github.com/marcus-ai/marcus/internal/agent"
	"github.com/marcus-ai/marcus/internal/conscience"
	"github.com/marcus-ai/marcus/internal/flow"
	"github.com/marcus-ai/marcus/internal/folder"
	"github.com/marcus-ai/marcus/internal/memory"
	"github.com/marcus-ai/marcus/internal/outcome"
	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/reflection"
	"github.com/marcus-ai/marcus/internal/trigger"
)

// EnhancedLoopEngine wraps flow.LoopEngine with Phase 1 capabilities
type EnhancedLoopEngine struct {
	baseEngine       *flow.LoopEngine
	outcomeTracker   *outcome.Tracker
	reflectionEngine *reflection.Engine
	swarmCoordinator *agent.SwarmCoordinator
	capabilityRegistry *conscience.Registry
	confidenceScorer *conscience.Scorer
	triggerEngine    *trigger.Engine
	protocol         *agent.Protocol
	blackboard       *agent.Blackboard
	config           EnhancedConfig
}

// EnhancedConfig holds configuration for enhanced features
type EnhancedConfig struct {
	EnableOutcomeTracking   bool `toml:"enable_outcome_tracking"`
	EnableReflection        bool `toml:"enable_reflection"`
	EnableSwarm             bool `toml:"enable_swarm"`
	EnableConfidenceScoring bool `toml:"enable_confidence_scoring"`
	EnableTriggers          bool `toml:"enable_triggers"`
	MaxSwarms               int  `toml:"max_swarms"`
	MaxOutcomes             int  `toml:"max_outcomes"`
	ReflectionInterval      int  `toml:"reflection_interval_minutes"`
}

// DefaultEnhancedConfig returns default configuration
func DefaultEnhancedConfig() EnhancedConfig {
	return EnhancedConfig{
		EnableOutcomeTracking:   true,
		EnableReflection:        true,
		EnableSwarm:             true,
		EnableConfidenceScoring: true,
		EnableTriggers:          true,
		MaxSwarms:               5,
		MaxOutcomes:             10000,
		ReflectionInterval:      30, // Deep reflection every 30 minutes
	}
}

// NewEnhancedLoopEngine creates a new enhanced loop engine
func NewEnhancedLoopEngine(
	baseEngine *flow.LoopEngine,
	folder *folder.FolderEngine,
	cfg *config.Config,
	mem *memory.Manager,
	dataDir string,
) *EnhancedLoopEngine {
	enhancedCfg := DefaultEnhancedConfig()

	// Create outcome tracker
	tracker := outcome.NewTracker(dataDir)

	// Create reflection engine
	reflectionEng := reflection.NewEngine(dataDir, mem, tracker)

	// Create capability registry
	capRegistry := conscience.NewRegistry(dataDir)

	// Create confidence scorer
	scorer := conscience.NewScorer(mem, tracker)

	// Create protocol and blackboard for agent communication
	protocol := agent.NewProtocol(100)
	blackboard := agent.NewBlackboard(500)

	// Create swarm coordinator
	swarmCoord := agent.NewSwarmCoordinator(
		agent.NewInMemoryAgentRegistry(folder, cfg, dataDir),
		protocol,
		blackboard,
		enhancedCfg.MaxSwarms,
	)

	// Create trigger engine
	triggerEng := trigger.NewEngine(dataDir, folder, cfg, mem, nil)

	// Register built-in capabilities
	capRegistry.RegisterCapability(conscience.Capability{
		ID:          "cap_read_file",
		Name:        "read_file",
		Type:        conscience.CapabilityTool,
		Description: "Read file contents",
		Proficiency: 0.9,
		Tags:        []string{"file", "read"},
	})
	capRegistry.RegisterCapability(conscience.Capability{
		ID:          "cap_write_file",
		Name:        "write_file",
		Type:        conscience.CapabilityTool,
		Description: "Write file contents",
		Proficiency: 0.85,
		Tags:        []string{"file", "write"},
	})
	capRegistry.RegisterCapability(conscience.Capability{
		ID:          "cap_run_command",
		Name:        "run_command",
		Type:        conscience.CapabilityTool,
		Description: "Execute shell commands",
		Proficiency: 0.8,
		Tags:        []string{"shell", "command"},
	})
	capRegistry.RegisterCapability(conscience.Capability{
		ID:          "cap_search_code",
		Name:        "search_code",
		Type:        conscience.CapabilityTool,
		Description: "Search code for patterns",
		Proficiency: 0.85,
		Tags:        []string{"search", "code"},
	})

	e := &EnhancedLoopEngine{
		baseEngine:         baseEngine,
		outcomeTracker:     tracker,
		reflectionEngine:   reflectionEng,
		swarmCoordinator:   swarmCoord,
		capabilityRegistry: capRegistry,
		confidenceScorer:   scorer,
		triggerEngine:      triggerEng,
		protocol:           protocol,
		blackboard:         blackboard,
		config:             enhancedCfg,
	}

	// Start periodic deep reflection
	if enhancedCfg.EnableReflection {
		go e.startPeriodicReflection()
	}

	return e
}

// Run executes a task with enhanced capabilities
func (e *EnhancedLoopEngine) Run(ctx context.Context, goal, taskID string, maxIterations int) (*flow.LoopState, error) {
	// Assess confidence before starting
	if e.config.EnableConfidenceScoring {
		assessment := e.confidenceScorer.Assess(ctx, goal, conscience.ConfidenceFactors{
			HistoricalRate:    e.getHistoricalSuccessRate(goal),
			ContextComplete:   0.8, // Would be calculated from actual context
			Complexity:        e.estimateComplexity(goal),
			ToolAvailability:  1.0, // All tools available
			TimePressure:      0.2, // Low time pressure
			Ambiguity:         e.estimateAmbiguity(goal),
		})

		// If confidence is very low, consider asking for help
		if assessment.Level == conscience.ConfidenceLevelVeryLow {
			canDo, cap := e.capabilityRegistry.CanDo(goal, 0.5)
			if !canDo {
				shouldAsk, reason := e.capabilityRegistry.ShouldAskForHelp(goal)
				if shouldAsk {
					capName := ""
					if cap != nil {
						capName = cap.Name
					}
					return nil, fmt.Errorf("low confidence for task: %s. Consider asking for help: %s", reason, capName)
				}
			}
		}
	}

	// Check if this is a complex goal that needs a swarm
	if e.config.EnableSwarm && e.isComplexGoal(goal) {
		return e.runSwarm(ctx, goal, taskID)
	}

	// Run base engine
	state, err := e.baseEngine.Run(ctx, goal, taskID, maxIterations)

	// Record outcome
	if e.config.EnableOutcomeTracking && state != nil {
		e.recordOutcome(goal, taskID, state)
	}

	// Reflect on the execution
	if e.config.EnableReflection && state != nil {
		e.reflectOnExecution(ctx, goal, state)
	}

	return state, err
}

// runSwarm executes a complex goal using multiple agents
func (e *EnhancedLoopEngine) runSwarm(ctx context.Context, goal, taskID string) (*flow.LoopState, error) {
	// Recommend agent composition
	roles := e.swarmCoordinator.RecommendSwarmComposition(goal)

	// Spawn swarm
	swarm, err := e.swarmCoordinator.SpawnSwarm(ctx, goal, roles)
	if err != nil {
		return nil, fmt.Errorf("spawn swarm: %w", err)
	}

	// Wait for swarm completion (simplified - real implementation would monitor)
	timeout := time.After(10 * time.Minute)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.swarmCoordinator.TerminateSwarm(swarm.ID, "context cancelled")
			return nil, ctx.Err()
		case <-timeout:
			e.swarmCoordinator.TerminateSwarm(swarm.ID, "timeout")
			return nil, fmt.Errorf("swarm execution timeout")
		case <-ticker.C:
			swarmObj, ok := e.swarmCoordinator.GetSwarm(swarm.ID)
			if !ok {
				continue
			}
			if swarmObj.Status == "completed" {
				// Swarm completed - create summary state
				return &flow.LoopState{
					Done:         true,
					FinishReason: "swarm_completed",
					Progress:     fmt.Sprintf("Swarm completed with %d agents", len(swarmObj.Agents)),
				}, nil
			} else if swarmObj.Status == "failed" {
				return &flow.LoopState{
					Done:         true,
					FinishReason: "swarm_failed",
					Progress:     "Swarm execution failed",
				}, fmt.Errorf("swarm execution failed")
			}
		}
	}
}

// recordOutcome records the outcome of an execution
func (e *EnhancedLoopEngine) recordOutcome(goal, taskID string, state *flow.LoopState) {
	success := state.Done && state.FinishReason != "error"
	duration := time.Since(time.Now()) // Would track actual start time

	// Determine action types used from tool results
	var actionTypes []outcome.ActionType
	for _, result := range state.ToolResults {
		actionTypes = append(actionTypes, outcome.ActionType(result.ToolName))
	}

	// Record outcome for each tool used
	for _, actionType := range actionTypes {
		e.outcomeTracker.RecordOutcome(outcome.ActionOutcome{
			ID:         generateOutcomeID(),
			Timestamp:  time.Now(),
			ActionType: actionType,
			Context:    goal,
			Success:    success,
			Duration:   duration,
			Tags:       []string{"task:" + taskID},
		})
	}
}

// reflectOnExecution performs reflection on completed execution
func (e *EnhancedLoopEngine) reflectOnExecution(ctx context.Context, goal string, state *flow.LoopState) {
	// Extract key decision points
	for _, decision := range state.Decisions {
		outcome := "success"
		if !state.Done || state.FinishReason == "error" {
			outcome = "failure"
		}

		learned := e.extractLearning(decision, state)
		if learned != "" {
			e.reflectionEngine.Reflect(ctx, goal, decision.ModelText, outcome, learned)
		}
	}
}

// extractLearning extracts a learning point from a decision
func (e *EnhancedLoopEngine) extractLearning(decision flow.DecisionLogEntry, state *flow.LoopState) string {
	// Simple extraction - would be enhanced with LLM analysis
	if len(decision.Actions) == 0 {
		return ""
	}

	if state.FinishReason == "error" {
		return fmt.Sprintf("Actions %v led to error: %s", decision.Actions, state.FinishReason)
	}

	if state.Done {
		return fmt.Sprintf("Actions %v successfully completed the task", decision.Actions)
	}

	return ""
}

// startPeriodicReflection starts background periodic deep reflection
func (e *EnhancedLoopEngine) startPeriodicReflection() {
	interval := time.Duration(e.config.ReflectionInterval) * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()
		insights, err := e.reflectionEngine.DeepReflect(ctx)
		if err != nil {
			continue
		}
		if len(insights) > 0 {
			// Record insights to memory
			for _, insight := range insights {
				_ = insight // Insight already recorded to memory by DeepReflect
			}
			_ = e.reflectionEngine.GetReflectionSummary()
		}
	}
}

// getHistoricalSuccessRate gets historical success rate for similar tasks
func (e *EnhancedLoopEngine) getHistoricalSuccessRate(goal string) float64 {
	stats := e.outcomeTracker.GetAllStats()
	if len(stats) == 0 {
		return 0.5 // Neutral
	}

	var total, success float64
	for _, s := range stats {
		total += float64(s.TotalAttempts)
		success += float64(s.Successful)
	}

	if total == 0 {
		return 0.5
	}

	return success / total
}

// estimateComplexity estimates task complexity (0-1, higher is more complex)
func (e *EnhancedLoopEngine) estimateComplexity(goal string) float64 {
	// Simple heuristic based on goal length and keywords
	complexity := 0.3 // Base complexity

	complexKeywords := []string{"implement", "design", "architecture", "refactor", "migrate", "integrate"}
	for _, keyword := range complexKeywords {
		if contains(goal, keyword) {
			complexity += 0.1
		}
	}

	// Length factor
	if len(goal) > 100 {
		complexity += 0.1
	}
	if len(goal) > 200 {
		complexity += 0.1
	}

	return min(complexity, 1.0)
}

// estimateAmbiguity estimates task ambiguity (0-1, higher is more ambiguous)
func (e *EnhancedLoopEngine) estimateAmbiguity(goal string) float64 {
	// Check for ambiguous language
	ambiguousWords := []string{"maybe", "perhaps", "try", "see if", "check", "look into"}
	ambiguity := 0.2 // Base ambiguity

	for _, word := range ambiguousWords {
		if contains(goal, word) {
			ambiguity += 0.15
		}
	}

	// Check for missing specifics
	if !contains(goal, "file") && !contains(goal, "function") && !contains(goal, "test") {
		ambiguity += 0.2
	}

	return min(ambiguity, 1.0)
}

// isComplexGoal determines if a goal requires swarm execution
func (e *EnhancedLoopEngine) isComplexGoal(goal string) bool {
	// Check for multi-step indicators
	complexIndicators := []string{
		"implement feature",
		"add support for",
		"create new",
		"design and implement",
		"build a",
		"set up",
		"integrate",
	}

	for _, indicator := range complexIndicators {
		if contains(goal, indicator) {
			return true
		}
	}

	return false
}

// GetOutcomeTracker returns the outcome tracker
func (e *EnhancedLoopEngine) GetOutcomeTracker() *outcome.Tracker {
	return e.outcomeTracker
}

// GetReflectionEngine returns the reflection engine
func (e *EnhancedLoopEngine) GetReflectionEngine() *reflection.Engine {
	return e.reflectionEngine
}

// GetSwarmCoordinator returns the swarm coordinator
func (e *EnhancedLoopEngine) GetSwarmCoordinator() *agent.SwarmCoordinator {
	return e.swarmCoordinator
}

// GetCapabilityRegistry returns the capability registry
func (e *EnhancedLoopEngine) GetCapabilityRegistry() *conscience.Registry {
	return e.capabilityRegistry
}

// GetConfidenceScorer returns the confidence scorer
func (e *EnhancedLoopEngine) GetConfidenceScorer() *conscience.Scorer {
	return e.confidenceScorer
}

// GetTriggerEngine returns the trigger engine
func (e *EnhancedLoopEngine) GetTriggerEngine() *trigger.Engine {
	return e.triggerEngine
}

// GetSummary returns a summary of all enhanced features
func (e *EnhancedLoopEngine) GetSummary() string {
	var summary string

	summary += "=== Phase 1 Enhanced Loop Engine ===\n\n"

	summary += "Outcome Tracking:\n"
	summary += e.outcomeTracker.GetSummary()
	summary += "\n"

	summary += "Reflection:\n"
	summary += e.reflectionEngine.GetReflectionSummary()
	summary += "\n"

	summary += "Capability Registry:\n"
	summary += e.capabilityRegistry.GetSummary()
	summary += "\n"

	summary += "Confidence Scoring:\n"
	summary += e.confidenceScorer.GetConfidenceSummary()
	summary += "\n"

	return summary
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func generateOutcomeID() string {
	return "outcome_" + time.Now().Format("20060102150405.000000")
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
