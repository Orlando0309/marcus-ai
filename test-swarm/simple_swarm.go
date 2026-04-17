//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/marcus-ai/marcus/internal/agent"
	"github.com/marcus-ai/marcus/internal/config"
	"github.com/marcus-ai/marcus/internal/flow"
	"github.com/marcus-ai/marcus/internal/folder"
	"github.com/marcus-ai/marcus/internal/memory"
	"github.com/marcus-ai/marcus/internal/provider"
	"github.com/marcus-ai/marcus/internal/session"
	"github.com/marcus-ai/marcus/internal/task"
	"github.com/marcus-ai/marcus/internal/tool"
)

func main() {
	fmt.Println("============================================================")
	fmt.Println("SIMPLE SWARM TEST: Sequential Agent Execution")
	fmt.Println("============================================================")
	fmt.Println()

	testDir := filepath.Join(".", "test-swarm", "simple-out")
	os.RemoveAll(testDir)
	os.MkdirAll(testDir, 0755)

	cfg, _ := config.Load()
	os.Chdir(testDir)
	fmt.Printf("Working directory: %s\n", testDir)
	fmt.Println()

	// Create dependencies
	mem := memory.NewManager(testDir, 8)
	taskStore := task.NewStore(filepath.Join(testDir, ".marcus", "tasks"))

	homeDir, _ := os.UserHomeDir()
	folderEngine := folder.NewFolderEngine(
		filepath.Join(homeDir, ".marcus"),
		filepath.Join(testDir, ".marcus"),
		nil,
	)
	folderEngine.Boot()

	baseEngine, _ := flow.NewEngine(cfg, nil)

	toolRunner, _ := tool.BuildRunner(tool.BuildOptions{
		BaseDir: testDir,
		Config:  cfg,
		Folders: folderEngine,
	})

	prov, _ := provider.Stack(cfg.Provider, cfg.Model, cfg.ProviderFallbacks)
	providerRuntime := provider.NewRuntime(prov, testDir, true)

	contextAsm := &simpleContextAssembler{}

	// Create base loop engine
	loopEngine := baseEngine.LoopEngine(toolRunner, taskStore, mem, contextAsm, providerRuntime)

	// Create agent registry WITH the loop engine
	agentReg := agent.NewInMemoryAgentRegistry(folderEngine, cfg, testDir, loopEngine)

	fmt.Println("Testing sequential agent execution...")
	fmt.Println()

	// Test 1: Architect agent
	fmt.Println("TEST 1: Architect Agent - Design project structure")
	fmt.Println("-" + string(make([]byte, 50)))
	architectGoal := "Create a README.md describing a simple Python REST API project structure."

	architect, err := agentReg.Spawn(agent.RoleArchitect, architectGoal, "", agent.SpawnOptions{
		Timeout:       3 * time.Minute,
		MaxIterations: 10,
	})
	if err != nil {
		fmt.Printf("Failed to spawn architect: %v\n", err)
	} else {
		fmt.Printf("Spawned architect agent: %s\n", architect.ID)
		fmt.Printf("Goal: %s\n", architect.Goal)

		// Wait for completion
		fmt.Println("Waiting for agent to complete (up to 60 seconds)...")
		time.Sleep(60 * time.Second)

		// Check status
		updatedArch, ok := agentReg.Get(architect.ID)
		if ok {
			fmt.Printf("Status: %s\n", updatedArch.Status)
			if updatedArch.Result != nil {
				fmt.Printf("Summary: %s\n", updatedArch.Result.Summary)
				if updatedArch.Result.Success {
					fmt.Println("[OK] Architect completed!")
				} else {
					fmt.Printf("[FAIL] Architect failed: %s\n", updatedArch.Result.Error)
				}
			}
		}
	}
	fmt.Println()

	// Check what was created
	fmt.Println("Files created:")
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
	fmt.Println()

	// Show README content if created
	readmePath := filepath.Join(testDir, "README.md")
	if content, err := os.ReadFile(readmePath); err == nil {
		fmt.Println("README.md content:")
		fmt.Println("---")
		fmt.Println(string(content))
		fmt.Println("---")
	} else {
		fmt.Println("README.md was not created (agent may still be running or timed out)")
	}

	fmt.Println()
	fmt.Println("Test complete!")
}

type simpleContextAssembler struct{}

func (s *simpleContextAssembler) Assemble(input string, sess *session.Session) flow.Snapshot {
	return flow.Snapshot{Text: input}
}
