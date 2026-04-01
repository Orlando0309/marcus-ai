package agent

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// BlackboardEntryType represents the type of blackboard entry
type BlackboardEntryType string

const (
	EntryFact       BlackboardEntryType = "fact"        // Established fact
	EntryHypothesis BlackboardEntryType = "hypothesis"  // Unproven hypothesis
	EntryFinding    BlackboardEntryType = "finding"     // Discovered information
	EntryDecision   BlackboardEntryType = "decision"    // Made decision
	EntryArtifact   BlackboardEntryType = "artifact"    // Generated artifact (code, doc, etc.)
	EntryQuestion   BlackboardEntryType = "question"    // Open question
	EntryTask       BlackboardEntryType = "task"        // Task or subtask
)

// BlackboardEntry is a piece of shared knowledge
type BlackboardEntry struct {
	ID          string                `json:"id"`
	CreatedBy   string                `json:"created_by"` // Agent ID who created it
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
	Type        BlackboardEntryType   `json:"type"`
	Subject     string                `json:"subject"`      // Brief title
	Content     string                `json:"content"`      // Full content
	Tags        []string              `json:"tags"`         // For categorization
	Confidence  float64               `json:"confidence"`   // 0-1 confidence level
	Sources     []string              `json:"sources"`      // IDs of entries this is based on
	References  []string              `json:"references"`   // IDs of related entries
	Metadata    map[string]any        `json:"metadata,omitempty"`
	ExpiresAt   *time.Time            `json:"expires_at,omitempty"` // Optional expiry
}

// Blackboard is a shared workspace for agent collaboration
type Blackboard struct {
	mu            sync.RWMutex
	entries       map[string]*BlackboardEntry
	indexByTag    map[string]map[string]bool
	indexByType   map[BlackboardEntryType]map[string]bool
	indexBySubject map[string]map[string]bool
	subscribers   []func(BlackboardEntry)
	maxEntries    int
}

// NewBlackboard creates a new shared blackboard
func NewBlackboard(maxEntries int) *Blackboard {
	return &Blackboard{
		entries:        make(map[string]*BlackboardEntry),
		indexByTag:     make(map[string]map[string]bool),
		indexByType:    make(map[BlackboardEntryType]map[string]bool),
		indexBySubject: make(map[string]map[string]bool),
		maxEntries:     maxEntries,
	}
}

// Write adds or updates an entry on the blackboard
func (b *Blackboard) Write(entry BlackboardEntry) string {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Generate ID if not present
	if entry.ID == "" {
		entry.ID = generateEntryID()
	}

	entry.CreatedAt = time.Now()
	entry.UpdatedAt = time.Now()

	// Check if updating existing entry
	existing, exists := b.entries[entry.ID]
	if exists {
		entry.CreatedAt = existing.CreatedAt
	}

	// Add to storage
	b.entries[entry.ID] = &entry

	// Update indexes
	b.indexEntry(entry)

	// Enforce max entries (remove oldest non-permanent)
	if len(b.entries) > b.maxEntries {
		b.removeOldest()
	}

	// Notify subscribers
	for _, sub := range b.subscribers {
		go sub(entry)
	}

	return entry.ID
}

// Read retrieves an entry by ID
func (b *Blackboard) Read(id string) (*BlackboardEntry, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	entry, ok := b.entries[id]
	if !ok {
		return nil, false
	}

	// Return copy
	copy := *entry
	return &copy, true
}

// Delete removes an entry from the blackboard
func (b *Blackboard) Delete(id string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry, ok := b.entries[id]
	if !ok {
		return false
	}

	// Remove from indexes
	b.deindexEntry(*entry)

	delete(b.entries, id)
	return true
}

// Query searches for entries matching criteria
func (b *Blackboard) Query(query Query) []BlackboardEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var results []BlackboardEntry

	for _, entry := range b.entries {
		if !query.Matches(entry) {
			continue
		}
		results = append(results, *entry)
	}

	// Sort by relevance (confidence, recency)
	sortEntriesByRelevance(results, query.SortBy)

	// Apply limit
	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}

	return results
}

