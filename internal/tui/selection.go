package tui

import (
	"encoding/base64"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// selectionState tracks mouse-based text selection in the viewport.
type selectionState struct {
	active   bool // mouse button held down, selection in progress
	finished bool // selection complete (mouse released), ready for copy
	startLine int // line index in plainLines (normalized)
	startCol  int // byte offset within plain line (normalized)
	endLine   int
	endCol    int
}

// normalize ensures start <= end (line-wise, and column-wise within same line).
func (s *selectionState) normalize() {
	if s.startLine < 0 {
		s.startLine = 0
		s.startCol = 0
	}
	if s.endLine < 0 {
		s.endLine = 0
		s.endCol = 0
	}

	if s.startLine > s.endLine || (s.startLine == s.endLine && s.startCol > s.endCol) {
		s.startLine, s.endLine = s.endLine, s.startLine
		s.startCol, s.endCol = s.endCol, s.startCol
	}
}

// clear resets the selection state.
func (s *selectionState) clear() {
	s.active = false
	s.finished = false
	s.startLine = -1
	s.startCol = 0
	s.endLine = -1
	s.endCol = 0
}

// hasSelection returns true if there is a valid selection (active or finished).
func (s *selectionState) hasSelection() bool {
	s.normalize()
	return s.startLine >= 0 && s.endLine >= s.startLine && !(s.startLine == s.endLine && s.startCol == s.endCol)
}

// selectedText extracts the selected text from plain lines.
func (s *selectionState) selectedText(plainLines []string) string {
	if !s.hasSelection() || len(plainLines) == 0 {
		return ""
	}

	s.normalize()

	startLine := s.startLine
	endLine := s.endLine
	if startLine < 0 {
		startLine = 0
	}
	if startLine >= len(plainLines) {
		return ""
	}
	if endLine >= len(plainLines) {
		endLine = len(plainLines) - 1
	}
	if endLine < startLine {
		return ""
	}

	var b strings.Builder
	for i := startLine; i <= endLine; i++ {
		line := plainLines[i]

		selStart := s.startCol
		if i > startLine {
			selStart = 0
		}
		selEnd := s.endCol
		if i < endLine {
			selEnd = len(line)
		}

		if selStart < 0 {
			selStart = 0
		}
		if selStart > len(line) {
			selStart = len(line)
		}
		if selEnd > len(line) {
			selEnd = len(line)
		}
		if selEnd < selStart {
			selEnd = selStart
		}

		if i > startLine {
			b.WriteByte('\n')
		}
		b.WriteString(line[selStart:selEnd])
	}
	return b.String()
}

// styleLineForSelection takes a styled viewport content line and wraps the selected
// portion in reverse-video ANSI codes. It uses plainOffsetToStyledPos to map plain-text
// byte offsets to positions in the styled content.
func styleLineForSelection(styledLine string, selStart, selEnd int) string {
	if selStart >= selEnd {
		return styledLine
	}

	styledStart := plainOffsetToStyledPos(styledLine, selStart)
	styledEnd := plainOffsetToStyledPos(styledLine, selEnd)

	if styledStart >= len(styledLine) {
		return styledLine
	}
	if styledEnd > len(styledLine) {
		styledEnd = len(styledLine)
	}
	if styledStart >= styledEnd {
		return styledLine
	}

	var b strings.Builder
	b.WriteString(styledLine[:styledStart])
	b.WriteString("\x1b[7m") // reverse video
	// Post-process the selected portion: after every SGR reset (ESC[0m, ESC[m),
	// re-assert reverse video so that \x1b[0m from markdown styling doesn't
	// kill the selection highlight.
	b.WriteString(fixSGRResets(styledLine[styledStart:styledEnd]))
	b.WriteString("\x1b[27m") // reverse video off
	b.WriteString(styledLine[styledEnd:])
	return b.String()
}

// fixSGRResets inserts \x1b[7m after every SGR reset sequence within s.
// SGR reset sequences are ESC[0m and ESC[m, which lipgloss emits at the end
// of styled spans. Without this fix, \x1b[0m inside a selected region would
// kill the reverse-video highlight, making the selection invisible.
func fixSGRResets(s string) string {
	if len(s) == 0 {
		return s
	}
	// Fast path: no escape characters means no resets to fix.
	if !strings.Contains(s, "\x1b") {
		return s
	}
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			// Found CSI. Check if it's a bare SGR reset: ESC[m or ESC[0m.
			csiEnd := i + 2 // points past ESC[
			isReset := false
			end := i + 2
			if csiEnd < len(s) && s[csiEnd] == 'm' {
				// ESC[m — bare reset (3 chars total)
				isReset = true
				end = i + 3
			} else if csiEnd < len(s) && s[csiEnd] == '0' {
				// Check for ESC[0m or ESC[0;...m
				j := csiEnd + 1
				for j < len(s) && s[j] != 'm' && s[j] >= 0x30 && s[j] <= 0x3f {
					j++
				}
				if j < len(s) && s[j] == 'm' {
					// First param is 0 (or 0X, 00, etc.) — this is a reset.
					isReset = true
					end = j + 1
				}
			}
			if isReset {
				// Write the reset sequence, then re-enable reverse video.
				b.WriteString(s[i:end])
				b.WriteString("\x1b[7m")
				i = end
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// overlaySelection applies reverse-video highlighting to the selected region
// across all content lines. The selection range is in plain-text line/column coordinates.
func overlaySelection(content string, sel selectionState) string {
	if !sel.hasSelection() {
		return content
	}

	sel.normalize()
	lines := strings.Split(content, "\n")

	if sel.startLine < 0 || sel.startLine >= len(lines) {
		return content
	}

	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}

		if i < sel.startLine || i > sel.endLine {
			b.WriteString(line)
			continue
		}

		plain := stripANSI(line)
		if len(plain) == 0 {
			b.WriteString(line)
			continue
		}

		var lineSelStart, lineSelEnd int
		if i == sel.startLine {
			lineSelStart = sel.startCol
		} else {
			lineSelStart = 0
		}
		if i == sel.endLine {
			lineSelEnd = sel.endCol
		} else {
			lineSelEnd = len(plain)
		}

		if lineSelStart < 0 {
			lineSelStart = 0
		}
		if lineSelEnd > len(plain) {
			lineSelEnd = len(plain)
		}

		b.WriteString(styleLineForSelection(line, lineSelStart, lineSelEnd))
	}
	return b.String()
}

// plainOffsetToStyledPos maps a byte offset in the plain (ANSI-stripped) text
// to the corresponding byte position in the original styled text.
// It walks the styled string, skipping ANSI escape sequences, counting visible characters.
func plainOffsetToStyledPos(styledLine string, plainOffset int) int {
	plainCount := 0
	i := 0
	runes := []byte(styledLine)
	for i < len(runes) {
		if runes[i] == '\x1b' {
			i++ // skip ESC
			if i < len(runes) && runes[i] == '[' {
				// CSI: skip until final byte (0x40-0x7e)
				i++
				for i < len(runes) && (runes[i] < 0x40 || runes[i] > 0x7e) {
					i++
				}
				if i < len(runes) {
					i++ // skip final byte
				}
			} else if i < len(runes) && runes[i] == ']' {
				// OSC: skip until BEL or ST
				i++
				for i < len(runes) && runes[i] != '\x07' && !(runes[i] == '\x1b' && i+1 < len(runes) && runes[i+1] == '\\') {
					i++
				}
				if i < len(runes) {
					if runes[i] == '\x07' {
						i++
					} else {
						i += 2
					}
				}
			} else if i < len(runes) {
				i++ // skip one more byte
			}
			continue
		}
		if plainCount >= plainOffset {
			return i
		}
		plainCount++
		i++
	}
	return len(styledLine)
}

// styledColToPlainOffset maps a visible column position in the styled line
// to a byte offset in the corresponding plain text.
// It walks the styled string, skipping ANSI codes, counting visible characters.
func styledColToPlainOffset(styledLine string, styledCol int) int {
	plainOffset := 0
	visibleCol := 0
	i := 0
	runes := []byte(styledLine)
	for i < len(runes) {
		if runes[i] == '\x1b' {
			i++ // skip ESC
			if i < len(runes) && runes[i] == '[' {
				// CSI: skip until final byte (0x40-0x7e)
				i++
				for i < len(runes) && (runes[i] < 0x40 || runes[i] > 0x7e) {
					i++
				}
				if i < len(runes) {
					i++ // skip final byte
				}
			} else if i < len(runes) && runes[i] == ']' {
				// OSC: skip until BEL or ST
				i++
				for i < len(runes) && runes[i] != '\x07' && !(runes[i] == '\x1b' && i+1 < len(runes) && runes[i+1] == '\\') {
					i++
				}
				if i < len(runes) {
					if runes[i] == '\x07' {
						i++
					} else {
						i += 2
					}
				}
			} else if i < len(runes) {
				i++ // skip one more byte
			}
			continue
		}
		if visibleCol >= styledCol {
			break
		}
		visibleCol++
		plainOffset++
		i++
	}
	return plainOffset
}

// copyToClipboardCmd returns a tea.Cmd that copies the given text to the system
// clipboard using the OSC 52 escape sequence. Most modern terminals (kitty, alacritty,
// ghostty, wezterm, iTerm2) support this. It also works over SSH.
func copyToClipboardCmd(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	return func() tea.Msg {
		encoded := base64.StdEncoding.EncodeToString([]byte(text))
		fmt.Printf("\x1b]52;c;%s\x07", encoded)
		return nil
	}
}

// stripANSI removes all ANSI escape sequences from a string, returning the plain text.
// Handles CSI (ESC[), OSC (ESC]), and other escape sequences.
func stripANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			i++ // skip ESC
			if i < len(s) && s[i] == '[' {
				// CSI sequence: ESC [ ... final byte (0x40-0x7e)
				i++ // skip '['
				for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
					i++
				}
				if i < len(s) {
					i++ // skip the final byte
				}
				continue
			}
			if i < len(s) && s[i] == ']' {
				// OSC sequence: ESC ] ... BEL (\x07) or ST (ESC \)
				i++ // skip ']'
				for i < len(s) && s[i] != '\x07' && !(s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\') {
					i++
				}
				if i < len(s) {
					if s[i] == '\x07' {
						i++
					} else {
						i += 2 // skip ESC and '\'
					}
				}
				continue
			}
			// Unknown escape: skip one more byte (typical for simple ESC sequences).
			if i < len(s) {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
