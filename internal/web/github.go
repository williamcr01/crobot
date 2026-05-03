package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const ghAPIBase = "https://api.github.com"

// extractGitHub fetches content from a GitHub URL using the public API.
// No API key required for public repos.
func extractGitHub(ctx context.Context, rawURL string, forceClone bool) (*ExtractedContent, error) {
	owner, repo, ref, path, kind := parseGitHubURL(rawURL)
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("invalid GitHub URL: %s", rawURL)
	}

	client := &http.Client{Timeout: 15 * time.Second}

	switch kind {
	case "commit":
		return fetchGitHubCommit(ctx, client, owner, repo, path)
	case "blob":
		return fetchGitHubFile(ctx, client, owner, repo, ref, path)
	case "tree":
		return fetchGitHubTree(ctx, client, owner, repo, ref, path)
	default:
		return fetchGitHubRoot(ctx, client, owner, repo, ref)
	}
}

type ghRepo struct {
	DefaultBranch string `json:"default_branch"`
	Description   string `json:"description"`
}

type ghContent struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Path        string `json:"path"`
	DownloadURL string `json:"download_url"`
	SHA         string `json:"sha"`
}

type ghTreeItem struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type ghCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
	} `json:"commit"`
}

func fetchGitHubRoot(ctx context.Context, client *http.Client, owner, repo, ref string) (*ExtractedContent, error) {
	// Get repo info.
	info, err := ghGet[ghRepo](ctx, client, fmt.Sprintf("%s/repos/%s/%s", ghAPIBase, owner, repo))
	if err != nil {
		return nil, fmt.Errorf("github repo: %w", err)
	}

	if ref == "" {
		ref = info.DefaultBranch
	}

	// Get README.
	readmeURL := fmt.Sprintf("%s/repos/%s/%s/readme", ghAPIBase, owner, repo)
	readmeResp, err := ghGet[ghContent](ctx, client, readmeURL)
	var readmeContent string
	if err == nil && readmeResp.DownloadURL != "" {
		if rc, e := ghFetchRaw(ctx, client, readmeResp.DownloadURL); e == nil {
			readmeContent = rc
		}
	}

	// Get root tree listing.
	treeURL := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=0", ghAPIBase, owner, repo, ref)
	tree, treeListErr := ghGet[struct {
		Tree []ghTreeItem `json:"tree"`
	}](ctx, client, treeURL)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s/%s\n", owner, repo))
	if info.Description != "" {
		b.WriteString(fmt.Sprintf("\n%s\n", info.Description))
	}
	b.WriteString(fmt.Sprintf("\nDefault branch: %s\n\n", ref))

	if treeListErr == nil && len(tree.Tree) > 0 {
		b.WriteString("## Repository contents\n\n")
		for _, item := range tree.Tree {
			if item.Type == "tree" {
				b.WriteString(fmt.Sprintf("  📁 %s/\n", item.Path))
			} else {
				b.WriteString(fmt.Sprintf("  📄 %s\n", item.Path))
			}
		}
		b.WriteString("\n")
	}

	if readmeContent != "" {
		b.WriteString("## README\n\n")
		b.WriteString(readmeContent)
	}

	return &ExtractedContent{
		URL:     fmt.Sprintf("https://github.com/%s/%s", owner, repo),
		Title:   fmt.Sprintf("%s/%s", owner, repo),
		Content: b.String(),
	}, nil
}

func fetchGitHubFile(ctx context.Context, client *http.Client, owner, repo, ref, path string) (*ExtractedContent, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", ghAPIBase, owner, repo, path, ref)
	content, err := ghGet[ghContent](ctx, client, url)
	if err != nil {
		return nil, fmt.Errorf("github file: %w", err)
	}

	if content.DownloadURL == "" {
		return nil, fmt.Errorf("cannot retrieve file: %s/%s/%s", owner, repo, path)
	}

	body, err := ghFetchRaw(ctx, client, content.DownloadURL)
	if err != nil {
		return nil, fmt.Errorf("github download: %w", err)
	}

	return &ExtractedContent{
		URL:     fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", owner, repo, ref, path),
		Title:   fmt.Sprintf("%s/%s/%s", owner, repo, path),
		Content: body,
	}, nil
}

