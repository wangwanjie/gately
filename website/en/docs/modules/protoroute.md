# Protocol Routing

Several services share one public port (for example a website, a VPN, SSH and Remote Desktop all on 443). Gately reads the first data the client sends to identify the protocol, then looks at the domain (SNI) and ALPN in the TLS handshake or at the HTTP Host, and passes the whole connection through to the matching backend. Nothing is decrypted or terminated here, so TLS certificates stay on the backends. It does the job of haproxy's `req_ssl_sni` rules plus sslh.

Routing by domain (SNI routing) is one case of it: add a TLS route and fill in its domains.

Each rule has:

- Listening: the protocol (TCP only; tcp is dual-stack, tcp4 / tcp6 a single address family), the listen address and ports (`443`, `443,8443` or a range);
- Routes: each route has a protocol, a name, target addresses (several addresses are tried in turn, with failover), an optional target port (empty means the listening port) and a PROXY protocol setting (off, v1 or v2). The PROXY setting belongs to the route because some backends require it and others reject it;
- A default target: the rule's own target addresses, target ports and PROXY protocol setting, used when no route matches or the protocol is not recognized;
- A detection timeout: how many seconds to wait for recognizable data from the client; 0 means the default of 5 seconds.

## Recognized protocols

| Route protocol | Recognized by | Further matching |
| --- | --- | --- |
| TLS | The TLS handshake record (ClientHello) | Domain (SNI), ALPN |
| HTTP | The method and the `HTTP/` version in the request line, including CONNECT, WebDAV methods and HTTP/2 with prior knowledge (h2c) | Domain (Host header) |
| SSH | The client version string `SSH-` | — |
| RDP (Remote Desktop) | The TPKT header and X.224 connection request | — |
| SOCKS4 / SOCKS5 | SOCKS4 and SOCKS4a requests and SOCKS5 method negotiation | — |
| OpenVPN (TCP) | The client hard reset packet, including tls-auth, tls-crypt and tls-crypt-v2 | — |
| XMPP | An XML declaration or `<stream:stream` | — |
| MQTT | The CONNECT packet (3.1, 3.1.1, 5.0) | — |
| PostgreSQL | The startup message, SSLRequest and GSSENCRequest | — |
| Minecraft (Java Edition) | The handshake packet; the server list ping of 1.6 and earlier | Server address |
| RTSP | Request line version `RTSP/1.0` or `RTSP/2.0` (IP cameras, NVRs) | — |
| SIP | Request line version `SIP/2.0` (TCP) | — |
| Redis | Commands sent as RESP arrays | — |
| AMQP | The `AMQP` protocol header (0-9-1 and 1.0, RabbitMQ and others) | — |
| SMB | SMB1, SMB2 and SMB3 negotiate requests in a NetBIOS session message | — |
| Git | The first pkt-line of the `git://` protocol (git daemon) | — |
| IRC | The CAP, PASS, NICK or USER command a client sends first | — |
| ADB | The CNXN message of the Android Debug Bridge (adb connect) | — |
| BitTorrent | An unencrypted peer handshake | — |
| Wait timeout | The client sends nothing within the detection timeout | — |

## Matching

TLS, HTTP and Minecraft routes match by domain (Minecraft uses the server address the client typed, found in the handshake; data appended by Forge, BungeeCord or TCPShield is ignored): exact domains (`vpn.example.com`) first, then the longest wildcard (`*.example.com`, any depth), then `*` (any connection that carries a domain), and finally routes without domains (every connection of that protocol, including those without SNI or Host). Domains are case-insensitive.

TLS routes can also list ALPN values: the route matches only when the client offers one of them. Under the same domain, routes with ALPN take precedence, and when the ALPN does not match the search moves on to broader domains. For example, send `acme-tls/1` to the program that answers TLS-ALPN-01 certificate validation, or `postgresql` to PostgreSQL 17 direct TLS connections. Within a protocol, a domain (with a given ALPN) can belong to one route only; every other protocol allows one route.

"Wait timeout" is for protocols where the server sends first, such as VNC, MySQL, FTP, SMTP, POP3 and IMAP: their clients connect and wait for the server, so Gately receives nothing and cannot tell which one it is. Each port can therefore send only one of them to the Wait timeout route, and those connections are forwarded once the detection timeout expires. The detection timeout accepts one decimal place: TLS, SSH, HTTP and similar clients send their first packet within milliseconds of the handshake, so with a Wait timeout route 0.3–1 seconds is enough, and that is all VNC and similar connections wait.

