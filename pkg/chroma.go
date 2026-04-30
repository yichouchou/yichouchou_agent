package pkg

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/yichouchou/yichouchou_agent/conf"

	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores"
	"github.com/tmc/langchaingo/vectorstores/chroma"
)

type ChromaStore struct {
	store  *chroma.Store
	config conf.ChromaConfig
}

func NewChromaStore(host string, port int, collectionName string) *ChromaStore {
	return &ChromaStore{
		config: conf.ChromaConfig{
			Host:       host,
			Port:       port,
			Collection: collectionName,
		},
	}
}

func (c *ChromaStore) Connect() error {
	chromaURL := fmt.Sprintf("http://%s:%d", c.config.Host, c.config.Port)

	store, err := chroma.New(
		chroma.WithChromaURL(chromaURL),
		chroma.WithNameSpace(c.config.Collection),
	)
	if err != nil {
		return fmt.Errorf("failed to create Chroma store: %w", err)
	}
	c.store = &store

	return nil
}

func (c *ChromaStore) Query(ctx context.Context, queryText string, limit int) ([]schema.Document, error) {
	if c.store == nil {
		return nil, fmt.Errorf("Chroma store not initialized")
	}

	if limit <= 0 {
		limit = 5
	}

	results, err := c.store.SimilaritySearch(ctx, queryText, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query Chroma: %w", err)
	}

	return results, nil
}

func (c *ChromaStore) Search(query string) string {
	docs, err := c.Query(context.Background(), query, 5)
	if err != nil {
		return ""
	}

	if len(docs) == 0 {
		return ""
	}

	var results []string
	for _, doc := range docs {
		title := ""
		if t, ok := doc.Metadata["title"].(string); ok {
			title = t
		}
		source := ""
		if s, ok := doc.Metadata["source"].(string); ok {
			source = s
		}
		results = append(results, fmt.Sprintf("【%s | %s】\n%s", source, title, doc.PageContent))
	}

	return strings.Join(results, "\n\n---\n\n")
}

func (c *ChromaStore) AddDocuments(ctx context.Context, docs []schema.Document) error {
	if c.store == nil {
		return fmt.Errorf("Chroma store not initialized")
	}

	_, err := c.store.AddDocuments(ctx, docs, vectorstores.WithNameSpace(c.config.Collection))
	if err != nil {
		return fmt.Errorf("failed to add documents: %w", err)
	}

	return nil
}

type HybridRAG struct {
	notionRAG *NotionRAG
	chroma    *ChromaStore
}

func NewHybridRAG() *HybridRAG {
	return &HybridRAG{
		notionRAG: nil,
		chroma:    nil,
	}
}

func (h *HybridRAG) SetNotionRAG(rag *NotionRAG) {
	h.notionRAG = rag
}

func (h *HybridRAG) SetChroma(chromaStore *ChromaStore) {
	h.chroma = chromaStore
}

func (h *HybridRAG) Search(query string) string {
	var results []string

	if h.notionRAG != nil {
		notionResults := h.notionRAG.Search(query)
		if notionResults != "" {
			results = append(results, "【来自 Notion 知识库】\n"+notionResults)
		}
	}

	if h.chroma != nil {
		chromaResults := h.chroma.Search(query)
		if chromaResults != "" {
			results = append(results, "【来自 Chroma 向量库】\n"+chromaResults)
		}
	}

	if len(results) == 0 {
		return ""
	}

	return strings.Join(results, "\n\n========================================\n\n")
}

func (h *HybridRAG) Query(ctx context.Context, query string) (string, error) {
	context := h.Search(query)

	if context == "" {
		return "", fmt.Errorf("no relevant documents found")
	}

	fullPrompt := fmt.Sprintf("请根据以下知识库内容回答问题。\n\n知识库内容:\n%s\n\n用户问题: %s", context, query)

	if h.notionRAG != nil && h.notionRAG.llmClient != nil {
		return h.notionRAG.llmClient.Call(ctx, fullPrompt)
	}

	return "", fmt.Errorf("LLM client not initialized")
}

func (h *HybridRAG) GetSourceCount() (notionCount, chromaCount int) {
	if h.notionRAG != nil {
		notionCount = h.notionRAG.GetPageCount()
	}
	if h.chroma != nil {
		chromaCount = 1
	}
	return
}

func InitChromaStore() (*ChromaStore, error) {
	chromaConfig := conf.GetChromaConfig()
	if chromaConfig == nil {
		return nil, fmt.Errorf("Chroma config not found")
	}

	chromaStore := NewChromaStore(chromaConfig.Host, chromaConfig.Port, chromaConfig.Collection)

	if err := chromaStore.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to Chroma: %w", err)
	}

	return chromaStore, nil
}

func InitHybridRAG() (*HybridRAG, error) {
	hybrid := NewHybridRAG()

	notionRAG, err := InitFromEnv()
	if err != nil {
		log.Printf("[WARNING] Failed to initialize Notion RAG: %v", err)
	} else {
		hybrid.SetNotionRAG(notionRAG)
		log.Printf("[INFO] Notion RAG initialized with %d documents", notionRAG.GetPageCount())
	}

	chromaStore, err := InitChromaStore()
	if err != nil {
		log.Printf("[WARNING] Failed to initialize Chroma store: %v", err)
	} else {
		hybrid.SetChroma(chromaStore)
		log.Printf("[INFO] Chroma store initialized")
	}

	if hybrid.notionRAG == nil && hybrid.chroma == nil {
		return nil, fmt.Errorf("failed to initialize any RAG source")
	}

	return hybrid, nil
}
