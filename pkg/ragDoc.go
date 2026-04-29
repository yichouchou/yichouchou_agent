package pkg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/tmc/langchaingo/schema"
)

type NotionRAG struct {
	documents []schema.Document
	llmClient *LLMClient
}

func NewNotionRAG() (*NotionRAG, error) {
	return &NotionRAG{
		documents: make([]schema.Document, 0),
	}, nil
}

func InitFromEnv() (*NotionRAG, error) {
	rag, err := NewNotionRAG()
	if err != nil {
		return nil, err
	}

	notionKey := GetNotionAPIKey()
	if notionKey == "" {
		return nil, fmt.Errorf("Notion API key is not set")
	}

	log.Printf("[INFO] Searching for accessible Notion pages...")

	pageIDs, err := searchNotionPages(notionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to search Notion pages: %w", err)
	}

	log.Printf("[INFO] Found %d accessible pages", len(pageIDs))

	for _, pageID := range pageIDs {
		doc, err := fetchNotionPageAsDocument(pageID, notionKey)
		if err != nil {
			log.Printf("[WARNING] Failed to fetch page %s: %v", pageID, err)
			continue
		}
		rag.documents = append(rag.documents, *doc)
		log.Printf("[INFO] Loaded: %s", doc.Metadata["title"])
	}

	log.Printf("[INFO] Total documents loaded: %d", len(rag.documents))
	return rag, nil
}

func searchNotionPages(apiKey string) ([]string, error) {
	client := &http.Client{}
	var allPageIDs []string
	var cursor string

	for {
		url := "https://api.notion.com/v1/search"
		if cursor != "" {
			url += "?start_cursor=" + cursor
		}

		reqBody := map[string]interface{}{
			"filter": map[string]string{
				"value":    "page",
				"property": "object",
			},
			"page_size": 100,
		}

		body, err := json.Marshal(reqBody)
		if err != nil {
			return nil, err
		}

		req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Notion-Version", "2025-09-03")
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		var searchResult struct {
			Results []struct {
				ID string `json:"id"`
			} `json:"results"`
			HasMore bool   `json:"has_more"`
			Cursor  string `json:"next_cursor"`
		}

		if err := json.Unmarshal(respBody, &searchResult); err != nil {
			return nil, fmt.Errorf("failed to parse search response: %w", err)
		}

		for _, result := range searchResult.Results {
			allPageIDs = append(allPageIDs, result.ID)
		}

		if !searchResult.HasMore || searchResult.Cursor == "" {
			break
		}
		cursor = searchResult.Cursor
	}

	return allPageIDs, nil
}

type RichText struct {
	PlainText string `json:"plain_text"`
}

type Block struct {
	Type      string `json:"type"`
	Paragraph struct {
		RichText []RichText `json:"rich_text"`
	} `json:"paragraph"`
	ChildPage struct {
		Title string `json:"title"`
	} `json:"child_page"`
}

type NotionResponse struct {
	Results    []Block `json:"results"`
	NextCursor string  `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
}

type PageResponse struct {
	Properties struct {
		Title struct {
			Title []RichText `json:"title"`
		} `json:"title"`
	} `json:"properties"`
}

func fetchNotionPageAsDocument(pageID, apiKey string) (*schema.Document, error) {
	client := &http.Client{}

	pageReq, _ := http.NewRequest("GET", fmt.Sprintf("https://api.notion.com/v1/pages/%s", pageID), nil)
	pageReq.Header.Set("Authorization", "Bearer "+apiKey)
	pageReq.Header.Set("Notion-Version", "2025-09-03")

	pageResp, err := client.Do(pageReq)
	if err != nil {
		return nil, err
	}
	defer pageResp.Body.Close()

	pageBody, _ := io.ReadAll(pageResp.Body)
	var pageData PageResponse
	json.Unmarshal(pageBody, &pageData)

	title := "Untitled"
	if len(pageData.Properties.Title.Title) > 0 {
		title = pageData.Properties.Title.Title[0].PlainText
	}

	blocksReq, _ := http.NewRequest("GET", fmt.Sprintf("https://api.notion.com/v1/blocks/%s/children", pageID), nil)
	blocksReq.Header.Set("Authorization", "Bearer "+apiKey)
	blocksReq.Header.Set("Notion-Version", "2025-09-03")

	blocksResp, err := client.Do(blocksReq)
	if err != nil {
		return nil, err
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

	return &schema.Document{
		PageContent: content.String(),
		Metadata: map[string]interface{}{
			"source":  "notion",
			"page_id": pageID,
			"title":   title,
		},
	}, nil
}

func (r *NotionRAG) Search(query string) string {
	query = strings.ToLower(query)
	var results []string

	for _, doc := range r.documents {
		title := ""
		if t, ok := doc.Metadata["title"].(string); ok {
			title = t
		}

		if strings.Contains(strings.ToLower(doc.PageContent), query) ||
			strings.Contains(strings.ToLower(title), query) {
			results = append(results, fmt.Sprintf("【%s】\n%s", title, doc.PageContent))
		}
	}

	if len(results) == 0 {
		for _, doc := range r.documents {
			title := ""
			if t, ok := doc.Metadata["title"].(string); ok {
				title = t
			}
			results = append(results, fmt.Sprintf("【%s】\n%s", title, doc.PageContent))
		}
	}

	return strings.Join(results, "\n\n---\n\n")
}

func (r *NotionRAG) Query(ctx context.Context, query string) (string, error) {
	if r.llmClient == nil {
		return "", fmt.Errorf("LLM client not initialized")
	}

	context := r.Search(query)
	return r.llmClient.CallWithContext(ctx, query, context)
}

func (r *NotionRAG) SetLLMClient(client *LLMClient) {
	r.llmClient = client
}

func (r *NotionRAG) GetPageCount() int {
	return len(r.documents)
}

func (r *NotionRAG) GetDocuments() []schema.Document {
	return r.documents
}
