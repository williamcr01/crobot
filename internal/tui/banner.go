package tui

import (
	"fmt"
	"strings"
)

const logo = `
   ██████╗██████╗  ██████╗ ██████╗  ██████╗ ████████╗
  ██╔════╝██╔══██╗██╔═══██╗██╔══██╗██╔═══██╗╚══██╔══╝
  ██║     ██████╔╝██║   ██║██████╔╝██║   ██║   ██║   
  ██║     ██╔══██╗██║   ██║██╔══██╗██║   ██║   ██║   
  ╚██████╗██║  ██║╚██████╔╝██████╔╝╚██████╔╝   ██║   
   ╚═════╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝  ╚═════╝    ╚═╝   
`

// Render returns the ASCII banner with model and version info.
func Render(model, version string) string {
	if model == "" {
		model = "(no model)"
	}
	var b strings.Builder
	for _, line := range strings.Split(strings.Trim(logo, "\n"), "\n") {
		b.WriteString("\x1b[36m\x1b[1m") // cyan + bold
		b.WriteString(line)
		b.WriteString("\x1b[0m") // reset
		b.WriteString("\n")
	}
	line := fmt.Sprintf("  \x1b[2mmodel\x1b[0m  \x1b[36m%s\x1b[0m", model)
	if version != "" && version != "dev" {
		line += fmt.Sprintf("  \x1b[2mversion\x1b[0m  \x1b[36m%s\x1b[0m", version)
	}
	b.WriteString(line + "\n")
	return b.String()
}
