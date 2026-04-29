package tui

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	gast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// RenderMarkdown parses markdown text and renders it with Lipgloss styles
// for terminal display, wrapping to the given width.
func RenderMarkdown(text string, width int) string {
	if width < 20 {
		width = 20
	}
	innerWidth := width - 4
	if innerWidth < 16 {
		innerWidth = 16
	}

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
	)
	md.SetRenderer(
		renderer.NewRenderer(
			renderer.WithNodeRenderers(
				util.Prioritized(newNodeRenderer(innerWidth), 1000),
			),
		),
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(text), &buf); err != nil {
		return text // fallback
	}
	return buf.String()
}

// --- Node renderer ---

type mdRenderer struct {
	width int
}

func newNodeRenderer(width int) renderer.NodeRenderer {
	return &mdRenderer{width: width}
}

func (r *mdRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	// Block nodes.
	reg.Register(ast.KindDocument, r.renderDocument)
	reg.Register(ast.KindHeading, r.renderHeading)
	reg.Register(ast.KindParagraph, r.renderParagraph)
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
	reg.Register(ast.KindList, r.renderList)
	reg.Register(ast.KindListItem, r.renderListItem)
	reg.Register(ast.KindThematicBreak, r.renderThematicBreak)

	// GFM tables.
	reg.Register(gast.KindTable, r.renderTable)
	reg.Register(gast.KindTableHeader, r.renderTableHeader)
	reg.Register(gast.KindTableRow, r.renderTableRow)
	reg.Register(gast.KindTableCell, r.renderTableCell)

	// We handle inline nodes ourselves in collectInline, but register
	// them as no-ops so goldmark knows we handle them.
	reg.Register(ast.KindText, r.renderText)
	reg.Register(ast.KindString, r.renderText)
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
	reg.Register(ast.KindTextBlock, r.renderTextBlock)
}

// --- Document ---

func (r *mdRenderer) renderDocument(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}

// --- Heading ---

func (r *mdRenderer) renderHeading(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	n := node.(*ast.Heading)
	text := collectText(source, n)

	style := H4Style
	switch n.Level {
	case 1:
		style = H1Style
	case 2:
		style = H2Style
	case 3:
		style = H3Style
	}

	// Render inline formatting within heading.
	styled := collectInline(source, n, lipgloss.Style{})
	wrapped := wrapLine(styled, r.width)

	w.WriteString("\n")
	w.WriteString(style.Render(wrapped))
	if n.Level == 1 {
		w.WriteString("\n")
	}
	_ = text
	return ast.WalkSkipChildren, nil
}

// --- Paragraph ---

func (r *mdRenderer) renderParagraph(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	styled := collectInline(source, node, lipgloss.Style{})
	wrapped := wrapLine(styled, r.width)
	if strings.TrimSpace(wrapped) == "" {
		return ast.WalkSkipChildren, nil
	}

	w.WriteString("\n")
	w.WriteString(BodyTextStyle.Render(wrapped))
	w.WriteString("\n")
	return ast.WalkSkipChildren, nil
}

// --- Inline text ---

func (r *mdRenderer) renderText(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	// Handled by collectInline — no-op here.
	return ast.WalkContinue, nil
}

// --- Code block ---

func (r *mdRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	n := node.(*ast.FencedCodeBlock)
	var lang string
	if n.Info != nil {
		info := n.Info.Text(source)
		lang = string(bytes.TrimSpace(info))
	}

	// Get code body: lines between fences.
	lines := n.Lines()
	codeWidth := r.width - 2
	if codeWidth < 10 {
		codeWidth = 10
	}

	var codeBuf bytes.Buffer
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		codeBuf.Write(line.Value(source))
	}

	codeText := codeBuf.String()
	// Trim trailing newline.
	codeText = strings.TrimRight(codeText, "\n")

	w.WriteString("\n")
	if lang != "" {
		w.WriteString(Dim.Render("  " + lang))
		w.WriteString("\n")
	}

	var bodyBuf strings.Builder
	for _, line := range strings.Split(codeText, "\n") {
		if bodyBuf.Len() > 0 {
			bodyBuf.WriteByte('\n')
		}
		bodyBuf.WriteString(truncateToWidth(line, codeWidth))
	}

	blockStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#1a1a2e")).
		Width(r.width).
		Padding(0, 1)

	w.WriteString(blockStyle.Render(CodeBlockStyle.Render(bodyBuf.String())))
	w.WriteString("\n")
	return ast.WalkSkipChildren, nil
}

