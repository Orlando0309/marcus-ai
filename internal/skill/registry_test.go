package skill

import (
	"context"
	"testing"
)

// MockSkill is a test implementation of Skill
type MockSkill struct {
	name        string
	pattern     string
	description string
	runCalled   bool
	lastArgs    []string
	returnDone  bool
}

func (m *MockSkill) Name() string { return m.name }

func (m *MockSkill) Pattern() string { return m.pattern }

func (m *MockSkill) Description() string { return m.description }

func (m *MockSkill) Run(ctx context.Context, args []string, deps Dependencies) (Result, error) {
	m.runCalled = true
	m.lastArgs = args
	return Result{Done: m.returnDone}, nil
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.skills == nil {
		t.Error("skills map not initialized")
	}
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	skill := &MockSkill{name: "test", pattern: "/test"}

	r.Register(skill)

	if len(r.skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(r.skills))
	}

	if r.skills["/test"] != skill {
		t.Error("skill not registered correctly")
	}
}

func TestRegistry_Parse(t *testing.T) {
	r := NewRegistry()
	skill := &MockSkill{name: "commit", pattern: "/commit", description: "Create commit"}
	r.Register(skill)

	tests := []struct {
		name         string
		input        string
		wantSkill    bool
		wantArgs     []string
		wantPattern  string
	}{
		{
			name:        "simple command",
			input:       "/commit",
			wantSkill:   true,
			wantArgs:    []string{},
			wantPattern: "/commit",
		},
		{
			name:        "command with args",
			input:       "/commit -m \"test message\"",
			wantSkill:   true,
			wantArgs:    []string{"-m", "\"test", "message\""},
			wantPattern: "/commit",
		},
		{
			name:      "not a command",
			input:     "hello world",
			wantSkill: false,
		},
		{
			name:      "empty string",
			input:     "",
			wantSkill: false,
		},
		{
			name:      "whitespace only",
			input:     "   ",
			wantSkill: false,
		},
		{
			name:      "unknown command",
			input:     "/unknown",
			wantSkill: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, args, ok := r.Parse(tt.input)
			if ok != tt.wantSkill {
				t.Errorf("Parse() ok = %v, want %v", ok, tt.wantSkill)
				return
			}
			if !tt.wantSkill {
				return
			}
			if s.Pattern() != tt.wantPattern {
				t.Errorf("Parse() pattern = %s, want %s", s.Pattern(), tt.wantPattern)
			}
			if len(args) != len(tt.wantArgs) {
				t.Errorf("Parse() args = %v, want %v", args, tt.wantArgs)
			}
		})
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()
	skill := &MockSkill{name: "help", pattern: "/help"}
	r.Register(skill)

	t.Run("existing skill", func(t *testing.T) {
		s, ok := r.Get("/help")
		if !ok {
			t.Error("expected to find skill")
		}
		if s.Name() != "help" {
			t.Errorf("expected skill name 'help', got %s", s.Name())
		}
	})

	t.Run("non-existent skill", func(t *testing.T) {
		_, ok := r.Get("/nonexistent")
		if ok {
			t.Error("expected not to find skill")
		}
	})
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()

	// Empty registry
	skills := r.List()
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}

	// Add skills
	s1 := &MockSkill{name: "help", pattern: "/help"}
	s2 := &MockSkill{name: "clear", pattern: "/clear"}
	r.Register(s1)
	r.Register(s2)

	skills = r.List()
	if len(skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skills))
	}
}

func TestRegistry_Patterns(t *testing.T) {
	r := NewRegistry()
	r.Register(&MockSkill{pattern: "/help"})
	r.Register(&MockSkill{pattern: "/clear"})

	patterns := r.Patterns()
	if len(patterns) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(patterns))
	}

	// Check that expected patterns exist
	hasHelp := false
	hasClear := false
	for _, p := range patterns {
		if p == "/help" {
			hasHelp = true
		}
		if p == "/clear" {
			hasClear = true
		}
	}
	if !hasHelp || !hasClear {
		t.Error("expected patterns not found")
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()

	// Concurrent writes
	for i := 0; i < 100; i++ {
		go func(i int) {
			skill := &MockSkill{name: string(rune(i)), pattern: "/" + string(rune(i))}
			r.Register(skill)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		go func() {
			r.List()
			r.Patterns()
		}()
	}
}

// Parser tests
func TestParseSlashCommand(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantCmd    string
		wantArgs   []string
		wantOk     bool
	}{
		{
			name:     "simple command",
			input:    "/help",
			wantCmd:  "help",
			wantArgs: []string{},
			wantOk:   true,
		},
		{
			name:     "command with args",
			input:    "/model ollama:qwen",
			wantCmd:  "model",
			wantArgs: []string{"ollama:qwen"},
			wantOk:   true,
		},
		{
			name:     "command with multiple args",
			input:    "/commit -m \"hello world\" --amend",
			wantCmd:  "commit",
			wantArgs: []string{"-m", "\"hello", "world\"", "--amend"},
			wantOk:   true,
		},
		{
			name:   "no slash prefix",
			input:  "help",
			wantOk: false,
		},
		{
			name:   "empty string",
			input:  "",
			wantOk: false,
		},
		{
			name:   "whitespace",
			input:  "   ",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args, ok := ParseSlashCommand(tt.input)
			if ok != tt.wantOk {
				t.Errorf("ParseSlashCommand() ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if !tt.wantOk {
				return
			}
			if cmd != tt.wantCmd {
				t.Errorf("ParseSlashCommand() cmd = %v, want %v", cmd, tt.wantCmd)
			}
			if len(args) != len(tt.wantArgs) {
				t.Errorf("ParseSlashCommand() args = %v, want %v", args, tt.wantArgs)
			}
		})
	}
}

func TestIsSlashCommand(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/help", true},
		{"/model", true},
		{"hello", false},
		{"", false},
		{"  /help", true},
		{"/", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := IsSlashCommand(tt.input); got != tt.want {
				t.Errorf("IsSlashCommand(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractCommandName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/help", "help"},
		{"/model ollama:qwen", "model"},
		{"hello", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ExtractCommandName(tt.input); got != tt.want {
				t.Errorf("ExtractCommandName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
