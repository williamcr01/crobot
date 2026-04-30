package prompt

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"crobot/internal/config"
)

// Build assembles the system prompt from the user config, replacing placeholders
// and appending dynamic context about the current environment.
func Build(cfg config.AgentConfig, cwd string) string {
	prompt := cfg.SystemPrompt
	if prompt == "" {
		prompt = config.DEFAULTS.SystemPrompt
	}

	if cfg.AppendPrompt != "" {
		prompt += "\n\n" + cfg.AppendPrompt
	}

	// Replace {cwd} with actual working directory.
	prompt = strings.ReplaceAll(prompt, "{cwd}", cwd)

	// Append project context files discovered from cwd and its ancestors.
	contextFiles := loadProjectContextFiles(cwd)

	// Append dynamic context.
	var b strings.Builder
	b.WriteString(prompt)
	if len(contextFiles) > 0 {
		b.WriteString("\n\n# Project Context\n\n")
		b.WriteString("Project-specific instructions and guidelines:\n\n")
		for _, file := range contextFiles {
			b.WriteString("## ")
			b.WriteString(file.path)
			b.WriteString("\n\n")
			b.WriteString(file.content)
			if !strings.HasSuffix(file.content, "\n") {
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString("Current date: ")
	b.WriteString(time.Now().Format("2006-01-02 15:04 MST"))
	b.WriteString("\n")
	b.WriteString("Shell: ")
	if bash := os.Getenv("SHELL"); bash != "" {
		b.WriteString(bash)
	} else {
		b.WriteString("unknown")
	}
	b.WriteString("\n")
	b.WriteString("Platform: ")
	b.WriteString(runtime.GOOS)
	b.WriteString("/")
	b.WriteString(runtime.GOARCH)
	b.WriteString("\n")

	return b.String()
}

type contextFile struct {
	path    string
	content string
}

func loadProjectContextFiles(cwd string) []contextFile {
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		absCwd = cwd
	}

	var files []contextFile
	seen := make(map[string]bool)

	for _, dir := range ancestorDirs(absCwd) {
		file, ok := loadContextFileFromDir(dir)
		if !ok || seen[file.path] {
			continue
		}
		files = append(files, file)
		seen[file.path] = true
	}

	return files
}

func ancestorDirs(cwd string) []string {
	var dirs []string
	current := filepath.Clean(cwd)
	for {
		dirs = append(dirs, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	// Return root-to-cwd so broader rules appear before more specific rules.
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}
	return dirs
}

func loadContextFileFromDir(dir string) (contextFile, bool) {
	candidates := []string{"AGENTS.md", "AGENTS.MD", "AGENT.md", "AGENT.MD"}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		content, err := os.ReadFile(path)
		if err == nil {
			return contextFile{path: path, content: string(content)}, true
		}
		if !os.IsNotExist(err) {
			continue
		}
	}
	return contextFile{}, false
}
