# Web Services

Serve reverse proxy, redirects, static files and fixed text by domain. This is Gately's unified web entry point. Menu location: sidebar "Web → Web Services".

## Structure: listeners, sites and the default site

The configuration has three layers:

1. Listener: one listening port. Fields include network (`tcp` dual-stack, `tcp4`, `tcp6`), listen address (empty for all addresses), port, HTTPS switch, minimum TLS version (1.0 / 1.1 / 1.2 / 1.3), and whether to accept the PROXY protocol.
2. Site: a site under a listener, matched by domain. Domains can be an exact domain, `*.example.com` (matches subdomains at any depth) or `*` (matches everything). Matching priority is exact domain, wildcard, then `*`.
3. Default site: used when no site matches. When it is not enabled, a 404 is returned. The default site can also be set to "Close connection" so requests for unknown domains get no response at all. Once configured, the default site is shown last in the site list; until then, click "Set default site" on the listener card to configure it.

Saving a change to a site or to the default site swaps the site list in place: the listener keeps running, open connections (WebSocket included) stay up, and each site keeps its statistics. If the new configuration is invalid, the previous sites keep serving. Changing the listener's own settings (network, address, port, HTTPS, minimum TLS, HTTP/3, PROXY protocol) restarts that listener.

A site can have a "Group" (up to 64 characters; the default site cannot be grouped). Under each listener, the list shows groups as collapsible sections: groups appear in the order they first occur, ungrouped sites come after them, and the default site always comes last. The collapsed state of each group is kept in the browser. Click "Sort" to enter sort mode, where you can move sites up and down, drag them within their group, move whole groups and reorder listeners. Order only affects the list: domain matching follows the priority above regardless of site order, and saving the new order swaps the site list in place without dropping connections. The search box at the top of the page filters sites by domain, name, group, backend or remark.

In sort mode you can also check several sites (the checkbox on a group header selects the whole group). Dragging any checked site moves all of them, and dropping onto a site or the header of another group moves them into that group. The toolbar moves the checked sites into a group in one step, or out of their group when the name is left empty; checked sites under different listeners are saved per listener.

Hover over a group header and click the pencil button to rename the group: everything in it moves to the new name and keeps its position and order. Choosing a name that is already in use merges the two groups after a confirmation.

## Site types

