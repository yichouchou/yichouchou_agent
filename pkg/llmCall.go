package pkg

import (
	"context"
	"fmt"
	"os"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

// LLMConfig holds LLM configuration
type LLMConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

// LLMClient wraps langchaingo LLM for MiniMax API
type LLMClient struct {
	client llms.LLM
	model  string
}

// NewLLMClient creates a new LLM client using langchaingo
func NewLLMClient(apiKey, baseURL, model string) (*LLMClient, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	if baseURL == "" {
		baseURL = "https://api.minimax.chat/v1"
	}

	if model == "" {
		model = "MiniMax-M2.7"
	}

	client, err := openai.New(
		openai.WithToken(apiKey),
		openai.WithBaseURL(baseURL),
		openai.WithModel(model),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	return &LLMClient{
		client: client,
		model:  model,
	}, nil
}

// Call sends a prompt to the LLM and returns the response using langchaingo
func (l *LLMClient) Call(ctx context.Context, prompt string) (string, error) {
	return l.client.Call(ctx, prompt)
}

// CallWithContext sends a prompt with custom context using langchaingo
func (l *LLMClient) CallWithContext(ctx context.Context, prompt string, context string) (string, error) {
	fullPrompt := prompt
	if context != "" {
		fullPrompt = fmt.Sprintf("请根据以下知识库内容回答问题。\n\n知识库内容:\n%s\n\n用户问题: %s", context, prompt)
	}
	return l.client.Call(ctx, fullPrompt)
}

// GetModel returns the model name
func (l *LLMClient) GetModel() string {
	return l.model
}

// InitLLM initializes the global LLM client from environment or config file
func InitLLM() (*LLMClient, error) {
	apiKey := GetMinimaxAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("MINIMAX_API_KEY is not set")
	}

	baseURL := os.Getenv("MINIMAX_BASE_URL")
	model := os.Getenv("MINIMAX_MODEL")
	if model == "" {
		model = "MiniMax-M2.7"
	}

	return NewLLMClient(apiKey, baseURL, model)
}