// --- Blockquote ---

func (r *mdRenderer) renderBlockquote(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		w.WriteString("\n")
		return ast.WalkContinue, nil
	}

	w.WriteString("\n")
	quoteWidth := r.width - 2
	if quoteWidth < 10 {
		quoteWidth = 10
	}

	// Collect all paragraph content within the blockquote.
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() == ast.KindParagraph {
			text := collectInline(source, child, lipgloss.Style{})
			wrapped := wrapLine(text, quoteWidth)
			for _, wl := range strings.Split(wrapped, "\n") {
				w.WriteString(QuoteBar.Render("│ "))
				w.WriteString(QuoteStyle.Render(wl))
				w.WriteString("\n")
			}
		}
	}
	return ast.WalkSkipChildren, nil
}

// --- Lists ---

func (r *mdRenderer) renderList(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		w.WriteString("\n")
	}
	return ast.WalkContinue, nil
}

func (r *mdRenderer) renderListItem(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	n := node.(*ast.ListItem)
	parent := node.Parent()
	isOrdered := parent.Kind() == ast.KindList && parent.(*ast.List).IsOrdered()

	// Determine bullet.
	bullet := "• "
	if isOrdered {
		// Count previous siblings to determine item number.
		num := parent.(*ast.List).Start
		for sib := node.PreviousSibling(); sib != nil; sib = sib.PreviousSibling() {
			num++
		}
		bullet = fmt.Sprintf("%d. ", num)
	}

	// Check for task checkbox.
	isTask := false
	taskDone := false
	firstChild := n.FirstChild()
	if firstChild != nil && firstChild.Kind() == ast.KindText {
		text := string(firstChild.Text(source))
		if strings.HasPrefix(text, "[ ] ") {
			isTask = true
			taskDone = false
			// Replace the text node content by trimming the prefix (handled below).
		} else if strings.HasPrefix(text, "[x] ") || strings.HasPrefix(text, "[X] ") {
			isTask = true
			taskDone = true
		}
	}

	// Indent based on nesting level.
	indent := ""
	ancestor := node.Parent()
	depth := 0
	for ancestor != nil {
		if ancestor.Kind() == ast.KindList {
			depth++
		}
		ancestor = ancestor.Parent()
	}
	if depth > 0 {
		indent = strings.Repeat("  ", depth-1)
	}

	listWidth := r.width - len(indent) - 2 // 2 for bullet
	if listWidth < 10 {
		listWidth = 10
	}

	// Collect text from paragraph/textblock children, and render nested lists.
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() == ast.KindList {
			// Nested list: recurse with extra indent.
			r.renderNestedList(w, source, child, indent+"  ")
			continue
		}
		if child.Kind() != ast.KindParagraph && child.Kind() != ast.KindTextBlock {
			continue
		}

		text := collectInline(source, child, lipgloss.Style{})

		// Trim task checkbox prefix from the raw text.
		if isTask {
			if taskDone {
				text = strings.TrimPrefix(text, "[x] ")
				text = strings.TrimPrefix(text, "[X] ")
				text = strings.TrimSpace(text)
				text = TaskDoneStyle.Render("☒ ") + BodyTextStyle.Render(text)
			} else {
				text = strings.TrimPrefix(text, "[ ] ")
				text = strings.TrimSpace(text)
				text = TaskOpenStyle.Render("☐ ") + BodyTextStyle.Render(text)
			}
			wrapped := wrapLine(text, listWidth)
			for _, wl := range strings.Split(wrapped, "\n") {
				w.WriteString(indent)
				w.WriteString(wl)
				w.WriteString("\n")
			}
		} else {
			wrapped := wrapLine(text, listWidth)
			for i, wl := range strings.Split(wrapped, "\n") {
				w.WriteString(indent)
				if i == 0 {
					w.WriteString(BodyTextStyle.Render(bullet))
				} else {
					w.WriteString(strings.Repeat(" ", len(bullet)))
				}
				w.WriteString(BodyTextStyle.Render(wl))
				w.WriteString("\n")
			}
		}
	}
	return ast.WalkSkipChildren, nil
}

