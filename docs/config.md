# Crobot user configuration

Crobot reads user configuration from:

```text
~/.crobot/agent.config.json
```

If the file does not exist, Crobot creates it as an empty JSON object:

```json
{}
```

An empty config uses all defaults. Add only the settings you want to override.

## Minimal examples

Append extra instructions to the default system prompt:

```json
{
  "appendPrompt": "Always explain risky file edits before making them."
}
```

Select a provider and model:

```json
{
  "provider": "openrouter",
  "model": "anthropic/claude-sonnet-4.5",
  "thinking": "medium"
}
```

Override the default system prompt entirely:

```json
{
  "systemPrompt": "You are a concise coding assistant. Current working directory: {cwd}"
}
```

## Defaults

```json
{
  "provider": "",
  "model": "",
  "thinking": "none",
  "maxTurns": -1,
  "systemPrompt": "",
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
  },
  "openrouter": {
    "cache": false,
    "cacheTTL": 0
  }
}
```

## Fields

### `provider`

Provider to use for model requests.

Default: `""`.

Supported values:

- `""` — no provider selected
- `"openrouter"`
- `"openai"`
- `"openai-responses-ws"` — OpenAI Responses API over WebSocket, using the `openai` API key
- `"openai-codex"`
- `"deepseek"`
- `"gemini"`
- `"kimi"`
- `"kimi-code"`
- `"anthropic"`
- `"opencode-zen"` — OpenCode Zen
- `"opencode-go"` — OpenCode Go

Credentials are not stored here. Put credentials in `~/.crobot/auth.json`.

Gemini lists models dynamically from the Gemini API.

Kimi uses Moonshot/Kimi Open Platform API keys under the `kimi` auth entry (pay-per-token). Kimi Code is a separate subscription plan with its own API key under the `kimi-code` auth entry, using the endpoint `https://api.kimi.com/coding/v1`.

### `model`

Model ID to use with the selected provider.

Default: `""`.

When empty, the TUI shows `(no model)`.

### `thinking`

Reasoning effort sent to the provider.

Default: `"none"`.

Valid values:

- `"none"`
- `"minimal"`
- `"low"`
- `"medium"`
- `"high"`
- `"xhigh"`

### `maxTurns`

Maximum number of model turns in a single user request. A turn is one model response, including responses that request tool calls.

Default: `-1` (unlimited).

Set to `-1` to disable the limit and allow unlimited turns.

### `systemPrompt`

Replaces the built-in system prompt when non-empty.

Default: `""` (use built-in prompt).

If this field is missing or set to `""`, Crobot uses the built-in prompt that lists available tools and the current working directory.

The placeholder `{cwd}` is replaced with the current working directory.

### `appendPrompt`

Adds text after the active system prompt.

Default: `""`.

Use this when you want to keep Crobot's default prompt and add your own instructions.

The placeholder `{cwd}` is replaced with the current working directory.

### `sessionDir`

Directory where sessions are stored.

Default: `"~/.crobot/sessions"`.

### `sessions`

Controls session persistence and retention.

#### `sessions.retentionDays`

Number of days to retain sessions before pruning.

Default: `30`.

#### `sessions.maxSessions`

Maximum number of session files to keep.

Default: `50`.

#### `sessions.keepNamed`

Keep sessions that have a custom title (not the auto-generated prompt-based title).

Default: `true`.

#### `sessions.pruneOnStartup`

Prune old sessions at startup.

Default: `true`.

#### `sessions.pruneEmptyAfterHours`

Prune sessions that have no sent messages after this many hours.

Default: `24`.

### `showBanner`

Shows or hides the startup banner.

Default: `true`.

### `slashCommands`

Enables slash commands.

Default: `true`.

### `reasoning`

Shows or hides streamed reasoning output.

Default: `true`.

### `alignment`

Controls output text alignment.

Default: `"left"`.

Valid values:

- `"left"`
- `"centered"`

Auto-saved when changed via `/alignment`.

### `theme`

Active theme name (without `.json`).

Default: `""` (uses `crobot-dark`).

Built-in themes are `crobot-dark`, `crobot-light`, and `crobot-monochrome`. Custom themes are loaded from `~/.crobot/themes/<name>.json`.

Auto-saved when changed via `/theme`.

### `compaction.enabled`

Enables automatic context compaction when the conversation exceeds the token threshold.

Default: `true`.

Manual compaction with `/compact` always works regardless of this setting.

### `compaction.reserveTokens`

Tokens reserved for the LLM's response. Compaction triggers when the estimated context exceeds `contextWindow - reserveTokens`.

Default: `16384`.

### `compaction.keepRecentTokens`

Approximate tokens of recent conversation to preserve (not summarize) when compacting.

Default: `20000`.

### `compaction.model`

Optional model override for summarization. When empty or unset, the current conversation model is used.

Default: `""`.

Auto-saved when changed via `/model` while in the compaction context.

Example:

```json
{
  "compaction": {
    "model": "openai/gpt-4o-mini"
  }
}
```

### `plugins.enabled`

Enables plugin loading.

Default: `true`.

### `plugins.directories`

Plugin directories to scan.

Default:

```json
["~/.crobot/plugins"]
```

### `plugins.permissions`

Permissions available to plugins.

Default:

```json
["file_read", "file_write", "bash", "tool_call", "send_message"]
```

### `openrouter.cache`

Enables OpenRouter response caching by sending `X-OpenRouter-Cache: true` on model requests.

Default: `false`.

On cache hits, OpenRouter returns zero billed usage, so Crobot's cost total remains unchanged for that request.

Example:

```json
{
  "provider": "openrouter",
  "openrouter": {
    "cache": true,
    "cacheTTL": 300
  }
}
```

### `openrouter.cacheTTL`

Optional response cache TTL in seconds.

Default: `0`, which lets OpenRouter use its default TTL.

Valid values:

- `0`
- `1` through `86400`

## Auto-saved settings

When changed from inside the app, Crobot auto-saves only these fields to `~/.crobot/agent.config.json`:

- `provider`
- `model`
- `thinking`
- `alignment`
- `theme`

Other settings are read-only at runtime; edit the config file manually to change them.
