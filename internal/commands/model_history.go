package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	// MaxRecentModels is the maximum number of recently used models to track.
	MaxRecentModels = 10

	// historyFileName is the file name for the model history JSON.
	historyFileName = "model-history.json"
)

// ModelHistory tracks recently used models for display priority.
type ModelHistory struct {
	// Recent is an ordered list of "provider/modelID" strings, most recent first.
	Recent []string `json:"recent"`

	mu   sync.Mutex
	path string
}

// NewModelHistory creates or loads model history from a config directory.
// If the file exists, it is loaded. Otherwise an empty history is created.
// The path is derived from the config directory (typically ~/.crobot/).
func NewModelHistory(configDir string) *ModelHistory {
	h := &ModelHistory{
		Recent: make([]string, 0, MaxRecentModels),
		path:   filepath.Join(configDir, historyFileName),
	}
	data, err := os.ReadFile(h.path)
	if err == nil {
		var loaded ModelHistory
		if json.Unmarshal(data, &loaded) == nil && len(loaded.Recent) > 0 {
			h.Recent = loaded.Recent
			if len(h.Recent) > MaxRecentModels {
				h.Recent = h.Recent[:MaxRecentModels]
			}
		}
	}
	return h
}

// Record adds a model to the history. If it already exists, it is moved to
// the front. The list is truncated to MaxRecentModels entries.
func (h *ModelHistory) Record(provider, modelID string) {
	if provider == "" || modelID == "" {
		return
	}
	entry := provider + "/" + modelID

	h.mu.Lock()
	defer h.mu.Unlock()

	// Remove if already present.
	for i, e := range h.Recent {
		if e == entry {
			h.Recent = append(h.Recent[:i], h.Recent[i+1:]...)
			break
		}
	}

	// Prepend.
	h.Recent = append([]string{entry}, h.Recent...)

	// Trim.
	if len(h.Recent) > MaxRecentModels {
		h.Recent = h.Recent[:MaxRecentModels]
	}

	_ = h.save()
}

// Recency returns -1 if the model has not been used, or its positional index
// (0 = most recent). Lower values = more recent. Used for sorting.
func (h *ModelHistory) Recency(provider, modelID string) int {
	if provider == "" || modelID == "" {
		return -1
	}
	entry := provider + "/" + modelID

	h.mu.Lock()
	defer h.mu.Unlock()

	for i, e := range h.Recent {
		if e == entry {
			return i
		}
	}
	return -1
}

// save writes the history to disk.
func (h *ModelHistory) save() error {
	if h.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal model history: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(h.path), 0o755); err != nil {
		return fmt.Errorf("create model history dir: %w", err)
	}
	if err := os.WriteFile(h.path, data, 0o644); err != nil {
		return fmt.Errorf("write model history: %w", err)
	}
	return nil
}
