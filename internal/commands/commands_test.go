package commands

import (
	"errors"
	"strings"
	"testing"
)

func TestParse_Valid(t *testing.T) {
	cmd, args, ok := Parse("/model gpt-4")
	if !ok {
		t.Fatal("expected ok")
	}
	if cmd != "model" {
		t.Errorf("expected 'model', got %s", cmd)
	}
	if len(args) != 1 || args[0] != "gpt-4" {
		t.Errorf("expected ['gpt-4'], got %v", args)
	}
}

func TestParse_MultipleArgs(t *testing.T) {
	cmd, args, ok := Parse("/export /tmp/out.md")
	if !ok {
		t.Fatal("expected ok")
	}
	if cmd != "export" {
		t.Errorf("expected 'export', got %s", cmd)
	}
	if len(args) != 1 || args[0] != "/tmp/out.md" {
		t.Errorf("expected ['/tmp/out.md'], got %v", args)
	}
}

func TestParse_NoArgs(t *testing.T) {
	cmd, args, ok := Parse("/help")
	if !ok {
		t.Fatal("expected ok")
	}
	if cmd != "help" {
		t.Errorf("expected 'help', got %s", cmd)
	}
	if len(args) != 0 {
		t.Errorf("expected no args, got %v", args)
	}
}

func TestParse_NotACommand(t *testing.T) {
	_, _, ok := Parse("hello world")
	if ok {
		t.Fatal("expected not ok")
	}
}

func TestParse_Empty(t *testing.T) {
	_, _, ok := Parse("")
	if ok {
		t.Fatal("expected not ok for empty input")
	}
}

func TestParse_JustSlash(t *testing.T) {
	_, _, ok := Parse("/")
	if ok {
		t.Fatal("expected not ok for just slash")
	}
}

func TestParse_LeadingSpaces(t *testing.T) {
	cmd, _, ok := Parse("  /new")
	if !ok {
		t.Fatal("expected ok")
	}
	if cmd != "new" {
		t.Errorf("expected 'new', got %s", cmd)
	}
}

func TestRegisterAndExecute(t *testing.T) {
	reg := NewRegistry()
	reg.Register(Command{
		Name:        "echo",
		Description: "Echoes back args",
		Args:        "<message>",
		Handler: func(args []string) (string, error) {
			return strings.Join(args, " "), nil
		},
	})

	result, err := reg.Execute("/echo hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Execute("/nonexistent")
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("expected unknown command error, got %v", err)
	}
}

func TestExecute_NotCommand(t *testing.T) {
	reg := NewRegistry()
	result, err := reg.Execute("just a message")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
}

func TestHelpText(t *testing.T) {
	reg := NewRegistry()
	reg.Register(Command{Name: "model", Description: "Switch model", Args: "<name>"})
	reg.Register(Command{Name: "help", Description: "Show help"})

	help := reg.HelpText()
	if !strings.Contains(help, "/help") {
		t.Error("help should contain /help")
	}
	if !strings.Contains(help, "/model") {
		t.Error("help should contain /model")
	}
	if !strings.Contains(help, "Switch model") {
		t.Error("help should contain descriptions")
	}
	if !strings.Contains(help, "<name>") {
		t.Error("help should contain args")
	}
	if strings.Index(help, "/help") > strings.Index(help, "/model") {
		t.Error("help commands should be sorted")
	}
}

func TestSuggestions(t *testing.T) {
	reg := NewRegistry()
	reg.Register(Command{Name: "model", Description: "Switch model", Args: "<name>"})
	reg.Register(Command{Name: "help", Description: "Show help"})
	reg.Register(Command{Name: "new", Description: "New session"})

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "slash shows all", input: "/", want: []string{"help", "model", "new"}},
		{name: "prefix filters", input: "/mo", want: []string{"model"}},
		{name: "leading spaces allowed", input: "  /h", want: []string{"help"}},
		{name: "no matches", input: "/x", want: []string{}},
		{name: "hide after args", input: "/model gpt-4", want: nil},
		{name: "not command", input: "hello /mo", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reg.Suggestions(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("expected %d suggestions, got %d", len(tt.want), len(got))
			}
			for i, want := range tt.want {
				if got[i].Name != want {
					t.Errorf("suggestion %d: expected %q, got %q", i, want, got[i].Name)
				}
			}
		})
	}
}