// renderNestedList renders a sub-list with extra indentation.
func (r *mdRenderer) renderNestedList(w util.BufWriter, source []byte, node ast.Node, indent string) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() != ast.KindListItem {
			continue
		}
		n := child.(*ast.ListItem)
		parent := node
		isOrdered := parent.Kind() == ast.KindList && parent.(*ast.List).IsOrdered()
		bullet := "• "
		if isOrdered {
			// Count previous siblings to determine item number.
			num := parent.(*ast.List).Start
			for sib := child.PreviousSibling(); sib != nil; sib = sib.PreviousSibling() {
				num++
			}
			bullet = fmt.Sprintf("%d. ", num)
		}

		for gc := n.FirstChild(); gc != nil; gc = gc.NextSibling() {
			if gc.Kind() == ast.KindList {
				r.renderNestedList(w, source, gc, indent+"  ")
				continue
			}
			if gc.Kind() != ast.KindParagraph && gc.Kind() != ast.KindTextBlock {
				continue
			}
			text := collectInline(source, gc, lipgloss.Style{})
			wrapped := wrapLine(text, r.width-len(indent)-2)
			for i, wl := range strings.Split(wrapped, "\n") {
				w.WriteString(indent)
				if i == 0 {
					w.WriteString(BodyTextStyle.Render(bullet))
				} else {
					w.WriteString(strings.Repeat(" ", len(bullet)))
				}
				w.WriteString(BodyTextStyle.Render(wl))
				w.WriteString("\n")
			}
		}
	}
}

// --- Thematic break ---

func (r *mdRenderer) renderThematicBreak(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		w.WriteString("\n")
		w.WriteString(HRStyle.Render(strings.Repeat("─", r.width)))
		w.WriteString("\n")
	}
	return ast.WalkContinue, nil
}

// --- HTML block ---

func (r *mdRenderer) renderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		raw := string(node.Text(source))
		// Strip HTML tags, keep text.
		text := stripHTMLTags(raw)
		text = strings.TrimSpace(text)
		if text != "" {
			wrapped := wrapLine(text, r.width)
			w.WriteString("\n")
			w.WriteString(BodyTextStyle.Render(wrapped))
			w.WriteString("\n")
		}
	}
	return ast.WalkSkipChildren, nil
}

// --- Text block (inside list items) ---

func (r *mdRenderer) renderTextBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	// Handled by renderListItem via collectInline.
	return ast.WalkContinue, nil
}

// --- Table ---

func (r *mdRenderer) renderTable(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		w.WriteString("\n")
		return ast.WalkContinue, nil
	}

	// Collect all cells as raw (unwrapped) text.
	type rowData struct {
		cells []string
	}
	var headerRows []rowData
	var bodyRows []rowData
	var alignments []alignment

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case gast.KindTableHeader:
			rd := rowData{}
			for cell := child.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if cell.Kind() == gast.KindTableCell {
					tc := cell.(*gast.TableCell)
					rd.cells = append(rd.cells, collectInline(source, cell, lipgloss.Style{}))
					alignments = append(alignments, tableAlign(tc.Alignment))
				}
			}
			headerRows = append(headerRows, rd)
		case gast.KindTableRow:
			rd := rowData{}
			for cell := child.FirstChild(); cell != nil; cell = cell.NextSibling() {
				if cell.Kind() == gast.KindTableCell {
					rd.cells = append(rd.cells, collectInline(source, cell, lipgloss.Style{}))
				}
			}
			bodyRows = append(bodyRows, rd)
		}
	}

	maxCols := 0
	for _, row := range headerRows {
		if len(row.cells) > maxCols {
			maxCols = len(row.cells)
		}
	}
	for _, row := range bodyRows {
		if len(row.cells) > maxCols {
			maxCols = len(row.cells)
		}
	}
	if maxCols == 0 {
		return ast.WalkSkipChildren, nil
	}
	for len(alignments) < maxCols {
		alignments = append(alignments, alignLeft)
	}

	// Compute column widths: for each cell, wrap at available width, find longest line.
	colWidths := make([]int, maxCols)
	for _, row := range headerRows {
		for i, cell := range row.cells {
			colWidths[i] = max(colWidths[i], longestLine(cell))
		}
	}
	for _, row := range bodyRows {
		for i, cell := range row.cells {
			if i < maxCols {
				colWidths[i] = max(colWidths[i], longestLine(cell))
			}
		}
	}
	for i := range colWidths {
		if colWidths[i] < 3 {
			colWidths[i] = 3
		}
	}

	// Shrink to fit.
	totalSep := (maxCols - 1) * 3
	totalPad := 2
	available := r.width - totalSep - totalPad
	if available < maxCols*3 {
		available = maxCols * 3
	}
	colWidths = shrinkColumns(colWidths, available)

	// Wrap each cell at its column width.
	wrapCells := func(row rowData) [][]string {
		wrapped := make([][]string, maxCols)
		for i, cell := range row.cells {
			wrapped[i] = wrapToLines(cell, colWidths[i])
		}
		return wrapped
	}

	// Render.
	w.WriteString("\n")
	w.WriteString(renderTableTopBorder(colWidths))
	w.WriteString("\n")

	for _, row := range headerRows {
		writeTableRowLines(w, wrapCells(row), colWidths, alignments, TableHeader)
	}
	if len(headerRows) > 0 {
		w.WriteString(renderTableSepBorder(colWidths))
		w.WriteString("\n")
	}

	for _, row := range bodyRows {
		writeTableRowLines(w, wrapCells(row), colWidths, alignments, TableCell)
	}

	w.WriteString(renderTableBotBorder(colWidths))
	return ast.WalkSkipChildren, nil
}

