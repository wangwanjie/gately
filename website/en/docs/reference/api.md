# API Tokens and OpenAPI

Every API used by Gately's management UI can be called over HTTP. Scripts and automation authenticate with API tokens, and the API reference is generated automatically from the registered routes.

## API tokens

UI path: sidebar "System" group → "API Tokens".

### Create a token

Click "New Token" and fill in:

| Item | Description |
| --- | --- |
| Name | Required, up to 64 characters, identifies the purpose, such as "monitoring script" |
| Expiry | Choose "Never expires", 7 days, 30 days, 90 days, 1 year or a custom date; a custom time must be later than now |
| Permissions | Required, select at least one. A new token has no permissions by default |

Each instance can store up to 100 tokens.

After creation, the dialog shows the plaintext token only once, in the format `gtl_` followed by 48 hexadecimal characters. Only the SHA-256 hash is stored on disk. If you lose it, you can only "Regenerate" it, and the old value stops working immediately. The list identifies tokens with a hint in the form `gtl_xxxx…yyyy`, and shows the last used time and source IP (persisted to disk with a delay of up to one minute).

Editing a token lets you change its name, permissions, expiry and enable switch. Changes take effect immediately and the plaintext token stays the same. An expired token can keep its original expiry while you change only the other fields.

### Authentication

Send `Authorization: Bearer <token>` in the request header:

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status
```

Request bodies use `Content-Type: application/json`, and errors are returned uniformly as `{"error": "message"}`. If a token is disabled, expired or unknown, the request is still checked as a login session and ends up with a not-logged-in error.

### Scopes

A permission has the form `<scope>:read` or `<scope>:write`, and write includes read. A scope is a module ID (such as `ddns`, `portforward`, `notify`, `backup`) or one of five built-in scopes:

| Scope | Name | Available permissions | Endpoints |
| --- | --- | --- | --- |
| `status` | System status | `status:read` | `/api/status`, `/api/system/info`, `/api/system/update` |
| `logs` | System logs | `logs:read` | `/api/logs` |
| `import` | Lucky config import | `import:write` | `/api/import/lucky` |
| `fileusers` | File service shared users | `fileusers:read`, `fileusers:write` | `/api/fileusers` |
| `metrics` | Global metrics (Prometheus) | `metrics:read` | `/api/metrics` |

Wildcards:

| Form | Meaning |
| --- | --- |
| `*:read` | Read-only access to all modules |
| `*:write` or `*` | Read/write access to all modules |
| `ddns:read` | Read-only access to DDNS only |

The permission an endpoint needs is determined by its path and method:

- A path `/api/<module>/...` needs a permission for the `<module>` scope: GET and HEAD require `read`, and other methods require `write`.
- Module logs `GET /api/<module>/logs` are read-only, and clearing logs (DELETE) is a write operation.
- The following GET endpoints return sensitive content or open an interactive channel, so they are treated as `write`: endpoints whose path contains a `ws` (WebSocket terminal), `exec`, `download`, `export`, `read`, `config`, `preview-archive`, `files`, `sftp` or `secrets` segment, for example certificate download, running commands in a container and reading files. Masked passwords and keys can be fetched in plain text from the `.../secrets` endpoint of the matching resource; this needs write permission, and read-only accounts and read-only tokens get 403. Container details at `/api/docker/containers/{id}` need only `docker:read`; environment variable values and start commands in them are masked without `docker:write`, and the plain text is available from `/api/docker/containers/{id}/secrets`.
- The legacy endpoints `PUT /api/webdav/shared-users`, `PUT /api/ftp/shared-users` and `PUT /api/smb/shared-users` all modify the single list of shared users used by the three services, so they require `webdav:write`, `ftp:write` and `smb:write` together; the newer `/api/fileusers` endpoint uses the `fileusers` scope.
- `GET /api/session` and `GET /api/openapi.json` accept any valid token.
- Write access to the following modules can run commands on the server, which equals command execution as the Gately process user: `ddns` (command IP source), `cron` (scheduled tasks), `guard` (command log source), `stun` (rule scripts), `ssl` (mapping scripts and local deploy commands), `webterminal`, `docker` (exec in containers and Compose), `frp`, `filebrowser` and `cloudflared` (custom program paths). These modules are marked "Runs commands" when you create a token; grant them only to scripts you fully trust. When a token calls the Lucky import (`import:write`), imported items are always disabled; `enable=1` only applies to signed-in sessions.

### Operations tokens cannot perform

The following endpoints accept login sessions only, and any token access returns 403:

- Account and two-factor authentication: `/api/account`
- Token management: `/api/tokens`
- Audit log and configuration history: `/api/audit`, `/api/history`
- System settings, system token, restart, online upgrade: `/api/system/settings`, `/api/system/token`, `/api/system/restart`, `/api/system/upgrade`
- Diagnostics bundle, self-check and onboarding: `/api/system/diagnostics`, `/api/system/selfcheck`, `/api/system/onboarding`
- Configuration backup download and restore: `/api/system/backup`, `/api/system/restore`
- Local backup file download: `/api/backup/local`
- Web Terminal security settings: `/api/webterminal/security`
- Login and logout: `/api/login`, `/api/logout`

A request with insufficient permissions returns `token lacks permission <permission>` and records a warning in the system logs, including the token name, source IP and the rejected endpoint.

### Legacy token

The single global token from older versions is migrated automatically on upgrade into a token named "Legacy token (migrated from an older version)" with the permission `*`, marked "Legacy" in the list. Create new tokens with only the permissions they need and delete it. "Regenerate" on the legacy token also removes the legacy mark.

::: tip Recommendation
Create a separate token for each script, select only the permissions it needs, and set an expiry. If a token leaks, delete or regenerate just that one.
:::

## OpenAPI and API docs

### The docs page in the management UI

Sidebar "System" group → "API Docs". The page shows the Base URL and authentication notes, lists all endpoints grouped by feature, and supports searching by path or summary. Expanding an endpoint shows the required permission, parameters, request body fields and a copyable curl example. Endpoints that need a login session are marked "Login session only" and have no curl example.

### Get the OpenAPI document

While an instance is running, `GET /api/openapi.json` returns an OpenAPI 3.0 document that any valid token or login session can read:

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/openapi.json -o openapi.json
```

