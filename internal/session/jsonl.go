package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const CurrentVersion = 1

// Header is the first line of new session files.
type Header struct {
	Type      string    `json:"type"`
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	CWD       string    `json:"cwd,omitempty"`
	Title     string    `json:"title,omitempty"`
}

// Record is a single entry in the session log.
type Record struct {
	Type        string         `json:"type,omitempty"`
	ID          string         `json:"id,omitempty"`
	ParentID    string         `json:"parentId,omitempty"`
	Role        string         `json:"role,omitempty"`
	Content     string         `json:"content,omitempty"`
	Title       string         `json:"title,omitempty"`
	FirstPrompt string         `json:"firstPrompt,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// SessionInfo describes a session file for listing, pruning, and display.
type SessionInfo struct {
	Path         string
	ID           string
	CWD          string
	Title        string
	FirstPrompt  string
	Created      time.Time
	Modified     time.Time
	MessageCount int
	FirstMessage string
	SizeBytes    int64
}

// RetentionPolicy controls automatic session pruning.
type RetentionPolicy struct {
	MaxAge              time.Duration
	MaxSessions         int
	KeepNamed           bool
	KeepCurrentPath     string
	PruneEmptyOlderThan time.Duration
}

// PruneResult summarizes deleted sessions.
type PruneResult struct {
	Deleted int
	Paths   []string
}

// Manager handles append-only JSONL session files.
type Manager struct {
	dir       string
	sessionID string
	path      string
	cwd       string
	mu        sync.Mutex
}

// NewManager creates a session manager. Kept for compatibility; prefer Create.
func NewManager(dir, sessionID string) (*Manager, error) {
	cwd, _ := os.Getwd()
	return createWithID(dir, sessionID, cwd)
}

// Create creates a new session with a generated ID.
func Create(dir, cwd string) (*Manager, error) {
	return createWithID(dir, newID(), cwd)
}

func createWithID(dir, sessionID, cwd string) (*Manager, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("session dir is empty")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}
	path := filepath.Join(absDir, fmt.Sprintf("session-%s.jsonl", sessionID))
	m := &Manager{dir: absDir, sessionID: sessionID, path: path, cwd: cwd}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := m.writeHeader(); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// Open opens an existing session file.
func Open(path string) (*Manager, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	header, _, err := readFile(absPath)
	if err != nil {
		return nil, err
	}
	id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(absPath), "session-"), ".jsonl")
	cwd := ""
	if header != nil {
		id = header.ID
		cwd = header.CWD
	}
	return &Manager{dir: filepath.Dir(absPath), sessionID: id, path: absPath, cwd: cwd}, nil
}

// ContinueRecent opens the most recently modified valid session, or creates one.
func ContinueRecent(dir, cwd string) (*Manager, error) {
	infos, err := List(dir)
	if err != nil {
		return nil, err
	}
	if len(infos) == 0 {
		return Create(dir, cwd)
	}
	return Open(infos[0].Path)
}

func newID() string { return fmt.Sprintf("%x", time.Now().UnixNano()) }

func (m *Manager) writeHeader() error {
	h := Header{Type: "session", Version: CurrentVersion, ID: m.sessionID, Timestamp: time.Now(), CWD: m.cwd}
	data, err := json.Marshal(h)
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, append(data, '\n'), 0o644)
}

// Path returns the full path to the session file.
func (m *Manager) Path() string { return m.path }
func (m *Manager) ID() string   { return m.sessionID }

// Append writes a single record as a JSON line to the session file.
func (m *Manager) Append(rec Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rec.Type == "" {
		rec.Type = "message"
	}
	if rec.ID == "" {
		rec.ID = newID()
	}
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now()
	}

	f, err := os.OpenFile(m.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	defer f.Close()
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write session: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync session: %w", err)
	}
	return nil
}

// Load reads all message records from the session file.
func (m *Manager) Load() ([]Record, error) {
	_, records, err := readFile(m.path)
	return records, err
}

func (m *Manager) Info() (SessionInfo, error) {
	return buildInfo(m.path)
}

// SetTitleFromPrompt stores display metadata derived from the first user prompt.
// It is append-only so existing session history is not rewritten.
func (m *Manager) SetTitleFromPrompt(prompt string) error {
	info, err := m.Info()
	if err != nil {
		return err
	}
	if info.Title != "" || info.MessageCount > 0 {
		return nil
	}
	title := DeriveTitle(prompt)
	return m.Append(Record{Type: "session_info", Title: title, FirstPrompt: strings.TrimSpace(prompt), Timestamp: time.Now()})
}

// DeriveTitle creates a compact display title from a first user prompt.
func DeriveTitle(prompt string) string {
	title := strings.Join(strings.Fields(prompt), " ")
	title = strings.Trim(title, "`#*_~> -\t\n\r")
	if title == "" {
		return "(untitled)"
	}
	const max = 80
	if len(title) <= max {
		return title
	}
	cut := title[:max]
	if idx := strings.LastIndex(cut, " "); idx >= 40 {
		cut = cut[:idx]
	}
	return strings.TrimSpace(cut) + "…"
}

