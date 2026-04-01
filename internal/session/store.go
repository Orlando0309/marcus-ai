package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/marcus-ai/marcus/internal/provider"
)

// Turn is a single conversation turn in a Marcus session.
type Turn struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// ActionLog records an approved or rejected proposal.
type ActionLog struct {
	Label     string    `json:"label"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// Event captures structured session activity for resume and auditability.
type Event struct {
	Kind      string            `json:"kind"`
	Role      string            `json:"role,omitempty"`
	Name      string            `json:"name,omitempty"`
	Content   string            `json:"content,omitempty"`
	Input     string            `json:"input,omitempty"`
	Status    string            `json:"status,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// Session is the persisted chat state.
type Session struct {
	ID               string             `json:"id"`
	CreatedAt        time.Time          `json:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at"`
	Turns            []Turn             `json:"turns"`
	Actions          []ActionLog        `json:"actions"`
	Events           []Event            `json:"events,omitempty"`
	ProviderMessages []provider.Message `json:"provider_messages,omitempty"`
	LastContext      string             `json:"last_context,omitempty"`
}

// Store persists sessions under `.marcus/sessions/`.
type Store struct {
	root string
}

// NewStore creates a session store.
func NewStore(projectRoot string) *Store {
	if projectRoot == "" {
		return &Store{}
	}
	return &Store{root: filepath.Join(projectRoot, ".marcus", "sessions")}
}

// LoadLatest restores the latest session if present.
func (s *Store) LoadLatest() (*Session, error) {
	if s.root == "" {
		return newSession(), nil
	}
	if err := os.MkdirAll(s.root, 0755); err != nil {
		return nil, err
	}
	path := filepath.Join(s.root, "latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newSession(), nil
		}
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

// Save writes the latest session snapshot and a timestamped archive.
func (s *Store) Save(sess *Session) error {
	if s.root == "" || sess == nil {
		return nil
	}
	if err := os.MkdirAll(s.root, 0755); err != nil {
		return err
	}
	sess.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.root, "latest.json"), data, 0600); err != nil {
		return err
	}
	archive := filepath.Join(s.root, sess.ID+".json")
	return os.WriteFile(archive, data, 0600)
}

// AppendTurn records a turn and trims history to the provided limit.
func (s *Session) AppendTurn(role, content string, maxTurns int) {
	s.Turns = append(s.Turns, Turn{
		Role:      role,
		Content:   content,
		CreatedAt: time.Now().UTC(),
	})
	s.Events = append(s.Events, Event{
		Kind:      "turn",
		Role:      role,
		Content:   content,
		CreatedAt: time.Now().UTC(),
	})
	if maxTurns > 0 && len(s.Turns) > maxTurns {
		s.Turns = s.Turns[len(s.Turns)-maxTurns:]
	}
	s.UpdatedAt = time.Now().UTC()
}

// AppendAction records an action state transition.
func (s *Session) AppendAction(label, status string) {
	s.Actions = append(s.Actions, ActionLog{
		Label:     label,
		Status:    status,
		CreatedAt: time.Now().UTC(),
	})
	s.Events = append(s.Events, Event{
		Kind:      "action",
		Name:      label,
		Status:    status,
		CreatedAt: time.Now().UTC(),
	})
	s.UpdatedAt = time.Now().UTC()
}

// AppendEvent records one structured event in the session history.
func (s *Session) AppendEvent(kind, role, name, content, input, status string, metadata map[string]string) {
	s.Events = append(s.Events, Event{
		Kind:      kind,
		Role:      role,
		Name:      name,
		Content:   content,
		Input:     input,
		Status:    status,
		Metadata:  metadata,
		CreatedAt: time.Now().UTC(),
	})
	s.UpdatedAt = time.Now().UTC()
}

// SetProviderMessages stores the structured message transcript used by the agent loop.
func (s *Session) SetProviderMessages(messages []provider.Message, maxMessages int) {
	if maxMessages > 0 && len(messages) > maxMessages {
		messages = messages[len(messages)-maxMessages:]
	}
	s.ProviderMessages = append([]provider.Message(nil), messages...)
	s.UpdatedAt = time.Now().UTC()
}

// RecentProviderMessages returns the newest provider messages up to n.
func (s *Session) RecentProviderMessages(n int) []provider.Message {
	if n <= 0 || len(s.ProviderMessages) <= n {
		return append([]provider.Message(nil), s.ProviderMessages...)
	}
	return append([]provider.Message(nil), s.ProviderMessages[len(s.ProviderMessages)-n:]...)
}

// RecentTurns returns the newest turns up to n.
func (s *Session) RecentTurns(n int) []Turn {
	if n <= 0 || len(s.Turns) <= n {
		return s.Turns
	}
	return s.Turns[len(s.Turns)-n:]
}

func newSession() *Session {
	now := time.Now().UTC()
	return &Session{
		ID:        now.Format("2006-01-02T15-04-05"),
		CreatedAt: now,
		UpdatedAt: now,
	}
}
