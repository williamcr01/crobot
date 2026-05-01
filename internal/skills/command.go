package skills

import (
	"fmt"
	"os"
	"strings"
)

// ExpandSkillCommand recognizes /skill:name [args] in text and expands it into
// a <skill> XML block containing the full skill content (without frontmatter).
//
// Returns (expandedText, wasExpanded).
// If the skill is not found, returns the original text unchanged.
// If reading the file fails, returns an error message as expanded text.
func ExpandSkillCommand(text string, skills []Skill) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/skill:") {
		return text, false
	}

	// Extract skill name and optional args.
	rest := trimmed[len("/skill:"):]
	spaceIdx := strings.Index(rest, " ")
	var skillName, args string
	if spaceIdx == -1 {
		skillName = rest
	} else {
		skillName = rest[:spaceIdx]
		args = strings.TrimSpace(rest[spaceIdx+1:])
	}

	// Find the skill by name.
	var skill *Skill
	for _, s := range skills {
		if s.Name == skillName {
			skill = &s
			break
		}
	}
	if skill == nil {
		return text, false // unknown skill, pass through
	}

	// Read the skill file.
	raw, err := os.ReadFile(skill.FilePath)
	if err != nil {
		return fmt.Sprintf("Error reading skill %q: %v", skill.Name, err), true
	}

	// Strip frontmatter to get the body.
	body := strings.TrimSpace(StripFrontmatter(raw))

	// Build the skill block.
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<skill name="%s" location="%s">`, skill.Name, skill.FilePath))
	b.WriteString(fmt.Sprintf("\nReferences are relative to %s.\n\n", skill.BaseDir))
	b.WriteString(body)
	b.WriteString("\n</skill>")

	if args != "" {
		b.WriteString("\n\n")
		b.WriteString(args)
	}

	return b.String(), true
}