func TestHandlerError(t *testing.T) {
	reg := NewRegistry()
	reg.Register(Command{
		Name: "fail",
		Handler: func(args []string) (string, error) {
			return "", errors.New("something went wrong")
		},
	})

	_, err := reg.Execute("/fail")
	if err == nil || !strings.Contains(err.Error(), "something went wrong") {
		t.Errorf("expected handler error, got %v", err)
	}
}

// stubModelRegistry is a test stub implementing ModelRegistry.
type stubModelRegistry struct {
	models []ModelInfo
}

func (s *stubModelRegistry) GetAll() []ModelInfo { return s.models }

func (s *stubModelRegistry) Filter(prefix string) []ModelInfo {
	if prefix == "" {
		return s.models
	}
	var res []ModelInfo
	for _, m := range s.models {
		if strings.Contains(strings.ToLower(m.ID), strings.ToLower(prefix)) ||
			strings.Contains(strings.ToLower(m.Provider), strings.ToLower(prefix)) {
			res = append(res, m)
		}
	}
	return res
}

func TestModelSuggestions_NoRegistry(t *testing.T) {
	reg := NewRegistry()
	suggestions := reg.FilterModels("")
	if suggestions != nil {
		t.Fatalf("expected nil suggestions without model registry, got %v", suggestions)
	}
}

func TestModelSuggestions_AllModels(t *testing.T) {
	reg := NewRegistry()
	reg.SetModelRegistry(&stubModelRegistry{
		models: []ModelInfo{
			{ID: "openai/gpt-4o", Provider: "openrouter"},
			{ID: "anthropic/claude-opus-4-7", Provider: "openrouter"},
		},
	})

	suggestions := reg.FilterModels("")
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 model suggestions, got %d", len(suggestions))
	}
	if suggestions[0].ModelID != "openai/gpt-4o" {
		t.Errorf("expected ModelID openai/gpt-4o, got %q", suggestions[0].ModelID)
	}
	if suggestions[0].Args != "openrouter" {
		t.Errorf("expected Args openrouter, got %q", suggestions[0].Args)
	}
}

func TestModelSuggestions_Filtered(t *testing.T) {
	reg := NewRegistry()
	reg.SetModelRegistry(&stubModelRegistry{
		models: []ModelInfo{
			{ID: "openai/gpt-4o", Provider: "openrouter"},
			{ID: "anthropic/claude-opus-4-7", Provider: "openrouter"},
			{ID: "anthropic/claude-sonnet-4", Provider: "openrouter"},
		},
	})

	suggestions := reg.FilterModels("anthropic")
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 matching model suggestions, got %d", len(suggestions))
	}
	for _, s := range suggestions {
		if !strings.HasPrefix(s.ModelID, "anthropic/") {
			t.Errorf("expected anthropic model, got %q", s.ModelID)
		}
	}
}

func TestModelSuggestions_NoMatch(t *testing.T) {
	reg := NewRegistry()
	reg.SetModelRegistry(&stubModelRegistry{
		models: []ModelInfo{
			{ID: "openai/gpt-4o", Provider: "openrouter"},
		},
	})

	suggestions := reg.FilterModels("nonexistent")
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions, got %d", len(suggestions))
	}
}

func TestModelSuggestions_PreservesCommandSuggestions(t *testing.T) {
	reg := NewRegistry()
	reg.Register(Command{Name: "help", Description: "Show help"})
	reg.Register(Command{Name: "model", Description: "Switch model"})
	reg.SetModelRegistry(&stubModelRegistry{
		models: []ModelInfo{
			{ID: "openai/gpt-4o", Provider: "openrouter"},
		},
	})

	// FilterModels returns model results
	suggestions := reg.FilterModels("")
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 model suggestion, got %d", len(suggestions))
	}

	// /model shows as a command suggestion (not a model suggestion)
	suggestions = reg.Suggestions("/mo")
	if len(suggestions) != 1 || suggestions[0].Name != "model" {
		t.Fatalf("expected model command suggestion, got %v", suggestions)
	}

	// Other commands still work normally
	suggestions = reg.Suggestions("/h")
	if len(suggestions) != 1 || suggestions[0].Name != "help" {
		t.Fatalf("expected help command suggestion, got %v", suggestions)
	}
}

