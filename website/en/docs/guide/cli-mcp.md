# Command Line & MCP

The `gately` executable ships with two subcommands: `gately ctl` calls the management API of a running instance from the terminal, and `gately mcp` exposes the same endpoints to AI clients as MCP tools. Both reach the instance over HTTP with an [API token](/en/docs/reference/api). The endpoint list comes from the instance's `/api/openapi.json`, and the token determines the permissions.

In addition, `gately reset-password` and `gately reset-totp` work directly on the local configuration directory without the API, to recover the admin account after a forgotten password or a lost authenticator; see [Reset password and two-factor authentication](/en/docs/guide/install#reset).

`gately service install|uninstall|start|stop|restart|status` registers Gately as a system service that starts at boot; see [Start at boot](/en/docs/guide/install#service).

## Connection settings

Connection details are resolved in the following order, with later sources overriding earlier ones:

1. Default address `http://127.0.0.1:16680`
2. Config file `ctl.json`
3. Environment variables `GATELY_URL` and `GATELY_TOKEN`
4. Command-line options `-url` and `-token`

The config file is `gately/ctl.json` under the user config directory of your system (`~/.config/gately/ctl.json` on Linux, `~/Library/Application Support/gately/ctl.json` on macOS). Example contents:

```json
{
  "url": "http://127.0.0.1:16680",
  "token": "gtl_xxxx",
  "insecure": false
}
```

`gately ctl login` generates this file.

## gately ctl

### Subcommands

| Command | Description |
| --- | --- |
| `gately ctl ls [keyword]` | List the endpoints the token can access, showing method, path and summary; optionally filter by keyword |
| `gately ctl get <path>` | Send a GET request |
| `gately ctl post <path> [JSON or @file]` | Send a POST request |
| `gately ctl put <path> [JSON or @file]` | Send a PUT request |
| `gately ctl delete <path> [JSON or @file]` | Send a DELETE request |
| `gately ctl login <url> <token>` | Verify the connection and save it to the config file |

The path must start with `/api/`. The request body can be a JSON literal, `@filename` (read from a file) or `-` (read from standard input), and must be valid JSON. The `patch` method is also available, and `ls` can be written as `list`.

### Connection options

Options go before the subcommand:

| Option | Description |
| --- | --- |
| `-url <url>` | Management address, starting with http or https |
| `-token <token>` | API token |
| `-insecure` | Skip HTTPS certificate verification, for internal instances with self-signed certificates |
| `-raw` | Print the raw response; by default JSON is formatted |

A successful command exits with code 0. When the API returns a non-2xx status, it prints `错误: HTTP 状态码: 说明` (Error: HTTP status code: message) and exits with 1; argument problems exit with 2.

### Examples

```bash
# Save the connection config (first requests /api/session to verify the token)
gately ctl login http://192.168.1.2:16680 gtl_xxxx

# List the endpoints the token can access, filtered to lines containing ddns
gately ctl ls ddns

# Read the system status
gately ctl get /api/status

# Wake a device
gately ctl post /api/wol/devices/abc123/wake

# Disable a DDNS task
gately ctl put /api/ddns/tasks/abc/enable '{"enable":false}'

# Read the request body from a file
gately ctl post /api/ddns/tasks @task.json

# Use environment variables for a single command
GATELY_URL=https://gw.example.com GATELY_TOKEN=gtl_xxxx gately ctl get /api/status
```

::: tip Tip
`ls` lists the endpoints the token can access. Endpoints available only to login sessions (accounts, token management, backup restore and so on) do not appear. Whether a write operation is allowed depends on the token's permissions; insufficient permissions return 403.
:::

## gately mcp

`gately mcp` serves MCP over standard input and output (JSON-RPC, one message per line) and is launched as a child process by an AI client.

### Options

| Option | Description |
| --- | --- |
| `--url <url>` | Management address |
| `--token <token>` | API token |
| `--write` | Also expose write tools |
| `--insecure` | Skip HTTPS certificate verification |

Connection details come from the same sources as `gately ctl` (config file, `GATELY_URL`, `GATELY_TOKEN`, options).

### Tool generation rules

- After startup, the server reads the instance's OpenAPI document when the client first requests the tool list, and generates one tool for each endpoint the token can access.
- Tool names are built from the method and path, such as `get_ddns_tasks` and `put_ddns_tasks_key_enable`, up to 64 characters; a sequence number is appended to resolve duplicates.
- Path parameters become required string parameters; GET tools take an optional `query` parameter (such as `since=10`); endpoints with a request body take a `body` parameter whose schema comes from OpenAPI.
- Tool descriptions include the endpoint summary, the method and path, and the required permission; tools carry the `readOnlyHint` annotation, and DELETE tools carry `destructiveHint`.
- Output longer than 200000 characters is truncated.

### Read-only by default

Without `--write`, only GET and HEAD tools are exposed, and endpoints such as `.../secrets` that return plain-text passwords are left out; they are provided only with `--write`. Add `--write` when you want the AI to modify configuration. Even with writing enabled, the token's own permissions still apply, so the safer approach is to create a dedicated token for MCP and grant only the permissions it needs.

::: warning Note
`--write` lets the AI client call every write endpoint within the token's permissions, including deletion. Validate your workflow with a read-only token first, then grant write permission as needed.
:::

### Client configuration

For clients that use the `mcpServers` format, such as Claude Desktop and Cursor:

```json
{
  "mcpServers": {
    "gately": {
      "command": "gately",
      "args": ["mcp"],
      "env": {
        "GATELY_URL": "http://127.0.0.1:16680",
        "GATELY_TOKEN": "gtl_xxxx"
      }
    }
  }
}
```

To enable writing, change `args` to `["mcp", "--write"]`. `command` can be the absolute path to `gately`.

Claude Code adds the server from the command line:

```bash
claude mcp add gately -e GATELY_URL=http://127.0.0.1:16680 -e GATELY_TOKEN=gtl_xxxx -- gately mcp
```
