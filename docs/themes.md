# Themes

Crobot themes are JSON files that control terminal colors for the TUI.

## Built-in themes

Crobot ships with these themes:

- `crobot-dark` — default dark theme
- `crobot-light` — light terminal theme
- `crobot-monochrome` — grayscale theme

Set the active theme from inside Crobot with:

```text
/theme
```

This opens an interactive picker, like `/model`. Type to filter, use arrow keys to move, and press Enter to select. The selected theme is applied immediately and saved to `~/.crobot/agent.config.json`.

You can also edit `~/.crobot/agent.config.json` manually:

```json
{
  "theme": "crobot-light"
}
```

If `theme` is missing or empty, Crobot uses `crobot-dark`.

## User theme location

Install custom themes here:

```text
~/.crobot/themes/<theme-name>.json
```

For example:

```text
~/.crobot/themes/amber.json
```

Activate it from Crobot with `/theme`, or manually with:

```json
{
  "theme": "amber"
}
```

Use the filename without `.json`.

## Theme file format

A theme file has optional metadata plus `colors` and `bold` maps:

```json
{
  "name": "amber",
  "description": "Warm amber theme",
  "colors": {
    "bodyText": "#e2e8f0",
    "toolBg": "#1a1a2e",
    "h1": "#fbbf24",
    "h2": "#fde68a",
    "link": "#93c5fd",
    "errorMessage": "#f87171"
  },
  "bold": {
    "h1": true,
    "h2": true,
    "toolTitle": true
  }
}
```

Rules:

- Colors must be hex strings: `#RGB`, `#RGBA`, `#RRGGBB`, or `#RRGGBBAA`.
- `name` and `description` are optional.
- Every color key is optional.
- Missing keys fall back to the default value, which is transparent for `toolBg` or the `crobot-dark` value for all other colors.
- Selecting a theme with `/theme` applies it immediately.
- If you edit the config or theme file manually, restart Crobot to reload it.

## Color keys

| Key | What it controls |
|-----|------------------|
| `dim` | Dim secondary text |
| `cyan` | General cyan/accent color |
| `green` | Success and green accent text |
| `yellow` | Warning/accent text |
| `red` | Error/failure text |
| `gray` | Gray metadata text |
| `toolBg` | Tool and code block background (transparent by default) |
| `toolTitle` | Tool name text |
| `toolOutput` | Tool output text |
| `toolMeta` | Tool status/duration text |
| `bashHeader` | Bash command header text |
| `userPrompt` | User message text |
| `userCaret` | User prompt caret |
| `inputCursor` | Input cursor glyph |
| `errorMessage` | Error message text |
| `h1` | Markdown heading level 1 |
| `h2` | Markdown heading level 2 |
| `h3` | Markdown heading level 3 |
| `h4` | Markdown heading level 4 |
| `bodyText` | Main assistant/body text |
| `thinking` | Reasoning/thinking text |
| `code` | Inline code text |
| `codeBlock` | Code block text |
| `strike` | Strikethrough text |
| `link` | Link text |
| `image` | Image alt text marker |
| `quote` | Blockquote text |
| `quoteBar` | Blockquote vertical bar |
| `hr` | Markdown horizontal rule |
| `taskDone` | Completed task checkbox |
| `taskOpen` | Open task checkbox |
| `tableBorder` | Markdown table borders |
| `tableHeader` | Markdown table header text |
| `tableCell` | Markdown table body text |

## Bold keys

The `bold` object accepts the same style keys plus `bold` itself. Values are booleans.

Common bold keys:

```json
{
  "bold": {
    "bold": true,
    "toolTitle": true,
    "bashHeader": true,
    "h1": true,
    "h2": true,
    "h3": true,
    "h4": true,
    "tableHeader": true
  }
}
```