func TestFilterModels_RecentFirst(t *testing.T) {
	reg := NewRegistry()
	reg.SetModelRegistry(&stubModelRegistry{
		models: []ModelInfo{
			{ID: "model-c", Provider: "test"},
			{ID: "model-b", Provider: "test"},
			{ID: "model-a", Provider: "test"},
		},
	})

	history := NewModelHistory(t.TempDir())
	history.Record("test", "model-b")
	history.Record("test", "model-c")
	reg.SetModelHistory(history)

	suggestions := reg.FilterModels("")
	if len(suggestions) != 3 {
		t.Fatalf("expected 3 suggestions, got %d", len(suggestions))
	}

	// model-c was most recently used, then model-b, then model-a (not in history)
	if suggestions[0].ModelID != "model-c" {
		t.Fatalf("expected model-c first (most recent), got %q", suggestions[0].ModelID)
	}
	if suggestions[1].ModelID != "model-b" {
		t.Fatalf("expected model-b second, got %q", suggestions[1].ModelID)
	}
	if suggestions[2].ModelID != "model-a" {
		t.Fatalf("expected model-a last (not in history), got %q", suggestions[2].ModelID)
	}
}

func TestFilterModels_NoHistory(t *testing.T) {
	reg := NewRegistry()
	reg.SetModelRegistry(&stubModelRegistry{
		models: []ModelInfo{
			{ID: "model-b", Provider: "test"},
			{ID: "model-a", Provider: "test"},
		},
	})

	// Without history, original order is preserved.
	suggestions := reg.FilterModels("")
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(suggestions))
	}
	if suggestions[0].ModelID != "model-b" {
		t.Fatalf("expected model-b first (original order), got %q", suggestions[0].ModelID)
	}
}

func TestFilterModels_WithFilterIgnoresHistory(t *testing.T) {
	reg := NewRegistry()
	reg.SetModelRegistry(&stubModelRegistry{
		models: []ModelInfo{
			{ID: "model-a", Provider: "test"},
			{ID: "model-b", Provider: "test"},
		},
	})

	history := NewModelHistory(t.TempDir())
	history.Record("test", "model-b")
	reg.SetModelHistory(history)

	// When a filter is active, sorting by recency is skipped.
	suggestions := reg.FilterModels("a")
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].ModelID != "model-a" {
		t.Fatalf("expected model-a (matching filter), got %q", suggestions[0].ModelID)
	}
}

func TestStableSortByRecency(t *testing.T) {
	history := NewModelHistory(t.TempDir())
	history.Record("test", "model-c")
	history.Record("test", "model-a")

	cmds := []Command{
		{ModelID: "model-a", ModelProvider: "test"},
		{ModelID: "model-b", ModelProvider: "test"},
		{ModelID: "model-c", ModelProvider: "test"},
	}

	stableSortByRecency(cmds, history)

	// model-a has recency 0 (most recent), model-c has recency 1, model-b has recency -1
	if cmds[0].ModelID != "model-a" {
		t.Fatalf("expected model-a first, got %q", cmds[0].ModelID)
	}
	if cmds[1].ModelID != "model-c" {
		t.Fatalf("expected model-c second, got %q", cmds[1].ModelID)
	}
	if cmds[2].ModelID != "model-b" {
		t.Fatalf("expected model-b last, got %q", cmds[2].ModelID)
	}
}

func TestStableSortByRecency_Empty(t *testing.T) {
	// Should not panic with nil or empty slice.
	history := NewModelHistory(t.TempDir())
	stableSortByRecency(nil, history)
	stableSortByRecency([]Command{}, history)
}

func TestRecencyBefore(t *testing.T) {
	tests := []struct {
		a, b int
		want bool
	}{
		{0, 1, true},        // more recent (0) before less recent (1)
		{1, 0, false},       // less recent (1) not before more recent (0)
		{0, -1, true},       // recent before unknown
		{-1, 0, false},      // unknown not before recent
		{-1, -1, false},     // unknown preserves order
		{2, 3, true},        // lower recency index = more recent
		{3, 2, false},       // higher recency index = less recent
	}

	for _, tt := range tests {
		got := recencyBefore(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("recencyBefore(%d, %d) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
