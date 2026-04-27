package tui

import "github.com/charmbracelet/bubbles/spinner"

// NewLoaderSpinner creates a spinner for the "Working" indicator.
func NewLoaderSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = Yellow
	return s
}
