package session

import (
	"testing"

	"github.com/marcus-ai/marcus/internal/provider"
)

func TestSessionStoresStructuredTranscript(t *testing.T) {
	s := newSession()
	s.AppendTurn("user", "hello", 10)
	s.AppendAction("write file", "applied")
	s.AppendEvent("tool_result", "tool", "read_file", "ok", `{"path":"a.txt"}`, "ok", map[string]string{"path": "a.txt"})
	s.SetProviderMessages([]provider.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}, 10)
	if len(s.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(s.Events))
	}
	if len(s.ProviderMessages) != 3 {
		t.Fatalf("expected provider messages to persist")
	}
}
