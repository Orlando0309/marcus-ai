package folder

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/pelletier/go-toml/v2"
)

// FolderEngine discovers and loads Marcus folders
type FolderEngine struct {
	globalPath  string
	projectPath string
	registry    *Registry
	watcher     *fsnotify.Watcher
	mu          sync.RWMutex
	notify      func(string)
	debounceMu  sync.Mutex
	debounceMap map[string]*time.Timer
}

// NewFolderEngine creates a new folder engine
func NewFolderEngine(globalPath, projectPath string, notify func(string)) *FolderEngine {
	return &FolderEngine{
		globalPath:  globalPath,
		projectPath: projectPath,
		registry:    NewRegistry(),
		notify:      notify,
		debounceMap: make(map[string]*time.Timer),
	}
}

// Boot discovers all folders and registers them
func (fe *FolderEngine) Boot() error {
	fe.mu.Lock()
	fe.registry = NewRegistry()
	fe.mu.Unlock()

	// Walk all scopes in order, later scopes override earlier
	scopes := []string{fe.globalPath, fe.projectPath}
	for _, scope := range scopes {
		if scope == "" || !pathExists(scope) {
			continue
		}
		if err := fe.walkScope(scope); err != nil {
			return fmt.Errorf("walk scope %s: %w", scope, err)
		}
	}

	// Start hot-reload watcher
	return fe.startWatcher()
}

// walkScope walks a directory scope and registers all units
func (fe *FolderEngine) walkScope(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) < 2 {
			return nil
		}

		// Depth 1 = category (flows, tools, agents, memory...)
		// Depth 2 = unit name
		category := parts[0]
		name := parts[1]
		unitPath := filepath.Join(root, category, name)
		if path != unitPath {
			return nil
		}

		switch category {
		case "flows":
			return fe.registerFlow(unitPath, name)
		case "tools":
			return fe.registerTool(unitPath, name)
		case "agents":
			return fe.registerAgent(unitPath, name)
		case "memory":
			return fe.registerMemory(unitPath, name)
		}

		return nil
	})
}

// registerFlow loads and validates a flow folder
func (fe *FolderEngine) registerFlow(path, name string) error {
	flowPath := filepath.Join(path, "flow.toml")
	data, err := os.ReadFile(flowPath)
	if err != nil {
		return fmt.Errorf("read flow.toml: %w", err)
	}

	var flow FlowDef
	if err := toml.Unmarshal(data, &flow); err != nil {
		return fmt.Errorf("parse flow.toml: %w", err)
	}

	flow.Name = name
	flow.Path = path
	flow.ContextPath = filepath.Join(path, "context.md")
	if flow.Flow.Name != "" {
		flow.Name = flow.Flow.Name
	}
	if flow.Description == "" {
		flow.Description = flow.Flow.Description
	}
	if flow.Version == "" {
		flow.Version = flow.Flow.Version
	}
	if flow.Author == "" {
		flow.Author = flow.Flow.Author
	}

	// Validate flow
	if err := validateFlow(path); err != nil {
		return fmt.Errorf("validate flow: %w", err)
	}

	fe.mu.Lock()
	fe.registry.Flows[name] = &flow
	fe.mu.Unlock()

	if fe.notify != nil {
		fe.notify(fmt.Sprintf("Loaded flow: %s", name))
	}

	return nil
}