| Type | Description |
| --- | --- |
| Reverse proxy | Forwards to one or more backends. Backend addresses use the form `http(s)://host:port`, and WebSocket is supported |
| Redirect | Redirects to a full URL with status code 301 / 302 / 307 / 308 (default 302), optionally keeping the original path and query string. The URL may contain `{host}`, which is replaced by the domain the visitor used: `https://{host}` with "Keep path" sends every HTTP request to HTTPS on the same domain. Variables such as `{path}`, `{1}` and `{STUN_RuleName_PORT}` are also available; see [Redirect variables](#redirect-vars). |
| Static files | The directory must be an absolute path. With "Directory listing" off, a directory request returns a response only when `index.html` exists. "Clean URLs" tries `path.html` and `path/index.html` when a file is missing (for VitePress and similar generators, like nginx `try_files $uri $uri.html $uri/index.html`). "404 page" is a path inside the directory, such as `404.html`, returned with status 404 |
| Fixed text | Custom content, `Content-Type` (default `text/plain; charset=utf-8`) and status code (100-599, default 200) |
| Close connection | Sends nothing and closes the connection, like nginx `return 444`. Typically used as the default site so requests scanning unknown domains get no response; the access log records them as 444; it skips authentication, access control, the WAF and rewrites, so the other site settings have no effect |

Sites of every type can set an access log, cross-origin (CORS), custom request and response headers (one `Key: Value` per line; an empty Value removes that header), and can reference a [WAF Firewall](/en/docs/modules/waf) instance (if the instance does not exist or is disabled, requests pass through). See [Request and response headers](#headers) for how request headers, response headers and Content-Type are edited.

Access log entries record the client IP, method, domain and path, status code, duration and site name, followed by the Referer and User-Agent in nginx format (`-` when missing), so different devices behind the same public IP can be told apart by User-Agent. The log can be filtered by site, status code, client IP and more; see [viewing and filtering runtime logs](/en/docs/guide/configuration#log-filter).

### Redirect variables {#redirect-vars}

The target of a redirect site can use the following variables:

| Variable | Meaning |
| --- | --- |
| `{host}` | The domain the visitor requested, without the port |
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
- If the expanded target has no host (for example `{1}` is empty because an exact domain matched, but the target is just `https://{1}`), the site returns 503; if the visitor's host contains characters that never appear in a domain, such as `&`, `=`, `/` or `@`, the site returns 400 and the host is not inserted into the target;
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
| Add X-Forwarded-* | Writes `X-Forwarded-For` / `X-Forwarded-Proto` / `X-Forwarded-Host` / `X-Forwarded-Port` and `X-Real-IP`, telling the backend the visitor's real IP, original protocol, domain and port | The backend logs real IPs or must know the outside is HTTPS to build correct links (otherwise it may bounce back to http or loop after login). Recommended for most apps |
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

## Transport and protocols

- HTTPS: uses a certificate from the [SSL Certificates](/en/docs/modules/ssl) module that covers the domain;
- Plain HTTP on the same port: a listener with HTTPS can also accept plain HTTP, telling the protocols apart by the first byte of each connection. *Redirect to HTTPS* sends `http://host:port` to `https://host:port` (301 for GET/HEAD, 308 for other methods), useful when only one port is available and visitors may type the wrong scheme; *Serve* answers over unencrypted HTTP. ACME HTTP-01 challenges are still answered, and changing this option restarts the listener;
- HTTP/3 (QUIC): turn it on for a listener with HTTPS enabled. HTTP/3 is then also served over UDP on the same port number and advertised to browsers with the `Alt-Svc` response header; allow that UDP port in your firewall and port mapping;
- WebSocket: supported automatically by reverse proxy;
- PROXY protocol: a listener can accept PROXY headers sent by upstream components (such as haproxy and frp). It trusts only the IPs or subnets listed in "Global settings → Trusted PROXY sources"; leaving it empty trusts no source. Reverse proxy can send PROXY headers to the backend. `X-Forwarded-Proto: https` on connections from a trusted source is honored too: session cookies get Secure, ports that redirect plain HTTP no longer redirect, and with "Add X-Forwarded-*" on, https is passed on to the backend.

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

On a site, tick "Use shared users" next to web or Basic authentication. The site's own users keep working as well. You can also select sites in bulk in the site list on the Auth users page, grouped by listener and filterable: selecting a site without authentication also turns on web auth; a site that authenticates only with shared users cannot be cleared there, so it never becomes public by accident (turn off authentication in the site settings if you want it public); sites using forward auth cannot use shared users. Changes to users, OIDC and sites on the page take effect together when you click Save at the bottom; if any of them fails validation nothing is saved, and leaving the page with unsaved changes asks for confirmation first.

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

Closing the connection, as a site type or as the deny action, only withholds the HTTP response and cannot hide that the service is up:

- the TCP handshake has completed, so the client knows the port is listening;
- HTTP/1.x closes the connection, while HTTP/2 and HTTP/3 only reset the request and keep the connection open for reuse;
- over HTTPS, domains without a certificate already fail during the TLS handshake, so requests that reach the default site have received the certificate and the domain names in it.

To make scanners see only timeouts afterwards, ban their addresses in [Intrusion Prevention](/en/docs/modules/guard); with firewall sync on, their traffic is dropped in the kernel.

### Subscribed IP lists

Add lists under "Global settings → Subscribed IP lists": enter the URL and refresh interval (24 hours by default). Gately downloads each list on schedule and caches it in the configuration directory, so it works after a restart without network access. The content has one IP, CIDR or `start-end` range per line; anything after `#`, `;` or `//` is a comment. For Chinese provinces and cities you can use [metowolf/iplist](https://github.com/metowolf/iplist), for example `https://metowolf.github.io/iplist/data/cncity/440000.txt` (Guangdong).

- Allowlists and blocklists of sites and path rules can reference several lists, checked together with the manually entered ranges; matching any of them counts as being on the list;
- If downloaded content cannot be parsed (for example an error page was returned), the previous list is kept, the reason is shown on the list, and the download is retried an hour later;
- Until a list has downloaded successfully it counts as empty: an allowlist therefore denies access, and a blocklist blocks nothing extra;
- A list still referenced by a site cannot be deleted.

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

The ban list in "Global settings" shows the currently banned IPs, the reason (repeated authentication failures or manual ban) and the expiry time, and each entry can be lifted with "Unban". You can also ban manually: enter an IP and a number of hours (1 to 720, which is at most 30 days); the API defaults to 24 hours.

The ban list is shared with [Intrusion Prevention](/en/docs/modules/guard): when the Intrusion Prevention module is on, this section only shows the number of bans and links to the Intrusion Prevention page for managing them. Bans also apply to port forwarding, STUN and other services, and are dropped in the system firewall when firewall sync is on; with "Ignore LAN addresses" on, LAN addresses are never banned automatically.

### GeoIP country filter

A site's "Country filter" has three modes: off, allow only, and deny. Enter two-letter ISO country codes separated by commas, such as `CN, HK, US`. At least one country is required when enabled.

- Filtering depends on a GeoIP database, which you must first import under "Global settings → GeoIP database": upload a `.mmdb` file, or enter a download URL. MaxMind GeoLite2, DB-IP and other country or city databases are supported; the upload limit is 129 MB;
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

- The longest prefix wins, and the path is forwarded unchanged (the prefix is not stripped);
- Other settings of the site, such as the Host option, certificate checks, forwarded headers, the PROXY protocol, the load-balancing policy and health checks, apply to every path backend;
- Compression, rate limiting, caching and rewrites apply to the whole site.

## Route by user {#user-routes}

For a reverse proxy site, send each signed-in user's requests to their own backends, like nginx `map $remote_user $backend`. The site needs basic auth, web auth or forward auth first. Then add rules under "Route by user" on the "Security" page of the site editor: each rule lists a set of users and the backends for those users, written like the site's backends.

Users are matched by these identifiers:

- The user name of a site user or a [shared user](#shared-users);
- For third-party sign-in, `provider-key:id`, for example `github:octocat`; the identity's other identifiers match too;
- For forward auth, the `Remote-User` copied from the auth service's response.

Routing:

- Users not listed, and visitors who are not signed in on public paths (paths that need no authentication), use the site's backends. A visitor already signed in through web auth is routed by identity on public paths too, so pages of the same app reach the same backend;
- Path backends with their own backends (see the previous section) take priority; path backends that only add headers use the user's backends;
- When "Pass user name" is set to a header name such as `X-Remote-User`, the signed-in user name is put in that header for the backend. The same header sent by visitors is always removed, so it cannot be forged;
- With the in-memory response cache enabled, the cache key includes the chosen user route, and also the user name when "Pass user name" is on, so responses from different backends or for different users are kept apart.

Saving checks that the site has authentication enabled, that each rule has at least one user and valid backends, that no user appears in two rules, and that the header name is valid and is not one that affects forwarding or authentication, such as `Host`, `Cookie`, `Authorization` or `X-Forwarded-For`. In the site list, a site with user routes shows a tag with the number of rules.

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

- Only GET requests are cached, and the request must not contain `Authorization`, `Cookie` or `Range` headers, nor `Cache-Control: no-cache`;
- Only responses with status 200 are cached; a response is skipped if it contains `Set-Cookie`, `Vary: *`, or a `Cache-Control` of `no-store` / `private` / `no-cache`;
- `s-maxage` is honored first, then `max-age`;
- A single entry is limited to 4 MB and also to at most a quarter of the total capacity;
- A hit returns the `X-Cache: HIT` and `Age` headers, and a miss returns `X-Cache: MISS`;
- The cache key includes Host, the full request URI and `Accept-Encoding`; with [route by user](#user-routes), it also includes the chosen user route, and the user name when "Pass user name" is on.

### Path rewrite and redirect

Rules are matched in order, and matching stops at the first hit.

| Field | Description |
| --- | --- |
| `pattern` | A regular expression matched against the request path, such as `^/old/(.*)$` |
| `to` | The replacement result; use `$1` to reference groups |
| `redirect` | Off for an internal rewrite (the rewritten path is sent to the backend, and the target must start with `/`); on for a redirect |
| `code` | Redirect status code 301 / 302 / 307 / 308, default 302 |

Redirects keep the original query parameters. A redirect target may contain `{host}`, the domain the visitor used. Redirect rules run before authentication, like nginx `location = / { return 302 ...; }`, so a path such as `/` can send visitors on without asking them to sign in.

## Access statistics and monitoring

### Site statistics

For a running listener, click "Stats" on a site row to open the statistics window, where you can choose the last 1 hour, 6 hours or 24 hours:

- Minute-level access statistics: one point per minute, kept for the last 24 hours, including a request count curve and 4xx / 5xx error curves;
- Status code distribution (2xx / 3xx / 4xx / 5xx) and cumulative response traffic;
- Top 10 source IPs and top 10 countries (country statistics need an imported GeoIP database);
- Reverse proxy sites show the health status, weight, backup flag and current connection count of each backend.

Statistics are kept in memory and start counting again after a restart or after the listener's own settings change. Editing a site keeps its counters. The rankings track at most 5000 distinct sources.

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
| `gately_upstream_up` | gauge | Whether a backend is healthy: 1 healthy, 0 unavailable (with label `upstream`) |

All metrics carry the labels `listener` (listener name#listener key) and `site` (site name, or the first domain when unnamed).

## Import nginx config {#import-nginx}

"Import nginx" at the top of the page converts existing nginx sites into Gately listeners and sites. We recommend running `nginx -T` on the nginx host and pasting the full output: it contains every file pulled in by include, and Gately expands include the same way nginx does.

- Preview first: sites are grouped by listening port, each with its type, backend or directory, source file and line number, plus anything that needs attention (directives using variables, regex locations, domains without a certificate, and so on). Sites that cannot be converted come with the reason;
- Sites on the same port are merged into the existing listener, otherwise a new listener is created; domains that already exist are skipped, so importing again is safe;
- Imports are disabled by default, so you can run alongside nginx and review them one by one before enabling. Tick "Enable right after import" to apply them immediately (a new listener fails to start while nginx still holds the same port);
- What is converted: `proxy_pass` (including upstream weights, backup servers and load-balancing policies), `root` / `alias` static directories (`autoindex`, `try_files $uri.html`, `error_page 404`), `return` redirects and fixed content, whole-site `rewrite` redirects, `add_header`, `proxy_set_header`, `gzip`, `allow` / `deny`, location-level path backends, access control and redirects, and `listen ... proxy_protocol` and `quic`;
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
| `add_header` | response headers |
| `return 444` | a Close connection site |
| `error_page 497 https://$host:$server_port$request_uri` in an HTTPS server | *Plain HTTP on same port* set to *Redirect to HTTPS* on a new listener; when merged into an existing listener, only a warning is shown and its settings stay unchanged |

## Deployment tip

Web Services need to listen on host ports such as 80 / 443. For Docker deployments, `--network host` is recommended.
