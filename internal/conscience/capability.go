package conscience

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// CapabilityType represents the type of capability
type CapabilityType string

const (
	CapabilityTool       CapabilityType = "tool"        // Can use a specific tool
	CapabilityFlow       CapabilityType = "flow"        // Can execute a flow
	CapabilityKnowledge  CapabilityType = "knowledge"   // Has knowledge about a domain
	CapabilityReasoning  CapabilityType = "reasoning"   // Can perform reasoning type
	CapabilityIntegration CapabilityType = "integration" // Can integrate with external system
)

// Capability represents a single capability
type Capability struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        CapabilityType    `json:"type"`
	Description string            `json:"description"`
	Proficiency float64           `json:"proficiency"` // 0-1 score
	LastUsed    time.Time         `json:"last_used"`
	UsageCount  int               `json:"usage_count"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Limitations []string          `json:"limitations,omitempty"` // Known limitations
}

// CapabilityRegistry tracks what MARCUS can and cannot do
type Registry struct {
	mu              sync.RWMutex
	capabilities    map[string]*Capability
	dataDir         string
	selfAssessment  *SelfAssessment
}

// SelfAssessment tracks MARCUS's self-knowledge
type SelfAssessment struct {
	Strengths          []string  `json:"strengths"`
	Weaknesses         []string  `json:"weaknesses"`
	KnownLimitations   []string  `json:"known_limitations"`
	RequiresHumanHelp  []string  `json:"requires_human_help"` // Tasks that need human intervention
	LastUpdated        time.Time `json:"last_updated"`
}

// NewRegistry creates a new capability registry
func NewRegistry(dataDir string) *Registry {
	r := &Registry{
		capabilities: make(map[string]*Capability),
		dataDir:      filepath.Join(dataDir, "conscience"),
		selfAssessment: &SelfAssessment{
			Strengths:        make([]string, 0),
			Weaknesses:       make([]string, 0),
			KnownLimitations: make([]string, 0),
			RequiresHumanHelp: make([]string, 0),
			LastUpdated:      time.Now(),
		},
	}
	r.loadCapabilities()
	r.loadSelfAssessment()
	return r
}

// RegisterCapability registers or updates a capability
func (r *Registry) RegisterCapability(cap Capability) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.capabilities[cap.ID]; ok {
		existing.Proficiency = cap.Proficiency
		existing.LastUsed = cap.LastUsed
		existing.UsageCount = cap.UsageCount
		if cap.Description != "" {
			existing.Description = cap.Description
		}
		if len(cap.Tags) > 0 {
			existing.Tags = cap.Tags
		}
		if len(cap.Limitations) > 0 {
			existing.Limitations = cap.Limitations
		}
	} else {
		r.capabilities[cap.ID] = &cap
	}

	r.saveCapabilities()
}

// RecordUsage records usage of a capability
func (r *Registry) RecordUsage(capabilityID string, success bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cap, ok := r.capabilities[capabilityID]
	if !ok {
		return
	}

	cap.UsageCount++
	cap.LastUsed = time.Now()

	// Adjust proficiency based on success
	if success {
		cap.Proficiency = min(1.0, cap.Proficiency+0.01)
	} else {
		cap.Proficiency = max(0.0, cap.Proficiency-0.02)
	}

	r.saveCapabilities()
}

// GetCapability returns a capability by ID
func (r *Registry) GetCapability(id string) (*Capability, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cap, ok := r.capabilities[id]
	if !ok {
		return nil, false
	}

	copy := *cap
	return &copy, true
}

// ListCapabilities returns all capabilities
func (r *Registry) ListCapabilities() []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()

	caps := make([]Capability, 0, len(r.capabilities))
	for _, c := range r.capabilities {
		caps = append(caps, *c)
	}

	// Sort by proficiency descending
	sort.Slice(caps, func(i, j int) bool {
		return caps[i].Proficiency > caps[j].Proficiency
	})

	return caps
}

// FindCapabilities finds capabilities matching a query
func (r *Registry) FindCapabilities(query string, capType CapabilityType) []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []Capability
	queryLower := strings.ToLower(query)

	for _, cap := range r.capabilities {
		// Filter by type if specified
		if capType != "" && cap.Type != capType {
			continue
		}

		// Match against name, description, tags
		matched := strings.Contains(strings.ToLower(cap.Name), queryLower) ||
			strings.Contains(strings.ToLower(cap.Description), queryLower)

		if !matched {
			for _, tag := range cap.Tags {
				if strings.Contains(strings.ToLower(tag), queryLower) {
					matched = true
					break
				}
			}
		}

		if matched {
			results = append(results, *cap)
		}
	}

	return results
}

// CanDo checks if MARCUS can perform a task with acceptable proficiency
func (r *Registry) CanDo(taskDescription string, minProficiency float64) (bool, *Capability) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	taskLower := strings.ToLower(taskDescription)

	for _, cap := range r.capabilities {
		if cap.Proficiency < minProficiency {
			continue
		}

		// Check if capability matches task
		if strings.Contains(taskLower, strings.ToLower(cap.Name)) ||
			strings.Contains(taskLower, strings.ToLower(cap.Description)) {
			for _, tag := range cap.Tags {
				if strings.Contains(taskLower, strings.ToLower(tag)) {
					return true, cap
				}
			}
		}
	}

	return false, nil
}

// ShouldAskForHelp determines if MARCUS should ask for human help
func (r *Registry) ShouldAskForHelp(taskDescription string) (bool, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	taskLower := strings.ToLower(taskDescription)

	// Check if task matches known limitations
	for _, limitation := range r.selfAssessment.RequiresHumanHelp {
		if strings.Contains(taskLower, strings.ToLower(limitation)) {
			return true, limitation
		}
	}

	// Check if no capable capability exists
	for _, cap := range r.capabilities {
		if cap.Proficiency > 0.7 {
			if strings.Contains(taskLower, strings.ToLower(cap.Name)) {
				return false, ""
			}
		}
	}

	// If we have very low proficiency capabilities for this task, ask for help
	hasLowProficiency := false
	for _, cap := range r.capabilities {
		if cap.Proficiency < 0.3 {
			if strings.Contains(taskLower, strings.ToLower(cap.Name)) {
				hasLowProficiency = true
				break
			}
		}
	}

	if hasLowProficiency {
		return true, "low proficiency in required capability"
	}

	return false, ""
}

// UpdateSelfAssessment updates the self-assessment
func (r *Registry) UpdateSelfAssessment(strengths, weaknesses, limitations, needsHelp []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.selfAssessment.Strengths = strengths
	r.selfAssessment.Weaknesses = weaknesses
	r.selfAssessment.KnownLimitations = limitations
	r.selfAssessment.RequiresHumanHelp = needsHelp
	r.selfAssessment.LastUpdated = time.Now()

	r.saveSelfAssessment()
}

// GetSelfAssessment returns the current self-assessment
func (r *Registry) GetSelfAssessment() SelfAssessment {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return *r.selfAssessment
}

// GetConfidenceLevel returns overall confidence level for a task
func (r *Registry) GetConfidenceLevel(taskDescription string) ConfidenceLevel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	taskLower := strings.ToLower(taskDescription)
	var totalProficiency float64
	var count int

	for _, cap := range r.capabilities {
		if strings.Contains(taskLower, strings.ToLower(cap.Name)) ||
			strings.Contains(taskLower, strings.ToLower(cap.Description)) {
			totalProficiency += cap.Proficiency
			count++
		}
	}

	if count == 0 {
		return ConfidenceLevelUnknown
	}

	avgProficiency := totalProficiency / float64(count)

	if avgProficiency >= 0.8 {
		return ConfidenceLevelHigh
	} else if avgProficiency >= 0.5 {
		return ConfidenceLevelMedium
	} else if avgProficiency >= 0.3 {
		return ConfidenceLevelLow
	}

	return ConfidenceLevelVeryLow
}

// ConfidenceLevel represents confidence in performing a task
type ConfidenceLevel string

const (
	ConfidenceLevelHigh     ConfidenceLevel = "high"
	ConfidenceLevelMedium   ConfidenceLevel = "medium"
	ConfidenceLevelLow      ConfidenceLevel = "low"
	ConfidenceLevelVeryLow  ConfidenceLevel = "very_low"
	ConfidenceLevelUnknown  ConfidenceLevel = "unknown"
)

// GetSummary returns a human-readable summary
func (r *Registry) GetSummary() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("Capability Summary:\n\n")

	sb.WriteString("Registered Capabilities:\n")
	for _, cap := range r.capabilities {
		sb.WriteString("- ")
		sb.WriteString(cap.Name)
		sb.WriteString(" (")
		sb.WriteString(string(cap.Type))
		sb.WriteString("): ")
		sb.WriteString(formatProficiency(cap.Proficiency))
		sb.WriteString("\n")
	}

	sb.WriteString("\nSelf-Assessment:\n")
	sb.WriteString("Strengths: ")
	sb.WriteString(strings.Join(r.selfAssessment.Strengths, ", "))
	sb.WriteString("\n")
	sb.WriteString("Weaknesses: ")
	sb.WriteString(strings.Join(r.selfAssessment.Weaknesses, ", "))
	sb.WriteString("\n")
	sb.WriteString("Requires Human Help: ")
	sb.WriteString(strings.Join(r.selfAssessment.RequiresHumanHelp, ", "))

	return sb.String()
}

func formatProficiency(p float64) string {
	if p >= 0.8 {
		return "expert"
	} else if p >= 0.6 {
		return "proficient"
	} else if p >= 0.4 {
		return "intermediate"
	} else if p >= 0.2 {
		return "beginner"
	}
	return "novice"
}

func (r *Registry) saveCapabilities() error {
	if r.dataDir == "" {
		return nil
	}

	if err := os.MkdirAll(r.dataDir, 0755); err != nil {
		return err
	}

	path := filepath.Join(r.dataDir, "capabilities.json")
	data, _ := json.MarshalIndent(r.capabilities, "", "  ")
	return os.WriteFile(path, data, 0644)
}

func (r *Registry) loadCapabilities() error {
	if r.dataDir == "" {
		return nil
	}

	path := filepath.Join(r.dataDir, "capabilities.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &r.capabilities)
}

func (r *Registry) saveSelfAssessment() error {
	if r.dataDir == "" {
		return nil
	}

	if err := os.MkdirAll(r.dataDir, 0755); err != nil {
		return err
	}

	path := filepath.Join(r.dataDir, "self_assessment.json")
	data, _ := json.MarshalIndent(r.selfAssessment, "", "  ")
	return os.WriteFile(path, data, 0644)
}

func (r *Registry) loadSelfAssessment() error {
	if r.dataDir == "" {
		return nil
	}

	path := filepath.Join(r.dataDir, "self_assessment.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &r.selfAssessment)
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
