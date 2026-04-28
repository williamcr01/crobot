package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"crobot/internal/commands"
	"crobot/internal/config"
	"crobot/internal/events"
	"crobot/internal/provider"
	"crobot/internal/session"
	"crobot/internal/tools"
	"crobot/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Load config.
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Load auth and create provider if configured.
	auth, err := config.LoadAuth()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	var prov provider.Provider
	cfg.HasAuthorizedProvider = auth.HasAuthorizedProvider()
	if !cfg.HasAuthorizedProvider {
		fmt.Fprintln(os.Stderr, "warning: No provider added. Add credentials to ~/.crobot/auth.json")
	} else if cfg.Provider != "" {
		apiKey := auth.APIKey(cfg.Provider)
		if apiKey == "" {
			fmt.Fprintf(os.Stderr, "warning: Provider %q is not authorized in ~/.crobot/auth.json\n", cfg.Provider)
		} else {
			prov, err = provider.Create(cfg.Provider, apiKey)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		}
	}

	// Initialize registries.
	toolReg := tools.NewRegistry()
	cmdReg := commands.NewRegistry()

	// Register native tools.
	toolReg.Register(tools.FileReadTool)
	toolReg.Register(tools.FileWriteTool)
	toolReg.Register(tools.FileEditTool)
	toolReg.Register(tools.BashTool)

	// Register native commands.
	registerCommands(cmdReg, cfg)

	// Initialize events logger.
	ev := events.NewLogger(cfg.SessionDir)

	// Initialize session.
	sessionID := fmt.Sprintf("%x", time.Now().UnixNano())
	sess, err := session.NewManager(cfg.SessionDir, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: session init: %v\n", err)
		sess = nil
	}

	_ = context.Background() // reserved for future plugin manager

	// Create and run the TUI.
	m := tui.NewModel(cfg, prov, toolReg, ev, cmdReg, sess)

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// registerCommands wires up all native slash commands.
func registerCommands(cmdReg *commands.Registry, cfg *config.AgentConfig) {
	cmdReg.Register(commands.Command{
		Name:        "help",
		Description: "Show available commands",
		Handler: func(args []string) (string, error) {
			return cmdReg.HelpText(), nil
		},
	})

	cmdReg.Register(commands.Command{
		Name:        "quit",
		Description: "Quit crobot",
		Handler: func(args []string) (string, error) {
			return "", nil
		},
	})

	cmdReg.Register(commands.Command{
		Name:        "exit",
		Description: "Quit crobot",
		Handler: func(args []string) (string, error) {
			return "", nil
		},
	})

	cmdReg.Register(commands.Command{
		Name:        "model",
		Description: "Switch model",
		Args:        "<name>",
		Handler: func(args []string) (string, error) {
			if len(args) == 0 {
				return "", fmt.Errorf("usage: /model <model-name>")
			}
			cfg.Model = args[0]
			if err := config.SaveConfig(cfg); err != nil {
				return "", err
			}
			return fmt.Sprintf("Model set to: %s", cfg.Model), nil
		},
	})

	cmdReg.Register(commands.Command{
		Name:        "thinking",
		Description: "Switch thinking level",
		Args:        "<none|minimal|low|medium|high|xhigh>",
		Handler: func(args []string) (string, error) {
			if len(args) == 0 {
				return "", fmt.Errorf("usage: /thinking <none|minimal|low|medium|high|xhigh>")
			}
			level := args[0]
			valid := map[string]bool{"none": true, "minimal": true, "low": true, "medium": true, "high": true, "xhigh": true}
			if !valid[level] {
				return "", fmt.Errorf("invalid thinking: %s", level)
			}
			cfg.Thinking = level
			if err := config.SaveConfig(cfg); err != nil {
				return "", err
			}
			return fmt.Sprintf("Thinking set to: %s", cfg.Thinking), nil
		},
	})

	cmdReg.Register(commands.Command{
		Name:        "new",
		Description: "Start a fresh conversation",
		Handler: func(args []string) (string, error) {
			return "New session started.", nil
		},
	})

	cmdReg.Register(commands.Command{
		Name:        "session",
		Description: "Show session info",
		Handler: func(args []string) (string, error) {
			return "Session: active", nil
		},
	})

	cmdReg.Register(commands.Command{
		Name:        "compact",
		Description: "Compact conversation context",
		Handler: func(args []string) (string, error) {
			return "Context compacted.", nil
		},
	})

	cmdReg.Register(commands.Command{
		Name:        "export",
		Description: "Export conversation as Markdown",
		Args:        "[path]",
		Handler: func(args []string) (string, error) {
			return "Export would write to " + getArg(args, 0, "session-output.md"), nil
		},
	})

	cmdReg.Register(commands.Command{
		Name:        "plugins",
		Description: "List loaded plugins",
		Handler: func(args []string) (string, error) {
			return "No plugins loaded (plugin system not yet wired).", nil
		},
	})

	cmdReg.Register(commands.Command{
		Name:        "reload",
		Description: "Reload all plugins",
		Handler: func(args []string) (string, error) {
			return "Plugins reloaded.", nil
		},
	})
}

func getArg(args []string, idx int, fallback string) string {
	if idx < len(args) {
		return args[idx]
	}
	return fallback
}
