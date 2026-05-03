package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	readability "codeberg.org/readeck/go-readability/v2"
)

const (
	defaultTimeout  = 30 * time.Second
	maxResponseSize = 5 * 1024 * 1024 // 5MB
)

// ExtractOptions configures content extraction.
type ExtractOptions struct {
	Prompt     string // Question for video analysis
	Timestamp  string // Frame extraction timestamp
	Frames     int    // Number of frames
	ForceClone bool   // Force clone large repos
}

// ExtractContent extracts readable content from a URL.
func ExtractContent(ctx context.Context, url string, cfg *Config, opts ExtractOptions) (*ExtractedContent, error) {
	// GitHub URLs.
	if isGitHubURL(url) {
		return extractGitHub(ctx, url, opts.ForceClone)
	}

	// YouTube URLs.
	if isYouTubeURL(url) {
		return extractYouTube(ctx, url, cfg, opts)
	}

	// Regular HTTP extraction.
	return extractHTTP(ctx, url)
}

// isGitHubURL checks if the URL points to GitHub.
func isGitHubURL(url string) bool {
	return strings.Contains(url, "github.com/")
}

// isYouTubeURL checks if the URL points to YouTube.
func isYouTubeURL(url string) bool {
	return strings.Contains(url, "youtube.com/watch") ||
		strings.Contains(url, "youtu.be/") ||
		strings.Contains(url, "youtube.com/shorts/") ||
		strings.Contains(url, "youtube.com/live/") ||
		strings.Contains(url, "youtube.com/embed/") ||
		strings.Contains(url, "youtube.com/v/")
}

// extractHTTP fetches and extracts content from a web page.
func extractHTTP(ctx context.Context, url string) (*ExtractedContent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	client := &http.Client{Timeout: defaultTimeout}
	resp, err := client.Do(req)
	if err != nil {
		// Try Jina Reader fallback.
		return extractWithJina(ctx, url)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return extractWithJina(ctx, url)
	}

	// Check content type.
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") && !strings.Contains(contentType, "application/xhtml") {
		return extractWithJina(ctx, url)
	}

	limited := io.LimitReader(resp.Body, maxResponseSize)
	htmlBytes, err := io.ReadAll(limited)
	if err != nil {
		return extractWithJina(ctx, url)
	}

	html := string(htmlBytes)
	title, content, err := parseReadable(html, url)
	if err != nil || content == "" {
		return extractWithJina(ctx, url)
	}

	return &ExtractedContent{
		URL:     url,
		Title:   title,
		Content: content,
	}, nil
}

// parseReadable extracts readable content from HTML.
func parseReadable(htmlStr, rawURL string) (title, content string, err error) {
	pageURL, _ := url.Parse(rawURL)
	article, err := readability.FromReader(strings.NewReader(htmlStr), pageURL)
	if err != nil {
		return "", "", err
	}

	// Render the article node to HTML, then convert to markdown.
	if article.Node == nil {
		return "", "", fmt.Errorf("readability returned nil node")
	}
	var buf strings.Builder
	if err := article.RenderHTML(&buf); err != nil {
		return "", "", err
	}

	md, err := newMarkdownConverter().ConvertString(buf.String())
	if err != nil {
		return article.Title(), "", err
	}

	title = article.Title()
	if title == "" {
		title = extractTitleFromHTML(htmlStr, rawURL)
	}

	return title, strings.TrimSpace(md), nil
}

// extractWithJina uses Jina Reader as a fallback.
func extractWithJina(ctx context.Context, url string) (*ExtractedContent, error) {
	jinaURL := "https://r.jina.ai/" + url
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jinaURL, nil)
	if err != nil {
		return nil, fmt.Errorf("jina request: %w", err)
	}
	req.Header.Set("Accept", "text/markdown")
	req.Header.Set("X-No-Cache", "true")

	client := &http.Client{Timeout: defaultTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jina fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jina returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("jina read: %w", err)
	}

	content := string(body)
	// Jina prepends metadata; try to find the markdown content marker.
	if idx := strings.Index(content, "Markdown Content:"); idx >= 0 {
		content = strings.TrimSpace(content[idx+len("Markdown Content:"):])
	}

	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("jina returned empty content")
	}

	title := extractFirstHeading(content, url)

	return &ExtractedContent{
		URL:     url,
		Title:   title,
		Content: content,
	}, nil
}

func newMarkdownConverter() *converter.Converter {
	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
		),
	)
	return conv
}

func extractTitleFromHTML(html, fallbackURL string) string {
	// Simple title extraction from <title> tag.
	lower := strings.ToLower(html)
	start := strings.Index(lower, "<title>")
	end := strings.Index(lower, "</title>")
	if start >= 0 && end > start {
		return strings.TrimSpace(html[start+7 : end])
	}

	// Fallback to last path segment.
	if idx := strings.LastIndex(fallbackURL, "/"); idx >= 0 && idx < len(fallbackURL)-1 {
		return fallbackURL[idx+1:]
	}
	return fallbackURL
}

func extractFirstHeading(content, fallbackURL string) string {
	// Try to find a # heading.
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(trimmed[2:])
		}
	}

	// Fallback to last path segment.
	if idx := strings.LastIndex(fallbackURL, "/"); idx >= 0 && idx < len(fallbackURL)-1 {
		return fallbackURL[idx+1:]
	}
	return fallbackURL
}


