package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type ChatRequest struct {
	Message string `json:"message"`
}

type ChatResponse struct {
	Response string `json:"response"`
}

type LLMConfig struct {
	APIKey  string
	BaseURL string
	Model  string
}

type KnowledgeItem struct {
	PageID   string
	Title   string
	Content string
}

type RichText struct {
	PlainText string `json:"plain_text"`
	Text      struct {
		Content string `json:"content"`
	} `json:"text"`
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
	NextCursor string  `json:"next_cursor"`
	HasMore   bool    `json:"has_more"`
}

type PageResponse struct {
	Properties struct {
		Title struct {
			Title []RichText `json:"title"`
		} `json:"title"`
	} `json:"properties"`
}

var llmConfig LLMConfig
var knowledgeBase []KnowledgeItem

func initKnowledgeBase() error {
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
		knowledgeBase = append(knowledgeBase, KnowledgeItem{
			PageID:   pageID,
			Title:   title,
			Content: content,
		})
		log.Printf("[INFO] Loaded knowledge from Notion page: %s", title)
	}

	log.Printf("[INFO] Total knowledge items loaded: %d", len(knowledgeBase))
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

func searchKnowledge(query string) string {
	query = strings.ToLower(query)
	var results []string

	for _, item := range knowledgeBase {
		if strings.Contains(strings.ToLower(item.Content), query) ||
			strings.Contains(strings.ToLower(item.Title), query) {
			results = append(results, fmt.Sprintf("【%s】\n%s", item.Title, item.Content))
		}
	}

	if len(results) == 0 {
		for _, item := range knowledgeBase {
			results = append(results, fmt.Sprintf("【%s】\n%s", item.Title, item.Content))
		}
	}

	return strings.Join(results, "\n\n---\n\n")
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
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

	context := searchKnowledge(req.Message)
	log.Printf("[INFO] Retrieved context length: %d", len(context))

	prompt := req.Message
	if context != "" {
		prompt = fmt.Sprintf("请根据以下知识库内容回答问题。\n\n知识库内容:\n%s\n\n用户问题: %s", context, req.Message)
	}

	answer, err := callMiniMaxAPI(prompt, llmConfig)
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

func callMiniMaxAPI(message string, config LLMConfig) (string, error) {
	type Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	type MiniMaxRequest struct {
		Model       string    `json:"model"`
		Messages    []Message `json:"messages"`
		Temperature float64   `json:"temperature"`
	}

	type Choice struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}

	type MiniMaxResponse struct {
		Choices []Choice `json:"choices"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	apiURL := fmt.Sprintf("%s/text/chatcompletion_v2", strings.TrimSuffix(config.BaseURL, "/"))

	reqBody := MiniMaxRequest{
		Model: config.Model,
		Messages: []Message{
			{Role: "user", Content: message},
		},
		Temperature: 0.7,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Printf("[DEBUG] Calling MiniMax API: %s", apiURL)

	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+config.APIKey)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("[DEBUG] Response status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var miniResp MiniMaxResponse
	if err := json.Unmarshal(body, &miniResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if miniResp.Error != nil {
		return "", fmt.Errorf("MiniMax API error: %s", miniResp.Error.Message)
	}

	if len(miniResp.Choices) == 0 {
		return "", fmt.Errorf("empty response from API")
	}

	return miniResp.Choices[0].Message.Content, nil
}

func main() {
	log.Println("========================================")
	log.Println("Server starting...")

	if err := initKnowledgeBase(); err != nil {
		log.Printf("[WARNING] Failed to initialize knowledge base: %v", err)
	}

	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
		log.Fatal("MINIMAX_API_KEY is not set")
	}

	baseURL := os.Getenv("MINIMAX_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.minimax.chat/v1"
	}

	llmConfig = LLMConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:  "MiniMax-M2.7",
	}

	log.Printf("[INFO] LLM Config initialized")
	log.Printf("[INFO] Model: %s", llmConfig.Model)

	fs := http.FileServer(http.Dir("./frontend"))
	http.Handle("/", fs)
	http.HandleFunc("/api/chat", chatHandler)

	log.Println("[INFO] Server starting on :8080")
	log.Println("========================================")

	log.Fatal(http.ListenAndServe(":8080", nil))
}