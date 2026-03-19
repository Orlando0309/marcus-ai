package task

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	StatusQueue   = "queue"
	StatusActive  = "active"
	StatusDone    = "done"
	StatusBlocked = "blocked"
)

// Task is the durable task record Marcus keeps on disk.
type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	SourceKind  string    `json:"source_kind,omitempty"`
	SourcePath  string    `json:"source_path,omitempty"`
	SourceText  string    `json:"source_text,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TODOHint is a task candidate discovered from source code comments.
type TODOHint struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Update is the model-facing task update format.
type Update struct {
	ID          string `json:"id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
}

// Store manages file-backed task state in `.marcus/tasks/`.
type Store struct {
	root string
}

// NewStore creates a task store rooted at the project path.
func NewStore(projectRoot string) *Store {
	if projectRoot == "" {
		return &Store{}
	}
	return &Store{root: filepath.Join(projectRoot, ".marcus", "tasks")}
}

// EnsureStructure creates the task directories if needed.
func (s *Store) EnsureStructure() error {
	if s.root == "" {
		return nil
	}
	for _, dir := range []string{
		s.root,
		filepath.Join(s.root, StatusQueue),
		filepath.Join(s.root, StatusActive),
		filepath.Join(s.root, StatusDone),
		filepath.Join(s.root, StatusBlocked),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// ApplyUpdates stores or updates tasks returned by the model.
func (s *Store) ApplyUpdates(updates []Update) ([]Task, error) {
	if err := s.EnsureStructure(); err != nil {
		return nil, err
	}
	if s.root == "" || len(updates) == 0 {
		return nil, nil
	}

	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	applied := make([]Task, 0, len(updates))
	for _, update := range updates {
		task := Task{
			ID:          update.ID,
			Title:       strings.TrimSpace(update.Title),
			Description: strings.TrimSpace(update.Description),
			Status:      normalizeStatus(update.Status),
			UpdatedAt:   time.Now().UTC(),
		}
		if task.Title == "" {
			continue
		}
		if task.ID == "" {
			task.ID = slugify(task.Title)
		}
		if existing, ok := index[task.ID]; ok {
			task = existing
			task.Title = strings.TrimSpace(update.Title)
			task.Description = strings.TrimSpace(update.Description)
			if update.Status != "" {
				task.Status = normalizeStatus(update.Status)
			}
			task.UpdatedAt = time.Now().UTC()
		}
		if err := s.write(task); err != nil {
			return nil, err
		}
		applied = append(applied, task)
	}

	return applied, nil
}

// SyncTODOHints creates or reopens durable tasks from TODO comments.
func (s *Store) SyncTODOHints(hints []TODOHint) ([]Task, error) {
	if err := s.EnsureStructure(); err != nil {
		return nil, err
	}
	if s.root == "" || len(hints) == 0 {
		return nil, nil
	}

	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	changed := make([]Task, 0, len(hints))
	for _, hint := range hints {
		id := todoTaskID(hint.Path, hint.Text)
		existing, ok := index[id]
		task := Task{
			ID:          id,
			Title:       hint.Text,
			Description: fmt.Sprintf("TODO in %s:%d", hint.Path, hint.Line),
			Status:      StatusQueue,
			SourceKind:  "todo",
			SourcePath:  hint.Path,
			SourceText:  hint.Text,
			UpdatedAt:   time.Now().UTC(),
		}
		if ok {
			task = existing
			task.Title = hint.Text
			task.Description = fmt.Sprintf("TODO in %s:%d", hint.Path, hint.Line)
			task.SourceKind = "todo"
			task.SourcePath = hint.Path
			task.SourceText = hint.Text
			if task.Status == StatusDone {
				task.Status = StatusQueue
			}
			task.UpdatedAt = time.Now().UTC()
		}
		if err := s.write(task); err != nil {
			return nil, err
		}
		if !ok || existing.Status == StatusDone {
			changed = append(changed, task)
		}
	}
	return changed, nil
}

// ReconcileTODOHints marks TODO-backed tasks done once the TODO is gone.
func (s *Store) ReconcileTODOHints(paths []string, hints []TODOHint) ([]Task, error) {
	if s.root == "" || len(paths) == 0 {
		return nil, nil
	}
	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	pathSet := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		pathSet[path] = struct{}{}
	}
	activeTodoIDs := make(map[string]struct{}, len(hints))
	for _, hint := range hints {
		activeTodoIDs[todoTaskID(hint.Path, hint.Text)] = struct{}{}
	}

	var changed []Task
	for _, task := range index {
		if task.SourceKind != "todo" {
			continue
		}
		if _, ok := pathSet[task.SourcePath]; !ok {
			continue
		}
		if _, ok := activeTodoIDs[task.ID]; ok {
			if task.Status == StatusDone {
				task.Status = StatusQueue
				task.UpdatedAt = time.Now().UTC()
				if err := s.write(task); err != nil {
					return nil, err
				}
				changed = append(changed, task)
			}
			continue
		}
		if task.Status != StatusDone {
			task.Status = StatusDone
			task.UpdatedAt = time.Now().UTC()
			if err := s.write(task); err != nil {
				return nil, err
			}
			changed = append(changed, task)
		}
	}
	return changed, nil
}

// Summary renders a short human-readable summary for prompting.
func (s *Store) Summary() string {
	tasks, err := s.List()
	if err != nil || len(tasks) == 0 {
		return "No durable tasks recorded."
	}

	var lines []string
	for i, task := range tasks {
		if i >= 8 {
			break
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s: %s", task.Status, task.ID, task.Title))
	}
	return strings.Join(lines, "\n")
}

// Counts returns task counts by status.
func (s *Store) Counts() map[string]int {
	counts := map[string]int{
		StatusQueue:   0,
		StatusActive:  0,
		StatusDone:    0,
		StatusBlocked: 0,
	}
	tasks, err := s.List()
	if err != nil {
		return counts
	}
	for _, task := range tasks {
		counts[task.Status]++
	}
	return counts
}

// List returns all known tasks, newest first.
func (s *Store) List() ([]Task, error) {
	if s.root == "" {
		return nil, nil
	}
	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	tasks := make([]Task, 0, len(index))
	for _, task := range index {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt)
	})
	return tasks, nil
}

func (s *Store) loadIndex() (map[string]Task, error) {
	index := make(map[string]Task)
	for _, status := range []string{StatusQueue, StatusActive, StatusDone, StatusBlocked} {
		dir := filepath.Join(s.root, status)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			var task Task
			if err := json.Unmarshal(data, &task); err != nil {
				return nil, err
			}
			task.Status = status
			index[task.ID] = task
		}
	}
	return index, nil
}

func (s *Store) write(task Task) error {
	for _, status := range []string{StatusQueue, StatusActive, StatusDone, StatusBlocked} {
		_ = os.Remove(filepath.Join(s.root, status, task.ID+".json"))
	}
	path := filepath.Join(s.root, task.Status, task.ID+".json")
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case StatusActive, "in_progress":
		return StatusActive
	case StatusDone, "complete", "completed":
		return StatusDone
	case StatusBlocked:
		return StatusBlocked
	default:
		return StatusQueue
	}
}

func slugify(input string) string {
	s := strings.ToLower(strings.TrimSpace(input))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteRune('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = fmt.Sprintf("task-%d", time.Now().Unix())
	}
	return out
}

func todoTaskID(path, text string) string {
	return slugify(path + "-" + text)
}
