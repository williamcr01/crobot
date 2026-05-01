package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Helper to write a SKILL.md file with frontmatter.
func writeSkill(t *testing.T, dir, name, description string, disable bool) string {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\n"
	content += "name: " + name + "\n"
	content += "description: " + description + "\n"
	if disable {
		content += "disable-model-invocation: true\n"
	}
	content += "---\n"
	content += "# " + name + "\n\nThis is the " + name + " skill content.\n"

	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadSkillsFromDir(t *testing.T) {
	dir := t.TempDir()

	// Create two valid skills.
	writeSkill(t, dir, "git-commit", "Execute git commit with conventional commit message", false)
	writeSkill(t, dir, "web-search", "Search the web for information", false)

	result := loadSkillsFromDir(dir, SourceAgents)
	if len(result.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(result.Skills))
	}

	// Verify skill fields.
	for _, s := range result.Skills {
		if s.Name == "" {
			t.Error("skill name should not be empty")
		}
		if s.Description == "" {
			t.Errorf("skill %q description should not be empty", s.Name)
		}
		if s.FilePath == "" {
			t.Errorf("skill %q file path should not be empty", s.Name)
		}
		if s.BaseDir == "" {
			t.Errorf("skill %q base dir should not be empty", s.Name)
		}
		if s.Source != SourceAgents {
			t.Errorf("skill %q source should be %q, got %q", s.Name, SourceAgents, s.Source)
		}
	}
}

func TestLoadSkillsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	result := loadSkillsFromDir(dir, SourceAgents)
	if len(result.Skills) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(result.Skills))
	}
}

func TestLoadSkillsNonexistentDir(t *testing.T) {
	result := loadSkillsFromDir("/nonexistent/path", SourceAgents)
	if len(result.Skills) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(result.Skills))
	}
}

func TestLoadSkillMissingDescription(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "no-desc")
	os.MkdirAll(skillDir, 0o755)
	content := "---\nname: no-desc\n---\n# No Description\n"
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)

	result := loadSkillsFromDir(dir, SourceAgents)
	if len(result.Skills) != 0 {
		t.Fatalf("expected 0 skills (missing description), got %d", len(result.Skills))
	}
	if len(result.Diagnostics) == 0 {
		t.Fatal("expected diagnostics for missing description")
	}
}

func TestLoadSkillInvalidName(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "INVALID_NAME")
	os.MkdirAll(skillDir, 0o755)
	content := "---\nname: INVALID_NAME\ndescription: A skill with invalid name\n---\n# Invalid\n"
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)

	result := loadSkillsFromDir(dir, SourceAgents)
	// Should still load despite name warning.
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill (loaded despite name warning), got %d", len(result.Skills))
	}
	// But should have a diagnostic for the name mismatch.
	hasNameDiag := false
	for _, d := range result.Diagnostics {
		if strings.Contains(d.Message, "name") {
			hasNameDiag = true
			break
		}
	}
	if !hasNameDiag {
		t.Error("expected a diagnostic about invalid name")
	}
}

func TestLoadSkillsNameCollision(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()

	writeSkill(t, first, "calendar", "Calendar from first dir", false)
	writeSkill(t, second, "calendar", "Calendar from second dir", false)

	// Load both with explicit paths.
	result := LoadSkills(
		"",
		[]string{first, second},
		false, // no defaults
	)

	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill after dedup, got %d", len(result.Skills))
	}

	// Last writer wins: second should win.
	if result.Skills[0].Source != SourceExplicit || result.Skills[0].Name != "calendar" {
		t.Fatalf("unexpected winner: %+v", result.Skills[0])
	}

	// Should have collision diagnostic.
	hasCollision := false
	for _, d := range result.Diagnostics {
		if d.Collision != nil && d.Collision.Name == "calendar" {
			hasCollision = true
			break
		}
	}
	if !hasCollision {
		t.Error("expected collision diagnostic")
	}
}

func TestLoadSkillsSymlinkDedup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping symlink test in short mode")
	}
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	linkDir := filepath.Join(dir, "link")

	writeSkill(t, realDir, "my-skill", "Real skill", false)

	// Create a symlink to the real directory.
	if err := os.Symlink(filepath.Join(realDir, "my-skill"), filepath.Join(linkDir, "my-skill")); err != nil {
		t.Skip("symlink creation failed:", err)
	}

	result := LoadSkills(
		"",
		[]string{realDir, linkDir},
		false,
	)

	// Should only have one instance of the skill despite loading both paths.
	if len(result.Skills) != 1 {
		t.Fatalf("expected 1 skill after symlink dedup, got %d", len(result.Skills))
	}
}

