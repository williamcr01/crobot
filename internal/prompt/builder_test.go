package prompt

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"crobot/internal/config"
)

func TestBuild_Defaults(t *testing.T) {
	cfg := config.DEFAULTS
	cwd := "/home/test/project"
	result := Build(cfg, cwd)

	if !strings.Contains(result, cwd) {
		t.Error("result should contain cwd")
	}
	if strings.Contains(result, "{cwd}") {
		t.Error("result should not contain {cwd} placeholder")
	}
}

func TestBuild_ContainsDynamicContext(t *testing.T) {
	cfg := config.DEFAULTS
	result := Build(cfg, "/tmp")

	if !strings.Contains(result, "Current date:") {
		t.Error("result should contain current date")
	}
	if !strings.Contains(result, "Platform:") {
		t.Error("result should contain platform info")
	}
	if !strings.Contains(result, runtime.GOOS+"/"+runtime.GOARCH) {
		t.Error("result should contain runtime info")
	}
	if !strings.Contains(result, "Shell:") {
		t.Error("result should contain shell info")
	}
}

func TestBuild_ShellEnvVar(t *testing.T) {
	cfg := config.DEFAULTS
	oldShell := os.Getenv("SHELL")
	defer os.Setenv("SHELL", oldShell)
	os.Setenv("SHELL", "/bin/zsh")

	result := Build(cfg, "/tmp")
	if !strings.Contains(result, "/bin/zsh") {
		t.Error("result should contain the SHELL env var value")
	}
}

func TestBuild_CustomPrompt(t *testing.T) {
	cfg := config.DEFAULTS
	cfg.SystemPrompt = "Custom prompt for {cwd}"
	result := Build(cfg, "/my/dir")

	if !strings.Contains(result, "Custom prompt for /my/dir") {
		t.Errorf("expected custom prompt, got: %s", result)
	}
}

func TestBuild_EmptyPrompt(t *testing.T) {
	cfg := config.AgentConfig{
		SystemPrompt: "",
		Display:      config.DEFAULTS.Display,
	}
	result := Build(cfg, "/tmp")
	if !strings.Contains(result, "coding assistant") {
		t.Error("empty prompt should use default")
	}
}

func TestBuild_TrailingContent(t *testing.T) {
	cfg := config.DEFAULTS
	result := Build(cfg, "/tmp")
	lines := strings.Split(strings.TrimSpace(result), "\n")

	// Last non-empty line should be platform info.
	lastContent := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			lastContent = strings.TrimSpace(lines[i])
			break
		}
	}
	if lastContent != "Platform: "+runtime.GOOS+"/"+runtime.GOARCH {
		t.Errorf("expected trailing platform line, got %q", lastContent)
	}
}
