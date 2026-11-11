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
| `shutdownMode` | Empty | Remote shutdown method: `agent` (agent on the device), `ssh` (SSH command) or `http` (HTTP request); leave empty to disable shutdown |
| `shutdownConn` | Empty | [Web Terminal](/en/docs/modules/terminal) SSH connection used by the `ssh` method |
| `shutdownCommand` | None | Shutdown command of the `ssh` method, single line, such as `shutdown -h now` or `shutdown /s /t 0` |
| `agentToken` | Generated automatically | Agent token of the `agent` method, masked in the API |
| `shutdownHttp` | Empty | Request of the `http` method: `method`, `url`, `headers`, `body`, `insecure` |
| `bemfaTopic` | Empty | Bemfa topic; leave empty to skip voice control; must not repeat across devices |
| `remark` | Empty | Remark |

Click Wake in the device list to send the magic packet, and the page shows whether the device is online according to `checkAddr`.

### Remote shutdown

Choose a method under *Remote shutdown* on the device, and the device card gets a shutdown button; the API is `POST /api/wol/devices/{key}/shutdown`. The request body can be omitted, or use `{"action": "reboot"}` or `{"action": "sleep"}` to reboot or put the device to sleep (the agent method only). Only one action runs at a time per device, for at most 30 seconds; after a successful shutdown or sleep, the Bemfa switch state is written as off.

| Method | Suitable for | Supported actions |
| --- | --- | --- |
| Agent | Windows, Linux, macOS and OpenWrt; the device does not need to open a port or enable SSH | Shut down, reboot, sleep |
| SSH command | Hosts with an SSH service enabled and already saved in Web Terminal | Shut down |
| HTTP request | Services that offer an HTTP interface, such as Home Assistant, smart plugs, or a shutdown tool already on the device | Shut down |

#### Agent {#agent}

An agent script runs on the controlled device. It connects to Gately on its own, long-polls for instructions, and on receiving one performs the shutdown, reboot or sleep locally. The agent only accepts these three fixed actions, so Gately cannot make it run any other command; the device does not need to open any port, and Gately does not store the device's login credentials.

1. Set the device's shutdown method to *Agent* and save; Gately generates a token for the device and opens the install instructions (open them again later from *Agent* on the device card);
2. Make sure *Gately address* is an admin address the device can reach (by default the address currently in the browser; when accessing through the fnOS unified gateway, change it to the direct address), and turn on *Skip certificate verification* if Gately uses a self-signed certificate;
3. Select the device's operating system, copy the install command and run it on the device:

| System | Where to run | Installed as | Log |
| --- | --- | --- | --- |
| Windows | PowerShell run as administrator | A boot-time scheduled task "Gately Agent" under the SYSTEM account | `C:\ProgramData\GatelyAgent\agent.log` |
| Linux | Terminal (needs sudo, and curl or wget) | systemd or OpenRC service `gately-agent` | `journalctl -u gately-agent` or `/var/log/gately-agent.log` |
| macOS | Terminal (needs the administrator password) | launchd daemon `cn.gately.agent` | `/var/log/gately-agent.log` |
| OpenWrt | SSH (root), with curl or the system's own wget | procd service `gately-agent`, configuration kept across system upgrades | `logread -e gately-agent` |

The install command downloads the script from Gately (it can also be downloaded directly from the install instructions); the token is passed in the request body and an environment variable, never in the URL. Once the agent connects, the device card shows online, along with the hostname, system and source address the agent reports; devices using the agent method do not need a check address.

- The token is equivalent to the device's power switch. The configuration file is readable only by root (on Windows, SYSTEM and administrators). One token works for one device only: if another machine connects with the same token it is rejected and a message appears in the log;
- An instruction is valid for 10 seconds after it is issued: the agent confirms first, and runs it only after Gately replies. When the agent is offline an error is returned right away, and the instruction is not queued until the next time it comes online;
- After you *Regenerate* the token or switch the shutdown method to another one, the old agent is disconnected immediately and must be reinstalled with the new command;
- The agent polls every 25 seconds, and after a disconnect retries at intervals from 5 seconds up to 60 seconds;
- Uninstall: copy the uninstall command from the install instructions. On Windows it is `powershell -ExecutionPolicy Bypass -File "$env:ProgramData\GatelyAgent\gately-agent.ps1" uninstall`, on Linux and macOS `sudo sh /usr/local/bin/gately-agent uninstall`, and on OpenWrt `sh /usr/bin/gately-agent uninstall`;
- Sleep: Windows suspends to memory (a machine with sleep turned off fails, and the log says so), Linux uses `systemctl suspend`, macOS uses `pmset sleepnow`; to wake a sleeping device with Wake-on-LAN, allow wake-on-LAN in the network adapter or the system.

