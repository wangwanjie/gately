# STUN Traversal

When your ISP does not provide a fixed public port, STUN traversal uses NAT hole punching to obtain a public mapped address and port, so that services on your LAN can be reached from the internet.

## Capabilities

- TCP / UDP traversal with keep-alive;
- UPnP / NAT-PMP creates port mappings on the router automatically;
- Built-in forwarding sends the public port obtained through traversal directly to a LAN target;
- Webhooks and scripts fire when the public address or port changes, which you can use to update DNS, notify clients, and more.

## Using it with other modules

The public port obtained through STUN usually changes. Combine it with:

- [Dynamic DNS](/en/docs/modules/ddns) or a Webhook / script to sync the new address to your DNS records;
- [Web Services](/en/docs/modules/web) to provide an HTTPS entry point.
- Redirect sites in Web Services can reference the rule's current public address with variables such as `{STUN_RuleName_PORT}` and follow the new port automatically when it changes; see [Redirect variables](/en/docs/modules/web#redirect-vars).

## Limitations and multi-layer NAT

- **Carrier-grade NAT (CGNAT)**: when the router's WAN gets an internal address such as `100.64.0.0/10`, traversal depends on the type of the carrier's NAT. Full cone (any outside address can reach the mapped port) works; port-restricted or symmetric NAT does not, in which case use [Tailscale, frp or Cloudflare Tunnel](/en/docs/modules/tunnels) instead.
- **UPnP behind multiple NATs**: with UPnP / NAT-PMP on, Gately maps the tunnel port to this host on the router. If the router's external address differs from the public address STUN reports, there is another NAT outside it and the public port belongs to that outer NAT, so Gately keeps only the tunnel port mapping and no longer adds a mapping for the public port.
- **Fix the local port and open it in the firewall**: an empty local port is picked at random on every start. If the host runs a firewall (for example the fnOS system firewall), give the rule a fixed local port and allow it in the firewall (you can reserve a port range for STUN rules); otherwise the firewall drops inbound connections and the traversal address cannot be reached.
- **TCP and the router's cone NAT**: on some routers "NAT1 / full cone" only applies to UDP and TCP source ports are rewritten on the way out. TCP rules need UPnP enabled, or a port mapping of the tunnel port to this host on the router.

## Deployment tip

STUN needs to listen on host ports directly. For Docker deployments, `--network host` is recommended.
