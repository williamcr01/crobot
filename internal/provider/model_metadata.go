package provider

import "strings"

// IsSubscriptionProvider reports providers where token usage is covered by a subscription.
func IsSubscriptionProvider(providerName string) bool {
	return strings.EqualFold(strings.TrimSpace(providerName), "openai-codex")
}

// CalculateCost fills usage.Cost from pricing in USD per million tokens.
func CalculateCost(usage *Usage, pricing Pricing, subscription bool) {
	if usage == nil {
		return
	}
	usage.Subscription = subscription
	if subscription {
		usage.Cost = Cost{}
		return
	}
	usage.Cost.Input = pricing.InputPerMTok * float64(usage.InputTokens) / 1_000_000
	usage.Cost.Output = pricing.OutputPerMTok * float64(usage.OutputTokens) / 1_000_000
	usage.Cost.CacheRead = pricing.CacheReadPerMTok * float64(usage.CacheReadTokens) / 1_000_000
	usage.Cost.CacheWrite = pricing.CacheWritePerMTok * float64(usage.CacheWriteTokens) / 1_000_000
	usage.Cost.Total = usage.Cost.Input + usage.Cost.Output + usage.Cost.CacheRead + usage.Cost.CacheWrite
}

// PricingForModel returns best-known model pricing in USD per million tokens.
func PricingForModel(providerName, modelID string) Pricing {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	model := strings.ToLower(strings.TrimSpace(modelID))
	if pricing, ok := knownPricing[providerName+"/"+model]; ok {
		return pricing
	}
	if pricing, ok := knownPricing[model]; ok {
		return pricing
	}
	return Pricing{}
}

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

var knownPricing = map[string]Pricing{
	"openai/gpt-4o":       {InputPerMTok: 2.5, OutputPerMTok: 10},
	"openai/gpt-4o-mini":  {InputPerMTok: 0.15, OutputPerMTok: 0.6},
	"openai/gpt-4.1":      {InputPerMTok: 2, OutputPerMTok: 8},
	"openai/gpt-4.1-mini": {InputPerMTok: 0.4, OutputPerMTok: 1.6},
	"openai/gpt-4.1-nano": {InputPerMTok: 0.1, OutputPerMTok: 0.4},
	"openai/gpt-5":        {InputPerMTok: 1.25, OutputPerMTok: 10, CacheReadPerMTok: 0.125},
	"openai/gpt-5-mini":   {InputPerMTok: 0.25, OutputPerMTok: 2, CacheReadPerMTok: 0.025},
	"openai/gpt-5-nano":   {InputPerMTok: 0.05, OutputPerMTok: 0.4, CacheReadPerMTok: 0.005},
	"anthropic/claude-sonnet-4-5": {InputPerMTok: 3, OutputPerMTok: 15, CacheReadPerMTok: 0.3, CacheWritePerMTok: 3.75},
	"anthropic/claude-sonnet-4.5": {InputPerMTok: 3, OutputPerMTok: 15, CacheReadPerMTok: 0.3, CacheWritePerMTok: 3.75},
	"anthropic/claude-opus-4-7":   {InputPerMTok: 5, OutputPerMTok: 25, CacheReadPerMTok: 0.5, CacheWritePerMTok: 6.25},
	"anthropic/claude-opus-4.7":   {InputPerMTok: 5, OutputPerMTok: 25, CacheReadPerMTok: 0.5, CacheWritePerMTok: 6.25},
	"google/gemini-2.5-pro":       {InputPerMTok: 1.25, OutputPerMTok: 10, CacheReadPerMTok: 0.125},
	"google/gemini-2.5-flash":     {InputPerMTok: 0.075, OutputPerMTok: 0.3, CacheReadPerMTok: 0.01875},
	"deepseek/deepseek-v4-pro":    {InputPerMTok: 0.14, OutputPerMTok: 0.28, CacheReadPerMTok: 0.0028},
	"deepseek/deepseek-v4-flash":  {InputPerMTok: 0.435, OutputPerMTok: 0.87, CacheReadPerMTok: 0.003625},
	"deepseek-v4-pro":             {InputPerMTok: 0.14, OutputPerMTok: 0.28, CacheReadPerMTok: 0.0028},
	"deepseek-v4-flash":           {InputPerMTok: 0.435, OutputPerMTok: 0.87, CacheReadPerMTok: 0.003625},
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
