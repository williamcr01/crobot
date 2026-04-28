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
