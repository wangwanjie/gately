# Web Services

Serve reverse proxy, redirects, static files and fixed text by domain. This is Gately's unified web entry point. Menu location: sidebar "Web → Web Services".

## Structure: listeners, sites and the default site

The configuration has three layers:

1. Listener: one listening port. Fields include network (`tcp` dual-stack, `tcp4`, `tcp6`), listen address (empty for all addresses), port, HTTPS switch, minimum TLS version (1.0 / 1.1 / 1.2 / 1.3), and whether to accept the PROXY protocol.
2. Site: a site under a listener, matched by domain. Domains can be an exact domain, `*.example.com` (matches subdomains at any depth), `api-*.example.com` (one `*` within the first label matches zero or more characters in that label and stays within it), or `*` (matches everything). Matching priority is exact domain, wildcard, then `*`. When several wildcards match, the longer rule wins; for rules of the same length, the site listed first wins. First-label wildcards also work with imports, batch editing and the `{1}` redirect variable. HTTPS sites need a certificate that covers the actual hostnames, such as `*.example.com`; certificate authorities issue wildcard certificates only in the `*.example.com` form.
3. Default site: used when no site matches. When it is not enabled, a 404 is returned. The default site can also be set to "Close connection" so requests for unknown domains get no response at all. Once configured, the default site is shown last in the site list; until then, click "Set default site" on the listener card to configure it.

Saving a change to a site or to the default site swaps the site list in place: the listener keeps running, open connections (WebSocket included) stay up, and each site keeps its statistics. If the new configuration is invalid, the previous sites keep serving. Changing the listener's own settings (network, address, port, HTTPS, minimum TLS, HTTP/3, PROXY protocol) restarts that listener.

Each exact domain in the site list is a full clickable URL that opens in a new tab, with a button next to it that copies the URL; wildcard domains (`*.example.com`, `*`) stay plain text. The URL is derived from the listener: `https` when HTTPS is on, `http` otherwise, with the listener port, leaving out default ports such as 443 and 80, for example `https://example.com:8443`. When the listener's access port is empty and an enabled Protocol Routing or Port Forwarding rule forwards to this listener, site links automatically use the rule's entry (listen) port, for example 443 for a listener on 8443 that Protocol Routing feeds from 443. The rule's target address must reach the listener: a loopback or unspecified address for a listener on all addresses, the same address for a listener on a specific address. Protocol Routing is matched per domain the same way it routes (exact domain, then the most specific wildcard, then `*`, then the default target), skipping disabled routes and routes that only accept non-browser ALPN such as `acme-tls/1`; Port Forwarding rules that forward only UDP don't count; port ranges map one to one; the first matching enabled rule wins. The listener header shows the detected port and the rule it came from, and the **Access address** field uses it as its placeholder; an access port you fill in still takes precedence. When visitors come through a router port mapping, frp or a CDN (or a rule forwards to a LAN address of this host), the public scheme or port can differ from the listener; set them in the listener's **Access address** (for example https and 443 when a CDN pulls from http on 8080). The access address only shapes the links, and changing it does not restart the listener. On save, domains are stored the way browsers send them: lowercase, IPv6 without brackets, internationalized domains in punycode (`中文.com` becomes `xn--fiq228c.com`); values that can never match, such as ones with a port, a path or `@`, are rejected. Imports from Lucky or nginx skip only the invalid domains and say so.

A site can have a "Group" (up to 64 characters; the default site cannot be grouped). Under each listener, the list shows groups as collapsible sections: groups appear in the order they first occur, ungrouped sites come after them, and the default site always comes last. The collapsed state of each group is kept in the browser. Click "Sort & batch" to enter sort mode, where you can move sites up and down, drag them within their group, move whole groups and reorder listeners. Order only affects the list: domain matching follows the priority above regardless of site order, and saving the new order swaps the site list in place without dropping connections. The search box at the top of the page filters sites by domain, name, group, backend or remark, and collapsed listener cards expand automatically while searching.

In sort mode you can also check several sites (the checkbox on a group header selects the whole group). Dragging any checked site moves all of them, and dropping onto a site or the header of another group moves them into that group. The toolbar moves the checked sites into a group in one step, or out of their group when the name is left empty; checked sites under different listeners are saved per listener.

Hover over a group header and click the pencil button to rename the group: everything in it moves to the new name and keeps its position and order. Choosing a name that is already in use merges the two groups after a confirmation.

### Moving sites {#move-sites}

A site can be moved to another listener together with all of its settings:

- When editing a site, change **Listener**; on save the changes are saved first and the site is then moved. When adding a site, it chooses which listener the site is created in.
- In sort mode, check sites (across listeners if needed) and click **Move to listener** in the toolbar, then choose the target listener and group (by default each site keeps its own group). Sites are placed at the end of the target listener or at the end of the chosen group; sites already in the target listener stay where they are.
- In sort mode, drag sites above or below a site of another listener to insert them there and join its group, or onto a group header to put them at the end of that group. While dragging, listeners without regular sites show a drop area. Dropping asks for confirmation.

Each move is a single request: the source and target listeners are each reloaded once and their site lists are replaced in place, so connections to other sites stay open. If the target listener already has a site with the same domain, the move is rejected and no listener changes; the default site belongs to its listener and cannot be moved. If a site's key is already taken in the target listener, the site gets a new key, and Performance monitoring alert rules that reference the site are updated. Sign-in sessions are isolated per listener and site, so signed-in visitors have to sign in again after a move; the redemption counts of share links move with the site, so a limited link does not become usable again; the site's statistics and live connections start over. Moving a site that requires sign-in from an HTTPS listener to a plain HTTP listener warns that passwords and sign-in cookies will be sent unencrypted.

### Bulk operations

Check sites in sort mode (across listeners if needed) and the toolbar offers the bulk operations below. Each operation is submitted in one go: it is saved only when every site passes validation, and if any site fails nothing changes and the failing site is named. Each affected listener reloads once and swaps its site list in place without dropping connections.

- **Enable / Disable**: sites already in the target state are left alone.
- **Delete**: deletes the checked sites after confirmation; the default site cannot be deleted.
- **Bulk domains**:
  - Add domains: one per line (commas also work); domains a site already has are skipped, and new domains are appended at the end;
  - Remove domains: matched by full domain; domains a site does not have are ignored, and a site cannot lose all of its domains;
  - Replace main domain: swaps a domain suffix for a new one. The old domain itself and its subdomains (including the `*.old-domain` wildcard) are replaced, so with `old.com` → `new.com`, `www.old.com` becomes `www.new.com`. Duplicates after the replacement are merged, and sites that do not match stay unchanged.

  A newly added domain is rejected if it would appear on several sites of the same listener, since only the first of those sites could ever match it; different listeners can use the same domain.
