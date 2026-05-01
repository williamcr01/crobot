package compaction

import (
	"context"
	"strings"
	"testing"

	"crobot/internal/config"
	"crobot/internal/provider"
)

func makeMessages(count int) []MessageItem {
	msgs := make([]MessageItem, count)
	for i := 0; i < count; i++ {
		if i%2 == 0 {
			msgs[i] = MessageItem{
				Role:    roleUser,
				Content: strings.Repeat("user message content ", 10),
			}
		} else {
			msgs[i] = MessageItem{
				Role:    roleAssistant,
				Content: strings.Repeat("assistant response content ", 20),
			}
		}
	}
	return msgs
}

func TestEstimateTokens(t *testing.T) {
	msg := MessageItem{
		Role:    roleUser,
		Content: strings.Repeat("a", 400),
	}
	tokens := estimateTokens(msg)
	if tokens != 100 {
		t.Errorf("expected 100 tokens for 400 chars, got %d", tokens)
	}
}

func TestEstimateTokensWithToolCalls(t *testing.T) {
	msg := MessageItem{
		Role:    roleAssistant,
		Content: "response",
		ToolCalls: []ToolRenderItem{
			{Args: strings.Repeat("x", 200), Output: strings.Repeat("y", 200)},
		},
	}
	tokens := estimateTokens(msg)
	// 8 (content) + 200 (args) + 200 (output) = 408 chars → 102 tokens
	expected := 102
	if tokens != expected {
		t.Errorf("expected %d tokens, got %d", expected, tokens)
	}
}

func TestEstimateContextTokens(t *testing.T) {
	msgs := makeMessages(10)
	tokens := estimateContextTokens(msgs)
	if tokens < 900 {
		t.Errorf("expected >900 tokens for 10 messages, got %d", tokens)
	}
}

func TestCanCompact_Empty(t *testing.T) {
	if CanCompact(nil) {
		t.Error("expected CanCompact false for nil")
	}
	if CanCompact([]MessageItem{}) {
		t.Error("expected CanCompact false for empty")
	}
}

func TestCanCompact_WithMessages(t *testing.T) {
	msgs := makeMessages(4)
	if !CanCompact(msgs) {
		t.Error("expected CanCompact true with messages")
	}
}

func TestCanCompact_AlreadyCompacted(t *testing.T) {
	msgs := []MessageItem{
		{Role: roleUser, Content: "hi"},
		{Role: "compaction", Content: "summary"},
	}
	if CanCompact(msgs) {
		t.Error("expected CanCompact false when already compacted")
	}
}

func TestFindCutPoint_SmallSession(t *testing.T) {
	msgs := makeMessages(4)
	// keepTokens is larger than the session → no cut point
	cut := findCutPoint(msgs, 100000)
	if cut != -1 {
		t.Errorf("expected -1 for small session, got %d", cut)
	}
}

func TestFindCutPoint_CutsAtUser(t *testing.T) {
	msgs := makeMessages(20)
	// Set keep low enough to trigger a cut
	cut := findCutPoint(msgs, 500)
	if cut <= 0 {
		t.Errorf("expected positive cut index, got %d", cut)
	}
	if msgs[cut].Role != roleUser {
		t.Errorf("expected user message at cut point, got %s", msgs[cut].Role)
	}
}

func TestFindCutPoint_Empty(t *testing.T) {
	cut := findCutPoint(nil, 500)
	if cut != 0 {
		t.Errorf("expected 0 for nil messages, got %d", cut)
	}
}

func TestShouldCompact_Disabled(t *testing.T) {
	msgs := makeMessages(500) // huge session
	settings := config.CompactionConfig{
		Enabled:          false,
		ReserveTokens:    16384,
		KeepRecentTokens: 20000,
	}
	if ShouldCompact(msgs, settings) {
		t.Error("expected no compaction when disabled")
	}
}

func TestShouldCompact_Enabled_Small(t *testing.T) {
	msgs := makeMessages(4)
	settings := config.CompactionConfig{
		Enabled:          true,
		ReserveTokens:    16384,
		KeepRecentTokens: 20000,
	}
	if ShouldCompact(msgs, settings) {
		t.Error("expected no compaction for small session")
	}
}

func TestShouldCompact_AlreadyCompacted(t *testing.T) {
	msgs := makeMessages(500)
	msgs = append(msgs, MessageItem{Role: "compaction", Content: "summary"})
	settings := config.CompactionConfig{
		Enabled:          true,
		ReserveTokens:    16384,
		KeepRecentTokens: 20000,
	}
	if ShouldCompact(msgs, settings) {
		t.Error("expected no compaction when already compacted")
	}
}

