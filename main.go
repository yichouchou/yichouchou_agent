package main

import (
	"github.com/yichouchou/yichouchou_agent/backend"
	"github.com/yichouchou/yichouchou_agent/pkg"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	log.Println("========================================")
	log.Println("Server starting...")

	if err := pkg.InitKnowledgeBase(); err != nil {
		log.Printf("[WARNING] Failed to initialize knowledge base: %v", err)
	}

	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
		log.Fatal("MINIMAX_API_KEY is not set")
	}

	baseURL := os.Getenv("MINIMAX_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.minimax.chat/v1"
	}

	pkg.LlmConfig = pkg.LLMConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   "MiniMax-M2.7",
	}

	log.Printf("[INFO] LLM Config initialized")
	log.Printf("[INFO] Model: %s", pkg.LlmConfig.Model)

	fs := http.FileServer(http.Dir("./frontend"))
	http.Handle("/", fs)
	http.HandleFunc("/api/chat", backend.ChatHandler)

	log.Println("[INFO] Server starting on :8080")
	log.Println("========================================")
	httpServer := &http.Server{
		Addr:         ":8080",
		Handler:      http.DefaultServeMux,
		ReadTimeout:  180 * time.Second,
		WriteTimeout: 180 * time.Second,
	}
	log.Fatal(httpServer.ListenAndServe())
}
