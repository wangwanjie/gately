---
description: Replace haproxy, nginx, fail2ban and acme.sh on a cloud server with one Gately process. Side-by-side mapping, migration order, rollback, and the like-for-like benchmark numbers from the README.
---

# Replace haproxy, nginx, fail2ban and acme.sh on a cloud server

A small cloud server often runs this stack: haproxy routes port 443 by SNI, nginx terminates TLS and proxies, acme.sh renews certificates from cron, and fail2ban bans IPs it finds in logs. Four programs, four configurations. Gately covers the same ground with Protocol Routing, Web Services, SSL Certificates and Intrusion Prevention, in one process with one UI. This guide gives the mapping, a migration order and a way back.

## When to use this, and what you need

- A Linux cloud server running some or all of haproxy (TCP mode, SNI routing), nginx (TLS and reverse proxy), acme.sh and fail2ban.
- Root access, and the ability to change the security group in the provider's console if needed.
- Both stacks coexist during the migration, and there is a few seconds' port handover at the switch. Pick a quiet moment.
- Read [Run everything on one port 443](/en/docs/tutorials/one-port-443) first; this guide reuses its routing layout.

## What you end up with

| Before | After |
| --- | --- |
| haproxy: `mode tcp`, `req_ssl_sni`, `send-proxy-v2` | [Protocol Routing](/en/docs/modules/protoroute): TLS routes by domain, PROXY v2 |
| nginx: `server`, `proxy_pass`, `return 301`, `auth_request`, `add_header` | [Web Services](/en/docs/modules/web): listeners, sites, redirect sites, forward auth, response headers |
| acme.sh: cron, DNS API, `--reloadcmd` | [SSL Certificates](/en/docs/modules/ssl): ACME DNS-01 / HTTP-01, automatic renewal, automatic deployment |
| fail2ban: `sshd`, `nginx-botsearch` and other jails | [Intrusion Prevention](/en/docs/modules/guard): log rule templates, web path probing, TLS handshake failures, trap ports, bans written to nftables |

The resulting layout:

```text
visitor ─443─▶ Protocol Routing ─┬─ vpn.example.com ─▶ 127.0.0.1:4443
                                 └─ rest ─▶ 127.0.0.1:8443 (Web Services) ─▶ apps
```

## Performance reference

The Performance section of the Gately README compares haproxy + nginx with Gately on one machine, using the configuration of a real server: 90 domains routed by SNI, PROXY protocol v2 between the two hops and an ECDSA P-256 certificate with TLS 1.3. Each stack was limited to 2 CPU cores.

| Scenario | haproxy + nginx | Gately |
| --- | --- | --- |
| HTTPS reverse proxy, keep-alive, 1 KB responses, 64 connections | 14,767 req/s (p99 35.9 ms) | 26,434 req/s (p99 5.7 ms) |
| A new TLS connection for every request, 32 at a time | 2,557 req/s | 3,448 req/s |
| 1 MB responses, 16 connections | 1,053 MB/s | 1,585 MB/s |
| SNI passthrough of a TLS stream, 4 connections | 2,927 MB/s | 4,962 MB/s |
| SNI passthrough, new connections per second, 64 at a time | 6,334 /s | 6,266 /s (on par) |
| Memory under load | haproxy ≈ 21 MB + nginx ≈ 21 MB | ≈ 110 MB (one process with every module) |

The host was an Intel Core i5-13500T and each case ran for 10 seconds. nginx used the proxy settings of the production site, which open a new backend connection per request; a tuned nginx with upstream keep-alive narrows the gap on small responses. On a real 2 vCPU cloud host the static site served 5.5k req/s against 3.4k req/s. New TLS connections were on par (about 480-550 per second for both) with an X25519 client; with the post-quantum hybrid key exchange that Go clients and recent browsers offer, Gately completed about 13% fewer handshakes in that run.

