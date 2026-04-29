package pkg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

type LLMConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

var LlmConfig LLMConfig

func CallMiniMaxAPI(message string, config LLMConfig) (string, error) {
	type Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	type MiniMaxRequest struct {
		Model       string    `json:"model"`
		Messages    []Message `json:"messages"`
		Temperature float64   `json:"temperature"`
	}

	type Choice struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	type MiniMaxResponse struct {
		Choices []Choice `json:"choices"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	apiURL := fmt.Sprintf("%s/text/chatcompletion_v2", strings.TrimSuffix(config.BaseURL, "/"))

	reqBody := MiniMaxRequest{
		Model: config.Model,
		Messages: []Message{
			{Role: "user", Content: message},
		},
		Temperature: 0.7,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Printf("[DEBUG] Calling MiniMax API: %s", apiURL)

	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+config.APIKey)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("[DEBUG] Response status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var miniResp MiniMaxResponse
	if err := json.Unmarshal(body, &miniResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if miniResp.Error != nil {
		return "", fmt.Errorf("MiniMax API error: %s", miniResp.Error.Message)
	}

	if len(miniResp.Choices) == 0 {
		return "", fmt.Errorf("empty response from API")
	}

	return miniResp.Choices[0].Message.Content, nil
}
