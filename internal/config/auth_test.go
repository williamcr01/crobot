package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureAuthCreatesEmptyAuthFile(t *testing.T) {
	home := withTempHome(t)

	if err := EnsureAuth(); err != nil {
		t.Fatalf("EnsureAuth: %v", err)
	}
	path := filepath.Join(home, ".crobot", "auth.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "{}" {
		t.Fatalf("expected empty auth object, got %s", string(data))
	}
}

func TestLoadAuthAPIKeyProvider(t *testing.T) {
	withTempHome(t)
	path, err := AuthPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"openrouter":{"type":"apiKey","apiKey":"sk-test"}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	auth, err := LoadAuth()
	if err != nil {
		t.Fatalf("LoadAuth: %v", err)
	}
	if got := auth.APIKey("openrouter"); got != "sk-test" {
		t.Fatalf("expected openrouter key, got %q", got)
	}
	if !auth.HasAuthorizedProvider() {
		t.Fatal("expected authorized provider")
	}
}

func TestLoadAuthEmptyHasNoAuthorizedProvider(t *testing.T) {
	withTempHome(t)

	auth, err := LoadAuth()
	if err != nil {
		t.Fatalf("LoadAuth: %v", err)
	}
	if auth.HasAuthorizedProvider() {
		t.Fatal("empty auth should not have authorized provider")
	}
}

func TestLoadAuthOAuthProvider(t *testing.T) {
	withTempHome(t)
	path, err := AuthPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"openai-codex":{"type":"oauth","access":"oauth-access","refresh":"oauth-refresh","expires":4102444800000}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	auth, err := LoadAuth()
	if err != nil {
		t.Fatalf("LoadAuth: %v", err)
	}
	if got := auth.APIKey("openai-codex"); got != "oauth-access" {
		t.Fatalf("expected openai oauth access token, got %q", got)
	}
	if got := auth.APIKey("openai"); got != "" {
		t.Fatalf("oauth token should not authorize openai API-key provider, got %q", got)
	}
	if !auth.HasAuthorizedProvider() {
		t.Fatal("expected authorized provider")
	}
}

func TestLoadAuthOpenAIApiKeyAndOAuthCanCoexist(t *testing.T) {
	withTempHome(t)
	path, err := AuthPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"openai":{"type":"apiKey","apiKey":"sk-test"},"openai-codex":{"type":"oauth","access":"oauth-access"}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	auth, err := LoadAuth()
	if err != nil {
		t.Fatalf("LoadAuth: %v", err)
	}
	if got := auth.APIKey("openai"); got != "sk-test" {
		t.Fatalf("expected openai API key, got %q", got)
	}
	if got := auth.APIKey("openai-codex"); got != "oauth-access" {
		t.Fatalf("expected openai oauth token, got %q", got)
	}
}

func TestLogoutOAuthProviderRemovesOpenAICodex(t *testing.T) {
	withTempHome(t)
	path, err := AuthPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"openai-codex":{"type":"oauth","access":"current"},"openrouter":{"type":"apiKey","apiKey":"sk-or"}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	auth, err := LoadAuth()
	if err != nil {
		t.Fatalf("LoadAuth: %v", err)
	}
	providers := auth.OAuthProviders()
	if len(providers) != 1 || providers[0] != "openai-codex" {
		t.Fatalf("expected openai-codex provider, got %#v", providers)
	}

	if err := LogoutOAuthProvider("openai-codex"); err != nil {
		t.Fatalf("LogoutOAuthProvider: %v", err)
	}
	auth, err = LoadAuth()
	if err != nil {
		t.Fatalf("LoadAuth: %v", err)
	}
	if got := auth.APIKey("openai-codex"); got != "" {
		t.Fatalf("expected oauth removed, got %q", got)
	}
	if got := auth.APIKey("openrouter"); got != "sk-or" {
		t.Fatalf("expected openrouter preserved, got %q", got)
	}
}
