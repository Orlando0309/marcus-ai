package codeintel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// SemanticIndex indexes code files using semantic embeddings
type SemanticIndex struct {
	baseDir     string
	provider    EmbeddingProvider
	vectorStore VectorStore
	cache       *EmbeddingCache
	mu          sync.RWMutex
	indexed     map[string]FileEmbedding // path -> embedding info
}

// FileEmbedding stores embedding info for a file
type FileEmbedding struct {
	Path      string            `json:"path"`
	Checksum  string            `json:"checksum"`
	Embedding []float32         `json:"embedding,omitempty"`
	Chunks    []ChunkEmbedding  `json:"chunks,omitempty"`
	Language  string            `json:"language"`
	UpdatedAt int64             `json:"updated_at"`
}

// ChunkEmbedding stores embedding for a code chunk
type ChunkEmbedding struct {
	Content   string    `json:"content"`
	StartLine int       `json:"start_line"`
	EndLine   int       `json:"end_line"`
	Embedding []float32 `json:"embedding,omitempty"`
}

// SemanticSearchResult is a result from semantic search
type SemanticSearchResult struct {
	Path       string  `json:"path"`
	Score      float32 `json:"score"`
	Content    string  `json:"content,omitempty"`
	StartLine  int     `json:"start_line,omitempty"`
	EndLine    int     `json:"end_line,omitempty"`
	Language   string  `json:"language"`
}

// NewSemanticIndex creates a new semantic index
func NewSemanticIndex(baseDir string, provider EmbeddingProvider, store VectorStore) *SemanticIndex {
	if store == nil {
		store = NewInMemoryVectorStore()
	}
	return &SemanticIndex{
		baseDir:     baseDir,
		provider:    provider,
		vectorStore: store,
		cache:       NewEmbeddingCache(),
		indexed:     make(map[string]FileEmbedding),
	}
}

// Build indexes all files in the repository
func (s *SemanticIndex) Build(ctx context.Context, paths []string) error {
	if s.provider == nil {
		return fmt.Errorf("no embedding provider configured")
	}

	// If no paths provided, scan the base directory
	if len(paths) == 0 {
		var err error
		paths, err = s.scanDirectory()
		if err != nil {
			return fmt.Errorf("scan directory: %w", err)
		}
	}

	// Index files in batches
	batchSize := 10
	for i := 0; i < len(paths); i += batchSize {
		end := i + batchSize
		if end > len(paths) {
			end = len(paths)
		}

		if err := s.indexBatch(ctx, paths[i:end]); err != nil {
			return fmt.Errorf("index batch: %w", err)
		}
	}

	return nil
}

// scanDirectory finds all indexable files
func (s *SemanticIndex) scanDirectory() ([]string, error) {
	var paths []string

	err := filepath.Walk(s.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		if info.IsDir() {
			// Skip hidden directories and common non-source directories
			name := filepath.Base(path)
			if strings.HasPrefix(name, ".") ||
				name == "node_modules" ||
				name == "vendor" ||
				name == "dist" ||
				name == "build" ||
				name == "target" ||
				name == "__pycache__" ||
				name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden files and non-source files
		name := filepath.Base(path)
		if strings.HasPrefix(name, ".") {
			return nil
		}

		// Only index source code files
		if !isSourceFile(path) {
			return nil
		}

		paths = append(paths, path)
		return nil
	})

	return paths, err
}

// isSourceFile checks if a file is a source code file
func isSourceFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go", ".py", ".js", ".ts", ".jsx", ".tsx", ".rs", ".c", ".cpp", ".h", ".hpp",
		".java", ".kt", ".scala", ".rb", ".php", ".cs", ".swift", ".m", ".mm",
		".md", ".markdown", ".txt", ".json", ".yaml", ".yml", ".toml":
		return true
	}
	return false
}

// indexBatch indexes a batch of files
func (s *SemanticIndex) indexBatch(ctx context.Context, paths []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Prepare texts for embedding
	texts := make([]string, 0, len(paths))
	fileInfos := make([]*FileInfo, 0, len(paths))

	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			continue // Skip files we can't read
		}

		// Skip binary files
		if isBinary(content) {
			continue
		}

		// Check if file has changed
		checksum := sha256.Sum256(content)
		checksumStr := hex.EncodeToString(checksum[:])

		relPath := s.relPath(path)
		if existing, ok := s.indexed[relPath]; ok && existing.Checksum == checksumStr {
			continue // Skip unchanged files
		}

		text := string(content)
		if len(text) > 8000 {
			text = text[:8000] // Limit size for embedding
		}

		texts = append(texts, text)
		fileInfos = append(fileInfos, &FileInfo{
			Path:     relPath,
			Content:  text,
			Checksum: checksumStr,
			Language: detectLanguageFromPath(path),
		})
	}

	if len(texts) == 0 {
		return nil
	}

	// Generate embeddings
	embeddings, err := s.provider.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("generate embeddings: %w", err)
	}

	// Store embeddings
	for i, info := range fileInfos {
		fileEmb := FileEmbedding{
			Path:      info.Path,
			Checksum:  info.Checksum,
			Embedding: embeddings[i],
			Language:  info.Language,
		}

		s.indexed[info.Path] = fileEmb

		// Store in vector store
		metadata := map[string]any{
			"path":     info.Path,
			"language": info.Language,
			"type":     "file",
		}
		s.vectorStore.Store(info.Path, embeddings[i], metadata)

		// Cache the embedding
		s.cache.Set(info.Path, embeddings[i])
	}

	return nil
}

