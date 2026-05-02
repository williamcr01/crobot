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

## Plugin authoring (Rust)

Plugins are WebAssembly modules compiled to `wasm32-wasip1`. Any language that targets WASI works, but Rust is the primary supported toolchain.

### Required exports

Every plugin must export:

```text
malloc(size: i32) -> i32
free(ptr: i32)
describe() -> i64
```

### malloc/free export caveat

The Rust `wasm32-wasip1` target links wasi-libc which provides `malloc` and `free`, but the toolchain does **not** auto-export them. You must force export them via linker flags in `.cargo/config.toml`:

```toml
[target.wasm32-wasip1]
rustflags = ["-C", "link-args=--export=malloc --export=free"]
```

Without this, crobot will report a load error: `missing required export malloc`.

### Full example: coder-utils

Source lives at `~/.crobot/plugins/coder-utils/`. Cargo.toml:

```toml
[package]
name = "coder_utils"
version = "1.0.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]

[dependencies]
serde = { version = "1", features = ["derive"] }
serde_json = "1"
uuid = { version = "1", features = ["v4"] }
```

Building:

```bash
cargo build --target wasm32-wasip1 --release
cp target/wasm32-wasip1/release/coder_utils.wasm ~/.crobot/plugins/coder-utils.wasm
```

### Plugin structure

A plugin declares its capabilities in `describe()` by returning a JSON manifest (see [Manifest](#manifest)). The manifest lists tools, hooks, and commands the plugin implements.

### Tool implementation

A tool is a function callable by the LLM. Implement it by exporting `execute`:

```rust
#[no_mangle]
pub extern "C" fn execute(
    name_ptr: u32,
    name_len: u32,
    args_ptr: u32,
    args_len: u32,
) -> u64 {
    // 1. Read tool name and args from guest memory
    let name = read_string(name_ptr, name_len);
    let args = read_json(args_ptr, args_len);

    // 2. Process
    let result = match name.as_str() {
        "reverse_text" => {
            let text = args["text"].as_str().unwrap_or("");
            let reversed: String = text.chars().rev().collect();
            json!({ "content": reversed, "error": "" })
        }
        _ => json!({ "content": null, "error": format!("unknown tool: {}", name) }),
    };

    // 3. Write result JSON to guest memory, return packed ptr|len
    write_json_to_memory(&result)
}
```

Input format: `name` is a UTF-8 string at `(name_ptr, name_len)`, `args` is UTF-8 JSON at `(args_ptr, args_len)`.

Output format: packed `i64` pointer to a JSON object with `content` (any) and `error` (string). If `error` is non-empty the tool call is considered failed.

### Hook implementation

Hooks run before or after certain actions. Implement them by exporting the corresponding function (e.g. `pre_tool_call`, `pre_prompt`, `post_response`, etc.).

The `pre_tool_call` hook runs before every tool invocation. It receives JSON input and returns JSON output:

```rust
#[no_mangle]
pub extern "C" fn pre_tool_call(json_ptr: u32, json_len: u32) -> u64 {
    let input: PreToolCallInput = read_json(json_ptr, json_len);

    // Check tool name
    if input.name == "bash" {
        if let Some(cmd) = input.args.get("command").and_then(|c| c.as_str()) {
            // Return skip: true to block the tool call
            if is_dangerous(cmd) {
                return write_json_to_memory(&PreToolCallOutput { skip: true });
            }
        }
    }

    // Return skip: false to allow
    write_json_to_memory(&PreToolCallOutput { skip: false })
}
```

**Important:** The hook only blocks what you explicitly check. It is not a universal safety net. For example, checking for `rm -rf /` will not block `rm some-file` unless you add that pattern too.

### Command implementation

A command is a slash command (like `/uuid`). Implement it by exporting `execute_command`:

```rust
#[no_mangle]
pub extern "C" fn execute_command(
    cmd_ptr: u32,
    cmd_len: u32,
    args_ptr: u32,
    args_len: u32,
) -> u64 {
    let cmd = read_string(cmd_ptr, cmd_len);

    let result = match cmd.as_str() {
        "uuid" => {
            let u = uuid::Uuid::new_v4();
            json!({ "output": u.to_string(), "error": "" })
        }
        _ => json!({ "output": "", "error": format!("unknown command: {}", cmd) }),
    };

    write_json_to_memory(&result)
}
```

Input: `cmd` is a UTF-8 string at `(cmd_ptr, cmd_len)`, `args` is a JSON array at `(args_ptr, args_len)`.

Output: JSON with `output` (string) and `error` (string). If `error` is non-empty the command fails.

### Memory helpers

The host reads plugin memory directly via the ABI. Your `malloc` must allocate memory that remains valid until the host calls `free`. Typical implementation using Rust's global allocator with a size header:

```rust
use std::alloc::{alloc, dealloc, Layout};

const HEADER_ALIGN: usize = 8;

#[no_mangle]
pub unsafe extern "C" fn malloc(size: usize) -> *mut u8 {
    let layout = Layout::from_size_align(HEADER_ALIGN + size, HEADER_ALIGN).unwrap();
    let ptr = alloc(layout);
    if ptr.is_null() {
        return ptr;
    }
    ptr::write(ptr as *mut usize, size);
    ptr.add(HEADER_ALIGN)
}

#[no_mangle]
pub unsafe extern "C" fn free(ptr: *mut u8) {
    if ptr.is_null() {
        return;
    }
    let header = ptr.sub(HEADER_ALIGN) as *mut usize;
    let size = ptr::read(header);
    let layout = Layout::from_size_align(HEADER_ALIGN + size, HEADER_ALIGN).unwrap();
    dealloc(header as *mut u8, layout);
}
```

However, when using wasi-libc (the default for `wasm32-wasip1`), you can reuse its `malloc`/`free` via `extern "C"` declarations and force their export via `.cargo/config.toml` as described above. This is simpler and generates smaller binaries.
