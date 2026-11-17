---
description: How to reach a NAS from outside without a public IPv4 address. Four routes compared (IPv6 with DDNS, STUN, frp on your own VPS, Cloudflare Tunnel), with prerequisites, trade-offs and the Gately setup steps for each.
---

# Reach a NAS without a public IPv4 address

Your broadband sits behind carrier-grade NAT, or the ISP hands out only a private IPv4 address, and you cannot reach your NAS, photo library or smart-home hub from outside. Four ways out are common: IPv6 with DDNS, STUN traversal, frp on your own VPS, and Cloudflare Tunnel. Their prerequisites and costs differ. Work out where you stand, pick one, and follow its steps.

## First, find out where you stand

Open the "Self-check" on Gately's Overview page and look at the NAT line:

- "This machine holds the public IPv6 address …": the IPv6 route is available.
- "A carrier-grade NAT address (…) was found on a network interface" (the `100.64.0.0/10` range) or "Behind NAT: this machine only has a private address": there is no public IPv4, so you need IPv6, STUN or a tunnel.
- If the router's WAN address is itself the public IPv4 address (it matches the public address the lookup reports), plain port forwarding works and you do not need this guide.

## Choosing among the four routes

**IPv6 + DDNS**

- Needs: public IPv6 on your broadband and a router that allows inbound traffic; IPv6 on the visitor's side too.
- Upside: free and direct, limited only by your line, any TCP/UDP port.
- Cost: visitors without IPv6 (some offices, hotels, mobile networks) cannot connect; some ISPs block common inbound ports.

**STUN traversal**

- Needs: an ISP or upstream router with NAT1 (full-cone); for TCP, router cooperation.
- Upside: no server needed, direct.
- Cost: the public port is assigned by the ISP and changes, so the address carries a port; symmetric or port-restricted NAT cannot be traversed at all.

**frp on your own VPS**

- Needs: a server with a public IP and a domain.
- Upside: most reliable, any protocol, fixed ports, you control all the traffic.
- Cost: you run and maintain a VPS, and traffic uses its bandwidth.

**Cloudflare Tunnel**

- Needs: a domain hosted at Cloudflare, and web-type services only.
- Upside: no server or public IP needed, free.
- Cost: traffic passes through Cloudflare; limits on upload size and content type; SSH and other non-web services need a client on the visitor's side.

A reasonable order: if every device you connect from has IPv6, use IPv6. If you have or are willing to rent a VPS, use frp. If you only publish web services and do not want to run a server, use Cloudflare Tunnel. If it is only you and your NAT is NAT1, STUN is the quickest. When only you and your family need access, a mesh such as Tailscale, NetBird or EasyTier keeps every port closed to the internet; see [Tunnels](/en/docs/modules/tunnels).

All four routes share the HTTPS part: domain, certificate and site setup are covered in [Wildcard certificate and DDNS](/en/docs/tutorials/wildcard-cert-ddns).

## Route A: IPv6 + DDNS

### Prerequisites

- Your broadband has a public IPv6 prefix and the NAS has a global unicast address (starting with `2` or `3`).
- The router's IPv6 firewall can allow inbound connections to the NAS.
- The device you connect from has IPv6. A phone with Wi-Fi turned off is a handy test.

### Steps

