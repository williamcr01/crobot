package themes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadTheme loads a theme by name.
//
// Built-in themes (crobot-dark, crobot-light, crobot-monochrome) are loaded from
// embedded JSON. User themes are loaded from ~/.crobot/themes/<name>.json.
//
// Missing color keys fall back to the crobot-dark defaults.
// If the theme file doesn't exist, the default theme is returned with a nil error.
func LoadTheme(name string) (*Theme, error) {
	if name == "" {
		return DefaultTheme(), nil
	}

	// Check built-in themes first.
	if jsonData := builtinThemeJSON(name); jsonData != "" {
		return LoadThemeJSON(name, jsonData)
	}
	if name == "crobot-dark" {
		return DefaultTheme(), nil
	}

	// Try user theme from disk.
	dir, err := themesDir()
	if err != nil {
		return DefaultTheme(), nil
	}

	path := filepath.Join(dir, name+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultTheme(), nil
		}
		return DefaultTheme(), fmt.Errorf("reading theme %q: %w", path, err)
	}

	loaded := &Theme{}
	if err := json.Unmarshal(data, loaded); err != nil {
		return DefaultTheme(), fmt.Errorf("parsing theme %q: %w", path, err)
	}
	if loaded.Name == "" {
		loaded.Name = name
	}

	if loaded.Colors == nil {
		loaded.Colors = make(map[StyleName]string)
	}

	// Merge: for each known style, use the loaded color if present, else default.
	def := DefaultTheme()
	for _, sn := range allStyleNames() {
		if _, ok := loaded.Colors[sn]; !ok {
			loaded.Colors[sn] = def.Colors[sn]
		}
	}

	// Merge bold flags.
	if loaded.Bold == nil {
		loaded.Bold = make(map[StyleName]bool)
	}
	for _, sn := range allBoldNames() {
		if _, ok := loaded.Bold[sn]; !ok {
			loaded.Bold[sn] = def.Bold[sn]
		}
	}

	// Validate all colors are valid hex.
	for sn, c := range loaded.Colors {
		if err := validHex(c); err != nil {
			return DefaultTheme(), fmt.Errorf("invalid color %q for %q: %w", c, sn, err)
		}
	}

	return loaded, nil
}

// validHex checks that s is a valid hex color string (#RGB, #RRGGBB, or #RRGGBBAA).
func validHex(s string) error {
	if len(s) < 4 || s[0] != '#' {
		return fmt.Errorf("must start with #")
	}
	validLen := false
	for _, l := range []int{4, 5, 7, 9} {
		if len(s) == l {
			validLen = true
			break
		}
	}
	if !validLen {
		return fmt.Errorf("invalid length %d (expected 4, 5, 7, or 9)", len(s))
	}
	for _, c := range s[1:] {
		if !isHexRune(c) {
			return fmt.Errorf("non-hex character %c", c)
		}
	}
	return nil
}

func isHexRune(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// themesDir returns ~/.crobot/themes.
func themesDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".crobot", "themes"), nil
}

// themesJSON returns the embedded JSON for a built-in theme name, or empty string.
func builtinThemeJSON(name string) string {
	switch name {
	case "crobot-light":
		return lightThemeJSON
	case "crobot-dark":
		return "" // handled by DefaultTheme()
	case "crobot-monochrome":
		return monochromeThemeJSON
	}
	return ""
}

// EnsureThemeDir creates ~/.crobot/themes if it doesn't exist.
// Called during startup from main.go so users can drop theme files in.
func EnsureThemeDir() error {
	dir, err := themesDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}

// LoadThemeJSON parses a JSON string into a merged Theme (with defaults).
// Used by built-in themes loaded via //go:embed.
func LoadThemeJSON(name, jsonData string) (*Theme, error) {
	loaded := &Theme{}
	if err := json.Unmarshal([]byte(jsonData), loaded); err != nil {
		return DefaultTheme(), fmt.Errorf("parsing builtin theme %q: %w", name, err)
	}
	if loaded.Name == "" {
		loaded.Name = name
	}
	if loaded.Colors == nil {
		loaded.Colors = make(map[StyleName]string)
	}

	def := DefaultTheme()
	for _, sn := range allStyleNames() {
		if _, ok := loaded.Colors[sn]; !ok {
			loaded.Colors[sn] = def.Colors[sn]
		}
	}
	if loaded.Bold == nil {
		loaded.Bold = make(map[StyleName]bool)
	}
	for _, sn := range allBoldNames() {
		if _, ok := loaded.Bold[sn]; !ok {
			loaded.Bold[sn] = def.Bold[sn]
		}
	}
	for sn, c := range loaded.Colors {
		if err := validHex(c); err != nil {
			return DefaultTheme(), fmt.Errorf("invalid color %q for %q: %w", c, sn, err)
		}
	}

	return loaded, nil
}

// ThemePaths returns a list of installed user theme file paths.
func ThemePaths() ([]string, error) {
	dir, err := themesDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	return paths, nil
}
