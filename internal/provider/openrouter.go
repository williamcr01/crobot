package provider

import (
	"fmt"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func init() {
	Register("openrouter", NewOpenRouter)
}

const openRouterBaseURL = "https://openrouter.ai/api/v1"

// NewOpenRouter creates an OpenRouter provider using OpenRouter's OpenAI-compatible API.
func NewOpenRouter(apiKey string) (Provider, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("openrouter: missing API key")
	}
	return &OpenAIProvider{
		name:   "openrouter",
		apiKey: apiKey,
		client: openai.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(openRouterBaseURL),
		),
	}, nil
}