func tableAlign(a gast.Alignment) alignment {
	switch a {
	case gast.AlignCenter:
		return alignCenter
	case gast.AlignRight:
		return alignRight
	default:
		return alignLeft
	}
}

func (r *mdRenderer) renderTableHeader(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil // handled by renderTable
}

func (r *mdRenderer) renderTableRow(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil // handled by renderTable
}

func (r *mdRenderer) renderTableCell(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil // handled by renderTable
}

// --- Inline collector ---

// collectInline recursively collects styled text from an AST node and its children.
func collectInline(source []byte, node ast.Node, parentStyle lipgloss.Style) string {
	var b strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case ast.KindText, ast.KindString:
			b.WriteString(parentStyle.Render(string(child.Text(source))))

		case ast.KindEmphasis:
			n := child.(*ast.Emphasis)
			if n.Level == 2 {
				style := parentStyle.Inherit(BoldStyle)
				b.WriteString(collectInline(source, child, style))
			} else {
				style := parentStyle.Inherit(ItalicStyle)
				b.WriteString(collectInline(source, child, style))
			}

		case ast.KindCodeSpan:
			style := parentStyle.Inherit(CodeStyle)
			inner := collectInline(source, child, style)
			b.WriteString(inner)

		case ast.KindLink:
			style := parentStyle.Inherit(LinkStyle)
			inner := collectInline(source, child, style)
			b.WriteString(inner)

		case ast.KindImage:
			alt := collectText(source, child)
			b.WriteString(ImageStyle.Render("[img: " + alt + "]"))

		case gast.KindStrikethrough:
			style := parentStyle.Inherit(StrikeStyle)
			b.WriteString(collectInline(source, child, style))

		case ast.KindRawHTML:
			raw := string(child.Text(source))
			// Replace <br> variants with newlines before stripping tags.
			raw = strings.ReplaceAll(raw, "<br/>", "\n")
			raw = strings.ReplaceAll(raw, "<br />", "\n")
			raw = strings.ReplaceAll(raw, "<br>", "\n")
			b.WriteString(stripHTMLTags(raw))

		default:
			// Recursively collect from unknown nodes.
			inner := collectInline(source, child, parentStyle)
			b.WriteString(inner)
		}
	}
	return b.String()
}

// collectText collects plain text from a node and all children.
func collectText(source []byte, node ast.Node) string {
	var b strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case ast.KindText, ast.KindString:
			b.Write(child.Text(source))
		default:
			b.WriteString(collectText(source, child))
		}
	}
	return b.String()
}

// --- Table rendering helpers ---

type alignment int

const (
	alignLeft alignment = iota
	alignCenter
	alignRight
)

func renderTableTopBorder(widths []int) string {
	return renderTableFrame(widths, "┌", "┬", "┐", "─")
}

func renderTableSepBorder(widths []int) string {
	return renderTableFrame(widths, "├", "┼", "┤", "─")
}

func renderTableBotBorder(widths []int) string {
	return renderTableFrame(widths, "└", "┴", "┘", "─")
}

func renderTableFrame(widths []int, left, mid, right, horiz string) string {
	var b strings.Builder
	b.WriteString(TableBorder.Render(left))
	for i, w := range widths {
		if i > 0 {
			b.WriteString(TableBorder.Render(mid))
		}
		b.WriteString(TableBorder.Render(strings.Repeat(horiz, w+2)))
	}
	b.WriteString(TableBorder.Render(right))
	return b.String()
}

