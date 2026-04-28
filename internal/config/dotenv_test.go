package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_WithDotEnvFile(t *testing.T) {
	withTempHome(t)
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)
	os.Unsetenv("OPENROUTER_API_KEY")

	if err := os.WriteFile(".env", []byte("OPENROUTER_API_KEY=sk-or-v1-from-dotenv"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := os.Getenv("OPENROUTER_API_KEY"); got != "sk-or-v1-from-dotenv" {
		t.Errorf("expected key from .env, got %s", got)
	}
}

func TestLoadConfig_DotEnvEnvOverride(t *testing.T) {
	withTempHome(t)
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-env-override")
	defer os.Unsetenv("OPENROUTER_API_KEY")
	if err := os.WriteFile(".env", []byte("OPENROUTER_API_KEY=sk-or-v1-dotenv"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := os.Getenv("OPENROUTER_API_KEY"); got != "sk-or-v1-env-override" {
		t.Errorf("expected env override, got %s", got)
	}
}

func TestLoadConfig_MissingDotEnv(t *testing.T) {
	withTempHome(t)
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)
	os.Unsetenv("OPENROUTER_API_KEY")

	if _, err := LoadConfig(); err != nil {
		t.Fatalf("missing .env should not fail config load: %v", err)
	}
}

func TestLoadConfig_DotEnvIgnoresComments(t *testing.T) {
	withTempHome(t)
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)
	os.Unsetenv("OPENROUTER_API_KEY")

	content := "# this is a comment\n#OPENROUTER_API_KEY=sk-commented\n\nOPENROUTER_API_KEY=sk-or-v1-after-comment\n"
	if err := os.WriteFile(".env", []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := os.Getenv("OPENROUTER_API_KEY"); got != "sk-or-v1-after-comment" {
		t.Errorf("expected key after comment, got %s", got)
	}
}

func TestLoadConfig_DotEnvFilepath(t *testing.T) {
	withTempHome(t)
	dir := t.TempDir()
	child := filepath.Join(dir, "subdir")
	os.MkdirAll(child, 0o755)
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("OPENROUTER_API_KEY=sk-or-v1-parent"), 0o644); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	os.Chdir(child)
	defer os.Chdir(origWd)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Unsetenv("OPENROUTER_API_KEY")

	if _, err := LoadConfig(); err != nil {
		t.Fatalf("child dir without .env should still load config: %v", err)
	}
	if got := os.Getenv("OPENROUTER_API_KEY"); got != "" {
		t.Errorf("expected cwd-only .env lookup, got %s", got)
	}
}
