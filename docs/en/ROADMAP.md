# Gately Technical Design and Roadmap

## Goals

Build an open-source, auditable, community-driven network gateway tool: it covers the features home network, NAS, and server users rely on most, namely DDNS, STUN traversal, port forwarding, reverse proxy, and certificate management, and it can import existing Lucky configurations.

Design principles:

1. **Transparency**: by default it connects to no project-operated server; every outbound address is visible, editable, and can be turned off.
2. **Portability**: Lucky `.lkcf` configuration import is provided.
3. **Auditable configuration**: configuration is plain-text JSON (file mode 0600) and can be put under git directly for diffs and rollbacks.
4. **Single-file deployment**: one static binary with the frontend embedded, supporting Docker, systemd, and OpenWrt; optional modules and UI languages can be left out at build time for a smaller custom build.

## Tech Stack

| Layer | Choice | Rationale |
| --- | --- | --- |
| Backend | Go 1.26+ | Mature libraries such as lego, pion/stun, frp, cloudflared, coraza, and rclone can be reused directly; static builds cross-compile to arm/arm64/mips/riscv routers and NAS boxes |
| HTTP | Standard library `net/http` (Go 1.22 routing syntax) | Minimal dependencies, easy to audit |
| Storage | One plain-text JSON file per module, written atomically | Easy to import, back up, and git diff |
| Authentication | bcrypt passwords + HMAC-signed session tokens, login failure rate limiting, TOTP support planned | No external dependencies |
| Logging | In-memory ring buffer per module + batched JSON Lines files (size-based rotation, restored after restart) + incremental fetch endpoint | Simple log polling endpoint, incremental refresh in the frontend |
| Frontend | Vue 3 + TypeScript + Vite + Element Plus, bundled with `go:embed` | Rich component set, fast development |
| Release | GoReleaser multi-arch binaries + multi-arch Docker images | — |

## Directory Layout

```
cmd/gately/            Program entry point
internal/app/          Wiring: load config, start modules, HTTP server
internal/config/       Modular JSON config storage
internal/auth/         Admin authentication and sessions
internal/logbuf/       Per-module log ring buffer
internal/httpx/        HTTP utilities and middleware
internal/module/       Module interface and registry
internal/modules/*/    Feature modules
internal/modules/registry/  Module registration: one mod_<module>.go per optional module (build tag no_<module>), cross-module features provided by injection
tools/custombuild/     Custom builds: turn modules.json selections into build tags, languages and Docker build arguments, and package them
modules.json           Optional modules, dependencies, size estimates, language tiers, build targets and presets (read by build.gately.cn)
internal/luckyimport/  Lucky .lkcf decryption and field mapping
web/                   Frontend project; build output is embedded by web/embed.go
```

## Milestones

Ordered by the user's actual usage (18 STUN, 25 port forwarding, 27 reverse proxy, 5 certificate, and 75 DDNS records).

