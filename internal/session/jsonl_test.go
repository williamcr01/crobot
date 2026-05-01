package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSession_AppendAndLoad(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(dir, "test-session")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	rec1 := Record{Role: "user", Content: "hello", Timestamp: now}
	rec2 := Record{Role: "assistant", Content: "hi there", Timestamp: now.Add(time.Second)}

	if err := mgr.Append(rec1); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Append(rec2); err != nil {
		t.Fatal(err)
	}

	records, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].Role != "user" || records[0].Content != "hello" {
		t.Errorf("unexpected first record: %+v", records[0])
	}
	if records[1].Role != "assistant" || records[1].Content != "hi there" {
		t.Errorf("unexpected second record: %+v", records[1])
	}
}

func TestSession_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(dir, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}

	records, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Errorf("expected empty records, got %d", len(records))
	}
}

func TestSession_MultipleSessions(t *testing.T) {
	dir := t.TempDir()

	mgr1, err := NewManager(dir, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	mgr2, err := NewManager(dir, "session-2")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	if err := mgr1.Append(Record{Role: "user", Content: "session1 msg", Timestamp: now}); err != nil {
		t.Fatal(err)
	}
	if err := mgr2.Append(Record{Role: "user", Content: "session2 msg", Timestamp: now}); err != nil {
		t.Fatal(err)
	}

	// Each manager only sees its own file.
	r1, _ := mgr1.Load()
	if len(r1) != 1 || r1[0].Content != "session1 msg" {
		t.Errorf("unexpected session1: %+v", r1)
	}
	r2, _ := mgr2.Load()
	if len(r2) != 1 || r2[0].Content != "session2 msg" {
		t.Errorf("unexpected session2: %+v", r2)
	}

	// Files exist at separate paths.
	if mgr1.Path() == mgr2.Path() {
		t.Error("paths should differ")
	}
	if !filepath.IsAbs(mgr1.Path()) {
		t.Errorf("expected absolute path, got %s", mgr1.Path())
	}
}

func TestSession_DirCreated(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "sessions")
	mgr, err := NewManager(dir, "createdir")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("directory should exist")
	}
	if err := mgr.Append(Record{Role: "system", Content: "test", Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mgr.Path()); os.IsNotExist(err) {
		t.Error("file should exist after append")
	}
}

func TestSession_CreateWritesHeaderAndInfo(t *testing.T) {
	dir := t.TempDir()
	mgr, err := Create(dir, "/tmp/project")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Append(Record{Role: "user", Content: "first", Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}
	info, err := mgr.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != mgr.ID() || info.CWD != "/tmp/project" || info.MessageCount != 1 || info.FirstMessage != "first" {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestSession_ListSortedByModified(t *testing.T) {
	dir := t.TempDir()
	old, err := NewManager(dir, "old")
	if err != nil {
		t.Fatal(err)
	}
	if err := old.Append(Record{Role: "user", Content: "old", Timestamp: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	newer, err := NewManager(dir, "new")
	if err != nil {
		t.Fatal(err)
	}
	if err := newer.Append(Record{Role: "user", Content: "new", Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}

	infos, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(infos))
	}
	if infos[0].ID != newer.ID() {
		t.Fatalf("expected newest first, got %+v", infos)
	}
}

func TestSession_ExportMarkdown(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(dir, "export")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Append(Record{Role: "user", Content: "hello", Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.md")
	if err := mgr.ExportMarkdown(out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "## User") || !strings.Contains(string(data), "hello") {
		t.Fatalf("unexpected export: %s", string(data))
	}
}

func TestSession_PruneByMaxSessionsKeepsCurrent(t *testing.T) {
	dir := t.TempDir()
	old, err := NewManager(dir, "old-prune")
	if err != nil {
		t.Fatal(err)
	}
	if err := old.Append(Record{Role: "user", Content: "old", Timestamp: time.Now().Add(-2 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	current, err := NewManager(dir, "current-prune")
	if err != nil {
		t.Fatal(err)
	}
	if err := current.Append(Record{Role: "user", Content: "current", Timestamp: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	newer, err := NewManager(dir, "newer-prune")
	if err != nil {
		t.Fatal(err)
	}
	if err := newer.Append(Record{Role: "user", Content: "newer", Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}

	result, err := Prune(dir, RetentionPolicy{MaxSessions: 1, KeepCurrentPath: current.Path()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 {
		t.Fatalf("expected 1 deleted, got %+v", result)
	}
	if _, err := os.Stat(old.Path()); !os.IsNotExist(err) {
		t.Fatalf("expected old session deleted, stat err=%v", err)
	}
	if _, err := os.Stat(current.Path()); err != nil {
		t.Fatalf("expected current kept: %v", err)
	}
	if _, err := os.Stat(newer.Path()); err != nil {
		t.Fatalf("expected newest kept: %v", err)
	}
}

func TestSession_AppendWithMetadata(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(dir, "meta-test")
	if err != nil {
		t.Fatal(err)
	}

	rec := Record{
		Role:      "assistant",
		Content:   "response",
		Timestamp: time.Now(),
		Metadata:  map[string]any{"tokens": 42, "model": "test"},
	}
	if err := mgr.Append(rec); err != nil {
		t.Fatal(err)
	}

	records, _ := mgr.Load()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Metadata["tokens"].(float64) != 42 {
		t.Errorf("unexpected metadata: %+v", records[0].Metadata)
	}
}
