# Phase 1 Implementation: Smarter, Swarm, Conscience & Autonomous

## Summary

This implementation adds foundational capabilities to MARCUS to make it:
- **Smarter** - Learns from outcomes, reflects on decisions
- **More Swarm-like** - Multi-agent collaboration with coordinated teams
- **More Self-Aware (Conscience)** - Knows its capabilities and confidence levels
- **More Autonomous** - Event-driven triggers for automated execution

## Components Implemented

### 1. Outcome Tracker (`internal/outcome/tracker.go`)

Tracks success/failure of every action MARCUS takes.

**Features:**
- Records outcomes for each tool/action type
- Calculates success rates, performance scores
- Identifies recurring failure patterns
- Persists statistics across sessions

**Key Types:**
- `Tracker` - Main tracking engine
- `ActionOutcome` - Single action result
- `ToolStats` - Per-tool statistics
- `Pattern` - Recurring failure patterns

**Usage:**
```go
tracker := outcome.NewTracker(dataDir)
tracker.RecordOutcome(outcome.ActionOutcome{
    ActionType: outcome.ActionWriteFile,
    Context:    "Implement feature X",
    Success:    true,
    Duration:   500 * time.Millisecond,
})
stats := tracker.GetStats(outcome.ActionWriteFile)
fmt.Printf("Success rate: %.1f%%", stats.SuccessRate())
```

### 2. Reflection Engine (`internal/reflection/engine.go`)

Learns from past decisions and outcomes to improve future performance.

**Features:**
- Records reflections on completed actions
- Extracts heuristics from experience
- Deep reflection using LLM analysis
- Persists learned heuristics

**Key Types:**
- `Engine` - Reflection engine
- `Reflection` - Single reflection entry
- `Heuristic` - Learned rule/pattern
- `Insight` - Deep analytical insight

**Usage:**
```go
reflectionEng := reflection.NewEngine(dataDir, mem, tracker)
reflectionEng.Reflect(ctx, goal, decision, outcome, learned)
heuristics := reflectionEng.GetHeuristics(currentContext)
```

### 3. Agent Protocol & Blackboard (`internal/agent/protocol.go`, `internal/agent/blackboard.go`)

Enables inter-agent communication and shared knowledge.

**Protocol Features:**
- Structured message passing between agents
- Priority-based message queues
- Broadcast and point-to-point messaging
- Agent registration with capabilities

**Blackboard Features:**
- Shared workspace for agent collaboration
- Entry types: fact, hypothesis, finding, decision, artifact
- Query by tags, types, subjects
- Automatic indexing and relevance scoring

**Key Types:**
- `Protocol` - Message passing system
- `Blackboard` - Shared knowledge space
- `Message` - Inter-agent message
- `BlackboardEntry` - Knowledge entry

### 4. Swarm Coordinator (`internal/agent/swarm.go`)

Orchestrates multi-agent teams for complex goals.

**Features:**
- Spawns agent teams based on goal analysis
- Recommends agent composition from goal keywords
- Phased execution plan (Planning → Implementation → Review → Debugging)
- Swarm lifecycle management (form, active, pause, complete, terminate)

**Key Types:**
- `SwarmCoordinator` - Team orchestrator
- `Swarm` - Active agent team
- `SwarmPlan` - Execution plan with phases
- `SwarmPhase` - Single phase in plan

**Usage:**
```go
swarmCoord := agent.NewSwarmCoordinator(registry, protocol, blackboard, maxSwarms)
roles := swarmCoord.RecommendSwarmComposition("Implement a new API endpoint")
swarm, _ := swarmCoord.SpawnSwarm(ctx, goal, roles)
```

### 5. Capability Registry (`internal/conscience/capability.go`)

Self-awareness: MARCUS knows what it can and cannot do.

**Features:**
- Registers capabilities with proficiency levels
- Tracks capability usage and adjusts proficiency
- Self-assessment of strengths/weaknesses
- Determines when to ask for human help

**Key Types:**
- `Registry` - Capability registry
- `Capability` - Single capability with proficiency
- `SelfAssessment` - MARCUS's self-knowledge
- `ConfidenceLevel` - High/Medium/Low/VeryLow/Unknown

**Usage:**
```go
capRegistry := conscience.NewRegistry(dataDir)
capRegistry.RegisterCapability(conscience.Capability{
    Name: "write_file",
    Type: conscience.CapabilityTool,
    Proficiency: 0.85,
})
canDo, cap := capRegistry.CanDo("Write a new file", 0.5)
shouldAsk, reason := capRegistry.ShouldAskForHelp("Perform surgery")
```

### 6. Confidence Scorer (`internal/conscience/confidence.go`)

Estimates confidence before taking actions.

**Features:**
- Multi-factor confidence calculation
- Historical performance calibration
- Brier score for calibration tracking
- Action recommendations based on confidence

**Key Types:**
- `Scorer` - Confidence calculator
- `ConfidenceFactors` - Input factors
- `ConfidenceAssessment` - Result with recommendation
- `CalibrationStats` - Calibration statistics

