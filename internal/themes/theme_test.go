package themes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTheme_EmptyNameReturnsDefault(t *testing.T) {
	th, err := LoadTheme("")
	if err != nil {
		t.Fatalf("LoadTheme returned error: %v", err)
	}
	if th.Name != "crobot-dark" {
		t.Fatalf("expected crobot-dark, got %q", th.Name)
	}
}

func TestLoadTheme_BuiltinLight(t *testing.T) {
	th, err := LoadTheme("crobot-light")
	if err != nil {
		t.Fatalf("LoadTheme returned error: %v", err)
	}
	if th.Name != "crobot-light" {
		t.Fatalf("expected crobot-light, got %q", th.Name)
	}
	if th.Colors[StyleBodyText] == DefaultTheme().Colors[StyleBodyText] {
		t.Fatalf("expected light theme to override bodyText")
	}
}

func TestLoadTheme_FileNotFoundReturnsDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	th, err := LoadTheme("missing")
	if err != nil {
		t.Fatalf("LoadTheme returned error: %v", err)
	}
	if th.Name != "crobot-dark" {
		t.Fatalf("expected crobot-dark, got %q", th.Name)
	}
}

func TestLoadTheme_PartialFileMergesDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".crobot", "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "custom.json")
	if err := os.WriteFile(path, []byte(`{
		"name": "custom",
		"colors": {
			"bodyText": "#010203",
			"toolBg": "#111111"
		},
		"bold": {
			"h1": false
		}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	th, err := LoadTheme("custom")
	if err != nil {
		t.Fatalf("LoadTheme returned error: %v", err)
	}
	if th.Colors[StyleBodyText] != "#010203" {
		t.Fatalf("expected custom bodyText, got %q", th.Colors[StyleBodyText])
	}
	if th.Colors[StyleGreen] != DefaultTheme().Colors[StyleGreen] {
		t.Fatalf("expected default green fallback, got %q", th.Colors[StyleGreen])
	}
	if th.Bold[StyleH1] {
		t.Fatalf("expected h1 bold override false")
	}
}

func TestLoadTheme_InvalidHexReturnsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".crobot", "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`{
		"colors": {
			"bodyText": "not-a-color"
		}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadTheme("bad"); err == nil {
		t.Fatalf("expected invalid hex error")
	}
}

func TestEnsureThemeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := EnsureThemeDir(); err != nil {
		t.Fatalf("EnsureThemeDir returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".crobot", "themes")); err != nil {
		t.Fatalf("expected themes directory: %v", err)
	}
}