// Query represents a blackboard query
type Query struct {
	Tags       []string
	Types      []BlackboardEntryType
	Subject    string
	MinConfidence float64
	CreatedAfter  time.Time
	Limit      int
	SortBy     string // "confidence", "recency", "relevance"
}

// Matches checks if an entry matches the query
func (q *Query) Matches(entry *BlackboardEntry) bool {
	// Check tags
	if len(q.Tags) > 0 {
		hasTag := false
		for _, tag := range q.Tags {
			for _, entryTag := range entry.Tags {
				if tag == entryTag {
					hasTag = true
					break
				}
			}
		}
		if !hasTag {
			return false
		}
	}

	// Check types
	if len(q.Types) > 0 {
		hasType := false
		for _, t := range q.Types {
			if entry.Type == t {
				hasType = true
				break
			}
		}
		if !hasType {
			return false
		}
	}

	// Check subject
	if q.Subject != "" {
		if !containsIgnoreCase(entry.Subject, q.Subject) &&
		   !containsIgnoreCase(entry.Content, q.Subject) {
			return false
		}
	}

	// Check confidence
	if entry.Confidence < q.MinConfidence {
		return false
	}

	// Check creation time
	if !q.CreatedAfter.IsZero() && entry.CreatedAt.Before(q.CreatedAfter) {
		return false
	}

	return true
}

// FindByTag finds all entries with a specific tag
func (b *Blackboard) FindByTag(tag string, limit int) []BlackboardEntry {
	return b.Query(Query{
		Tags:  []string{tag},
		Limit: limit,
	})
}

// FindByType finds all entries of a specific type
func (b *Blackboard) FindByType(entryType BlackboardEntryType, limit int) []BlackboardEntry {
	return b.Query(Query{
		Types: []BlackboardEntryType{entryType},
		Limit: limit,
	})
}

// FindRelated finds entries related to a given entry
func (b *Blackboard) FindRelated(entryID string, limit int) []BlackboardEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	entry, ok := b.entries[entryID]
	if !ok {
		return nil
	}

	var results []BlackboardEntry
	for _, refID := range entry.References {
		if ref, ok := b.entries[refID]; ok {
			results = append(results, *ref)
		}
	}
	for _, srcID := range entry.Sources {
		if src, ok := b.entries[srcID]; ok {
			results = append(results, *src)
		}
	}

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

// Link creates a relationship between entries
func (b *Blackboard) Link(fromID, toID, relationshipType string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	from, ok := b.entries[fromID]
	if !ok {
		return ErrEntryNotFound
	}

	to, ok := b.entries[toID]
	if !ok {
		return ErrEntryNotFound
	}

	// Add bidirectional reference
	from.References = appendUnique(from.References, toID)
	to.Sources = appendUnique(to.Sources, fromID)

	from.UpdatedAt = time.Now()
	to.UpdatedAt = time.Now()

	return nil
}

// Subscribe registers a callback for new entries
func (b *Blackboard) Subscribe(callback func(BlackboardEntry)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers = append(b.subscribers, callback)
}

// GetAllEntries returns all entries (for debugging/export)
func (b *Blackboard) GetAllEntries() []BlackboardEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	entries := make([]BlackboardEntry, 0, len(b.entries))
	for _, e := range b.entries {
		entries = append(entries, *e)
	}
	return entries
}

// GetStats returns blackboard statistics
func (b *Blackboard) GetStats() BlackboardStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := BlackboardStats{
		TotalEntries: len(b.entries),
		ByType:       make(map[BlackboardEntryType]int),
		ByTag:        make(map[string]int),
	}

	for _, e := range b.entries {
		stats.ByType[e.Type]++
		for _, tag := range e.Tags {
			stats.ByTag[tag]++
		}
	}

	return stats
}

