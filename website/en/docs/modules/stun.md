# STUN Traversal

When your ISP does not provide a fixed public port, STUN traversal uses NAT hole punching to obtain a public mapped address and port, so that services on your LAN can be reached from the internet.

## Capabilities

- TCP / UDP traversal with keep-alive;
- UPnP / NAT-PMP creates port mappings on the router automatically;
- Built-in forwarding sends the public port obtained through traversal directly to a LAN target;
- Webhooks and scripts fire when the public address or port changes, which you can use to update DNS, notify clients, and more; the Webhook can be sent through a proxy (the global update proxy or a custom one).
- Source IP access control (allowlist / blocklist) for the built-in forwarding.

## Using it with other modules

The public port obtained through STUN usually changes. Combine it with:

- [Dynamic DNS](/en/docs/modules/ddns) or a Webhook / script to sync the new address to your DNS records; Dynamic DNS record values can also use `{STUN_rule name_PORT}` and similar variables and sync as soon as the rule's address changes, see [Record value variables](/en/docs/modules/ddns#record-vars);
- [Web Services](/en/docs/modules/web) to provide an HTTPS entry point.
- Redirect sites in Web Services can reference the rule's current public address with variables such as `{STUN_RuleName_PORT}` and follow the new port automatically when it changes; see [Redirect variables](/en/docs/modules/web#redirect-vars).

## Limitations and multi-layer NAT

- **Carrier-grade NAT (CGNAT)**: when the router's WAN gets an internal address such as `100.64.0.0/10`, traversal depends on the type of the carrier's NAT. Full cone (any outside address can reach the mapped port) works; port-restricted or symmetric NAT does not, in which case use [Tailscale, frp or Cloudflare Tunnel](/en/docs/modules/tunnels) instead.
- **UPnP behind multiple NATs**: with UPnP / NAT-PMP on, Gately maps the tunnel port to this host on the router. If the router's external address differs from the public address STUN reports, there is another NAT outside it and the public port belongs to that outer NAT, so Gately keeps only the tunnel port mapping and no longer adds a mapping for the public port.
- **Fix the local port and open it in the firewall**: an empty local port is picked at random on every start. If the host runs a firewall (for example the fnOS system firewall), give the rule a fixed local port and allow it in the firewall (you can reserve a port range for STUN rules); otherwise the firewall drops inbound connections and the traversal address cannot be reached.
- **TCP and the router's cone NAT**: on some routers "NAT1 / full cone" only applies to UDP and TCP source ports are rewritten on the way out. TCP rules need UPnP enabled, or a port mapping of the tunnel port to this host on the router.

## Open firewall port {#auto-firewall}

When a rule has "Open firewall port" turned on, Gately allows the channel port in this host's firewall (and the UPnP / NAT-PMP internal port when one is set), and removes the rule when the STUN rule stops or is deleted. Use it when Gately runs on a router such as OpenWrt, or on a Linux host with a firewall enabled. Linux only.

- nftables: an allow rule is inserted at the top of every filter chain on the input hook (for example OpenWrt fw4 and firewalld);
- iptables: an allow rule is inserted at the top of the INPUT chain of the filter table (for example OpenWrt fw3 and ufw);
- the rules carry a `gately-open:` comment; a firewall reload clears them, and Gately checks every 30 seconds and adds them back;
- kernels without nf_tables (such as Synology DSM) use iptables (legacy) instead: the rules go into a filter chain of Gately's own (starting with `GTLY_OPEN_`), and the first rule of INPUT jumps to it; when the system firewall puts its own rules in front, the jump is moved back to the top within 30 seconds. The Docker image includes iptables (legacy);
- Docker deployments need the host network and `cap_add: NET_ADMIN`; the rule card shows the result or the reason it failed;
- firewalls that do not go through nftables / iptables, such as the fnOS system firewall, still need the port allowed by hand in their settings.

## Source IP access control

With the built-in forwarding, each rule can use an allowlist or a blocklist that decides by source IP whether traffic is forwarded to the target. Lists and subscribed IP lists work the same as in [Port Forwarding](/en/docs/modules/portforward#source-ip-access-control), and Intrusion Prevention bans come first here too. The list only applies to forwarding; STUN queries and the channel check are unaffected. Editing the list does not redo the traversal, so the public port stays the same. UDP is checked per packet. Without the built-in forwarding, outside connections do not pass through Gately and the list has no effect. "Require knock" can be chosen as well: only sources that knocked successfully with fwknop are forwarded, see [Port Knocking](/en/docs/modules/knock#gately-entries). Each rule can also set an **Always-deny list** (`ipDenyList` / `ipDenySets`) that works independently of the allowlist or blocklist mode: a matching source is always rejected, checked before the allowlist and blocklist, and an address that is also on the allowlist is rejected too. It takes manual IPs or subnets and subscribed lists, for deny-first setups such as subscribing to a malicious-IP feed while only your own subnets are admitted.

## Deployment tip

STUN needs to listen on host ports directly. For Docker deployments, `--network host` is recommended.
