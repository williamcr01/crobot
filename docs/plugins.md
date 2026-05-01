# WASM Plugins

Crobot loads WASM plugins from configured plugin directories and lets them add tools, middleware hooks, and slash commands.

## Configuration

Plugins are configured in `~/.crobot/agent.config.json`:

```json
{
  "plugins": {
    "enabled": true,
    "directories": ["~/.crobot/plugins"],
    "permissions": ["file_read", "file_write", "bash", "tool_call", "send_message"]
  }
}
```

`permissions` are currently the default permission set for all loaded plugins.

## Commands

```text
/plugins
```

Lists loaded plugins, tools, hooks, commands, permissions, and load errors.

```text
/reload
```

Unloads plugin tools/commands/modules and reloads all plugins from disk.

## ABI v1

A plugin must export:

```text
malloc(size: i32) -> i32
free(ptr: i32)
describe() -> i64
```

Functions returning data return a packed `i64`:

```text
(ptr << 32) | len
```

Returned data is UTF-8 JSON in guest memory. The host reads and then frees returned buffers.

Optional exports:

```text
execute(name_ptr, name_len, args_ptr, args_len) -> i64
pre_prompt(json_ptr, json_len) -> i64
post_response(json_ptr, json_len) -> i64
pre_tool_call(json_ptr, json_len) -> i64
post_tool_result(json_ptr, json_len) -> i64
on_event(json_ptr, json_len)
execute_command(cmd_ptr, cmd_len, args_ptr, args_len) -> i64
```

## Manifest

`describe()` returns:

```json
{
  "abi_version": 1,
  "name": "echo",
  "version": "1.0.0",
  "description": "Echo plugin",
  "tools": [
    {
      "name": "echo",
      "description": "Echo text",
      "input_schema": {
        "type": "object",
        "properties": {
          "text": {"type": "string"}
        },
        "required": ["text"]
      }
    }
  ],
  "hooks": ["pre_tool_call"],
  "commands": [
    {"name": "echoctl", "description": "Echo command", "args": "<text>"}
  ]
}
```

## Tool protocol

`execute(name, args)` receives:

```json
{"name":"echo","args":{"text":"hello"}}
```

and returns:

```json
{"content":"hello","error":""}
```

If `error` is non-empty, the tool call fails.

## Hook protocols

### `pre_prompt`

Input/output:

```json
{
  "system_prompt": "...",
  "messages": [
    {"role":"user","content":"hello"}
  ]
}
```

### `post_response`

Input/output:

```json
{
  "Text": "assistant response",
  "Usage": null
}
```

### `pre_tool_call`

Input:

```json
{"name":"bash","args":{"command":"ls"}}
```

Output:

```json
{"name":"bash","args":{"command":"pwd"},"skip":false}
```

Set `skip: true` to prevent the tool call.

### `post_tool_result`

Input:

```json
{"name":"bash","args":{"command":"ls"},"result":{"stdout":"..."}}
```

Output:

```json
{"result":"modified result"}
```

### `on_event`

Receives stable event JSON:

```json
{"type":"text_delta","data":{"text_delta":"hello"}}
```

## Command protocol

`execute_command(command, args)` returns:

```json
{"output":"command output","error":""}
```

## Host functions

Imported from module `env`:

```text
host_log
host_config_get
host_env_get
host_file_read
host_file_write
host_tool_call
host_send_message
host_get_cwd
```

Restricted host functions require matching plugin permissions. `host_tool_call` does not allow recursive plugin tool calls.

## Safety model

- Plugins run in isolated wazero runtimes.
- WASI is instantiated without filesystem preopens, inherited environment, or inherited args.
- Guest calls are serialized per plugin.
- Calls have timeouts.
- Native tools and native slash commands cannot be replaced by plugins.
