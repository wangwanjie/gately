# WAF Firewall

A web application firewall based on [Coraza](https://coraza.io/) and OWASP CRS v4, which can be attached to sites in [Web Services](/en/docs/modules/web).

## Capabilities

- Built-in OWASP CRS v4 rule set, with rule files that can be enabled individually;
- Custom SecLang rules;
- Rule groups can be reordered; match logs record the client IP, domain, path and rule ID, and can be filtered by domain, IP or keywords;
- Blocking mode and detection-only mode;
- Adjustable request body inspection limit: over the limit, reject the request or inspect only the first part; request body inspection can be skipped by path;
- WebDAV compatibility mode: allows the methods, headers and any upload content type used by cloud drives and WebDAV;
- Can use a self-hosted SafeLine detection engine, alone or together with rule groups;
- Multiple instances, referenced by sites as needed.

## Recommendations

1. For a new site, start with "Detect only" and watch the logs for a while to confirm there are no false positives;
2. Adjust rule files or add custom exceptions for any false positives;
3. Then switch to "Block".

## Admin UI requests

When the admin UI is reverse proxied through a site with WAF, requests carrying a valid administrator session of this instance skip the WAF (SafeLine included). Saved configurations often contain file paths (such as the remarks written by the nginx import), HTML (custom error pages) and scripts (certificate deployment), which CRS path traversal, XSS and command injection rules would block with 403. The session is signed by the server and only the administrator role counts; requests from read-only accounts, visitors and other instances' admin UIs are still checked.

## Signed-in users

When a site enables "Trust signed-in users" (see [Web service](./web)), visitors authenticated by that site's Gately authentication (web authentication, third-party login, shared-user single sign-on, verified basic authentication) skip the WAF. Visitors who are not signed in, requests to the login page itself, forward authentication and share links are still checked.

## Rule group order

The rule groups of an instance load from top to bottom; use the move up and move down buttons to reorder them. Within the same phase, earlier rules run first:

- Blocking rules (deny and the like) work in any position;
- Runtime allow or exclusion rules (`allow`, `ctl:ruleRemoveById`, `ctl:ruleRemoveTargetById`, etc.) must sit above the OWASP CRS group, or CRS may block the request first;
- Configuration-time directives such as `SecRuleRemoveById` and `SecRuleUpdateTargetById` only affect rules that are already loaded, so they must sit below the CRS group; above it they have no effect.

## Sites that take large uploads

By default the WAF inspects request bodies of up to 12.5 MB and rejects larger requests (413), so cloud drives, WebDAV and file managers cannot upload big files. Adjust this in the instance's "Request body inspection":

- **Inspection limit**: 0 means the default 12.5 MB, up to 1024 MB;
- **Over the limit**: "Reject the request" keeps the previous behavior; "Inspect the first part only" inspects only the part within the limit and passes the rest to the backend unchanged, which suits sites that take big uploads;
- **WebDAV and upload compatible**: by default OWASP CRS only allows GET, HEAD, POST and OPTIONS, and blocks the `Lock-Token`, `If` and `Content-Range` headers and content types outside its allow list, so WebDAV clients cannot list folders, upload or lock files. When enabled, PUT, DELETE, PROPFIND, PROPPATCH, MKCOL, COPY, MOVE, LOCK, UNLOCK and similar methods and these headers are allowed, and upload content types are no longer restricted; request bodies of PUT and `application/octet-stream` are treated as file content and no longer inspected as form parameters. Checks for injection, path traversal and other attacks still apply;
- **Paths without body inspection**: one path prefix per line (such as `/api/upload` or `/remote.php/dav/`). Matching requests skip body inspection; headers, URL and query parameters are still inspected.

We recommend a separate instance for cloud drive and WebDAV sites: turn on "WebDAV and upload compatible" and choose "Inspect the first part only", add the upload endpoints to the skipped paths if needed, and let other sites keep the defaults.

curl, rclone and similar clients send an `Expect: 100-continue` header when uploading larger content, which CRS 4 blocks by default. All Gately instances allow this header (Gately handles 100-continue itself), so no extra setting is needed.

## Using SafeLine

[SafeLine](https://waf.chaitin.com/) is a web application firewall by Chaitin Tech that detects attacks with a semantic analysis engine and produces few false positives. Gately can send requests to your self-hosted SafeLine detection engine (safeline-detector) and allow or block them based on the result; entry points, certificates, authentication and bans stay in Gately, so SafeLine no longer needs to be an extra reverse proxy layer.

1. Install SafeLine Community Edition following its official documentation; the default installation directory is `/data/safeline`.
2. Choose how to connect to the detection engine:
   - **Unix socket** (Gately and SafeLine on the same machine): the engine listens on `/data/safeline/resources/detector/snserver.sock` by default, so use the address `unix:/data/safeline/resources/detector/snserver.sock`. When Gately runs in Docker, mount the `/data/safeline/resources/detector` directory into the Gately container and use the path inside the container.
   - **TCP**: add the two lines below to `/data/safeline/resources/detector/detector.yml`, then run `docker restart safeline-detector`. After switching to TCP the engine no longer listens on the Unix socket.

     ```yaml
     bind_addr: 0.0.0.0
     listen_port: 8000
     ```

     Use the IP and port of the safeline-detector container as the address, for example `172.22.222.5:8000`; `docker inspect safeline-detector | grep IPAddress` shows the container IP. When Gately runs on a Docker bridge network, join it to SafeLine's network with `docker network connect safeline-ce <Gately container name>`. The detection engine has no authentication, so do not expose port 8000 to the internet.
3. In the WAF instance, turn on "Use SafeLine", enter the address and click "Test connection". Gately sends a heartbeat and then a sample SQL injection request for inspection; "the sample attack was detected" means the engine works. This sample attack shows up in the SafeLine console's attack records with source 127.0.0.1 and domain gately-test.invalid.
4. Reference the instance from a Web Service site.

Notes:

- With SafeLine connected, rule groups can be left empty; if you keep them, requests are checked by SafeLine first and then by the rule groups. In "Detect only" mode, attacks detected by SafeLine are only logged and the request goes through.
- Blocked requests: browsers see a block page with the event ID, other clients get the same JSON as SafeLine's official plugins (including `event_id`). The status code is the one suggested by the engine, usually 403. Blocks are counted in access logs (`deny=waf`), site statistics and WAF block events, and the "WAF blocks" preset of Intrusion Guard applies as well.
- Attacks are also recorded in the SafeLine console and can be looked up by event ID; these records do not belong to any SafeLine site.
- Up to 1024 KB of the request body is sent for inspection by default; the rest is not inspected and is forwarded to the backend unchanged. "Paths without body inspection" and "WebDAV and upload compatible" apply to SafeLine too; gRPC request bodies of unknown length over HTTP/2 are not inspected.
- The detection timeout defaults to 1 second. When the engine is unavailable (connection failure or timeout), requests are allowed by default, the instance shows "SafeLine down" and an error is logged; with "Reject requests" such requests get 503. Right after the engine starts it may return unrecognized results for a few seconds, which are treated the same way.
- Only requests are inspected; responses are not sent to SafeLine.

## Testing

The WAF is tested with real attack payloads, covering blocking of path traversal and similar attacks.
