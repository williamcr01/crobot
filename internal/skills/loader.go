package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	maxNameLength        = 64
	maxDescriptionLength = 1024
)

// AgentsSkillsDir returns the path to the shared ~/.agents/skills/ directory.
func AgentsSkillsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".agents", "skills"), nil
}

// CrobotSkillsDir returns the path to ~/.crobot/skills/.
func CrobotSkillsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".crobot", "skills"), nil
}

// ProjectSkillsDir returns the path to ./.crobot/skills/ relative to cwd.
func ProjectSkillsDir(cwd string) string {
	return filepath.Join(cwd, ".crobot", "skills")
}

// LoadSkills loads skills from all configured locations.
// Loading order (last writer wins on name collision):
//  1. ~/.agents/skills/       (lowest priority)
//  2. ~/.crobot/skills/
//  3. ./.crobot/skills/
//  4. explicitPaths           (highest priority)
//
// If includeDefaults is false, only explicitPaths are loaded.
func LoadSkills(cwd string, explicitPaths []string, includeDefaults bool) LoadResult {
	var allSkills []Skill
	var allDiagnostics []Diagnostic
	seenNames := make(map[string]bool)
	seenRealPaths := make(map[string]bool)

	// addSkills merges a LoadResult, with last-writer-wins for name collisions.
	addSkills := func(result LoadResult) {
		allDiagnostics = append(allDiagnostics, result.Diagnostics...)
		for _, skill := range result.Skills {
			realPath, err := filepath.EvalSymlinks(skill.FilePath)
			if err != nil {
				realPath = skill.FilePath
			}
			// Skip if we already loaded this exact file (via symlink).
			if seenRealPaths[realPath] {
				continue
			}
			seenRealPaths[realPath] = true

			if prev, exists := findSkillByName(allSkills, skill.Name); exists {
				allDiagnostics = append(allDiagnostics, Diagnostic{
					Level:   "warning",
					Message: fmt.Sprintf(`skill name "%s" collision: "%s" overrides "%s"`, skill.Name, skill.FilePath, prev.FilePath),
					Path:    skill.FilePath,
					Collision: &CollisionDiagnostic{
						Name:       skill.Name,
						WinnerPath: skill.FilePath,
						LoserPath:  prev.FilePath,
					},
				})
				// Replace the previous entry.
				replaceSkill(&allSkills, skill)
			} else {
				allSkills = append(allSkills, skill)
				seenNames[skill.Name] = true
			}
		}
	}

	if includeDefaults {
		// 1. ~/.agents/skills/
		if agentsDir, err := AgentsSkillsDir(); err == nil {
			addSkills(loadSkillsFromDir(agentsDir, SourceAgents))
		}

		// 2. ~/.crobot/skills/
		if crobotDir, err := CrobotSkillsDir(); err == nil {
			addSkills(loadSkillsFromDir(crobotDir, SourceCrobot))
		}

		// 3. ./.crobot/skills/
		addSkills(loadSkillsFromDir(ProjectSkillsDir(cwd), SourceProject))
	}

	// 4. Explicit paths (--skill flag).
	for _, rawPath := range explicitPaths {
		resolved := resolvePath(rawPath, cwd)
		info, err := os.Stat(resolved)
		if err != nil {
			allDiagnostics = append(allDiagnostics, Diagnostic{
				Level:   "warning",
				Message: fmt.Sprintf("skill path does not exist: %s", resolved),
				Path:    resolved,
			})
			continue
		}
		if info.IsDir() {
			addSkills(loadSkillsFromDir(resolved, SourceExplicit))
		} else if strings.HasSuffix(resolved, ".md") {
			skill, diags := loadSkillFromFile(resolved, SourceExplicit)
			if skill != nil {
				addSkills(LoadResult{Skills: []Skill{*skill}, Diagnostics: diags})
			} else {
				for _, d := range diags {
					allDiagnostics = append(allDiagnostics, d)
				}
			}
		} else {
			allDiagnostics = append(allDiagnostics, Diagnostic{
				Level:   "warning",
				Message: fmt.Sprintf("skill path is not a markdown file: %s", resolved),
				Path:    resolved,
			})
		}
	}

	return LoadResult{
		Skills:      allSkills,
		Diagnostics: allDiagnostics,
	}
}

// loadSkillsFromDir scans a directory for SKILL.md files.
// Discovery rules:
//   - If a directory contains SKILL.md, treat it as a skill root and do not recurse.
//   - Otherwise, look for SKILL.md in subdirectories.
//   - Skips dot-prefixed entries and node_modules.
func loadSkillsFromDir(dir string, source string) LoadResult {
	info, err := os.Stat(dir)
	if err != nil {
		return LoadResult{}
	}
	if !info.IsDir() {
		return LoadResult{}
	}

	// Check if this directory IS a skill root (contains SKILL.md).
	if hasFile(dir, "SKILL.md") {
		skill, diags := loadSkillFromFile(filepath.Join(dir, "SKILL.md"), source)
		if skill != nil {
			return LoadResult{Skills: []Skill{*skill}, Diagnostics: diags}
		}
		return LoadResult{Diagnostics: diags}
	}

	// Otherwise, scan subdirectories for SKILL.md.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return LoadResult{}
	}

	var allSkills []Skill
	var allDiagnostics []Diagnostic

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if name == "node_modules" {
			continue
		}
		if !entry.IsDir() {
			continue
		}
		subDir := filepath.Join(dir, name)
		subInfo, err := os.Stat(subDir)
		if err != nil || !subInfo.IsDir() {
			continue
		}
		if !hasFile(subDir, "SKILL.md") {
			continue
		}
		skill, diags := loadSkillFromFile(filepath.Join(subDir, "SKILL.md"), source)
		if skill != nil {
			allSkills = append(allSkills, *skill)
		}
		allDiagnostics = append(allDiagnostics, diags...)
	}

	return LoadResult{Skills: allSkills, Diagnostics: allDiagnostics}
}

