package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const splitDiffMinWidth = 88

var hunkHeaderRE = regexp.MustCompile(`^@@\s+-(\d+)(?:,(\d+))?\s+\+(\d+)(?:,(\d+))?\s+@@`)

func isDiffOutput(toolName, output string) bool {
	if toolName != "file_edit" && toolName != "file_write" {
		return false
	}
	return strings.HasPrefix(output, "--- ") && strings.Contains(output, "\n+++ ")
}

type diffLineKind int

const (
	diffMeta diffLineKind = iota
	diffHunk
	diffContext
	diffAdd
	diffRemove
)

type parsedDiffLine struct {
	kind   diffLineKind
	oldNum int
	newNum int
	text   string
	raw    string
}

type diffStats struct {
	added   int
	removed int
	hunks   int
}

func formatDiffPreview(output string, width int, maxLines int, collapsed bool, s Styles) string {
	parsed, stats := parseUnifiedDiff(output)
	if width >= splitDiffMinWidth {
		return formatSplitDiffPreview(parsed, stats, width, maxLines, collapsed, s)
	}
	return formatUnifiedDiffPreview(parsed, stats, width, maxLines, collapsed, s)
}

func parseUnifiedDiff(output string) ([]parsedDiffLine, diffStats) {
	lines := strings.Split(output, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	oldNum, newNum := 0, 0
	var stats diffStats
	parsed := make([]parsedDiffLine, 0, len(lines))
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "@@"):
			if m := hunkHeaderRE.FindStringSubmatch(line); len(m) >= 4 {
				oldNum, _ = strconv.Atoi(m[1])
				newNum, _ = strconv.Atoi(m[3])
			}
			stats.hunks++
			parsed = append(parsed, parsedDiffLine{kind: diffHunk, text: line, raw: line})
		case strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "... diff truncated"):
			parsed = append(parsed, parsedDiffLine{kind: diffMeta, text: line, raw: line})
		case strings.HasPrefix(line, "+"):
			parsed = append(parsed, parsedDiffLine{kind: diffAdd, newNum: newNum, text: strings.TrimPrefix(line, "+"), raw: line})
			newNum++
			stats.added++
		case strings.HasPrefix(line, "-"):
			parsed = append(parsed, parsedDiffLine{kind: diffRemove, oldNum: oldNum, text: strings.TrimPrefix(line, "-"), raw: line})
			oldNum++
			stats.removed++
		case strings.HasPrefix(line, " "):
			parsed = append(parsed, parsedDiffLine{kind: diffContext, oldNum: oldNum, newNum: newNum, text: strings.TrimPrefix(line, " "), raw: line})
			oldNum++
			newNum++
		default:
			parsed = append(parsed, parsedDiffLine{kind: diffMeta, text: line, raw: line})
		}
	}
	return parsed, stats
}

func formatUnifiedDiffPreview(lines []parsedDiffLine, stats diffStats, width int, maxLines int, collapsed bool, s Styles) string {
	rows := make([]string, 0, len(lines)+1)
	rows = append(rows, s.ToolMeta.Render(truncateToWidth(fmt.Sprintf("↳ diff +%d -%d • %d hunks", stats.added, stats.removed, stats.hunks), width)))
	for _, line := range lines {
		if line.kind == diffMeta || line.kind == diffHunk {
			continue
		}
		rows = append(rows, styleNumberedDiffLine(line, width, s))
	}
	return collapseDiffRows(rows, maxLines, collapsed, s)
}

func styleNumberedDiffLine(line parsedDiffLine, width int, s Styles) string {
	oldLabel, newLabel := "    ", "    "
	if line.oldNum > 0 {
		oldLabel = fmt.Sprintf("%4d", line.oldNum)
	}
	if line.newNum > 0 {
		newLabel = fmt.Sprintf("%4d", line.newNum)
	}
	prefix := "│"
	if line.kind == diffAdd {
		prefix = "▌"
	} else if line.kind == diffRemove {
		prefix = "▌"
	}
	raw := fmt.Sprintf("%s %s %s %s", oldLabel, newLabel, prefix, line.text)
	return styleDiffLine(truncateToWidth(raw, width), line.kind, s)
}

func formatSplitDiffPreview(lines []parsedDiffLine, stats diffStats, width int, maxLines int, collapsed bool, s Styles) string {
	rows := []string{s.ToolMeta.Render(truncateToWidth(fmt.Sprintf("↳ diff +%d -%d • %d hunks", stats.added, stats.removed, stats.hunks), width))}
	colWidth := (width - 3) / 2
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line.kind == diffMeta || line.kind == diffHunk {
			continue
		}
		if line.kind == diffRemove && i+1 < len(lines) && lines[i+1].kind == diffAdd {
			rows = append(rows, formatSplitRow(line, lines[i+1], colWidth, s))
			i++
			continue
		}
		if line.kind == diffAdd {
			rows = append(rows, formatSplitRow(parsedDiffLine{}, line, colWidth, s))
		} else {
			rows = append(rows, formatSplitRow(line, parsedDiffLine{}, colWidth, s))
		}
	}
	return collapseDiffRows(rows, maxLines, collapsed, s)
}

func formatSplitRow(left, right parsedDiffLine, colWidth int, s Styles) string {
	l := formatSplitCell(left, true, colWidth, s)
	r := formatSplitCell(right, false, colWidth, s)
	return l + s.Dim.Render(" │ ") + r
}

func formatSplitCell(line parsedDiffLine, left bool, width int, s Styles) string {
	if line.kind == 0 && line.raw == "" && line.text == "" {
		return s.ToolOutput.Render(fmt.Sprintf("%-*s", width, ""))
	}
	num := line.newNum
	marker := "▌"
	if left {
		num = line.oldNum
	}
	if line.kind == diffContext {
		marker = "│"
	}
	cell := truncateToWidth(fmt.Sprintf("%4d %s %s", num, marker, line.text), width)
	return styleDiffLine(fmt.Sprintf("%-*s", width, cell), line.kind, s)
}

func collapseDiffRows(rows []string, maxLines int, collapsed bool, s Styles) string {
	hidden := 0
	if len(rows) > maxLines {
		hidden = len(rows) - maxLines
		rows = rows[:maxLines]
	}
	var b strings.Builder
	for i, row := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(row)
	}
	if collapsed && hidden > 0 {
		b.WriteByte('\n')
		b.WriteString(s.ToolMeta.Render(fmt.Sprintf("… %d more lines (ctrl+o to expand)", hidden)))
	} else if hidden > 0 {
		b.WriteByte('\n')
		b.WriteString(s.ToolMeta.Render(fmt.Sprintf("… %d more lines", hidden)))
	}
	return b.String()
}

func styleDiffLine(line string, kind diffLineKind, s Styles) string {
	switch kind {
	case diffHunk:
		return s.Cyan.Render(line)
	case diffMeta:
		return s.Dim.Render(line)
	case diffAdd:
		return s.Green.Render(line)
	case diffRemove:
		return s.Red.Render(line)
	default:
		return s.ToolOutput.Render(line)
	}
}
