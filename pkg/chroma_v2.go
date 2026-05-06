package pkg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/yichouchou/yichouchou_agent/conf"
	"github.com/tmc/langchaingo/schema"
)

// ChromaStoreV2 uses Chroma REST API directly with MiniMax embeddings
type ChromaStoreV2 struct {
	host       string
	port       int
	collection string
	embedder   *MiniMaxEmbedder
}

// NewChromaStoreV2 creates a new Chroma store using direct REST API
func NewChromaStoreV2(host string, port int, collection string) *ChromaStoreV2 {
	return &ChromaStoreV2{
		host:       host,
		port:       port,
		collection: collection,
	}
}

// Connect initializes the embedder
func (c *ChromaStoreV2) Connect(apiKey string) error {
	embedder, err := NewMiniMaxEmbedder(apiKey)
	if err != nil {
		return fmt.Errorf("failed to create embedder: %w", err)
	}
	c.embedder = embedder
	return nil
}

func (c *ChromaStoreV2) baseURL() string {
	return fmt.Sprintf("http://%s:%d", c.host, c.port)
}

// AddDocuments adds documents to Chroma with embeddings
func (c *ChromaStoreV2) AddDocuments(ctx context.Context, docs []schema.Document) error {
	if c.embedder == nil {
		return fmt.Errorf("embedder not initialized, call Connect first")
	}

	if len(docs) == 0 {
		return nil
	}

	// Extract texts for embedding
	texts := make([]string, len(docs))
	for i, doc := range docs {
		texts[i] = doc.PageContent
	}

	// Get embeddings from MiniMax
	embeddings, err := c.embedder.EmbedTexts(texts)
	if err != nil {
		return fmt.Errorf("failed to get embeddings: %w", err)
	}

	// Prepare Chroma request
	ids := make([]string, len(docs))
	documents := texts
	metadatas := make([]map[string]interface{}, len(docs))

	for i, doc := range docs {
		ids[i] = fmt.Sprintf("doc_%d", i)
		metadatas[i] = doc.Metadata
	}

	addRequest := map[string]interface{}{
		"ids":       ids,
		"documents": documents,
		"embeddings": embeddings,
		"metadatas": metadatas,
	}

	url := fmt.Sprintf("%s/api/v1/collections/%s/add", c.baseURL(), c.collection)
	
	body, err := json.Marshal(addRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Chroma returned status %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("[INFO] Successfully added %d documents to Chroma", len(docs))
	return nil
}

// Query searches for similar documents
func (c *ChromaStoreV2) Query(ctx context.Context, queryText string, n int) ([]schema.Document, error) {
	if c.embedder == nil {
		return nil, fmt.Errorf("embedder not initialized")
	}

	// Get query embedding
	embeddings, err := c.embedder.EmbedTexts([]string{queryText})
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/collections/%s/query", c.baseURL(), c.collection)
	
	queryRequest := map[string]interface{}{
		"query_embeddings": embeddings,
		"n_results":        n,
		"include":          []string{"documents", "metadatas", "distances"},
	}

	body, err := json.Marshal(queryRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send query: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Documents [][]string                   `json:"documents"`
		Metadatas [][]map[string]interface{}   `json:"metadatas"`
		Distances [][]float64                  `json:"distances"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	docs := make([]schema.Document, 0)
	for i, docList := range result.Documents {
		for j, doc := range docList {
			meta := map[string]interface{}{}
			if i < len(result.Metadatas) && j < len(result.Metadatas[i]) {
				meta = result.Metadatas[i][j]
			}
			docs = append(docs, schema.Document{
				PageContent: doc,
				Metadata:    meta,
			})
		}
	}

	return docs, nil
}

// Search is a helper for similarity search
func (c *ChromaStoreV2) Search(query string, limit int) string {
	docs, err := c.Query(context.Background(), query, limit)
	if err != nil {
		log.Printf("[ERROR] Chroma search failed: %v", err)
		return ""
	}

	if len(docs) == 0 {
		return ""
	}

	var results []string
	for _, doc := range docs {
		title := ""
		if t, ok := doc.Metadata["title"].(string); ok {
			title = t
		}
		results = append(results, fmt.Sprintf("【%s】\n%s", title, doc.PageContent))
	}

	return strings.Join(results, "\n\n---\n\n")
}

// InitChromaStoreV2 creates a ChromaStoreV2 and connects it
func InitChromaStoreV2(apiKey string) (*ChromaStoreV2, error) {
	cfg := conf.GetChromaConfig()
	if cfg == nil {
		return nil, fmt.Errorf("Chroma config not found")
	}

	store := NewChromaStoreV2(cfg.Host, cfg.Port, cfg.Collection)
	if err := store.Connect(apiKey); err != nil {
		return nil, fmt.Errorf("failed to connect to Chroma: %w", err)
	}

	return store, nil
}