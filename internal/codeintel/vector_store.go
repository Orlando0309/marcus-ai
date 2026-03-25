package codeintel

import (
	"fmt"
	"math"
	"sync"
)

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
// This is a simplified implementation - a production version would use a library like hnsw-go
type HNSWVectorStore struct {
	mu            sync.RWMutex
	embeddings    map[string][]float32
	metadata      map[string]map[string]any
	maxNeighbors  int
	entryPoint    string
	layers        map[int]map[string][]string // layer -> node -> neighbors
	currentLayer  int
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
	}
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

	// Update HNSW structure (simplified - add to layer 0)
	if s.entryPoint == "" {
		s.entryPoint = id
	}

	// Add to layer 0
	if s.layers[0] == nil {
		s.layers[0] = make(map[string][]string)
	}

	// Find nearest neighbors for the new node (simplified - check all)
	neighbors := s.findNearestNeighbors(embedding, s.maxNeighbors)
	s.layers[0][id] = neighbors

	return nil
}

// findNearestNeighbors finds the k nearest neighbors for an embedding (brute force for simplicity)
func (s *HNSWVectorStore) findNearestNeighbors(embedding []float32, k int) []string {
	type scored struct {
		id    string
		score float32
	}

	scores := make([]scored, 0, len(s.embeddings))
	for id, emb := range s.embeddings {
		if len(emb) == len(embedding) {
			scores = append(scores, scored{id: id, score: cosineSimilarity(embedding, emb)})
		}
	}

	// Sort by score descending
	for i := 0; i < len(scores) && i < k; i++ {
		maxIdx := i
		for j := i + 1; j < len(scores); j++ {
			if scores[j].score > scores[maxIdx].score {
				maxIdx = j
			}
		}
		scores[i], scores[maxIdx] = scores[maxIdx], scores[i]
	}

	result := make([]string, 0, k)
	for i := 0; i < len(scores) && i < k; i++ {
		result = append(result, scores[i].id)
	}

	return result
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

	// For simplicity, use brute force search for now
	// A full HNSW implementation would traverse the hierarchical graph
	return s.bruteForceSearch(embedding, limit)
}

// bruteForceSearch performs a brute force search
func (s *HNSWVectorStore) bruteForceSearch(embedding []float32, limit int) ([]SearchResult, error) {
	type scoredResult struct {
		id    string
		score float32
	}

	results := make([]scoredResult, 0, len(s.embeddings))
	for id, emb := range s.embeddings {
		if len(emb) != len(embedding) {
			continue
		}
		score := cosineSimilarity(embedding, emb)
		results = append(results, scoredResult{id: id, score: score})
	}

	// Sort by score descending
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
