package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// AuthProvider stores credentials for a provider in ~/.crobot/auth.json.
type AuthProvider struct {
	Type      string `json:"type,omitempty"`
	APIKey    string `json:"apiKey,omitempty"`
	Access    string `json:"access,omitempty"`
	Refresh   string `json:"refresh,omitempty"`
	Expires   int64  `json:"expires,omitempty"`
	AccountID string `json:"accountId,omitempty"`
}

// AuthConfig maps provider IDs to credentials.
type AuthConfig map[string]AuthProvider

// AuthPath returns the Crobot user auth file path.
func AuthPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth.json"), nil
}

// EnsureAuth creates ~/.crobot/auth.json as an empty object when missing.
func EnsureAuth() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	path := filepath.Join(dir, "auth.json")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// LoadAuth loads ~/.crobot/auth.json, creating it first when missing.
func LoadAuth() (AuthConfig, error) {
	if err := EnsureAuth(); err != nil {
		return nil, err
	}
	path, err := AuthPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var auth AuthConfig
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", path, err)
	}
	if auth == nil {
		auth = AuthConfig{}
	}
	return auth, nil
}

// APIKey returns an API key for provider, if present.
func (a AuthConfig) APIKey(provider string) string {
	entry, ok := a[provider]
	if !ok {
		return ""
	}
	if entry.Type == "oauth" {
		return entry.Access
	}
	if entry.Type != "" && entry.Type != "apiKey" {
		return ""
	}
	return entry.APIKey
}

// RefreshOpenAIOAuth refreshes an OpenAI OAuth access token when it is close to expiry.
func RefreshOpenAIOAuth() error {
	auth, err := LoadAuth()
	if err != nil {
		return err
	}
	entry, ok := auth["openai"]
	if !ok || entry.Type != "oauth" || entry.Refresh == "" {
		return nil
	}
	if entry.Access != "" && entry.Expires > time.Now().Add(2*time.Minute).UnixMilli() {
		return nil
	}
	refreshed, err := refreshOpenAIToken(entry.Refresh)
	if err != nil {
		return err
	}
	if refreshed.AccountID == "" {
		refreshed.AccountID = entry.AccountID
	}
	auth["openai"] = refreshed
	return SaveAuth(auth)
}

// SaveAuth writes credentials to ~/.crobot/auth.json.
func SaveAuth(auth AuthConfig) error {
	path, err := AuthPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func refreshOpenAIToken(refreshToken string) (AuthProvider, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", "app_EMoamEEZ73f0CkXaXp7hrann")
	resp, err := http.PostForm("https://auth.openai.com/oauth/token", form)
	if err != nil {
		return AuthProvider{}, fmt.Errorf("refresh openai oauth token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AuthProvider{}, fmt.Errorf("refresh openai oauth token: %s", resp.Status)
	}
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return AuthProvider{}, err
	}
	if body.AccessToken == "" || body.RefreshToken == "" || body.ExpiresIn == 0 {
		return AuthProvider{}, fmt.Errorf("refresh openai oauth token: incomplete token response")
	}
	return AuthProvider{Type: "oauth", Access: body.AccessToken, Refresh: body.RefreshToken, Expires: time.Now().Add(time.Duration(body.ExpiresIn) * time.Second).UnixMilli()}, nil
}

// HasAuthorizedProvider reports whether any provider has usable credentials.
func (a AuthConfig) HasAuthorizedProvider() bool {
	for provider := range a {
		if a.APIKey(provider) != "" {
			return true
		}
	}
	return false
}
