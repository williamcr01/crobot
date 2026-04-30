package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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

func (p *OpenAIProvider) ListModelInfo(ctx context.Context) ([]ModelInfo, error) {
	if p.name != "openrouter" {
		ids, err := p.ListModels(ctx)
		if err != nil {
			return nil, err
		}
		models := make([]ModelInfo, 0, len(ids))
		for _, id := range ids {
			models = append(models, ModelInfo{ID: id})
		}
		return models, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openRouterBaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter list models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openrouter list models: %s", resp.Status)
	}

	var payload struct {
		Data []struct {
			ID            string `json:"id"`
			ContextLength int    `json:"context_length"`
			Pricing       struct {
				Prompt          string `json:"prompt"`
				Completion      string `json:"completion"`
				InputCacheRead  string `json:"input_cache_read"`
				InputCacheWrite string `json:"input_cache_write"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("openrouter list models decode: %w", err)
	}

	models := make([]ModelInfo, 0, len(payload.Data))
	for _, item := range payload.Data {
		if item.ID == "" {
			continue
		}
		models = append(models, ModelInfo{
			ID:            item.ID,
			ContextLength: item.ContextLength,
			Pricing: Pricing{
				InputPerMTok:      openRouterPricePerMillion(item.Pricing.Prompt),
				OutputPerMTok:     openRouterPricePerMillion(item.Pricing.Completion),
				CacheReadPerMTok:  openRouterPricePerMillion(item.Pricing.InputCacheRead),
				CacheWritePerMTok: openRouterPricePerMillion(item.Pricing.InputCacheWrite),
			},
		})
	}
	return models, nil
}

func openRouterPricePerMillion(raw string) float64 {
	price, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return price * 1_000_000
}
