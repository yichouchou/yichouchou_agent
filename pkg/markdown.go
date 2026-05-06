package pkg

import (
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tmc/langchaingo/schema"
	"github.com/yichouchou/yichouchou_agent/conf"
)

type MarkdownLoader struct {
	documents   []schema.Document
	chunkedDocs []chunkedDocument
	bm25Stats   bm25Statistics
	k1          float64
	b           float64
}

type chunkedDocument struct {
	title       string
	content     string
	contentType string
	headingPath string
	sourceFile  string
	wordCount   int
}

type bm25Statistics struct {
	docCount       int
	avgDocLength   float64
	totalDocLength int
	docFreq        map[string]int
}

func NewMarkdownLoader() *MarkdownLoader {
	return &MarkdownLoader{
		documents:   make([]schema.Document, 0),
		chunkedDocs: make([]chunkedDocument, 0),
		k1:          1.2,
		b:           0.75,
		bm25Stats: bm25Statistics{
			docFreq: make(map[string]int),
		},
	}
}

func InitMarkdownLoader() (*MarkdownLoader, error) {
	loader := NewMarkdownLoader()

	markdownDir := conf.GetMarkdownDir()
	if markdownDir == "" {
		log.Printf("[INFO] Markdown directory not configured, skipping markdown loading")
		return loader, nil
	}

	if err := loader.LoadFromDirectory(markdownDir); err != nil {
		return nil, fmt.Errorf("failed to load markdown files: %w", err)
	}

	loader.processDocuments()
	loader.precomputeBM25Stats()

	log.Printf("[INFO] Markdown loader initialized with %d documents (%d chunks)", loader.GetDocumentCount(), len(loader.chunkedDocs))
	return loader, nil
}

func (m *MarkdownLoader) LoadFromDirectory(dirPath string) error {
	info, err := os.Stat(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("[WARNING] Markdown directory %s does not exist", dirPath)
			return nil
		}
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dirPath)
	}

	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			if err := m.LoadFile(path); err != nil {
				log.Printf("[WARNING] Failed to load markdown file %s: %v", path, err)
			}
		}

		return nil
	})

	return err
}

func (m *MarkdownLoader) LoadFile(filePath string) error {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	title := strings.TrimSuffix(filepath.Base(filePath), ".md")

	doc := schema.Document{
		PageContent: string(content),
		Metadata: map[string]interface{}{
			"source": "markdown",
			"file":   filePath,
			"title":  title,
		},
	}

	m.documents = append(m.documents, doc)
	log.Printf("[DEBUG] Loaded markdown file: %s", filePath)
	return nil
}

func (m *MarkdownLoader) processDocuments() {
	for _, doc := range m.documents {
		filePath := ""
		if f, ok := doc.Metadata["file"].(string); ok {
			filePath = f
		}
		title := ""
		if t, ok := doc.Metadata["title"].(string); ok {
			title = t
		}

		chunks := m.chunkMarkdown(doc.PageContent, title, filePath)
		m.chunkedDocs = append(m.chunkedDocs, chunks...)
	}
}

