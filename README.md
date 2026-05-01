# Crobot

![Crobot app screenshot](assets/crobot.png)

Crobot is a minimal agentic assistant built in Go with Bubble Tea.

## Features

- Terminal UI with streaming assistant responses
- Provider support for OpenRouter, OpenAI, OpenAI Codex OAuth, DeepSeek, and Anthropic
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
./build/agent
```

Run it:

```sh
./build/agent
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

Plugin support is planned. Crobot already reserves plugin configuration under `plugins` in `~/.crobot/agent.config.json`, including plugin directories and permissions. The intended model is a WASM middleware/tool system for extending prompts, responses, tool calls, and custom commands.

Plugin loading is not currently wired into the app.

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
