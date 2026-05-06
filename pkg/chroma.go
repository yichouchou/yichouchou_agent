package pkg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

	embedder, err := CreateMiniMaxEmbedder()
	if err != nil {
		return fmt.Errorf("failed to create embedder: %w", err)
	}

	store, err := chroma.New(
		chroma.WithChromaURL(chromaURL),
		chroma.WithNameSpace(c.config.Collection),
		chroma.WithEmbedder(embedder),
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
	docs := c.SearchDocs(query)
	var results []string
	for _, doc := range docs {
		results = append(results, fmt.Sprintf("🗄️ 来源: ChromaDB | 文档: %s\n────────────────────────────────────\n%s", doc.title, doc.content))
	}
	return strings.Join(results, "\n\n---\n\n")
}

func (c *ChromaStore) SearchDocs(query string) []docInfo {
	docs, err := c.Query(context.Background(), query, 5)
	if err != nil {
		return nil
	}

	if len(docs) == 0 {
		return nil
	}

	var results []docInfo
	for _, doc := range docs {
		title := ""
		if t, ok := doc.Metadata["title"].(string); ok {
			title = t
		}
		if title == "" {
			title = "未命名文档"
		}
		results = append(results, docInfo{
			sourceType: "ChromaDB",
			title:      title,
			content:    doc.PageContent,
		})
	}

	return results
}

type docInfo struct {
	sourceType string
	title      string
	content    string
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

func (c *ChromaStore) GetDocumentCount() (int, error) {
	if c.store == nil {
		return 0, fmt.Errorf("Chroma store not initialized")
	}

	url := fmt.Sprintf("http://%s:%d/api/v1/collections/%s/count", c.config.Host, c.config.Port, c.config.Collection)
	resp, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("failed to get count from Chroma: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to parse count response: %w", err)
	}
	return result.Count, nil
}

func (c *ChromaStore) DeleteByMetadata(key, value string) error {
	if c.store == nil {
		return fmt.Errorf("Chroma store not initialized")
	}

	url := fmt.Sprintf("http://%s:%d/api/v1/collections/%s/delete", c.config.Host, c.config.Port, c.config.Collection)

	body := map[string]interface{}{
		"where": map[string]string{key: value},
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete by metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete returned status: %d", resp.StatusCode)
	}
	return nil
}

type HybridRAG struct {
	notionRAG      *NotionRAG
	chroma         *ChromaStore
	markdownLoader *MarkdownLoader
	llmClient      *LLMClient
}

func NewHybridRAG() *HybridRAG {
	return &HybridRAG{
		notionRAG:      nil,
		chroma:         nil,
		markdownLoader: nil,
		llmClient:      nil,
	}
}

func (h *HybridRAG) SetLLMClient(client *LLMClient) {
	h.llmClient = client
}

func (h *HybridRAG) SetNotionRAG(rag *NotionRAG) {
	h.notionRAG = rag
}

func (h *HybridRAG) SetChroma(chromaStore *ChromaStore) {
	h.chroma = chromaStore
}

func (h *HybridRAG) SetMarkdownLoader(loader *MarkdownLoader) {
	h.markdownLoader = loader
}

func (h *HybridRAG) Search(query string) string {
	var results []string

	if h.notionRAG != nil {
		notionResults := h.notionRAG.Search(query)
		if notionResults != "" {
			results = append(results, notionResults)
		}
	}

	if h.chroma != nil {
		chromaResults := h.chroma.Search(query)
		if chromaResults != "" {
			results = append(results, chromaResults)
		}
	}

	if h.markdownLoader != nil {
		markdownResults := h.markdownLoader.Search(query)
		if markdownResults != "" {
			results = append(results, markdownResults)
		}
	}

	if len(results) == 0 {
		return ""
	}

	return strings.Join(results, "\n\n────────────────────────────────────────────────────────────────\n\n")
}

func (h *HybridRAG) Query(ctx context.Context, query string) (string, error) {
	var allDocs []docInfo

	if h.notionRAG != nil {
		docs := h.notionRAG.SearchDocs(query)
		for _, doc := range docs {
			allDocs = append(allDocs, docInfo{
				sourceType: "Notion",
				title:      doc.title,
				content:    doc.content,
			})
		}
	}

	if h.chroma != nil {
		docs := h.chroma.SearchDocs(query)
		for _, doc := range docs {
			allDocs = append(allDocs, docInfo{
				sourceType: "ChromaDB",
				title:      doc.title,
				content:    doc.content,
			})
		}
	}

	if h.markdownLoader != nil {
		docs := h.markdownLoader.SearchDocs(query)
		for _, doc := range docs {
			allDocs = append(allDocs, docInfo{
				sourceType: "Markdown",
				title:      doc.title,
				content:    doc.content,
			})
		}
	}

	if len(allDocs) == 0 {
		return "抱歉，我在知识库中没有找到与您的问题相关的内容。您可以尝试使用其他关键词提问，或者检查知识库是否已正确配置。", nil
	}

	var context strings.Builder
	context.WriteString("【知识库引用文档列表】\n\n")
	for i, doc := range allDocs {
		context.WriteString(fmt.Sprintf("文档 %d:\n", i+1))
		context.WriteString(fmt.Sprintf("  来源类型: %s\n", doc.sourceType))
		context.WriteString(fmt.Sprintf("  文档标题: %s\n", doc.title))
		context.WriteString(fmt.Sprintf("  内容:\n%s\n\n", doc.content))
	}

	context.WriteString(fmt.Sprintf("【用户问题】\n%s\n\n【回答要求】\n1. 必须基于以上知识库内容回答问题\n2. 回答中必须明确标注每条信息的来源类型（Notion/ChromaDB/Markdown）和文档标题\n3. 使用以下格式标注来源：【来源:XXX | 标题:XXX】\n4. 如果涉及多个来源，请分别标注\n5. 回答要准确、简洁、有条理\n6. 在回答的每个关键点后面，都要附上来源标注", query))

	fullPrompt := context.String()

	if h.llmClient != nil {
		return h.llmClient.Call(ctx, fullPrompt)
	}

	return "", fmt.Errorf("LLM client not initialized")
}

func (h *HybridRAG) GetSourceCount() (notionCount, chromaCount, markdownCount int) {
	if h.notionRAG != nil {
		notionCount = h.notionRAG.GetPageCount()
	}
	if h.chroma != nil {
		chromaCount = 1
	}
	if h.markdownLoader != nil {
		markdownCount = h.markdownLoader.GetDocumentCount()
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
