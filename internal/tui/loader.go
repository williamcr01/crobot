package tui

import (
	"github.com/charmbracelet/bubbles/spinner"
)

// NewLoaderSpinner creates a spinner for the "Working" indicator.
func NewLoaderSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	// Style is set by the caller via SetLoaderSpinnerStyle.
	return s
}

// SetLoaderSpinnerStyle sets the style on a loader spinner.
func SetLoaderSpinnerStyle(s *spinner.Model, styles Styles) {
	s.Style = styles.Yellow
}
