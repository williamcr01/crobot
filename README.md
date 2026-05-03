# Crobot

![Crobot app screenshot](assets/crobot.png)

Crobot is a minimal agentic assistant built in Go with Bubble Tea.

## Features

- Terminal UI with streaming assistant responses
- Provider support for OpenRouter, OpenAI, OpenAI Codex OAuth, DeepSeek, Gemini, Kimi, and Anthropic
- Local tools for file read, file write, file edit, and bash commands
- Slash commands for model selection, login/logout, context management, sessions, and display settings
- Configurable system prompt, reasoning level, compaction, and output alignment

## Build

Requirements:

- Go 1.24+

Build the binary:

```sh
./build.sh
```

The binary is written to:

```text
./build/crobot
```

Run it:

```sh
./build/crobot
```

## Authentication

Crobot stores credentials in:

```text
~/.crobot/auth.json
```

The file is created automatically if it does not exist. Add a provider credential manually or use `/login` in the TUI for OpenAI Codex OAuth.

Example OpenRouter auth:

```json
{
  "openrouter": {
    "type": "apiKey",
    "apiKey": "sk-or-v1-your-key-here"
  }
}
```

Example OpenAI auth:

```json
{
  "openai": {
    "type": "apiKey",
    "apiKey": "sk-your-key-here"
  }
}
```

Example Kimi auth (pay-per-token Moonshot Developer API):

```json
{
  "kimi": {
    "type": "apiKey",
    "apiKey": "sk-your-moonshot-key-here"
  }
}
```

Example Kimi Code auth (subscription coding plan):

```json
{
  "kimi-code": {
    "type": "apiKey",
    "apiKey": "sk-your-kimi-code-key-here"
  }
}
```

Kimi's public Open Platform uses prepaid balance/recharge. Kimi Code is a separate subscription plan with its own API key and endpoint (`https://api.kimi.com/coding/v1`). Use `provider: "kimi"` with the Moonshot Developer API or `provider: "kimi-code"` for the Kimi Code plan. Model IDs include `kimi-k2.6`, `kimi-k2.5`, `kimi-k2`, etc.

## Configuration

Crobot reads user configuration from:

```text
~/.crobot/agent.config.json
```

An empty config uses defaults. The full default config is:

```json
{
  "provider": "",
  "model": "",
  "thinking": "none",
  "maxTurns": 50,
  "systemPrompt": "You are Crobot, a coding assistant. You have access to the following tools:\nfile read,\nfile write,\nfile edit,\nbash,\n\nCurrent working directory: {cwd}",
  "appendPrompt": "",
  "sessionDir": "~/.crobot/sessions",
  "sessions": {
    "retentionDays": 30,
    "maxSessions": 50,
    "keepNamed": true,
    "pruneOnStartup": true,
    "pruneEmptyAfterHours": 24
  },
  "showBanner": true,
  "slashCommands": true,
  "reasoning": true,
  "alignment": "left",
  "theme": "",
  "compaction": {
    "enabled": true,
    "reserveTokens": 16384,
    "keepRecentTokens": 20000,
    "model": ""
  },
  "plugins": {
    "enabled": true,
    "directories": ["~/.crobot/plugins"],
    "permissions": ["file_read", "file_write", "bash", "tool_call", "send_message"]
  }
}
```

## Themes

Crobot supports JSON themes installed in:

```text
~/.crobot/themes/<theme-name>.json
```

Open the interactive theme picker from inside Crobot:

```text
/theme
```

The selected theme is applied immediately and saved to `~/.crobot/agent.config.json`.

You can also set the active theme manually:

```json
{
  "theme": "crobot-light"
}
```

Built-in themes are `crobot-dark`, `crobot-light`, and `crobot-monochrome`. Custom themes use the filename without `.json`.

See [docs/themes.md](docs/themes.md) for the full theme format, install instructions, and color key reference.

## Plugins

Crobot supports WASM plugins for adding tools, middleware hooks, and custom slash commands. Plugins load from configured directories under `plugins` in `~/.crobot/agent.config.json`.

Useful commands:

```text
/plugins  List loaded plugins and load errors
/reload   Unload and reload all plugins
```

See [docs/plugins.md](docs/plugins.md) for the ABI, manifest format, permissions, and authoring details.

## Sessions

Crobot stores sessions in `sessionDir` as JSONL files. By default it prunes sessions on startup, keeping sessions modified within 30 days and at most 50 sessions, while keeping the current session.

Startup flags:

```text
-h, --help            Show help and exit
-c, --continue        Continue the most recent session
    --session <path>  Open a specific session file
    --no-session      Run without saving a session
    --skill <path>    Load a skill from directory or .md file (repeatable)
```

You can also run `crobot help` as a subcommand.

## Slash commands

Inside the TUI:

```text
/help                  Show available commands
/model                 Open the model picker
/theme                 Open the theme picker
/login                 Add OAuth credentials
/logout                Remove OAuth credentials
/thinking <level>      Set reasoning effort
/new                   Start a fresh conversation
/resume                Resume a previous session
/session               Show session info
/compact [instruction] Compact conversation context
/export [path]         Export conversation as Markdown
/alignment <value>     Set output alignment
/quit                  Quit Crobot
```

You can also use `!` for shell shortcuts where supported by the input parser.

## Development

Run tests, race detection, coverage, and a build:

```sh
./test.sh
```

Build only:

```sh
./build.sh
```