func TestNeedsCompaction(t *testing.T) {
	settings := config.CompactionConfig{
		Enabled:          true,
		ReserveTokens:    16384,
		KeepRecentTokens: 20000,
	}
	// 128k window - 16k reserve = 112k threshold
	if needsCompaction(50000, settings) {
		t.Error("expected false for 50000 tokens (below 112k)")
	}
	if !needsCompaction(120000, settings) {
		t.Error("expected true for 120000 tokens (above 112k)")
	}
}

func TestSerializeMessages(t *testing.T) {
	msgs := []MessageItem{
		{Role: roleUser, Content: "hello"},
		{Role: roleAssistant, Content: "hi there", Reasoning: "thinking..."},
		{Role: roleAssistant, Content: "done", ToolCalls: []ToolRenderItem{
			{CallID: "call_1", Args: `{"path":"foo.go"}`},
		}},
	}
	result := serializeMessages(msgs)
	if !strings.Contains(result, "[User]: hello") {
		t.Error("expected user message in serialized output")
	}
	if !strings.Contains(result, "[Assistant thinking]: thinking...") {
		t.Error("expected reasoning in serialized output")
	}
	if !strings.Contains(result, "[Assistant tool call]") {
		t.Error("expected tool call in serialized output")
	}
}

func TestSerializeMessages_ToolResultTruncation(t *testing.T) {
	longOutput := strings.Repeat("x", 3000)
	msgs := []MessageItem{
		{Role: "tool", Content: longOutput},
	}
	result := serializeMessages(msgs)
	if len(result) > 2500 {
		t.Errorf("expected truncated output, got %d chars", len(result))
	}
	if !strings.Contains(result, "truncated") {
		t.Error("expected truncation marker")
	}
}

func TestCompact_TooSmall(t *testing.T) {
	msgs := makeMessages(4)
	settings := config.CompactionConfig{
		Enabled:          true,
		ReserveTokens:    16384,
		KeepRecentTokens: 20000,
	}
	_, err := Compact(nil, nil, "", settings, msgs, "", "")
	if err == nil {
		t.Error("expected error for session too small")
	}
	if !strings.Contains(err.Error(), "too small") {
		t.Errorf("expected 'too small' error, got %v", err)
	}
}

// makeLargeMessages creates messages that serialize to at least charCount characters.
func makeLargeMessages(charCount int) []MessageItem {
	return []MessageItem{
		{Role: roleUser, Content: strings.Repeat("x ", charCount/2)},
	}
}

func TestCanModelHandleSummary_EmptyModel(t *testing.T) {
	msgs := makeMessages(10)
	err := canModelHandleSummary("openai", "", msgs)
	if err == nil {
		t.Error("expected error for empty model ID")
	}
	if !strings.Contains(err.Error(), "no model specified") {
		t.Errorf("expected 'no model specified' error, got %v", err)
	}
}

func TestCanModelHandleSummary_EnoughRoom(t *testing.T) {
	msgs := makeMessages(10)
	err := canModelHandleSummary("openai", "gpt-5", msgs)
	if err != nil {
		t.Errorf("expected no error for small messages with large model, got %v", err)
	}
}

func TestCanModelHandleSummary_TooSmall(t *testing.T) {
	// Create enough content to exceed a 128K model's 80% threshold (~102K tokens).
	// Need serialized chars > (102400 - 500) * 4 ≈ 407600
	msgs := makeLargeMessages(450000)
	err := canModelHandleSummary("openai", "gpt-4o-mini", msgs)
	if err == nil {
		t.Error("expected error for too much content on a 128K model")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected 'exceeds' in error, got %v", err)
	}
}

// fakeProvider is a minimal provider stub for testing fallback in Compact.
type fakeProvider struct {
	name string
	resp string
	err  error
}

func (f *fakeProvider) Name() string                                            { return f.name }
func (f *fakeProvider) Send(_ context.Context, _ provider.Request) (*provider.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &provider.Response{Text: f.resp}, nil
}
func (f *fakeProvider) Stream(_ context.Context, _ provider.Request) (<-chan provider.StreamEvent, error) { return nil, nil }
func (f *fakeProvider) ListModels(_ context.Context) ([]string, error)          { return nil, nil }

