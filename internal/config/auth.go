package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AuthProvider stores credentials for a provider in ~/.crobot/auth.json.
type AuthProvider struct {
	Type   string `json:"type,omitempty"`
	APIKey string `json:"apiKey,omitempty"`
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
	if entry.Type != "" && entry.Type != "apiKey" {
		return ""
	}
	return entry.APIKey
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
