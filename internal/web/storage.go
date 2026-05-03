package web

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// ResolvedProvider is a concrete search provider name.
type ResolvedProvider string

// SearchResult represents a single search result from a provider.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

// ExtractedContent holds content extracted from a URL.
type ExtractedContent struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Error   string `json:"error,omitempty"`
}

// QueryResultData holds the result of a single query search.
type QueryResultData struct {
	Query    string         `json:"query"`
	Answer   string         `json:"answer"`
	Results  []SearchResult `json:"results"`
	Error    string         `json:"error,omitempty"`
	Provider string         `json:"provider,omitempty"`
}

// SearchResponse is the result of a single search.
type SearchResponse struct {
	Answer        string             `json:"answer"`
	Results       []SearchResult     `json:"results"`
	InlineContent []ExtractedContent `json:"inline_content,omitempty"`
	Provider      string             `json:"provider"`
}

// StoredData is the stored result of a search or fetch operation.
type StoredData struct {
	ID        string             `json:"id"`
	Type      string             `json:"type"` // "search" or "fetch"
	Timestamp int64              `json:"timestamp"`
	Queries   []QueryResultData  `json:"queries,omitempty"`
	URLs      []ExtractedContent `json:"urls,omitempty"`
}

// Storage provides session-scoped result storage for later retrieval
// via get_search_content.
type Storage struct {
	mu   sync.RWMutex
	data map[string]StoredData
}

// NewStorage creates a new empty storage.
func NewStorage() *Storage {
	return &Storage{data: make(map[string]StoredData)}
}

// GenerateID returns a random 8-character hex ID.
func (s *Storage) GenerateID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Store saves a StoredData entry.
func (s *Storage) Store(data StoredData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[data.ID] = data
}

// Get retrieves a StoredData entry by ID.
func (s *Storage) Get(id string) (StoredData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.data[id]
	return d, ok
}

// All returns all stored entries.
func (s *Storage) All() []StoredData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]StoredData, 0, len(s.data))
	for _, d := range s.data {
		out = append(out, d)
	}
	return out
}

// Clear removes all stored entries.
func (s *Storage) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[string]StoredData)
}

// FindByQuery returns entries that contain results for a given query string.
func (s *Storage) FindByQuery(query string) []StoredData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []StoredData
	for _, d := range s.data {
		if d.Type == "search" {
			for _, q := range d.Queries {
				if q.Query == query {
					out = append(out, d)
					break
				}
			}
		}
	}
	return out
}

// FindByURL returns entries that contain content for a given URL.
func (s *Storage) FindByURL(url string) []StoredData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []StoredData
	for _, d := range s.data {
		if d.Type == "fetch" {
			for _, u := range d.URLs {
				if u.URL == url {
					out = append(out, d)
					break
				}
			}
		}
	}
	return out
}