| Phase | Scope | Acceptance |
| --- | --- | --- |
| ✅ **M0 Skeleton** | Config storage, admin login, module framework, log buffer, admin UI shell, Dockerfile | After login, the overview and module menu are visible |
| ✅ **M1 Port forwarding** | TCP/UDP, v4/v6, multiple ports and port ranges, multiple targets, connection limits, start/stop and logs; import of Lucky port forwarding rules | Unit tests cover TCP/UDP forwarding; 25/25 rules from a real sample import successfully |
| ✅ **M2 DDNS** | IP lookup (URL/interface/command), IPv4/IPv6, Alibaba Cloud/Cloudflare/DNSPod (Tencent Cloud)/Huawei Cloud/custom callback, webhooks; import | 11 providers (Alibaba Cloud signing checked against the official examples); 2/2 tasks and 75 records from a real sample import successfully |
| ✅ **M3 STUN traversal** | Public mapping via STUN, TCP keep-alive, UPnP/NAT-PMP, built-in forwarding, webhook and script variables; import | Local end-to-end tests + real traversal on a home network; 18/18 rules from a real sample import successfully |
| ✅ **M4 Certificates** | ACME (lego, DNS-01 reusing the M2 providers), automatic renewal, export to a path, change scripts | Pebble end-to-end (wildcard + renewal); 5/5 certificates from a real sample import with no reissue needed |
| ✅ **M5 Web services** | Reverse proxy, redirect, static files, per-domain routing, automatic TLS, BasicAuth, IP filtering; import | Covers routing, 4 site types, authentication, WebSocket, HTTPS, PROXY protocol in and out; 27 sites from a real sample import successfully |
| ✅ M6 Operations | Wake-on-LAN, web terminal (SSH/local shell), scheduled tasks, admin two-step verification | 3/3 WOL and 14/14 terminal connections imported; TOTP passes the RFC 6238 test vectors |
| ✅ M7 Ecosystem (partial) | Coraza WAF, GitHub/Google/QQ/OIDC third-party login (site authentication and admin login, Lucky identity tables can be migrated), Docker management, WebDAV, FTP server, storage management (local/remote WebDAV mounts and online file management) | WAF tested with real attack payloads; Docker verified read-only in a real environment |
| ✅ M7.5 Mesh networking and tunnels | Tailscale, NetBird, frp (multiple frpc and frps instances), cloudflared, integrated by supervising the official binaries as processes | Unit tests cover config generation, process supervision, and log collection; upstream programs are not bundled |
| ✅ M8 Trust and operations | Operation audit log, config history and rollback, declarative config directory; passkey login and read-only accounts; ACME HTTP-01; HTTP/3; global Prometheus metrics and Grafana dashboard; network self-check and first-run setup; new DDNS providers Route 53, OVH, Gandi, NS1, Bunny, Namecheap, DynDNS2; new notification channels ntfy, Gotify, Pushover, Discord, Slack, ServerChan, PushPlus, and rule templates; cosign signatures, build provenance, and SBOM for release artifacts; systemd and OpenWrt service templates | WebAuthn tested end to end with a software authenticator; Route 53 signing checked against the official AWS SigV4 test vectors; HTTP/3 tested with a quic-go client |
| ✅ M8.5 Installation and upgrades | fnOS native app (.fpk, integrated with the fnOS unified gateway and fnOS login session); online upgrade for Linux binaries; OpenWrt packages (.ipk for 24.10 and earlier, .apk from 25.12) | fpk tested in an fnOS virtual machine; OpenWrt packages tested in the official 24.10 / 25.12 rootfs containers and on iStoreOS, with CI smoke tests covering install and removal |
| ✅ M8.6 Migration and operations | nginx config import (`nginx -T`, with include expansion, upstream and Authelia's `auth_request`); certificate auto deploy (push over SSH and run a command, restart Docker containers); WAF request body inspection limit, inspecting only the first part when over the limit, and per-path skipping; subscribed IP lists (refreshed on a schedule, referenced by sites and path rules); self-check adds HTTPS site certificate coverage and frp proxy conflicts; online upgrade supports four-part version numbers | nginx import verified with a real production config (77 sites, Authelia, upstreams); SSH push tested end to end against an SFTP test server; WAF verified with a 2 MB request body for rejection, first-part-only inspection and skipped paths |
| ✅ M8.7 Intrusion Prevention | fail2ban-style log jails (log files, systemd journal, command output, Docker container logs, failed admin logins, WAF blocks) with templates for common services; trap ports and port-scan detection; one ban list (merging the web service's failed-auth bans, persisted, escalating durations, manual network and permanent bans, whitelist); port forwarding, web services, STUN, WebDAV, FTP and the admin console all reject banned addresses; bans synced to nftables (a separate `inet gately` table) and dropped in the kernel | Trap ports, scan detection (IPv4/IPv6), all four log sources, persistence and rule self-healing verified in isolated network namespaces on fnOS (nftables 1.0.6) and iStoreOS 24.10 (nftables 1.1.1); permission detection and the user-space trap fallback verified in Docker containers |
| M9 To be decided | Multiple admins and fine-grained roles | Scheduled based on community demand |

## Design Trade-offs

- Online upgrade is offered only for Linux binary deployments and is triggered manually by an admin only: after download, the SHA-256 is checked against checksums.txt and the new program is test-run, and the old version is kept on replacement; Docker, fnOS, and OpenWrt packages continue to be upgraded through the image, the app center, and the package manager. Custom builds do not offer online upgrade (it would install the full edition); rebuild with the same options on build.gately.cn and upload the upgrade package.
- No remote configuration is pushed, and no device information is reported.
- Configuration is stored in plain text; sensitive fields are masked in API output.