// registerTool loads and validates a tool folder
func (fe *FolderEngine) registerTool(path, name string) error {
	toolPath := filepath.Join(path, "tool.toml")
	data, err := os.ReadFile(toolPath)
	if err != nil {
		// tool.toml is optional for shell-based tools
		if os.IsNotExist(err) {
			for _, scriptName := range []string{"run.sh", "run.ps1", "run.cmd", "run.bat"} {
				runScript := filepath.Join(path, scriptName)
				if _, err := os.Stat(runScript); err == nil {
					tool := &ToolDef{
						Name: name,
						Path: path,
						Type: "shell",
					}
					fe.mu.Lock()
					fe.registry.Tools[name] = tool
					fe.mu.Unlock()
					return nil
				}
			}
			return fmt.Errorf("tool.toml or runnable script required")
		}
		return fmt.Errorf("read tool.toml: %w", err)
	}

	var tool ToolDef
	if err := toml.Unmarshal(data, &tool); err != nil {
		return fmt.Errorf("parse tool.toml: %w", err)
	}

	tool.Name = name
	tool.Path = path

	fe.mu.Lock()
	fe.registry.Tools[name] = &tool
	fe.mu.Unlock()

	if fe.notify != nil {
		fe.notify(fmt.Sprintf("Loaded tool: %s", name))
	}

	return nil
}

// registerAgent loads and validates an agent folder
func (fe *FolderEngine) registerAgent(path, name string) error {
	agentPath := filepath.Join(path, "agent.toml")
	data, err := os.ReadFile(agentPath)
	if err != nil {
		return fmt.Errorf("read agent.toml: %w", err)
	}

	var agent AgentDef
	if err := toml.Unmarshal(data, &agent); err != nil {
		return fmt.Errorf("parse agent.toml: %w", err)
	}

	agent.Name = name
	agent.Path = path

	// Validate agent
	if err := validateAgent(path); err != nil {
		return fmt.Errorf("validate agent: %w", err)
	}

	// Load custom system prompt from system.md
	if sysPrompt, err := agent.ReadSystemPrompt(); err == nil && sysPrompt != "" {
		agent.Autonomy.SystemPrompt = sysPrompt
	}

	// Apply defaults for unset fields
	if agent.Autonomy.IterationLimit == 0 {
		agent.Autonomy.IterationLimit = 10
	}
	if len(agent.Rules.SafeActions) == 0 {
		agent.Rules.SafeActions = []string{"list_files", "read_file", "search_code", "find_symbol", "list_symbols"}
	}
	if len(agent.Rules.AutoRunCommands) == 0 {
		agent.Rules.AutoRunCommands = []string{"go build", "cargo build", "npm run build", "ruff check", "ruff format", "go test", "go vet", "golangci-lint run"}
	}
	if agent.Rules.WriteIf == "" {
		agent.Rules.WriteIf = "first_wave"
	}

	fe.mu.Lock()
	fe.registry.Agents[name] = &agent
	fe.mu.Unlock()

	if fe.notify != nil {
		fe.notify(fmt.Sprintf("Loaded agent: %s (role=%s)", name, agent.Role))
	}

	return nil
}

// registerMemory loads a memory folder
func (fe *FolderEngine) registerMemory(path, name string) error {
	fe.mu.Lock()
	fe.registry.Memories[name] = &MemoryDef{
		Name: name,
		Path: path,
	}
	fe.mu.Unlock()
	return nil
}

// startWatcher starts the hot-reload file watcher
func (fe *FolderEngine) startWatcher() error {
	if fe.watcher != nil {
		_ = fe.watcher.Close()
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	fe.watcher = watcher

	scopes := []string{fe.globalPath, fe.projectPath}
	for _, scope := range scopes {
		if scope == "" || !pathExists(scope) {
			continue
		}
		if err := filepath.WalkDir(scope, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				return nil
			}
			return watcher.Add(path)
		}); err != nil {
			return err
		}
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				fe.handleFSEvent(event)
			case <-watcher.Errors:
				return
			}
		}
	}()

	return nil
}