func TestFormatSkillsForPrompt(t *testing.T) {
	skills := []Skill{
		{Name: "git-commit", Description: "Commit changes", FilePath: "/a/b/git-commit/SKILL.md", BaseDir: "/a/b/git-commit"},
		{Name: "web-search", Description: "Search the web", FilePath: "/a/b/web-search/SKILL.md", BaseDir: "/a/b/web-search"},
	}

	output := FormatSkillsForPrompt(skills)

	if !strings.Contains(output, "<available_skills>") {
		t.Error("expected <available_skills> tag")
	}
	if !strings.Contains(output, "git-commit") {
		t.Error("expected git-commit skill name")
	}
	if !strings.Contains(output, "web-search") {
		t.Error("expected web-search skill name")
	}
	if !strings.Contains(output, "Commit changes") {
		t.Error("expected git-commit description")
	}
	if !strings.Contains(output, "/a/b/git-commit/SKILL.md") {
		t.Error("expected git-commit file path")
	}
}

func TestFormatSkillsForPromptDisableModelInvocation(t *testing.T) {
	skills := []Skill{
		{Name: "auto-skill", Description: "Auto invocable"},
		{Name: "manual-skill", Description: "Manual only", DisableModelInvocation: true},
	}

	output := FormatSkillsForPrompt(skills)

	if !strings.Contains(output, "auto-skill") {
		t.Error("expected auto-skill in prompt")
	}
	if strings.Contains(output, "manual-skill") {
		t.Error("manual-skill should not appear in prompt")
	}
}

func TestFormatSkillsForPromptEmpty(t *testing.T) {
	if s := FormatSkillsForPrompt(nil); s != "" {
		t.Errorf("expected empty string for nil skills, got %q", s)
	}
	if s := FormatSkillsForPrompt([]Skill{}); s != "" {
		t.Errorf("expected empty string for empty skills, got %q", s)
	}
}

func TestFormatSkillsForPromptXMLEscape(t *testing.T) {
	skills := []Skill{
		{
			Name:        "test-skill",
			Description: "Escape <test> & 'quotes'",
			FilePath:    "/a/b/test/SKILL.md",
		},
	}
	output := FormatSkillsForPrompt(skills)
	if strings.Contains(output, "<test>") {
		t.Error("description should have XML-escaped <test>")
	}
	if !strings.Contains(output, "&lt;test&gt;") {
		t.Error("expected &lt;test&gt; in output")
	}
}

func TestExpandSkillCommand(t *testing.T) {
	dir := t.TempDir()
	path := writeSkill(t, dir, "test-skill", "A test skill", false)

	// Read back to verify write.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("skill file is empty")
	}

	skills := []Skill{
		{
			Name:        "test-skill",
			Description: "A test skill",
			FilePath:    path,
			BaseDir:     filepath.Dir(path),
		},
	}

	t.Run("simple invocation", func(t *testing.T) {
		result, expanded := ExpandSkillCommand("/skill:test-skill", skills)
		if !expanded {
			t.Fatal("expected expansion")
		}
		if !strings.Contains(result, "<skill name=\"test-skill\"") {
			t.Error("expected skill XML block")
		}
		if !strings.Contains(result, "skill content") {
			t.Error("expected skill content")
		}
	})

	t.Run("with args", func(t *testing.T) {
		result, expanded := ExpandSkillCommand("/skill:test-skill please help", skills)
		if !expanded {
			t.Fatal("expected expansion")
		}
		if !strings.Contains(result, "please help") {
			t.Error("expected args after skill block")
		}
	})

	t.Run("unknown skill", func(t *testing.T) {
		result, expanded := ExpandSkillCommand("/skill:nonexistent", skills)
		if expanded {
			t.Fatal("should not expand unknown skill")
		}
		if result != "/skill:nonexistent" {
			t.Error("should return original text unchanged")
		}
	})

	t.Run("not a skill command", func(t *testing.T) {
		result, expanded := ExpandSkillCommand("regular message", skills)
		if expanded {
			t.Fatal("should not expand regular message")
		}
		if result != "regular message" {
			t.Error("should return original text unchanged")
		}
	})
}

