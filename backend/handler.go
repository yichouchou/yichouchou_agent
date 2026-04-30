package backend

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/yichouchou/yichouchou_agent/pkg"
)

type ChatRequest struct {
	Message string `json:"message"`
}

type ChatResponse struct {
	Response string `json:"response"`
}

var hybridRAG *pkg.HybridRAG
var llmClient *pkg.LLMClient

func InitHandler(hybrid *pkg.HybridRAG, llm *pkg.LLMClient) {
	hybridRAG = hybrid
	llmClient = llm
}

func ChatHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("[INFO] Received %s request from %s", r.Method, r.RemoteAddr)

	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

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

	ctx := context.Background()

	answer, err := hybridRAG.Query(ctx, req.Message)
	if err != nil {
		log.Printf("[ERROR] RAG query failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[RESPONSE] Answer: %s", answer)

	chatResp := ChatResponse{Response: answer}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chatResp)
}