1. Open "Network → Dynamic DNS", click "Add task", choose the DNS provider and enter the credentials (steps 1 and 2 of [Wildcard certificate and DDNS](/en/docs/tutorials/wildcard-cert-ddns)).
2. On the "IP source" tab turn off "Get IPv4" and turn on "Get IPv6". When the NAS itself holds the public IPv6 address, set "Method" to "Network interface" and pick the interface; use "Filter regex" to choose a particular address if there are several. The default "Query URL" also works. Click "Test fetch" to confirm it returns the NAS's address.
3. On the "DNS records" tab add one record: Host `nas`, Root domain `example.com`, type `AAAA`, value `{ipv6Addr}`. Save, then click "Force sync".
4. Under "Web → Web Services" add a listener with Network "Dual-stack" or "IPv6 only", port `443` and HTTPS on, then add the site `nas.example.com` with your application as the backend. An HTTPS site needs a certificate that covers the domain.
5. Allow the inbound connections:
   - On the router: add an IPv6 firewall rule allowing TCP 443 (or your chosen port) to the NAS's address. The ISP-delegated prefix can change, so the address can be matched by its suffix.
   - Gately on OpenWrt or on a Linux host with a firewall: turn on "Open firewall port" on the listener and Gately opens it in the local firewall.
   - Writing the rule by hand is covered in [Web Services · Manual firewall](/en/docs/modules/web#manual-firewall).

### Verify

```bash
# Resolves to the NAS's IPv6 address
dig +short AAAA nas.example.com

# From a network that has IPv6 (phone hotspot or another IPv6 machine)
curl -6 -I https://nas.example.com/
```

### Troubleshooting

- **The port is unreachable.** First confirm the service itself works on the LAN with `curl -6 http://[NAS IPv6]:port`, then check the router's IPv6 firewall, and finally whether the ISP blocks inbound 80/443. Move to a high port such as `8443` if it does.
- **DNS did not follow a prefix change.** Gately checks every 300 seconds by default; shorten the check interval in the task or click "Force sync".
- **The record holds an address that is not the NAS's.** A temporary IPv6 address or the router's address was picked up. Use the "Network interface" method with a "Filter regex" to select the right address.

## Route B: STUN traversal

STUN punches through NAT to obtain a public address and port, and outside connections reach the NAS through Gately's built-in forwarder.

### Prerequisites

- The ISP or upstream router uses NAT1 (full-cone). Port-restricted or symmetric NAT cannot be traversed; use route A, C or D.
- For TCP, some routers' "NAT1" applies to UDP only. Turn on UPnP, or map the channel port to the device running Gately on the router.
- If the host has a firewall, fix the channel local port and allow it.

### Steps

1. Open "Network → STUN Traversal" and click "Add rule".
2. On the "Basic" tab enter: Rule name `nas`, Traversal type `TCP`, a fixed "Channel local port" such as `50443` (`0` is random and changes on every start), the NAS's LAN address as "Target address" and the service port as "Target port".
3. On a router or a Linux host with a firewall, turn on "Open firewall port". System firewalls that do not use nftables or iptables, such as fnOS's, need the port opened in the system settings by hand.
4. Save. The "Public address" on the rule card shows the IP and port obtained; click it to copy.
5. For a domain name: in a DDNS task's "DNS records" add an A record with the value `{STUN_nas_IP}`, and a TXT record `port={STUN_nas_PORT}` to keep the port. A task that references them syncs immediately when the address or port changes. See [record value variables](/en/docs/modules/ddns#record-vars).
6. The public port changes, so the address is `https://nas.example.com:public-port`. To avoid typing the port, put a "Redirect" site on an entry that is always reachable (for example the IPv6 site from route A) with the target `https://{host}:{STUN_nas_PORT}`; see [redirect variables in Web Services](/en/docs/modules/web#redirect-vars).

### Verify

From outside (phone data), open the public address shown on the card. By default STUN's reachability check connects to its own public address, and the card shows an error when that fails.

### Troubleshooting

- **A public address appears but outside access fails.** Most often the host firewall drops inbound connections. Fix the channel local port and allow it, or use the router's UPnP.
- **The port changes after a few days.** The ISP reassigned the mapping. The STUN variables in DDNS records update automatically, but the address you type has to be fetched again, so this is a poor fit for family members who are not technical.
- **UPnP mappings look wrong behind double NAT.** The router's external address differs from the public address STUN obtained, so another NAT sits outside. Details in [STUN Traversal · Limits and multi-layer NAT](/en/docs/modules/stun).

## Route C: frp on your own VPS

Gately at home is the frp client and connects to frps on the VPS, which exposes the NAS's port there. Gately on the VPS then serves it under your domain with HTTPS.

```text
visitor ─HTTPS─▶ Gately on the VPS (certificate, login, WAF)
                   └─ 127.0.0.1:15000 ◀─ frps ◀─ frpc (home) ─▶ NAS:5000
```

### Prerequisites

- A VPS with a public IPv4 address, running Gately, and a domain pointing at it.
- Gately at home (it only needs to run frpc).

### Steps

**1. On the VPS: the frps server**

1. Open "Networking & Tunnels → frp" and add an frps instance on the "Server (frps)" tab. If the card says the program is not installed, click "Download and install" in the "frp online install" card.
2. Fill in: "Bind port" `7000`; "Token" a random string (strongly recommended); "Proxy bind address" `127.0.0.1`, so remote ports requested by frpc are visible only on the VPS itself and Web Services publishes them; "Allowed remote ports" can be limited to `15000-15100`.
3. Turn on "Open firewall port", or allow TCP 7000 in your cloud security group. Save and enable.

**2. At home: the frpc client**

1. On the "Client (frpc)" tab click "Add frpc instance". Enter the VPS's IP or domain as "Server address", `7000` as "Server port", the same "Token", then save and enable.
2. Click "Add proxy": type `tcp`, name `nas`, "Local address" the NAS's LAN address, "Local port" the service port (say `5000`), "Remote port" `15000`.
3. The instance card shows it connected.

**3. On the VPS: domain and HTTPS**

1. Under "Web → SSL Certificates" issue a certificate for `nas.example.com` ([Wildcard certificate and DDNS](/en/docs/tutorials/wildcard-cert-ddns)).
2. Under "Web Services" add a listener (port `443`, HTTPS) and a site: domain `nas.example.com`, type "Reverse proxy", backend `http://127.0.0.1:15000`.

### Verify

```bash
curl -I https://nas.example.com/
```

The frpc card shows connected and the Web Services site's request count grows, so traffic goes through the VPS.

### Troubleshooting

- **frpc cannot connect.** The tokens differ, port 7000 is not open on the VPS, or the "Bind port" is already in use.
- **The site returns 502.** The frpc "Remote port" does not match the site's backend port, or the NAS service is down.
- **You want the NAS application to see the visitor's real IP.** The site option "Add X-Forwarded-*" is on by default on the VPS; the application has to trust that source.
- **You want a raw TCP port instead of HTTPS.** Leave frps "Proxy bind address" empty and open the remote port.

Transport protocols, KCP/QUIC and the frps source-IP allowlist are described under [frp](/en/docs/modules/tunnels#frp).

## Route D: Cloudflare Tunnel

The `cloudflared` that Gately manages dials out from your home to Cloudflare. When someone visits your domain, traffic returns to your home through the tunnel. No public IP and no open port are needed.

### Prerequisites

- The domain is hosted at Cloudflare.
- You only need web-type services (HTTP, HTTPS, WebSocket). SSH, RDP and the like need Cloudflare's client on the visitor's side and are out of scope here.
- Know the limits: traffic passes through Cloudflare; the free plan caps a single request body at 100 MB, which affects large uploads; heavy video or other non-web content is governed by Cloudflare's terms of service, so read them.

### Steps

1. In Cloudflare's Zero Trust dashboard create a tunnel (Networks → Tunnels, connector type Cloudflared) and copy the tunnel token. Menu names change as Cloudflare redesigns the dashboard; follow what you see.
2. In Gately open "Networking & Tunnels → Cloudflare Tunnel". If it says cloudflared is not installed, click "Download and install" in the online install card at the bottom.
3. Click "Add tunnel", set "Mode" to "Token tunnel", paste the "Tunnel token", then save and enable. The card shows edge connections.
4. Prepare the local service: under "Web → Web Services" add a listener with listen address `127.0.0.1`, port `8080` and HTTPS off (Cloudflare provides the certificate), then add the site `nas.example.com`, type "Reverse proxy", with the NAS as backend.
5. Set the public hostname, either way:
   - In the Cloudflare tunnel's "Public Hostname" add `nas.example.com` with the service `http://127.0.0.1:8080`.
   - Or fill "API token" in Gately's tunnel settings (it needs "Cloudflare Tunnel: Edit", plus "DNS: Edit" to create DNS records), click "Routes" on the tunnel card, add the hostname `nas.example.com` with the service `http://127.0.0.1:8080`, turn on "Sync DNS" and save.
6. To get visitors' real IPs into the access log and IP lists: under "Web Services → Global settings → Visitor real IP" put `127.0.0.1` in "Trusted proxies" and choose `CF-Connecting-IP` as "Real IP headers". cloudflared connects to Gately on this machine and the listener binds only to loopback, so no other source can forge that header.

### Verify

```bash
curl -I https://nas.example.com/
```

The response carries `server: cloudflare` and the Gately site's request count grows, which shows traffic returning through the tunnel.

### Troubleshooting

- **The tunnel card stays on "Connecting".** The network restricts UDP; set the tunnel's "Protocol" to `http2`.
- **Cloudflare returns 502 or 1033.** cloudflared is not connected, or nothing listens on the local service the public hostname points at. Check the port from step 4.
- **Large uploads fail.** They exceed Cloudflare's request body limit. Use route A or C.
- **A WebSocket application misbehaves.** Cloudflare supports WebSocket; check the application's own reverse-proxy settings, see [HTTPS and sign-in for popular apps](/en/docs/tutorials/https-and-login-for-apps).

## Related

- [STUN Traversal](/en/docs/modules/stun) and [Dynamic DNS](/en/docs/modules/ddns).
- [Tailscale / NetBird / EasyTier / frp / cloudflared](/en/docs/modules/tunnels).
- [Run everything on one port 443](/en/docs/tutorials/one-port-443).
- [HTTPS and sign-in for popular apps](/en/docs/tutorials/https-and-login-for-apps).