func (m *MarkdownLoader) chunkMarkdown(content, title, filePath string) []chunkedDocument {
	var chunks []chunkedDocument
	var currentHeadingPath []string

	lines := strings.Split(content, "\n")
	var currentBlock strings.Builder
	var currentBlockType = "paragraph"

	for _, line := range lines {
		headingMatch := regexp.MustCompile(`^(#{1,6})\s+(.+)$`).FindStringSubmatch(line)
		if len(headingMatch) > 0 {
			if currentBlock.Len() > 0 {
				contentStr := strings.TrimSpace(currentBlock.String())
				if contentStr != "" {
					chunks = append(chunks, chunkedDocument{
						title:       title,
						content:     contentStr,
						contentType: currentBlockType,
						headingPath: strings.Join(currentHeadingPath, " > "),
						sourceFile:  filePath,
						wordCount:   len(tokenize(contentStr)),
					})
				}
				currentBlock.Reset()
			}

			level := len(headingMatch[1])
			headingText := headingMatch[2]

			if level <= len(currentHeadingPath) {
				currentHeadingPath = currentHeadingPath[:level-1]
			}
			currentHeadingPath = append(currentHeadingPath, headingText)
			currentBlockType = "heading"
			currentBlock.WriteString(line)
			continue
		}

		if strings.HasPrefix(line, "```") {
			if currentBlock.Len() > 0 {
				contentStr := strings.TrimSpace(currentBlock.String())
				if contentStr != "" {
					chunks = append(chunks, chunkedDocument{
						title:       title,
						content:     contentStr,
						contentType: currentBlockType,
						headingPath: strings.Join(currentHeadingPath, " > "),
						sourceFile:  filePath,
						wordCount:   len(tokenize(contentStr)),
					})
				}
				currentBlock.Reset()
			}
			currentBlockType = "code"
			currentBlock.WriteString(line + "\n")
			continue
		}

		if strings.HasPrefix(line, "|") && strings.Contains(line, "---") {
			currentBlockType = "table"
		}

		if line == "" && currentBlockType != "code" && currentBlock.Len() > 0 {
			contentStr := strings.TrimSpace(currentBlock.String())
			if contentStr != "" {
				chunks = append(chunks, chunkedDocument{
					title:       title,
					content:     contentStr,
					contentType: currentBlockType,
					headingPath: strings.Join(currentHeadingPath, " > "),
					sourceFile:  filePath,
					wordCount:   len(tokenize(contentStr)),
				})
			}
			currentBlock.Reset()
			currentBlockType = "paragraph"
			continue
		}

		if currentBlock.Len() > 0 && currentBlockType != "code" {
			currentBlock.WriteString("\n")
		}
		currentBlock.WriteString(line)
	}

	if currentBlock.Len() > 0 {
		contentStr := strings.TrimSpace(currentBlock.String())
		if contentStr != "" {
			chunks = append(chunks, chunkedDocument{
				title:       title,
				content:     contentStr,
				contentType: currentBlockType,
				headingPath: strings.Join(currentHeadingPath, " > "),
				sourceFile:  filePath,
				wordCount:   len(tokenize(contentStr)),
			})
		}
	}

	return chunks
}

func (m *MarkdownLoader) precomputeBM25Stats() {
	if len(m.chunkedDocs) == 0 {
		return
	}

	m.bm25Stats.docCount = len(m.chunkedDocs)
	m.bm25Stats.totalDocLength = 0

	for _, chunk := range m.chunkedDocs {
		m.bm25Stats.totalDocLength += chunk.wordCount

		words := tokenize(chunk.content)
		uniqueWords := make(map[string]bool)
		for _, word := range words {
			uniqueWords[word] = true
		}

		titleWords := tokenize(chunk.title)
		for _, word := range titleWords {
			uniqueWords[word] = true
		}

		headingWords := tokenize(chunk.headingPath)
		for _, word := range headingWords {
			uniqueWords[word] = true
		}

		for word := range uniqueWords {
			m.bm25Stats.docFreq[word]++
		}
	}

	m.bm25Stats.avgDocLength = float64(m.bm25Stats.totalDocLength) / float64(m.bm25Stats.docCount)
}

func (m *MarkdownLoader) idf(word string) float64 {
	if m.bm25Stats.docCount == 0 {
		return 0
	}

	df := m.bm25Stats.docFreq[word]
	if df == 0 {
		df = 1
	}

	return math.Log((float64(m.bm25Stats.docCount)-float64(df)+0.5)/(float64(df)+0.5) + 1.0)
}

func (m *MarkdownLoader) bm25Score(chunk chunkedDocument, queryTokens []string) float64 {
	if m.bm25Stats.docCount == 0 || chunk.wordCount == 0 {
		return 0
	}

	score := 0.0
	contentLower := strings.ToLower(chunk.content)
	titleLower := strings.ToLower(chunk.title)
	headingLower := strings.ToLower(chunk.headingPath)

	for _, token := range queryTokens {
		tokenLower := strings.ToLower(token)

		tfContent := float64(strings.Count(contentLower, tokenLower))
		tfTitle := float64(strings.Count(titleLower, tokenLower)) * 2.0
		tfHeading := float64(strings.Count(headingLower, tokenLower)) * 1.5

		tf := tfContent + tfTitle + tfHeading

		if tf == 0 {
			continue
		}

		idf := m.idf(tokenLower)
		docLength := float64(chunk.wordCount)

		numerator := tf * (m.k1 + 1)
		denominator := tf + m.k1*(1-m.b+m.b*(docLength/m.bm25Stats.avgDocLength))

		score += idf * (numerator / denominator)
	}

	return score
}

