# Web Terminal

Connect to SSH and Telnet servers or the local shell from your browser. Sessions are persisted on the server and can be reattached after a disconnect. The entry point is "Web Terminal" in the sidebar.

## Connection types

| Type | Description |
| --- | --- |
| SSH | Supports password and private key authentication (paste a private key or specify a `keyFile` path on the server; key passphrases are supported). You can set the `workDir` to enter after login, `keepAlive`, the connection timeout (1 to 120 seconds, default 15) and a proxy |
| Telnet | Default port 23. When prompts such as `login:`, `username:` and `password:` are detected, the corresponding field is auto-filled once each. A proxy can be set |
| Local shell | Opens a shell directly on the device running Gately. You can specify the shell path, working directory and extra environment variables |

Each connection can have a remark. The connection list supports drag-and-drop sorting and "Test connection": for SSH it returns the server version and latency, for Telnet it only verifies TCP reachability (including through the proxy), and local shell connections cannot be tested. When you edit a connection, the password, private key, key passphrase and proxy password are shown masked, and the original values are kept if left unchanged.

Under "Settings" you can maintain "SSH credential presets" (a password or private key, optionally with a default username). Choose a credential preset when adding or editing an SSH connection to reuse it. Several connections can share one credential, and edits to the preset apply the next time those connections connect. A preset still referenced by a connection cannot be deleted.

### Proxy

SSH and Telnet connections can each use their own outbound proxy of type `socks5` or `http`, with an optional proxy username and password.

### Trust on first use for host keys

When an SSH connection is first established, the server's host fingerprint is recorded and trusted, and it is verified on every later connection. After a server is reinstalled or its keys change, run "Reset host fingerprint" on the connection to trust it again.

## Sessions

- Sessions are persisted on the server. After a browser refresh or disconnect you can reattach from the session list; on attach the output buffer is replayed first, then output is streamed live;
- Multiple clients can attach to the same session at once, with shared input and broadcast output;
- Sessions can have a remark. The session list shows creation time, last activity time, window size and bytes sent and received;
- After a page refresh, sessions that are still alive are restored as tabs, along with the split layout;
- The terminal has built-in search (type a keyword, press Enter for the next match and Shift+Enter for the previous one), copy, paste, clear screen, font size adjustment, full screen and several terminal color schemes (following the interface theme by default, or fixed to one scheme).

### Settings

Adjust these in "Settings" at the top right of the page:

| Field | Default | Description |
| --- | --- | --- |
| `allowLocalShell` | Off | Whether opening the local shell is allowed |
| `maxSessions` | 20 | Maximum concurrent sessions, up to 1000. Creating a session beyond the limit returns HTTP 429 |
| `idleTimeout` | 0 | A session ends automatically after this many minutes without input or output; 0 means no limit |
| `bufferSize` | 256 | Replay buffer kept per session (KB), up to 16384 |
| `heartbeatInterval` | 30 | WebSocket heartbeat interval (seconds), up to 600 |
| `sessionKeepAlive` | 0 | Minutes a session is kept after the browser disconnects; 0 ends it immediately on disconnect, up to 10080 |

::: tip
To be able to reconnect after a disconnect, set `sessionKeepAlive` to a value greater than 0. With 0, the session ends as soon as the browser disconnects.
:::

## SFTP file manager

The folder icon in the terminal toolbar opens the file manager panel. SSH connections operate on remote files through SFTP, and local shell connections operate directly on the file system of the device running Gately. Telnet connections have no file manager. The local file manager requires "Allow local shell" to be on.

| Capability | Description |
| --- | --- |
| Browse | Breadcrumb and parent-directory navigation, directories first and sorted by name, with switchable sort order; shows permissions, size, modified time and owner |
| Create | New folder, new empty file |
| Upload | Multi-file upload; on desktop, whole folders can be uploaded (subdirectory structure is kept). Files with the same name are overwritten, and the panel shows upload progress |
| Download | Streaming download with resume support (`Range`) |
| Online editing | Preview and save text files; files larger than 1 MB or binary files cannot be previewed |
| Rename, copy, paste | When copying into the same directory, the file is renamed automatically to `name copy.ext`, `name copy 2.ext` |
| Delete | Multi-select batch delete; directories are deleted recursively; the root directory is protected |
| Compress and extract | Compress to `zip` or `tar.gz`; extract `zip`, `tar`, `tar.gz` and `tgz`; archive contents (up to 10000 entries) can be previewed before extracting. Compression and extraction are done by Gately itself, so no commands need to be installed on the remote host |
| Change permissions | Octal permissions (0 to 7777), with a recursive option for directories |
| Quick access | Save frequently used directories for each connection and jump to them in one click |

When extracting, an archive containing any entry with `..` is rejected as a whole to prevent directory traversal, symbolic link and hard link entries are skipped, and only the rwx bits of file permissions are kept. On mobile, the panel provides a multi-select mode and an action sheet.

## Shortcuts

The "Shortcuts" dropdown lists the global shortcuts, and clicking one sends it to the current terminal. Each shortcut has a `name`, a `command` and `sendEnter` (whether to press Enter automatically after sending). Add, edit and delete shortcuts through "Manage shortcuts" at the bottom of the dropdown.

## Split view

The "Split" menu in the toolbar offers side-by-side, stacked and grid layouts, with up to 4 panes at a time. Once split view is on, any connection you open joins the split automatically, and you can also click another tab to add it. The title bar of a pane lets you remove a session from the split. The split layout is stored on the server and restored after a page refresh. On narrow screens, split view is turned off automatically, and the saved layout is not deleted.

## Mobile support

- On narrow screens, the connection list and the terminal are separate views, and the back button at the top left of the terminal view returns to the list;
- The terminal area follows the visual viewport and shrinks when the soft keyboard appears;
- The virtual key bar is a single row you can swipe sideways. It provides `Esc`, `Tab`, `Ctrl`, `Alt` (sticky), arrow keys (hold to repeat), common symbols and a shortcuts entry, with a keyboard button pinned on the right that shows or hides the keyboard. By default the bar appears and disappears with the on-screen keyboard; while the keyboard is closed, a floating button in the bottom-right corner brings it back. To use arrow keys, paging and other keys without the keyboard, turn on **Always show key bar** in the **More** menu.

## Security

A terminal is equivalent to running arbitrary commands on the target host, so Gately applies these restrictions by default:

- **The local shell is off by default** and must be turned on explicitly in the settings;
- **Agreement confirmation**: the first time you enable the local shell, create a session, attach to a session or use the file manager, a user agreement appears and you must tick it to continue;
- **Two-factor authentication (2FA)**: after you bind a TOTP authenticator by scanning the QR code in the security settings, the actions above require a 6-digit code. After verification, the current login token does not need to verify again for 30 minutes; after 5 consecutive wrong codes, that login token is locked for 1 minute;
- Connection passwords, private keys and proxy passwords are masked in API output.

::: warning
Before enabling the local shell, turn on two-factor authentication for the management UI, and use it together with "LAN-only access" or a VPN.
:::

## API

See the [Web Terminal API](/en/docs/reference/webterminal-api) for the complete API reference.
