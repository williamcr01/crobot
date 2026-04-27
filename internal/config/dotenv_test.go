package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_WithDotEnvFile(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	// Unset so it doesn't interfere.
	os.Unsetenv("OPENROUTER_API_KEY")

	// Write .env file.
	if err := os.WriteFile(".env", []byte("OPENROUTER_API_KEY=sk-or-v1-from-dotenv"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "sk-or-v1-from-dotenv" {
		t.Errorf("expected key from .env, got %s", cfg.APIKey)
	}
}

func TestLoadConfig_DotEnvEnvOverride(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	os.Setenv("OPENROUTER_API_KEY", "sk-or-v1-env-override")
	defer os.Unsetenv("OPENROUTER_API_KEY")

	// .env with different value should NOT override existing env var.
	if err := os.WriteFile(".env", []byte("OPENROUTER_API_KEY=sk-or-v1-dotenv"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "sk-or-v1-env-override" {
		t.Errorf("expected env override, got %s", cfg.APIKey)
	}
}

func TestLoadConfig_MissingDotEnv(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	os.Unsetenv("OPENROUTER_API_KEY")

	// No .env file, no env var -> should fail.
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error when no key available")
	}
}

func TestLoadConfig_DotEnvIgnoresComments(t *testing.T) {
	dir := t.TempDir()
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	os.Unsetenv("OPENROUTER_API_KEY")

	content := "# this is a comment\n#OPENROUTER_API_KEY=sk-commented\n\nOPENROUTER_API_KEY=sk-or-v1-after-comment\n"
	if err := os.WriteFile(".env", []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "sk-or-v1-after-comment" {
		t.Errorf("expected key after comment, got %s", cfg.APIKey)
	}
}

func TestLoadConfig_DotEnvFilepath(t *testing.T) {
	// Verify that the config reads .env from cwd, not from some other path.
	dir := t.TempDir()
	child := filepath.Join(dir, "subdir")
	os.MkdirAll(child, 0o755)

	// Write .env in parent, not in child.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("OPENROUTER_API_KEY=sk-or-v1-parent"), 0o644); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	os.Chdir(child)
	defer os.Chdir(origWd)
	defer os.Unsetenv("OPENROUTER_API_KEY")
	os.Unsetenv("OPENROUTER_API_KEY")

	// Child dir has no .env, parent does. Should error since cwd is child.
	_, err := LoadConfig()
	if err == nil {
		t.Error("expected error: child dir has no .env and no env var")
	}
}
