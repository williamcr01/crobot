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
  "maxTurns": 50,
  "systemPrompt": "You are Crobot, a coding assistant. You have access to the following tools:\nfile read,\nfile write\nfile edit\nbash\n\nCurrent working directory: {cwd}",
  "appendPrompt": "",
  "sessionDir": "~/.crobot/sessions",
  "showBanner": true,
  "slashCommands": true,
  "display": {
    "toolDisplay": "grouped",
    "reasoning": true,
    "inputStyle": "block"
  },
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
- `"anthropic"`

Credentials are not stored here. Put credentials in `~/.crobot/auth.json`.

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

Default: `-1`.

Set to `-1` to disable the limit and allow unlimited turns.

### `systemPrompt`

Replaces the built-in system prompt when non-empty.

Default: built-in Crobot prompt.

If this field is missing or set to `""`, Crobot uses the built-in prompt.

The placeholder `{cwd}` is replaced with the current working directory.

### `appendPrompt`

Adds text after the active system prompt.

Default: `""`.

Use this when you want to keep Crobot's default prompt and add your own instructions.

The placeholder `{cwd}` is replaced with the current working directory.

### `sessionDir`

Directory where sessions are stored.

Default: `"~/.crobot/sessions"`.

### `showBanner`

Shows or hides the startup banner.

Default: `true`.

### `slashCommands`

Enables slash commands.

Default: `true`.

### `display.toolDisplay`

Controls tool rendering style.

Default: `"grouped"`.

Valid values:

- `"grouped"`
- `"emoji"`
- `"minimal"`
- `"hidden"`

### `display.reasoning`

Shows or hides streamed reasoning output.

Default: `true`.

### `display.inputStyle`

Controls the input box style.

Default: `"block"`.

Valid values:

- `"block"`
- `"bordered"`
- `"plain"`

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

## Auto-saved settings

When changed from inside the app, Crobot auto-saves only:

- `provider`
- `model`
- `thinking`
- `compaction.model`

Other settings are preserved but not automatically added or changed.
