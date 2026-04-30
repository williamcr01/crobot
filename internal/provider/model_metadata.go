package provider

import "strings"

// ContextWindowForModel returns the best known context window for a provider/model.
// This mirrors pi-mono's approach of keeping contextWindow as model metadata, with
// a conservative fallback when a model is unknown.
func ContextWindowForModel(providerName, modelID string) int {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	model := strings.ToLower(strings.TrimSpace(modelID))
	if model == "" {
		return 128_000
	}

	if context, ok := knownContextWindows[providerName+"/"+model]; ok {
		return context
	}
	if context, ok := knownContextWindows[model]; ok {
		return context
	}

	// OpenRouter model IDs include the upstream provider prefix. Match families
	// after that prefix so switching between OpenRouter models updates instantly
	// even before live model metadata is available.
	switch {
	case strings.Contains(model, "gemini-1.5-pro") || strings.Contains(model, "gemini-2.5-pro") || strings.Contains(model, "gemini-3"):
		return 1_000_000
	case strings.Contains(model, "gemini"):
		return 1_000_000
	case strings.Contains(model, "claude-opus-4.7") || strings.Contains(model, "claude-opus-4-7"):
		return 1_000_000
	case strings.Contains(model, "claude-sonnet-4.6") || strings.Contains(model, "claude-sonnet-4-6"):
		return 1_000_000
	case strings.Contains(model, "claude-sonnet-4.5") || strings.Contains(model, "claude-sonnet-4-5"):
		return 200_000
	case strings.Contains(model, "claude"):
		return 200_000
	case strings.Contains(model, "gpt-5.4") || strings.Contains(model, "gpt-5.5"):
		return 272_000
	case strings.Contains(model, "gpt-4.1"):
		return 1_000_000
	case strings.Contains(model, "gpt-5"):
		return 400_000
	case strings.Contains(model, "o1") || strings.Contains(model, "o3") || strings.Contains(model, "o4"):
		return 200_000
	case strings.Contains(model, "gpt-4o") || strings.Contains(model, "gpt-4-turbo"):
		return 128_000
	case strings.Contains(model, "deepseek-v4") || strings.Contains(model, "deepseek-r1"):
		return 1_000_000
	case strings.Contains(model, "deepseek"):
		return 128_000
	default:
		return 128_000
	}
}

var knownContextWindows = map[string]int{
	"openai/gpt-4.1":       1_000_000,
	"openai/gpt-4.1-mini":  1_000_000,
	"openai/gpt-4.1-nano":  1_000_000,
	"openai/gpt-4o":        128_000,
	"openai/gpt-4o-mini":   128_000,
	"openai/gpt-5":         400_000,
	"openai/gpt-5-mini":    400_000,
	"openai/gpt-5-nano":    400_000,
	"openai/gpt-5.4":       272_000,
	"openai/gpt-5.5":       272_000,
	"anthropic/claude-opus-4-7":    1_000_000,
	"anthropic/claude-opus-4.7":    1_000_000,
	"anthropic/claude-sonnet-4-6":  1_000_000,
	"anthropic/claude-sonnet-4.6":  1_000_000,
	"anthropic/claude-sonnet-4-5":  200_000,
	"anthropic/claude-sonnet-4.5":  200_000,
	"anthropic/claude-3-5-sonnet":  200_000,
	"anthropic/claude-3.5-sonnet":  200_000,
	"anthropic/claude-3-7-sonnet":  200_000,
	"anthropic/claude-3.7-sonnet":  200_000,
	"google/gemini-2.5-pro":        1_000_000,
	"google/gemini-2.5-flash":      1_000_000,
	"google/gemini-3-pro":          1_000_000,
	"deepseek/deepseek-v4-pro":     1_000_000,
	"deepseek/deepseek-v4-flash":   1_000_000,
	"deepseek-v4-pro":              1_000_000,
	"deepseek-v4-flash":            1_000_000,
}
