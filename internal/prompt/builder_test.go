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
		t.Error("result should contain bash info")
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
		Reasoning:    config.DEFAULTS.Reasoning,
	}
	result := Build(cfg, "/tmp")
	if !strings.Contains(result, "You are Crobot") {
		t.Error("empty prompt should use default")
	}
}

func TestBuild_AppendPrompt(t *testing.T) {
	cfg := config.DEFAULTS
	cfg.AppendPrompt = "Extra instructions for {cwd}"
	result := Build(cfg, "/my/dir")

	if !strings.Contains(result, "You are Crobot") {
		t.Error("appendPrompt should keep the base prompt")
	}
	if !strings.Contains(result, "Extra instructions for /my/dir") {
		t.Errorf("expected appended prompt, got: %s", result)
	}
}

func TestBuild_CustomPromptWithAppendPrompt(t *testing.T) {
	cfg := config.DEFAULTS
	cfg.SystemPrompt = "Custom base"
	cfg.AppendPrompt = "Custom append"
	result := Build(cfg, "/tmp")

	if !strings.Contains(result, "Custom base\n\nCustom append") {
		t.Errorf("expected appendPrompt after custom systemPrompt, got: %s", result)
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

func TestBuild_AppendsAgentsFilesFromAncestors(t *testing.T) {
	root := t.TempDir()
	child := root + string(os.PathSeparator) + "pkg" + string(os.PathSeparator) + "subpkg"
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+string(os.PathSeparator)+"AGENTS.md", []byte("root rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg := root + string(os.PathSeparator) + "pkg"
	if err := os.WriteFile(pkg+string(os.PathSeparator)+"AGENT.md", []byte("pkg rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(child+string(os.PathSeparator)+"AGENTS.md", []byte("child rules"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := Build(config.DEFAULTS, child)

	if !strings.Contains(result, "# Project Context") {
		t.Error("result should include project context section")
	}
	rootIdx := strings.Index(result, "root rules")
	pkgIdx := strings.Index(result, "pkg rules")
	childIdx := strings.Index(result, "child rules")
	if rootIdx == -1 || pkgIdx == -1 || childIdx == -1 {
		t.Fatalf("expected all context files in prompt, got: %s", result)
	}
	if !(rootIdx < pkgIdx && pkgIdx < childIdx) {
		t.Fatalf("expected root-to-cwd context ordering, got indexes root=%d pkg=%d child=%d", rootIdx, pkgIdx, childIdx)
	}
}

func TestBuild_PrefersAgentsOverAgentInSameDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+string(os.PathSeparator)+"AGENTS.md", []byte("agents rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+string(os.PathSeparator)+"AGENT.md", []byte("agent rules"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := Build(config.DEFAULTS, dir)
	if !strings.Contains(result, "agents rules") {
		t.Error("result should include AGENTS.md")
	}
	if strings.Contains(result, "agent rules") {
		t.Error("result should prefer AGENTS.md over AGENT.md in the same directory")
	}
}
