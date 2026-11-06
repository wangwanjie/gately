# Intrusion Prevention

fail2ban-style intrusion prevention: Gately finds brute-force attempts in logs, sets traps on unused ports, counts port scans, adds malicious IPs to a ban list and syncs the list to the system firewall (nftables), which drops all of their traffic in the kernel.

## Capabilities

- **Log jails**: read log files, the systemd journal, command output (such as `logread -f` on OpenWrt) or Docker container logs, find failures with regular expressions and ban an IP that reaches the count within the time window. Templates are included for SSH, Dropbear, Nginx, Vaultwarden, Home Assistant, Proxmox VE and more, and you can paste sample lines into the editor to test the filters;
- **Gately events**: failed admin console logins and WAF blocks can also feed a jail, no regular expression needed;
- **Web path probing**: counts requests handled by the web service by listener, path and status code, without reading logs; sources that request decoy paths such as `/.env` or `/.git`, or keep scanning unknown domains, are banned;
- **Trap ports**: traps on ports this host does not use (23, 135, 1433, 3389, 5900, 6379 and others by default). Any source that connects is banned;
- **Port-scan detection**: a source that connects to closed ports on this host more often than the threshold per minute is banned;
- **One ban list**: manual bans (CIDR networks and permanent bans included) and the web service's authentication-failure bans are managed here. Manual bans cannot cover loopback addresses, since connections forwarded through this machine all show that address. Bans are persisted and restored after a restart;
- **Escalating bans**: when an address is banned again within 30 days, the duration doubles up to a maximum;
- **Whitelist**: LAN, loopback, 100.64.0.0/10 (Tailscale) and the IPv6 /64 networks of this host's interfaces are ignored by default (bans are per /64, so public IPv6 devices on the same LAN are not banned along with an offender), and you can add IPs or networks. A single whitelisted IPv6 address protects its whole /64.

## Where bans apply

| Environment | Effect of a ban | Trap ports | Port-scan detection |
| --- | --- | --- | --- |
| Linux (root or `CAP_NET_ADMIN`, nftables installed) | All traffic is dropped in the kernel, including SSH, other programs and traffic forwarded to Docker containers and LAN devices | Handled by the kernel, catches SYN-only scans | Available |
| Docker (host network + `cap_add: NET_ADMIN`) | Same as above, rules are written to the host | Same | Available |
| fnOS package, OpenWrt 22.03 and later | Same as above (runs as root, nftables is built in) | Same | Available |
| Everything else (Windows, macOS, FreeBSD, Linux without nftables or permission) | Only Gately's own services: port forwarding, web service, STUN, WebDAV, FTP, SMB, frps logins and the admin console | Served by Gately, only catches connections that complete the handshake | Not available |

The top of the page shows the firewall sync status and, when it is not available, the reason and how to fix it.

## Firewall rules

With "Sync to system firewall" on, Gately creates its own `inet gately` table in nftables and leaves the system's rules (fw4, Docker, ufw and so on) alone:

- The `prerouting` chain (priority -190, after connection tracking and before destination NAT) drops packets from banned addresses, whether they are for this host or forwarded to a container or LAN device. Replies to connections this host opens itself (DNS, package mirrors and so on) still get through even if the other end is banned. IPv6 is banned per /64;
- The `input` chain only looks at new connections to this host, for trap ports and scan detection. Whitelisted addresses are skipped;
- The whole table is deleted when sync is turned off or Gately stops. If another program removes it, Gately rewrites it within 10 seconds.

"View rules" on the page shows the complete rules currently in place.

::: tip Docker
Use `network_mode: host` and grant `NET_ADMIN` so the rules are written to the host:

```yaml
    network_mode: host
    cap_add:
      - NET_ADMIN
```

Without it, bans still apply to Gately's own services. To read host logs, mount the log directory into the container, e.g. `- /var/log:/var/log/host:ro`, and enter `/var/log/host/auth.log` in the jail. The systemd journal source is not available in a container.
:::

## Log jails

| Source | Enter | Notes |
| --- | --- | --- |
| Log file | File paths, one per line | Reads new lines from the end of the file and follows rotation (rename or truncate) |
| systemd journal | Service units such as `ssh.service` | Read through `journalctl -f` |
| Command output | One command such as `logread -f` | Restarted when it exits |
| Docker container | Container name | Read through the Docker connection of the Docker module |
| Gately event | Admin login failure / WAF block | Events carry the source IP, no filter needed |
| Web path probing | Listeners (optional) and path rules | Counts web service requests, see below |

Filters are Go regular expressions (RE2 syntax), one per line, with `<HOST>` standing for the source address, for example:

```
Failed password for (?:invalid user )?\S* ?from <HOST>
```

Lines matching an "Ignore" expression are not counted. The time window, failure count and ban duration are set per jail; the duration can be the default, custom or permanent.

## Web path probing

With the source set to "Web path probing", the jail counts requests handled by the web service directly, without reading logs:

