package codeintel

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

// PgVectorStore is a vector store using PostgreSQL with pgvector extension
type PgVectorStore struct {
	db           *sql.DB
	tableName    string
	dimensions   int
	mu           sync.RWMutex
	maxRetries   int
}

// PgVectorConfig holds PostgreSQL/pgvector configuration
type PgVectorConfig struct {
	DSN         string `toml:"dsn"`  // PostgreSQL connection string
	TableName   string `toml:"table_name"`
	Dimensions  int    `toml:"dimensions"`
	MaxRetries  int    `toml:"max_retries,omitempty"`
}

// NewPgVectorStore creates a new pgvector store
func NewPgVectorStore(cfg PgVectorConfig) (*PgVectorStore, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("DSN is required for pgvector store")
	}
	if cfg.TableName == "" {
		cfg.TableName = "embeddings"
	}
	if cfg.Dimensions <= 0 {
		cfg.Dimensions = 1536 // Default for text-embedding-3-small
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}

	db, err := sql.Open("postgres", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	store := &PgVectorStore{
		db:         db,
		tableName:  cfg.TableName,
		dimensions: cfg.Dimensions,
		maxRetries: cfg.MaxRetries,
	}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the necessary tables and indexes
func (p *PgVectorStore) initSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Enable pgvector extension
	_, err := p.db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		return fmt.Errorf("enable vector extension: %w", err)
	}

	// Create embeddings table
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			embedding vector(%d),
			metadata JSONB,
			document TEXT,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)
	`, p.tableName, p.dimensions)

	_, err = p.db.ExecContext(ctx, createTableSQL)
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	// Create index for cosine similarity search
	indexSQL := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS %s_embedding_idx ON %s
		USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100)
	`, p.tableName+"_idx", p.tableName)

	_, err = p.db.ExecContext(ctx, indexSQL)
	if err != nil {
		// Index creation might fail if table is empty, that's okay
		// The index will be created when data is inserted
		if !strings.Contains(err.Error(), "cannot create index") {
			return fmt.Errorf("create index: %w", err)
		}
	}

	return nil
}

// Store stores an embedding with metadata
func (p *PgVectorStore) Store(id string, embedding []float32, metadata map[string]any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return p.storeWithRetry(ctx, id, embedding, metadata)
}

// storeWithRetry stores with retries
func (p *PgVectorStore) storeWithRetry(ctx context.Context, id string, embedding []float32, metadata map[string]any) error {
	var lastErr error
	for attempt := 0; attempt < p.maxRetries; attempt++ {
		if err := p.store(ctx, id, embedding, metadata); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}
	return lastErr
}

// store stores a single embedding
func (p *PgVectorStore) store(ctx context.Context, id string, embedding []float32, metadata map[string]any) error {
	// Convert embedding to string format for pgvector
	embeddingStr := p.embeddingToString(embedding)

	// Convert metadata to JSON
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	// Upsert query (INSERT ... ON CONFLICT UPDATE)
	query := fmt.Sprintf(`
		INSERT INTO %s (id, embedding, metadata, updated_at)
		VALUES ($1, $2::vector, $3::jsonb, NOW())
		ON CONFLICT (id) DO UPDATE SET
			embedding = EXCLUDED.embedding,
			metadata = EXCLUDED.metadata,
			updated_at = NOW()
	`, p.tableName)

	_, err = p.db.ExecContext(ctx, query, id, embeddingStr, metadataJSON)
	if err != nil {
		return fmt.Errorf("execute upsert: %w", err)
	}

	return nil
}

// embeddingToString converts a float32 slice to pgvector string format
func (p *PgVectorStore) embeddingToString(embedding []float32) string {
	var sb strings.Builder
	sb.WriteString("[")
	for i, v := range embedding {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%.6f", v))
	}
	sb.WriteString("]")
	return sb.String()
}

// Query searches for similar embeddings
func (p *PgVectorStore) Query(embedding []float32, limit int) ([]SearchResult, error) {
	if len(embedding) == 0 {
		return nil, fmt.Errorf("empty query embedding")
	}

	if limit <= 0 {
		limit = 10
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	embeddingStr := p.embeddingToString(embedding)

	// Query using cosine similarity (1 - cosine_distance gives cosine similarity)
	query := fmt.Sprintf(`
		SELECT id, 1 - (embedding <=> $1::vector) AS similarity, metadata
		FROM %s
		ORDER BY embedding <=> $1::vector
		LIMIT $2
	`, p.tableName)

	rows, err := p.db.QueryContext(ctx, query, embeddingStr, limit)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var id string
		var score float32
		var metadataJSON []byte

		if err := rows.Scan(&id, &score, &metadataJSON); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		var metadata map[string]any
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
				return nil, fmt.Errorf("unmarshal metadata: %w", err)
			}
		}

		results = append(results, SearchResult{
			ID:       id,
			Score:    score,
			Metadata: metadata,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return results, nil
}

// Delete removes an embedding
func (p *PgVectorStore) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", p.tableName)
	_, err := p.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("execute delete: %w", err)
	}

	return nil
}

