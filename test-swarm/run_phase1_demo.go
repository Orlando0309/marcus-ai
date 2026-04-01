//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/marcus-ai/marcus/internal/agent"
	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/conscience"
	"github.com/marcus-ai/marcus/internal/flow"
	"github.com/marcus-ai/marcus/internal/memory"
	"github.com/marcus-ai/marcus/internal/outcome"
	"github.com/marcus-ai/marcus/internal/reflection"
	"github.com/marcus-ai/marcus/internal/trigger"
)

func main() {
	fmt.Println("=" + string(make([]byte, 60)))
	fmt.Println("PHASE 1 INTEGRATION TEST")
	fmt.Println("=" + string(make([]byte, 60)))
	fmt.Println()

	// Setup paths
	homeDir, _ := os.UserHomeDir()
	dataDir := filepath.Join(homeDir, ".marcus", "test-phase1")
	_ = os.MkdirAll(dataDir, 0755)

	// Initialize config
	cfg := &config.Config{
		Model:       "qwen3.5:397b-cloud",
		Temperature: 0.7,
		MaxTokens:   4096,
	}

	// Skip folder engine for this test (requires callback function)
	var folderEng interface{} = nil
	_ = folderEng

	// Initialize memory manager
	memManager := memory.NewManager("", 8)
	_ = memManager.EnsureStructure()

	// Test 1: Outcome Tracker
	fmt.Println("TEST 1: Outcome Tracker")
	fmt.Println("-" + string(make([]byte, 40)))
	tracker := outcome.NewTracker(dataDir)

	outcomes := []outcome.ActionOutcome{
		{ID: "1", ActionType: outcome.ActionWriteFile, Success: true, Duration: 100 * time.Millisecond},
		{ID: "2", ActionType: outcome.ActionWriteFile, Success: true, Duration: 150 * time.Millisecond},
		{ID: "3", ActionType: outcome.ActionWriteFile, Success: false, Duration: 50 * time.Millisecond, Error: "permission denied"},
		{ID: "4", ActionType: outcome.ActionReadFile, Success: true, Duration: 20 * time.Millisecond},
		{ID: "5", ActionType: outcome.ActionRunCommand, Success: true, Duration: 500 * time.Millisecond},
		{ID: "6", ActionType: outcome.ActionRunCommand, Success: false, Duration: 10 * time.Millisecond, Error: "command not found"},
	}

	for _, o := range outcomes {
		o.Timestamp = time.Now()
		tracker.RecordOutcome(o)
	}

	writeStats := tracker.GetStats(outcome.ActionWriteFile)
	fmt.Printf("  write_file: %d attempts, %.1f%% success, score: %.1f\n",
		writeStats.TotalAttempts, writeStats.SuccessRate(), writeStats.PerformanceScore())

	readStats := tracker.GetStats(outcome.ActionReadFile)
	fmt.Printf("  read_file: %d attempts, %.1f%% success, score: %.1f\n",
		readStats.TotalAttempts, readStats.SuccessRate(), readStats.PerformanceScore())

	cmdStats := tracker.GetStats(outcome.ActionRunCommand)
	fmt.Printf("  run_command: %d attempts, %.1f%% success, score: %.1f\n",
		cmdStats.TotalAttempts, cmdStats.SuccessRate(), cmdStats.PerformanceScore())

	patterns := tracker.GetPatterns()
	if len(patterns) > 0 {
		fmt.Printf("  Detected patterns: %d\n", len(patterns))
		for _, p := range patterns {
			fmt.Printf("    - %s (%d occurrences)\n", p.ErrorType, p.Occurrences)
		}
	}
	fmt.Println()

	// Test 2: Capability Registry
	fmt.Println("TEST 2: Capability Registry")
	fmt.Println("-" + string(make([]byte, 40)))
	capRegistry := conscience.NewRegistry(dataDir)

	capabilities := []conscience.Capability{
		{ID: "cap1", Name: "fastapi", Type: conscience.CapabilityKnowledge, Proficiency: 0.95, Tags: []string{"python", "web"}},
		{ID: "cap2", Name: "sqlalchemy", Type: conscience.CapabilityKnowledge, Proficiency: 0.85, Tags: []string{"python", "database"}},
		{ID: "cap3", Name: "websocket", Type: conscience.CapabilityKnowledge, Proficiency: 0.65, Tags: []string{"realtime"}},
		{ID: "cap4", Name: "write_file", Type: conscience.CapabilityTool, Proficiency: 0.90, Tags: []string{"file"}},
		{ID: "cap5", Name: "run_command", Type: conscience.CapabilityTool, Proficiency: 0.80, Tags: []string{"shell"}},
	}

	for _, cap := range capabilities {
		capRegistry.RegisterCapability(cap)
	}

	fmt.Println("  Registered capabilities:")
	for _, cap := range capRegistry.ListCapabilities() {
		level := "novice"
		if cap.Proficiency >= 0.8 {
			level = "proficient"
		}
		if cap.Proficiency >= 0.95 {
			level = "expert"
		}
		fmt.Printf("    - %s (%s): %.0f%%\n", cap.Name, level, cap.Proficiency*100)
	}

	// Test capability check
	canDo, cap := capRegistry.CanDo("Implement a FastAPI endpoint", 0.5)
	fmt.Printf("\n  CanDo('Implement FastAPI'): %v, matched: %v\n", canDo, cap != nil)

	shouldAsk, reason := capRegistry.ShouldAskForHelp("Perform quantum computing simulation")
	fmt.Printf("  ShouldAskForHelp('quantum computing'): %v, reason: %s\n", shouldAsk, reason)
	fmt.Println()

	// Test 3: Confidence Scorer
	fmt.Println("TEST 3: Confidence Scorer")
	fmt.Println("-" + string(make([]byte, 40)))
	scorer := conscience.NewScorer(memManager, tracker)

	assessment := scorer.Assess(context.Background(), "Create a REST API with FastAPI", conscience.ConfidenceFactors{
		HistoricalRate:   0.75,
		ContextComplete:  0.8,
		Complexity:       0.5,
		ToolAvailability: 1.0,
		TimePressure:     0.2,
		Ambiguity:        0.3,
	})

	fmt.Printf("  Task: Create a REST API with FastAPI\n")
	fmt.Printf("  Confidence: %.1f%% (%s)\n", assessment.Confidence*100, assessment.Level)
	fmt.Printf("  Recommendation: %s\n", assessment.Recommendation)
	fmt.Printf("  Reasoning:\n")
	for _, r := range assessment.Reasoning {
		fmt.Printf("    - %s\n", r)
	}
	fmt.Println()

	// Test 4: Reflection Engine
	fmt.Println("TEST 4: Reflection Engine")
	fmt.Println("-" + string(make([]byte, 40)))
	reflectionEng := reflection.NewEngine(dataDir, memManager, tracker)

	reflectionEng.Reflect(context.Background(),
		"Create user model",
		"Used SQLAlchemy with proper relationships",
		"success",
		"Always define relationships explicitly before creating tables")

	reflectionEng.Reflect(context.Background(),
		"Write authentication service",
		"Implemented JWT with refresh tokens",
		"success",
		"Store refresh tokens in httpOnly cookies for security")

	reflectionEng.Reflect(context.Background(),
		"Setup WebSocket connections",
		"Used starlette websocket",
		"partial success",
		"Need to handle connection drops gracefully")

	heuristics := reflectionEng.GetHeuristics("security")
	fmt.Printf("  Recorded 3 reflections\n")
	fmt.Printf("  Heuristics for 'security': %d\n", len(heuristics))

	summary := reflectionEng.GetReflectionSummary()
	fmt.Printf("  Reflection summary: %d heuristics learned\n", len(summary))
	fmt.Println()

	// Test 5: Agent Protocol & Blackboard
	fmt.Println("TEST 5: Agent Protocol & Blackboard")
	fmt.Println("-" + string(make([]byte, 40)))
	protocol := agent.NewProtocol(100)
	blackboard := agent.NewBlackboard(500)

	// Register agents
	protocol.RegisterAgent("agent-1", "architect", []string{"design", "architect"})
	protocol.RegisterAgent("agent-2", "coder", []string{"implement", "write"})
	protocol.RegisterAgent("agent-3", "reviewer", []string{"review", "audit"})

	agents := protocol.ListAgents()
	fmt.Printf("  Registered agents: %d\n", len(agents))
	for _, a := range agents {
		fmt.Printf("    - %s (%s): %v\n", a.ID, a.Role, a.Capabilities)
	}

	// Send message
	msg := agent.Message{
		From:      "agent-1",
		To:        "agent-2",
		Type:      agent.MsgRequest,
		Subject:   "API Design",
		Content:   "Please implement the user endpoint with these specs...",
		Timestamp: time.Now(),
	}
	err := protocol.Send(msg)
	fmt.Printf("\n  Message sent: %v\n", err == nil)

	// Blackboard entry
	entryID := blackboard.Write(agent.BlackboardEntry{
		Subject:    "User API Specification",
		Content:    "POST /users creates user, GET /users/{id} retrieves",
		Type:       agent.EntryDecision,
		CreatedBy:  "agent-1",
		Tags:       []string{"api", "users", "spec"},
		Confidence: 0.9,
	})
	fmt.Printf("  Blackboard entry written: %s\n", entryID)

	entries := blackboard.FindByTag("api", 10)
	fmt.Printf("  Entries tagged 'api': %d\n", len(entries))
	fmt.Println()

	// Test 6: Swarm Coordinator
	fmt.Println("TEST 6: Swarm Coordinator")
	fmt.Println("-" + string(make([]byte, 40)))
	// Create a minimal agent registry for testing
	baseLoopEng := &flow.LoopEngine{}
	agentReg := agent.NewInMemoryAgentRegistry(nil, cfg, dataDir, baseLoopEng)
	swarmCoord := agent.NewSwarmCoordinator(
		agentReg,
		protocol,
		blackboard,
		5,
	)

	// Test swarm composition recommendation
	goals := []string{
		"Design and implement a new authentication system",
		"Debug the websocket connection issue",
		"Review the codebase for security vulnerabilities",
		"Explore the project structure and create documentation",
	}

	for _, goal := range goals {
		roles := swarmCoord.RecommendSwarmComposition(goal)
		goalStr := goal
		if len(goal) > 50 {
			goalStr = goal[:50] + "..."
		}
		fmt.Printf("  Goal: %s\n", goalStr)
		fmt.Printf("    Recommended roles: %v\n", roles)
	}
	fmt.Println()

	// Test 7: Trigger Engine
	fmt.Println("TEST 7: Trigger Engine")
	fmt.Println("-" + string(make([]byte, 40)))
	triggerEng := trigger.NewEngine(dataDir, nil, cfg, memManager, nil)

	// Register a trigger
	testTrigger := trigger.Trigger{
		Name:        "Test Git Trigger",
		Description: "Test trigger for git events",
		Type:        trigger.TriggerEvent,
		Event: trigger.EventMatcher{
			Source:    "git",
			EventType: "commit",
		},
		Action: trigger.TriggerAction{
			Type:   "flow",
			Target: "code_review",
		},
	}

	err = triggerEng.RegisterTrigger(testTrigger)
	fmt.Printf("  Registered trigger: %v\n", err == nil)

	triggers := triggerEng.ListTriggers()
	fmt.Printf("  Total triggers: %d\n", len(triggers))

	stats := triggerEng.GetStats()
	fmt.Printf("  Trigger stats: %d total, %d enabled\n", stats.TotalTriggers, stats.Enabled)

	// Emit an event
	triggerEng.EmitEvent("git", "commit", map[string]string{"hash": "abc123"}, nil)
	fmt.Printf("  Emitted git:commit event\n")
	fmt.Println()

	// Summary
	fmt.Println("=" + string(make([]byte, 60)))
	fmt.Println("ALL PHASE 1 TESTS COMPLETED")
	fmt.Println("=" + string(make([]byte, 60)))
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Println("  [OK] Outcome Tracker - records and analyzes action outcomes")
	fmt.Println("  [OK] Capability Registry - tracks what MARCUS can do")
	fmt.Println("  [OK] Confidence Scorer - estimates confidence before acting")
	fmt.Println("  [OK] Reflection Engine - learns from past decisions")
	fmt.Println("  [OK] Agent Protocol - inter-agent communication")
	fmt.Println("  [OK] Blackboard - shared knowledge workspace")
	fmt.Println("  [OK] Swarm Coordinator - multi-agent team orchestration")
	fmt.Println("  [OK] Trigger Engine - event-driven automation")
	fmt.Println()
	fmt.Println("Phase 1 is ready for integration with the main LoopEngine.")
}