Gately uses more memory because it is one process holding every module. The figures depend on hardware, configuration and traffic, and show only that Gately is in the same league as haproxy + nginx. Measure with your own workload. Method and details are in the [README's Performance section](https://github.com/wangwanjie/gately#performance).

## Steps

### 0. Take stock and back up

```bash
cp -a /etc/haproxy /etc/nginx /etc/fail2ban ~/.acme.sh /root/backup-$(date +%F)/ 2>/dev/null
nginx -T > /root/backup-$(date +%F)/nginx-T.txt
haproxy -c -f /etc/haproxy/haproxy.cfg
fail2ban-client status
```

Write down: which backend each SNI domain goes to in haproxy and whether `send-proxy` is used; which ports nginx listens on; which certificates acme.sh issued and which DNS provider API it uses; which fail2ban jails are enabled.

### 1. Install Gately without exposing the admin port

With Docker, use host networking and grant `NET_ADMIN` so bans reach the host firewall (see the [Quick Start](/en/docs/guide/quick-start)). To let Intrusion Prevention read host logs, mount the log directory into the container:

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  -v /var/log:/var/log/host:ro \
  ghcr.io/wangwanjie/gately:latest
```

The admin port defaults to `16680`. Change the password right after the first sign-in and set a secure entrance or a source restriction as in the [Hardening Guide](/en/docs/guide/hardening). The systemd journal (`journalctl`) is not available inside the container; for that, use the directly installed program, see [Installation](/en/docs/guide/install).

### 2. Certificates: let Gately take over from acme.sh

Pick one:

- **Issue again.** Under "SSL Certificates" add an ACME certificate with DNS-01, choose the same DNS provider acme.sh used, enter the credentials, and list `example.com` and `*.example.com`. Steps are in [Wildcard certificate and DDNS](/en/docs/tutorials/wildcard-cert-ddns). Gately renews it from then on.
- **Read the existing files first.** "Add certificate", source "Read from file", with acme.sh's `fullchain.cer` as the certificate path and the `.key` file as the private key path (with Docker, mount `~/.acme.sh` into the container). Gately checks the files regularly and reloads them; switch to Gately issuing its own later.

Once Gately has issued or read the certificate successfully, remove acme.sh's renewal job (`acme.sh --uninstall-cronjob`) so both do not renew at once.

### 3. Import the nginx sites

Run `nginx -T` on nginx, click "Import nginx" under "Web → Web Services", paste the output, preview, and import (they stay disabled). Details, and how Nginx Proxy Manager differs, are in [Migrate from nginx or Nginx Proxy Manager](/en/docs/tutorials/migrate-from-nginx).

### 4. Rebuild the haproxy routing

Match haproxy's `use_backend` and `default_backend` lines with a rule under "Network → Protocol Routing" (see "Example: replacing haproxy" in [Protocol Routing](/en/docs/modules/protoroute)):

| haproxy | Protocol Routing |
| --- | --- |
| `bind 192.0.2.10:443` | Listen address `192.0.2.10` (the server's public address), listen port `443` |
| `use_backend ocserv if { req_ssl_sni -i tz.example.com }` | One TLS route: domain `tz.example.com`, target `127.0.0.1`, port `443` |
| `server ocserv 127.0.0.1:443 send-proxy-v2` | PROXY "PROXY v2" on that route |
| `default_backend web` | "Default target" `127.0.0.1`, target port `8443`, "Default target PROXY" "PROXY v2" |

Using the specific public address as listen address lets it coexist with backends bound to `127.0.0.1:443`. When Gately's Web Services serve the websites:

1. Put `127.0.0.0/8` in "Trusted PROXY sources" under "Web Services → Global settings".
2. Change the imported 443 listener to listen address `127.0.0.1`, port `8443`, and turn on "Accept PROXY protocol".
3. Create a separate listener for port 80 with one redirect site whose domain is `*` and whose target is `https://{host}`, with "Keep path and query" ticked.

### 5. Rebuild the fail2ban rules

Open "Intrusion Prevention":

1. In the page's "Settings" turn on "Sync to system firewall", and add your own egress IP to the whitelist.
2. "From template": "SSH (log file)" reading `/var/log/auth.log` (`/var/log/host/auth.log` inside Docker) or "SSH (systemd journal)". For nginx jails, use the built-in web templates such as "Web decoy paths", "Unknown domain scans", "Web 404 probing" and "TLS abnormal handshake": they count the requests Gately handles itself, with no log reading.
3. On the "Trap ports & scan detection" tab turn on "Trap ports" and "Port-scan detection" (they need the firewall sync).
4. Disable fail2ban with `systemctl disable --now fail2ban` so two rule sets do not act on the same events.

Gately writes bans into its own `inet gately` nftables table and leaves existing rules alone.

### 6. Switch over

1. Confirm in the provider's security group that TCP 80 and 443 are open.
2. Stop haproxy and nginx (`systemctl stop haproxy nginx`) to free the ports.
3. Enable Gately's protocol routing rule and the Web Services listeners and sites.
4. Check each site (see below) and watch the run log for errors.

### Roll back

Disable Gately's routing rule and Web Services listeners, and start haproxy and nginx again:

```bash
systemctl start nginx haproxy
```

Re-enable fail2ban and the acme.sh cron job as well. Gately keeps its configuration, and its [config history](/en/docs/guide/audit) can restore the state before a change.

## Verify

```bash
# Per domain: status code and certificate
curl -sI https://example.com/ | head -1
echo | openssl s_client -connect example.com:443 -servername example.com 2>/dev/null | openssl x509 -noout -dates

# Routing: the VPN domain reaches the VPN backend
curl -sk -o /dev/null -w "%{http_code}\n" --resolve tz.example.com:443:203.0.113.10 https://tz.example.com/

# Firewall: Gately's table exists
nft list table inet gately
```

On the Protocol Routing card check that each route's and the default target's connection counts grow. On the Intrusion Prevention page check the bans and the firewall sync status. In Web Services confirm the apps receive the real IP.

## Troubleshooting

**Sites do not load after the switch and the log says the port is in use.** haproxy or nginx did not stop cleanly. Find the holder with `ss -ltnp | grep -E ':(80|443)\b'`.

**The application sees 127.0.0.1 as the source.** The default target's PROXY setting is not v2, or the Web listener lacks "Accept PROXY protocol". Both ends must agree.

**You banned yourself.** Add your egress IP to the Intrusion Prevention whitelist, and do it before turning on the firewall sync.

**Certificate renewal fails.** acme.sh and Gately renewing at the same time, or too few permissions on the DNS credentials. Look at the SSL Certificates card and the run log; see the troubleshooting section of [Wildcard certificate and DDNS](/en/docs/tutorials/wildcard-cert-ddns).

**A fail2ban jail has no counterpart.** Intrusion Prevention log rules read files, the systemd journal, command output and Docker container logs, and the rule editor lets you paste a sample log to test the regex. Turn a custom `failregex` into the rule's filter regex, with `<HOST>` standing for the source address.

## Related

- [Protocol Routing](/en/docs/modules/protoroute), [Web Services](/en/docs/modules/web), [SSL Certificates](/en/docs/modules/ssl), [Intrusion Prevention](/en/docs/modules/guard).
- [Run everything on one port 443](/en/docs/tutorials/one-port-443).
- [Migrate from nginx or Nginx Proxy Manager](/en/docs/tutorials/migrate-from-nginx).
