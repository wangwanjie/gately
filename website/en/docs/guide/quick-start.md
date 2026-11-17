# Quick Start

Deploy, sign in and create your first rule in five minutes.

## 1. Start

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  ghcr.io/wangwanjie/gately:latest
```

To manage Docker from Gately, add `-v /var/run/docker.sock:/var/run/docker.sock` to the command. `--cap-add NET_ADMIN` lets Intrusion Prevention write bans to the host firewall. Port forwarding, STUN and web services listen on host ports, so `--network host` is recommended.

If GHCR is slow for you, use `vanjayak/gately:latest` from Docker Hub instead; it is the same image.

::: tip fnOS users
Install the native app instead: it opens from the desktop icon through the fnOS unified gateway and can sign fnOS administrators in with their fnOS session. See [fnOS native app](/en/docs/guide/install#fnos).
:::

## 2. Sign in

Open `http://<device-ip>:16680` in a browser. The username is `admin` and the default password is `gately.cn`. The default password is public: after signing in, a password change dialog opens, and you can continue only after setting a new password of at least 10 characters that is hard to guess.

On a fresh install, after the password change Gately first asks "What will you use Gately for?": pick any of "Publish home services on a domain", "Port forwarding / hole punching", "Dynamic DNS", "Mesh networking", "File sharing", "VPS entry point" or "Everything". Only the modules for the chosen scenes stay on, the others are turned off (their settings are kept), and Gately restarts once; turn them on any time under Settings → Modules or with "Add more features" at the bottom of the sidebar. "Skip" keeps everything on. Upgraded instances never see this step. The "Getting started" checklist at the top of the overview lists the next steps for the chosen scenes and ticks them off from the actual configuration; it can be collapsed or closed.

The sidebar of the management UI lists every page by group. Press Ctrl+K (⌘K on macOS) or click the search box at the top of the sidebar to open global search: it jumps to pages and their tabs by name and also finds site domains, port forwarding rules, DDNS records and certificate domains. On a phone, tap the search icon in the top bar.

## 3. Apply basic hardening

Right after signing in, go to "Settings" and:

1. Enable two-factor authentication (TOTP), or register a Passkey;
2. Set a secure entrance path, LAN-only access or admin UI HTTPS as needed;
3. Check the security score at the top of "Settings → Account & Security" and resolve the flagged items.

See the [Hardening Guide](/en/docs/guide/hardening) for more recommendations.

## 4. Add your first rule

"Quick site" on the Web Services page publishes a site in three steps: enter the domain, the backend address (with "Test", or prefilled from the Home Assistant, Jellyfin, Emby, Vaultwarden, Nextcloud, Immich, Proxmox VE, Synology DSM and fnOS templates), then choose HTTP or HTTPS. A listener is reused or created automatically; see [Web Services · Quick site](/en/docs/modules/web#quick-site).

Pick a module that fits your need:

| Need | Module |
| --- | --- |
| Your home public IP keeps changing and you want a stable domain name | [Dynamic DNS](/en/docs/modules/ddns) |
| Your ISP gives no fixed public port and you want to reach home services from outside | [STUN Traversal](/en/docs/modules/stun) |
| Forward one port to another device on your LAN | [Port Forwarding](/en/docs/modules/portforward) |
| Reach multiple LAN services through one domain with HTTPS | [SSL Certificates](/en/docs/modules/ssl) + [Web Services](/en/docs/modules/web) |
| Connect networks across locations | [Tailscale / frp](/en/docs/modules/tunnels) |

## 5. Next steps

Step-by-step tutorials for specific goals:

- [Reach a NAS without a public IPv4 address](/en/docs/tutorials/remote-access-without-public-ip): IPv6 + DDNS, STUN, frp or Cloudflare Tunnel, and how to choose;
- [Run everything on one port 443](/en/docs/tutorials/one-port-443): HTTPS, SSH and RDP sharing one port;
- [HTTPS and sign-in for Jellyfin, Immich, Home Assistant and Vaultwarden](/en/docs/tutorials/https-and-login-for-apps);
- [Wildcard certificate and DDNS in 10 minutes](/en/docs/tutorials/wildcard-cert-ddns);
- [Migrate from nginx or Nginx Proxy Manager](/en/docs/tutorials/migrate-from-nginx);
- [Replace haproxy, nginx, fail2ban and acme.sh on a cloud server](/en/docs/tutorials/replace-haproxy-nginx-fail2ban).
