# TODO before v0.1.0

## Features
- [ ] Web search, fetch tools
  - [ ] Exa
  - [ ] Gemini
  - [ ] More?
- [ ] Subagent support
- [x] More providers (openrouter, openai, openai-responses-ws, openai-codex, anthropic, deepseek, gemini, kimi, kimi-code, opencode-zen, opencode-go)
- [x] Plugin system (WASM via wazero)
- [ ] MCP support
- [x] Headless use (crobot -p "prompt" or crobot --prompt "prompt")
- [x] Make all tool calls look better
  - [x] Better formatting
  - [x] Show cleaner diff with edit/write tools
  - [x] remove the ok/error
- [x] Add more tools
  - [x] grep
  - [x] find
  - [x] ls
  - [ ] recall session
  - [ ] web search/fetch tools
- [x] Allow agent to use multiple read only tools at the same time in one turn
- [ ] Formatting newline in centered alignment mode
- [ ] Memory for the agent
- [x] Ship with docs (README.md, docs/*.md)
- [x] Arrow keys to scroll messages
- [x] Show session name
- [x] Copy like claude code

## Bugs
- [x] Bug when moving cursor in input field with arrow keys
- [x] Bug no line wrapping in input field
- [x] Bug with provider and model resetting and not remembering
- [x] Show tool calls right when called not only after they are succesful
