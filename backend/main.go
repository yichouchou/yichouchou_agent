package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

type ChatRequest struct {
	Message string `json:"message"`
}

type ChatResponse struct {
	Response string `json:"response"`
}

var (
	llm    llms.LLM
	logger *log.Logger
	file   *os.File
)

func initLog() {
	// Create logs directory
	if err := os.MkdirAll("logs", 0755); err != nil {
		log.Fatalf("Failed to create logs directory: %v", err)
	}

	// Open log file
	f, err := os.OpenFile("logs/app.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	file = f

	// Multi-writer: both console and file
	writer := io.MultiWriter(os.Stdout, file)
	logger = log.New(writer, "", log.LstdFlags|log.Lshortfile)
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Printf("[ERROR] Failed to decode request: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Log LLM input
	logger.Printf("[INPUT] Message: %s", req.Message)

	ctx := context.Background()
	answer, err := llm.Call(ctx, req.Message)
	if err != nil {
		logger.Printf("[ERROR] LLM call failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log LLM output
	logger.Printf("[OUTPUT] Response: %s", answer)

	resp := ChatResponse{Response: answer}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	// Initialize logging
	initLog()
	defer file.Close()

	logger.Println("========================================")
	logger.Println("Server starting...")

	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
		logger.Fatal("[ERROR] MINIMAX_API_KEY is not set")
	}

	baseURL := os.Getenv("MINIMAX_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.minimax.chat/v1"
	}

	var err error
	llm, err = openai.New(
		openai.WithToken(apiKey),
		openai.WithBaseURL(baseURL),
		openai.WithModel("MiniMax-M2.7"),
	)
	if err != nil {
		logger.Fatalf("[ERROR] Failed to create LLM: %v", err)
	}

	logger.Printf("[INFO] LLM initialized with model: MiniMax-M2.7")

	fs := http.FileServer(http.Dir("./frontend"))
	http.Handle("/", fs)
	http.HandleFunc("/api/chat", chatHandler)

	logger.Println("[INFO] Server starting on :8080")
	logger.Println("========================================")

	httpServer := &http.Server{
		Addr:         ":8080",
		Handler:      http.DefaultServeMux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	logger.Fatal(httpServer.ListenAndServe().Error())
}