package pkg

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/tmc/langchaingo/schema"
	"github.com/yichouchou/yichouchou_agent/conf"
)

type SyncState struct {
	LastSync        string            `json:"last_sync"`
	SyncedDocuments map[string]string `json:"synced_documents"`
	FailedDocuments []string          `json:"failed_documents"`
}

var syncState = &SyncState{
	SyncedDocuments: make(map[string]string),
}

type ChunkConfig struct {
	ChunkSize    int
	ChunkOverlap int
}

var defaultChunkConfig = ChunkConfig{
	ChunkSize:    500,
	ChunkOverlap: 50,
}

func GetSyncStatePath() string {
	if path := os.Getenv("SYNC_STATE_FILE"); path != "" {
		return path
	}
	return "notion_sync_state.json"
}

func LoadSyncState() error {
	path := GetSyncStatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read sync state: %w", err)
	}
	return json.Unmarshal(data, &syncState)
}

func SaveSyncState() error {
	path := GetSyncStatePath()
	data, err := json.MarshalIndent(syncState, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sync state: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func ComputeChecksum(content string) string {
	hash := md5.Sum([]byte(content))
	return fmt.Sprintf("%x", hash)
}

func shouldSkipDocument(pageID, checksum string) bool {
	if existingChecksum, exists := syncState.SyncedDocuments[pageID]; exists {
		return existingChecksum == checksum
	}
	return false
}

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

	notionKey := conf.GetNotionAPIKey()
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
	Object      string `json:"object"`
	Type        string `json:"type"`
	ID          string `json:"id"`
	HasChildren bool   `json:"has_children"`
	Paragraph   struct {
		RichText []RichText `json:"rich_text"`
	} `json:"paragraph"`
	ChildPage struct {
		Title string `json:"title"`
	} `json:"child_page"`
	Heading1 struct {
		RichText []RichText `json:"rich_text"`
	} `json:"heading_1"`
	Heading2 struct {
		RichText []RichText `json:"rich_text"`
	} `json:"heading_2"`
	Heading3 struct {
		RichText []RichText `json:"rich_text"`
	} `json:"heading_3"`
	BulletedListItem struct {
		RichText []RichText `json:"rich_text"`
	} `json:"bulleted_list_item"`
	NumberedListItem struct {
		RichText []RichText `json:"rich_text"`
	} `json:"numbered_list_item"`
	Quote struct {
		RichText []RichText `json:"rich_text"`
	} `json:"quote"`
	Code struct {
		RichText []RichText `json:"rich_text"`
		Language string     `json:"language"`
	} `json:"code"`
	Table struct {
		TableWidth int `json:"table_width"`
	} `json:"table"`
	TableRow struct {
		Cells [][]RichText `json:"cells"`
	} `json:"table_row"`
	TableHeader struct {
		Cells [][]RichText `json:"cells"`
	} `json:"table_header"`
}

type NotionResponse struct {
	Results    []Block `json:"results"`
	NextCursor string  `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
}

type PageResponse struct {
	ID         string `json:"id"`
	LastEdited string `json:"last_edited_time"`
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

	var content strings.Builder
	err = fetchBlockChildren(pageID, apiKey, &content, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch block children: %w", err)
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

func fetchBlockChildren(blockID, apiKey string, content *strings.Builder, depth int) error {
	client := &http.Client{}
	var cursor string

	for {
		url := fmt.Sprintf("https://api.notion.com/v1/blocks/%s/children", blockID)
		if cursor != "" {
			url += "?start_cursor=" + cursor
		}

		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Notion-Version", "2025-09-03")

		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var blocksData NotionResponse
		if err := json.Unmarshal(body, &blocksData); err != nil {
			return fmt.Errorf("failed to parse blocks: %w", err)
		}

		isInTable := false
		for _, block := range blocksData.Results {
			switch block.Type {
			case "paragraph":
				if isInTable {
					content.WriteString("\n")
					isInTable = false
				}
				for _, rt := range block.Paragraph.RichText {
					content.WriteString(rt.PlainText)
				}
				content.WriteString("\n")

			case "heading_1":
				if isInTable {
					content.WriteString("\n")
					isInTable = false
				}
				content.WriteString("# ")
				for _, rt := range block.Heading1.RichText {
					content.WriteString(rt.PlainText)
				}
				content.WriteString("\n")

			case "heading_2":
				if isInTable {
					content.WriteString("\n")
					isInTable = false
				}
				content.WriteString("## ")
				for _, rt := range block.Heading2.RichText {
					content.WriteString(rt.PlainText)
				}
				content.WriteString("\n")

			case "heading_3":
				if isInTable {
					content.WriteString("\n")
					isInTable = false
				}
				content.WriteString("### ")
				for _, rt := range block.Heading3.RichText {
					content.WriteString(rt.PlainText)
				}
				content.WriteString("\n")

			case "bulleted_list_item":
				if isInTable {
					content.WriteString("\n")
					isInTable = false
				}
				content.WriteString("- ")
				for _, rt := range block.BulletedListItem.RichText {
					content.WriteString(rt.PlainText)
				}
				content.WriteString("\n")

			case "numbered_list_item":
				if isInTable {
					content.WriteString("\n")
					isInTable = false
				}
				content.WriteString("1. ")
				for _, rt := range block.NumberedListItem.RichText {
					content.WriteString(rt.PlainText)
				}
				content.WriteString("\n")

			case "quote":
				if isInTable {
					content.WriteString("\n")
					isInTable = false
				}
				content.WriteString("> ")
				for _, rt := range block.Quote.RichText {
					content.WriteString(rt.PlainText)
				}
				content.WriteString("\n")

			case "code":
				if isInTable {
					content.WriteString("\n")
					isInTable = false
				}
				content.WriteString("```" + block.Code.Language + "\n")
				for _, rt := range block.Code.RichText {
					content.WriteString(rt.PlainText)
				}
				content.WriteString("\n```\n")

			case "child_page":
				if isInTable {
					content.WriteString("\n")
					isInTable = false
				}
				content.WriteString("[子页面: " + block.ChildPage.Title + "]\n")

			case "table":
				content.WriteString("【表格开始】\n")
				isInTable = true
				if block.HasChildren {
					err := fetchBlockChildren(block.ID, apiKey, content, depth+1)
					if err != nil {
						return err
					}
				}
				content.WriteString("【表格结束】\n")
				isInTable = false

			case "table_header":
				isInTable = true
				for i, cell := range block.TableHeader.Cells {
					if i > 0 {
						content.WriteString(" | ")
					}
					for _, rt := range cell {
						content.WriteString(rt.PlainText)
					}
				}
				content.WriteString("\n")
				for i := 0; i < len(block.TableHeader.Cells); i++ {
					if i > 0 {
						content.WriteString("|")
					}
					content.WriteString("---")
				}
				content.WriteString("\n")

			case "table_row":
				isInTable = true
				for i, cell := range block.TableRow.Cells {
					if i > 0 {
						content.WriteString(" | ")
					}
					for _, rt := range cell {
						content.WriteString(rt.PlainText)
					}
				}
				content.WriteString("\n")
			}
		}

		if !blocksData.HasMore || blocksData.NextCursor == "" {
			break
		}
		cursor = blocksData.NextCursor
	}

	return nil
}

