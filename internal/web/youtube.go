package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"google.golang.org/genai"
)

const ytDefaultModel = "gemini-2.5-flash"

// extractYouTube fetches a YouTube video description using the Gemini API.
// Requires geminiApiKey in config. Falls back to scraping the page metadata.
func extractYouTube(ctx context.Context, rawURL string, cfg *Config, opts ExtractOptions) (*ExtractedContent, error) {
	videoID := extractYouTubeID(rawURL)
	if videoID == "" {
		return nil, fmt.Errorf("could not extract YouTube video ID from URL: %s", rawURL)
	}

	// If we have a Gemini API key, use it for full video understanding.
	if cfg.GeminiAPIKey != "" {
		result, err := extractYouTubeWithGemini(ctx, cfg.GeminiAPIKey, rawURL, opts.Prompt)
		if err == nil {
			return result, nil
		}
		// Fall through to page scrape on error.
	}

	// Fallback: scrape video page metadata.
	return extractYouTubeFromPage(ctx, rawURL, videoID)
}

// extractYouTubeWithGemini uses the Gemini API to analyze the video.
func extractYouTubeWithGemini(ctx context.Context, apiKey, videoURL, prompt string) (*ExtractedContent, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini client: %w", err)
	}

	query := fmt.Sprintf("Describe the content of this YouTube video: %s", videoURL)
	if prompt != "" {
		query = fmt.Sprintf("Watch this YouTube video and answer: %s\n\nVideo URL: %s", prompt, videoURL)
	}

	contents := []*genai.Content{
		genai.NewContentFromText(query, genai.RoleUser),
	}

	config := &genai.GenerateContentConfig{}
	resp, err := client.Models.GenerateContent(ctx, ytDefaultModel, contents, config)
	if err != nil {
		return nil, fmt.Errorf("gemini video analysis: %w", err)
	}

	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return nil, fmt.Errorf("gemini returned no content")
	}

	var parts []string
	for _, p := range resp.Candidates[0].Content.Parts {
		if !p.Thought && p.Text != "" {
			parts = append(parts, p.Text)
		}
	}

	text := strings.TrimSpace(strings.Join(parts, "\n"))
	if text == "" {
		return nil, fmt.Errorf("gemini returned empty analysis")
	}

	return &ExtractedContent{
		URL:     videoURL,
		Title:   "YouTube Video",
		Content: text,
	}, nil
}

// extractYouTubeFromPage scrapes basic metadata from the YouTube video page.
func extractYouTubeFromPage(ctx context.Context, rawURL, videoID string) (*ExtractedContent, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512 * 1024))
	if err != nil {
		return nil, err
	}

	html := string(body)
	title := extractYTMeta(html, "name=\"title\"")
	description := extractYTMeta(html, "name=\"description\"")

	if title == "" || description == "" {
		return nil, fmt.Errorf(
			"youtube extraction requires a Gemini API key. "+
				"Add one to ~/.crobot/web-search.json to enable video understanding.\n\n"+
				"Page title: %s\nVideo ID: %s",
			title, videoID,
		)
	}

	return &ExtractedContent{
		URL:   rawURL,
		Title: title,
		Content: fmt.Sprintf("# %s\n\n%s\n\nVideo ID: %s\nURL: %s",
			title, description, videoID, rawURL),
	}, nil
}

// extractYouTubeID extracts the video ID from various YouTube URL formats.
func extractYouTubeID(rawURL string) string {
	// Handle youtu.be/{id}
	if strings.Contains(rawURL, "youtu.be/") {
		idx := strings.Index(rawURL, "youtu.be/")
		id := rawURL[idx+9:]
		if q := strings.Index(id, "?"); q >= 0 {
			id = id[:q]
		}
		if q := strings.Index(id, "&"); q >= 0 {
			id = id[:q]
		}
		if len(id) == 11 {
			return id
		}
		return ""
	}

	// Handle youtube.com/watch?v={id}
	if strings.Contains(rawURL, "youtube.com/watch") {
		if q := strings.Index(rawURL, "?"); q >= 0 {
			params := rawURL[q+1:]
			for _, param := range strings.Split(params, "&") {
				if strings.HasPrefix(param, "v=") {
					return param[2:]
				}
			}
		}
	}

	// Handle /shorts/{id}, /embed/{id}, /v/{id}, /live/{id}
	patterns := []string{"/shorts/", "/embed/", "/v/", "/live/"}
	for _, p := range patterns {
		if idx := strings.Index(rawURL, p); idx >= 0 {
			id := rawURL[idx+len(p):]
			if q := strings.IndexAny(id, "?&"); q >= 0 {
				id = id[:q]
			}
			if q := strings.Index(id, "/"); q >= 0 {
				id = id[:q]
			}
			if len(id) >= 11 {
				return id
			}
		}
	}

	return ""
}

// extractYTMeta extracts a meta tag content value from YouTube HTML.
func extractYTMeta(html, nameAttr string) string {
	idx := strings.Index(html, nameAttr)
	if idx < 0 {
		return ""
	}
	rest := html[idx:]
	contentIdx := strings.Index(rest, "content=")
	if contentIdx < 0 {
		return ""
	}
	rest = rest[contentIdx+8:]
	// Find the start (first " or ')
	end := strings.IndexAny(rest, "\"'")
	if end < 0 {
		return ""
	}
	rest = rest[end+1:]
	end = strings.IndexAny(rest, "\"'")
	if end < 0 {
		return ""
	}
	return rest[:end]
}