func TestLoadSkillsFull(t *testing.T) {
	// Create a temp directory structure mimicking the real layout.
	root := t.TempDir()

	// ~/.agents/skills/ (lowest priority)
	agentsDir := filepath.Join(root, "agents", "skills")
	writeSkill(t, agentsDir, "common", "Common skill from agents", false)

	// ~/.crobot/skills/
	crobotDir := filepath.Join(root, "crobot", "skills")
	writeSkill(t, crobotDir, "common", "Common skill from crobot", false) // same name, should override
	writeSkill(t, crobotDir, "crobot-only", "Only in crobot", true)

	// ./.crobot/skills/ (project-local, should override both)
	projectDir := filepath.Join(root, "project", ".crobot", "skills")
	writeSkill(t, projectDir, "common", "Common skill from project", false)
	writeSkill(t, projectDir, "project-only", "Only in project", false)

	// Manually load in order to test priority.
	var allSkills []Skill
	var allDiags []Diagnostic

	// Simulate the load order.
	for _, load := range []struct {
		dir    string
		source string
	}{
		{agentsDir, SourceAgents},
		{crobotDir, SourceCrobot},
		{projectDir, SourceProject},
	} {
		r := loadSkillsFromDir(load.dir, load.source)
		allDiags = append(allDiags, r.Diagnostics...)
		for _, s := range r.Skills {
			// Check for collision with existing.
			if prev, exists := findSkillByName(allSkills, s.Name); exists {
				allDiags = append(allDiags, Diagnostic{
					Level:   "warning",
					Message: `collision: "` + s.FilePath + `" overrides "` + prev.FilePath + `"`,
					Path:    s.FilePath,
					Collision: &CollisionDiagnostic{
						Name:       s.Name,
						WinnerPath: s.FilePath,
						LoserPath:  prev.FilePath,
					},
				})
				replaceSkill(&allSkills, s)
			} else {
				allSkills = append(allSkills, s)
			}
		}
	}

	// "common" should be from project (highest priority).
	common, found := findSkillByName(allSkills, "common")
	if !found {
		t.Fatal("common skill not found")
	}
	if common.Source != SourceProject {
		t.Errorf("expected common from project, got source=%q", common.Source)
	}

	// Should have collision diagnostics.
	collisionCount := 0
	for _, d := range allDiags {
		if d.Collision != nil {
			collisionCount++
		}
	}
	if collisionCount != 2 {
		t.Errorf("expected 2 collisions (agents->crobot, crobot->project), got %d", collisionCount)
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		parent  string
		wantErr bool
	}{
		{"git-commit", "git-commit", false},
		{"my-skill", "my-skill", false},
		{"test123", "test123", false},
		{"a", "a", false},
		{"UPPERCASE", "UPPERCASE", true}, // uppercase is invalid
		{"has spaces", "has spaces", true},
		{"-leading", "-leading", true},
		{"trailing-", "trailing-", true},
		{"double--hyphen", "double--hyphen", true},
		{"mismatch", "different-dir", true},
	}

	for _, tt := range tests {
		errs := validateName(tt.name, tt.parent)
		hasErr := len(errs) > 0
		if hasErr != tt.wantErr {
			t.Errorf("validateName(%q, %q): got errs=%v, wantErr=%v", tt.name, tt.parent, errs, tt.wantErr)
		}
	}
}

func TestValidateDescription(t *testing.T) {
	if errs := validateDescription(""); len(errs) == 0 {
		t.Error("expected error for empty description")
	}
	if errs := validateDescription("   "); len(errs) == 0 {
		t.Error("expected error for whitespace-only description")
	}
	if errs := validateDescription("A valid description"); len(errs) > 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
	longDesc := strings.Repeat("x", maxDescriptionLength+1)
	if errs := validateDescription(longDesc); len(errs) == 0 {
		t.Error("expected error for too-long description")
	}
}

func TestParseFrontmatter(t *testing.T) {
	t.Run("valid frontmatter", func(t *testing.T) {
		raw := []byte("---\nname: test\ndescription: A test\n---\n\n# Body content")
		fm, body, err := parseFrontmatter(raw)
		if err != nil {
			t.Fatal(err)
		}
		if fm.Name != "test" {
			t.Errorf("expected name=test, got %q", fm.Name)
		}
		if fm.Description != "A test" {
			t.Errorf("expected description='A test', got %q", fm.Description)
		}
		if body != "# Body content" {
			t.Errorf("expected body='# Body content', got %q", body)
		}
	})

	t.Run("no frontmatter", func(t *testing.T) {
		raw := []byte("# Just a heading")
		_, body, err := parseFrontmatter(raw)
		if err != nil {
			t.Fatal(err)
		}
		if body != "# Just a heading" {
			t.Errorf("expected body unchanged, got %q", body)
		}
	})

	t.Run("disable model invocation", func(t *testing.T) {
		raw := []byte("---\nname: manual\ndescription: Manual skill\ndisable-model-invocation: true\n---\n\nBody")
		fm, _, err := parseFrontmatter(raw)
		if err != nil {
			t.Fatal(err)
		}
		if !fm.DisableModelInvocation {
			t.Error("expected disable-model-invocation to be true")
		}
	})

	t.Run("name falls back to parent dir", func(t *testing.T) {
		raw := []byte("---\ndescription: No name in frontmatter\n---\n\nBody")
		fm, _, err := parseFrontmatter(raw)
		if err != nil {
			t.Fatal(err)
		}
		if fm.Name != "" {
			t.Errorf("expected empty name, got %q", fm.Name)
		}
	})
}

func TestStripFrontmatter(t *testing.T) {
	raw := []byte("---\nname: test\ndescription: Test\n---\n\n# Real content\n\nMore content.")
	body := StripFrontmatter(raw)
	if !strings.Contains(body, "# Real content") {
		t.Error("expected body to contain real content")
	}
	if strings.Contains(body, "name: test") {
		t.Error("body should not contain frontmatter")
	}
}
