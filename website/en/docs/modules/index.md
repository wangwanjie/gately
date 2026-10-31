# Modules Overview

Every Gately capability is an independent module. Each can be enabled, disabled and configured on its own, and each saves its configuration as a plain-text JSON file.

You can turn off a whole module under "Settings → Modules". A disabled module still loads its configuration but starts no services, serves no admin API (its endpoints return 403) and is hidden from the UI; its configuration is kept and comes back when you turn it on again. Modules that others depend on cannot be turned off, and changes take effect after a restart.

| Module | Capabilities | Docs |
| --- | --- | --- |
| Dynamic DNS | 11 DNS providers, IP from URL / network interface / command, IPv4 / IPv6, Webhook | [View](/en/docs/modules/ddns) |
| STUN Traversal | TCP / UDP, keep-alive, UPnP / NAT-PMP, built-in forwarding, Webhook and script triggers on change | [View](/en/docs/modules/stun) |
| Port Forwarding | TCP / UDP, IPv4 / IPv6, port ranges, multi-target failover, connection limits, traffic statistics | [View](/en/docs/modules/portforward) |
| Protocol Routing | Several services share one port: TLS by SNI and ALPN, HTTP by Host, Minecraft by server address, plus SSH, RDP, SOCKS, OpenVPN, RTSP, SIP, XMPP, MQTT, AMQP, PostgreSQL, Redis, SMB, Git, IRC, ADB, BitTorrent, and a wait-timeout route for VNC and similar | [View](/en/docs/modules/protoroute) |
| SSL Certificates | Automatic ACME DNS-01 issuance and renewal, upload, mapping to a directory with scripts | [View](/en/docs/modules/ssl) |
| Web Services | Reverse proxy, redirects, static files, HTTPS, authentication, access control, load balancing and health checks, caching and rate limiting, access statistics | [View](/en/docs/modules/web) |
| OIDC single sign-on | Offers the shared users to Gitea, OpenList, Grafana and other apps over OpenID Connect, replacing Authelia's OIDC provider | [View](/en/docs/modules/oidc) |
| WAF Firewall | Coraza + OWASP CRS v4 | [View](/en/docs/modules/waf) |
| Intrusion Prevention | Log jails (fail2ban-style), trap ports and port-scan detection, with bans synced to nftables | [View](/en/docs/modules/guard) |
| Tailscale / frp / cloudflared | Networking and NAT traversal with managed official binaries | [View](/en/docs/modules/tunnels) |
| Docker | Container creation and editing, logs and terminal, Compose, image updates and upgrades, networks and volumes | [View](/en/docs/modules/docker) |
| Web Terminal | SSH, Telnet, local shell, SFTP file manager, persistent sessions | [View](/en/docs/modules/terminal) |
| Network Diagnostics | Ping, traceroute, port probe, DNS lookup, speed test, LAN scan | [View](/en/docs/modules/nettools) |
| Notification Center | Multi-channel event push and rules | [View](/en/docs/modules/notify) |
| Configuration Backup | Encrypted backups, retention policy, scheduled backup and restore | [View](/en/docs/modules/backup) |
| File and Ops Tools | WebDAV, FTP, SMB, File Browser, storage mounts, Wake-on-LAN, scheduled tasks | [View](/en/docs/modules/tools) |
