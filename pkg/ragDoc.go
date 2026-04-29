package pkg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// KnowledgeItem represents a piece of knowledge from Notion
type KnowledgeItem struct {
	PageID  string
	Title   string
	Content string
}

// NotionRAG handles RAG with Notion documents using langchaingo
type NotionRAG struct {
	pages     []KnowledgeItem
	llmClient *LLMClient
}

// NewNotionRAG creates a new Notion RAG handler with langchaingo
func NewNotionRAG() (*NotionRAG, error) {
	return &NotionRAG{
		pages: make([]KnowledgeItem, 0),
	}, nil
}

// InitFromNotion initializes RAG from Notion pages using langchaingo-compatible API
func (r *NotionRAG) InitFromNotion(pageIDs []string) error {
	cmd := exec.Command("cat", os.Getenv("HOME")+"/.config/notion/api_key")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to read Notion API key: %w", err)
	}
	notionKey := strings.TrimSpace(string(out))

	for _, pageID := range pageIDs {
		page, err := fetchNotionPage(pageID, notionKey)
		if err != nil {
			log.Printf("[WARNING] Failed to fetch Notion page %s: %v", pageID, err)
			continue
		}
		r.pages = append(r.pages, page)
		log.Printf("[INFO] Loaded knowledge from Notion page: %s", page.Title)
	}

	log.Printf("[INFO] Total knowledge items loaded: %d", len(r.pages))
	return nil
}

// Search searches for relevant documents using keyword matching
func (r *NotionRAG) Search(query string) string {
	query = strings.ToLower(query)
	var results []string

	for _, page := range r.pages {
		if strings.Contains(strings.ToLower(page.Content), query) ||
			strings.Contains(strings.ToLower(page.Title), query) {
			results = append(results, fmt.Sprintf("【%s】\n%s", page.Title, page.Content))
		}
	}

	// Return all if no match
	if len(results) == 0 {
		for _, page := range r.pages {
			results = append(results, fmt.Sprintf("【%s】\n%s", page.Title, page.Content))
		}
	}

	return strings.Join(results, "\n\n---\n\n")
}

// Query sends a query to the LLM with the retrieved context using langchaingo
func (r *NotionRAG) Query(ctx context.Context, query string) (string, error) {
	if r.llmClient == nil {
		return "", fmt.Errorf("LLM client not initialized")
	}

	context := r.Search(query)
	return r.llmClient.CallWithContext(ctx, query, context)
}

// SetLLMClient sets the LLM client for querying
func (r *NotionRAG) SetLLMClient(client *LLMClient) {
	r.llmClient = client
}

// GetPageCount returns the number of loaded pages
func (r *NotionRAG) GetPageCount() int {
	return len(r.pages)
}

// InitFromEnv creates RAG from Notion with default pages
func InitFromEnv() (*NotionRAG, error) {
	rag, err := NewNotionRAG()
	if err != nil {
		return nil, err
	}

	pageIDs := []string{
		"33bd07d5-bafb-80e4-9a2a-cffbb9d73d5e",
		"317d07d5-bafb-80d1-ae57-e3f6612f296a",
	}

	if err := rag.InitFromNotion(pageIDs); err != nil {
		return nil, err
	}

	return rag, nil
}

// Notion API types
type RichText struct {
	PlainText string `json:"plain_text"`
}

type Block struct {
	Type       string `json:"type"`
	Paragraph struct {
		RichText []RichText `json:"rich_text"`
	} `json:"paragraph"`
	ChildPage struct {
		Title string `json:"title"`
	} `json:"child_page"`
}

type NotionResponse struct {
	Results    []Block `json:"results"`
	NextCursor string `json:"next_cursor"`
	HasMore   bool   `json:"has_more"`
}

type PageResponse struct {
	Properties struct {
		Title struct {
			Title []RichText `json:"title"`
		} `json:"title"`
	} `json:"properties"`
}

