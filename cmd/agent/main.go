package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"crobot/internal/commands"
	"crobot/internal/config"
	"crobot/internal/events"
	pluginpkg "crobot/internal/plugins"
	"crobot/internal/provider"
	"crobot/internal/session"
	"crobot/internal/skills"
	"crobot/internal/themes"
	"crobot/internal/tools"
	"crobot/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func main() {
	parsed, remainingArgs, err := parseStartupArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if parsed.help {
		fmt.Print(cliHelpText())
		return
	}
	if parsed.showVersion {
		fmt.Print(cliVersionText())
		return
	}
	os.Args = append([]string{os.Args[0]}, remainingArgs...)

	// Load config.
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Load auth and create provider if configured.
	if err := config.RefreshOpenAIOAuth(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	auth, err := config.LoadAuth()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	prov, startupWarning, err := createStartupProvider(cfg, auth)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if startupWarning != "" {
		fmt.Fprintln(os.Stderr, startupWarning)
	}

	// Initialize registries.
	toolReg := tools.NewRegistry()
	cmdReg := commands.NewRegistry()

	// Create model registry and load models for every authorized provider.
	modelReg := provider.NewModelRegistry()
	for _, providerName := range []string{"openrouter", "openai", "openai-responses-ws", "openai-codex", "deepseek", "anthropic"} {
		apiKey := auth.APIKey(providerName)
		if apiKey == "" {
			continue
		}
		if prov != nil && cfg.Provider == providerName {
			modelReg.AddProvider(prov)
			continue
		}
		tmpProv, err := provider.Create(providerName, apiKey)
		if err == nil {
			modelReg.AddProvider(tmpProv)
		} else {
			fmt.Fprintf(os.Stderr, "warning: creating %s model reader: %v\n", providerName, err)
		}
	}
	ctx := context.Background()
	if err := modelReg.LoadModels(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load models: %v\n", err)
	}
	// Create model history for recently used models.
	configDir, err := config.ConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: config dir: %v\n", err)
	}
	modelHistory := commands.NewModelHistory(configDir)
	if cfg.Provider != "" && cfg.Model != "" {
		modelHistory.Record(cfg.Provider, cfg.Model)
	}
	cmdReg.SetModelHistory(modelHistory)
	cmdReg.SetModelRegistry(modelReg)

	// Register native tools.
	toolReg.Register(tools.FileReadTool)
	toolReg.Register(tools.FileWriteTool)
	toolReg.Register(tools.FileEditTool)
	toolReg.Register(tools.BashTool)
	toolReg.Register(tools.GrepTool)
	toolReg.Register(tools.FindTool)
	toolReg.Register(tools.LsTool)

	// Register native commands.
	registerCommands(cmdReg, cfg)

	// Initialize events logger.
	ev := events.NewLogger(cfg.SessionDir)

	// Initialize plugin manager and register plugin management commands.
	pluginMgr := pluginpkg.NewManager(cfg.Plugins, toolReg, cmdReg, ev)
	registerPluginManagementCommands(cmdReg, pluginMgr)
	if err := pluginMgr.LoadAll(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "warning: loading plugins: %v\n", err)
	}

	// Initialize session.
	cwd, _ := os.Getwd()
	var sess *session.Manager
	if !parsed.noSession {
		if parsed.sessionPath != "" {
			sess, err = session.Open(parsed.sessionPath)
		} else if parsed.continueRecent {
			sess, err = session.ContinueRecent(cfg.SessionDir, cwd)
		} else {
			sess, err = session.Create(cfg.SessionDir, cwd)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: session init: %v\n", err)
		sess = nil
	} else if sess != nil && cfg.Sessions.PruneOnStartup {
		_, _ = session.Prune(cfg.SessionDir, session.RetentionPolicy{
			MaxAge:              time.Duration(cfg.Sessions.RetentionDays) * 24 * time.Hour,
			MaxSessions:         cfg.Sessions.MaxSessions,
			KeepNamed:           cfg.Sessions.KeepNamed,
			KeepCurrentPath:     sess.Path(),
			PruneEmptyOlderThan: time.Duration(cfg.Sessions.PruneEmptyAfterHours) * time.Hour,
		})
	}

	// Load skills from ~/.agents/skills/, ~/.crobot/skills/, ./.crobot/skills/, and --skill paths.
	// Only metadata (name, description, path) is loaded into the system prompt.
	// Full content is loaded lazily when the model calls read() or the user uses /skill:name.
	skillResult := skills.LoadSkills(cwd, parsed.skillPaths, true)
	for _, d := range skillResult.Diagnostics {
		if d.Level == "warning" {
			fmt.Fprintf(os.Stderr, "skills warning: %s (%s)\n", d.Message, d.Path)
		}
	}

	// Register the /skills command (after loading, so we can capture the loaded skills).
	registerSkillsCommand(cmdReg, skillResult.Skills)

	// Headless mode: run a single prompt and print the response to stdout.
	if parsed.promptText != "" {
		if prov == nil {
			fmt.Fprintf(os.Stderr, "error: no provider configured\n")
			os.Exit(1)
		}
		runHeadless(cfg, prov, toolReg, pluginMgr, skillResult.Skills, parsed.promptText)
		return
	}

	// Load theme and create styles.
	if err := themes.EnsureThemeDir(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: creating theme directory: %v\n", err)
	}
	theme, err := themes.LoadTheme(cfg.Theme)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: loading theme %q: %v, using default\n", cfg.Theme, err)
		theme = themes.DefaultTheme()
	}
	styles := tui.NewStyles(theme)
	lipgloss.SetColorProfile(termenv.TrueColor)

	// Create and run the TUI.
	m := tui.NewModel(cfg, prov, toolReg, ev, cmdReg, modelReg, modelHistory, sess, auth.APIKey, skillResult.Skills, styles, pluginMgr)

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

