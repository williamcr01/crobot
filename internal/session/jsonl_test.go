package session

import (
	"os"
	"path/filepath"
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