// BlackboardStats holds blackboard statistics
type BlackboardStats struct {
	TotalEntries int                        `json:"total_entries"`
	ByType       map[BlackboardEntryType]int `json:"by_type"`
	ByTag        map[string]int              `json:"by_tag"`
}

func (b *Blackboard) indexEntry(entry BlackboardEntry) {
	// Index by tags
	for _, tag := range entry.Tags {
		if b.indexByTag[tag] == nil {
			b.indexByTag[tag] = make(map[string]bool)
		}
		b.indexByTag[tag][entry.ID] = true
	}

	// Index by type
	if b.indexByType[entry.Type] == nil {
		b.indexByType[entry.Type] = make(map[string]bool)
	}
	b.indexByType[entry.Type][entry.ID] = true

	// Index by subject keywords
	for _, word := range tokenize(entry.Subject) {
		if b.indexBySubject[word] == nil {
			b.indexBySubject[word] = make(map[string]bool)
		}
		b.indexBySubject[word][entry.ID] = true
	}
}

func (b *Blackboard) deindexEntry(entry BlackboardEntry) {
	// Remove from tag indexes
	for _, tag := range entry.Tags {
		if index := b.indexByTag[tag]; index != nil {
			delete(index, entry.ID)
		}
	}

	// Remove from type index
	if index := b.indexByType[entry.Type]; index != nil {
		delete(index, entry.ID)
	}

	// Remove from subject index
	for _, word := range tokenize(entry.Subject) {
		if index := b.indexBySubject[word]; index != nil {
			delete(index, entry.ID)
		}
	}
}

func (b *Blackboard) removeOldest() {
	var oldestID string
	var oldestTime time.Time

	for id, entry := range b.entries {
		if oldestID == "" || entry.CreatedAt.Before(oldestTime) {
			oldestID = id
			oldestTime = entry.CreatedAt
		}
	}

	if oldestID != "" {
		entry := b.entries[oldestID]
		b.deindexEntry(*entry)
		delete(b.entries, oldestID)
	}
}

func sortEntriesByRelevance(entries []BlackboardEntry, sortBy string) {
	switch sortBy {
	case "confidence":
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Confidence > entries[j].Confidence
		})
	case "recency":
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].CreatedAt.After(entries[j].CreatedAt)
		})
	default:
		// Default: combined score (confidence * 0.6 + recency * 0.4)
		sort.Slice(entries, func(i, j int) bool {
			scoreI := entries[i].Confidence*0.6 + recencyScore(entries[i])*0.4
			scoreJ := entries[j].Confidence*0.6 + recencyScore(entries[j])*0.4
			return scoreI > scoreJ
		})
	}
}

func recencyScore(entry BlackboardEntry) float64 {
	hoursSince := time.Since(entry.CreatedAt).Hours()
	// Score decays from 1 to 0 over 168 hours (1 week)
	score := 1.0 - (hoursSince / 168.0)
	if score < 0 {
		return 0
	}
	return score
}

func generateEntryID() string {
	return "bb_" + time.Now().Format("20060102150405.000000")
}

func containsIgnoreCase(s, substr string) bool {
	return contains(s, substr)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func appendUnique(slice []string, item string) []string {
	for _, existing := range slice {
		if existing == item {
			return slice
		}
	}
	return append(slice, item)
}

func tokenize(s string) []string {
	// Simple tokenization
	s = strings.ToLower(s)
	var tokens []string
	for _, word := range strings.Fields(s) {
		word = strings.Trim(word, ".,;:()[]{}\"'-")
		if len(word) >= 3 {
			tokens = append(tokens, word)
		}
	}
	return tokens
}

// Errors
var (
	ErrEntryNotFound = &BlackboardError{"entry not found"}
)

// BlackboardError represents a blackboard error
type BlackboardError struct {
	Message string
}

func (e *BlackboardError) Error() string {
	return e.Message
}