func registerPluginManagementCommands(cmdReg *commands.Registry, pluginMgr *pluginpkg.Manager) {
	cmdReg.Register(commands.Command{
		Name:        "plugins",
		Description: "List loaded WASM plugins",
		Source:      "native",
		Handler: func(args []string) (string, error) {
			return pluginMgr.Summary(), nil
		},
	})
	cmdReg.Register(commands.Command{
		Name:        "reload",
		Description: "Reload WASM plugins",
		Source:      "native",
		Handler: func(args []string) (string, error) {
			if err := pluginMgr.Reload(context.Background()); err != nil {
				return "", err
			}
			return pluginMgr.Summary(), nil
		},
	})
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
		Description: "Switch model (interactive picker)",
		Handler: func(args []string) (string, error) {
			return "No models available. Try /model <search> or check your provider connection.", nil
		},
	})

	cmdReg.Register(commands.Command{
		Name:        "login",
		Description: "Login to an OAuth provider",
		Handler: func(args []string) (string, error) {
			return "Use /login to open the OAuth provider picker.", nil
		},
	})

	cmdReg.Register(commands.Command{
		Name:        "logout",
		Description: "Logout from an OAuth provider",
		Handler: func(args []string) (string, error) {
			return "Use /logout to open the OAuth provider picker.", nil
		},
	})

	cmdReg.Register(commands.Command{
		Name:        "theme",
		Description: "Switch theme (interactive picker)",
		Handler: func(args []string) (string, error) {
			return "Use /theme to open the theme picker.", nil
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
			return "", nil
		},
	})

	cmdReg.Register(commands.Command{
		Name:        "resume",
		Description: "Resume a previous session",
		Handler: func(args []string) (string, error) {
			return "", nil
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
		Args:        "<instruction-optional>",
		Handler: func(args []string) (string, error) {
			return "Compacting context...", nil
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

	cmdReg.Register(commands.Command{
		Name:        "alignment",
		Description: "Set output alignment",
		Args:        "<left|centered>",
		Handler: func(args []string) (string, error) {
			if len(args) == 0 {
				return "", fmt.Errorf("usage: /alignment <left|centered>")
			}
			val := args[0]
			valid := map[string]bool{"left": true, "centered": true}
			if !valid[val] {
				return "", fmt.Errorf("invalid alignment: %s (valid: left, centered)", val)
			}
			cfg.Alignment = val
			if err := config.SaveConfig(cfg); err != nil {
				return "", err
			}
			return fmt.Sprintf("Alignment set to: %s", cfg.Alignment), nil
		},
	})
}

// registerSkillsCommand registers the /skills slash command.
func registerSkillsCommand(cmdReg *commands.Registry, skls []skills.Skill) {
	cmdReg.Register(commands.Command{
		Name:        "skills",
		Description: "List loaded skills",
		Handler: func(args []string) (string, error) {
			if len(skls) == 0 {
				return "No skills loaded.", nil
			}
			var b strings.Builder
			b.WriteString(fmt.Sprintf("Loaded skills (%d):\n", len(skls)))
			for _, s := range skls {
				disable := ""
				if s.DisableModelInvocation {
					disable = " (manual only)"
				}
				b.WriteString(fmt.Sprintf("  %s  %s%s\n", s.Name, s.Description, disable))
				b.WriteString(fmt.Sprintf("       %s\n", s.FilePath))
			}
			return b.String(), nil
		},
	})
}

func getArg(args []string, idx int, fallback string) string {
	if idx < len(args) {
		return args[idx]
	}
	return fallback
}