// handleFSEvent handles a filesystem event
func (fe *FolderEngine) handleFSEvent(event fsnotify.Event) {
	path := event.Name
	if info, err := os.Stat(path); err == nil && info.IsDir() && event.Has(fsnotify.Create) {
		_ = fe.watcher.Add(path)
	}

	unitPath, category, name := fe.resolveUnit(path)
	if unitPath == "" || name == "" {
		return
	}

	fe.debounceReload(unitPath, func() {
		switch category {
		case "flows":
			if err := fe.registerFlow(unitPath, name); err != nil {
				fe.safeNotify(fmt.Sprintf("Reload failed: %s: %v", name, err))
				return
			}
			fe.safeNotify(fmt.Sprintf("Reloaded flow: %s", name))
		case "tools":
			if err := fe.registerTool(unitPath, name); err != nil {
				fe.safeNotify(fmt.Sprintf("Reload failed: %s: %v", name, err))
				return
			}
			fe.safeNotify(fmt.Sprintf("Reloaded tool: %s", name))
		case "agents":
			if err := fe.registerAgent(unitPath, name); err != nil {
				fe.safeNotify(fmt.Sprintf("Reload failed: %s: %v", name, err))
				return
			}
			fe.safeNotify(fmt.Sprintf("Reloaded agent: %s", name))
		case "memory":
			_ = fe.registerMemory(unitPath, name)
			fe.safeNotify(fmt.Sprintf("Reloaded memory: %s", name))
		}
	})
}

func (fe *FolderEngine) resolveUnit(path string) (string, string, string) {
	dir := path
	for {
		for _, scope := range []string{fe.projectPath, fe.globalPath} {
			if scope == "" || !strings.HasPrefix(dir, scope) {
				continue
			}
			rel, err := filepath.Rel(scope, dir)
			if err != nil {
				continue
			}
			parts := strings.Split(filepath.ToSlash(rel), "/")
			if len(parts) >= 2 {
				category, name := parts[0], parts[1]
				switch category {
				case "flows", "tools", "agents", "memory":
					return filepath.Join(scope, category, name), category, name
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", ""
		}
		dir = parent
	}
}

// GetFlow returns a flow by name
func (fe *FolderEngine) GetFlow(name string) (*FlowDef, bool) {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	flow, ok := fe.registry.Flows[name]
	return flow, ok
}

// GetTool returns a tool by name
func (fe *FolderEngine) GetTool(name string) (*ToolDef, bool) {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	tool, ok := fe.registry.Tools[name]
	return tool, ok
}

// ToolDefs returns all discovered tool definitions.
func (fe *FolderEngine) ToolDefs() []*ToolDef {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	defs := make([]*ToolDef, 0, len(fe.registry.Tools))
	for _, def := range fe.registry.Tools {
		defs = append(defs, def)
	}
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Name < defs[j].Name
	})
	return defs
}

// ListFlows returns all flow names
func (fe *FolderEngine) ListFlows() []string {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	flows := make([]string, 0, len(fe.registry.Flows))
	for name := range fe.registry.Flows {
		flows = append(flows, name)
	}
	sort.Strings(flows)
	return flows
}

// ListTools returns all tool names
func (fe *FolderEngine) ListTools() []string {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	tools := make([]string, 0, len(fe.registry.Tools))
	for name := range fe.registry.Tools {
		tools = append(tools, name)
	}
	sort.Strings(tools)
	return tools
}

// ListAgents returns all agent names
func (fe *FolderEngine) ListAgents() []string {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	agents := make([]string, 0, len(fe.registry.Agents))
	for name := range fe.registry.Agents {
		agents = append(agents, name)
	}
	sort.Strings(agents)
	return agents
}

// GetAgent returns an agent by name
func (fe *FolderEngine) GetAgent(name string) (*AgentDef, bool) {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	agent, ok := fe.registry.Agents[name]
	return agent, ok
}

func (fe *FolderEngine) debounceReload(key string, fn func()) {
	fe.debounceMu.Lock()
	defer fe.debounceMu.Unlock()
	if timer, ok := fe.debounceMap[key]; ok {
		timer.Stop()
	}
	fe.debounceMap[key] = time.AfterFunc(250*time.Millisecond, func() {
		fn()
		fe.debounceMu.Lock()
		delete(fe.debounceMap, key)
		fe.debounceMu.Unlock()
	})
}

func (fe *FolderEngine) safeNotify(message string) {
	if fe.notify != nil {
		fe.notify(message)
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