// writeTableRowLines renders a table row that may span multiple lines per cell.
func writeTableRowLines(w util.BufWriter, cells [][]string, widths []int, aligns []alignment, style lipgloss.Style) {
	// Determine max line count for this row.
	maxLines := 0
	for _, cellLines := range cells {
		if len(cellLines) > maxLines {
			maxLines = len(cellLines)
		}
	}
	if maxLines == 0 {
		maxLines = 1
	}

	for lineIdx := 0; lineIdx < maxLines; lineIdx++ {
		var b strings.Builder
		b.WriteString(TableBorder.Render("│ "))
		for colIdx, cellLines := range cells {
			if colIdx > 0 {
				b.WriteString(TableBorder.Render(" │ "))
			}
			text := ""
			if lineIdx < len(cellLines) {
				text = cellLines[lineIdx]
			}
			padded := padCell(text, widths[colIdx], alignFor(aligns, colIdx))
			b.WriteString(style.Render(padded))
		}
		b.WriteString(TableBorder.Render(" │"))
		b.WriteByte('\n')
		w.WriteString(b.String())
	}
}

func alignFor(aligns []alignment, i int) alignment {
	if i < len(aligns) {
		return aligns[i]
	}
	return alignLeft
}

func padRow(row []string, count int) []string {
	for len(row) < count {
		row = append(row, "")
	}
	return row
}

func padCell(text string, width int, align alignment) string {
	textLen := displayWidth(text)
	if textLen > width {
		return truncateToWidth(text, width)
	}
	pad := width - textLen
	switch align {
	case alignCenter:
		left := pad / 2
		right := pad - left
		return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
	case alignRight:
		return strings.Repeat(" ", pad) + text
	default:
		return text + strings.Repeat(" ", pad)
	}
}

// longestLine returns the display width of the longest line in a (possibly multi-line) string.
func longestLine(text string) int {
	longest := 0
	for _, line := range strings.Split(text, "\n") {
		w := displayWidth(line)
		if w > longest {
			longest = w
		}
	}
	return longest
}

// wrapToLines wraps a string at the given width, returning one line per wrap.
func wrapToLines(text string, width int) []string {
	if text == "" {
		return []string{""}
	}
	wrapped := wrapLine(text, width)
	return strings.Split(wrapped, "\n")
}

func shrinkColumns(widths []int, available int) []int {
	total := 0
	for _, w := range widths {
		total += w
	}
	if total <= available {
		return widths
	}
	result := make([]int, len(widths))
	remaining := available
	for i, w := range widths {
		proportional := w * available / total
		if proportional < 3 {
			proportional = 3
		}
		result[i] = proportional
		remaining -= proportional
	}
	for remaining != 0 {
		for i := range result {
			if remaining == 0 {
				break
			}
			if remaining > 0 {
				result[i]++
				remaining--
			} else if result[i] > 3 {
				result[i]--
				remaining++
			}
		}
		allMin := true
		for _, w := range result {
			if w > 3 {
				allMin = false
				break
			}
		}
		if allMin && remaining < 0 {
			break
		}
	}
	return result
}

func displayWidth(s string) int {
	return runewidth.StringWidth(stripStyles(s))
}

func stripStyles(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])
			continue
		}
		// ESC found — parse the escape sequence.
		i++
		if i >= len(s) {
			break
		}
		switch s[i] {
		case '[':
			// CSI: skip parameter/intermediate bytes (0x30-0x3f, 0x20-0x2f)
			// until final byte (0x40-0x7e).
			for i++; i < len(s); i++ {
				b := s[i]
				if b >= 0x40 && b <= 0x7e {
					break
				}
			}
		case ']':
			// OSC: skip until ST (ESC \) or BEL (0x07).
			for i++; i < len(s); i++ {
				if s[i] == 0x07 {
					break
				}
				if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
					i++
					break
				}
			}
		default:
			// Other escape: skip one character (simple ESC+X sequences).
		}
	}
	return b.String()
}

func stripHTMLTags(text string) string {
	var b strings.Builder
	inTag := false
	for i := 0; i < len(text); i++ {
		if text[i] == '<' {
			inTag = true
			continue
		}
		if inTag {
			if text[i] == '>' {
				inTag = false
			}
			continue
		}
		b.WriteByte(text[i])
	}
	return b.String()
}
