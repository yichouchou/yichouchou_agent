package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

type ChatRequest struct {
	Message string `json:"message"`
}

type ChatResponse struct {
	Response string `json:"response"`
}

var llm llms.LLM

func chatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	answer, err := llm.Call(ctx, req.Message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := ChatResponse{Response: answer}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
		log.Fatal("MINIMAX_API_KEY is not set")
	}

	baseURL := os.Getenv("MINIMAX_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.minimax.chat/v1"
	}

	var err error
	llm, err = openai.New(
		openai.WithToken(apiKey),
		openai.WithBaseURL(baseURL+"/text/"),
		openai.WithModel("MiniMax-M2.5"),
	)
	if err != nil {
		log.Fatalf("Failed to create LLM: %v", err)
	}

	fs := http.FileServer(http.Dir("./frontend"))
	http.Handle("/", fs)
	http.HandleFunc("/api/chat", chatHandler)

	fmt.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}