These connections go to the default target: no route matches; the data belongs to none of the protocols above (for example protocols without a signature such as Shadowsocks, VMess or encrypted BitTorrent, and protocols whose first packet is only a length field and easily mistaken for others, such as MongoDB or DNS over TCP); the ClientHello cannot be parsed; or the client sends nothing and there is no Wait timeout route. Without a default target these connections are closed and counted as "dropped" on the rule card. A client that disconnects before detection finishes is dropped.

## Notes

- TCP only. UDP traffic such as HTTP/3 (QUIC) or WireGuard is not routed; use a separate rule for it;
- The encrypted variants of SIP, Redis, AMQP, IRC and similar protocols run over TLS; route them with TLS routes by domain or ALPN;
- The ClientHello may be split across several TLS records and may be up to 64 KB; HTTP, RTSP and SIP request lines may be up to 8 KB, and up to 32 KB of HTTP request headers are read to find the Host;
- Each connection is detected once, so later requests on the same HTTP keep-alive connection go to the backend chosen first;
- The rule card shows how many connections each route and the default target have received; debug entries in the run log record the detected protocol, SNI, ALPN or Host of each connection;
- Changes to routes, the default target and the detection timeout take effect immediately without stopping the listener, and open connections are not affected.

## Example: replacing haproxy

An haproxy setup like this:

```text
frontend https
    bind 192.0.2.10:443
    mode tcp
    tcp-request inspect-delay 5s
    tcp-request content accept if { req_ssl_hello_type 1 }
    use_backend ocserv if { req_ssl_sni -i tz.example.com }
    default_backend web
backend ocserv
    server ocserv 127.0.0.1:443 send-proxy-v2
backend web
    server web 127.0.0.1:8443 send-proxy-v2
```

becomes one Gately rule:

| Setting | Value |
| --- | --- |
| Protocol | TCP |
| Listen address / port | `192.0.2.10` / `443` |
| Route `ocserv` | protocol TLS, domain `tz.example.com`, target `127.0.0.1`, port `443`, PROXY v2 |
| Default target | `127.0.0.1`, target port `8443`, PROXY v2 |

Set the listen address to the server's public address so that it does not clash with backends that listen on `127.0.0.1:443`. To serve websites with Gately itself, add a [Web Services](/en/docs/modules/web) listener on `127.0.0.1:8443` with "Accept PROXY protocol" on and `127.0.0.0/8` in the trusted PROXY sources. Stop haproxy before enabling the rule, because only one program can listen on the port.

## Example: replacing sslh

These sslh arguments:

```text
sslh --listen 0.0.0.0:443 --timeout 2 \
     --ssh 127.0.0.1:22 --openvpn 127.0.0.1:1194 --tls 127.0.0.1:8443
```

map to one Gately rule:

| Setting | Value |
| --- | --- |
| Protocol | TCP |
| Listen address / port | empty / `443` |
| Detection timeout | `2` |
| Route `ssh` | protocol SSH, target `127.0.0.1`, port `22` |
| Route `vpn` | protocol OpenVPN, target `127.0.0.1`, port `1194` |
| Route `web` | protocol TLS, no domains, target `127.0.0.1`, port `8443` |
| Route `vnc` (optional) | protocol Wait timeout, target `127.0.0.1`, port `5900` |

sslh sends connections to SSH after the timeout by default. Gately recognizes SSH right away from its `SSH-` version string, so the timeout can go to a server-first service such as VNC instead. You can keep adding RDP, SOCKS and HTTP routes, or split TLS between backends by domain.

## Upgrading from port forwarding

Protocol routing used to be the "SNI routing" mode of port forwarding rules. On the first start after upgrading, SNI routing rules in port forwarding move to the Protocol Routing module automatically: each rule keeps its key, name, enabled state, group and all routes, and the old routes become TLS routes. If the port forwarding module was turned off, Protocol Routing is turned off too, and API tokens with port forwarding access also get the same level of access to Protocol Routing.

After the move, manage these rules through `/api/protoroute/rules`: the route field is renamed from `sniRoutes` to `routes` and the `mode` field is no longer used; routing rules submitted to `/api/portforward/rules` are rejected. The metrics are now `gately_protoroute_*`; see [Monitoring](/en/docs/reference/monitoring).

## Deployment tip

Protocol routing listens on host ports directly; for Docker, `--network host` is recommended.

## Groups and sorting

Rules can have a "Group". Grouping, sorting and search work as in [Port Forwarding](/en/docs/modules/portforward#groups-and-sorting); the search box filters by name, group, listen port, target, domain or route protocol.
