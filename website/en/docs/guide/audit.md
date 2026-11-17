# Audit Log & Config History

Gately holds DNS keys and certificate private keys, so who did what and when, and how the configuration changed, should always be traceable and reversible.

## Audit log

The "Audit Log" tab of the sidebar page "System → Logs" lists the following actions in reverse chronological order. You can filter by category or search by path, user or IP:

| Category | What is recorded |
| --- | --- |
| Sign-in | Successful and failed sign-ins with a password, Passkey or third-party provider, and sign-outs |
| Change | Every write operation made from the admin UI or with an API token (create, update, delete, enable/disable, rollback and so on) |
| Sensitive access | Downloading configuration backups, diagnostics bundles or certificate private keys, opening the Web terminal or a container terminal, and similar |

Each record contains the time, the actor (username or `api-token:<token name>`), source IP, method, path and status code. **Request bodies and secrets are never recorded.**

- The log is written to `audit.log` (JSON Lines) in the config directory. The file is rotated once it exceeds 5 MiB, and 3 rotated files are kept;
- When the admin UI sits behind a reverse proxy on the same host, the real source is recorded and the proxy address it passed through is noted separately; see [Security Model](/en/docs/guide/security) for how this works;
- Changes and sensitive access are emitted as `audit.change` and `audit.access` events, which you can push from the [Notification Center](/en/docs/modules/notify) with the "Configuration changes" and "Security & sign-ins" templates;
- Read-only accounts and API tokens cannot view the audit log.

## Config history and rollback

Before overwriting a configuration file, Gately saves the old content as a history version under `history/<name>/` in the config directory, keeping the latest 30 versions per configuration. `account.json` (account credentials) and `tokens.json` (tokens) have no history.

In the "Config History" tab of "System → Logs":

1. Choose a configuration file to see the save time and size of each version;
2. Click "Compare" to see a line-by-line diff between that version and the current configuration;
3. Click "Roll back" to restore that version; Gately then restarts automatically to apply it.

The current content before a rollback is also saved as a new history version, so a rollback can itself be undone.
