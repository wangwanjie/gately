# Introduction

Gately is an open-source, auditable network gateway toolkit for home networks, NAS boxes, cloud hosts and servers of all kinds. DDNS, STUN NAT traversal, port forwarding, reverse proxy, certificates, WAF and more are bundled into a single ~32 MB binary.

## Why Gately

Software that sits on the network edge and holds DNS keys and certificate private keys should be transparent. Gately is fully open source, so users and the community can review the code and configuration directly and see exactly what it does. Gately's promises:

- Source code is open under Apache-2.0 and welcomes community review
- No connections to project servers and no device reporting by default; third-party sign-in uses your own OAuth apps, with no relay
- Every outbound address (public IP lookup, STUN servers, etc.) is visible, editable and can be turned off
- Configuration is plain JSON, ready for git-based auditing and rollback

## Design principles

1. **Transparency**: no connections to project servers by default; every outbound address is visible, editable and can be turned off.
2. **Migratable**: existing configurations such as nginx sites can be imported; imported rules are disabled by default for a gradual switchover.
3. **Auditable configuration**: plain JSON (file mode 0600) that goes straight into git for diffs and rollbacks.
4. **Single-file deployment**: one static binary with the frontend embedded, supporting Docker, systemd and OpenWrt.

## Tech stack

| Layer | Choice |
| --- | --- |
| Backend | Go, statically compiled, cross-compiles to arm / arm64 / mips / riscv routers and NAS devices |
| HTTP | Standard library `net/http` |
| Storage | One plain JSON file per module, written atomically |
| Authentication | bcrypt passwords + HMAC-signed session tokens, login rate limiting, TOTP two-factor authentication |
| Frontend | Vue 3 + TypeScript + Vite + Element Plus, bundled into the binary via `go:embed` |

## Feature overview

Dynamic DNS, STUN traversal, port forwarding, protocol routing, SSL certificates, web services (reverse proxy, access control, health checks, caching and rate limiting, traffic stats, OIDC single sign-on), WAF firewall, intrusion prevention, port knocking, Tailscale, frp, Cloudflare Tunnel, Docker management, web terminal, network diagnostics, Wake-on-LAN, WebDAV, FTP, SMB, File Browser (managed FileBrowser Quantum web file manager), storage mounts, scheduled tasks, notification center and configuration backup, plus two-factor authentication, secure entrance path, API tokens, CLI and MCP. See the [Modules Overview](/en/docs/modules/) for details.

## Next steps

- Want to try it right away: [Quick Start](/en/docs/guide/quick-start)
- Migrating from existing tools: [Migration guide](/en/docs/guide/migration)
- Care about privacy and security: [Security Model & Privacy Promise](/en/docs/guide/security)