func TestCompact_MainModelTooSmall(t *testing.T) {
	// No separate compaction model, main model too small → error.
	// 3 messages: large (summarized) + assistant (summarized) + small recent (kept).
	prov := &fakeProvider{name: "openai", resp: "test summary"}
	msgs := []MessageItem{
		{Role: roleUser, Content: strings.Repeat("x ", 225000)}, // ~450K chars → ~112K tokens
		{Role: roleAssistant, Content: "intermediate response"},
		{Role: roleUser, Content: "recent question"},
	}
	settings := config.CompactionConfig{
		Enabled:          true,
		KeepRecentTokens: 1, // just keep the last user message
	}
	_, err := Compact(context.Background(), prov, "gpt-4o-mini", settings, msgs, "", "")
	if err == nil {
		t.Fatal("expected error when main model is too small")
	}
	if !strings.Contains(err.Error(), "cannot handle summarization") {
		t.Errorf("expected 'cannot handle summarization' error, got %v", err)
	}
}

func TestCompact_FallbackOnCompactionModel(t *testing.T) {
	// Compaction model too small (128K), main model big enough (400K) → falls back.
	prov := &fakeProvider{name: "openai", resp: "fallback summary"}
	msgs := []MessageItem{
		{Role: roleUser, Content: strings.Repeat("x ", 225000)}, // ~450K chars → ~112K tokens
		{Role: roleAssistant, Content: "intermediate response"},
		{Role: roleUser, Content: "recent question"},
	}
	settings := config.CompactionConfig{
		Enabled:          true,
		KeepRecentTokens: 1,
		Model:            "gpt-4o-mini", // 128K context — too small
	}
	result, err := Compact(context.Background(), prov, "gpt-5", settings, msgs, "", "")
	if err != nil {
		t.Fatalf("expected fallback to succeed, got %v", err)
	}
	if !strings.Contains(result.Summary, "fallback summary") {
		t.Errorf("expected fallback summary, got %q", result.Summary)
	}
}

func TestCompact_BothModelsTooSmall_SameID(t *testing.T) {
	// Both compaction model and main model are the same ID and too small → error.
	prov := &fakeProvider{name: "openai", resp: "test"}
	msgs := []MessageItem{
		{Role: roleUser, Content: strings.Repeat("x ", 225000)}, // ~450K chars → ~112K tokens
		{Role: roleAssistant, Content: "intermediate response"},
		{Role: roleUser, Content: "recent question"},
	}
	settings := config.CompactionConfig{
		Enabled:          true,
		KeepRecentTokens: 1,
		Model:            "gpt-4o-mini",
	}
	_, err := Compact(context.Background(), prov, "gpt-4o-mini", settings, msgs, "", "")
	if err == nil {
		t.Fatal("expected error when models too small")
	}
	// settings.Model == model (same ID), so the simpler error path is taken.
	if !strings.Contains(err.Error(), "cannot handle summarization") {
		t.Errorf("expected 'cannot handle summarization' error, got %v", err)
	}
}

func TestCompact_BothModelsTooSmall_DifferentIDs(t *testing.T) {
	// Compaction model and main model are different IDs, both too small → dual-model error.
	prov := &fakeProvider{name: "openai", resp: "test"}
	// Use an unknown model that defaults to 128K context.
	msgs := []MessageItem{
		{Role: roleUser, Content: strings.Repeat("x ", 225000)}, // ~450K chars → ~112K tokens
		{Role: roleAssistant, Content: "intermediate response"},
		{Role: roleUser, Content: "recent question"},
	}
	settings := config.CompactionConfig{
		Enabled:          true,
		KeepRecentTokens: 1,
		Model:            "some-vendor/tiny-model", // unknown → 128K — too small
	}
	_, err := Compact(context.Background(), prov, "other-vendor/other-model", settings, msgs, "", "")
	if err == nil {
		t.Fatal("expected error when both models too small")
	}
	if !strings.Contains(err.Error(), "compaction model") || !strings.Contains(err.Error(), "main model") {
		t.Errorf("expected error mentioning both models, got %v", err)
	}
}

func TestBuildMessagesForAgent_CompactionRole(t *testing.T) {
	msgs := []MessageItem{
		{Role: "compaction", Content: "summary text"},
		{Role: roleUser, Content: "recent message"},
	}
	result := buildMessagesForAgent(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Role != roleSystem {
		t.Errorf("expected compaction role to become system, got %s", result[0].Role)
	}
	if result[1].Role != roleUser {
		t.Errorf("expected user role preserved, got %s", result[1].Role)
	}
}