The agent endpoints live under `/api/wol/public/agent/`. They need no login and only accept the device token; when a security entry is set, requests carrying a valid token can still reach them, and "Local network only" and the ban list still apply.

#### SSH command

Choose an SSH connection from Web Terminal and enter a shutdown command:

- The connection must have been opened once in Web Terminal so its host key is recorded; Gately never trusts unknown hosts unattended;
- Each shutdown opens a new SSH connection and closes it afterwards, so a stale connection from before the device rebooted is never reused;
- Shutting down often drops the SSH connection before an exit code is returned; this counts as the shutdown having started;
- The form offers common commands for Linux (`shutdown -h now`), Windows (`shutdown /s /t 0`) and macOS (`sudo -n shutdown -h now`). Windows needs the OpenSSH server enabled; on Linux, non-root users can use `sudo -n shutdown -h now` with passwordless sudo for that command.

#### HTTP request

Enter the request URL (`http://` or `https://`), method, headers and body; a 2xx response status counts as success, and the response content is shown in the result. With the method set to *Auto*, a request with a body uses `POST` and otherwise `GET`; a body starting with `{` or `[` without a specified `Content-Type` is sent as JSON. Put credentials (such as `Authorization: Bearer …`) in the headers; read-only accounts see the values masked, and the address in logs contains no query parameters. When the target uses a self-signed certificate, turn on *Skip certificate verification*.

API tokens need `webterminal:write` in addition to `wol:write` to create, edit or copy devices or trigger a shutdown (the SSH method can set or run commands over an SSH connection); listing the available SSH connections needs `wol:read` and `webterminal:read`; viewing the agent token (`GET /api/wol/devices/{key}/secrets`) and regenerating it (`POST /api/wol/devices/{key}/agent-token`) need `wol:write`.

### Bemfa voice control {#bemfa}

With [Bemfa](https://cloud.bemfa.com) connected, voice assistants such as Xiao AI, Xiaodu and Google Assistant can turn devices on and off:

1. In the Bemfa console, create a topic under MQTT device cloud whose name ends with `001` (socket) or `006` (switch), for example `pc001`;
2. Enter the same topic on the device, then enter the user private key from the console under *Wake-on-LAN → Bemfa* and enable it;
3. Link your Bemfa account in Mi Home (Xiao AI), Xiaodu or Google Home and sync devices.

"Turn on" then wakes the device and "turn off" shuts it down (remote shutdown must be set up); after a successful action the state is written back to Bemfa. Gately connects to `bemfa.com:9503` with MQTT 3.1.1 over TLS and verifies the certificate; the private key is the client ID, used only on this encrypted connection and masked in the API. After a disconnect it retries from 5 seconds up to every 5 minutes; the previous state the server replays on subscribe (retained messages and anything received within 3 seconds of subscribing) is not executed, and the same command repeated within 10 seconds runs once; waking or shutting down from the UI or API also writes the state back. Only one client can be online per private key: if another program or another Gately connects with the same key, the two keep kicking each other off.

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
- When Gately manages external programs (frp, Tailscale, NetBird, Cloudflare Tunnel, File Browser), the "Overview" page also has a "Managed Processes" card listing each process with its module and instance, program name, PID, uptime, restart count, CPU and resident memory, with the totals in the card header; click a row to open its module. Only the process itself is counted, not the children it starts, and paths and launch arguments are not shown;
- The "System Logs" page collects the logs of all modules, including the runtime records of modules such as Docker, FTP and Scheduled Tasks.

## Themes

The UI offers 15 palettes in two groups, Gately originals (Emerald, Ocean, Fresh, Sakura, Sunset, Midnight, Punk) and classics (GitHub, Nord, Dracula, Catppuccin, Tokyo Night, Gruvbox, Rosé Pine, Solarized), and every palette has a light and a dark variant. The mode can be light, dark or follow system, which picks the light or dark variant of the current palette. Any palette can take one of 10 preset accent colors or a color from the system picker, and the logo and login page gradient follow it; choosing a palette again clears the custom accent. Corners can be sharp, the palette's own or round. Theme settings are stored locally in the browser.
