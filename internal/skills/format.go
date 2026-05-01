package skills

import (
	"fmt"
	"strings"
)

// FormatSkillsForPrompt emits an <available_skills> XML block for the system prompt.
// Only skills without DisableModelInvocation are included.
// Only metadata (name, description, location) is included — not the full content.
// Returns an empty string if there are no visible skills.
func FormatSkillsForPrompt(skills []Skill) string {
	// Filter out skills that cannot be auto-invoked by the model.
	var visible []Skill
	for _, s := range skills {
		if !s.DisableModelInvocation {
			visible = append(visible, s)
		}
	}

	if len(visible) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\nThe following skills provide specialized instructions for specific tasks.")
	b.WriteString("\nUse the read tool to load a skill's file when the task matches its description.")
	b.WriteString("\nWhen a skill file references a relative path, resolve it against the skill directory (parent of SKILL.md / dirname of the path) and use that absolute path in tool commands.")
	b.WriteString("\n")
	b.WriteString("\n<available_skills>")

	for _, s := range visible {
		b.WriteString("\n  <skill>")
		b.WriteString(fmt.Sprintf("\n    <name>%s</name>", escapeXML(s.Name)))
		b.WriteString(fmt.Sprintf("\n    <description>%s</description>", escapeXML(s.Description)))
		b.WriteString(fmt.Sprintf("\n    <location>%s</location>", escapeXML(s.FilePath)))
		b.WriteString("\n  </skill>")
	}

	b.WriteString("\n</available_skills>")
	b.WriteString("\n")
	return b.String()
}

// escapeXML escapes special XML characters in a string.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