// fetchNotionPage fetches a page from Notion API
func fetchNotionPage(pageID, apiKey string) (KnowledgeItem, error) {
	client := &http.Client{}

	// Get page title
	pageReq, _ := http.NewRequest("GET", fmt.Sprintf("https://api.notion.com/v1/pages/%s", pageID), nil)
	pageReq.Header.Set("Authorization", "Bearer "+apiKey)
	pageReq.Header.Set("Notion-Version", "2025-09-03")

	pageResp, err := client.Do(pageReq)
	if err != nil {
		return KnowledgeItem{}, err
	}
	defer pageResp.Body.Close()

	pageBody, _ := io.ReadAll(pageResp.Body)
	var pageData PageResponse
	json.Unmarshal(pageBody, &pageData)

	title := "Untitled"
	if len(pageData.Properties.Title.Title) > 0 {
		title = pageData.Properties.Title.Title[0].PlainText
	}

	// Get page content
	blocksReq, _ := http.NewRequest("GET", fmt.Sprintf("https://api.notion.com/v1/blocks/%s/children", pageID), nil)
	blocksReq.Header.Set("Authorization", "Bearer "+apiKey)
	blocksReq.Header.Set("Notion-Version", "2025-09-03")

	blocksResp, err := client.Do(blocksReq)
	if err != nil {
		return KnowledgeItem{}, err
	}
	defer blocksResp.Body.Close()

	blocksBody, _ := io.ReadAll(blocksResp.Body)
	var blocksData NotionResponse
	json.Unmarshal(blocksBody, &blocksData)

	var content strings.Builder
	for _, block := range blocksData.Results {
		switch block.Type {
		case "paragraph":
			for _, rt := range block.Paragraph.RichText {
				content.WriteString(rt.PlainText)
			}
		case "child_page":
			content.WriteString(" [子页面: " + block.ChildPage.Title + "] ")
		}
		content.WriteString("\n")
	}

	return KnowledgeItem{
		PageID:  pageID,
		Title:  title,
		Content: content.String(),
	}, nil
}

// Legacy functions for backward compatibility
var KnowledgeBase []KnowledgeItem

func SearchKnowledge(query string) string {
	query = strings.ToLower(query)
	var results []string

	for _, item := range KnowledgeBase {
		if strings.Contains(strings.ToLower(item.Content), query) ||
			strings.Contains(strings.ToLower(item.Title), query) {
			results = append(results, fmt.Sprintf("【%s】\n%s", item.Title, item.Content))
		}
	}

	if len(results) == 0 {
		for _, item := range KnowledgeBase {
			results = append(results, fmt.Sprintf("【%s】\n%s", item.Title, item.Content))
		}
	}

	return strings.Join(results, "\n\n---\n\n")
}

func InitKnowledgeBase() error {
	cmd := exec.Command("cat", os.Getenv("HOME")+"/.config/notion/api_key")
	out, err := cmd.Output()
	if err != nil {
		log.Printf("[WARNING] Failed to read Notion API key: %v", err)
		return err
	}
	notionKey := strings.TrimSpace(string(out))

	pageIDs := []string{
		"33bd07d5-bafb-80e4-9a2a-cffbb9d73d5e",
		"317d07d5-bafb-80d1-ae57-e3f6612f296a",
	}

	for _, pageID := range pageIDs {
		page, err := fetchNotionPage(pageID, notionKey)
		if err != nil {
			log.Printf("[WARNING] Failed to fetch Notion page %s: %v", pageID, err)
			continue
		}
		KnowledgeBase = append(KnowledgeBase, page)
		log.Printf("[INFO] Loaded knowledge from Notion page: %s", page.Title)
	}

	log.Printf("[INFO] Total knowledge items loaded: %d", len(KnowledgeBase))
	return nil
}

// NewLangChainLLM creates a new langchaingo LLM client (alias for backward compatibility)
func NewLangChainLLM(apiKey, baseURL, model string) (llms.LLM, error) {
	if baseURL == "" {
		baseURL = "https://api.minimax.chat/v1"
	}
	if model == "" {
		model = "MiniMax-M2.7"
	}

	return openai.New(
		openai.WithToken(apiKey),
		openai.WithBaseURL(baseURL),
		openai.WithModel(model),
	)
}