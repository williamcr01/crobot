# Web Search

Crobot can search the web and fetch page content using configurable search providers. This is powered by three native tools: `web_search`, `fetch_content`, and `get_search_content`.

## Configuration

Search provider API keys are stored in:

```text
~/.crobot/web-search.json
```

If the file does not exist, Crobot creates an empty config. API keys can also be set via environment variables as fallbacks.

### Basic setup

Add at least one provider API key. All providers support environment variable fallback.

```json
{
  "exaApiKey": "exa-your-key-here",
  "perplexityApiKey": "pplx-your-key-here",
  "geminiApiKey": "AIza-your-key-here",
  "serperApiKey": "your-serper-key",
  "braveApiKey": "BSA-your-key-here",
  "tavilyApiKey": "tvly-your-key-here"
}
```

### Select a default provider

By default Crobot picks the first available provider in priority order. Override with the `provider` field:

```json
{
  "provider": "gemini",
  "geminiApiKey": "AIza-your-key-here"
}
```

Valid provider values: `auto` (default), `exa`, `perplexity`, `gemini`, `serper`, `brave`, `tavily`.

### Environment variables

Each key can be set as an environment variable instead of the config file. File values take precedence.

```bash
export EXA_API_KEY="exa-your-key-here"
export PERPLEXITY_API_KEY="pplx-your-key-here"
export GEMINI_API_KEY="AIza-your-key-here"
export SERPER_API_KEY="your-serper-key"
export BRAVE_API_KEY="BSA-your-key-here"
export TAVILY_API_KEY="tvly-your-key-here"
```

## Providers

### Exa

Semantic/neural search engine built for AI. Returns rich page content, highlights, and summaries.

```json
{ "exaApiKey": "exa-your-key" }
```

Get a key at [exa.ai](https://exa.ai) (paid).

### Perplexity

AI-synthesized answers with citations using the `sonar` model. OpenAI-compatible API.

```json
{ "perplexityApiKey": "pplx-your-key" }
```

Get a key at [perplexity.ai](https://perplexity.ai) (pay-as-you-go).

### Gemini

Uses Google's search grounding via the Gemini API. Returns synthesized answers with grounded source URLs.

```json
{ "geminiApiKey": "AIza-your-key" }
```

Get a free key at [aistudio.google.com](https://aistudio.google.com) (1,500 requests/day free tier).

### Brave

Independent search index via Brave Search API. Standard web search results with snippets.

```json
{ "braveApiKey": "BSA-your-key" }
```

Get a key at [brave.com/search/api](https://brave.com/search/api) (2,000 queries/month free tier).

### Tavily

Search API purpose-built for AI agents. Returns structured results with AI-generated answers.

```json
{ "tavilyApiKey": "tvly-your-key" }
```

Get a key at [tavily.com](https://tavily.com) (1,000 queries/month free tier).

### Serper

Google search results via a simple REST API. Returns organic results, knowledge graph, and answer boxes.

```json
{ "serperApiKey": "your-key" }
```

Get a key at [serper.dev](https://serper.dev) (2,500 free queries on signup).

## Provider priority

When `provider` is set to `auto` (or omitted), Crobot tries providers in this order, skipping any without a configured API key:

1. Exa
2. Perplexity
3. Gemini
4. Brave
5. Tavily
6. Serper

The first available provider is used for all searches in the session. If a provider fails during a search, Crobot automatically falls back through the remaining available providers.

## What works with no configuration

| Tool | With no API keys |
|---|---|
| `web_search` | Returns a helpful error listing which provider keys to add |
| `fetch_content` | Works for standard web pages, GitHub repos, and YouTube page metadata using HTTP fetch + Readability + GitHub API + Jina Reader |
| `get_search_content` | Works -- retrieves stored fetch results |

`fetch_content` uses:
- **Web pages**: Mozilla Readability to extract clean article content from HTML, with Jina Reader (`r.jina.ai`) as a free fallback. No API key needed.
- **GitHub repos**: Uses the public GitHub API to list repo trees, fetch file contents, and retrieve READMEs. No API key needed for public repos.
- **YouTube videos**: Falls back to scraping page metadata (title + description) without an API key. For full video understanding (transcript, visual analysis, answering questions about the video), a Gemini API key is required.

## Web search tool

The `web_search` tool accepts a single query or multiple queries for comprehensive research:

```json
{
  "query": "single search query",
  "queries": ["query one", "query two"],
  "numResults": 5,
  "includeContent": false,
  "recencyFilter": "day | week | month | year",
  "domainFilter": ["github.com", "-reddit.com"],
  "provider": "auto | exa | perplexity | gemini | serper | brave | tavily"
}
```

- `query` / `queries` -- Single query or batch of 2-4 varied queries for broader coverage
- `numResults` -- Results per query (default: 5, max: 20)
- `includeContent` -- Fetch full page content in background (notify when ready)
- `recencyFilter` -- `day`, `week`, `month`, or `year`
- `domainFilter` -- Include or exclude domains (prefix with `-` to exclude)
- `provider` -- Override the default provider for this search

Results are stored and can be retrieved later with `get_search_content`.

## Fetch content tool

The `fetch_content` tool extracts readable content from web pages, GitHub repositories, and YouTube videos:

```json
{
  "url": "https://example.com/article",
  "urls": ["url1", "url2"],
  "prompt": "question about this video",
  "timestamp": "23:41-25:00",
  "frames": 6,
  "forceClone": false
}
```

- `url` / `urls` -- Single URL or multiple URLs
- `prompt` -- Question to ask about a YouTube video (requires Gemini API key)
- `timestamp` -- Extract video frames at a timestamp or range (`"23:41"`, `"23:41-25:00"`, `"85"`)
- `frames` -- Number of frames to extract (max 12)
- `forceClone` -- Force clone large GitHub repos exceeding the size threshold

**GitHub URLs** (`github.com/{owner}/{repo}`) are handled via the GitHub API:
- Root URLs return repo description, file tree, and README
- `/blob/` paths return file contents
- `/tree/` paths return directory listing
- `/commit/` paths return commit message
- No authentication required for public repos

**YouTube URLs** are handled in two tiers:
- **With Gemini API key**: Full video understanding via Gemini (transcript, visual analysis, answers with `prompt` parameter)
- **Without key**: Falls back to scraping page metadata (title + description only)

Content over 30,000 characters is truncated in-line but stored in full for retrieval via `get_search_content`.

## Get search content tool

Retrieve full content from a previous `web_search` or `fetch_content` call:

```json
{
  "responseId": "abc12345",
  "query": "original search query",
  "url": "https://example.com"
}
```

- `responseId` -- Direct lookup by stored result ID
- `query` -- Look up stored search results by query text
- `queryIndex` -- Select which result when multiple match the same query (default: 0)
- `url` -- Look up stored fetch results by URL
- `urlIndex` -- Select which result when multiple match the same URL (default: 0)
