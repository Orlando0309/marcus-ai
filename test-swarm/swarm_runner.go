package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/flow"
	"github.com/marcus-ai/marcus/internal/folder"
	"github.com/marcus-ai/marcus/internal/loop"
	"github.com/marcus-ai/marcus/internal/memory"
	"github.com/marcus-ai/marcus/internal/provider"
	"github.com/marcus-ai/marcus/internal/session"
	"github.com/marcus-ai/marcus/internal/task"
	"github.com/marcus-ai/marcus/internal/tool"
)

func main() {
	fmt.Println("============================================================")
	fmt.Println("REAL SWARM TEST: Marcus Building Complex Python API")
	fmt.Println("============================================================")
	fmt.Println()

	// Create test directory
	testDir := filepath.Join(".", "test-swarm", "swarm-output")
	if err := os.RemoveAll(testDir); err != nil {
		fmt.Printf("Failed to clean test dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(testDir, 0755); err != nil {
		fmt.Printf("Failed to create test dir: %v\n", err)
		os.Exit(1)
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Working directory: %s\n", testDir)
	fmt.Println()

	// Create memory manager
	mem := memory.NewManager(testDir, 8)

	// Create task store
	taskStore := task.NewStore(filepath.Join(testDir, ".marcus", "tasks"))

	// Create folder engine
	homeDir, _ := os.UserHomeDir()
	folderEngine := folder.NewFolderEngine(
		filepath.Join(homeDir, ".marcus"),
		filepath.Join(testDir, ".marcus"),
		nil,
	)
	if err := folderEngine.Boot(); err != nil {
		fmt.Printf("Folder engine boot failed: %v\n", err)
		os.Exit(1)
	}

	// Create base flow engine
	baseEngine, err := flow.NewEngine(cfg, nil)
	if err != nil {
		fmt.Printf("Failed to create flow engine: %v\n", err)
		os.Exit(1)
	}

	// Create enhanced loop engine with ALL Phase 1 components
	fmt.Println("Initializing Enhanced LoopEngine with Phase 1 components...")
	enhancedEngine := loop.NewEnhancedLoopEngine(
		baseEngine.LoopEngine(nil, taskStore, mem, nil, nil),
		folderEngine,
		cfg,
		mem,
		filepath.Join(testDir, ".marcus"),
	)

	// Check swarm capability
	swarmCoord := enhancedEngine.GetSwarmCoordinator()
	if swarmCoord == nil {
		fmt.Println("ERROR: Swarm coordinator not available")
		os.Exit(1)
	}
	fmt.Println("  - Swarm Coordinator: OK")

	// Check capability registry
	capRegistry := enhancedEngine.GetCapabilityRegistry()
	if capRegistry == nil {
		fmt.Println("ERROR: Capability registry not available")
		os.Exit(1)
	}
	fmt.Println("  - Capability Registry: OK")

	// Check outcome tracker
	tracker := enhancedEngine.GetOutcomeTracker()
	if tracker == nil {
		fmt.Println("ERROR: Outcome tracker not available")
		os.Exit(1)
	}
	fmt.Println("  - Outcome Tracker: OK")

	// Check reflection engine
	reflectionEngine := enhancedEngine.GetReflectionEngine()
	if reflectionEngine == nil {
		fmt.Println("ERROR: Reflection engine not available")
		os.Exit(1)
	}
	fmt.Println("  - Reflection Engine: OK")

	fmt.Println()
	fmt.Println("All Phase 1 components initialized successfully!")
	fmt.Println()

	// Simple goal for architect agent to design structure
	goal := `Design a Python project structure for a REST API.
Create only a README.md that describes:
- Project overview
- File structure
- Dependencies needed

Keep it brief - just the README, no code files.`

	fmt.Println("============================================================")
	fmt.Println("SWARM EXECUTION STARTING")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Printf("Goal: Create complex Python REST API\n")
	fmt.Println()

	// Check if swarm is recommended
	isComplex := enhancedEngine.IsComplexGoal(goal)
	fmt.Printf("Is complex goal (should trigger swarm): %v\n", isComplex)
	fmt.Println()

	if isComplex {
		fmt.Println("Swarm execution recommended!")
		fmt.Println("Recommended agent composition:")
		roles := swarmCoord.RecommendSwarmComposition(goal)
		for i, role := range roles {
			fmt.Printf("  %d. %s\n", i+1, role)
		}
		fmt.Println()
	}

	// Build tool runner
	toolRunner, err := tool.BuildRunner(tool.BuildOptions{
		BaseDir: testDir,
		Config:  cfg,
		Folders: folderEngine,
	})
	if err != nil {
		fmt.Printf("Failed to build tool runner: %v\n", err)
		os.Exit(1)
	}

	// Create provider runtime
	prov, err := provider.Stack(cfg.Provider, cfg.Model, cfg.ProviderFallbacks)
	if err != nil {
		fmt.Printf("Failed to get provider: %v\n", err)
		os.Exit(1)
	}
	providerRuntime := provider.NewRuntime(prov, testDir, true)

	// Create context assembler
	contextAsm := &simpleContextAssembler{}

	// Create base loop engine (for tools)
	baseLoopEngine := baseEngine.LoopEngine(toolRunner, taskStore, mem, contextAsm, providerRuntime)

	// Replace base engine in enhanced engine with the properly wired one
	enhancedEngine.SetBaseEngine(baseLoopEngine)

	// Also update the swarm coordinator's registry with the working engine
	enhancedEngine.GetSwarmCoordinator().UpdateRegistryEngine(baseLoopEngine)

	// Run the goal using ENHANCED engine (which triggers swarm)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Println("Starting autonomous swarm execution...")
	fmt.Println("(This will take time as Marcus builds each file)")
	fmt.Println()

	startTime := time.Now()
	state, err := enhancedEngine.Run(ctx, goal, "swarm-test-001", 50)
	elapsed := time.Since(startTime)

	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("EXECUTION COMPLETE")
	fmt.Println("============================================================")
	fmt.Println()

	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	}

	if state != nil {
		fmt.Printf("Done: %v\n", state.Done)
		fmt.Printf("Finish Reason: %s\n", state.FinishReason)
		fmt.Printf("Iteration: %d\n", state.Iteration)
		fmt.Printf("Elapsed: %v\n", elapsed)
		fmt.Println()

		// Show outcome stats
		fmt.Println("Outcome Statistics:")
		fmt.Println(tracker.GetSummary())
		fmt.Println()

		// Show what was created
		fmt.Println("Created files:")
		filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				rel, _ := filepath.Rel(testDir, path)
				fmt.Printf("  %s\n", rel)
			}
			return nil
		})
	}

	fmt.Println()
	fmt.Println("Test complete!")
}

// simpleContextAssembler is a minimal context assembler for testing
type simpleContextAssembler struct{}

func (s *simpleContextAssembler) Assemble(input string, sess *session.Session) flow.Snapshot {
	return flow.Snapshot{
		Text: input,
	}
}
