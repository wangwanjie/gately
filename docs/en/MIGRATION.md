# Migrating from Lucky to Gately

> Gately is an independent project and is not affiliated with Lucky; compatibility relies on publicly observable formats.

Gately can read Lucky's `.lkcf` configuration files, or the whole-site zip exported by Lucky's "Back up configuration", and migrate your rules, tasks, and certificates. You do not need to uninstall Lucky. Both can run side by side while you switch over one module at a time.

::: tip Migrating from nginx
This guide covers migrating from Lucky. Existing nginx sites can be imported directly under "Web Services → Import nginx"; see [Web Services · Import nginx config](/en/docs/modules/web#import-nginx).
:::

## 1. Preparation

1. Locate Lucky's configuration directory. For Docker deployments this is usually the host directory mapped to `/app/conf` in the container, which contains files such as `lucky_base.lkcf` and `lucky_ddns.lkcf`.
2. Deploy Gately (see the README). Recommendations:
   - Use `--network host`, so it listens directly on host ports.
   - The admin port defaults to 16680, which does not conflict with Lucky's 16601.
   - If Lucky's scripts or certificate mappings use in-container paths (for example `/etc/lucky/custom_scripts` or `/data/nginx/...`), mount the same host directories at the **same paths** in the Gately container. No edits are needed after import.

## 2. Import

Open Gately's "Settings → Backup & Migration" and select all the `lucky_*.lkcf` files, or select the whole-site configuration zip exported by Lucky (the `.lkcf` files inside the archive are detected automatically and everything else is ignored). Click "Preview" first to see how many entries each module will import and which notices apply, then click "Import" to confirm.

Importing repeatedly is safe: rules, tasks, and certificates whose names match existing entries are skipped.

Supported files:

| File | Imported into | Notes |
| --- | --- | --- |
| lucky_portforward.lkcf | Port Forwarding | TLS/stream encryption and rate-limit options are not supported yet; a notice is shown |
| lucky_ddns.lkcf | Dynamic DNS | 11 providers supported; Lucky's official IP lookup URLs are removed |
| lucky_stun.lkcf | STUN Traversal | Includes the global STUN server list and the global webhook |
| lucky_ssl.lkcf | SSL Certificates | Already-issued certificates are imported as-is and are not re-requested before they expire |
| lucky_reverseproxy.lkcf | Web Services | Sites, authentication, and WAF references (when a sub-rule's WAF is `main`, the listener rule's instance is reused); variables in redirect sub-rules such as `{host}{path}{args}`, `{port}`, `{1}` and `{STUN_RuleName_PORT}` keep working |
| lucky_coraza.lkcf | WAF Firewall | Instance keys are preserved, so site references remain valid |
| lucky_wol.lkcf | Wake-on-LAN | |
| lucky_webterminal.lkcf | Web Terminal | SSH and Telnet connections, proxies, `keyFile`, `workDir`, timeouts, environment variables, quick access directories, session settings, global shortcuts, and the user agreement and 2FA secret; SSH connections record the host fingerprint on first connect |
| lucky_webdav.lkcf | WebDAV | A user's mount list becomes multiple directories, and users with the same name and password are merged; mounts that reference Storage resolve to the corresponding local directories |
| lucky_ftpserver.lkcf | FTP | Listen port, passive port range, TLS and timeout parameters, plus users (handled the same way as WebDAV) |
| lucky_storagemanagement.lkcf | Storage Mounts | Local directories and WebDAV storage; Alibaba Cloud Drive, system mounts, and access proxies are not migrated |
| lucky_frp.lkcf | frp | The first client instance (server address, token, transport protocol, and proxies) and the first server instance; visitors, plugin proxies, and the dashboard are not migrated |
| lucky_cloudflared.lkcf | Cloudflare Tunnel | Tunnels that use a tunnel token (with their API token); Access clients and tunnels that only have an API token are not migrated |
| lucky_base.lkcf | Global settings, Web Services | Intranet-only access, login failure limit, session duration, time zone, custom DNS (including the `doh@` syntax), GC parameters, and trusted PROXY protocol sources. The admin port, security entrance, administrator account, and two-factor authentication are not migrated |

Lucky's scheduled tasks, IP filters, and Docker settings are empty in typical configurations and are not migrated yet. If yours have content, configure them manually.

**Imported rules are disabled by default.** This prevents an import on a test machine from pointing production domains at the wrong address, or from competing with a still-running Lucky for ports and router UPnP mappings. Once you are sure Gately is running on the target device and Lucky has been stopped, you can check "Keep the enabled state from Lucky after import". Rules then run immediately according to their enabled state in Lucky (including each module's master switch).

## 3. Switching over

Switch one module at a time and verify each before moving on:

1. **Certificates**: Enable the certificate in Gately. Enabling writes the certificate to the mapped directory and runs the change script (with the same file names as Lucky), so make sure the directory is writable.
2. **DDNS**: Disable the task in Lucky → enable the matching task in Gately → click "Force sync" to confirm the records are updated.
3. **Port Forwarding / STUN**: First disable the rule in Lucky (to release the port and UPnP mapping), then enable it in Gately. After a STUN rule is enabled, the public port may change; scripts and webhooks are notified automatically.
4. **Web Services**: Disable Lucky's listener rule → enable it in Gately. HTTPS sites need an enabled certificate in the Certificates module that covers the domain.
5. Stop the Lucky container only after everything checks out.

## 4. Mounts for Docker deployments

Lucky's script and certificate mappings use paths inside the Lucky container. The Gately container must mount the same host directories at the same in-container paths. Compare against Lucky's compose file and add them one by one, for example:

```yaml
volumes:
  - ./config:/etc/gately
  - /var/run/docker.sock:/var/run/docker.sock
  # If Lucky maps certificates to /data/nginx/ssl, mount the same host directory at the same path
  - /srv/nginx/ssl:/data/nginx/ssl
  # Scripts called by rules (check that the path entered in Lucky really exists inside the Lucky container)
  - /srv/lucky/custom_scripts:/etc/lucky/custom_scripts:ro
```

The Gately image ships with `sh`, `bash`, and `curl`, so common notification scripts run as-is.

## 5. Items that need manual handling

- **Third-party sign-in**: Lucky's GitHub/QQ/Google sign-in goes through Lucky's official relay server. Gately uses OAuth apps that you register yourself instead. When importing, also select `lucky_thirdPartyAuthManager.lkcf` (the identity table), `lucky_reverseproxy.lkcf`, and `lucky_base.lkcf`:
  - Third-party identities referenced by sites and the admin console are migrated from the identity table into allow lists (GitHub uses the numeric ID, Google uses `sub`, QQ uses `openid`, and the username and email go into the remark).
  - The admin console's sign-in methods are under "Settings → Account & Security → Third-party sign-in", and the sites' sign-in methods are under "Web Services → Global settings". They take effect only after you enter your own Client ID/Secret and callback URL and enable them; import never enables them automatically. Use `github`, `google`, and `qq` as the provider identifiers so they match the migrated identities.
  - A QQ `openid` differs for every QQ Connect app. After switching to your own app, the first sign-in is rejected with the new account identifier. Add that identifier to the allow list.
  - Sites that depend solely on third-party sign-in and have no migratable identity stay disabled after import.
- **ZeroSSL certificates**: Gately reuses the EAB credentials from Lucky to register a new ACME account. If ZeroSSL rejects them, regenerate the EAB in the ZeroSSL dashboard.
- **frp instances**: Gately has exactly one frpc and one frps. Extra instances in Lucky are listed in the report; move them to other machines or merge them into the same instance. A frpc already configured in Gately to connect to another server, or an already enabled frps, is not overwritten. The frps dashboard account is managed by Lucky, so if you need the dashboard after migration, set the port and password again.
- **Storage references**: Lucky's FTP / WebDAV users can mount storage from "Storage Management". When importing, also select `lucky_storagemanagement.lkcf` (or import the whole-site zip directly). Local storage resolves to the corresponding directories. WebDAV storage, Alibaba Cloud Drive storage, and rclone mounts are not supported yet and are flagged in the report.
- **Lucky features not yet implemented**: DLNA, rclone cloud drive mounts, Alibaba Cloud Drive, and the IP database (`lucky_ipdb.lkcf`) are not migrated yet and are skipped during import.
- **Lucky's official servers**: Gately never connects to addresses such as `66666.host` or `lucky666.cn`. After migration, public IP lookups use third-party public services (changeable in the DDNS task).

## 6. Rolling back

Gately does not modify any of Lucky's files. To roll back, disable the corresponding rules in Gately and re-enable them in Lucky.
