package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/marcus-ai/marcus/internal/task"
)

type TodoWriteTool struct {
	baseDir string
}

type todoWriteItem struct {
	ID          string   `json:"id"`
	Content     string   `json:"content"`
	Status      string   `json:"status"`
	Description string   `json:"description,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

func NewTodoWriteTool(baseDir string) *TodoWriteTool {
	return &TodoWriteTool{baseDir: baseDir}
}

func (t *TodoWriteTool) Name() string { return "todo_write" }

func (t *TodoWriteTool) Description() string {
	return "Create or update durable task checklist items"
}

func (t *TodoWriteTool) Schema() JSONSchema {
	return JSONSchema{
		Type: "object",
		Properties: map[string]*JSONProperty{
			"todos": {Type: "array", Description: "Todo items with id, content, and status"},
		},
		Required: []string{"todos"},
	}
}

func (t *TodoWriteTool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	_ = ctx
	var params struct {
		Todos []todoWriteItem `json:"todos"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, fmt.Errorf("unmarshal input: %w", err)
	}
	store := task.NewStore(t.baseDir)
	updates := make([]task.Update, 0, len(params.Todos))
	dag := &task.DAG{}
	seenNodes := map[string]struct{}{}
	for _, item := range params.Todos {
		title := strings.TrimSpace(item.Content)
		if title == "" {
			title = strings.TrimSpace(item.Description)
		}
		if title == "" {
			continue
		}
		updates = append(updates, task.Update{
			ID:          strings.TrimSpace(item.ID),
			Title:       title,
			Description: strings.TrimSpace(item.Description),
			Status:      strings.TrimSpace(item.Status),
		})
		nodeID := strings.TrimSpace(item.ID)
		if nodeID == "" {
			nodeID = task.SlugifyForTool(title)
		}
		if _, ok := seenNodes[nodeID]; !ok {
			dag.Nodes = append(dag.Nodes, nodeID)
			seenNodes[nodeID] = struct{}{}
		}
		for _, dep := range item.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			dag.Edges = append(dag.Edges, task.Edge{From: dep, To: nodeID})
		}
	}
	applied, err := store.ApplyUpdates(updates)
	if err != nil {
		return nil, err
	}
	if len(dag.Nodes) > 0 || len(dag.Edges) > 0 {
		_ = store.SaveDAG(dag)
	}
	return json.Marshal(map[string]any{
		"count": appliedCount(applied),
		"tasks": applied,
	})
}

func appliedCount(tasks []task.Task) int {
	return len(tasks)
}