- **Listeners**: pick one or more; leave empty for all listeners;
- **Paths**: one per line; leave empty for all paths. `*` matches any characters (including `/`), the whole path must match, case-insensitive. For example `*/.env` matches `/.env` and `/api/.env`. Lines starting with `~` are Go regular expressions. Variants such as `//.env` or `/a/../.env` also match through the normalized path;
- **Excluded paths**: same syntax; matching requests are not counted;
- **What counts**: by default only 404 and 444 (missing content, or a connection closed by a "Close connection" site), so requests for pages that exist do not count. With "Any status code" every response counts, which suits decoy paths because single-page apps return 200 for any path. "Any status code" needs path rules or "Unknown domains only", otherwise saving is rejected;
- **Access control denials**: when a site's When denied is set to Close connection, requests rejected by a ban, IP access control or the country filter count as 403 on regular sites, so legitimate visitors outside the list are not banned, and as 444 on the default site, the same as a Close connection default site; a default site that uses a reverse proxy with an allowlist for your own IPs still has unknown domain scans counted;
- **Unknown domains only**: counts only requests whose domain matches no site, that is, requests handled by the default site, or answered with 404 by the listener when there is no default site.

Three templates are included:

| Template | Condition | Notes |
| --- | --- | --- |
| Web decoy paths | `/.env`, `/.git`, `wp-login.php`, `phpinfo.php`, `phpmyadmin` and more, any status code, once | Bans for 1 day. Remove matching lines first if a site actually runs WordPress, phpMyAdmin and the like |
| Unknown domain scans | Unknown domains only, 5 times in 1 minute | Bans for 1 day. Pairs with a "Close connection" default site: a scanner is banned after a few 444s, and with firewall sync later connections are dropped |
| Web 404 probing | All paths, 10 times in 1 minute | Paths browsers request on their own, such as favicon, robots.txt and `.map` files, are excluded. 404s returned by proxied backends count too, so missing gallery thumbnails or an app polling a missing endpoint can trigger it; check the access log before enabling and raise the count or add excluded paths if needed |

"Probe ban" on a listener card of the web service opens this page with that listener selected and the decoy template applied; the rule is stored and enforced by intrusion protection. While editing a jail you can paste request paths to test the path rules.

::: warning Ban scope
Whichever listener triggers it, the ban goes into the same ban list and applies to all listeners, sites and Gately's other services; with firewall sync on, the system firewall also drops all traffic from the address.
:::

The source address is the same as in the access log and is only restored through the PROXY protocol. For sites behind a CDN that passes the real IP in a request header, the CDN node would be banned, so do not use path probing on such listeners. When Gately sits behind another reverse proxy on the same host, the source is a loopback address: with "Ignore LAN addresses" on it is never banned, and the jail has no effect.

## Port-scan detection trade-offs

Scan detection counts new connections to ports **this host is not listening on**. Listening TCP ports are refreshed every 10 seconds and treated as open; ports forwarded elsewhere by other firewall rules never pass through the `input` chain and are not counted. The default threshold is 12 per minute. A normal client retrying a closed port does so about 5 times a minute, so do not go below 10.

Trap ports that are in use are excluded automatically so legitimate visitors are not banned.

## IPv6 strict mode

IPv6 has no NAT, so every device has a public address, and once that address shows up in DNS or certificate transparency logs, scanners can connect to it directly. The "IPv6 strict mode" option of port scan detection switches IPv6 to an allowlist: IPv6 sources may only connect to open ports (listening ports plus "Extra open ports"), and the first connection to any other port gets them banned, with no threshold counting. IPv4 still uses the threshold.

- Bans apply to the whole /64, so other devices in the same /64 are banned too. Whitelisted addresses and the addresses covered by "Ignore private addresses" are not affected.
- A port that starts listening counts as open within about 10 seconds, and the mode takes effect about 15 seconds after being turned on or after a restart; until then the threshold applies. The threshold also applies when the listening ports cannot be read.
- A port that stops listening still counts as open for 30 seconds, so visitors retrying while frps reconnects or a program or listener restarts are not banned. When the FTP service has a passive port range, the whole range counts as open; with passive ports assigned at random by the system they cannot be listed in advance, IPv6 data connections would be banned as scans, and the run log says so: set a passive port range in the FTP service.
- Ports used by other programs through firewall rules must be added to "Extra open ports" first, or sources connecting to them will be banned.
- Only new TCP connections are counted. Firewall sync must be active and port scan detection must be enabled.

## Relationship with the web service

Bans from the web service's "Ban after failed authentication" go into the same ban list, so with firewall sync on they apply to the whole host rather than only to web sites. With "Ignore LAN addresses" on, LAN addresses are never banned automatically.

The web service and the WAF only learn the real client address through the PROXY protocol. For sites behind a CDN that passes the real IP in a header, failed-auth bans and the "WAF blocks" jail would ban the CDN's nodes, so do not enable them for such sites.

## Notifications

Bans raise the `Intrusion Prevention: IP banned` event, which you can subscribe to in the [Notification Center](/en/docs/modules/notify). Addresses banned by trap ports and scan detection in the same check are combined into one notification.

## Verification

Verified in isolated network namespaces on fnOS (Debian 12, nftables 1.0.6) and iStoreOS 24.10 (nftables 1.1.1): trap ports and scan detection (IPv4 and IPv6) ban in the kernel and are imported into the ban list; log files (including rotation), the systemd journal, Docker container logs and failed admin logins all trigger bans; manual network and permanent bans, unbanning, escalating durations, restoring after a restart, rewriting after the table is removed externally and cleaning up on stop all behave as expected; a Docker container without `NET_ADMIN` is reported as lacking permission and falls back to user-space trap ports.
