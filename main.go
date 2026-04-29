package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/yichouchou/yichouchou_agent/pkg"
)

func main() {
	log.Println("========================================")
	log.Println("Server starting...")

	// Initialize RAG from Notion using langchaingo
	rag, ragErr := pkg.InitFromEnv()
	if ragErr != nil {
		log.Printf("[WARNING] Failed to initialize RAG: %v", ragErr)
	} else {
		log.Printf("[INFO] RAG initialized with %d pages", rag.GetPageCount())
	}

	// Initialize LLM client using langchaingo
	llmClient, llmErr := pkg.InitLLM()
	if llmErr != nil {
		log.Fatalf("[ERROR] Failed to initialize LLM client: %v", llmErr)
	}
	log.Printf("[INFO] LLM initialized with model: %s", llmClient.GetModel())

	// Set LLM client to RAG
	if rag != nil {
		rag.SetLLMClient(llmClient)
	}

	// HTTP handlers
	fs := http.FileServer(http.Dir("./frontend"))
	http.Handle("/", fs)
	http.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		handleChat(w, r, rag, llmClient)
	})

	log.Println("[INFO] Server starting on :8080")
	log.Println("========================================")

	log.Fatal(http.ListenAndServe(":8080", nil))
}

type ChatRequest struct {
	Message string `json:"message"`
}

type ChatResponse struct {
	Response string `json:"response"`
}

func handleChat(w http.ResponseWriter, r *http.Request, rag *pkg.NotionRAG, llm *pkg.LLMClient) {
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
		log.Printf("[ERROR] Failed to decode request: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	log.Printf("[REQUEST] Message: %s", req.Message)

	ctx := context.Background()

	// Use RAG to query with langchaingo
	var answer string
	var queryErr error
	
	if rag != nil {
		answer, queryErr = rag.Query(ctx, req.Message)
		if queryErr != nil {
			log.Printf("[ERROR] RAG query failed: %v", queryErr)
			// Fallback to direct LLM call
			answer, queryErr = llm.Call(ctx, req.Message)
			if queryErr != nil {
				log.Printf("[ERROR] LLM call failed: %v", queryErr)
				http.Error(w, queryErr.Error(), http.StatusInternalServerError)
				return
			}
		}
	} else {
		// Direct LLM call
		answer, queryErr = llm.Call(ctx, req.Message)
		if queryErr != nil {
			log.Printf("[ERROR] LLM call failed: %v", queryErr)
			http.Error(w, queryErr.Error(), http.StatusInternalServerError)
			return
		}
	}

	log.Printf("[RESPONSE] Answer: %s", answer)

	resp := ChatResponse{Response: answer}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}