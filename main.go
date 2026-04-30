package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/yichouchou/yichouchou_agent/backend"
	"github.com/yichouchou/yichouchou_agent/conf"
	"github.com/yichouchou/yichouchou_agent/pkg"
)

func initLogger() error {
	if err := os.MkdirAll("logs", 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	logFile, err := os.OpenFile("logs/app.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	multiWriter := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(multiWriter)
	log.SetFlags(log.Ldate | log.Ltime)

	return nil
}

func main() {
	if err := initLogger(); err != nil {
		log.Fatalf("[ERROR] Failed to initialize logger: %v", err)
	}

	log.Println("========================================")
	log.Println("Server starting...")

	if err := conf.LoadConfig(); err != nil {
		log.Printf("[WARNING] Failed to load config: %v", err)
	}

	rag, ragErr := pkg.InitFromEnv()
	if ragErr != nil {
		log.Printf("[WARNING] Failed to initialize Notion RAG: %v", ragErr)
	} else {
		log.Printf("[INFO] Notion RAG initialized with %d pages", rag.GetPageCount())
	}

	chromaStore, chromaErr := pkg.InitChromaStore()
	if chromaErr != nil {
		log.Printf("[WARNING] Failed to initialize Chroma store: %v", chromaErr)
	} else {
		log.Printf("[INFO] Chroma store initialized")
	}

	hybridRAG := pkg.NewHybridRAG()
	if rag != nil {
		hybridRAG.SetNotionRAG(rag)
	}
	if chromaStore != nil {
		hybridRAG.SetChroma(chromaStore)
	}

	llmClient, llmErr := pkg.InitLLM()
	if llmErr != nil {
		log.Fatalf("[ERROR] Failed to initialize LLM client: %v", llmErr)
	}
	log.Printf("[INFO] LLM initialized with model: %s", llmClient.GetModel())

	if rag != nil {
		rag.SetLLMClient(llmClient)
	}

	notionCount, chromaCount := hybridRAG.GetSourceCount()
	log.Printf("[INFO] Hybrid RAG ready - Notion: %d, Chroma: %d", notionCount, chromaCount)

	backend.InitHandler(hybridRAG, llmClient)

	fs := http.FileServer(http.Dir("./frontend"))
	http.Handle("/", fs)
	http.HandleFunc("/api/chat", backend.ChatHandler)

	log.Println("[INFO] Server starting on :8080")
	log.Println("========================================")

	log.Fatal(http.ListenAndServe(":8080", nil))
}
