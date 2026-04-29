package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

type ChatRequest struct {
	Message string `json:"message"`
}

type ChatResponse struct {
	Response string `json:"response"`
}

type LLMConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

var llmConfig LLMConfig

func chatHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[INFO] Received %s request from %s", r.Method, r.RemoteAddr)

	if r.Method != http.MethodPost {
		log.Printf("[WARNING] Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[ERROR] Failed to decode request body: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	log.Printf("[REQUEST] Message: %s", req.Message)

	answer, err := callMiniMaxAPI(req.Message, llmConfig)
	if err != nil {
		log.Printf("[ERROR] LLM call failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[RESPONSE] Answer: %s", answer)

	chatResp := ChatResponse{Response: answer}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(chatResp); err != nil {
		log.Printf("[ERROR] Failed to encode response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("[INFO] Request completed successfully")
}

func callMiniMaxAPI(message string, config LLMConfig) (string, error) {
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
	log.Printf("[DEBUG] Request: %s", string(jsonBody))

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
	log.Printf("[DEBUG] Response: %s", string(body))

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

func main() {
	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
	}

	if apiKey == "" {
		log.Fatal("MINIMAX_API_KEY is not set")
	}

	baseURL := os.Getenv("MINIMAX_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.minimax.chat/v1"
	}

	llmConfig = LLMConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   "MiniMax-M2.5",
	}

	log.Printf("[INFO] LLM Config initialized")
	log.Printf("[INFO] API Base URL: %s", llmConfig.BaseURL)
	log.Printf("[INFO] Model: %s", llmConfig.Model)

	fs := http.FileServer(http.Dir("./frontend"))
	http.Handle("/", fs)
	http.HandleFunc("/api/chat", chatHandler)

	fmt.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