**Recommendations:**
- `Proceed` - High confidence, execute
- `VerifyFirst` - Medium confidence, verify before executing
- `AskUser` - Low confidence, ask for help
- `ResearchFirst` - Low confidence, research more
- `Decline` - Very low confidence, decline task

**Usage:**
```go
scorer := conscience.NewScorer(mem, tracker)
assessment := scorer.Assess(ctx, task, conscience.ConfidenceFactors{
    HistoricalRate: 0.8,
    ContextComplete: 0.9,
    Complexity: 0.3,
})
if assessment.Recommendation == conscience.RecommendAsk {
    // Ask user for clarification
}
```

### 7. Trigger Engine (`internal/trigger/engine.go`)

Event-driven automation: "When X happens, do Y".

**Features:**
- Event-based triggers (git commits, file changes)
- Schedule-based triggers (cron expressions)
- Condition-based triggers (file exists, task status)
- Webhook triggers (HTTP endpoints)
- Debounce and throttle options

**Key Types:**
- `Engine` - Trigger management
- `Trigger` - Event-to-action mapping
- `Event` - Triggerable event
- `TriggerAction` - Action to execute

**Usage:**
```go
triggerEng := trigger.NewEngine(dataDir, folder, cfg, mem, flowExec)
triggerEng.RegisterTrigger(trigger.Trigger{
    Name: "Review on PR",
    Type: trigger.TriggerEvent,
    Event: trigger.EventMatcher{
        Source: "git",
        EventType: "push",
    },
    Action: trigger.TriggerAction{
        Type: "flow",
        Target: "code_review",
    },
})
triggerEng.EmitEvent("git", "push", payload, nil)
```

### 8. Enhanced Loop Engine (`internal/loop/integration.go`)

Integrates all Phase 1 components into the main execution loop.

**Features:**
- Wraps base `flow.LoopEngine` with enhanced capabilities
- Confidence assessment before execution
- Automatic swarm spawning for complex goals
- Outcome recording after execution
- Reflection on completed executions
- Periodic deep reflection

**Usage:**
```go
enhancedEngine := loop.NewEnhancedLoopEngine(
    baseEngine,
    folder,
    cfg,
    mem,
    dataDir,
)
state, err := enhancedEngine.Run(ctx, goal, taskID, maxIterations)
```

## File Structure

```
internal/
├── outcome/
│   └── tracker.go           # Outcome tracking
├── reflection/
│   └── engine.go            # Reflection engine
├── agent/
│   ├── protocol.go          # Message passing
│   ├── blackboard.go        # Shared knowledge
│   └── swarm.go             # Swarm coordination
├── conscience/
│   ├── capability.go        # Capability registry
│   └── confidence.go        # Confidence scorer
├── trigger/
│   └── engine.go            # Trigger engine
└── loop/
    └── integration.go       # Enhanced loop engine
```

## Configuration

Add to your `.marcus/marcus.toml`:

```toml
[phase1]
enable_outcome_tracking = true
enable_reflection = true
enable_swarm = true
enable_confidence_scoring = true
enable_triggers = true
max_swarms = 5
max_outcomes = 10000
reflection_interval_minutes = 30
```

## Next Steps (Phase 2)

1. **Semantic Context Assembler** - Embedding-based context ranking
2. **Debate System** - Multi-agent adversarial reasoning
3. **Progress Monitor** - Stuck state detection
4. **Daemon Mode** - Background execution
5. **External Integrations** - GitHub, Slack webhooks

## Testing

```bash
# Build
go build -o marcus.exe ./cmd/marcus

# Run tests
go test ./internal/outcome/...
go test ./internal/reflection/...
go test ./internal/agent/...
go test ./internal/conscience/...
go test ./internal/trigger/...
```

## API Examples

### Check capability before task
```go
canDo, cap := enhancedEngine.GetCapabilityRegistry().CanDo(goal, 0.5)
if !canDo {
    fmt.Printf("Task requires capability not in registry: %v\n", cap)
}
```

### Get confidence assessment
```go
assessment := enhancedEngine.GetConfidenceScorer().Assess(ctx, goal, factors)
fmt.Printf("Confidence: %.1f%% (%s)\n", assessment.Confidence*100, assessment.Level)
fmt.Printf("Recommendation: %s\n", assessment.Recommendation)
```

### Spawn swarm for complex task
```go
roles := enhancedEngine.GetSwarmCoordinator().RecommendSwarmComposition(goal)
swarm, _ := enhancedEngine.GetSwarmCoordinator().SpawnSwarm(ctx, goal, roles)
```

### Register trigger
```go
enhancedEngine.GetTriggerEngine().RegisterTrigger(trigger.Trigger{
    Name: "Auto-review on commit",
    Event: trigger.EventMatcher{
        Source: trigger.EventSourceGit,
        EventType: trigger.EventTypeCommit,
    },
    Action: trigger.TriggerAction{
        Type: "flow",
        Target: "code_review",
    },
})
```

### Get summary
```go
summary := enhancedEngine.GetSummary()
fmt.Println(summary)
```
