# Tailscale / NetBird / frp / cloudflared

These four modules integrate the official programs as managed processes: automatic restart, logs collected in the UI, and configuration done in the UI. Gately does not bundle these programs; you can install, update and roll them back online from the page.

## Online install, update and rollback {#install}

The frp, Cloudflare Tunnel, Tailscale, NetBird and [File Browser](/en/docs/modules/tools#filebrowser) pages each have an "online install" card at the bottom:

- **Download and install / Update to latest**: downloads the latest stable release from upstream, verifies it against the SHA-256 recorded upstream, checks that it runs on this machine, installs it into `bin/` under the configuration directory and then restarts the processes that use the program. Nothing is downloaded again when the latest version is already installed;
- **Check for updates**: only queries the latest stable upstream release without downloading; a "New version" tag appears next to the card title when one exists. With [automatic update checks](/en/docs/guide/install#update-check) on, Gately checks installed programs every 12 hours and publishes a `program.update` notification for a new version;
- **Install a version**: enter a version number (for example `v0.65.0` for frp, `2025.8.0` for cloudflared, `1.90.2` for Tailscale); this also works for downgrading;
- **Download proxy**: can be set per program, supports http, https and socks5, and is saved when an install starts; when empty, the update proxy from "Settings" is used;
- **Previous versions and rollback**: versions replaced by an update or rollback are kept in `bin/.versions/`, at most 3. If a new version has problems, click "Roll back" in the list to switch to an older one; the current version is kept as well, so you can switch back to it later;
- **Uninstall**: deletes the program installed in `bin/` and all previous versions; the module settings are kept. Processes using the program then restart and use a program of the same name in `PATH` if there is one; otherwise they cannot run.

| Program | Source | Installed files |
| --- | --- | --- |
| frp | GitHub fatedier/frp | `frpc`, `frps` |
| cloudflared | GitHub cloudflare/cloudflared | `cloudflared` |
| Tailscale | pkgs.tailscale.com static packages, Linux only | `tailscale`, `tailscaled` |
| NetBird | GitHub netbirdio/netbird, Linux, macOS and Windows | `netbird` |
| FileBrowser Quantum | GitHub gtsteffaniak/filebrowser | `filebrowser` |

Tailscale has no ready-to-run packages for macOS and Windows; use the official installer and enter its path in the settings. The card tells you when the current platform is unsupported.

## Restore defaults {#reset}

"Restore defaults" in the page header is separate from uninstalling: it stops all processes of the module, resets the settings (including the program path and download proxy) to their defaults and deletes the files Gately generated for it. Installed programs are kept.

| Module | What is cleared |
| --- | --- |
| frp | All frpc / frps instances, proxies and program settings, plus the config files generated under `frp/` |
| Cloudflare Tunnel | All tunnels and program settings |
| Tailscale | All instances and program settings, and the login state in the default state directories; a running Gately-managed tailscaled that uses the default state directory logs out first. Custom state directories and an existing system tailscaled are left alone |
| NetBird | All settings and the login state and keys in the default state directory; a running Gately-managed daemon logs out first, which removes this machine from the network. Custom state directories and an existing system NetBird service are left alone |
| File Browser | All instances and program settings, plus each instance's users, share links and caches; files in the shared directories stay untouched |

The old settings are kept in the [config history](/en/docs/guide/audit), so you can restore them after a mistake. To remove a module completely, uninstall the program first, then restore defaults. The API endpoints are `DELETE /api/{module}/install` (uninstall) and `POST /api/{module}/reset` (restore defaults).

## Program location

Executables are looked up in this order:

1. The path set in the module settings;
2. The `bin/` subdirectory of the configuration directory (where online installs go);
3. `PATH`.

When a path is set, the online-installed version is not used, and the card points this out. You can also put the binary into the configuration directory yourself, for example `./gately-data/bin/frpc` in Docker deployments. Tailscale needs both `tailscale` and `tailscaled`; when you use an existing system tailscaled, keep the installed `tailscale` command at the same version as the daemon.

Tokens and keys are stored with 0600 permissions. The cloudflared token is passed through an environment variable, so it does not appear in the process list.

## Tailscale

Multiple instances are supported: each instance runs its own tailscaled (separate state directory, control socket, WireGuard port and process), so one machine can join different tailnets, for example a kernel TUN instance for a remote Headscale and a userspace instance for a local one.

- A new instance's state directory defaults to `tailscale/<instance key>` under the configuration directory; you can also enter an existing state directory (for example the one an earlier tailscale container mounted) to keep the node identity without registering again;
- At most one kernel-mode instance (managed, without userspace networking) can be enabled at a time: tailscaled uses a fixed routing table and firewall mark, so two kernel instances would overwrite each other. Control sockets, state directories and fixed WireGuard ports cannot be shared by two instances either; saving or enabling reports the conflict;
- Editing, enabling or disabling an instance restarts only its own tailscaled, and log sources carry the instance name (such as `tailscaled:home`);
- When upgrading from 0.6.4 or earlier, the old single configuration is migrated to an instance named `tailscale`, with the same state directory, control socket and command line;
- The program paths (tailscaled, tailscale) and the online install are shared by all instances.

Each instance supports:

- Manages `tailscaled` (userspace networking, container-friendly) or attaches to an existing system daemon;
- Auth key or login link, with Headscale supported;
- Subnet routes and exit nodes;
- Node list and SOCKS5 proxy. The proxy has no authentication, so the UI warns when its listen address reaches beyond this machine (such as `0.0.0.0` or a LAN IP); listening on `127.0.0.1` only is recommended;
- Fixed WireGuard UDP port, making it easy to allow direct connections through a firewall;
- Extra tailscaled arguments and environment variables. If the host's iptables uses the nf_tables backend, add `TS_DEBUG_FIREWALL_MODE=nftables` to the environment variables, otherwise subnet forwarding will not work.

## NetBird

- Join a NetBird network on NetBird Cloud or on your own management server (enter its URL);
- Authenticate with a setup key or a login link. On Linux the login link uses device code login and can be opened on any device; on macOS and Windows the login usually has to be completed in a browser on the machine running Gately, so use a setup key when managing it remotely. The link is renewed automatically when it expires;
- Managed `netbird` daemon (userspace networking, container-friendly) or attach to the NetBird service already installed on the system;
- With userspace networking, other peers can reach services on this machine by default (including services listening only on 127.0.0.1; the access policies of the management server still apply), and local programs reach devices in the network through a SOCKS5 proxy (default `127.0.0.1:1080`);
- The SOCKS5 proxy has no authentication, and the UI warns when its listen address reaches beyond this machine. NetBird older than 0.75 ignores the configured address in userspace mode and always serves an unauthenticated SOCKS5 proxy on `0.0.0.0`, through which other devices can reach the network; when Gately manages the daemon and detects such a version it logs a warning. Upgrade to 0.75 or later;
- Network routes, exit nodes and access policies are configured in the NetBird dashboard; in Gately you choose whether to accept routes, act as a routing peer, accept DNS and block inbound connections;
- Peer list with direct or relayed connections, latency and network routes;
- Fixed WireGuard port, extra daemon arguments and environment variables (the advanced `NB_` options in the NetBird docs);
- With userspace networking off, kernel WireGuard is used: it needs root and a TUN device, creates the `wt0` interface and adds its own firewall rules (on OpenWrt it also allows inbound `wt0` traffic in fw4), and local services must listen on all addresses or on the NetBird address to be reachable from other peers;
- The `netbird` program is about 41 MB and uses about 60 MB of memory, and online install keeps up to 3 old versions in `bin/.versions/`. On routers with little flash, watch the free space: delete old versions in the install card, or put the program on external storage and enter its path in the settings;
- Restore defaults and `POST /api/netbird/logout` deregister this machine from the management server and remove it from the network; joining again requires authenticating again.

## frp

- **frpc client**: add as many frpc instances as you need; each connects to one frps and runs, starts, stops and restarts on its own. Proxies belong to an instance and support tcp / udp / http / https / stcp / xtcp; changing a proxy restarts only its instance;
- **Proxy access address**: frpc does not know the HTTP / HTTPS ports and subdomain host of frps, so fill them in under **Access address** on the frpc instance (one click fills them from a local frps instance; if a reverse proxy or CDN sits in front of frps, use the scheme and port visitors actually use). The domains of http / https proxies, including the full name built from a subdomain, then open on click and their full URLs can be copied; without it they stay plain text. The access address only shapes the links, and changing it does not restart frpc. Custom domains are stored lowercase and in punycode like in Web Services, and values with a port or path are rejected; a subdomain must be a single label such as `nas`, as frps requires;
- **frps server**: add several frps instances on different ports, each with a port allowlist, subdomains and a dashboard;
- **Upgrading from older versions**: the single frpc / frps settings of 0.5.9 and earlier are migrated automatically to instances named `frpc` / `frps`; existing proxies belong to that frpc instance, and the program paths move to the "Programs" card at the bottom of the page;
- **frps listen addresses**: "Bind address" sets where the frpc connection port listens, and "Proxy bind address" sets where the HTTP / HTTPS ports and the remote ports of tcp / udp proxies listen; leave either empty to listen on all addresses. To put frps behind a reverse proxy such as Gately, set the bind address to `127.0.0.1`, reverse proxy a site in Web Service to `127.0.0.1:<bind port>`, and set the frpc transport protocol to websocket or wss pointing at that site; leave the proxy bind address empty so the remote ports of tcp proxies stay reachable from outside. When importing a frps config without `proxyBindAddr`, it follows `bindAddr`, just like frps does;
- **frps source IP access control**: each frps instance can use an allowlist or a blocklist to decide by source IP whether frpc may log in (connect to the bind port). The list combines IPs / subnets typed in by hand with subscribed IP lists from Web Service; matching any of them counts as being on the list. Subscribed lists are downloaded on schedule by the Web Service module and take effect as soon as they refresh, with frps kept running; until the first download succeeds a list counts as empty. The check runs before frps verifies the token: Gately opens a check endpoint on the loopback address and appends `[[httpPlugins]]` (`ops = ["Login"]`) to the generated frps config, so it applies to frpc logins only, while the remote ports of tcp / udp proxies and the HTTP / HTTPS ports stay open as before. Intrusion Prevention bans take priority: a banned address cannot log in even inside an allowlisted subnet, and instances without access control also honour the ban list, whether or not firewall sync is on. A rejected frpc gets an error such as `gately: client address ... is not in the allowlist`, and the instance card shows how many logins were rejected since Gately started. The list is checked when frpc logs in and covers frpc connections to the frps bind port: after the list is tightened or an address is banned, an frpc that is already logged in stays connected and is rejected at its next login, so restart that frps instance when the change must apply at once; stcp / xtcp visitor connections do not go through the login check, so use the firewall sync of [Intrusion Prevention](/en/docs/modules/guard) to restrict them. When frps sits behind a reverse proxy (bind address `127.0.0.1`), the source frps sees is the proxy's address, so do the IP access control on the Web Service site instead; behind Protocol Routing or Port Forwarding, frps sees a local address and the list cannot tell visitors apart, so set source IP access control on the routing or forwarding rule;
- **Import existing config**: "Import config" in the page header supports legacy `frpc.ini` / `frps.ini` (including the `[common]` section) and frp 0.52+ `toml`. You can choose a file or paste the content, and import into an existing instance or a new one. Preview the parsed result and the ignored fields first, then confirm the import. When importing into an existing frpc instance you can append (proxies with the same name stay unchanged) or replace all proxies of that instance. Importing into an existing instance keeps its enabled state, and new instances are created disabled. It can also be called through the API at `POST /api/frp/import`. When migrating from Lucky, every frpc / frps instance in Lucky is imported as a Gately instance.

## Cloudflare Tunnel

- Token tunnels and temporary trycloudflare tunnels;
- Multiple tunnels with automatic restart;
- Shows the public address.

### Routes {#cloudflared-ingress}

The public hostnames and origin services (routes) of a token tunnel are stored on Cloudflare, and running cloudflared fetches them automatically. Fill in the optional **API token** in the tunnel settings and the Routes button on the card lets you edit them directly:

- The token needs the Cloudflare Tunnel: Edit permission, plus DNS: Edit on the zones to create DNS records automatically. The account and tunnel ID are read from the tunnel token.
- Each rule has a hostname, an optional path regex and a service (such as `http://localhost:8080`, `ssh://localhost:22` or `http_status:404`). Rules are matched in order and the first match wins. You can turn on Skip origin TLS verification and set the origin Host header; other origin settings are kept. The catch-all service handles requests that match no rule and defaults to `http_status:404`.
- Changes apply as soon as you save, with no tunnel restart. If the routes were changed in the Cloudflare dashboard or elsewhere in the meantime, saving says so and shows Reload, so their changes are kept. HTTP services cannot have a path: a lone trailing `/` (such as `http://nas:5000/`) is removed on save, since cloudflared would reject the whole config.
- When the tunnel is managed by a local config file (the dialog notes that remote routes do not apply), saving asks for confirmation once more: after the remote config is written, the tunnel is managed remotely and the routes in the local config file stop applying.
- With Sync DNS on, each hostname gets a proxied CNAME record pointing to `<tunnel ID>.cfargotunnel.com`; hostnames that already have other records, or a CNAME to the tunnel that is not proxied (traffic then does not enter the tunnel), keep them and are listed in the result. Zones are looked up only in the tunnel's account. Deleting a rule keeps its DNS record; remove it in Cloudflare if needed.

## How to choose

| Scenario | Recommendation |
| --- | --- |
| Build a private network across multiple devices | Tailscale |
| A private network where both the client and the management server are open source and self-hostable | NetBird |
| You own a public server and want to map internal services out through it | frp |
| No public IP and no server, and you want to expose a web service quickly | Cloudflare Tunnel |
| Your ISP provides a NAT that allows hole punching | [STUN Traversal](/en/docs/modules/stun) |