func (m *MarkdownLoader) Search(query string) string {
	docs := m.SearchDocs(query)
	var results []string
	for _, doc := range docs {
		results = append(results, fmt.Sprintf("📝 来源: Markdown | 文件: %s\n────────────────────────────────────\n%s", doc.title, doc.content))
	}
	return strings.Join(results, "\n\n---\n\n")
}

func (m *MarkdownLoader) SearchDocs(query string) []docInfo {
	if query == "" {
		return nil
	}

	queryTokens := tokenize(query)

	if len(queryTokens) == 0 {
		return nil
	}

	return m.bm25Search(queryTokens)
}

func (m *MarkdownLoader) bm25Search(queryTokens []string) []docInfo {
	type scoredChunk struct {
		title       string
		content     string
		headingPath string
		score       float64
		matchPos    int
	}

	var scoredChunks []scoredChunk
	queryStr := strings.Join(queryTokens, " ")

	for _, chunk := range m.chunkedDocs {
		bm25Score := m.bm25Score(chunk, queryTokens)

		if bm25Score > 0 {
			lowerQuery := strings.ToLower(queryStr)
			lowerContent := strings.ToLower(chunk.content)
			matchPos := len(lowerContent)

			if idx := strings.Index(lowerContent, lowerQuery); idx != -1 {
				matchPos = idx
			} else {
				for _, token := range queryTokens {
					if idx := strings.Index(lowerContent, strings.ToLower(token)); idx != -1 && idx < matchPos {
						matchPos = idx
					}
				}
			}

			scoredChunks = append(scoredChunks, scoredChunk{
				title:       chunk.title,
				content:     chunk.content,
				headingPath: chunk.headingPath,
				score:       bm25Score,
				matchPos:    matchPos,
			})
		}
	}

	sort.Slice(scoredChunks, func(i, j int) bool {
		if scoredChunks[i].score != scoredChunks[j].score {
			return scoredChunks[i].score > scoredChunks[j].score
		}
		return scoredChunks[i].matchPos < scoredChunks[j].matchPos
	})

	seenTitles := make(map[string]bool)
	seenHeadings := make(map[string]bool)
	var results []docInfo

	for _, chunk := range scoredChunks {
		if len(results) >= 5 {
			break
		}
		if seenTitles[chunk.title] {
			continue
		}

		chapterContent := m.getChapterContent(chunk.title, chunk.headingPath)

		seenTitles[chunk.title] = true
		seenHeadings[chunk.headingPath] = true

		results = append(results, docInfo{
			sourceType: "Markdown",
			title:      chunk.title,
			content:    chapterContent,
		})
	}

	return results
}

func (m *MarkdownLoader) getChapterContent(docTitle, headingPath string) string {
	var chapterContent strings.Builder

	for _, chunk := range m.chunkedDocs {
		if chunk.title != docTitle {
			continue
		}

		if strings.HasPrefix(chunk.headingPath, headingPath) ||
			strings.HasPrefix(headingPath, chunk.headingPath) ||
			chunk.headingPath == "" {
			if chapterContent.Len() > 0 {
				chapterContent.WriteString("\n\n")
			}
			chapterContent.WriteString(chunk.content)
		}
	}

	return chapterContent.String()
}

func tokenize(text string) []string {
	re := regexp.MustCompile(`[a-zA-Z0-9_\p{Han}]+`)
	return re.FindAllString(strings.ToLower(text), -1)
}

func (m *MarkdownLoader) GetDocumentCount() int {
	return len(m.documents)
}

func (m *MarkdownLoader) GetDocuments() []schema.Document {
	return m.documents
}

func (m *MarkdownLoader) AddDocuments(docs []schema.Document) {
	m.documents = append(m.documents, docs...)
	m.processDocuments()
	m.precomputeBM25Stats()
}