func (m *Manager) ExportMarkdown(path string) error {
	if path == "" {
		path = filepath.Join(m.dir, fmt.Sprintf("session-%s.md", m.sessionID))
	}
	records, err := m.Load()
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Crobot session\n\n")
	b.WriteString(fmt.Sprintf("Session: `%s`\n\n", m.sessionID))
	for _, r := range records {
		role := r.Role
		if role == "" {
			role = "message"
		}
		b.WriteString("## " + strings.Title(role) + "\n\n")
		b.WriteString(r.Content)
		b.WriteString("\n\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func readFile(path string) (*Header, []Record, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("open session: %w", err)
	}
	defer f.Close()
	var header *Header
	var records []Record
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		var probe struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(line, &probe)
		if probe.Type == "session" {
			var h Header
			if err := json.Unmarshal(line, &h); err != nil {
				return nil, nil, fmt.Errorf("unmarshal header: %w", err)
			}
			header = &h
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, nil, fmt.Errorf("unmarshal record: %w", err)
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan session: %w", err)
	}
	return header, records, nil
}

// List returns valid session files sorted by modified descending.
func List(dir string) ([]SessionInfo, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(absDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	infos := []SessionInfo{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") || strings.HasPrefix(e.Name(), "events-") {
			continue
		}
		info, err := buildInfo(filepath.Join(absDir, e.Name()))
		if err == nil {
			infos = append(infos, info)
		}
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Modified.After(infos[j].Modified) })
	return infos, nil
}

func buildInfo(path string) (SessionInfo, error) {
	header, records, err := readFile(path)
	if err != nil {
		return SessionInfo{}, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return SessionInfo{}, err
	}
	info := SessionInfo{Path: path, Modified: st.ModTime(), SizeBytes: st.Size(), FirstMessage: "(no messages)"}
	if header != nil {
		info.ID, info.CWD, info.Title, info.Created = header.ID, header.CWD, header.Title, header.Timestamp
	} else {
		info.ID = strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "session-"), ".jsonl")
		info.Created = st.ModTime()
	}
	for _, r := range records {
		if r.Type == "session_info" {
			if strings.TrimSpace(r.Title) != "" {
				info.Title = strings.TrimSpace(r.Title)
			}
			if strings.TrimSpace(r.FirstPrompt) != "" {
				info.FirstPrompt = strings.TrimSpace(r.FirstPrompt)
			}
			continue
		}
		if r.Role == "user" || r.Role == "assistant" {
			info.MessageCount++
		}
		if info.FirstMessage == "(no messages)" && r.Role == "user" && strings.TrimSpace(r.Content) != "" {
			info.FirstMessage = strings.TrimSpace(r.Content)
			if info.FirstPrompt == "" {
				info.FirstPrompt = info.FirstMessage
			}
		}
		if !r.Timestamp.IsZero() && r.Timestamp.After(info.Modified) {
			info.Modified = r.Timestamp
		}
	}
	return info, nil
}

func Prune(dir string, policy RetentionPolicy) (PruneResult, error) {
	infos, err := List(dir)
	if err != nil {
		return PruneResult{}, err
	}
	now := time.Now()
	keep := map[string]bool{}
	if policy.KeepCurrentPath != "" {
		if abs, err := filepath.Abs(policy.KeepCurrentPath); err == nil {
			keep[abs] = true
		}
	}
	deletePath := func(info SessionInfo, result *PruneResult) {
		abs, _ := filepath.Abs(info.Path)
		if keep[abs] || (policy.KeepNamed && info.Title != "") {
			return
		}
		if os.Remove(info.Path) == nil {
			result.Deleted++
			result.Paths = append(result.Paths, info.Path)
		}
	}
	var result PruneResult
	for _, info := range infos {
		if policy.PruneEmptyOlderThan > 0 && info.MessageCount == 0 && now.Sub(info.Modified) > policy.PruneEmptyOlderThan {
			deletePath(info, &result)
		}
	}
	infos, _ = List(dir)
	for _, info := range infos {
		if policy.MaxAge > 0 && now.Sub(info.Modified) > policy.MaxAge {
			deletePath(info, &result)
		}
	}
	infos, _ = List(dir)
	if policy.MaxSessions > 0 && len(infos) > policy.MaxSessions {
		for _, info := range infos[policy.MaxSessions:] {
			deletePath(info, &result)
		}
	}
	return result, nil
}