// FileInfo holds file information
type FileInfo struct {
	Path     string
	Content  string
	Checksum string
	Language string
}

// relPath returns the path relative to baseDir
func (s *SemanticIndex) relPath(path string) string {
	rel, _ := filepath.Rel(s.baseDir, path)
	return rel
}

// isBinary checks if content appears to be binary
func isBinary(data []byte) bool {
	// Check for null bytes (common in binary files)
	for _, b := range data[:min(len(data), 1024)] {
		if b == 0 {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// detectLanguageFromPath detects the programming language from file extension
func detectLanguageFromPath(path string) string {
ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".jsx":
		return "jsx"
	case ".tsx":
		return "tsx"
	case ".rs":
		return "rust"
	case ".c":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".h", ".hpp":
		return "c++"
	case ".java":
		return "java"
	case ".kt":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".cs":
		return "csharp"
	case ".swift":
		return "swift"
	case ".m", ".mm":
		return "objective-c"
	case ".md", ".markdown":
		return "markdown"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	default:
		return "unknown"
	}
}

// Search performs a semantic search
func (s *SemanticIndex) Search(ctx context.Context, query string, limit int) ([]SemanticSearchResult, error) {
	if s.provider == nil {
		return nil, fmt.Errorf("no embedding provider configured")
	}

	// Generate embedding for query
	embeddings, err := s.provider.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("generate query embedding: %w", err)
	}

	// Search vector store
	results, err := s.vectorStore.Query(embeddings[0], limit)
	if err != nil {
		return nil, fmt.Errorf("query vector store: %w", err)
	}

	// Convert to semantic search results
	searchResults := make([]SemanticSearchResult, 0, len(results))
	for _, result := range results {
		if meta, ok := result.Metadata["path"]; ok {
			path := meta.(string)
			lang := ""
			if l, ok := result.Metadata["language"]; ok {
				lang = l.(string)
			}
			searchResults = append(searchResults, SemanticSearchResult{
				Path:     path,
				Score:    result.Score,
				Language: lang,
			})
		}
	}

	return searchResults, nil
}

// SearchWithFilter performs a semantic search with language filter
func (s *SemanticIndex) SearchWithFilter(ctx context.Context, query string, languages []string, limit int) ([]SemanticSearchResult, error) {
	results, err := s.Search(ctx, query, limit*3) // Get more results for filtering
	if err != nil {
		return nil, err
	}

	if len(languages) == 0 {
		if len(results) > limit {
			return results[:limit], nil
		}
		return results, nil
	}

	// Filter by language
	filtered := make([]SemanticSearchResult, 0, limit)
	langSet := make(map[string]bool)
	for _, l := range languages {
		langSet[strings.ToLower(l)] = true
	}

	for _, result := range results {
		if langSet[strings.ToLower(result.Language)] {
			filtered = append(filtered, result)
			if len(filtered) >= limit {
				break
			}
		}
	}

	return filtered, nil
}

// Refresh updates the index for changed files
func (s *SemanticIndex) Refresh(ctx context.Context, paths []string) error {
	return s.Build(ctx, paths)
}

// Size returns the number of indexed files
func (s *SemanticIndex) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.indexed)
}

// Clear clears the index
func (s *SemanticIndex) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.indexed = make(map[string]FileEmbedding)
	s.vectorStore = NewInMemoryVectorStore()
	s.cache.Clear()
}

// Save persists the index to disk
func (s *SemanticIndex) Save(path string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data := struct {
		Files []FileEmbedding `json:"files"`
	}{
		Files: make([]FileEmbedding, 0, len(s.indexed)),
	}

	for _, file := range s.indexed {
		data.Files = append(data.Files, file)
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	if err := os.WriteFile(path, jsonData, 0644); err != nil {
		return fmt.Errorf("write index file: %w", err)
	}

	return nil
}

// Load loads the index from disk
func (s *SemanticIndex) Load(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read index file: %w", err)
	}

	var saved struct {
		Files []FileEmbedding `json:"files"`
	}

	if err := json.Unmarshal(data, &saved); err != nil {
		return fmt.Errorf("unmarshal index: %w", err)
	}

	// Clear existing index
	s.indexed = make(map[string]FileEmbedding)
	s.vectorStore = NewInMemoryVectorStore()

	// Restore embeddings
	for _, file := range saved.Files {
		s.indexed[file.Path] = file
		metadata := map[string]any{
			"path":     file.Path,
			"language": file.Language,
			"type":     "file",
		}
		s.vectorStore.Store(file.Path, file.Embedding, metadata)
		s.cache.Set(file.Path, file.Embedding)
	}

	return nil
}

// GetIndexedFiles returns a list of indexed file paths
func (s *SemanticIndex) GetIndexedFiles() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	files := make([]string, 0, len(s.indexed))
	for path := range s.indexed {
		files = append(files, path)
	}
	return files
}
