package codeintel

import (
	"fmt"
	"math"
	"sync"
)

// VectorStoreConfig holds configuration for vector store selection
type VectorStoreConfig struct {
	Type      string        `toml:"type"`       // "memory", "chroma", "pgvector"
	Chroma    ChromaConfig  `toml:"chroma,omitempty"`
	PgVector  PgVectorConfig `toml:"pgvector,omitempty"`
	Mock      MockConfig    `toml:"mock,omitempty"`
}

// MockConfig holds mock vector store configuration
type MockConfig struct {
	Dimensions int `toml:"dimensions"`
	NumItems   int `toml:"num_items"`
}

// NewVectorStoreFromConfig creates a vector store from configuration
func NewVectorStoreFromConfig(cfg VectorStoreConfig) (VectorStore, error) {
	switch cfg.Type {
	case "", "memory":
		return NewInMemoryVectorStore(), nil
	case "chroma":
		return NewChromaVectorStore(cfg.Chroma)
	case "pgvector":
		return NewPgVectorStore(cfg.PgVector)
	case "mock":
		return NewMockVectorStore(cfg.Mock), nil
	default:
		return nil, fmt.Errorf("unknown vector store type: %s", cfg.Type)
	}
}

// VectorStore persists and queries embeddings
type VectorStore interface {
	Store(id string, embedding []float32, metadata map[string]any) error
	Query(embedding []float32, limit int) ([]SearchResult, error)
	Delete(id string) error
	Size() int
}

// SearchResult represents a search result with similarity score
type SearchResult struct {
	ID       string
	Score    float32
	Metadata map[string]any
}

// InMemoryVectorStore is an in-memory vector store using brute force search
type InMemoryVectorStore struct {
	mu         sync.RWMutex
	embeddings map[string][]float32
	metadata   map[string]map[string]any
}

// NewInMemoryVectorStore creates a new in-memory vector store
func NewInMemoryVectorStore() *InMemoryVectorStore {
	return &InMemoryVectorStore{
		embeddings: make(map[string][]float32),
		metadata:   make(map[string]map[string]any),
	}
}

// Store stores an embedding with metadata
func (s *InMemoryVectorStore) Store(id string, embedding []float32, metadata map[string]any) error {
	if len(embedding) == 0 {
		return fmt.Errorf("empty embedding")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Make a copy of the embedding
	embCopy := make([]float32, len(embedding))
	copy(embCopy, embedding)
	s.embeddings[id] = embCopy

	// Store metadata
	if metadata != nil {
		metaCopy := make(map[string]any)
		for k, v := range metadata {
			metaCopy[k] = v
		}
		s.metadata[id] = metaCopy
	}

	return nil
}

// Query searches for similar embeddings
func (s *InMemoryVectorStore) Query(embedding []float32, limit int) ([]SearchResult, error) {
	if len(embedding) == 0 {
		return nil, fmt.Errorf("empty query embedding")
	}

	if limit <= 0 {
		limit = 10
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Brute force search - calculate cosine similarity with all embeddings
	type scoredResult struct {
		id    string
		score float32
	}

	results := make([]scoredResult, 0, len(s.embeddings))
	for id, emb := range s.embeddings {
		if len(emb) != len(embedding) {
			continue // Skip embeddings with different dimensions
		}
		score := cosineSimilarity(embedding, emb)
		results = append(results, scoredResult{id: id, score: score})
	}

	// Sort by score (descending)
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Return top results
	if limit > len(results) {
		limit = len(results)
	}

	output := make([]SearchResult, 0, limit)
	for i := 0; i < limit; i++ {
		result := SearchResult{
			ID:    results[i].id,
			Score: results[i].score,
		}
		if meta, ok := s.metadata[results[i].id]; ok {
			result.Metadata = make(map[string]any)
			for k, v := range meta {
				result.Metadata[k] = v
			}
		}
		output = append(output, result)
	}

	return output, nil
}

// Delete removes an embedding from the store
func (s *InMemoryVectorStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.embeddings, id)
	delete(s.metadata, id)
	return nil
}

// Size returns the number of stored embeddings
func (s *InMemoryVectorStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.embeddings)
}

// Clear removes all embeddings
func (s *InMemoryVectorStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embeddings = make(map[string][]float32)
	s.metadata = make(map[string]map[string]any)
}

// cosineSimilarity calculates the cosine similarity between two vectors
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct float64
	var normA float64
	var normB float64

	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// euclideanDistance calculates the Euclidean distance between two vectors
func euclideanDistance(a, b []float32) float32 {
	if len(a) != len(b) {
		return float32(math.Inf(1))
	}

	var sum float64
	for i := 0; i < len(a); i++ {
		diff := float64(a[i] - b[i])
		sum += diff * diff
	}

	return float32(math.Sqrt(sum))
}

