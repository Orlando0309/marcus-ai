package memory

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	ScopeUser      = "user"
	ScopeProject   = "project"
	ScopeReference = "reference"
)

type Entry struct {
	ID        string    `json:"id"`
	Scope     string    `json:"scope"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Source    string    `json:"source,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Manager struct {
	root        string
	recallLimit int
}

func NewManager(projectRoot string, recallLimit int) *Manager {
	if projectRoot == "" {
		return &Manager{recallLimit: recallLimit}
	}
	if recallLimit <= 0 {
		recallLimit = 8
	}
	return &Manager{
		root:        filepath.Join(projectRoot, ".marcus", "memory"),
		recallLimit: recallLimit,
	}
}

func (m *Manager) EnsureStructure() error {
	if m.root == "" {
		return nil
	}
	for _, dir := range []string{
		m.root,
		filepath.Join(m.root, ScopeUser),
		filepath.Join(m.root, ScopeProject),
		filepath.Join(m.root, ScopeReference),
		filepath.Join(m.root, "decisions"),
		filepath.Join(m.root, "patterns"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) Remember(scope, kind, title, content, source string, tags ...string) (*Entry, error) {
	if m.root == "" {
		return nil, nil
	}
	if err := m.EnsureStructure(); err != nil {
		return nil, err
	}
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" || content == "" {
		return nil, nil
	}
	if scope == "" {
		scope = ScopeProject
	}
	entry := &Entry{
		ID:        entryID(scope, kind, title, source),
		Scope:     scope,
		Kind:      strings.TrimSpace(kind),
		Title:     title,
		Content:   content,
		Source:    strings.TrimSpace(source),
		Tags:      tags,
		UpdatedAt: time.Now().UTC(),
	}
	return entry, m.write(*entry)
}

func (m *Manager) CaptureUserFeedback(input string) ([]Entry, error) {
	if m.root == "" {
		return nil, nil
	}
	lower := strings.ToLower(strings.TrimSpace(input))
	if lower == "" {
		return nil, nil
	}
	var captures []Entry
	candidates := []string{
		"i prefer ",
		"prefer ",
		"always ",
		"never ",
		"don't ",
		"do not ",
		"i like ",
		"i want ",
	}
	for _, prefix := range candidates {
		if strings.Contains(lower, prefix) {
			title := "User preference"
			entry, err := m.Remember(ScopeUser, "feedback", title, input, "conversation", "user-feedback")
			if err != nil {
				return nil, err
			}
			if entry != nil {
				captures = append(captures, *entry)
			}
			break
		}
	}
	return captures, nil
}

func (m *Manager) Recall(query string, limit int) ([]Entry, error) {
	if m.root == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = m.recallLimit
	}
	entries, err := m.List()
	if err != nil {
		return nil, err
	}
	type scored struct {
		Entry
		score int
	}
	queryTokens := tokenize(query)
	var scoredEntries []scored
	for _, entry := range entries {
		score := 1
		text := strings.ToLower(entry.Title + " " + entry.Content + " " + strings.Join(entry.Tags, " "))
		for _, token := range queryTokens {
			if strings.Contains(text, token) {
				score += 3
			}
		}
		// Recency boost: entries touched in the last 7 days rank higher.
		if time.Since(entry.UpdatedAt) < 7*24*time.Hour {
			score += 2
		}
		switch entry.Kind {
		case "pattern", "decision", "loop-decision":
			score += 1
		}
		if score > 1 || len(queryTokens) == 0 {
			scoredEntries = append(scoredEntries, scored{Entry: entry, score: score})
		}
	}
	sort.Slice(scoredEntries, func(i, j int) bool {
		if scoredEntries[i].score == scoredEntries[j].score {
			return scoredEntries[i].UpdatedAt.After(scoredEntries[j].UpdatedAt)
		}
		return scoredEntries[i].score > scoredEntries[j].score
	})
	if len(scoredEntries) > limit {
		scoredEntries = scoredEntries[:limit]
	}
	result := make([]Entry, 0, len(scoredEntries))
	for _, item := range scoredEntries {
		result = append(result, item.Entry)
	}
	return result, nil
}

// AppendEpisodic appends one conversation line to episodic.jsonl (short snippets).
func (m *Manager) AppendEpisodic(role, content string) error {
	if m.root == "" {
		return nil
	}
	if err := m.EnsureStructure(); err != nil {
		return err
	}
	content = trim(content, 800)
	rec, err := json.Marshal(map[string]string{
		"role": strings.TrimSpace(role),
		"text": content,
		"ts":   time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(m.root, "episodic.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(rec, '\n'))
	return err
}

// EpisodicSummary returns the last maxLines episodic entries as bullet text.
func (m *Manager) EpisodicSummary(maxLines int) string {
	if m.root == "" || maxLines <= 0 {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(m.root, "episodic.jsonl"))
	if err != nil || len(data) == 0 {
		return ""
	}
	raw := strings.TrimSuffix(string(data), "\n")
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	var out []string
	for _, ln := range lines {
		var row map[string]string
		if json.Unmarshal([]byte(ln), &row) != nil {
			continue
		}
		out = append(out, "- "+row["role"]+": "+trim(row["text"], 200))
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n")
}

func (m *Manager) Summary(query string, limit int) string {
	entries, err := m.Recall(query, limit)
	if err != nil || len(entries) == 0 {
		return "No durable memory recalled."
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, "- ["+entry.Scope+"/"+entry.Kind+"] "+entry.Title+": "+trim(entry.Content, 180))
	}
	return strings.Join(lines, "\n")
}

func (m *Manager) List() ([]Entry, error) {
	if m.root == "" {
		return nil, nil
	}
	var entries []Entry
	for _, scope := range []string{ScopeUser, ScopeProject, ScopeReference, "decisions", "patterns"} {
		dir := filepath.Join(m.root, scope)
		items, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, item := range items {
			if item.IsDir() || filepath.Ext(item.Name()) != ".json" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, item.Name()))
			if err != nil {
				return nil, err
			}
			var entry Entry
			if err := json.Unmarshal(data, &entry); err != nil {
				return nil, err
			}
			if entry.Scope == "" {
				entry.Scope = scope
			}
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].UpdatedAt.After(entries[j].UpdatedAt)
	})
	return entries, nil
}

func (m *Manager) write(entry Entry) error {
	scope := entry.Scope
	if scope == "" {
		scope = ScopeProject
	}
	path := filepath.Join(m.root, scope, entry.ID+".json")
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func entryID(scope, kind, title, source string) string {
	h := sha1.Sum([]byte(scope + "|" + kind + "|" + title + "|" + source))
	return hex.EncodeToString(h[:8])
}

func tokenize(input string) []string {
	parts := strings.Fields(strings.ToLower(input))
	var tokens []string
	for _, part := range parts {
		part = strings.Trim(part, " ,.;:()[]{}\"'")
		if len(part) >= 3 {
			tokens = append(tokens, part)
		}
	}
	return tokens
}

func trim(input string, limit int) string {
	if limit <= 0 || len(input) <= limit {
		return strings.TrimSpace(input)
	}
	return strings.TrimSpace(input[:limit]) + "..."
}
