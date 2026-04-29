package backend

import (
	"encoding/json"
	"fmt"
	"github.com/yichouchou/yichouchou_agent/pkg"
	"log"
	"net/http"
)

type ChatRequest struct {
	Message string `json:"message"`
}

type ChatResponse struct {
	Response string `json:"response"`
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

	context := pkg.SearchKnowledge(req.Message)
	log.Printf("[INFO] Retrieved context length: %d", len(context))

	prompt := req.Message
	if context != "" {
		prompt = fmt.Sprintf("请根据以下知识库内容回答问题。\n\n知识库内容:\n%s\n\n用户问题: %s", context, req.Message)
	}

	answer, err := pkg.CallMiniMaxAPI(prompt, pkg.LlmConfig)
	if err != nil {
		log.Printf("[ERROR] LLM call failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[RESPONSE] Answer: %s", answer)

	chatResp := ChatResponse{Response: answer}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chatResp)
}
