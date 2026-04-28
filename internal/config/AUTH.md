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

## File format

`auth.json` is a JSON object keyed by provider ID.

Each provider entry currently supports:

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

## Notes

- Do not commit `auth.json`.
- Keep `auth.json` in `~/.crobot/auth.json`.
- `agent.config.json` selects which provider/model to use.
- `auth.json` stores credentials for providers.