// MockVectorStore is a mock vector store for testing
type MockVectorStore struct {
	data       map[string]MockVectorItem
	dimensions int
}

// MockVectorItem is an item in the mock vector store
type MockVectorItem struct {
	Embedding []float32
	Metadata  map[string]any
}

// NewMockVectorStore creates a new mock vector store
func NewMockVectorStore(cfg MockConfig) *MockVectorStore {
	if cfg.Dimensions <= 0 {
		cfg.Dimensions = 128
	}
	return &MockVectorStore{
		data:       make(map[string]MockVectorItem),
		dimensions: cfg.Dimensions,
	}
}

// Store stores an embedding with metadata
func (m *MockVectorStore) Store(id string, embedding []float32, metadata map[string]any) error {
	if len(embedding) == 0 {
		return fmt.Errorf("empty embedding")
	}
	m.data[id] = MockVectorItem{
		Embedding: embedding,
		Metadata:  metadata,
	}
	return nil
}

// Query searches for similar embeddings
func (m *MockVectorStore) Query(embedding []float32, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	results := make([]SearchResult, 0, limit)
	for id, item := range m.data {
		score := cosineSimilarity(embedding, item.Embedding)
		results = append(results, SearchResult{
			ID:       id,
			Score:    score,
			Metadata: item.Metadata,
		})
	}
	return results[:min(limit, len(results))], nil
}

// Delete removes an embedding
func (m *MockVectorStore) Delete(id string) error {
	delete(m.data, id)
	return nil
}

// Size returns the number of stored embeddings
func (m *MockVectorStore) Size() int {
	return len(m.data)
}

// ScoredID is used for priority queue in approximate search
type ScoredID struct {
	ID    string
	Score float32
}

// PriorityQueue implements heap.Interface for finding top-k results
type PriorityQueue []ScoredID

