# Crobot provider authentication

Crobot reads provider credentials from:

```text
~/.crobot/auth.json
```

If the file does not exist, Crobot creates it as an empty JSON object:

```json
{}
```

An empty auth file means no provider has been added yet. Crobot can still start, but it cannot send model requests until a provider credential is configured.

## OpenRouter

Add an OpenRouter API key like this:

```json
{
  "openrouter": {
    "type": "apiKey",
    "apiKey": "sk-or-v1-your-key-here"
  }
}
```

Then select OpenRouter in `~/.crobot/agent.config.json`:

```json
{
  "provider": "openrouter",
  "model": "anthropic/claude-sonnet-4.5"
}
```

## OpenAI

API key:

```json
{
  "openai": {
    "type": "apiKey",
    "apiKey": "sk-your-key-here"
  }
}
```

OAuth tokens, compatible with pi-ai/OpenAI Codex login output, use a separate provider ID so API key and OAuth auth can coexist:

```json
{
  "openai-codex": {
    "type": "oauth",
    "access": "eyJ...",
    "refresh": "...",
    "expires": 1770000000000,
    "accountId": "acct_..."
  }
}
```

OAuth access tokens are sent as bearer tokens. Crobot refreshes them using the stored refresh token when they are close to expiry. `/login` writes OpenAI OAuth credentials to `openai-codex`.

### OpenAI Responses WebSocket

The `openai-responses-ws` provider reuses the `openai` API key. No separate auth entry is needed. Select it in config:

```json
{
  "provider": "openai-responses-ws",
  "model": "gpt-4.1"
}
```

Then select OpenAI in `~/.crobot/agent.config.json`:

```json
{
  "provider": "openai",
  "model": "gpt-4.1"
}
```

## Anthropic

Add an Anthropic API key like this:

```json
{
  "anthropic": {
    "type": "apiKey",
    "apiKey": "sk-ant-your-key-here"
  }
}
```

Then select Anthropic in `~/.crobot/agent.config.json` or via `/model`:

```json
{
  "provider": "anthropic",
  "model": "claude-sonnet-4-5-20250929"
}
```

## DeepSeek

Add a DeepSeek API key like this:

```json
{
  "deepseek": {
    "type": "apiKey",
    "apiKey": "sk-your-key-here"
  }
}
```

Then select DeepSeek in `~/.crobot/agent.config.json` or via `/model`:

```json
{
  "provider": "deepseek",
  "model": "deepseek-v4-pro"
}
```

## Gemini

Add a Gemini API key like this:

```json
{
  "gemini": {
    "type": "apiKey",
    "apiKey": "your-gemini-api-key"
  }
}
```

Then select Gemini in `~/.crobot/agent.config.json` or via `/model`:

```json
{
  "provider": "gemini",
  "model": "gemini-2.5-pro"
}
```

Crobot lists models dynamically from the Gemini API.

## Kimi

Add a Kimi API key (Moonshot Developer Platform):

```json
{
  "kimi": {
    "type": "apiKey",
    "apiKey": "sk-your-moonshot-key-here"
  }
}
```

Kimi's public Open Platform uses prepaid balance/recharge. Use `provider: "kimi"` in your config. Model IDs include `kimi-k2.6`, `kimi-k2.5`, `kimi-k2`, etc.

## Kimi Code

Kimi Code is a separate subscription coding plan with its own API key:

```json
{
  "kimi-code": {
    "type": "apiKey",
    "apiKey": "sk-your-kimi-code-key-here"
  }
}
```

Select it with `provider: "kimi-code"`. Uses the endpoint `https://api.kimi.com/coding/v1`.

## OpenCode

OpenCode provides two services — Zen (free/rate-limited) and Go (paid):

```json
{
  "opencode-zen": {
    "type": "apiKey",
    "apiKey": "sk-zen-your-key-here"
  },
  "opencode-go": {
    "type": "apiKey",
    "apiKey": "sk-go-your-key-here"
  }
}
```

Select with `provider: "opencode-zen"` or `provider: "opencode-go"`.

## File format

`auth.json` is a JSON object keyed by provider ID.

Each API-key provider entry supports:

```json
{
  "type": "apiKey",
  "apiKey": "..."
}
```

`type` may be omitted for API-key providers:

```json
{
  "openrouter": {
    "apiKey": "sk-or-v1-your-key-here"
  }
}
```

## Supported providers

Currently supported:

- `openrouter`
- `openai` (API key, Chat Completions)
- `openai-responses-ws` (API key, Responses API WebSocket; reuses the `openai` auth entry)
- `openai-codex` (ChatGPT/Codex OAuth)
- `anthropic`
- `gemini`
- `deepseek`
- `kimi` (Moonshot Developer Platform)
- `kimi-code` (subscription coding plan)
- `opencode-zen` (free/rate-limited)
- `opencode-go` (paid)

## Notes

- Do not commit `auth.json`.
- Keep `auth.json` in `~/.crobot/auth.json`.
- `agent.config.json` selects which provider/model to use.
- `auth.json` stores credentials for providers.