func (r *NotionRAG) Search(query string) string {
	docs := r.SearchDocs(query)
	var results []string
	for _, doc := range docs {
		results = append(results, fmt.Sprintf("📄 来源: Notion | 文档: %s\n────────────────────────────────────\n%s", doc.title, doc.content))
	}
	return strings.Join(results, "\n\n---\n\n")
}

func (r *NotionRAG) SearchDocs(query string) []docInfo {
	query = strings.ToLower(query)
	var results []docInfo

	for _, doc := range r.documents {
		title := ""
		if t, ok := doc.Metadata["title"].(string); ok {
			title = t
		}

		if strings.Contains(strings.ToLower(doc.PageContent), query) ||
			strings.Contains(strings.ToLower(title), query) {
			results = append(results, docInfo{
				sourceType: "Notion",
				title:      title,
				content:    doc.PageContent,
			})
		}
	}

	if len(results) == 0 {
		for _, doc := range r.documents {
			title := ""
			if t, ok := doc.Metadata["title"].(string); ok {
				title = t
			}
			results = append(results, docInfo{
				sourceType: "Notion",
				title:      title,
				content:    doc.PageContent,
			})
		}
	}

	return results
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

func ChunkText(text string, cfg ChunkConfig) []string {
	if text == "" {
		return nil
	}

	var chunks []string
	content := text

	segments := strings.Split(content, "\n\n")

	var currentChunk string
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}

		if len(currentChunk)+len(segment) > cfg.ChunkSize && currentChunk != "" {
			chunks = append(chunks, strings.TrimSpace(currentChunk))

			overlapLen := cfg.ChunkOverlap
			if overlapLen > len(currentChunk) {
				overlapLen = len(currentChunk)
			}
			currentChunk = currentChunk[len(currentChunk)-overlapLen:] + "\n\n" + segment
		} else {
			if currentChunk != "" {
				currentChunk += "\n\n"
			}
			currentChunk += segment
		}
	}

	if currentChunk != "" {
		chunks = append(chunks, strings.TrimSpace(currentChunk))
	}

	return chunks
}

func extractRelevantSnippet(content, query string, maxLength int) string {
	if content == "" {
		return ""
	}

	lowerContent := strings.ToLower(content)
	lowerQuery := strings.ToLower(query)

	idx := strings.Index(lowerContent, lowerQuery)
	if idx == -1 {
		if len(content) <= maxLength {
			return content
		}
		return content[:maxLength] + "..."
	}

	queryLen := len(query)
	start := idx - maxLength/3
	if start < 0 {
		start = 0
	}
	end := idx + queryLen + maxLength/3*2
	if end > len(content) {
		end = len(content)
	}

	snippet := content[start:end]

	if start > 0 {
		prevSpaceIdx := strings.LastIndex(content[:start], " ")
		if prevSpaceIdx != -1 {
			snippet = "..." + content[prevSpaceIdx+1:end]
		}
	}

	if end < len(content) {
		nextSpaceIdx := strings.Index(content[end:], " ")
		if nextSpaceIdx != -1 {
			snippet = content[start:end+nextSpaceIdx] + "..."
		} else {
			snippet = content[start:] + "..."
		}
	}

	return strings.TrimSpace(snippet)
}