func fetchGitHubTree(ctx context.Context, client *http.Client, owner, repo, ref, path string) (*ExtractedContent, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=0", ghAPIBase, owner, repo, ref)
	tree, err := ghGet[struct {
		Tree []ghTreeItem `json:"tree"`
	}](ctx, client, url)
	if err != nil {
		return nil, fmt.Errorf("github tree: %w", err)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s/%s/%s\n\n", owner, repo, path))

	for _, item := range tree.Tree {
		if strings.HasPrefix(item.Path, path+"/") || item.Path == path {
			display := strings.TrimPrefix(item.Path, path+"/")
			if display == "" || strings.Contains(display, "/") {
				continue
			}
			if item.Type == "tree" {
				b.WriteString(fmt.Sprintf("  📁 %s/\n", display))
			} else {
				b.WriteString(fmt.Sprintf("  📄 %s\n", display))
			}
		}
	}

	return &ExtractedContent{
		URL:     fmt.Sprintf("https://github.com/%s/%s/tree/%s/%s", owner, repo, ref, path),
		Title:   fmt.Sprintf("%s/%s/%s", owner, repo, path),
		Content: b.String(),
	}, nil
}

func fetchGitHubCommit(ctx context.Context, client *http.Client, owner, repo, sha string) (*ExtractedContent, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s", ghAPIBase, owner, repo, sha)
	commit, err := ghGet[ghCommit](ctx, client, url)
	if err != nil {
		return nil, fmt.Errorf("github commit: %w", err)
	}

	return &ExtractedContent{
		URL:     fmt.Sprintf("https://github.com/%s/%s/commit/%s", owner, repo, sha),
		Title:   fmt.Sprintf("commit %s", sha[:7]),
		Content: commit.Commit.Message,
	}, nil
}

// parseGitHubURL extracts components from a GitHub URL.
// Returns owner, repo, ref, path, kind.
func parseGitHubURL(rawURL string) (owner, repo, ref, path, kind string) {
	u := strings.TrimPrefix(rawURL, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "github.com/")
	parts := strings.Split(u, "/")
	// parts = [owner, repo, ...]
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", "", ""
	}
	owner = parts[0]
	repo = strings.TrimSuffix(parts[1], ".git")

	if len(parts) < 3 {
		return owner, repo, "", "", "root"
	}

	kind = parts[2]
	switch kind {
	case "tree":
		if len(parts) < 4 {
			return owner, repo, "", "", "root"
		}
		ref = parts[3]
		if len(parts) > 4 {
			path = strings.Join(filterEmpty(parts[4:]), "/")
		}
		return owner, repo, ref, path, "tree"
	case "blob":
		if len(parts) < 5 {
			kind = "root"
			return owner, repo, "", "", "root"
		}
		ref = parts[3]
		path = strings.Join(filterEmpty(parts[4:]), "/")
		return owner, repo, ref, path, "blob"
	case "commit":
		if len(parts) < 4 {
			return owner, repo, "", "", "root"
		}
		return owner, repo, "", parts[3], "commit"
	default:
		return owner, repo, "", "", "root"
	}
}

// ghGet performs a GET request to the GitHub API and unmarshals the JSON response.
// filterEmpty removes empty strings from a slice.
func filterEmpty(s []string) []string {
	var out []string
	for _, v := range s {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func ghGet[T any](ctx context.Context, client *http.Client, url string) (T, error) {
	var zero T
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "crobot")

	resp, err := client.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return zero, fmt.Errorf("github API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var val T
	if err := json.NewDecoder(resp.Body).Decode(&val); err != nil {
		return zero, err
	}
	return val, nil
}

// ghFetchRaw downloads raw content from a URL.
func ghFetchRaw(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "crobot")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return "", err
	}
	return string(body), nil
}
