package web

import (
	"testing"
)

func TestStorageStoreAndGet(t *testing.T) {
	s := NewStorage()
	id := s.GenerateID()

	data := StoredData{
		ID:        id,
		Type:      "search",
		Timestamp: 12345,
		Queries: []QueryResultData{
			{Query: "test query", Answer: "test answer"},
		},
	}
	s.Store(data)

	got, ok := s.Get(id)
	if !ok {
		t.Fatal("expected stored data")
	}
	if got.ID != id {
		t.Errorf("expected id %q, got %q", id, got.ID)
	}
	if got.Type != "search" {
		t.Errorf("expected type search, got %q", got.Type)
	}
	if len(got.Queries) != 1 || got.Queries[0].Query != "test query" {
		t.Errorf("unexpected queries: %v", got.Queries)
	}
}

func TestStorageGetMissing(t *testing.T) {
	s := NewStorage()
	_, ok := s.Get("nonexistent")
	if ok {
		t.Error("expected missing key")
	}
}

func TestStorageAll(t *testing.T) {
	s := NewStorage()
	s.Store(StoredData{ID: "a", Type: "search"})
	s.Store(StoredData{ID: "b", Type: "fetch"})

	all := s.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}
}

func TestStorageClear(t *testing.T) {
	s := NewStorage()
	s.Store(StoredData{ID: "a"})
	s.Store(StoredData{ID: "b"})

	s.Clear()
	if len(s.All()) != 0 {
		t.Error("expected empty after clear")
	}
}

func TestStorageFindByQuery(t *testing.T) {
	s := NewStorage()
	s.Store(StoredData{
		ID:   "s1",
		Type: "search",
		Queries: []QueryResultData{
			{Query: "golang concurrency"},
			{Query: "rust async"},
		},
	})
	s.Store(StoredData{
		ID:   "s2",
		Type: "search",
		Queries: []QueryResultData{
			{Query: "python asyncio"},
		},
	})

	results := s.FindByQuery("rust async")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "s1" {
		t.Errorf("expected s1, got %s", results[0].ID)
	}

	results = s.FindByQuery("nonexistent")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestStorageFindByURL(t *testing.T) {
	s := NewStorage()
	s.Store(StoredData{
		ID:   "f1",
		Type: "fetch",
		URLs: []ExtractedContent{
			{URL: "https://example.com", Title: "Example"},
			{URL: "https://test.com", Title: "Test"},
		},
	})
	s.Store(StoredData{
		ID:   "f2",
		Type: "fetch",
		URLs: []ExtractedContent{
			{URL: "https://other.com", Title: "Other"},
		},
	})

	results := s.FindByURL("https://example.com")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "f1" {
		t.Errorf("expected f1, got %s", results[0].ID)
	}

	results = s.FindByURL("https://nonexistent.com")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestStorageGenerateID(t *testing.T) {
	s := NewStorage()
	id1 := s.GenerateID()
	id2 := s.GenerateID()
	if id1 == id2 {
		t.Error("generated IDs should be unique")
	}
	if len(id1) != 8 {
		t.Errorf("expected 8-char hex ID, got %q (len=%d)", id1, len(id1))
	}
}
