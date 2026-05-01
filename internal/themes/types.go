package themes

// StyleName identifies each colorable UI element. The string is the JSON key
// used in theme files.
type StyleName string

const (
	StyleDim          StyleName = "dim"
	StyleBold         StyleName = "bold"
	StyleCyan         StyleName = "cyan"
	StyleGreen        StyleName = "green"
	StyleYellow       StyleName = "yellow"
	StyleRed          StyleName = "red"
	StyleGray         StyleName = "gray"
	StyleToolBg       StyleName = "toolBg"
	StyleToolTitle    StyleName = "toolTitle"
	StyleToolOutput   StyleName = "toolOutput"
	StyleToolMeta     StyleName = "toolMeta"
	StyleBashHeader   StyleName = "bashHeader"
	StyleUserPrompt   StyleName = "userPrompt"
	StyleUserCaret    StyleName = "userCaret"
	StyleInputCursor  StyleName = "inputCursor"
	StyleErrorMessage StyleName = "errorMessage"
	StyleH1           StyleName = "h1"
	StyleH2           StyleName = "h2"
	StyleH3           StyleName = "h3"
	StyleH4           StyleName = "h4"
	StyleBodyText     StyleName = "bodyText"
	StyleThinking     StyleName = "thinking"
	StyleCode         StyleName = "code"
	StyleCodeBlock    StyleName = "codeBlock"
	StyleStrike       StyleName = "strike"
	StyleLink         StyleName = "link"
	StyleImage        StyleName = "image"
	StyleQuote        StyleName = "quote"
	StyleQuoteBar     StyleName = "quoteBar"
	StyleHR           StyleName = "hr"
	StyleTaskDone     StyleName = "taskDone"
	StyleTaskOpen     StyleName = "taskOpen"
	StyleTableBorder  StyleName = "tableBorder"
	StyleTableHeader  StyleName = "tableHeader"
	StyleTableCell    StyleName = "tableCell"
)

// allStyleNames returns every defined style name. Used for validation and
// iteration when building Styles from a Theme.
func allStyleNames() []StyleName {
	return []StyleName{
		StyleDim, StyleCyan, StyleGreen, StyleYellow, StyleRed, StyleGray,
		StyleToolBg, StyleToolTitle, StyleToolOutput, StyleToolMeta, StyleBashHeader,
		StyleUserPrompt, StyleUserCaret, StyleInputCursor, StyleErrorMessage,
		StyleH1, StyleH2, StyleH3, StyleH4,
		StyleBodyText, StyleThinking, StyleCode, StyleCodeBlock,
		StyleStrike, StyleLink, StyleImage,
		StyleQuote, StyleQuoteBar, StyleHR,
		StyleTaskDone, StyleTaskOpen,
		StyleTableBorder, StyleTableHeader, StyleTableCell,
	}
}

func allBoldNames() []StyleName {
	names := []StyleName{StyleBold}
	names = append(names, allStyleNames()...)
	return names
}

// Theme represents a parsed theme. Colors are hex strings like "#22d3ee".
// Bold-related keys are booleans.
type Theme struct {
	Name        string               `json:"name,omitempty"`
	Description string               `json:"description,omitempty"`
	Colors      map[StyleName]string `json:"colors"`
	Bold        map[StyleName]bool   `json:"bold,omitempty"`
}
