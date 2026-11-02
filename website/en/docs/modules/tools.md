# File and Ops Tools

## WebDAV

Configure it under "WebDAV Service" in the sidebar "File Services" group.

| Field | Default | Description |
| --- | --- | --- |
| `enable` | Off | Enable the service; at least one user is required when enabled |
| `listenIp` | Empty | Listen address; leave empty to listen on all addresses |
| `port` | 18800 | Listen port |
| `tls` | Off | Enable HTTPS; the certificate is provided per SNI by the [SSL Certificates](/en/docs/modules/ssl) module |
| `users` | None | Multiple users. Each has `name`, `password`, `root` (an absolute directory path) and `readOnly`, or uses `mounts` to mount several directories |
| `sharedUsers` | Off | Allow [shared users](#shared-users) to sign in to this service |

Each user can map to one directory, or mount several directories:

- **Single directory**: set `root` and `readOnly`. After login, the root is that directory.
- **Multiple directories**: click "Mount multiple directories" in the UI, or fill in several entries in `mounts`. Each entry has `name` (display name, cannot contain `/`, unique within one user), `path` (an absolute path) and `readOnly`. After login, the root lists the directories by name. The root itself is not writable, and files cannot be moved between different directories.

## FTP

Configure it under "FTP Service" in the "File Services" group. FTP and FTPS are supported.

| Field | Default | Description |
| --- | --- | --- |
| `enable` | Off | Enable the service; at least one user is required when enabled |
| `listenIp` | Empty | Listen address; leave empty to listen on all addresses |
| `port` | 2121 | Listen port |
| `passiveStart` / `passiveEnd` | 0 | Passive-mode data port range. When both are 0 the system assigns ports randomly; when set, 1 ≤ start ≤ end ≤ 65535 must hold, and the range must be allowed in the firewall and port mappings |
| `publicHost` | Empty | IP advertised externally in passive mode. Behind NAT, enter the public or forwarded IP. Only IP addresses are supported; leave empty to use the connection address automatically |
| `tlsMode` | 0 | 0 allows both plain text and FTPS, 1 requires explicit FTPS (`AUTH TLS`), 2 is implicit FTPS (encrypted from connect). When non-zero, the certificate is provided per SNI by the [SSL Certificates](/en/docs/modules/ssl) module |
| `idleTimeout` | 900 | Seconds of idle time before a connection is closed |
| `connectionTimeout` | 30 | Timeout in seconds for establishing a data connection |
| `disableActiveMode` | Off | Use passive mode only, which suits NAT environments better |
| `users` | None | Multiple users, with the same fields as WebDAV: `name`, `password`, `root`, `readOnly` or `mounts`; user names must be unique |
| `sharedUsers` | Off | Allow [shared users](#shared-users) to sign in to this service |

## SMB

Configure it under "SMB Service" in the "File Services" group to share local directories with Windows Explorer, the macOS Finder, Linux and SMB clients on phones and TVs. The protocol implementation is built into Gately (based on [gosamba](https://github.com/ahmetozer/gosamba)) and supports SMB 2.0.2 to 3.1.1, NTLMv2 authentication and signing, with optional mandatory SMB 3 encryption; SMB 1 is not supported. Available on Linux and macOS only.

| Field | Default | Description |
| --- | --- | --- |
| `enable` | Off | Enable the service; at least one user is required when enabled |
| `listenIp` | Empty | Listen address; leave empty to listen on all addresses |
| `port` | 445 | Listen port. Windows only connects to 445; when the Samba that ships with a NAS such as fnOS or Synology already uses 445, choose another port, which macOS, Linux and most media players can put in the address |
| `requireEncryption` | Off | Require SMB 3 encryption on every connection; older devices and some TVs that only speak SMB 2 will not be able to connect |
| `mdns` | Off | Advertise the service over Bonjour so it shows up under Network in the macOS Finder; in Docker it only works with host networking |
| `users` | None | Multiple users with the same fields as WebDAV: `name`, `password`, `root`, `readOnly` or `mounts`; user names are case-insensitive and must be unique |
| `sharedUsers` | Off | Allow [shared users](#shared-users) to sign in to this service |

Shares belong to users, and a signed-in user only sees and connects to their own shares:

- **Single directory**: the share is named after the user, for example `\\192.168.1.2\alice` for the user `alice`.
- **Multiple directories**: each directory is a share whose `name` is the share name (case-insensitive and unique per user), and each can be read-only. Different users can have shares with the same name, each pointing at their own directory.

User and share names cannot contain `" / \ [ ] : ; | = , + * ? < >` and cannot start or end with `.`. NTLM authentication needs the plain password, so users whose password is stored as a bcrypt hash must set it again. Files are read and written as the Gately process.

"Connection addresses" at the bottom of the page lists the address of every share from the saved settings:

- Windows: enter `\\host\share` in the Explorer address bar, or run `net use Z: \\host\share /user:username`
- macOS: in the Finder choose Go → Connect to Server and enter `smb://host:port/share` (the port can be left out for 445)
- Linux: `mount -t cifs //host/share /mnt -o username=username,port=port`

To troubleshoot clients that cannot connect or fail to authenticate, set the environment variable `GATELY_SMB_DEBUG=1` and restart Gately; protocol debug logs are written to the runtime log. Remove the variable when done.

## Shared users {#shared-users}

WebDAV, FTP and SMB share one list of "shared users" with the same fields as each service's local users (name, password, `root` and `readOnly`, or `mounts`), stored in `fileusers.json` in the configuration directory. They are managed on the "File shared users" page in the "File Services" group of the sidebar, and saving applies to all three services; the local users of each service are unchanged. The top of the page shows whether each service allows shared users and lists shared users that have the same name as a local user or cannot be used by SMB.

- Each service decides on its own whether to accept shared users: turn on "Allow shared users" in the "Shared users" card of the service page (field `sharedUsers`) and save, and shared users can sign in to that service; the card also lists the saved shared users, and "Manage shared users" opens the "File shared users" page;
- When a local user has the same name, only the local user is checked (case-insensitive for SMB);
- Shared user names must be unique regardless of case and cannot contain a colon;
- Changes to shared users take effect at once: WebDAV checks the new settings on the next request, FTP and SMB restart, and connected clients need to reconnect;
- With the switch on and at least one shared user, the service can be enabled without any local user;
- A service enabled with shared users only must still have a shared user that can sign in after a change (for SMB, one usable by SMB), otherwise the change is rejected;
- If `fileusers.json` is broken by hand (for example an empty password or a directory that is not an absolute path), no shared user can sign in and the page shows a warning; fix it and save to restore them;
- SMB only accepts plain passwords, and user and share names must follow SMB's character rules; shared users that do not are reported on the "File shared users" page, the SMB page and in the runtime log and skipped, while WebDAV and FTP can still use them.

The API is `GET` / `PUT /api/fileusers`, which needs the `fileusers:read` / `fileusers:write` permission for API tokens; `services` in the response shows how each service uses them. The former `GET` / `PUT /api/<service>/shared-users` of each service are deprecated but still work and read and write the same list: reading uses the read permission of that service, and a change needs `webdav:write`, `ftp:write` and `smb:write` together.

**Self-service password change**: turn on Self-service password change on the File shared users page and save, and shared users can change their own password at `https://site-domain/__gately_auth/password` on any web service site with web authentication by entering the username, current password and new password (8–128 characters). WebDAV, FTP and SMB use the new password right away (FTP and SMB restart, so connected clients reconnect). Plaintext passwords stay plaintext and bcrypt hashes stay hashed; a wrong current password counts toward the same rate limit and automatic ban as web logins. The switch is stored as `selfService` in `fileusers.json`, with the same field name in the API. For web login shared users, see [self-service password change in the web service](/en/docs/modules/web#self-service-password).

## File Browser {#filebrowser}

Configured under "File Browser" in the "File Services" group. Gately hosts [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser), a web file manager that previews images, videos and documents, uploads and downloads files, creates share links and has its own users and permissions. The original filebrowser/filebrowser was archived in 2026 and no longer receives security fixes, so Gately hosts the maintained Quantum fork.

The program is not bundled with Gately. The "online install" card at the bottom of the page downloads FileBrowser Quantum from GitHub into `bin/filebrowser` under the config directory and supports updating, installing a specific version, a separate download proxy, rollback and uninstall; enabled instances restart after an update or rollback. See [Online install, update and rollback](/en/docs/modules/tunnels#install). "Restore defaults" in the page header deletes all instances and their data; see [Restore defaults](/en/docs/modules/tunnels#reset). You can also place the program yourself and enter its path. The program is looked up in this order: the path you entered, `bin/` under the config directory, then `PATH`; the page warns when the program found is not Quantum (for example the original one in `PATH`).

Each instance is a separate File Browser process:

| Field | Default | Description |
| --- | --- | --- |
| `listenIp` | empty | Listen address; empty listens on all IPv4 addresses. Use `127.0.0.1` when it is only reached through a Web Service reverse proxy |
| `port` | 8080 | Listen port |
| `baseUrl` | `/` | Base path; when reverse proxying by path in the Web Service (for example `/files`), enter the same path |
| `accessUrls` | Empty | Access URLs, several allowed, such as `https://files.example.com/` reverse proxied by the Web service; shown on the instance card. When empty, the URL is guessed from the listen address and port. The scheme, host and port of the first URL are written to File Browser's `http.externalUrl`, which share links use |
| `sources` | none | Shared directories, each with `path` (absolute), `name` (display name, defaults to the directory name) and `readOnly`; new users can access all of them by default |
| `authMethod` | `password` | Sign-in method: `password` for username and password, `proxy` to trust the username header from a reverse proxy, `noauth` for no sign-in |
| `proxyHeader` | `Remote-User` | Header that carries the username for `proxy`; missing users are created automatically |
| `adminUser` | `admin` | Admin username |
| `adminPassword` | none | Admin password, at least 8 characters, required for `password` |
| `trustProxyHeaders` | off | Trust `X-Forwarded-*` and `X-Real-IP`; turn on behind a reverse proxy |
| `disableWebDav` | off | Turn off the WebDAV interface built into File Browser |
| `locale` | empty | Default interface language for new users, such as `en` or `zh-cn`; empty means English |
| `tokenHours` | 2 | Session lifetime in hours |

- The admin password is synced to File Browser on every start, so the value in Gately wins; other users, share links and personal settings are managed in File Browser and stored in the instance's own database.
- The config file, database and cache live in `filebrowser/<instance key>/` under the config directory, with the config file at 0600; the admin password reaches the process through an environment variable and is never written to a file. Deleting an instance deletes this directory, and the files in the shared directories stay as they are.
- An instance whose shared directory does not exist does not start, and its card shows why.
- For access from outside, add a reverse proxy site in the [Web Service](/en/docs/modules/web) pointing to `http://127.0.0.1:port`, protect it with HTTPS and access control, and turn on "Trust proxy headers". The `proxy` method needs the Web Service forward auth to put the username into the header, and the File Browser port must stay closed to the outside, otherwise the header can be forged to sign in.
- Files are read and written as the Gately process.

## Storage Mounts

Add mounts under "Storage Mounts" in the "File Services" group, and manage files online on the page.

| Field | Description |
| --- | --- |
| `name`, `remark`, `enable` | Name, remark and enable switch |
| `type` | `local` (local directory), `webdav` (remote WebDAV) or `smb` (SMB share) |
| `path` | Local directory, must be an existing absolute path (used by `local`) |
| `url`, `username`, `password` | Remote address and account; the password is masked in the UI (used by `webdav` and `smb`). A WebDAV address starts with `http://` or `https://`; an SMB address looks like `smb://host[:port]/share[/subdir]`, SMB2/3 is supported and an empty username connects as guest |
| `skipVerify` | Skip HTTPS certificate verification (used by `webdav`) |
| `domain` | Domain or workgroup for SMB authentication, usually empty (used by `smb`) |
| `readOnly` | Mount read-only; the file manager rejects every write operation |

The online file manager supports browsing directories, downloading files, streaming uploads (the whole file is not loaded into memory), creating directories, renaming and deleting. A mount can serve as the target location of a backup task: backup files are written to the specified directory in the mount, and old files are pruned according to the retention policy.

## Wake-on-LAN

Manage devices under "Wake-on-LAN", and send magic packets to them (6 bytes of `0xFF` followed by the MAC repeated 16 times).

| Field | Default | Description |
| --- | --- | --- |
| `name` | None | Device name, required |
| `macs` | None | MAC addresses, one per line, at least one; you can enter several for one device |
| `broadcastIps` | Empty | IPv4 broadcast addresses. When empty, packets are sent to `255.255.255.255` and the broadcast address of each enabled network interface |
| `port` | 9 | Target UDP port |
| `repeat` | 3 | Number of times each MAC is sent, up to 20 |
| `checkAddr` | Empty | Online check address in the form `host:port`, for example `192.168.1.10:3389`; leave empty to skip the check |
| `remark` | Empty | Remark |

Click Wake in the device list to send the magic packet, and the page shows whether the device is online according to `checkAddr`.

## Scheduled Tasks

Run commands or send HTTP requests on a cron schedule.

- `spec` supports standard 5-field expressions, an optional seconds field (6 fields, such as `*/30 * * * * *`), and descriptors such as `@every 10m` and `@daily`;
- When `type` is `command`, `command` runs through `sh -c` (a temporary `.bat` on Windows), and the environment gets `taskName` plus an uppercase variable of the same name with the `GATELY_` prefix;
- When `type` is `http`, a request is sent according to the `http` config, with the fields `url`, `method` (GET, POST, PUT, PATCH, DELETE), `headers` (each as `Key: Value`), `body`, `successContent` (the response body must contain any one of its entries to count as success; when empty, only a 2xx status is checked), `timeout`, `insecureSkipVerify` and `proxy` (leave empty for no proxy, `update` for the global update proxy, or a `http://`, `https://` or `socks5://` proxy address);
- Tasks can be enabled or disabled, the page shows the next run time, and a task can be run immediately by hand;
- If the previous run of the same task has not finished, the new trigger is skipped;
- Each task keeps its latest 20 run records, including start time, duration, success or failure, whether it was triggered manually, and the output (a summary of up to 4000 characters);
- The creation form validates and parses the expression.

## Overview and System Logs

- The "Overview" page shows host information: CPU model, usage and load, memory, disk, network interface addresses and live rates, public IPv4 and IPv6 (click to copy), the service status of each module, and for the Gately process its version, commit, build time, start time, PID, CPU, resident memory, Go heap and goroutine count;
- The "System Logs" page collects the logs of all modules, including the runtime records of modules such as Docker, FTP and Scheduled Tasks.

## Themes

The UI offers 15 palettes in two groups, Gately originals (Emerald, Ocean, Fresh, Sakura, Sunset, Midnight, Punk) and classics (GitHub, Nord, Dracula, Catppuccin, Tokyo Night, Gruvbox, Rosé Pine, Solarized), and every palette has a light and a dark variant. The mode can be light, dark or follow system, which picks the light or dark variant of the current palette. Any palette can take one of 10 preset accent colors or a color from the system picker, and the logo and login page gradient follow it; choosing a palette again clears the custom accent. Corners can be sharp, the palette's own or round. Theme settings are stored locally in the browser.