The document uses the `bearerAuth` security scheme and adds extension fields to each endpoint:

| Extension field | Meaning |
| --- | --- |
| `x-required-scope` | The token permission needed to access this endpoint |
| `x-token-allowed` | Whether tokens may access it; `false` means login session only |
| `x-scope-domains` (document root) | List of all scopes |

The endpoint list used by [CLI and MCP](/en/docs/guide/cli-mcp) is read from this document.

### Command-line export

The `-api-docs` flag builds the application in a temporary configuration directory (it listens on no port and does not affect your existing configuration), writes the documents to the specified directory and exits:

```bash
gately -api-docs ./api-docs
```

It outputs two files:

- `openapi.json`: the OpenAPI 3.0 description
- `API.md`: the Markdown API docs, with a quick start, a scope table and endpoint tables grouped by feature

## Examples

```bash
# View system status (requires status:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# List all DDNS tasks (requires ddns:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# Sync a DDNS task immediately (requires ddns:write; take the key from the list returned by the previous step)
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/KEY/sync

# Fetch only module logs with a sequence number greater than 10 (requires ddns:read)
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?since=10"

# Fetch only the newest 200 entries, then pass the smallest seq as before to page back
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?limit=200"
curl -H "Authorization: Bearer gtl_xxxx" "http://localhost:16680/api/ddns/logs?before=1200&limit=200"
```

## Response language

Responses follow the language in the `Accept-Language` header, or the server language (the interface language in Settings) when the header is missing. Only text generated by Gately is translated: error responses, status and logs, provider and permission descriptions, and message fields in configuration data such as `error`, `lastError`, `details`, `warnings`, `message` and `title`. Names, remarks, groups, commands, domains, record values and other content you entered are always returned as saved, so reading and saving them back never rewrites them. Add `-H "Accept-Language: zh-CN"` if a script needs the messages in Chinese.

## Full API reference

See the [API Reference](/en/docs/reference/api-reference) for all endpoints, parameters and required permissions.
