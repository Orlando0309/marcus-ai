package skill

import (
	"strings"
	"sync"
)

// Registry manages skill registration and lookup
type Registry struct {
	skills map[string]Skill // keyed by Pattern()
	mu     sync.RWMutex
}

// NewRegistry creates a new skill registry
func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string]Skill),
	}
}

// Register adds a skill to the registry
func (r *Registry) Register(skill Skill) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.skills == nil {
		r.skills = make(map[string]Skill)
	}
	r.skills[skill.Pattern()] = skill
}

// Parse extracts a slash command from input and returns the matching skill
// Returns the skill, any arguments, and true if a skill was found
func (r *Registry) Parse(input string) (Skill, []string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return nil, nil, false
	}

	// Split into command and arguments
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil, nil, false
	}

	command := parts[0]
	args := parts[1:]

	skill, ok := r.skills[command]
	if !ok {
		return nil, nil, false
	}

	return skill, args, true
}

// Get retrieves a skill by its pattern
func (r *Registry) Get(pattern string) (Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skill, ok := r.skills[pattern]
	return skill, ok
}

// List returns all registered skills
func (r *Registry) List() []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skills := make([]Skill, 0, len(r.skills))
	for _, skill := range r.skills {
		skills = append(skills, skill)
	}
	return skills
}

// Patterns returns all registered skill patterns
func (r *Registry) Patterns() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	patterns := make([]string, 0, len(r.skills))
	for pattern := range r.skills {
		patterns = append(patterns, pattern)
	}
	return patterns
}