// Size returns the number of stored embeddings
func (p *PgVectorStore) Size() int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", p.tableName)
	err := p.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0
	}
	return count
}

// Close closes the database connection
func (p *PgVectorStore) Close() error {
	return p.db.Close()
}

// BatchStore stores multiple embeddings efficiently
func (p *PgVectorStore) BatchStore(embeddings []BatchEmbedding) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if len(embeddings) == 0 {
		return nil
	}

	// Start transaction
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := fmt.Sprintf(`
		INSERT INTO %s (id, embedding, metadata, document, created_at, updated_at)
		VALUES %s
		ON CONFLICT (id) DO UPDATE SET
			embedding = EXCLUDED.embedding,
			metadata = EXCLUDED.metadata,
			document = EXCLUDED.document,
			updated_at = NOW()
	`, p.tableName, p.placeholderValues(len(embeddings)))

	// Build value args
	args := make([]interface{}, 0, len(embeddings)*3)
	for _, emb := range embeddings {
		args = append(args, emb.ID)
		args = append(args, p.embeddingToString(emb.Embedding))

		metadataJSON, _ := json.Marshal(emb.Metadata)
		args = append(args, metadataJSON)
	}

	_, err = tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("execute batch insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// BatchEmbedding represents a batch of embeddings to store
type BatchEmbedding struct {
	ID        string
	Embedding []float32
	Metadata  map[string]any
	Document  string
}

// placeholderValues generates SQL placeholder values for batch insert
func (p *PgVectorStore) placeholderValues(count int) string {
	var sb strings.Builder
	for i := 0; i < count; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("($%d, $%d::vector, $%d::jsonb, $%d, NOW(), NOW())",
			i*4+1, i*4+2, i*4+3, i*4+4))
	}
	return sb.String()
}

// QueryWithFilter searches with metadata filters
func (p *PgVectorStore) QueryWithFilter(embedding []float32, limit int, filters map[string]any) ([]SearchResult, error) {
	if len(embedding) == 0 {
		return nil, fmt.Errorf("empty query embedding")
	}

	if limit <= 0 {
		limit = 10
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	embeddingStr := p.embeddingToString(embedding)

	// Build filter conditions
	whereClause := ""
	args := []interface{}{embeddingStr, limit}
	argIndex := 3

	for key, value := range filters {
		if whereClause == "" {
			whereClause = " WHERE "
		} else {
			whereClause += " AND "
		}

		// Handle different value types
		switch v := value.(type) {
		case string:
			whereClause += fmt.Sprintf("metadata->>'%s' = $%d", key, argIndex)
			args = append(args, v)
		case int, int64:
			whereClause += fmt.Sprintf("(metadata->>'%s')::int = $%d", key, argIndex)
			args = append(args, value)
		case bool:
			whereClause += fmt.Sprintf("(metadata->>'%s')::boolean = $%d", key, argIndex)
			args = append(args, v)
		default:
			whereClause += fmt.Sprintf("metadata->>'%s' = $%d", key, argIndex)
			args = append(args, fmt.Sprintf("%v", value))
		}
		argIndex++
	}

	query := fmt.Sprintf(`
		SELECT id, 1 - (embedding <=> $1::vector) AS similarity, metadata
		FROM %s%s
		ORDER BY embedding <=> $1::vector
		LIMIT $2
	`, p.tableName, whereClause)

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("execute filtered query: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var id string
		var score float32
		var metadataJSON []byte

		if err := rows.Scan(&id, &score, &metadataJSON); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		var metadata map[string]any
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
				return nil, fmt.Errorf("unmarshal metadata: %w", err)
			}
		}

		results = append(results, SearchResult{
			ID:       id,
			Score:    score,
			Metadata: metadata,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return results, nil
}

// HealthCheck checks database connectivity
func (p *PgVectorStore) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return p.db.PingContext(ctx)
}

// GetTableName returns the table name
func (p *PgVectorStore) GetTableName() string {
	return p.tableName
}
