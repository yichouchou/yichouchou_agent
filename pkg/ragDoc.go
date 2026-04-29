package pkg

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

var KnowledgeBase []KnowledgeItem

type KnowledgeItem struct {
	PageID  string
	Title   string
	Content string
}

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

type RichText struct {
	PlainText string `json:"plain_text"`
	Text      struct {
		Content string `json:"content"`
	} `json:"text"`
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
		content, title, err := fetchNotionPage(pageID, notionKey)
		if err != nil {
			log.Printf("[WARNING] Failed to fetch Notion page %s: %v", pageID, err)
			continue
		}
		KnowledgeBase = append(KnowledgeBase, KnowledgeItem{
			PageID:  pageID,
			Title:   title,
			Content: content,
		})
		log.Printf("[INFO] Loaded knowledge from Notion page: %s", title)
	}

	log.Printf("[INFO] Total knowledge items loaded: %d", len(KnowledgeBase))
	return nil
}

func fetchNotionPage(pageID, apiKey string) (string, string, error) {
	pageURL := fmt.Sprintf("https://api.notion.com/v1/pages/%s", pageID)
	pageReq, _ := http.NewRequest("GET", pageURL, nil)
	pageReq.Header.Set("Authorization", "Bearer "+apiKey)
	pageReq.Header.Set("Notion-Version", "2025-09-03")

	client := &http.Client{}
	pageResp, err := client.Do(pageReq)
	if err != nil {
		return "", "", err
	}
	defer pageResp.Body.Close()

	pageBody, _ := io.ReadAll(pageResp.Body)
	var pageData PageResponse
	json.Unmarshal(pageBody, &pageData)

	title := "Untitled"
	if len(pageData.Properties.Title.Title) > 0 {
		title = pageData.Properties.Title.Title[0].PlainText
	}

	blocksURL := fmt.Sprintf("https://api.notion.com/v1/blocks/%s/children", pageID)
	req, _ := http.NewRequest("GET", blocksURL, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Notion-Version", "2025-09-03")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var notionResp NotionResponse
	json.Unmarshal(body, &notionResp)

	var content strings.Builder
	for _, block := range notionResp.Results {
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

	return content.String(), title, nil
}
