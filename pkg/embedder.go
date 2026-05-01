package pkg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/yichouchou/yichouchou_agent/conf"
)

// MiniMaxEmbedder creates embeddings using MiniMax API directly
type MiniMaxEmbedder struct {
	apiKey  string
	baseURL string
	model   string
	dim     int
}

// NewMiniMaxEmbedder creates a new MiniMax embedder
func NewMiniMaxEmbedder(apiKey string) (*MiniMaxEmbedder, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	baseURL := "https://api.minimaxi.chat/v1"
	model := "emb"

	return &MiniMaxEmbedder{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		dim:     1024, // MiniMax embedding dimension
	}, nil
}

// EmbedTexts generates embeddings for multiple texts
func (e *MiniMaxEmbedder) EmbedTexts(texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	url := fmt.Sprintf("%s/text/embeddings", e.baseURL)

	reqBody := map[string]interface{}{
		"model": e.model,
		"input": texts,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	embeddings := make([][]float64, len(result.Data))
	for i, d := range result.Data {
		embeddings[i] = d.Embedding
	}

	return embeddings, nil
}

// Dimension returns the embedding dimension
func (e *MiniMaxEmbedder) Dimension() int {
	return e.dim
}

// CreateMiniMaxEmbedder is a helper to create embedder from config
func CreateMiniMaxEmbedder() (*MiniMaxEmbedder, error) {
	apiKey := conf.GetMinimaxAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("MiniMax API key not found")
	}
	return NewMiniMaxEmbedder(apiKey)
}