- **Bulk options**: turn Compression, Access log, CORS, Forwarded headers and Close connection when denied on or off, and set or clear the WAF. Items left at "Keep" retain each site's own setting. Settings that affect access, such as authentication, access control and backend addresses, are changed site by site.
- **Move to listener**: moves the checked sites to another listener, see [Moving sites](#move-sites).

The matching endpoint is `POST /api/webservice/sites/batch` with the body `{"sites": [{"listener": "listener key", "site": "site key"}], "action": "...", ...}`. `action` is `enable`, `disable`, `delete`, `addDomains`, `removeDomains` (with `domains`), `replaceDomain` (with `from` and `to`) or `set` (with `options`, which accepts only the option fields above). The response gives the number of sites changed and the number that needed no change. One request handles up to 500 sites and 100 domains. Listeners themselves also support the general `PUT /api/webservice/listeners-enable` and `POST /api/webservice/listeners-delete`. For `move`, `move` names the target, for example `{"listener": "target listener key"}`, optionally with `group` (the group to put the sites in; an empty string means ungrouped, omit it to keep each site's group) or `before` / `after` (a site key in the target listener to insert before or after, joining its group). `moved` in the response lists each site's key before and after the move.

## Site types

| Type | Description |
| --- | --- |
| Reverse proxy | Forwards to one or more backends. Backend addresses use the form `http(s)://host:port`, and WebSocket is supported |
| Redirect | Redirects to a full URL with status code 301 / 302 / 307 / 308 (default 302), optionally keeping the original path and query string. The URL may contain `{host}`, which is replaced by the domain the visitor used: `https://{host}` with "Keep path" sends every HTTP request to HTTPS on the same domain. Variables such as `{path}`, `{1}` and `{STUN_RuleName_PORT}` are also available; see [Redirect variables](#redirect-vars). |
| Static files | The directory must be an absolute path. With "Directory listing" off, a directory request returns a response only when `index.html` exists. "Clean URLs" tries `path.html` and `path/index.html` when a file is missing (for VitePress and similar generators, like nginx `try_files $uri $uri.html $uri/index.html`). "404 page" is a path inside the directory, such as `404.html`, returned with status 404 |
| Navigation panel | Serves a panel from the [navigation panel](#nav-panel) library: links to the services you use, grouped on one page; visitors on the LAN get LAN addresses automatically |
| Fixed text | Custom content, `Content-Type` (default `text/plain; charset=utf-8`) and status code (100-599, default 200) |
| Close connection | Sends nothing and closes the connection, like nginx `return 444`. Typically used as the default site so requests scanning unknown domains get no response; the access log records them as 444; it skips authentication, access control, the WAF and rewrites, so the other site settings have no effect |

Sites of every type can set an access log, cross-origin (CORS), custom request and response headers (one `Key: Value` per line; an empty Value removes that header), and can reference a [WAF Firewall](/en/docs/modules/waf) instance (if the instance does not exist or is disabled, requests pass through). See [Request and response headers](#headers) for how request headers, response headers and Content-Type are edited.

Access log entries record the client IP, method, domain and path, status code, duration and site name, followed by the Referer and User-Agent in nginx format (`-` when missing), so different devices behind the same public IP can be told apart by User-Agent. The log can be filtered by site, status code, client IP and more; see [viewing and filtering runtime logs](/en/docs/guide/configuration#log-filter).

### Redirect variables {#redirect-vars}

The target of a redirect site can use the following variables:

| Variable | Meaning |
| --- | --- |
| `{host}` | The domain the visitor requested, without the port; an IPv6 address keeps its brackets, e.g. `[2001:db8::1]` |
| `{port}` | The port the request came in on |
| `{hostAndPort}` | Domain and port, the same as the request's Host |
| `{path}` / `{PATH}` | Request path; `{PATH}` has no leading `/` |
| `{args}` / `{Args}` | Query string; `{args}` includes the leading `?` (empty when there is no query string), `{Args}` does not |
| `{domainPartN}` / `{pathPartN}` | The Nth part of the domain (split by `.`) or path (split by `/`), counting from 1 |
| `{ClientIP}` / `{RemoteIP}` | Client IP |
| `{time}` | Current time in the format `2006-01-02 15:04:05`, with the space written as `%20` (`2006-01-02%2015:04:05`) so the redirect address stays a valid URL |
| `{1}` | The part matched by `*` in a site domain such as `*.example.com`; empty when an exact domain matched, the full domain when `*` or the default site matched |
| `{STUN_RuleName_PORT}` / `{STUN_RuleName_IP}` / `{STUN_RuleName_ADDR}` | The current public port, IP and `IP:port` of a [STUN Traversal](/en/docs/modules/stun) rule; the rule name is the name shown in the list and may contain underscores |

- With "Keep path" on, a target containing `{path}`, `{PATH}` or `{pathPartN}` no longer gets the original path appended, and a target containing `{args}`, `{Args}` or its own query string no longer gets the original query string appended; the other part is still appended;
- When the STUN port changes, the site needs no edit: the next request goes to the new port. If the referenced rule does not exist, is disabled, has not traversed yet or is retrying, the site returns 503 and the runtime log records why;
- If the expanded target has no host (for example `{1}` is empty because an exact domain matched, but the target is just `https://{1}`), the site returns 503; if the visitor's host contains characters that never appear in a domain, such as `&`, `=`, `/` or `@`, the site returns 400 and the host is not inserted into the target; when the target uses `{host}`, `{hostAndPort}`, `{1}` or `{domainPartN}`, requests without a Host header (HTTP/1.0 probes and the like) also get 400;
- Site domains support only one `*`, so only `{1}` exists;
- Redirect targets in path rewrite rules still support only `{host}`; unknown `{xxx}` is kept as is.

Example: Cloudflare CDN sends `*.example.com` to port 2053 on this machine, and that listener has a redirect site for `*.example.com` with the target `https://{1}.v4.example.com:{STUN_web_PORT}{path}{args}`. Visiting `zly.example.com/a?b=1` redirects to `https://zly.v4.example.com:<STUN port>/a?b=1`; the STUN rule web forwards to another listener, which has reverse proxy sites for the domains under `*.v4.example.com`.

### Reverse proxy options {#proxy-options}

These are the "Options" on the "Basic" tab of the site editor. The question mark icon next to them shows a short explanation in the interface and links to this section.

| Option | What it does | When to turn it on |
| --- | --- | --- |
| Skip backend certificate verification | Does not verify the certificate when connecting to an HTTPS backend | The backend uses a self-signed or expired certificate, such as the https port of a NAS or router admin page. The backend identity can no longer be confirmed, so use it only for LAN backends |
| Use backend address as Host | Sets the Host header sent to the backend to the backend address; when off (default) the Host from the visitor request is kept | The backend only accepts its own address, e.g. it returns 400 / 403, redirects to the wrong address or reports a Host mismatch; usually needed when proxying to public websites or CDNs. Keep it off when the backend separates sites by domain or builds links with the visited domain |
| Disable connection reuse | Opens a new backend connection for every request (Keep-Alive off) | The backend sometimes resets connections, requests hang, or it does not support persistent connections. Adds some latency, so keep it off normally |
| Add X-Forwarded-* | Writes `X-Forwarded-For` / `X-Forwarded-Proto` / `X-Forwarded-Host` / `X-Forwarded-Port` and `X-Real-IP`, telling the backend the visitor's real IP, original protocol, domain and port; the visitor's source port goes in `X-Real-Port` and `X-Forwarded-For-Port` (`X-Forwarded-Port` is the port the visitor used to reach Gately) | The backend logs real IPs or must know the outside is HTTPS to build correct links (otherwise it may bounce back to http or loop after login). Recommended for most apps |
| Send PROXY v2 to the backend | Prepends a PROXY protocol v2 header to the TCP connection to pass the real client IP. When on, every request opens a new connection and HTTP/2 to the backend is not used. If the backend is Gately, turn on accepting the PROXY protocol on its listener and add this host to its trusted PROXY sources. | The backend does not read HTTP headers but supports the PROXY protocol, such as another Gately, haproxy or nginx with `proxy_protocol`. If the backend is not configured to accept it, every request fails, so leave it off if unsure |
| Use the visited domain as SNI | HTTPS backends only: sends the domain the visitor used during the handshake and verifies the certificate against it, like nginx `proxy_ssl_server_name on; proxy_ssl_name $host` | The backend selects certificates or routes by domain on one port, such as frp `vhost_https` or an nginx hosting several HTTPS sites. Every backend has its own connection pool, so connections are never reused across domains |
| Forward Gately sign-in cookies | Passes Gately's own sign-in cookies (single sign-on `gately_sso`, site session, third-party sign-in nonce) to the backend unchanged; when off (default) these cookies are not sent to the backend so sign-in credentials do not leak | The backend is another Gately that handles authentication and this site has none. For example a public Gately that only does WAF and access control and proxies to a LAN Gately, which handles web authentication and single sign-on. Not available while this site has authentication enabled; only use it when the backend is your own Gately |

Common combinations:

- Proxying a LAN web app (e.g. `http://192.168.1.10:5000`): turn on only "Add X-Forwarded-*";
- The backend is an https address with a self-signed certificate: also turn on "Skip backend certificate verification";
- Proxying a public website or third-party service: turn on "Use backend address as Host";
- Another service in front of the backend routes by SNI (e.g. frp `vhost_https`): turn on "Use the visited domain as SNI".
- Public Gately only for WAF, LAN Gately handles sign-in: no authentication on the public site, turn on "Forward Gately sign-in cookies", and for real client IPs also "Send PROXY v2 to the backend", with the LAN Gately accepting the PROXY protocol and trusting the public Gately.

### Request and response headers {#headers}

Configured on the "Headers" tab of the site editor. Request headers are sent to the backend and only apply to reverse proxy sites; response headers are returned to the client and apply to sites of every type.

- Row editor: clicking the name field lists common headers and what they are for; picking one fills in a common value, and the value field offers other common values. Both fields also accept free text. An empty value removes the header, and pasting a whole `Key: Value` line into the name field splits it automatically;
- "Add common security headers" adds `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN` and `Referrer-Policy: strict-origin-when-cross-origin`, skipping headers that already exist;
- "Text mode" lets you paste several `Key: Value` lines at once; the content is kept when switching back to the row editor;
- The extra request headers of each path in "Path backends" use the same editor and are applied after the site request headers; on a name clash the path wins.

Each row gets live feedback. Red means saving fails or the header cannot take effect, such as a missing name, a name with spaces, colons or other illegal characters, or a missing colon in text mode. Orange means it can be saved but may not do what you expect:

- `Host` set as a request header has no effect; to rewrite it, turn on "Use backend address as Host";
- A fixed `X-Forwarded-For` or `X-Real-IP` overrides the real client IP; to pass the client IP, turn on "Add X-Forwarded-*";
- `Content-Length`, `Connection`, `Transfer-Encoding` and similar headers are managed by the connection; setting them manually is ignored or breaks the transfer;
- Values are sent as is, and nginx variables such as `$host` are not supported; placeholders such as `<token>` in preset values must be replaced;
- Headers with the same name are applied in order, and the later one wins.

For "Fixed text" sites, `Content-Type` can be picked from common types or typed in. The interface checks the `type/subtype; param=value` format, warns that non-English text may be garbled when `text/*` declares no `charset`, and uses `text/plain; charset=utf-8` when empty.

### Navigation panels {#nav-panel}

The left menu item “Web → Navigation panels” (`#/webpanels`) manages the panel library. Each panel is a set of grouped links. Add a Web service site of type “Navigation panel”, choose the panel, and the site's root path (`/` or `/index.html`) shows the page; other paths return 404. The site's authentication, IP access control, country filter and WAF apply as usual. A start page is usually for yourself, so turn on web authentication or an IP allowlist. Changes to a panel take effect as soon as they are saved, without restarting the listener.

- **Links**: each link has a name, group, description, internet address and optional LAN address, icon and color, and can open in the same tab or be hidden. Groups appear in the order they first occur in the list; the editor can move a whole group up or down. “Add from Web service sites” lists sites that have a concrete address (the first non-wildcard domain with the listener's access scheme and port; listeners reached through protocol routing or port forwarding use the forwarded entry port) and adds the checked ones as links, keeping the site group as the link group.
- **Other access methods**: a service often has several entry points, such as direct IPv6, frp, STUN or a Cloudflare tunnel. Each link can have up to 8 more access methods (names up to 20 characters; addresses and variables as above). They appear as small links on the card, open in a new tab and are shown to LAN and internet visitors alike; one whose variable has no value yet is shown struck through as unavailable. When the main address is unavailable the other access methods can still be clicked, and search also matches their names.
- **Addresses**: `http`, `https`, `ftp`, `ftps`, `sftp`, `ssh`, `smb`, `afp`, `nfs`, `vnc`, `rdp` and `mailto` are supported; passwords are not allowed. Addresses can use `{host}`, `{port}`, `{hostAndPort}` (the domain and port the visitor used to open the panel), `{1}`, `{domainPartN}` and `{STUN_rule_PORT}`, `{STUN_rule_IP}`, `{STUN_rule_ADDR}`, written as in [redirect variables](#redirect-vars), for example `https://{host}:8443` or `http://{STUN_web_ADDR}/`. While a STUN rule has no public address yet, the link shows as unavailable. Variables taken from the request path and query, such as `{path}` and `{args}`, can't be used: otherwise anyone could craft a panel URL with a query string that changes where a link points. While `{1}` has no value (the panel site's domain is not a wildcard, or in the preview), links that use it show as unavailable. When the panel is opened by an IPv6 address, `{host}` expands with brackets, for example `[2001:db8::1]`.
- **LAN and internet**: when the visitor's address is loopback, private (10/8, 172.16/12, 192.168/16, fc00::/7) or link-local, or in the panel's “Extra LAN addresses” (for example `100.64.0.0/10` for Tailscale), links use their LAN address; links without one keep the internet address. Behind a CDN or reverse proxy, configure [trusted proxies](#real-ip), or the visitor address is the proxy's. When some link has a LAN address, visitors can switch between LAN and internet in the top-right corner; the choice is kept in a cookie, and “Auto” restores detection.
- **LAN detection and visibility**: LAN or internet is decided by the visitor's direct address. When internet visitors come in through frpc, cloudflared or a Docker port mapping on this machine, the direct address is this machine or a container network, so they are treated as LAN visitors. For such setups, have the forwarder send a PROXY header and enable the PROXY protocol on the listener, or configure [trusted proxies](#real-ip) when an HTTP proxy is in front. LAN addresses are visible to every visitor who can open the panel, and visitors can switch to the LAN view with `?net=lan`; to keep LAN addresses private, enable authentication or IP access control on the site.
- **Icons**: an emoji or text of up to 8 characters, an `http(s)` image URL, or an uploaded image (PNG, JPEG, GIF and WebP are scaled to 128 pixels; SVG and ICO are kept as they are; up to 96 KB each and 3 MB per panel). Leave it empty to show the first letter of the name. Text icons can have a background color; when empty one is picked from the name. You can also click Choose to use a built-in icon: Gately modules and related tools such as frp, Tailscale, NetBird and Cloudflare, common apps such as Jellyfin, Home Assistant and qBittorrent, systems such as OpenWrt, Synology and Proxmox, and general icons such as router and server (brand icons from Simple Icons, general icons from Lucide), stored as `builtin:id`. Built-in icons are drawn on a tile in the icon's own color; you can change the color, and the glyph turns dark on light colors.
- **Page**: title (the panel name when empty), subtitle, page icon and theme (system, light, dark). The page has a search box: press `/` to focus it, separate keywords with spaces; names, descriptions, addresses and groups are matched, Enter opens the first result and `Esc` clears. The editor's Preview renders the page for an internet or LAN visitor, with `{host}` expanded to the admin UI's domain.
- **Theme**: the same color schemes as the admin UI's theme picker (Gately originals and classic palettes), Emerald by default; the editor sets the panel's default. Visitors can switch the theme with the palette button at the top right: light, dark or system mode, a color scheme (previewed on hover, and spreading out from the click when chosen), and a custom accent color and corner radius; on phones the picker opens as a bottom sheet. The choice is saved in the visitor's browser, the panel setting applies until they choose, and the choice is shared with the admin UI when both are on the same domain. The editor's preview uses the panel setting and has no theme button.
- **Security**: the page is generated on the server with all names and descriptions escaped. Only the built-in styles and script carrying a random nonce run (`Content-Security-Policy`), other sites can't embed it, and icon images load with `no-referrer`. The page depends on the visitor address, so responses carry `Cache-Control: no-store`.

An instance can have up to 50 panels with up to 500 links each; a panel still used by a site can't be deleted. Read-only accounts and tokens can view panels. API: `GET|POST /api/webservice/panels`, `PUT|DELETE /api/webservice/panels/{key}`, `PUT /api/webservice/panels/{key}/enable` and `POST /api/webservice/panels/preview`.

## Transport and protocols

- HTTPS: uses a certificate from the [SSL Certificates](/en/docs/modules/ssl) module that covers the domain;
- Plain HTTP on the same port: a listener with HTTPS can also accept plain HTTP, telling the protocols apart by the first byte of each connection. *Redirect to HTTPS* sends `http://host:port` to `https://host:port` (301 for GET/HEAD, 308 for other methods), useful when only one port is available and visitors may type the wrong scheme; *Serve* answers over unencrypted HTTP. ACME HTTP-01 challenges are still answered, and changing this option restarts the listener;
- HTTP port: a listener can also take a plain HTTP port (for example 80 next to an HTTPS listener on 443) that shares all sites and settings with the main port, so HTTP needs no separate listener with the same sites filled in twice. With HTTPS on, choose *Redirect to HTTPS* (default) or *Serve*: Redirect keeps the visitor's domain, path and query and switches the port to the one in *Access address* (the listen port when empty, omitted for 443), using 301 for GET/HEAD and 308 for other methods; *Serve* answers over HTTP, handy when you switch between `http://` and `https://`. Domains without a matching site are not redirected: with no default site they get the unknown-domain 404, and a default site set to "close connection" closes it, the same as over HTTPS (this also applies to the same-port HTTP redirect). ACME HTTP-01 challenges are still answered, and the PROXY protocol and automatic firewall opening apply to this port too. If the HTTP port is taken, the main port keeps running and the listener status shows why;
- HTTP/3 (QUIC): turn it on for a listener with HTTPS enabled. HTTP/3 is then also served over UDP on the same port number and advertised to browsers with the `Alt-Svc` response header; allow that UDP port in your firewall and port mapping;
- WebSocket: supported automatically by reverse proxy;
- PROXY protocol: a listener can accept PROXY headers sent by upstream components (such as haproxy and frp). It trusts only the IPs or subnets listed in "Global settings → Trusted PROXY sources"; leaving it empty trusts no source. Reverse proxy can send PROXY headers to the backend. `X-Forwarded-Proto: https` on connections from a trusted source is honored too: session cookies get Secure, ports that redirect plain HTTP no longer redirect, and with "Add X-Forwarded-*" on, https is passed on to the backend.

### Visitor real IP (behind a CDN / WAF) {#real-ip}

When a site sits behind an HTTP proxy such as Cloudflare, Alibaba Cloud CDN or Tencent Cloud CDN, connections come from the proxy's origin-pull addresses. Fill in the trusted proxies (the proxy's origin-pull IPs or ranges, several providers allowed) under **Global settings → Visitor real IP** and choose the real IP headers. Gately then takes the visitor address from the request headers and uses it for access logs, IP allow/block lists, LAN-only access, country filtering, auth failure bans, statistics, path probing, and the `X-Real-IP` / `X-Forwarded-For` sent to backends.

- Headers are read only when the original peer is a trusted proxy. The original peer is the address after PROXY protocol handling: the address in the PROXY header for connections from a trusted PROXY source, otherwise the TCP peer. Headers sent by clients connecting directly are always ignored;
- Headers are checked in the chosen order and the first one that yields a valid address wins; `X-Forwarded-For` is used when none is chosen. Multi-value headers such as `X-Forwarded-For` are read right to left, skipping trusted proxies, and the first untrusted address is used, so values forged on the left by the client are never taken;
- Pick only headers your proxy overwrites. Cloudflare overwrites `CF-Connecting-IP`, for example, while other proxies usually pass through a client-supplied header of the same name; `X-Forwarded-For` is the safer choice there;
- With Cloudflare, set the real IP header to `CF-Connecting-IP`: Cloudflare writes the visitor address into it directly. `X-Forwarded-For` is read right to left skipping trusted proxies, so when the trusted proxies include Cloudflare's whole ranges, a request sent through Cloudflare's own egress (such as Workers) can carry a forged address on the left;
- `0.0.0.0/0` and `::/0` are rejected on save;
- When Protocol Routing hands connections to Web Services over loopback, enable sending the PROXY header in Protocol Routing and accepting it on the listener instead of adding `127.0.0.1` to the trusted proxies; otherwise visitors connecting straight to Protocol Routing could forge the headers. Passthrough routes in Protocol Routing remain plain TCP forwarding and are not affected;
- Requests from a trusted proxy carrying `X-Forwarded-Proto: https` are treated as HTTPS;
- Visitor source port: when the visitor address comes from a header, the source port comes only from the header configured in "Real port header" (for example `X-Real-Port` written by the proxy with `proxy_set_header X-Real-Port $remote_port`; the rightmost item if there are several). If it is not configured or missing, the port is unknown: sites with "Add X-Forwarded-*" send no `X-Real-Port` / `X-Forwarded-For-Port` to the backend, and live connections show only the address. Only use a header your proxy overwrites, otherwise visitors can forge the port;
- With no trusted proxies the behavior is unchanged. Connection-level bans (firewall rules written by Intrusion Prevention) match the TCP source and have no effect on visitors behind a CDN; Web Services' own bans apply to the real address.

## Authentication

Configure it on the "Security" page of the site editor:

- Web authentication: the browser shows a login page;
- Basic authentication: HTTP Basic;
- Users: one `username:password` per line, and the password can be a bcrypt hash. Basic authentication needs at least one user when enabled; web authentication needs at least one user or third-party login identity when enabled;
- Third-party login: GitHub, Google, QQ and OIDC are supported, using your own OAuth apps. Allowed identities go one per line in the form `provider:account`, for example `github:octocat` and `google:me@gmail.com`. A `#` at the end of a line adds a comment, and `github:*` means any account of that provider.

Third-party login providers are configured in "Global settings": enter your own Client ID / Secret and enable the provider, and also set a callback address (it must be served by one of Gately's listeners, for example `https://auth.example.com`). The callback URL to register with GitHub / Google / QQ is:

```text
https://auth.example.com/__gately_auth/oauth/callback
```

Third-party login for the management UI itself is configured separately under "Settings → Account & Security".

### Forward auth (Authelia and similar)

Hand authentication to an external service, like nginx `auth_request` or Traefik `forwardAuth`. Tick "Forward auth" on the "Security" page. It cannot be combined with Basic or web authentication on the same site.

| Field | Description |
| --- | --- |
| `url` | The check endpoint, such as Authelia's `http://127.0.0.1:9091/api/authz/auth-request` |
| `loginUrl` | Where to send visitors when the endpoint answers 401, such as `https://auth.example.com/?rd={url}&rm={method}`. `{url}` is the original address (URL-encoded) and `{method}` the request method. When empty, the endpoint's response is returned as it is |
| `loginFromLocation` | When `loginUrl` is empty and the endpoint answers 401 with a single absolute http(s) `Location`, redirect the visitor there with 302 and pass `Set-Cookie` and `Cache-Control` along. Meant for services that put a dynamic login address in the 401, such as Casdoor's native `/api/forward-auth`; any other `Location` is ignored and the 401 is returned as is. Off by default |
| `copyHeaders` | Identity headers copied from the endpoint's reply to the backend request; the default is `Remote-User`, `Remote-Groups`, `Remote-Email` and `Remote-Name` |

How it works:

- Every request first goes to `url` as a GET without a body, carrying the visitor's headers (cookies, `Authorization` and so on) plus `X-Original-Method`, `X-Original-URL`, `X-Forwarded-Method`, `X-Forwarded-Proto`, `X-Forwarded-Host`, `X-Forwarded-Uri`, `X-Forwarded-For` and `X-Real-Ip`. The timeout is 10 seconds;
- A 2xx answer lets the request through, with the identity headers added; a 401 redirects to `loginUrl` (status 302); other answers such as 403 or a redirect are returned to the visitor; an unreachable service gives 502;
- Identity headers sent by visitors are always removed, so they cannot be forged;
- It works with path rules and "Authentication scope", so only some paths (for example `/admin`) need a login, and with temporary share links.

### Shared users: TOTP and single sign-on {#shared-users}

Define accounts once and use them on several sites, with optional TOTP two-step verification. It covers what many people run Authelia for.

On the "Web → Auth users" page in the sidebar (the Web service global settings dialog only shows a summary and a link):

- Add users with a username and a password. Passwords are stored as bcrypt hashes and never returned by the API. To add TOTP, click "Enable TOTP", scan the QR code with an authenticator app (Google Authenticator, 1Password and so on) and save. Regenerating the key invalidates the old one;
- Optionally fill in an email, display name and groups; [OIDC single sign-on](/en/docs/modules/oidc) passes them to applications as the `email`, `name` and `groups` claims, and an OIDC client's allowed users can grant access by group with `group:name`;
- SSO domain, such as `example.com`: after signing in on any site under that domain or its subdomains, the other sites there need no further sign-in. It uses a parent-domain cookie (`gately_sso`, valid for 7 days) that is never forwarded to backends. Leave it empty to sign in on each site separately.

On a site, tick "Use shared users" next to web or Basic authentication. The site's own users keep working as well. You can also select sites in bulk in the site list on the Auth users page, grouped by listener and filterable: selecting a site without authentication also turns on web auth; clearing a site that authenticates only with shared users marks it "Auth off after saving", and Save lists such sites for confirmation; once confirmed, their authentication is turned off and they no longer require signing in; sites using forward auth cannot use shared users. Changes to users, OIDC and sites on the page take effect together when you click Save at the bottom; if any of them fails validation nothing is saved, and leaving the page with unsaved changes asks for confirmation first.

Behavior:

- The login page shows a verification code field when any shared user has TOTP. Each code works once;
- Changing a user's password or TOTP key ends that user's sessions;
- Basic authentication accepts only shared users without TOTP, because a browser password prompt has nowhere to enter a code;
- Failed attempts count towards the same rate limit and automatic ban as other sign-ins.

To let Gitea, OpenList, Grafana and other applications know which user signed in, offer the shared users to them through [OIDC single sign-on](/en/docs/modules/oidc).

### Self-service password change {#self-service-password}

Turn on Self-service password change on the Auth users page and shared users can change their own password without asking an administrator:

- Visit `https://site-domain/__gately_auth/password` on a site with Use shared users enabled; the login page also shows a Change password link. Users of [OIDC single sign-on](/en/docs/modules/oidc) can use `/__gately_oidc/password` on the issuer domain;
- Enter the username, the current password and the new password twice (8–128 characters, different from the current one); users with TOTP also enter a verification code;
- Afterwards the user's sessions on all sites end immediately and they sign in again with the new password; issued OIDC refresh tokens are revoked and old Access Tokens can no longer read user info, while tokens that apps validate on their own stay valid until they expire, and the apps' own sign-in sessions are unaffected;
- After a successful change, the same account cannot change its password again for 1 minute, so repeated changes do not keep FTP and SMB reloading and dropping other people's connections; when a service check rejects the save, the details go only to the web service log and the page asks the user to contact the administrator;
- A wrong current password or code counts toward the same rate limit and automatic ban as failed logins, and forms submitted from other websites are rejected; the web service log records the user and source IP;
- The same page can also change the password of [shared file users](/en/docs/modules/tools#shared-users). Each kind of account has its own switch; with both on, the page lets the user pick the account type. Accounts of the two kinds with the same name are independent.

## Access control

### IP allowlist and blocklist

"IP access control" on the site's "Security" page can be off, allowlist or blocklist. The list takes one IP or subnet per line (such as `192.168.1.0/24`), and can also reference subscribed IP lists.

### When denied

**When denied** on the site's Security tab decides how requests rejected by a ban, IP access control (including path rules) or the country filter are handled: **Return 403** by default; **Close connection** sends nothing and drops the request, logged as 444, the same as a Close connection site. Combined with an allowlist this lets your own IPs in and drops everyone else. Rate limiting (429) and WAF blocks are not affected.

The default site can be set up the same way: set the type to **Reverse proxy** with a backend, choose an allowlist or blocklist under IP access control (subscribed IP lists below can be used), and set **When denied** to **Close connection**. Requests that match the allowlist, or miss the blocklist, are proxied to the backend and the rest are dropped; a refreshed subscription list takes effect at once. The Close connection site type has no backend and drops every request.

Closing the connection, as a site type or as the deny action, only withholds the HTTP response and cannot hide that the service is up:

- the TCP handshake has completed, so the client knows the port is listening;
- HTTP/1.x closes the connection, while HTTP/2 and HTTP/3 only reset the request and keep the connection open for reuse;
- over HTTPS, domains without a certificate already fail during the TLS handshake, so requests that reach the default site have received the certificate and the domain names in it.

To make scanners see only timeouts afterwards, ban their addresses in [Intrusion Prevention](/en/docs/modules/guard); with firewall sync on, their traffic is dropped in the kernel.

### Subscribed IP lists

Add lists under "Global settings → Subscribed IP lists": enter the URL and refresh interval (24 hours by default). When the URL (such as GitHub) is unreachable directly, choose the global update proxy under "Download proxy" or enter a proxy address. Gately downloads each list on schedule and caches it in the configuration directory, so it works after a restart without network access. The content has one IP, CIDR or `start-end` range per line; anything after `#`, `;` or `//` is a comment. For Chinese provinces and cities you can use [metowolf/iplist](https://github.com/metowolf/iplist), for example `https://metowolf.github.io/iplist/data/cncity/440000.txt` (Guangdong).

- Allowlists and blocklists of sites and path rules can reference several lists, checked together with the manually entered ranges; matching any of them counts as being on the list;
- If downloaded content cannot be parsed (for example an error page was returned), the previous list is kept, the reason is shown on the list, and the download is retried an hour later;
- Until a list has downloaded successfully it counts as empty: an allowlist therefore denies access, and a blocklist blocks nothing extra;
- Disabling a list that is still referenced makes the allowlists using it deny every source and the blocklists using it block nothing;
- Deleting a referenced list is refused.

### Path rules

In "Path rules" on the "Security" page, override the site's authentication and IP settings by path prefix:

| Field | Values |
| --- | --- |
| `prefix` | A path prefix starting with `/`, such as `/admin` |
| `auth` | Inherit from site (empty), `off` (no authentication), `on` (authentication required) |
| `ipMode` / `ipList` | Inherit from site (empty), `whitelist` or `blacklist`, with a list of IPs or subnets |

Matching rules:

- The longest prefix wins;
- Matching follows path segment boundaries: `/admin` matches `/admin` and `/admin/x`, but not `/administrator`;
- The request path is normalized first (`.`, `..` and duplicate slashes are removed) and then matched, so `/public/../admin` is treated as `/admin`;
- Only one rule can be configured for the same prefix.

The "Authentication scope" option (`authOptional`): when ticked, authentication applies only to paths marked "Required" in the rules, and all other paths are public. This option needs Basic or web authentication to be enabled first.

The "Trust signed-in users" option (`trustAuthUsers`): when ticked, visitors authenticated by this site's Gately authentication skip the WAF and their requests are not counted by intrusion prevention's web path probing. It is meant for WAF, 404 or decoy path rules that block your own users by mistake. Only credentials issued by Gately count: web authentication and third-party login sessions, shared-user single sign-on and basic authentication (basic credentials are trusted only after they are first verified, and stop being trusted as soon as the password changes or the user is deleted); forward authentication and share links do not. For third-party login, only identities listed one by one in the allow list are trusted (matched by account or one of its aliases, such as the GitHub numeric ID or the OIDC sub); identities allowed through a wildcard such as `github:*` can still sign in and use the site, but their requests still go through the WAF and path probing, because a wildcard admits anyone who can register an account with that provider. Banned addresses, IP and country access control and rate limits still apply, and failed logins are still counted. This option needs Basic or web authentication to be enabled first; requests from signed-in users are no longer protected by the WAF, so only enable it when the users are trusted.

With two-tier Gately (an outer Gately, such as one on a VPS, only runs the WAF and reverse proxies, and an inner Gately handles sign-in), the outer instance cannot verify sessions issued by the inner one, so use the auth marker instead: enter the same "Auth marker key" in the web service global settings of both (at least 16 characters; click "Generate" and copy it to the other side), enable "Trust signed-in users" on the inner site, and tick "Forward Gately sign-in cookies" and "Trust backend auth marker" (`trustBackendAuth`, available when the site does not enable authentication itself) on the outer reverse proxy site. For trusted signed-in users the inner instance returns a signed marker in the `X-Gately-Auth` response header, bound to a hash of the credential the request used (session cookie or Authorization header), valid for 5 minutes and renewed on every response. Once the outer instance verifies the signature and confirms the marker is bound to the credential of the current request, later requests with that credential skip the outer WAF and are not counted by path probing. The first request with a credential still goes through the outer WAF (the WAF checks the request before it is forwarded to the backend). A Gately with a key configured removes the same header written by backend applications, and the marker is never returned to the visitor; an intercepted or cache-replayed marker cannot let a different credential through. Changing or clearing the key immediately ends what the outer instance has let through; after an inner session ends, the outer instance may keep skipping the WAF for that credential for up to 5 minutes, while the request itself is still rejected by the inner authentication.

### Temporary share links

After Basic or web authentication is enabled, you can create share links for a site so that visitors can access it without an account:

| Field | Description |
| --- | --- |
| Name | A label, such as "For Alice" |
| Expiry | Leave empty for no expiry |
| Uses `maxUses` | Maximum number of times the link can be redeemed; 0 means unlimited. The count resets after a restart |
| Path limit `pathPrefix` | When not empty, the link can only access paths under this prefix; it must start with `/` |

A visitor opens `https://domain/any-path?_gs=token` to redeem a session. After a successful redemption, the server sets a login-free cookie and redirects to the address without the `_gs` parameter. The session lasts at most 24 hours and never beyond the link's expiry. An invalid token is handled as a normal visit and does not reveal the reason for the failure. Sessions already issued become invalid immediately after the link is deleted. The token is generated after saving (at least 16 characters), and the full link can be copied in the UI.

### Authentication failure rate limiting and automatic ban

- After 10 failed authentications from the same source IP within 15 minutes, further attempts are rejected for a while;
- Under "Global settings → Auto ban → Ban on authentication failure", enter a number of minutes (0 to 43200); the source IP is banned when it reaches 10 failures. 0 means no ban, rate limiting only;
- An `AuthBanned` event is published when an automatic ban triggers, and it can be pushed through the [Notification Center](/en/docs/modules/notify);
- IPv6 addresses are counted and banned per /64 subnet.

### Ban list

The ban list in "Global settings" shows the currently banned IPs, the reason (repeated authentication failures or manual ban) and the expiry time, and each entry can be lifted with "Unban". You can also ban manually: enter an IP and a number of hours (1 to 720, which is at most 30 days); the API defaults to 24 hours. Loopback addresses and addresses within the trusted proxies cannot be banned manually, for the reason given under **Close and ban** below. Tick **Permanent** to ban without an expiry (seconds -1 in the API).

The ban list is shared with [Intrusion Prevention](/en/docs/modules/guard): when the Intrusion Prevention module is on, this section only shows the number of bans and links to the Intrusion Prevention page for managing them. Bans also apply to port forwarding, STUN and other services, and are dropped in the system firewall when firewall sync is on; with "Ignore LAN addresses" on, LAN addresses are never banned automatically.

### GeoIP country filter

A site's "Country filter" has three modes: off, allow only, and deny. Enter two-letter ISO country codes separated by commas, such as `CN, HK, US`. At least one country is required when enabled.

- Filtering depends on a GeoIP database, which you must first import under "Global settings → GeoIP database": upload a `.mmdb` file, or enter a download URL ("Download proxy" defaults to the global update proxy, and connects directly when no update proxy is set). MaxMind GeoLite2, DB-IP and other country or city databases are supported; the upload limit is 129 MB;
- When no database is loaded, filtering has no effect (all visitors pass) and the UI shows a notice;
- Private network addresses are always allowed;
- In "Deny" mode, addresses whose country cannot be found in the database are allowed.

## Path backends

For a reverse proxy site, send requests under a path prefix to other backends, like nginx `location /api/ { proxy_pass ...; }`. Add them under "Path backends" on the "Basic" page of the site editor.

| Field | Description |
| --- | --- |
| `prefix` | A path prefix such as `/api/`. Matching follows path segment boundaries: `/api` and `/api/` both match `/api` and `/api/x`, but not `/apix` |
| `targets` | The backends for this path, written like the site's backends. When empty, the site's backends are used and only the headers below are added |
| `requestHeaders` | Request headers added for this path, one `Key: Value` per line. They are applied after the site's headers, and a header with the same name overrides the site's |
| `stripPrefix` | When on, the path prefix is removed before forwarding: `/api/zyqd/abc` reaches the backend as `/abc`, the same as nginx `proxy_pass http://backend/;`. A path in the backend address is placed in front of the remaining path; the query string is kept. Off by default |

- The longest prefix wins. The path is forwarded unchanged by default; with `stripPrefix` on, the prefix is removed first (the path is normalized for `//`, `.` and `..` before the prefix is removed, so dot segments cannot escape the prefix);
- When importing nginx, a `proxy_pass` with a URI such as `location /api/ { proxy_pass http://backend/; }` becomes a rule with `stripPrefix` on; the path is forwarded unchanged when the URI equals the prefix or there is no URI;
- Other settings of the site, such as the Host option, certificate checks, forwarded headers, the PROXY protocol, the load-balancing policy and health checks, apply to every path backend;
- Compression, rate limiting, caching and rewrites apply to the whole site.

## Route by user {#user-routes}

For a reverse proxy site, send each signed-in user's requests to their own backends, like nginx `map $remote_user $backend`. The site needs basic auth, web auth or forward auth first. Then add rules under "Route by user" on the "Security" page of the site editor: each rule lists a set of users and the backends for those users, written like the site's backends.

Redirect, static file, navigation panel and fixed text sites can route by user too: users matching a rule are reverse proxied to that rule's backends, while users not listed and visitors who are not signed in on public paths get the site type's normal response. For example, administrators reach the admin backend while other signed-in users are redirected to a help page or see a fixed "no access" text. The backend options on the Basic tab and the load balancing, backend timeouts and health checks on the Traffic tab then apply to the per-user backends; the response cache is still only for reverse proxy sites. Close-connection sites skip authentication and cannot route by user.

Users are matched by these identifiers:

- The user name of a site user or a [shared user](#shared-users);
- For third-party sign-in, `provider-key:id`, for example `github:octocat`; the identity's other identifiers match too;
- For forward auth, the `Remote-User` copied from the auth service's response.

Routing:

- Users not listed, and visitors who are not signed in on public paths (paths that need no authentication), use the site's backends. A visitor already signed in through web auth is routed by identity on public paths too, so pages of the same app reach the same backend;
- Path backends with their own backends (see the previous section) take priority; path backends that only add headers use the user's backends;
- With the in-memory response cache enabled, the cache key includes the chosen user route, and also the identity passed to the backend when [Pass identity](#identity-headers) is set, so responses from different backends or for different users are kept apart.

Saving checks that the site has authentication enabled, that each rule has at least one user and valid backends, and that no user appears in two rules. In the site list, a site with user routes shows a tag with the number of rules.

### Pass identity {#identity-headers}

"Pass identity" puts the signed-in user's identity in request headers for the backend, for apps that identify users by header; Firefly III, for example, identifies accounts by `Remote-Email`. Each row picks a field and a header name:

| Field | Common header | Source |
|---|---|---|
| User name | `Remote-User` | The site user or shared user name; `provider-key:id` for third-party sign-in; `Remote-User` for forward auth |
| Email | `Remote-Email` | The shared user's email, or the `Remote-Email` copied back by forward auth |
| Display name | `Remote-Name` | The shared user's display name, or the `Remote-Name` copied back by forward auth |
| Groups | `Remote-Groups` | The shared user's groups (comma-separated), or the `Remote-Groups` copied back by forward auth |

- "Add common Remote-* headers" adds the rows from the table above for the fields the current sign-in method provides;
- Email, display name and groups can be chosen only on sites using [shared users](#shared-users) or forward auth; site users and third-party sign-in only have a user name. After a shared user's details change, signed-in sessions pass the new values at once; a site user with the same name as a shared user passes only the user name when signing in with the site user's password;
- Forward auth only trusts the headers it copies back: if `Remote-Email` and the like are not in the copy list, the same headers sent by visitors are never taken as identity; when the auth service returns no `Remote-User`, nothing else is passed either;
- The same headers sent by visitors are always removed, including spellings with underscores instead of hyphens (such as `Remote_Email`), so they cannot be forged; visitors who are not signed in or come through a share link get no identity at all, and a header is not sent when its value is empty;
- Saving checks that header names are valid and unique (case-insensitive, `-` and `_` count as the same), are not headers that affect forwarding or authentication such as `Host`, `Cookie`, `Authorization` or `X-Forwarded-For`, and do not clash with the site's or a path rule's request headers, which would override the sign-in result; at most 16 rows;
- "Pass user name" from earlier versions shows up as a user name row when editing the site and moves into this list on save.

## Reverse proxy enhancements

These options are on the "Traffic" page of the site editor.

### Backends, weight and backup

The backend list takes one address per line, optionally followed by parameters:

```text
http://10.0.0.2:8080 weight=3
http://10.0.0.3:8080 backup
```

- `weight=N`: weight, an integer from 1 to 100, default 1;
- `backup`: a standby backend that is used only when all non-backup backends are unavailable. When every backend is unavailable, all non-backup backends are tried again.
- When a backend cannot be reached (connection refused, unreachable or connect timeout), the same request is retried on the next backend right away, other primary backends first and then backups, trying at most 3 backends. The request has not been sent at that point, so resending POST and similar requests is safe. Failures after the connection is established are not retried, and the failed backend still counts toward passive checks.

### Load balancing policy

| UI option | `lbPolicy` | Description |
| --- | --- | --- |
| Round robin (weighted) | Empty | Default |
| Random (weighted) | `random` | |
| IP hash (sticky sessions) | `ip_hash` | The same client IP always goes to the same backend |
| Least connections | `least_conn` | Picks the smallest value of connections / weight |

### Health checks

The active check switch is off by default. Fields and defaults once enabled:

| Field | Default | Range |
| --- | --- | --- |
| Mode `mode` | `http` | `http` (requests the check path; a status code below 400 counts as healthy) or `tcp` (only tests the connection) |
| Path `path` | `/` | Starts with `/`, http mode only |
| Interval `interval` | 10 seconds | 1 to 3600 |
| Timeout `timeout` | 3 seconds | 1 to 60 |
| Consecutive failures `fails` | 3 failures mark it unavailable | 1 to 20 |
| Consecutive successes `passes` | 2 successes recover it | 1 to 20 |

When sending the PROXY protocol to the backend is enabled, HTTP checks switch to TCP automatically.

Passive checks are always in effect: among multiple backends, a backend with 3 consecutive connection failures is paused for 30 seconds before being probed again (a single backend with no active check is never removed). Once active checks are enabled, recovery is decided entirely by the check results. When a backend's state changes, upstream-down and upstream-recovered events are published, and they can be pushed through the [Notification Center](/en/docs/modules/notify).

### Backend timeouts {#backend-timeouts}

Timeouts for a reverse proxy site's connections to its backends, in seconds; 0 uses the default. Path backends and per-user backends use the same settings.

| Field | Default | Range | Meaning |
| --- | --- | --- | --- |
| Connect `timeouts.connect` | 10 s | 1 to 300 | Establishing the TCP connection to the backend |
| TLS handshake `timeouts.tlsHandshake` | 10 s | 1 to 300 | Completing the TLS handshake with an HTTPS backend |
| Response headers `timeouts.responseHeader` | No limit | 1 to 86400 | Waiting for the backend's response headers after the request, including its body, is sent |

- The response header timeout only covers the wait before headers arrive; the response body, file downloads and long-lived connections such as WebSocket and SSE are not affected after that. For large uploads the clock starts once the request body is fully sent;
- A timeout returns 504 (Gateway Timeout); other errors such as a refused connection return 502. Timeouts count toward passive health checks like connection failures;
- Raise the connect timeout for backends that start slowly or sit behind slow links; set a response header timeout when a stuck backend should fail fast.

### Visitor timeouts {#client-timeouts}

Idle timeouts toward visitors, in seconds; 0 uses the default of 60 seconds (the same as nginx's `client_body_timeout` and `send_timeout` defaults). Every site type except Close connection can set them.

| Field | Default | Range | Meaning |
| --- | --- | --- | --- |
| Request body `clientTimeouts.readIdle` | 60 s | 1 to 3600 | Longest wait for a single read of the request body |
| Response `clientTimeouts.writeIdle` | 60 s | 1 to 3600 | Longest wait for a single write of the response |

- Only a single read or write that stalls is timed, so large uploads and downloads that keep moving, time spent waiting for the backend and long-lived connections such as WebSocket are not affected;
- It drops visitors that send the request body or read the response extremely slowly (slowloris, slow read) so they cannot hold connections for long;
- A request body timeout returns 408 and closes the connection without counting against the backend's passive checks; a write timeout closes the connection;
- It applies to HTTP/1, HTTP/2 and HTTP/3.

### Compression

Compression can be enabled for reverse proxy, static file and fixed text sites (not for redirect sites). Based on `Accept-Encoding`, `br` is preferred, then `gzip`.

- Only text-like content is compressed: `text/*`, JSON, JavaScript, XML, XHTML, RSS / Atom, WebAssembly and SVG; `text/event-stream` is excluded;
- Responses whose declared `Content-Length` is below 256 bytes are left as they are;
- Responses that already have a `Content-Encoding` are left as they are; WebSocket upgrade requests pass straight through;
- After compression, strong ETags are converted to weak ETags.

### Per-IP rate limiting

Token-bucket rate limiting per client IP. Fields:

| Field | Description |
| --- | --- |
| `rate` | Requests allowed per second |
| `burst` | Burst capacity. When `rate` is set and `burst` is 0, it defaults to `rate` rounded down plus 1 |
| `maxConns` | Maximum concurrent requests per IP; 0 means unlimited |

When rate limiting is enabled, at least one of `rate` and `maxConns` must be set.

### In-memory response cache

Applies to reverse proxy sites only. The cache lives in memory and uses LRU eviction.

| Field | Default | Range |
| --- | --- | --- |
| `ttl` default duration | 0 | 0 to 30 days (in seconds). When the upstream declares no cache directive, 200 responses are cached for this long; 0 means only responses the upstream explicitly allows are cached |
| `maxMb` cache capacity | 64 | 1 to 4096 MB |

Caching rules:

- Only GET requests are cached, and the request must not contain a `Range` header or `Cache-Control: no-cache`;
- Requests with credentials are not cached: a request header among `Authorization`, `Proxy-Authorization`, `Cookie`, `X-Api-Key`, `X-Auth-Token`, `X-Access-Token`, `X-CSRF-Token`, `X-Emby-Token`, `X-Emby-Authorization`, `X-MediaBrowser-Token`, `X-Plex-Token`, `X-Jellyfin-Token`, or a query parameter `api_key`, `X-Plex-Token`, `token` or `access_token` (parameter names are case-insensitive);
- Requests whose raw path contains `;`, `\`, `%2F`, `%5C` (case-insensitive) are not cached, because such paths may mean something different to the backend after normalization;
- Only responses with status 200 are cached; a response is skipped if it contains `Set-Cookie`, `Vary: *`, or a `Cache-Control` of `no-store` / `private` / `no-cache`;
- The response's `Vary` may only list `Accept-Encoding`, plus `Origin` when CORS is enabled on the site; any other header in `Vary` keeps the response out of the cache, and so does an `Access-Control-Allow-Origin` from the backend other than `*`;
- When CORS is enabled on the site, cached entries do not keep the CORS headers Gately wrote for the visitor's origin; on a hit they are generated again from the current request's `Origin`;
- `s-maxage` is honored first, then `max-age`;
- A single entry is limited to 4 MB and also to at most a quarter of the total capacity;
- A hit returns the `X-Cache: HIT` and `Age` headers, and a miss returns `X-Cache: MISS`;
- The cache key includes Host, the full request URI and `Accept-Encoding`; with [route by user](#user-routes), it also includes the chosen user route, and the identity passed to the backend when [Pass identity](#identity-headers) is set.

Per-path caching (`rules`, "Per-path caching" in the site editor's "Traffic" tab, up to 200 rules):

| Field | Description |
| --- | --- |
| `path` | Path to match. It starts with `/`, contains no spaces, `?` or `#`, no `.` or `..` segments and no consecutive `/`, and is unique within the site. It must not contain `%` (enter the decoded path, such as `/中文/*`), and matching is case-sensitive. Without `*` it matches as a prefix on segment boundaries: `/api` matches `/api` and `/api/x`, and leaves `/apix` alone; with `*` the whole path is matched as a wildcard, and `*` matches any characters including `/`, such as `/api/products/*` or `/static/*.js` |
| `noCache` | On means "Don't cache": the path neither reads nor writes the cache |
| `ttl` | Cache duration, 0 to 30 days (in seconds); 0 uses the site's default duration |
| `override` | On means "Force cache": `no-store`, `no-cache` and `max-age` in the backend's `Cache-Control` are ignored and 200 responses are cached for the cache duration, while a backend `private` still keeps the response out of the cache; the rule or the site needs a duration greater than 0 |

- The UI offers three modes: "Cache" follows the backend's `Cache-Control` and uses the duration when the backend declares nothing; "Force cache" is `override`; "Don't cache" is `noCache`;
- The more specific rule wins: rules with more characters once `*` is removed are matched first, and ties keep the configured order; paths that match no rule use the site defaults. To cache only certain paths, set `/` to don't cache;
- Rules match the normalized request path, so `//api` and `/x/../api` are treated as `/api`;
- In every mode, the credentialed requests listed above, requests whose path contains `;` and the like, and responses with `Set-Cookie`, `Vary: *` or `Cache-Control: private` are never cached;
- Force-cached responses are shared by all visitors, so use it only for content that is the same for every visitor, such as static assets. Do not use it when the backend skips authentication by source IP (for example, no login on the LAN), or content seen by LAN visitors would be served from the cache to internet visitors.

Viewing and purging the cache: when editing a saved site, the "Traffic" tab shows the entry count and size of the site's running cache ("Current cache"), with "Refresh" and "Purge all", and each rule can purge the cache for its path. The matching API:

- `GET /api/webservice/cache?listener=&site=` returns `enabled`, `entries`, `bytes` and `maxBytes`; `site` is the site key, `default` for the default site; it returns 404 when the site does not exist or its listener is not running;
- `POST /api/webservice/cache/purge` with the body `{"listener", "site", "path"}`: an empty `path` clears the whole site cache, otherwise only entries whose path matches are cleared, with the same syntax and matching as a rule's `path`; it returns the number of removed entries `removed` and the cache state afterwards, and 400 when the site has no cache enabled.

### Path rewrite and redirect

Rules are matched in order, and matching stops at the first hit.

| Field | Description |
| --- | --- |
| `pattern` | A regular expression matched against the request path, such as `^/old/(.*)$` |
| `to` | The replacement result; use `$1` to reference groups |
| `redirect` | Off for an internal rewrite (the rewritten path is sent to the backend, and the target must start with `/`); on for a redirect |
| `code` | Redirect status code 301 / 302 / 307 / 308, default 302 |

Redirects keep the original query parameters. A redirect target may contain `{host}`, the domain the visitor used. Redirect rules run before authentication, like nginx `location = / { return 302 ...; }`, so a path such as `/` can send visitors on without asking them to sign in.

## Error pages {#error-pages}

The "Error pages" tab in the site editor controls the 404 and 5xx pages that visitors see. Gately's built-in style is used by default; you can switch to one of 10 built-in styles (Minimal, Terminal, Neon, Starry sky, Newspaper, Brutalist, Aurora, Pixel, Blueprint, Status chain) or use your own page. All styles fit phone screens, the text follows the language of the visitor's browser, and the eye button at the bottom right of the card previews different status codes at desktop and phone sizes.

- Scope: the 404 and 5xx pages Gately generates for a site, such as 502 when the backend is unreachable, 404 when a static file does not exist, 503 when a redirect target is temporarily unavailable, and the error when the forwarded authentication service is unavailable. When a static site has a "404 page", it still takes priority for missing files. The 403 from access control and the 429 from rate limiting are not affected.
- Default site: its setting also applies to the 404 shown when no site matches the requested domain, even when the default site is disabled.
- Clients other than browsers (`curl`, API calls and so on) still get a plain-text error, as before.

### Intercept backend errors {#intercept-errors}

A reverse proxy site can enable "Intercept backend errors" to replace the listed status codes returned by the backend with the page selected above, like nginx `proxy_intercept_errors on` plus `error_page`. Separate status codes with commas; ranges and shorthand are supported, for example `404, 500-599` or `5xx`, and only 400 to 599 is accepted.

- Only requests that open a page in a browser (`Accept` contains `text/html`) are replaced. API, image and script requests keep the backend's response, so a single-page app's JSON 404 is never swapped for a web page.
- The status code and headers such as `Set-Cookie` and `Retry-After` are kept; headers that describe the original body (content encoding, `ETag` and so on) are removed. `HEAD` requests return headers only.
- "Bulk options" can change the error page style of several sites at once; each site's interception setting stays as it is.

### Custom pages {#custom-error-pages}

Custom pages are kept in a page library, maintained under "Global settings → Custom error pages" or through "Manage custom pages" on a site's "Error pages" tab, and several sites can share one page. A page comes from one of two sources:

- Upload or paste: choose a `.html` / `.htm` file (UTF-8, up to 256 KB) or paste the code directly. The content is stored in the configuration and stays editable.
- Server file: enter the absolute path of a `.html` / `.htm` file (a path inside the container for Docker). The file is checked for changes on every use. A symbolic link must also point to an HTML file, and files whose content does not look like HTML are rejected. If the file is deleted or its content becomes invalid, visitors see the default style and the page is marked unavailable in the library.

A page can use placeholders, which are replaced with the details of the current error and HTML-escaped on output:

| Placeholder | Content |
| --- | --- |
| `{code}` | Status code, such as 502 |
| `{status}` | English status phrase, such as Bad Gateway |
| `{title}`, `{detail}` | Error title and description, in the visitor's language |
| `{host}`, `{path}` | The requested domain and path |
| `{time}` | Time of the error (UTC) |

Safety and usability are checked automatically before saving, and a preview is shown:

- An error page is same-origin with the site. To keep it from being used to steal cookies or forge actions, saving removes scripts, event attributes such as `onclick`, `javascript:` links, iframes, plugin objects, forms, `<base>`, `<meta http-equiv>` (automatic redirects and so on) and dangerous style constructs; the check result lists what was removed.
- Output carries `Content-Security-Policy: sandbox; default-src 'none'; ...`: the page runs in an isolated sandbox so injected scripts cannot execute, and nothing loads except inline styles and images or fonts embedded as `data:`, so the visitor's address is not leaked to third parties. External stylesheets, images, fonts and relative addresses on the site do not load, and the check warns about them; put styles in `<style>` and embed images and fonts as `data:`.
- Saving is rejected when the content is empty, not UTF-8, over the size limit or not HTML. A missing `viewport`, a non-UTF-8 encoding declaration, or no placeholder use produces a hint.

## Access statistics and monitoring

### Site statistics {#site-stats}

For a running listener, click "Stats" on a site row to open the statistics window, where you can choose the last 1 hour, 6 hours or 24 hours:

- Minute-level access statistics: one point per minute, kept for the last 24 hours, including a request count curve and 4xx / 5xx error curves;
- Time to first byte: P50 / P95 per minute and the highest P95 in the selected range. Latency runs from request arrival until the response headers are sent, without the response body transfer; WebSocket and other upgraded requests and aborted requests are not counted, and percentiles are estimated from the average time within fine-grained buckets. For threshold alerts see [Performance](/en/docs/modules/perf);
- Status code distribution (2xx / 3xx / 4xx / 5xx) and cumulative response traffic;
- Top 10 source IPs and top 10 countries (country statistics need an imported GeoIP database);
- Reverse proxy sites show the health status, weight, backup flag, current connection count and forwarded request count of each backend, including path-based and per-user backends (marked with their path or users);
- When requests were blocked, the totals by reason: banned, IP list, country filter, rate limit and WAF (failed sign-ins are not counted).

Statistics are kept in memory and start counting again after a restart or after the listener's own settings change. Editing a site keeps its counters. The rankings track at most 5000 distinct sources.

### Live connections {#connections}

For a running listener, each site row (including the default site) shows "N active" after its statistics: the number of requests the site is handling right now, updated as the page refreshes. Click **Connections** to open the connections window. It refreshes every 2 seconds and can be searched by IP:

- **Live**: source address and port, protocol (h1 / h2 / h3, WebSocket marked WS), request (method, host and path, without the query string), start time, duration so far and traffic (download; upload too when there is a request body or WebSocket). Sorted from longest-running, up to 500 rows. With [trusted proxies](#real-ip) configured the source is the real visitor address, with the proxy it came through noted.
- **History**: recently ended connections with start and end time, duration, traffic and result (completed / closed). Only connections that lasted at least 1 second or were closed manually are kept, the latest 200 per site, cleared on restart; see the access log for short requests.

The list counts requests in progress: an HTTP/1.1 keep-alive connection belongs to no site between requests, and one HTTP/2 or HTTP/3 connection can carry several requests at once, so requests are the unit that maps exactly to a site. An upgraded WebSocket stays listed until the connection closes.

**Close** on a row ends that request right after you confirm: HTTP/1.x closes the whole connection, HTTP/2 and HTTP/3 reset only that request and other requests on the same connection continue, and an upgraded WebSocket is closed together with its backend connection. It also takes effect at once when the visitor stops reading and sending is blocked. A closed request is logged as 444 in the access log (or keeps its status code if the response had started) and does not count as a backend error or toward path probing. The visitor can reconnect right away; to keep them out use **Close and ban**, which adds the source address (its /64 range for IPv6) to the [ban list](#ban-list) and closes its requests in progress on every listener and site. Loopback addresses (requests coming in through frp or a local forward without PROXY protocol all show the loopback address) and addresses within the trusted proxies (IPv6 checked by /64) cannot be banned, since banning them would block a large number of unrelated visitors; closing without banning has no such limit. The ban lasts 1 to 720 hours, or tick **Permanent**.

Read-only accounts and read-only tokens can view the list; closing and banning need write permission and are recorded in the audit log.

### Network topology {#topology}

**Web → Topology** in the side menu is a page of its own (`#/webtopology`); the **Topology** button in the Web Services page header opens it too. It lays out Internet → Gately → listeners → sites / listener actions → backends / outcomes, refreshes every 2 seconds, pauses while the page is in the background, and the canvas follows the light or dark mode of the current skin:

- **Nodes**: listeners show their address and running state, in red when they failed to start; sites show how they match (exact, wildcard, any domain, default site), QPS and requests in progress; reverse proxy sites expand into all their backends, including path-based and per-user backends (marked with their path or users), with unavailable backends in red; redirect, static file, fixed text and close-connection sites each lead to one outcome node. A default site that closes connections, plain HTTP requests redirected on an HTTPS port (301 / 308) and the 404 returned when no site matches and no default site is enabled are expected behavior and shown in gray. Disabled listeners and sites are listed too, shown as not running.
- **Links**: width grows with QPS, and links carrying traffic show a flowing animation. With more than 120 links only the highlighted path is animated, and nothing is animated when the system asks for reduced motion.
- **Highlighting**: hovering or clicking a node highlights its whole path, up to the Internet and down to the backends, labels the links on it with QPS and traffic and dims everything else; click an empty spot to reset.
- **Details panel**: for the selected node, QPS (average over the last 10 seconds), the 60-second average, download traffic, requests in progress, total requests, backend errors, blocked requests by reason, the status code distribution, per-second request and traffic curves for the last 60 seconds, and the active IPs that passed through it. The ⓘ next to each metric explains how it is measured. For a site the panel also opens its statistics, connections, access log and edit windows, a backend opens the access log and edit windows of its site, and a listener can be edited directly.
- **Viewing access logs**: after selecting a site or backend node, click "Access log" to open an access log containing only that site (matched by exact site name, so sites with similar names stay out); when the top filter is already set to an IP, the button becomes "Access log for this IP" and filters by that address as well. The log icon at the right of each active IP row in the details panel shows that address's access log for the selected site, or across all sites when the selection is not a site-type node. The filters in the log window can still be changed, and the site condition appears as a removable "Site: …" tag. The window also shows only visitors of that site's own listener, so a same-named site on another listener (for example separate IPv4 and IPv6 listeners both named HTTPS) stays out; this condition appears as a removable "Listener: …" tag. Only sites with access logs enabled have complete records, and the window shows a notice at the top when they are off; a site without a name is filtered by its first domain. The "Logs" button on each site row of the Web Services page opens the same window.
- **Active IPs**: visitor addresses with requests in the last 60 seconds, plus addresses that still have requests in progress. The canvas shows only counts; the full addresses are listed in the details panel. Type or pick an address at the top (or click one in the panel) to highlight only the path it took, down to the exact backend; **Clear filter** resets it. Each site, backend and listener action remembers up to 1024 recent addresses and the API returns at most 500, so with too many sources the panel notes that the list is incomplete. With [trusted proxies](#real-ip) configured these are the real visitor addresses.
- **Mask**: with the switch on, the chosen categories are shown with `*` for sharing screenshots; it only affects this page, and search still matches the full text. The categories are visitor IPs (active IP list and filter; the last part of IPv4, the last 64 bits of IPv6), backend addresses (IPs and host names of backends and redirect targets), listen addresses (listen IPs, listener error messages and sites that use an IP as domain) and domains (site domains and domains in site names; the first label and the suffix stay visible, such as `nas.***.com`). The first three are selected by default, and the choice is remembered.
- **Mask style**: the top of the Mask menu offers **Asterisks** or **Aliases**. Asterisks hide part of the value and keep the network and the ends; aliases replace the whole value with a number that stays the same on this page: visitor IPs become `Visitor-01`, IPs in backends and redirect targets `Backend-01`, listen addresses `Listen-01`, and domains keep only the suffix with the registrable part and the host part numbered separately, so `nas.home.example.com` becomes `host-01.domain-01.com` and subdomains of one domain share its `domain` number. Equal values always get the same alias and different values different ones, so you can still tell which nodes are the same. Numbers are given in order of appearance, kept only in the open page and renumbered after a reload.
- **Filter**: **Filter** in the top right can limit the view to some listeners; **Problems only** shows listeners that failed to start (with all their sites), enabled sites that are not running and sites with an unhealthy backend, while disabled listeners and sites count as expected; **Had blocks** shows, per reason (banned, IP list, country filter, rate limit, WAF), only sites that have blocked requests for that reason since Gately started. The conditions apply together and combine with the header search; filtering by problems or blocks hides listener-level actions and listeners with no sites left. Filters last only while the page is open.
- **Active only**: when there are many services, click **Active only** in the top right to show only sites and listener-level actions with requests in the last 60 seconds, requests in progress or active IPs passing through; proxy sites keep only their active backends (a site whose backends got no requests, for example because access control blocked them, is shown as an end node), and listeners without activity are hidden. In this mode listener folding and the "N more" summary nodes do not apply, and the canvas fits itself so these nodes are fully visible (at least 50% for Tree, Vertical and Sankey), updating as services start or stop being active. The number on the button is how many sites are active now, following the filter and search, and is shown even when the mode is off. It works together with the filter, search, IP highlighting and every layout, and the setting is remembered per device; when nothing has had requests in the last 60 seconds the canvas says so, and **Show all** leaves the mode.
- **Replay**: click **Replay** in the top right to open a timeline and look at the topology at an earlier moment. Recording is off by default; an administrator turns it on with **Start recording** on the timeline or in **Recording settings**. Once on, the whole topology (rates, counters, backend health, active IPs) is recorded as a frame every 5 seconds, compressed and kept only in memory up to the **Memory limit** (8 MiB by default, 1–256), dropping the oldest frames beyond it; consecutive identical frames (for example with no traffic) are stored once, so with little traffic you can look back a long way and with heavy traffic a shorter one. The timeline draws traffic as an area chart, marks periods with a listener that failed to start or an unhealthy backend in red along the bottom, and shades periods without recording (module disabled or no listener running). Click or drag the timeline to seek, and hover to see the QPS and active IPs at that moment; step frame by frame with the buttons or ← →, press Space to play or pause, choose a speed from 1× to 60×, and tick **Skip idle** to jump over periods with no traffic and no visitors during playback; the timeline can show everything recorded or the last 15 minutes up to 24 hours, remembered per device. During replay all seven layouts, the filter, Active only, masking and IP highlighting work as usual, and the curve in the details panel covers the 60 seconds before the chosen moment (one point per 5 seconds); **Connections** shows the current state, so it is hidden during replay. Click **Back to live** to leave. Recorded data includes visitor IPs and is cleared when Gately restarts or recording is turned off; recording a frame takes about 0.4 ms (measured with 40 sites and 200 active IPs) and about 1–2 KB after compression.
- **Layout**: the layout menu in the top right offers seven layouts; the choice is remembered, and switching fades between them and fits the canvas. **Tree** lays out five columns from left to right with the most detail; **Vertical** unfolds from top to bottom like an org chart, opens centered on Gately and is explored by dragging sideways; **Radial** puts Gately in the center with listeners, sites and backends on three concentric rings and names along the radius; **Sankey** draws every node as a bar and every link as a band, splitting each parent bar by the QPS share of its children, with bar heights and band widths following traffic while node positions stay put; **Sunburst** puts Gately in the center with listeners, sites and backends as three rings of segments; **Treemap** and **Bubbles** nest Gately, listeners, sites and backends as blocks or circles, every backend (or outcome) having the same area. Sunburst, Treemap and Bubbles have no links, darker meaning more traffic, which suits an overview of many sites. Sankey, Sunburst, Treemap and Bubbles have no fold buttons on the canvas; fold or expand a listener from its details panel.
- **Collapsing extra nodes**: each listener shows its first 8 sites and each site its first 6 backends by default; the rest are merged into a "N more sites / backends" node showing their combined QPS and requests in flight. Click it to expand them all, and click the **Collapse** node added at the end to fold them again. Active IPs on collapsed nodes count toward this node, and IP highlighting stops at it. Sites and backends with requests in the last 60 seconds or requests in progress are never collapsed: they stay visible in their original order even past the limit, so the summary node holds only idle ones and its subtitle reads "No requests in the last 60 seconds".
- **Pan, zoom and canvas size**: drag the canvas to pan and scroll to move up and down (the page scrolls once the edge is reached); hold Ctrl/⌘ while scrolling, or pinch on a trackpad or touch screen, to zoom between 15% and 250% around the pointer or the center of the pinch. The controls in the bottom right zoom in, zoom out, reset to 100% and fit to canvas (show all, the target icon); the rightmost button makes the canvas fill the browser window, and Esc or the same button restores it. Drag the bar below the canvas to change its height (touch works too); the height is remembered per device, and double-clicking the bar or clicking **Auto height** returns to automatic sizing.
- The search box in the page header filters sites in the topology (name, domain, group, backend address); listeners can be folded one by one or all at once. **Edit** and **Edit listener** in the details panel open the same editor as the Web Services page right on this page, and the topology refreshes immediately after saving, so you never leave the page; the statistics, connections and access log dialogs open on this page too.

All numbers come from this instance's in-memory counters: they reset on restart, editing a site keeps its counters, changing listener settings such as the port restarts that listener's counters, and other Gately instances are not aggregated. The view reads `GET /api/webservice/topology`, which read-only accounts and read-only tokens can access. Replay reads `GET /api/webservice/topology/replay` (overview and timeline) and `GET /api/webservice/topology/replay/frame` (the snapshot at a moment), which read-only accounts can use too; changing the recording settings is `PUT /api/webservice/topology/replay/settings` and needs write permission.

### Visitor audit {#visitor-audit}

The sidebar entry "Web → Visitor audit" (`#/webvisitors`) summarizes access to all sites by visitor address: who visited which sites and paths, when, under which identity, and how often they were blocked. Every site is counted, including the default site and requests that match no site, regardless of whether the site has access logging enabled; with [trusted proxies](#real-ip) configured, the real visitor address is recorded.

- **Summary and trend**: pick the range in the page header (today, 7 days, 30 days, the whole retention period). The cards show visitors (and new visitors), requests, blocked requests and their share, visitors that passed site authentication, visitors currently banned and scanner-like addresses; click a card to filter by it, click again to clear. In the daily bar chart the bar height is the request count and the red part is blocked requests; click or hover a day to see its visitors, requests and blocks.
- **List**: each visitor shows the country code (with a GeoIP database imported; private and local addresses show "LAN"), requests and blocks in the range, the most visited sites, identities, first and last visit, and the last request (status, method, path, domain and User-Agent). Search by address, identity, site, domain, path or User-Agent; filter by "With blocked requests", "Authenticated", "Administrators", "Banned", "Scanner-like" and site; sort by last visit, most requests, most blocked or first visit. Sites with the same name on two listeners carry the listener name.
- **Identity**: after site authentication succeeds (web login, basic auth, shared users, third-party login, forward auth) the user name is recorded; requests carrying this instance's admin console session are recorded as "admin". Failed password attempts and blocked requests record no identity.
- **Scanner-like**: addresses that were only blocked or only hit unmatched sites and never authenticated.
- **Details**: click a row to open the visitor: first and last visit, total requests, blocked requests, bytes sent, connections closed (444), a daily request chart over the retention period, status distribution and block reasons (ban, IP list, country filter, rate limit, WAF), sites visited, most requested paths (up to 10), identities and User-Agents. From here you can ban or unban (IPv6 is banned by /64), view the access log for this address, or delete the visitor's record.

**What is kept and where**: only per-visitor aggregated counts are stored, never individual requests; only the path part is recorded, and query strings, request bodies, cookies and credentials are always left out. Each visitor keeps up to 12 sites, 10 paths, 6 identities and 4 User-Agents; beyond that the entry with the fewest requests is replaced. For individual requests, enable the site's access log. Data lives in `visitors/visitors.jsonl.gz` under the config directory (mode 0600), is written about 10 minutes after changes and immediately on exit, and is not part of config backups.

**Settings**: "Settings" in the page header disables recording (existing data is kept), sets the retention in days (1-180, default 30) and the maximum number of visitors (100-100000, default 10000), and can clear all data. Visitors whose last visit is older than the retention are deleted automatically; shortening the retention deletes the excess data immediately, after a confirmation. When the visitor limit is exceeded, scanner-like addresses are evicted first, then those not seen for the longest time. Read-only accounts and read-only tokens can view; banning, deleting and changing settings need write access. API: `GET /api/webservice/visitors`, `GET /api/webservice/visitors/detail`, `POST /api/webservice/visitors/delete` and `GET|PUT /api/webservice/visitors/settings`.

### Prometheus metrics

The global metrics endpoint `GET /api/metrics` aggregates metrics from all modules; see [Monitoring](/en/docs/reference/monitoring). The module endpoint below covers Web Services metrics only.

The metrics endpoint is `GET /api/webservice/metrics` in Prometheus text format, and it requires login credentials or an [API token](/en/docs/reference/api-reference).

```bash
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/webservice/metrics
```

| Metric | Type | Description |
| --- | --- | --- |
| `gately_site_requests_total` | counter | Cumulative requests of a site |
| `gately_site_errors_total` | counter | Backend errors of a site |
| `gately_site_active_requests` | gauge | Requests in progress |
| `gately_site_response_bytes_total` | counter | Cumulative response bytes |
| `gately_site_responses_total` | counter | Responses counted by status class (label `class`: `2xx` to `5xx`) |
| `gately_site_blocked_total` | counter | Blocked requests by reason (label `reason`: `ban` banned, `ip` IP list, `country` country filter, `rateLimit` rate limit, `waf`) |
| `gately_upstream_up` | gauge | Whether a backend is healthy: 1 healthy, 0 unavailable (with label `upstream`) |
| `gately_upstream_requests_total` | counter | Requests forwarded to a backend (with label `upstream`) |
| `gately_upstream_response_bytes_total` | counter | Response body bytes returned by a backend, excluding data after a WebSocket upgrade (with label `upstream`) |
| `gately_listener_requests_total` | counter | Requests the listener handles itself without a site, label `action`: `https_redirect` (plain HTTP redirected on an HTTPS port) or `not_found` (404 when no site matches); carries only the `listener` label |

Except for `gately_listener_requests_total`, metrics carry the labels `listener` (listener name#listener key) and `site` (site name, or the first domain when unnamed). Path-based and per-user backends also carry a `route` label on `gately_upstream_*`: the path prefix, or `@` followed by the user list. The `upstream` label is the backend address without its user name, password and query string.

## Import nginx config {#import-nginx}

"Import nginx" at the top of the page converts existing nginx sites into Gately listeners and sites. We recommend running `nginx -T` on the nginx host and pasting the full output: it contains every file pulled in by include, and Gately expands include the same way nginx does.

- Preview first: sites are grouped by listening port, each with its type, backend or directory, source file and line number, plus anything that needs attention (directives using variables, regex locations, domains without a certificate, and so on). Sites that cannot be converted come with the reason;
- Sites on the same port are merged into the existing listener, otherwise a new listener is created; domains that already exist are skipped, so importing again is safe;
- Imports are disabled by default, so you can run alongside nginx and review them one by one before enabling. Tick "Enable right after import" to apply them immediately (a new listener fails to start while nginx still holds the same port);
- What is converted: `proxy_pass` (including upstream weights, backup servers and load-balancing policies), `root` / `alias` static directories (`autoindex`, `try_files $uri.html`, `error_page 404`), `return` redirects and fixed content, whole-site `rewrite` redirects, `add_header`, `proxy_set_header`, `gzip`, `allow` / `deny`, location-level path backends, access control and redirects, and `listen ... proxy_protocol` and `quic`;
- `proxy_connect_timeout` and `proxy_read_timeout` become the site's [connect and response header timeouts](#backend-timeouts); a value in location / takes precedence over the server level, and values above the limit are imported at the limit. Different values in other locations are not imported, and the preview says so;
- `client_body_timeout` and `send_timeout` become the site's [visitor timeouts](#client-timeouts), for every site type;
- `auth_request` becomes forward auth: the auth endpoint comes from the `proxy_pass` of the auth subrequest location (`set` variables are supported) and the login redirect from `error_page 401 =302`, so common Authelia setups convert directly. When it is enabled only in some locations, the site's "Authentication scope" is limited to those paths, which become path rules set to "Authentication required";
- `auth_basic` becomes basic authentication with shared users. nginx user files are not read, so nobody can get through until you add the users;
- Configure TCP/UDP forwarding from `stream` in [Port Forwarding](/en/docs/modules/portforward). Certificates are not imported: when an HTTPS site has no certificate, the preview lists the certificate files nginx uses, and you can add them by file path on the [SSL Certificates](/en/docs/modules/ssl) page.

## Replacing haproxy and nginx

Protocol Routing and Web Services together can replace an haproxy + nginx pair. A common layout is one [Protocol Routing](/en/docs/modules/protoroute) rule on port 443 that sends VPN and other TLS services straight to their backends and everything else to a Web Services listener that accepts the PROXY protocol, plus a Web Services listener on port 80 with a redirect site (`https://{host}`).

Converting nginx settings:

| nginx | Gately |
| --- | --- |
| `server` with `server_name` | a site with those domains |
| `location` with a different `proxy_pass` | path backends |
| `auth_request` (Authelia) | forward auth, or shared users with TOTP |
| `proxy_ssl_name $host` | use the visited domain as SNI |
| `return 301 https://$host$request_uri` | a redirect site with `https://{host}` and "Keep path" |
| `return 302 /admin` on `location = /` | a rewrite with "Redirect" on, pattern `^/$` |
| `try_files $uri $uri.html $uri/index.html` and `error_page 404` | static files with clean URLs and a 404 page |
| `error_page 502 503 504 /50x.html` and `proxy_intercept_errors on` | the site's [error pages](#error-pages) setting and "Intercept backend errors" |
| `add_header` | response headers |
| `return 444` | a Close connection site |
| `error_page 497 https://$host:$server_port$request_uri` in an HTTPS server | *Plain HTTP on same port* set to *Redirect to HTTPS* on a new listener; when merged into an existing listener, only a warning is shown and its settings stay unchanged |

## Open firewall port {#auto-firewall}

When Gately runs on a router such as OpenWrt, or on a Linux host with a firewall enabled, the system firewall blocks connections from outside by default, and a listener's port has to be allowed before it can be reached. With "Open firewall port" turned on for a listener, Gately allows only that listener's port in this host's firewall:

- Only ports that are actually listening are allowed. If the port is taken by another program and the listener fails to start, nothing is allowed, so that program (a router's admin page, for example) stays closed to the outside.
- The listener's network decides the address family: "IPv6 only" allows IPv6, "IPv4 only" allows IPv4, dual stack allows both. With a listen address set, only connections to that address are allowed. Listeners on 127.0.0.1 or ::1 need nothing.
- With HTTP/3 on, the UDP port with the same number is allowed too.
- The rule is removed when the listener is disabled, deleted, fails to start or the switch is turned off, and when Gately exits.
- nftables (OpenWrt fw4, firewalld and others): the rule is inserted at the top of the input filter chains. iptables / ip6tables (OpenWrt fw3, ufw and others): at the top of the INPUT chain. Rules carry a `gately-open:` comment and are restored within 30 seconds after the firewall reloads.
- Kernels without nf_tables (such as Synology DSM) use iptables (legacy) instead: the rules go into a filter chain of Gately's own (starting with `GTLY_OPEN_`), and the first rule of INPUT jumps to it; when the system firewall puts its own rules in front, the jump is moved back to the top within 30 seconds. The Docker image includes iptables (legacy).
- Linux only. Docker deployments need host networking and `cap_add: NET_ADMIN`. The listener card shows the result; when allowing fails, the "Firewall" tag turns orange.

Rules in [Port Forwarding](/en/docs/modules/portforward#auto-firewall) and [Protocol Routing](/en/docs/modules/protoroute#auto-firewall) have the same switch, and [STUN](/en/docs/modules/stun#auto-firewall) rules allow their channel port.

### Allowing ports by hand {#manual-firewall}

When allowing ports by hand, add a rule for each port that must be reachable and keep inbound traffic on the wan zone rejected. Setting the whole wan zone's input to accept exposes the router's admin page (80 / 443), SSH and other services to the internet as well. On OpenWrt, add it under Network → Firewall → Traffic Rules, or in `/etc/config/firewall`:

```
config rule
	option name 'Allow-Gately-HTTPS'
	option src 'wan'
	option family 'ipv6'
	option proto 'tcp'
	option dest_port '50663'
	option target 'ACCEPT'
```

- When Gately runs with Docker bridge networking, the container cannot see the router's firewall; allow the mapped port on the host or the router.
- When Gately runs on another device in the LAN (for example a container with its own IPv6 address through macvlan), the traffic is forwarded by the router, so add a forward rule on the router with lan as the destination zone, the device's address and the ports in use. ISP-assigned IPv6 prefixes change; match the address by its suffix, for example `option dest_ip '::1234/::ffff:ffff:ffff:ffff'`.

## Deployment tip

Web Services need to listen on host ports such as 80 / 443. For Docker deployments, `--network host` is recommended.