func (pq PriorityQueue) Len() int           { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool { return pq[i].Score < pq[j].Score }
func (pq PriorityQueue) Swap(i, j int)      { pq[i], pq[j] = pq[j], pq[i] }

func (pq *PriorityQueue) Push(x interface{}) {
	*pq = append(*pq, x.(ScoredID))
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

// HNSWVectorStore is a vector store using HNSW (Hierarchical Navigable Small World) algorithm
type HNSWVectorStore struct {
	mu            sync.RWMutex
	embeddings    map[string][]float32
	metadata      map[string]map[string]any
	maxNeighbors  int
	entryPoint    string
	layers        map[int]map[string][]string // layer -> node -> neighbors
	currentLayer  int
	maxLayer      int
}

// HNSWConfig holds HNSW configuration
type HNSWConfig struct {
	MaxNeighbors int `toml:"max_neighbors"` // M parameter
	MaxLayer     int `toml:"max_layer"`     // Maximum layer depth
}

// NewHNSWVectorStore creates a new HNSW vector store
func NewHNSWVectorStore(maxNeighbors int) *HNSWVectorStore {
	if maxNeighbors <= 0 {
		maxNeighbors = 32
	}
	return &HNSWVectorStore{
		embeddings:   make(map[string][]float32),
		metadata:     make(map[string]map[string]any),
		maxNeighbors: maxNeighbors,
		layers:       make(map[int]map[string][]string),
		currentLayer: 0,
		maxLayer:     16, // Maximum 16 layers
	}
}

// NewHNSWVectorStoreWithConfig creates HNSW store with custom config
func NewHNSWVectorStoreWithConfig(cfg HNSWConfig) *HNSWVectorStore {
	if cfg.MaxNeighbors <= 0 {
		cfg.MaxNeighbors = 32
	}
	if cfg.MaxLayer <= 0 {
		cfg.MaxLayer = 16
	}
	return &HNSWVectorStore{
		embeddings:   make(map[string][]float32),
		metadata:     make(map[string]map[string]any),
		maxNeighbors: cfg.MaxNeighbors,
		layers:       make(map[int]map[string][]string),
		maxLayer:     cfg.MaxLayer,
	}
}

// randomLayer generates a random layer level using exponential distribution
func (s *HNSWVectorStore) randomLayer() int {
	// Simple approximation of exponential distribution
	// In production, use proper random number generation
	level := 0
	for level < s.maxLayer && float64(level+1) < float64(s.maxLayer)*0.5 {
		level++
	}
	return level
}

// Store stores an embedding with metadata
func (s *HNSWVectorStore) Store(id string, embedding []float32, metadata map[string]any) error {
	if len(embedding) == 0 {
		return fmt.Errorf("empty embedding")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Make a copy of the embedding
	embCopy := make([]float32, len(embedding))
	copy(embCopy, embedding)
	s.embeddings[id] = embCopy

	// Store metadata
	if metadata != nil {
		metaCopy := make(map[string]any)
		for k, v := range metadata {
			metaCopy[k] = v
		}
		s.metadata[id] = metaCopy
	}

	// Initialize layers if needed
	for l := 0; l <= s.maxLayer; l++ {
		if s.layers[l] == nil {
			s.layers[l] = make(map[string][]string)
		}
	}

	// Determine the maximum layer for this element
	elementLayer := s.randomLayer()
	if elementLayer > s.currentLayer {
		s.currentLayer = elementLayer
	}

	// If first element, set as entry point
	if s.entryPoint == "" {
		s.entryPoint = id
		// Add to all layers up to elementLayer
		for l := 0; l <= elementLayer; l++ {
			s.layers[l][id] = []string{}
		}
		return nil
	}

	// Insert element using HNSW algorithm
	s.insertElement(id, embedding, elementLayer)

	return nil
}

// insertElement inserts an element into the HNSW graph
func (s *HNSWVectorStore) insertElement(id string, embedding []float32, elementLayer int) {
	// Start from the highest layer
	for layer := s.currentLayer; layer >= 0; layer-- {
		var neighbors []string

		if layer > elementLayer {
			// Element doesn't exist at this layer, search only
			continue
		}

		// Find nearest neighbors at this layer
		neighbors = s.findNearestNeighborsAtLayer(embedding, s.maxNeighbors, layer)

		// Add element to this layer with its neighbors
		s.layers[layer][id] = neighbors

		// Update neighbors to point back to new element
		s.updateNeighborLinks(layer, id, neighbors)
	}
}

// findNearestNeighborsAtLayer finds nearest neighbors at a specific layer using greedy search
func (s *HNSWVectorStore) findNearestNeighborsAtLayer(query []float32, k int, layer int) []string {
	if s.entryPoint == "" {
		return []string{}
	}

	// Start from entry point at the highest available layer
	currentLayer := s.currentLayer
	if layer < currentLayer {
		currentLayer = layer
	}

	// Greedy search from entry point
	current := s.entryPoint
	visited := make(map[string]bool)
	visited[current] = true

	// Local search - find closest node at this layer
	for {
		changed := false
		currentScore := s.getSimilarity(query, current)

		// Check neighbors at current layer
		neighbors := s.layers[currentLayer][current]
		for _, neighbor := range neighbors {
			if visited[neighbor] {
				continue
			}
			visited[neighbor] = true

			neighborScore := s.getSimilarity(query, neighbor)
			if neighborScore > currentScore {
				current = neighbor
				currentScore = neighborScore
				changed = true
				break
			}
		}

		if !changed {
			break
		}
	}

	// Now collect k nearest neighbors from current position
	return s.collectKNearest(query, current, k, layer, visited)
}

// collectKNearest collects k nearest neighbors using beam search
func (s *HNSWVectorStore) collectKNearest(query []float32, start string, k int, layer int, visited map[string]bool) []string {
	type candidate struct {
		id    string
		score float32
	}

	// Use a priority queue approach
	candidates := []candidate{{id: start, score: s.getSimilarity(query, start)}}
	best := make([]candidate, 0, k)
	expanded := make(map[string]bool)

	for len(candidates) > 0 && len(best) < k*2 {
		// Get best candidate
		bestIdx := 0
		for i := 1; i < len(candidates); i++ {
			if candidates[i].score > candidates[bestIdx].score {
				bestIdx = i
			}
		}
		current := candidates[bestIdx]
		candidates = append(candidates[:bestIdx], candidates[bestIdx+1:]...)

		if expanded[current.id] {
			continue
		}
		expanded[current.id] = true
		best = append(best, current)

		// Expand neighbors
		neighbors := s.layers[layer][current.id]
		for _, neighbor := range neighbors {
			if visited[neighbor] || expanded[neighbor] {
				continue
			}
			score := s.getSimilarity(query, neighbor)
			candidates = append(candidates, candidate{id: neighbor, score: score})
		}
	}

	// Return top k
	result := make([]string, 0, k)
	for i := 0; i < len(best) && i < k; i++ {
		result = append(result, best[i].id)
	}
	return result
}

// updateNeighborLinks updates bidirectional links between nodes
func (s *HNSWVectorStore) updateNeighborLinks(layer int, newNode string, neighbors []string) {
	// For each neighbor, add back-link to new node
	for _, neighbor := range neighbors {
		existingLinks := s.layers[layer][neighbor]

		// Check if link already exists
		hasLink := false
		for _, link := range existingLinks {
			if link == newNode {
				hasLink = true
				break
			}
		}

		if !hasLink {
			// Add back-link if not exceeding max neighbors
			if len(existingLinks) < s.maxNeighbors {
				s.layers[layer][neighbor] = append(existingLinks, newNode)
			}
		}
	}
}

// getSimilarity gets similarity between query and a stored embedding
func (s *HNSWVectorStore) getSimilarity(query []float32, id string) float32 {
	emb, ok := s.embeddings[id]
	if !ok || len(emb) != len(query) {
		return 0
	}
	return cosineSimilarity(query, emb)
}

// Query searches for similar embeddings using HNSW
func (s *HNSWVectorStore) Query(embedding []float32, limit int) ([]SearchResult, error) {
	if len(embedding) == 0 {
		return nil, fmt.Errorf("empty query embedding")
	}

	if limit <= 0 {
		limit = 10
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.entryPoint == "" {
		return []SearchResult{}, nil
	}

	// Use HNSW search algorithm
	resultIDs := s.hnswSearch(embedding, limit)

	// Convert to SearchResult format
	results := make([]SearchResult, 0, len(resultIDs))
	for _, id := range resultIDs {
		score := s.getSimilarity(embedding, id)
		result := SearchResult{
			ID:    id,
			Score: score,
		}
		if meta, ok := s.metadata[id]; ok {
			result.Metadata = make(map[string]any)
			for k, v := range meta {
				result.Metadata[k] = v
			}
		}
		results = append(results, result)
	}

	return results, nil
}

// hnswSearch performs HNSW search to find approximate nearest neighbors
func (s *HNSWVectorStore) hnswSearch(query []float32, limit int) []string {
	// Start from the highest layer
	currentLayer := s.currentLayer

	// Entry point for search
	current := s.entryPoint

	// Greedy search at each layer
	for layer := currentLayer; layer >= 0; layer-- {
		// Search at this layer
		current = s.searchAtLayer(query, current, layer)
	}

	// Collect final neighbors from layer 0
	return s.collectFinalNeighbors(query, current, limit)
}

// searchAtLayer performs greedy search at a specific layer
func (s *HNSWVectorStore) searchAtLayer(query []float32, start string, layer int) string {
	current := s.entryPoint
	currentScore := s.getSimilarity(query, current)
	visited := make(map[string]bool)
	visited[current] = true

	// Greedy descent
	for {
		improved := false
		neighbors := s.layers[layer][current]

		for _, neighbor := range neighbors {
			if visited[neighbor] {
				continue
			}
			visited[neighbor] = true

			score := s.getSimilarity(query, neighbor)
			if score > currentScore {
				current = neighbor
				currentScore = score
				improved = true
				break
			}
		}

		if !improved {
			break
		}
	}

	return current
}

// collectFinalNeighbors collects the final k nearest neighbors
func (s *HNSWVectorStore) collectFinalNeighbors(query []float32, start string, k int) []string {
	// BFS/DFS from the found entry point to collect k neighbors
	visited := make(map[string]bool)
	result := make([]string, 0, k)

	// Use priority queue to track best candidates
	type scoredID struct {
		id    string
		score float32
	}
	candidates := make([]scoredID, 0)

	// Start with the found node and its neighbors
	visited[start] = true
	candidates = append(candidates, scoredID{id: start, score: s.getSimilarity(query, start)})

	// Add layer 0 neighbors
	for _, neighbor := range s.layers[0][start] {
		if !visited[neighbor] {
			visited[neighbor] = true
			candidates = append(candidates, scoredID{id: neighbor, score: s.getSimilarity(query, neighbor)})
		}
	}

	// Sort by score descending
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Return top k
	for i := 0; i < len(candidates) && i < k; i++ {
		result = append(result, candidates[i].id)
	}

	return result
}

// Delete removes an embedding from the store
func (s *HNSWVectorStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.embeddings, id)
	delete(s.metadata, id)
	for _, layer := range s.layers {
		delete(layer, id)
		// Remove from neighbors' lists
		for node, neighbors := range layer {
			newNeighbors := make([]string, 0, len(neighbors))
			for _, n := range neighbors {
				if n != id {
					newNeighbors = append(newNeighbors, n)
				}
			}
			layer[node] = newNeighbors
		}
	}
	return nil
}

// Size returns the number of stored embeddings
func (s *HNSWVectorStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.embeddings)
}

// Clear removes all embeddings
func (s *HNSWVectorStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embeddings = make(map[string][]float32)
	s.metadata = make(map[string]map[string]any)
	s.entryPoint = ""
	s.currentLayer = 0
	for l := range s.layers {
		s.layers[l] = make(map[string][]string)
	}
}