// loadSkillFromFile parses a single SKILL.md file.
func loadSkillFromFile(filePath string, source string) (*Skill, []Diagnostic) {
	var diagnostics []Diagnostic

	raw, err := os.ReadFile(filePath)
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Level:   "warning",
			Message: fmt.Sprintf("failed to read skill file: %v", err),
			Path:    filePath,
		})
		return nil, diagnostics
	}

	fm, body, err := parseFrontmatter(raw)
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Level:   "warning",
			Message: fmt.Sprintf("invalid frontmatter: %v", err),
			Path:    filePath,
		})
		return nil, diagnostics
	}

	skillDir := filepath.Dir(filePath)
	parentDirName := filepath.Base(skillDir)

	// Use name from frontmatter, or fall back to parent directory name.
	name := fm.Name
	if name == "" {
		name = parentDirName
	}
	description := strings.TrimSpace(fm.Description)

	// Validate description.
	descErrs := validateDescription(description)
	for _, msg := range descErrs {
		diagnostics = append(diagnostics, Diagnostic{
			Level:   "warning",
			Message: msg,
			Path:    filePath,
		})
	}

	// Validate name.
	nameErrs := validateName(name, parentDirName)
	for _, msg := range nameErrs {
		diagnostics = append(diagnostics, Diagnostic{
			Level:   "warning",
			Message: msg,
			Path:    filePath,
		})
	}

	// Skip if description is missing or empty.
	if description == "" {
		diagnostics = append(diagnostics, Diagnostic{
			Level:   "warning",
			Message: "skill skipped: description is required",
			Path:    filePath,
		})
		return nil, diagnostics
	}

	_ = body // body is not stored in Skill — it's loaded lazily

	return &Skill{
		Name:                  name,
		Description:           description,
		FilePath:              filePath,
		BaseDir:               skillDir,
		DisableModelInvocation: fm.DisableModelInvocation,
		Source:                source,
	}, diagnostics
}

// parseFrontmatter splits markdown content into frontmatter YAML and body.
func parseFrontmatter(raw []byte) (SkillFrontmatter, string, error) {
	var fm SkillFrontmatter
	text := string(raw)

	// Must start with ---.
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return fm, text, nil // no frontmatter, that's ok for validation
	}

	// Find closing ---.
	rest := text[4:] // skip "---\n"
	end := strings.Index(rest, "\n---")
	if end == -1 {
		end = strings.Index(rest, "\r\n---")
	}
	if end == -1 {
		return fm, text, nil // no closing ---
	}

	yamlPart := rest[:end]
	body := rest[end+5:] // skip "\n---" plus optional newline
	if strings.HasPrefix(body, "\n") {
		body = body[1:]
	}

	if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil {
		return fm, text, fmt.Errorf("yaml parse error: %w", err)
	}

	return fm, body, nil
}

// StripFrontmatter removes YAML frontmatter from a SKILL.md file's content.
// Used by /skill:name expansion to get just the body.
func StripFrontmatter(raw []byte) string {
	_, body, _ := parseFrontmatter(raw)
	return body
}

// validateName checks skill name rules per Agent Skills spec.
func validateName(name, parentDirName string) []string {
	var errors []string

	if name != parentDirName {
		errors = append(errors, fmt.Sprintf(`name "%s" does not match parent directory "%s"`, name, parentDirName))
	}
	if len(name) > maxNameLength {
		errors = append(errors, fmt.Sprintf("name exceeds %d characters (%d)", maxNameLength, len(name)))
	}
	if !isValidSkillName(name) {
		errors = append(errors, "name must be lowercase a-z, 0-9, hyphens only")
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		errors = append(errors, "name must not start or end with a hyphen")
	}
	if strings.Contains(name, "--") {
		errors = append(errors, "name must not contain consecutive hyphens")
	}

	return errors
}

// validateDescription checks description rules per Agent Skills spec.
func validateDescription(description string) []string {
	desc := strings.TrimSpace(description)
	if desc == "" {
		return []string{"description is required"}
	}
	if len(desc) > maxDescriptionLength {
		return []string{fmt.Sprintf("description exceeds %d characters (%d)", maxDescriptionLength, len(desc))}
	}
	return nil
}

// isValidSkillName checks that the name is lowercase alphanumeric with hyphens.
func isValidSkillName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}

// hasFile checks if a file exists at dir/filename.
func hasFile(dir, filename string) bool {
	path := filepath.Join(dir, filename)
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// resolvePath resolves a path that may use ~/ or be relative to cwd.
func resolvePath(input, cwd string) string {
	if strings.HasPrefix(input, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, input[2:])
		}
	}
	if input == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
	}
	if filepath.IsAbs(input) {
		return filepath.Clean(input)
	}
	return filepath.Join(cwd, input)
}

// findSkillByName finds a skill by name in a slice. Returns the index if found.
func findSkillByName(skills []Skill, name string) (Skill, bool) {
	for _, s := range skills {
		if s.Name == name {
			return s, true
		}
	}
	return Skill{}, false
}

// replaceSkill replaces a skill with the same name, or appends if not found.
func replaceSkill(skills *[]Skill, skill Skill) {
	for i, s := range *skills {
		if s.Name == skill.Name {
			(*skills)[i] = skill
			return
		}
	}
	*skills = append(*skills, skill)
}
