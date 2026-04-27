package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Record is a single entry in the session log.
type Record struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Manager handles append-only JSONL session files.
type Manager struct {
	dir       string
	sessionID string
	path      string
	mu        sync.Mutex
}

// NewManager creates a session manager. The directory is created if it doesn't exist.
func NewManager(dir, sessionID string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("session-%s.jsonl", sessionID))
	return &Manager{
		dir:       dir,
		sessionID: sessionID,
		path:      path,
	}, nil
}

// Path returns the full path to the session file.
func (m *Manager) Path() string {
	return m.path
}

// Append writes a single record as a JSON line to the session file.
func (m *Manager) Append(rec Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	f, err := os.OpenFile(m.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write session: %w", err)
	}
	if _, err := f.Write([]byte("\n")); err != nil {
		return fmt.Errorf("write newline: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync session: %w", err)
	}
	return nil
}

// Load reads all records from the session file. Returns an empty slice if the file doesn't exist.
func (m *Manager) Load() ([]Record, error) {
	f, err := os.Open(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open session: %w", err)
	}
	defer f.Close()

	var records []Record
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var rec Record
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return nil, fmt.Errorf("unmarshal record: %w", err)
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan session: %w", err)
	}
	return records, nil
}
