# Gately Admin API

The Gately Admin API supports scripting and bulk automation.

Authentication: create a token on the "API Tokens" page of the admin console and send the header `Authorization: Bearer gtl_xxxx`.

Permissions: each token has only the permissions selected at creation, in the form `<module>:read` or `<module>:write`; write permission includes read. Account, two-factor authentication and token management endpoints, as well as backup, restore, restart and system settings endpoints, can be used by login sessions only.

Send the request body with `Content-Type: application/json`; errors are returned uniformly as `{"error": "message"}`.

Modules turned off under "Settings → Modules" serve no API: their endpoints return 403 and no longer appear in the API docs of the running instance.

Website: https://gately.cn

A machine-readable description is available in `openapi.json` (OpenAPI 3.0) in the same directory.

## Quick start

```bash
# View system status (requires status:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/status

# List all DDNS tasks (requires ddns:read)
curl -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks

# Sync a DDNS task immediately (requires ddns:write; take the key from the list returned in the previous step)
curl -X POST -H "Authorization: Bearer gtl_xxxx" http://localhost:16680/api/ddns/tasks/<key>/sync
```

## Permissions

| Permission domain | Name | Available permissions |
| --- | --- | --- |
| `portforward` | Port forwarding | `portforward:read`、`portforward:write` |
| `protoroute` | Protocol Routing | `protoroute:read`、`protoroute:write` |
| `ddns` | Dynamic DNS | `ddns:read`、`ddns:write` |
| `stun` | STUN traversal | `stun:read`、`stun:write` |
| `tailscale` | Tailscale networking | `tailscale:read`、`tailscale:write` |
| `frp` | frp tunneling | `frp:read`、`frp:write` |
| `cloudflared` | Cloudflare Tunnel | `cloudflared:read`、`cloudflared:write` |
| `ssl` | SSL certificate | `ssl:read`、`ssl:write` |
| `waf` | WAF firewall | `waf:read`、`waf:write` |
| `webservice` | Web service | `webservice:read`、`webservice:write` |
| `guard` | Intrusion Prevention | `guard:read`、`guard:write` |
| `wol` | Wake-on-LAN | `wol:read`、`wol:write` |
| `webterminal` | Web terminal | `webterminal:read`、`webterminal:write` |
| `cron` | Scheduled task | `cron:read`、`cron:write` |
| `docker` | Docker management | `docker:read`、`docker:write` |
| `webdav` | WebDAV Service | `webdav:read`、`webdav:write` |
| `ftp` | FTP service | `ftp:read`、`ftp:write` |
| `smb` | SMB Service | `smb:read`、`smb:write` |
| `filebrowser` | File Browser | `filebrowser:read`、`filebrowser:write` |
| `storage` | Storage Mounts | `storage:read`、`storage:write` |
| `notify` | Notification center | `notify:read`、`notify:write` |
| `nettools` | Network diagnostics | `nettools:read`、`nettools:write` |
| `backup` | Configuration backup | `backup:read`、`backup:write` |
| `status` | System status | `status:read` |
| `logs` | System logs | `logs:read` |
| `metrics` | Metrics (Prometheus) | `metrics:read` |
| `import` | Lucky configuration import | `import:write` |
| `fileusers` | File service shared users | `fileusers:read`、`fileusers:write` |

Wildcards: `*:read` means read-only access to all modules; `*:write` or `*` means read-write access to all modules.

## Port forwarding

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/portforward/logs` | `portforward:read` | View module runtime logs |
| DELETE | `/api/portforward/logs` | `portforward:write` | Clear module runtime logs |
| GET | `/api/portforward/rules` | `portforward:read` | List Port forwarding rule |
| POST | `/api/portforward/rules` | `portforward:write` | Create Port forwarding rule |
| PUT | `/api/portforward/rules/{key}` | `portforward:write` | Update Port forwarding rule |
| DELETE | `/api/portforward/rules/{key}` | `portforward:write` | Delete Port forwarding rule |
| PUT | `/api/portforward/rules/{key}/enable` | `portforward:write` | Enable or disable Port forwarding rule |
| PUT | `/api/portforward/rules-order` | `portforward:write` | Reorder Port forwarding rule |
| PUT | `/api/portforward/rules-group` | `portforward:write` | Set group in bulk: Port forwarding rule |
| GET | `/api/portforward/settings` | `portforward:read` | Read port forwarding global settings |
| PUT | `/api/portforward/settings` | `portforward:write` | Save port forwarding global settings |

## Protocol Routing

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/protoroute/logs` | `protoroute:read` | View module runtime logs |
| DELETE | `/api/protoroute/logs` | `protoroute:write` | Clear module runtime logs |
| GET | `/api/protoroute/rules` | `protoroute:read` | List protocol routing rule |
| POST | `/api/protoroute/rules` | `protoroute:write` | Create protocol routing rule |
| PUT | `/api/protoroute/rules/{key}` | `protoroute:write` | Update protocol routing rule |
| DELETE | `/api/protoroute/rules/{key}` | `protoroute:write` | Delete protocol routing rule |
| PUT | `/api/protoroute/rules/{key}/enable` | `protoroute:write` | Enable or disable protocol routing rule |
| PUT | `/api/protoroute/rules-order` | `protoroute:write` | Reorder protocol routing rule |
| PUT | `/api/protoroute/rules-group` | `protoroute:write` | Set group in bulk: protocol routing rule |
| GET | `/api/protoroute/settings` | `protoroute:read` | Get protocol routing settings |
| PUT | `/api/protoroute/settings` | `protoroute:write` | Save protocol routing settings |

## Dynamic DNS

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/ddns/logs` | `ddns:read` | View module runtime logs |
| DELETE | `/api/ddns/logs` | `ddns:write` | Clear module runtime logs |
| GET | `/api/ddns/tasks` | `ddns:read` | List DDNS task |
| POST | `/api/ddns/tasks` | `ddns:write` | Create DDNS task |
| PUT | `/api/ddns/tasks/{key}` | `ddns:write` | Update DDNS task |
| DELETE | `/api/ddns/tasks/{key}` | `ddns:write` | Delete DDNS task |
| PUT | `/api/ddns/tasks/{key}/enable` | `ddns:write` | Enable or disable DDNS task |
| PUT | `/api/ddns/tasks-order` | `ddns:write` | Reorder DDNS task |
| PUT | `/api/ddns/tasks-group` | `ddns:write` | Set group in bulk: DDNS task |
| GET | `/api/ddns/tasks/{key}/secrets` | `ddns:write` | View secrets in plain text |
| POST | `/api/ddns/tasks/{key}/sync` | `ddns:write` | Sync a DDNS task immediately |
| DELETE | `/api/ddns/tasks/{key}/records/{rec}` | `ddns:write` | Delete one record from a DDNS task |
| PUT | `/api/ddns/tasks/{key}/records-layout` | `ddns:write` | Reorder and group a task's DNS records |
| GET | `/api/ddns/providers` | `ddns:read` | List supported DNS providers and their required parameters |
| GET | `/api/ddns/netinterfaces` | `ddns:read` | List network interfaces and addresses on this machine |
| POST | `/api/ddns/iptest` | `ddns:write` | Test public IP lookup |
| GET | `/api/ddns/defaults` | `ddns:read` | Read DDNS defaults |
| GET | `/api/ddns/settings` | `ddns:read` | Read DDNS global settings |
| PUT | `/api/ddns/settings` | `ddns:write` | Save DDNS global settings |

## STUN traversal

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/stun/logs` | `stun:read` | View module runtime logs |
| DELETE | `/api/stun/logs` | `stun:write` | Clear module runtime logs |
| GET | `/api/stun/rules` | `stun:read` | List STUN rule |
| POST | `/api/stun/rules` | `stun:write` | Create STUN rule |
| PUT | `/api/stun/rules/{key}` | `stun:write` | Update STUN rule |
| DELETE | `/api/stun/rules/{key}` | `stun:write` | Delete STUN rule |
| PUT | `/api/stun/rules/{key}/enable` | `stun:write` | Enable or disable STUN rule |
| PUT | `/api/stun/rules-order` | `stun:write` | Reorder STUN rule |
| PUT | `/api/stun/rules-group` | `stun:write` | Set group in bulk: STUN rule |
| POST | `/api/stun/rules/{key}/restart` | `stun:write` | Restart a STUN rule |
| GET | `/api/stun/netinterfaces` | `stun:read` | List network interfaces and addresses on this machine |
| GET | `/api/stun/settings` | `stun:read` | Read STUN global settings |
| PUT | `/api/stun/settings` | `stun:write` | Save STUN global settings |

## Tailscale networking

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/tailscale/logs` | `tailscale:read` | View module runtime logs |
| DELETE | `/api/tailscale/logs` | `tailscale:write` | Clear module runtime logs |
| GET | `/api/tailscale/settings` | `tailscale:read` | Read Tailscale settings |
| PUT | `/api/tailscale/settings` | `tailscale:write` | Save Tailscale settings |
| GET | `/api/tailscale/secrets` | `tailscale:write` | View secrets in plain text |
| GET | `/api/tailscale/status` | `tailscale:read` | View Tailscale connection status |
| POST | `/api/tailscale/restart` | `tailscale:write` | Restart Tailscale |
| POST | `/api/tailscale/logout` | `tailscale:write` | Sign out of Tailscale |
| GET | `/api/tailscale/install` | `tailscale:read` | View the install info and progress of a managed program |
| POST | `/api/tailscale/install` | `tailscale:write` | Install or update a managed program online |
| POST | `/api/tailscale/install/rollback` | `tailscale:write` | Switch to a saved previous version |
| DELETE | `/api/tailscale/install` | `tailscale:write` | Uninstall a managed program |
| DELETE | `/api/tailscale/install/backups/{id}` | `tailscale:write` | Delete a saved previous version |
| POST | `/api/tailscale/reset` | `tailscale:write` | Restore the module default settings |

## frp tunneling

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/frp/logs` | `frp:read` | View module runtime logs |
| DELETE | `/api/frp/logs` | `frp:write` | Clear module runtime logs |
| GET | `/api/frp/clients` | `frp:read` | List frp client |
| POST | `/api/frp/clients` | `frp:write` | Create frp client |
| PUT | `/api/frp/clients/{key}` | `frp:write` | Update frp client |
| DELETE | `/api/frp/clients/{key}` | `frp:write` | Delete frp client |
| PUT | `/api/frp/clients/{key}/enable` | `frp:write` | Enable or disable frp client |
| PUT | `/api/frp/clients-order` | `frp:write` | Reorder frp client |
| PUT | `/api/frp/clients-group` | `frp:write` | Set group in bulk: frp client |
| GET | `/api/frp/clients/{key}/secrets` | `frp:write` | View secrets in plain text |
| GET | `/api/frp/servers` | `frp:read` | List frp server |
| POST | `/api/frp/servers` | `frp:write` | Create frp server |
| PUT | `/api/frp/servers/{key}` | `frp:write` | Update frp server |
| DELETE | `/api/frp/servers/{key}` | `frp:write` | Delete frp server |
| PUT | `/api/frp/servers/{key}/enable` | `frp:write` | Enable or disable frp server |
| PUT | `/api/frp/servers-order` | `frp:write` | Reorder frp server |
| PUT | `/api/frp/servers-group` | `frp:write` | Set group in bulk: frp server |
| GET | `/api/frp/servers/{key}/secrets` | `frp:write` | View secrets in plain text |
| GET | `/api/frp/proxies` | `frp:read` | List frp proxies |
| POST | `/api/frp/proxies` | `frp:write` | Create frp proxies |
| PUT | `/api/frp/proxies/{key}` | `frp:write` | Update frp proxies |
| DELETE | `/api/frp/proxies/{key}` | `frp:write` | Delete frp proxies |
| PUT | `/api/frp/proxies/{key}/enable` | `frp:write` | Enable or disable frp proxies |
| PUT | `/api/frp/proxies-order` | `frp:write` | Reorder frp proxies |
| PUT | `/api/frp/proxies-group` | `frp:write` | Set group in bulk: frp proxies |
| GET | `/api/frp/proxies/{key}/secrets` | `frp:write` | View secrets in plain text |
| POST | `/api/frp/import` | `frp:write` | Import an existing frpc / frps configuration (legacy ini and new toml are supported); preview only when apply is false |
| GET | `/api/frp/install` | `frp:read` | View the install info and progress of a managed program |
| POST | `/api/frp/install` | `frp:write` | Install or update a managed program online |
| POST | `/api/frp/install/rollback` | `frp:write` | Switch to a saved previous version |
| DELETE | `/api/frp/install` | `frp:write` | Uninstall a managed program |
| DELETE | `/api/frp/install/backups/{id}` | `frp:write` | Delete a saved previous version |
| POST | `/api/frp/clients/{key}/restart` | `frp:write` | Restart frpc instance |
| POST | `/api/frp/servers/{key}/restart` | `frp:write` | Restart frps instance |
| GET | `/api/frp/settings` | `frp:read` | Get program paths and versions |
| PUT | `/api/frp/settings` | `frp:write` | Save program paths |
| POST | `/api/frp/reset` | `frp:write` | Restore the module default settings |

## Cloudflare Tunnel

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/cloudflared/logs` | `cloudflared:read` | View module runtime logs |
| DELETE | `/api/cloudflared/logs` | `cloudflared:write` | Clear module runtime logs |
| GET | `/api/cloudflared/tunnels` | `cloudflared:read` | List Cloudflare Tunnel |
| POST | `/api/cloudflared/tunnels` | `cloudflared:write` | Create Cloudflare Tunnel |
| PUT | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Update Cloudflare Tunnel |
| DELETE | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Delete Cloudflare Tunnel |
| PUT | `/api/cloudflared/tunnels/{key}/enable` | `cloudflared:write` | Enable or disable Cloudflare Tunnel |
| PUT | `/api/cloudflared/tunnels-order` | `cloudflared:write` | Reorder Cloudflare Tunnel |
| PUT | `/api/cloudflared/tunnels-group` | `cloudflared:write` | Set group in bulk: Cloudflare Tunnel |
| GET | `/api/cloudflared/tunnels/{key}/secrets` | `cloudflared:write` | View secrets in plain text |
| POST | `/api/cloudflared/tunnels/{key}/restart` | `cloudflared:write` | Restart a Cloudflare Tunnel |
| GET | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:read` | Read the routes of a token tunnel |
| PUT | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:write` | Save the routes of a token tunnel |
| GET | `/api/cloudflared/settings` | `cloudflared:read` | Read Cloudflare Tunnel settings |
| PUT | `/api/cloudflared/settings` | `cloudflared:write` | Save Cloudflare Tunnel settings |
| GET | `/api/cloudflared/install` | `cloudflared:read` | View the install info and progress of a managed program |
| POST | `/api/cloudflared/install` | `cloudflared:write` | Install or update a managed program online |
| POST | `/api/cloudflared/install/rollback` | `cloudflared:write` | Switch to a saved previous version |
| DELETE | `/api/cloudflared/install` | `cloudflared:write` | Uninstall a managed program |
| DELETE | `/api/cloudflared/install/backups/{id}` | `cloudflared:write` | Delete a saved previous version |
| POST | `/api/cloudflared/reset` | `cloudflared:write` | Restore the module default settings |

## SSL certificate

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/ssl/logs` | `ssl:read` | View module runtime logs |
| DELETE | `/api/ssl/logs` | `ssl:write` | Clear module runtime logs |
| GET | `/api/ssl/certs` | `ssl:read` | List Certificate |
| POST | `/api/ssl/certs` | `ssl:write` | Create Certificate |
| PUT | `/api/ssl/certs/{key}` | `ssl:write` | Update Certificate |
| DELETE | `/api/ssl/certs/{key}` | `ssl:write` | Delete Certificate |
| PUT | `/api/ssl/certs/{key}/enable` | `ssl:write` | Enable or disable Certificate |
| PUT | `/api/ssl/certs-order` | `ssl:write` | Reorder Certificate |
| PUT | `/api/ssl/certs-group` | `ssl:write` | Set group in bulk: Certificate |
| GET | `/api/ssl/certs/{key}/secrets` | `ssl:write` | View secrets in plain text |
| POST | `/api/ssl/certs/{key}/renew` | `ssl:write` | Issue or renew a certificate immediately |
| GET | `/api/ssl/certs/{key}/download` | `ssl:write` | Download certificate and private key |
| POST | `/api/ssl/certs/{key}/deploy` | `ssl:write` | Deploy a certificate now |
| GET | `/api/ssl/deploy-targets` | `ssl:read` | List available deployment targets |
| GET | `/api/ssl/providers` | `ssl:read` | List supported DNS validation providers |
| GET | `/api/ssl/settings` | `ssl:read` | Read certificate global settings |
| PUT | `/api/ssl/settings` | `ssl:write` | Save certificate global settings |

## WAF firewall

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/waf/logs` | `waf:read` | View module runtime logs |
| DELETE | `/api/waf/logs` | `waf:write` | Clear module runtime logs |
| GET | `/api/waf/instances` | `waf:read` | List WAF instance |
| POST | `/api/waf/instances` | `waf:write` | Create WAF instance |
| PUT | `/api/waf/instances/{key}` | `waf:write` | Update WAF instance |
| DELETE | `/api/waf/instances/{key}` | `waf:write` | Delete WAF instance |
| PUT | `/api/waf/instances/{key}/enable` | `waf:write` | Enable or disable WAF instance |
| PUT | `/api/waf/instances-order` | `waf:write` | Reorder WAF instance |
| PUT | `/api/waf/instances-group` | `waf:write` | Set group in bulk: WAF instance |
| GET | `/api/waf/crs` | `waf:read` | List OWASP CRS rule files |
| GET | `/api/waf/crs/{file}` | `waf:read` | Read a CRS rule file |

## Web service

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/webservice/logs` | `webservice:read` | View module runtime logs |
| DELETE | `/api/webservice/logs` | `webservice:write` | Clear module runtime logs |
| GET | `/api/webservice/listeners` | `webservice:read` | List Web service listening |
| POST | `/api/webservice/listeners` | `webservice:write` | Create Web service listening |
| PUT | `/api/webservice/listeners/{key}` | `webservice:write` | Update Web service listening |
| DELETE | `/api/webservice/listeners/{key}` | `webservice:write` | Delete Web service listening |
| PUT | `/api/webservice/listeners/{key}/enable` | `webservice:write` | Enable or disable Web service listening |
| PUT | `/api/webservice/listeners-order` | `webservice:write` | Reorder Web service listening |
| GET | `/api/webservice/iplists` | `webservice:read` | List subscribed IP lists |
| POST | `/api/webservice/iplists` | `webservice:write` | Create Subscribed IP list |
| PUT | `/api/webservice/iplists/{key}` | `webservice:write` | Update Subscribed IP list |
| DELETE | `/api/webservice/iplists/{key}` | `webservice:write` | Delete Subscribed IP list |
| PUT | `/api/webservice/iplists/{key}/enable` | `webservice:write` | Enable or disable Subscribed IP list |
| PUT | `/api/webservice/iplists-order` | `webservice:write` | Reorder Subscribed IP list |
| POST | `/api/webservice/iplists/{key}/refresh` | `webservice:write` | Refresh a subscribed IP list now |
| GET | `/api/webservice/users` | `webservice:read` | Read shared users and the OIDC identity provider |
| PUT | `/api/webservice/users` | `webservice:write` | Save shared users and the OIDC identity provider, set sites in bulk |
| POST | `/api/webservice/import/nginx` | `webservice:write` | Import nginx config |
| GET | `/api/webservice/settings` | `webservice:read` | Read Web service global settings |
| GET | `/api/webservice/settings/secrets/totp` | `webservice:write` | View the TOTP secrets of shared users in plain text (indexed by username) |
| GET | `/api/webservice/settings/secrets/oauth` | `webservice:write` | View the Client Secrets of third-party logins in plain text (indexed by login method ID) |
| POST | `/api/webservice/totp-secret` | `webservice:write` | Generate a new TOTP key for a shared user (not saved; takes effect after saving settings) |
| PUT | `/api/webservice/settings` | `webservice:write` | Save Web service global settings |
| GET | `/api/webservice/stats` | `webservice:read` | Per-minute site statistics (?listener=&site=&minutes=) |
| GET | `/api/webservice/metrics` | `webservice:read` | Metrics in Prometheus text format |
| GET | `/api/webservice/bans` | `webservice:read` | Currently banned IPs |
| POST | `/api/webservice/bans` | `webservice:write` | Manually ban an IP |
| DELETE | `/api/webservice/bans` | `webservice:write` | Remove a ban (body is {"key": address}) |
| GET | `/api/webservice/geoip` | `webservice:read` | GeoIP database status |
| POST | `/api/webservice/geoip/upload` | `webservice:write` | Upload a GeoIP database (multipart field file, .mmdb) |
| POST | `/api/webservice/geoip/download` | `webservice:write` | Download a GeoIP database (.mmdb) from a URL |
| DELETE | `/api/webservice/geoip` | `webservice:write` | Remove the GeoIP database |

## Intrusion Prevention

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/guard/logs` | `guard:read` | View module runtime logs |
| DELETE | `/api/guard/logs` | `guard:write` | Clear module runtime logs |
| GET | `/api/guard/jails` | `guard:read` | List intrusion prevention rule |
| POST | `/api/guard/jails` | `guard:write` | Create intrusion prevention rule |
| PUT | `/api/guard/jails/{key}` | `guard:write` | Update intrusion prevention rule |
| DELETE | `/api/guard/jails/{key}` | `guard:write` | Delete intrusion prevention rule |
| PUT | `/api/guard/jails/{key}/enable` | `guard:write` | Enable or disable intrusion prevention rule |
| PUT | `/api/guard/jails-order` | `guard:write` | Reorder intrusion prevention rule |
| PUT | `/api/guard/jails-group` | `guard:write` | Set group in bulk: intrusion prevention rule |
| GET | `/api/guard/presets` | `guard:read` | Jail templates for common services |
| POST | `/api/guard/jails/test` | `guard:write` | Test filters against sample log lines |
| GET | `/api/guard/status` | `guard:read` | Firewall sync status and environment |
| GET | `/api/guard/settings` | `guard:read` | Global settings |
| PUT | `/api/guard/settings` | `guard:write` | Update global settings |
| GET | `/api/guard/bans` | `guard:read` | Currently banned IPs |
| POST | `/api/guard/bans` | `guard:write` | Ban an IP or network manually |
| DELETE | `/api/guard/bans` | `guard:write` | Remove a ban (body is {"key": address}) |
| GET | `/api/guard/firewall/rules` | `guard:read` | Rules Gately wrote to nftables (text) |

## Wake-on-LAN

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/wol/logs` | `wol:read` | View module runtime logs |
| DELETE | `/api/wol/logs` | `wol:write` | Clear module runtime logs |
| GET | `/api/wol/devices` | `wol:read` | List Wake a device |
| POST | `/api/wol/devices` | `wol:write` | Create Wake a device |
| PUT | `/api/wol/devices/{key}` | `wol:write` | Update Wake a device |
| DELETE | `/api/wol/devices/{key}` | `wol:write` | Delete Wake a device |
| PUT | `/api/wol/devices/{key}/enable` | `wol:write` | Enable or disable Wake a device |
| PUT | `/api/wol/devices-order` | `wol:write` | Reorder Wake a device |
| PUT | `/api/wol/devices-group` | `wol:write` | Set group in bulk: Wake a device |
| POST | `/api/wol/devices/{key}/wake` | `wol:write` | Send a Wake-on-LAN magic packet to a device |
| GET | `/api/wol/status` | `wol:read` | View device online status |

## Web terminal

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/webterminal/logs` | `webterminal:read` | View module runtime logs |
| DELETE | `/api/webterminal/logs` | `webterminal:write` | Clear module runtime logs |
| GET | `/api/webterminal/connections` | `webterminal:read` | List Terminal connection |
| POST | `/api/webterminal/connections` | `webterminal:write` | Create Terminal connection |
| PUT | `/api/webterminal/connections/{key}` | `webterminal:write` | Update Terminal connection |
| DELETE | `/api/webterminal/connections/{key}` | `webterminal:write` | Delete Terminal connection |
| PUT | `/api/webterminal/connections/{key}/enable` | `webterminal:write` | Enable or disable Terminal connection |
| PUT | `/api/webterminal/connections-order` | `webterminal:write` | Reorder Terminal connection |
| PUT | `/api/webterminal/connections-group` | `webterminal:write` | Set group in bulk: Terminal connection |
| GET | `/api/webterminal/connections/{key}/secrets` | `webterminal:write` | View secrets in plain text |
| DELETE | `/api/webterminal/connections/{key}/hostkey` | `webterminal:write` | Clear the recorded host key |
| POST | `/api/webterminal/connections/{key}/test` | `webterminal:write` | Test a saved connection |
| POST | `/api/webterminal/connections/test` | `webterminal:write` | Test temporary connection parameters |
| PUT | `/api/webterminal/connections/{key}/quickaccess` | `webterminal:write` | Set the quick access for a connection |
| GET | `/api/webterminal/credentials` | `webterminal:read` | List Entry |
| POST | `/api/webterminal/credentials` | `webterminal:write` | Create Entry |
| PUT | `/api/webterminal/credentials/{key}` | `webterminal:write` | Update Entry |
| DELETE | `/api/webterminal/credentials/{key}` | `webterminal:write` | Delete Entry |
| PUT | `/api/webterminal/credentials/{key}/enable` | `webterminal:write` | Enable or disable Entry |
| PUT | `/api/webterminal/credentials-order` | `webterminal:write` | Reorder Entry |
| GET | `/api/webterminal/credentials/{key}/secrets` | `webterminal:write` | View secrets in plain text |
| GET | `/api/webterminal/connections/{key}/ws` | `webterminal:write` | Open a terminal session (WebSocket) |
| GET | `/api/webterminal/sessions/{id}/ws` | `webterminal:write` | Attach to an existing terminal session (WebSocket) |
| GET | `/api/webterminal/sessions` | `webterminal:read` | List terminal sessions |
| GET | `/api/webterminal/sessions/{id}` | `webterminal:read` | View terminal session details |
| PUT | `/api/webterminal/sessions/{id}/remark` | `webterminal:write` | Modify the note of a terminal session |
| GET | `/api/webterminal/sessions/{id}/stats` | `webterminal:read` | View terminal session statistics |
| DELETE | `/api/webterminal/sessions/{id}` | `webterminal:write` | Close a terminal session |
| GET | `/api/webterminal/security` | Login sessions only | View terminal security settings status |
| POST | `/api/webterminal/security/agreement` | Login sessions only | Accept the terminal user agreement |
| POST | `/api/webterminal/security/2fa/setup` | Login sessions only | Generate a terminal two-factor secret |
| POST | `/api/webterminal/security/2fa/enable` | Login sessions only | Enable terminal two-factor authentication |
| POST | `/api/webterminal/security/2fa/disable` | Login sessions only | Disable terminal two-factor authentication |
| POST | `/api/webterminal/security/verify2fa` | Login sessions only | Verify a terminal two-factor code |
| GET | `/api/webterminal/security/check2fa` | Login sessions only | Check whether terminal two-factor verification has passed |
| GET | `/api/webterminal/sftp/{key}/list` | `webterminal:write` | List a remote directory |
| GET | `/api/webterminal/sftp/{key}/read` | `webterminal:write` | Read a remote file |
| POST | `/api/webterminal/sftp/{key}/write` | `webterminal:write` | Write a remote file |
| POST | `/api/webterminal/sftp/{key}/upload` | `webterminal:write` | Upload a file to a remote host |
| POST | `/api/webterminal/sftp/{key}/mkdir` | `webterminal:write` | Create a remote directory |
| POST | `/api/webterminal/sftp/{key}/touch` | `webterminal:write` | Create an empty remote file |
| POST | `/api/webterminal/sftp/{key}/rename` | `webterminal:write` | Rename or move a remote file |
| POST | `/api/webterminal/sftp/{key}/copy` | `webterminal:write` | Copy a remote file |
| POST | `/api/webterminal/sftp/{key}/chmod` | `webterminal:write` | Change remote file permissions |
| DELETE | `/api/webterminal/sftp/{key}/remove` | `webterminal:write` | Delete a remote file |
| POST | `/api/webterminal/sftp/{key}/compress` | `webterminal:write` | Compress files on a remote host |
| POST | `/api/webterminal/sftp/{key}/decompress` | `webterminal:write` | Extract files on a remote host |
| GET | `/api/webterminal/sftp/{key}/preview-archive` | `webterminal:write` | Preview the contents of a remote archive |
| GET | `/api/webterminal/settings` | `webterminal:read` | Read Web terminal settings |
| PUT | `/api/webterminal/settings` | `webterminal:write` | Save Web terminal settings |
| GET | `/api/webterminal/shells` | `webterminal:read` | List the shells available on this machine |
| GET | `/api/webterminal/globalshortcuts` | `webterminal:read` | Read global quick commands |
| PUT | `/api/webterminal/globalshortcuts` | `webterminal:write` | Save global quick commands |
| GET | `/api/webterminal/splitlayout` | `webterminal:read` | Read split-pane layout |
| PUT | `/api/webterminal/splitlayout` | `webterminal:write` | Save split-pane layout |
| DELETE | `/api/webterminal/splitlayout` | `webterminal:write` | Clear split-pane layout |

## Scheduled task

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/cron/logs` | `cron:read` | View module runtime logs |
| DELETE | `/api/cron/logs` | `cron:write` | Clear module runtime logs |
| GET | `/api/cron/tasks` | `cron:read` | List Scheduled task |
| POST | `/api/cron/tasks` | `cron:write` | Create Scheduled task |
| PUT | `/api/cron/tasks/{key}` | `cron:write` | Update Scheduled task |
| DELETE | `/api/cron/tasks/{key}` | `cron:write` | Delete Scheduled task |
| PUT | `/api/cron/tasks/{key}/enable` | `cron:write` | Enable or disable Scheduled task |
| PUT | `/api/cron/tasks-order` | `cron:write` | Reorder Scheduled task |
| PUT | `/api/cron/tasks-group` | `cron:write` | Set group in bulk: Scheduled task |
| POST | `/api/cron/tasks/{key}/run` | `cron:write` | Run a scheduled task immediately |
| POST | `/api/cron/parse` | `cron:write` | Parse and preview a Cron expression |

## Docker management

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/docker/logs` | `docker:read` | View module runtime logs |
| DELETE | `/api/docker/logs` | `docker:write` | Clear module runtime logs |
| GET | `/api/docker/tasks` | `docker:read` | List background tasks |
| GET | `/api/docker/tasks/{id}` | `docker:read` | View background task progress and output |
| DELETE | `/api/docker/tasks` | `docker:write` | Clean up finished background tasks |
| GET | `/api/docker/images` | `docker:read` | List images |
| POST | `/api/docker/images/pull` | `docker:write` | Pull an image |
| POST | `/api/docker/images/push` | `docker:write` | Push an image |
| POST | `/api/docker/images/search` | `docker:write` | Search images |
| POST | `/api/docker/images/load` | `docker:write` | Import an image file |
| POST | `/api/docker/images/build` | `docker:write` | Build an image |
| GET | `/api/docker/images/{id}` | `docker:read` | View image details |
| POST | `/api/docker/images/{id}/tag` | `docker:write` | Tag an image |
| DELETE | `/api/docker/images/{id}` | `docker:write` | Delete an image |
| GET | `/api/docker/images/{id}/export` | `docker:write` | Export an image |
| GET | `/api/docker/networks` | `docker:read` | List networks |
| POST | `/api/docker/networks` | `docker:write` | Create a network |
| DELETE | `/api/docker/networks/{id}` | `docker:write` | Delete a network |
| POST | `/api/docker/networks/{id}/connect` | `docker:write` | Connect a container to a network |
| POST | `/api/docker/networks/{id}/disconnect` | `docker:write` | Disconnect a container from a network |
| GET | `/api/docker/volumes` | `docker:read` | List volumes |
| POST | `/api/docker/volumes` | `docker:write` | Create a volume |
| DELETE | `/api/docker/volumes/{name}` | `docker:write` | Delete a volume |
| GET | `/api/docker/volumes/{name}/export` | `docker:write` | Export a volume |
| POST | `/api/docker/volumes/{name}/import` | `docker:write` | Import data into a volume |
| GET | `/api/docker/compose/projects` | `docker:read` | List Compose projects |
| POST | `/api/docker/compose/projects` | `docker:write` | Create a Compose project |
| POST | `/api/docker/compose/{project}/up` | `docker:write` | Start a Compose project |
| POST | `/api/docker/compose/{project}/pull` | `docker:write` | Pull Compose project images |
| POST | `/api/docker/compose/{project}/{action}` | `docker:write` | Run an action on a Compose project |
| GET | `/api/docker/compose/{project}/config` | `docker:write` | Read a Compose file |
| PUT | `/api/docker/compose/{project}/config` | `docker:write` | Save a Compose file |
| GET | `/api/docker/compose/{project}/logs` | `docker:read` | View Compose project logs |
| DELETE | `/api/docker/compose/{project}` | `docker:write` | Delete a Compose project |
| GET | `/api/docker/info` | `docker:read` | View Docker engine info |
| GET | `/api/docker/disk-usage` | `docker:read` | View Docker disk usage |
| POST | `/api/docker/prune` | `docker:write` | Clean up unused Docker resources |
| GET | `/api/docker/settings` | `docker:read` | Read Docker module settings |
| PUT | `/api/docker/settings` | `docker:write` | Save Docker module settings |
| GET | `/api/docker/settings/secrets` | `docker:write` | View registry passwords in plain text (keys are `registry:<registry address>`) |
| GET | `/api/docker/containers` | `docker:read` | List containers |
| POST | `/api/docker/containers` | `docker:write` | Create a container |
| POST | `/api/docker/containers/check-all` | `docker:write` | Check all containers for image updates |
| POST | `/api/docker/containers/upgrade-batch` | `docker:write` | Upgrade containers in bulk |
| POST | `/api/docker/images/upgrade-check` | `docker:write` | Check whether an image has a new version |
| GET | `/api/docker/containers/{id}` | `docker:read` | View container details |
| GET | `/api/docker/containers/{id}/secrets` | `docker:write` | View container environment variables and start command in plain text |
| DELETE | `/api/docker/containers/{id}` | `docker:write` | Delete a container |
| POST | `/api/docker/containers/{id}/edit` | `docker:write` | Modify container configuration and recreate it |
| POST | `/api/docker/containers/{id}/rename` | `docker:write` | Rename a container |
| POST | `/api/docker/containers/{id}/commit` | `docker:write` | Commit a container as an image |
| POST | `/api/docker/containers/{id}/check` | `docker:write` | Check whether a container image has updates |
| POST | `/api/docker/containers/{id}/upgrade` | `docker:write` | Upgrade a container image and recreate the container |
| POST | `/api/docker/containers/{id}/{action}` | `docker:write` | Run an action on a container |
| GET | `/api/docker/containers/{id}/logs` | `docker:read` | View container logs |
| GET | `/api/docker/containers/{id}/stats` | `docker:read` | View container resource usage |
| GET | `/api/docker/containers/{id}/processes` | `docker:read` | View processes inside a container |
| GET | `/api/docker/containers/{id}/files/list` | `docker:write` | List a directory inside a container |
| GET | `/api/docker/containers/{id}/files/read` | `docker:write` | Read a file inside a container |
| GET | `/api/docker/containers/{id}/files/download` | `docker:write` | Download a file from a container |
| POST | `/api/docker/containers/{id}/files/write` | `docker:write` | Write a file inside a container |
| POST | `/api/docker/containers/{id}/files/mkdir` | `docker:write` | Create a directory inside a container |
| POST | `/api/docker/containers/{id}/files/rename` | `docker:write` | Rename a file inside a container |
| POST | `/api/docker/containers/{id}/files/upload` | `docker:write` | Upload a file to a container |
| DELETE | `/api/docker/containers/{id}/files` | `docker:write` | Delete a file inside a container |
| GET | `/api/docker/containers/{id}/exec` | `docker:write` | Open a terminal inside a container (WebSocket) |

## WebDAV Service

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/webdav/logs` | `webdav:read` | View module runtime logs |
| DELETE | `/api/webdav/logs` | `webdav:write` | Clear module runtime logs |
| GET | `/api/webdav/settings` | `webdav:read` | Read WebDAV settings |
| GET | `/api/webdav/settings/secrets` | `webdav:write` | View local user passwords in plain text |
| GET | `/api/webdav/shared-users` | `webdav:read` | Get file service shared users (legacy endpoint) |
| PUT | `/api/webdav/shared-users` | `webdav:write,ftp:write,smb:write` | Save file service shared users (legacy endpoint) |
| PUT | `/api/webdav/settings` | `webdav:write` | Save WebDAV settings |

## FTP service

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/ftp/logs` | `ftp:read` | View module runtime logs |
| DELETE | `/api/ftp/logs` | `ftp:write` | Clear module runtime logs |
| GET | `/api/ftp/settings` | `ftp:read` | Read FTP settings |
| GET | `/api/ftp/settings/secrets` | `ftp:write` | View local user passwords in plain text |
| GET | `/api/ftp/shared-users` | `ftp:read` | Get file service shared users (legacy endpoint) |
| PUT | `/api/ftp/shared-users` | `webdav:write,ftp:write,smb:write` | Save file service shared users (legacy endpoint) |
| PUT | `/api/ftp/settings` | `ftp:write` | Save FTP settings |

## SMB Service

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/smb/logs` | `smb:read` | View module runtime logs |
| DELETE | `/api/smb/logs` | `smb:write` | Clear module runtime logs |
| GET | `/api/smb/settings` | `smb:read` | Get SMB settings |
| GET | `/api/smb/settings/secrets` | `smb:write` | View local user passwords in plain text |
| GET | `/api/smb/shared-users` | `smb:read` | Get file service shared users (legacy endpoint) |
| PUT | `/api/smb/shared-users` | `webdav:write,ftp:write,smb:write` | Save file service shared users (legacy endpoint) |
| PUT | `/api/smb/settings` | `smb:write` | Save SMB settings |

## File Browser

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/filebrowser/logs` | `filebrowser:read` | View module runtime logs |
| DELETE | `/api/filebrowser/logs` | `filebrowser:write` | Clear module runtime logs |
| GET | `/api/filebrowser/instances` | `filebrowser:read` | List File Browser instance |
| POST | `/api/filebrowser/instances` | `filebrowser:write` | Create File Browser instance |
| PUT | `/api/filebrowser/instances/{key}` | `filebrowser:write` | Update File Browser instance |
| DELETE | `/api/filebrowser/instances/{key}` | `filebrowser:write` | Delete File Browser instance |
| PUT | `/api/filebrowser/instances/{key}/enable` | `filebrowser:write` | Enable or disable File Browser instance |
| PUT | `/api/filebrowser/instances-order` | `filebrowser:write` | Reorder File Browser instance |
| PUT | `/api/filebrowser/instances-group` | `filebrowser:write` | Set group in bulk: File Browser instance |
| GET | `/api/filebrowser/instances/{key}/secrets` | `filebrowser:write` | View secrets in plain text |
| POST | `/api/filebrowser/instances/{key}/restart` | `filebrowser:write` | Restart a File Browser instance |
| GET | `/api/filebrowser/settings` | `filebrowser:read` | Read File Browser program settings |
| PUT | `/api/filebrowser/settings` | `filebrowser:write` | Save File Browser program settings |
| GET | `/api/filebrowser/install` | `filebrowser:read` | View the install info and progress of a managed program |
| POST | `/api/filebrowser/install` | `filebrowser:write` | Install or update a managed program online |
| POST | `/api/filebrowser/install/rollback` | `filebrowser:write` | Switch to a saved previous version |
| DELETE | `/api/filebrowser/install` | `filebrowser:write` | Uninstall a managed program |
| DELETE | `/api/filebrowser/install/backups/{id}` | `filebrowser:write` | Delete a saved previous version |
| POST | `/api/filebrowser/reset` | `filebrowser:write` | Restore the module default settings |

## Storage Mounts

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/storage/logs` | `storage:read` | View module runtime logs |
| DELETE | `/api/storage/logs` | `storage:write` | Clear module runtime logs |
| GET | `/api/storage/mounts` | `storage:read` | List Storage Mounts |
| POST | `/api/storage/mounts` | `storage:write` | Create Storage Mounts |
| PUT | `/api/storage/mounts/{key}` | `storage:write` | Update Storage Mounts |
| DELETE | `/api/storage/mounts/{key}` | `storage:write` | Delete Storage Mounts |
| PUT | `/api/storage/mounts/{key}/enable` | `storage:write` | Enable or disable Storage Mounts |
| PUT | `/api/storage/mounts-order` | `storage:write` | Reorder Storage Mounts |
| PUT | `/api/storage/mounts-group` | `storage:write` | Set group in bulk: Storage Mounts |
| GET | `/api/storage/mounts/{key}/secrets` | `storage:write` | View secrets in plain text |
| GET | `/api/storage/files/{key}/list` | `storage:write` | List a storage directory |
| GET | `/api/storage/files/{key}/download` | `storage:write` | Download a storage file |
| POST | `/api/storage/files/{key}/upload` | `storage:write` | Upload a file to storage |
| POST | `/api/storage/files/{key}/mkdir` | `storage:write` | Create a storage directory |
| POST | `/api/storage/files/{key}/rename` | `storage:write` | Rename or move a storage file |
| DELETE | `/api/storage/files/{key}` | `storage:write` | Delete a storage file |

## Notification center

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/notify/logs` | `notify:read` | View module runtime logs |
| DELETE | `/api/notify/logs` | `notify:write` | Clear module runtime logs |
| GET | `/api/notify/channels` | `notify:read` | List Notification channel |
| POST | `/api/notify/channels` | `notify:write` | Create Notification channel |
| PUT | `/api/notify/channels/{key}` | `notify:write` | Update Notification channel |
| DELETE | `/api/notify/channels/{key}` | `notify:write` | Delete Notification channel |
| PUT | `/api/notify/channels/{key}/enable` | `notify:write` | Enable or disable Notification channel |
| PUT | `/api/notify/channels-order` | `notify:write` | Reorder Notification channel |
| PUT | `/api/notify/channels-group` | `notify:write` | Set group in bulk: Notification channel |
| GET | `/api/notify/channels/{key}/secrets` | `notify:write` | View secrets in plain text |
| GET | `/api/notify/rules` | `notify:read` | List Notification rule |
| POST | `/api/notify/rules` | `notify:write` | Create Notification rule |
| PUT | `/api/notify/rules/{key}` | `notify:write` | Update Notification rule |
| DELETE | `/api/notify/rules/{key}` | `notify:write` | Delete Notification rule |
| PUT | `/api/notify/rules/{key}/enable` | `notify:write` | Enable or disable Notification rule |
| PUT | `/api/notify/rules-order` | `notify:write` | Reorder Notification rule |
| PUT | `/api/notify/rules-group` | `notify:write` | Set group in bulk: Notification rule |
| GET | `/api/notify/event-types` | `notify:read` | Event types that can be subscribed to |
| GET | `/api/notify/history` | `notify:read` | Recent delivery records |
| POST | `/api/notify/channels/{key}/test` | `notify:write` | Send a test notification to a channel |

## Network diagnostics

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/nettools/logs` | `nettools:read` | View module runtime logs |
| DELETE | `/api/nettools/logs` | `nettools:write` | Clear module runtime logs |
| POST | `/api/nettools/run` | `nettools:write` | Run a diagnostic tool and return a task (tool: ping / traceroute / tcp / dns / speed / scan) |
| GET | `/api/nettools/tasks` | `nettools:read` | Recent diagnostic tasks |
| GET | `/api/nettools/tasks/{id}` | `nettools:read` | Task details and output |
| DELETE | `/api/nettools/tasks/{id}` | `nettools:write` | Cancel a running task, or delete a finished one |
| GET | `/api/nettools/settings` | `nettools:read` | Diagnostic settings |
| PUT | `/api/nettools/settings` | `nettools:write` | Modify the probe blocklist |
| GET | `/api/nettools/networks` | `nettools:read` | Network ranges on this machine that can be used for LAN scanning |

## Configuration backup

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/backup/logs` | `backup:read` | View module runtime logs |
| DELETE | `/api/backup/logs` | `backup:write` | Clear module runtime logs |
| GET | `/api/backup/targets` | `backup:read` | List Backup destinations |
| POST | `/api/backup/targets` | `backup:write` | Create Backup destinations |
| PUT | `/api/backup/targets/{key}` | `backup:write` | Update Backup destinations |
| DELETE | `/api/backup/targets/{key}` | `backup:write` | Delete Backup destinations |
| PUT | `/api/backup/targets/{key}/enable` | `backup:write` | Enable or disable Backup destinations |
| PUT | `/api/backup/targets-order` | `backup:write` | Reorder Backup destinations |
| GET | `/api/backup/settings` | `backup:read` | Backup settings |
| PUT | `/api/backup/settings` | `backup:write` | Modify backup settings (send the placeholder for the passphrase to keep it unchanged) |
| GET | `/api/backup/secrets` | `backup:write` | View secrets in plain text |
| POST | `/api/backup/run` | `backup:write` | Run a backup now |
| GET | `/api/backup/status` | `backup:read` | Latest backup result and next run time |
| GET | `/api/backup/mounts` | `backup:read` | Storage mount points that can serve as backup destinations |
| GET | `/api/backup/local` | Login sessions only | List of local backup files |
| GET | `/api/backup/local/{name}` | Login sessions only | Download a local backup file |

## File service shared users

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/fileusers` | `fileusers:read` | Get file service shared users |
| PUT | `/api/fileusers` | `fileusers:write` | Save file service shared users |
| GET | `/api/fileusers/secrets` | `fileusers:write` | View shared user passwords in plain text |

## System

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/session` | Any valid token | View the current authenticated identity |
| GET | `/api/status` | `status:read` | View system and module status |
| GET | `/api/logs` | `logs:read` | View system logs |
| GET | `/api/system/settings` | Login sessions only | Read global settings |
| PUT | `/api/system/settings` | Login sessions only | Save global settings |
| GET | `/api/system/settings/secrets` | Login sessions only | View third-party login Client Secrets in plain text |
| GET | `/api/system/info` | `status:read` | View system information |
| GET | `/api/system/update` | `status:read` | Check for a new version |
| POST | `/api/system/upgrade` | Login sessions only | Upgrade online to the latest version |
| GET | `/api/system/upgrade/status` | Login sessions only | Show online upgrade progress |
| POST | `/api/system/upgrade/cancel` | Login sessions only | Cancel online upgrade |
| POST | `/api/system/upgrade/upload` | Login sessions only | Upgrade from an uploaded package |
| GET | `/api/system/backup` | Login sessions only | Download a configuration backup |
| GET | `/api/system/diagnostics` | Login sessions only | Export redacted diagnostics bundle |
| GET | `/api/audit` | Login sessions only | Query the audit log |
| GET | `/api/metrics` | `metrics:read` | Global Prometheus metrics |
| GET | `/api/history` | Login sessions only | List configuration history versions |
| GET | `/api/history/{name}/{id}/diff` | Login sessions only | Compare a history version with the current configuration |
| POST | `/api/history/{name}/{id}/restore` | Login sessions only | Roll a configuration back to a history version |
| POST | `/api/system/restore` | Login sessions only | Restore configuration from a backup |
| POST | `/api/system/restart` | Login sessions only | Restart Gately |
| POST | `/api/system/selfcheck` | Login sessions only | Network and configuration self-check |
| GET | `/api/system/onboarding` | Login sessions only | Whether first-run onboarding is pending |
| POST | `/api/system/onboarding/dismiss` | Login sessions only | Complete or skip first-run onboarding |
| GET | `/api/openapi.json` | Any valid token | Get the OpenAPI description of this API |

## Configuration import

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| POST | `/api/import/lucky` | `import:write` | Import Lucky configuration |

## API token

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/tokens` | Login sessions only | List API tokens |
| GET | `/api/tokens/scopes` | Login sessions only | List the permission domains that can be granted |
| POST | `/api/tokens` | Login sessions only | Create an API token |
| PUT | `/api/tokens/order` | Login sessions only | Reorder API tokens |
| PUT | `/api/tokens/group` | Login sessions only | Set the group of several API tokens |
| PUT | `/api/tokens/{id}` | Login sessions only | Update an API token's name, group, scopes, expiry and enabled state |
| POST | `/api/tokens/{id}/regenerate` | Login sessions only | Regenerate token plaintext |
| DELETE | `/api/tokens/{id}` | Login sessions only | Delete an API token |

## Account

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| PUT | `/api/account` | Login sessions only | Change the administrator username and password |
| PUT | `/api/account/ui-language` | Login sessions only | Record the admin console language |
| GET | `/api/account/passkeys` | Login sessions only | List registered Passkeys |
| POST | `/api/account/passkeys/begin` | Login sessions only | Start Passkey registration |
| POST | `/api/account/passkeys` | Login sessions only | Finish Passkey registration |
| DELETE | `/api/account/passkeys/{id}` | Login sessions only | Delete a Passkey |
| GET | `/api/account/users` | Login sessions only | List read-only accounts |
| POST | `/api/account/users` | Login sessions only | Create a read-only account |
| PUT | `/api/account/users/{name}` | Login sessions only | Reset a read-only account's password |
| DELETE | `/api/account/users/{name}` | Login sessions only | Delete a read-only account |
| GET | `/api/account/totp` | Login sessions only | View two-factor authentication status |
| POST | `/api/account/totp/setup` | Login sessions only | Generate a two-factor secret |
| POST | `/api/account/totp/enable` | Login sessions only | Enable two-factor authentication |
| POST | `/api/account/totp/disable` | Login sessions only | Disable two-factor authentication |

## Data structures

### backup.LocalFile

| Field | Type |
| --- | --- |
| `modTime` | Time (RFC 3339) |
| `name` | string |
| `size` | integer |

### backup.Result

| Field | Type |
| --- | --- |
| `error` | string |
| `file` | string |
| `ok` | boolean |
| `pruned` | integer |
| `target` | string |

### backup.Settings

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### backup.Status

| Field | Type |
| --- | --- |
| `lastRun` | Time (RFC 3339) |
| `manual` | boolean |
| `nextRun` | Time (RFC 3339) |
| `results` | Array of `backup.Result` |
| `running` | boolean |

### backup.Target

| Field | Type |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `key` | string |
| `mount` | string |
| `name` | string |

### backup.settingsView

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `hasPassword` | boolean |
| `keep` | integer |
| `localEnable` | boolean |
| `password` | string |
| `schedule` | string |

### banlist.Entry

| Field | Type |
| --- | --- |
| `count` | integer |
| `created` | Time (RFC 3339) |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | Time (RFC 3339) |

### cloudflared.Tunnel

| Field | Type |
| --- | --- |
| `apiToken` | string |
| `edgeIpVersion` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `mode` | string |
| `name` | string |
| `protocol` | string |
| `remark` | string |
| `service` | string |
| `token` | string |

### cron.Task

| Field | Type |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `group` | string |
| `http` | `webhook.Config` |
| `key` | string |
| `name` | string |
| `remark` | string |
| `spec` | string |
| `type` | string |

### crud.enableRequest

| Field | Type |
| --- | --- |
| `enable` | boolean |

### crud.groupRequest

| Field | Type |
| --- | --- |
| `group` | string |
| `keys` | Array of string |

### crud.orderRequest

| Field | Type |
| --- | --- |
| `keys` | Array of string |

### ddns.Record

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `line` | string |
| `remark` | string |
| `sub` | string |
| `ttl` | integer |
| `type` | string |
| `value` | string |
| `zone` | string |

### ddns.Task

| Field | Type |
| --- | --- |
| `credentials` | `dnsprovider.Credentials` |
| `enable` | boolean |
| `forceInterval` | integer |
| `group` | string |
| `httpTimeout` | integer |
| `interval` | integer |
| `ipv4` | `ipsource.Source` |
| `ipv6` | `ipsource.Source` |
| `key` | string |
| `name` | string |
| `provider` | string |
| `records` | Array of `ddns.Record` |
| `remark` | string |
| `webhook` | `webhook.Config` |

### ddns.recordLayout

| Field | Type |
| --- | --- |
| `group` | string |
| `key` | string |

### ddns.recordsLayoutRequest

| Field | Type |
| --- | --- |
| `records` | Array of `ddns.recordLayout` |

### dnsprovider.Credentials

| Field | Type |
| --- | --- |
| `extra` | Map of string |
| `id` | string |
| `secret` | string |

### events.TypeInfo

| Field | Type |
| --- | --- |
| `level` | string |
| `name` | string |
| `type` | string |

### filebrowser.Instance

| Field | Type |
| --- | --- |
| `accessUrls` | Array of string |
| `adminPassword` | string |
| `adminUser` | string |
| `authMethod` | string |
| `baseUrl` | string |
| `disableWebDav` | boolean |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenIp` | string |
| `locale` | string |
| `name` | string |
| `port` | integer |
| `proxyHeader` | string |
| `remark` | string |
| `sources` | Array of `filebrowser.Source` |
| `tokenHours` | integer |
| `trustProxyHeaders` | boolean |

### filebrowser.Source

| Field | Type |
| --- | --- |
| `name` | string |
| `path` | string |
| `readOnly` | boolean |

### frp.BinaryView

| Field | Type |
| --- | --- |
| `binary` | string |
| `error` | string |
| `version` | string |

### frp.Client

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `protocol` | string |
| `remark` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ClientSettings

| Field | Type |
| --- | --- |
| `protocol` | string |
| `serverAddr` | string |
| `serverPort` | integer |
| `token` | string |
| `user` | string |

### frp.ImportRequest

| Field | Type |
| --- | --- |
| `apply` | boolean |
| `content` | string |
| `name` | string |
| `proxyMode` | string |
| `role` | string |
| `target` | string |

### frp.ImportResponse

| Field | Type |
| --- | --- |
| `added` | integer |
| `applied` | boolean |
| `client` | `frp.ClientSettings` |
| `key` | string |
| `proxies` | Array of `frp.Proxy` |
| `role` | string |
| `server` | `frp.ServerSettings` |
| `warnings` | Array of string |

### frp.Proxy

| Field | Type |
| --- | --- |
| `client` | string |
| `customDomains` | Array of string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `localIp` | string |
| `localPort` | integer |
| `name` | string |
| `remark` | string |
| `remotePort` | integer |
| `secretKey` | string |
| `subdomain` | string |
| `type` | string |

### frp.Server

| Field | Type |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `proxyBindAddr` | string |
| `remark` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.ServerSettings

| Field | Type |
| --- | --- |
| `allowPorts` | string |
| `bindAddr` | string |
| `bindPort` | integer |
| `dashboardAddr` | string |
| `dashboardPort` | integer |
| `dashboardPwd` | string |
| `dashboardUser` | string |
| `proxyBindAddr` | string |
| `subdomainHost` | string |
| `token` | string |
| `vhostHttpPort` | integer |
| `vhostHttpsPort` | integer |

### frp.Settings

| Field | Type |
| --- | --- |
| `frpcPath` | string |
| `frpsPath` | string |

### frp.SettingsView

| Field | Type |
| --- | --- |
| `frpc` | `frp.BinaryView` |
| `frps` | `frp.BinaryView` |
| `settings` | `frp.Settings` |

### geoip.Info

| Field | Type |
| --- | --- |
| `buildTime` | Time (RFC 3339) |
| `loaded` | boolean |
| `size` | integer |
| `type` | string |

### guard.BanView

| Field | Type |
| --- | --- |
| `count` | integer |
| `created` | Time (RFC 3339) |
| `jail` | string |
| `key` | string |
| `reason` | string |
| `source` | string |
| `until` | Time (RFC 3339) |

### guard.FirewallStatus

| Field | Type |
| --- | --- |
| `active` | boolean |
| `checkedAt` | Time (RFC 3339) |
| `dropped` | Map of integer |
| `enabled` | boolean |
| `error` | string |
| `hits` | Map of integer |
| `openPorts` | integer |
| `reason` | string |
| `trapPorts` | string |
| `trapSkipped` | Array of integer |
| `version` | string |

### guard.Jail

| Field | Type |
| --- | --- |
| `anyStatus` | boolean |
| `banTime` | integer |
| `enable` | boolean |
| `filter` | string |
| `findTime` | integer |
| `group` | string |
| `ignore` | string |
| `key` | string |
| `maxRetry` | integer |
| `name` | string |
| `preset` | string |
| `remark` | string |
| `source` | string |
| `target` | string |
| `unknownHost` | boolean |

### guard.Platform

| Field | Type |
| --- | --- |
| `docker` | boolean |
| `inDocker` | boolean |
| `journal` | boolean |
| `os` | string |

### guard.ScanSettings

| Field | Type |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `excludePorts` | string |
| `threshold` | integer |

### guard.Settings

| Field | Type |
| --- | --- |
| `banSeconds` | integer |
| `firewall` | boolean |
| `ignoreList` | Array of string |
| `ignorePrivate` | boolean |
| `increment` | boolean |
| `maxBanSeconds` | integer |
| `scan` | `guard.ScanSettings` |
| `trap` | `guard.TrapSettings` |

### guard.Status

| Field | Type |
| --- | --- |
| `bans` | integer |
| `firewall` | `guard.FirewallStatus` |
| `platform` | `guard.Platform` |
| `userTrapPorts` | Array of integer |

### guard.TestResult

| Field | Type |
| --- | --- |
| `host` | string |
| `ignored` | boolean |
| `line` | string |
| `matched` | boolean |

### guard.TrapSettings

| Field | Type |
| --- | --- |
| `banSeconds` | integer |
| `enable` | boolean |
| `ports` | string |

### guard.banRequest

| Field | Type |
| --- | --- |
| `key` | string |
| `reason` | string |
| `seconds` | integer |

### guard.testRequest

| Field | Type |
| --- | --- |
| `filter` | string |
| `ignore` | string |
| `lines` | string |
| `source` | string |

### ipsource.Source

| Field | Type |
| --- | --- |
| `command` | string |
| `enable` | boolean |
| `interface` | string |
| `regex` | string |
| `type` | string |
| `urls` | Array of string |

### nettools.Request

| Field | Type |
| --- | --- |
| `cidr` | string |
| `count` | integer |
| `ports` | string |
| `seconds` | integer |
| `server` | string |
| `target` | string |
| `tool` | string |
| `type` | string |
| `url` | string |

### nettools.Settings

| Field | Type |
| --- | --- |
| `deny` | Array of string |

### nettools.Task

| Field | Type |
| --- | --- |
| `error` | string |
| `finished` | Time (RFC 3339) |
| `id` | string |
| `lines` | Array of string |
| `result` | Any |
| `started` | Time (RFC 3339) |
| `status` | string |
| `title` | string |
| `tool` | string |

### notify.Channel

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `from` | string |
| `group` | string |
| `host` | string |
| `key` | string |
| `kind` | string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `secret` | string |
| `security` | string |
| `target` | string |
| `url` | string |
| `user` | string |
| `webhook` | `webhook.Config` |

### notify.Record

| Field | Type |
| --- | --- |
| `channel` | string |
| `error` | string |
| `ok` | boolean |
| `rule` | string |
| `time` | Time (RFC 3339) |
| `title` | string |
| `type` | string |

### notify.Rule

| Field | Type |
| --- | --- |
| `channels` | Array of string |
| `enable` | boolean |
| `events` | Array of string |
| `group` | string |
| `key` | string |
| `minLevel` | string |
| `name` | string |
| `silence` | integer |

### portforward.Rule

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `mode` | string |
| `name` | string |
| `protocols` | Array of string |
| `proxyProtocol` | string |
| `remark` | string |
| `targetAddresses` | Array of string |
| `targetPorts` | string |
| `udpSessionTimeout` | integer |

### protoroute.Route

| Field | Type |
| --- | --- |
| `alpn` | Array of string |
| `domains` | Array of string |
| `name` | string |
| `protocol` | string |
| `proxyProtocol` | string |
| `targetAddresses` | Array of string |
| `targetPort` | integer |

### protoroute.Rule

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `listenAddress` | string |
| `listenPorts` | string |
| `maxConnections` | integer |
| `name` | string |
| `protocols` | Array of string |
| `proxyProtocol` | string |
| `remark` | string |
| `routes` | Array of `protoroute.Route` |
| `sniffTimeout` | number |
| `targetAddresses` | Array of string |
| `targetPorts` | string |

### ssl.ACMEConfig

| Field | Type |
| --- | --- |
| `ca` | string |
| `challenge` | string |
| `credentials` | `dnsprovider.Credentials` |
| `directoryUrl` | string |
| `disableAuthoritativeCheck` | boolean |
| `domains` | Array of string |
| `eabHmac` | string |
| `eabKid` | string |
| `email` | string |
| `httpPort` | integer |
| `keyType` | string |
| `network` | string |
| `profile` | string |
| `propagationTimeout` | integer |
| `provider` | string |
| `resolvers` | Array of string |
| `skipPropagationCheck` | boolean |
| `zone` | string |

### ssl.Cert

| Field | Type |
| --- | --- |
| `acme` | `ssl.ACMEConfig` |
| `certFile` | string |
| `certPem` | string |
| `deploys` | Array of `ssl.Deploy` |
| `enable` | boolean |
| `group` | string |
| `issuedProfile` | string |
| `issuerPem` | string |
| `key` | string |
| `keyFile` | string |
| `keyPem` | string |
| `mapping` | `ssl.Mapping` |
| `remark` | string |
| `source` | string |
| `updatedAt` | Time (RFC 3339) |

### ssl.Deploy

| Field | Type |
| --- | --- |
| `certName` | string |
| `command` | string |
| `connection` | string |
| `containers` | Array of string |
| `deployedAt` | Time (RFC 3339) |
| `deployedHash` | string |
| `dir` | string |
| `enable` | boolean |
| `keyName` | string |
| `remark` | string |
| `type` | string |

### ssl.Mapping

| Field | Type |
| --- | --- |
| `dir` | string |
| `enable` | boolean |
| `script` | string |

### storage.Mount

| Field | Type |
| --- | --- |
| `domain` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `name` | string |
| `password` | string |
| `path` | string |
| `readOnly` | boolean |
| `remark` | string |
| `skipVerify` | boolean |
| `type` | string |
| `url` | string |
| `username` | string |

### storage.MountInfo

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `readOnly` | boolean |
| `type` | string |

### stun.Bind

| Field | Type |
| --- | --- |
| `interface` | string |
| `ip` | string |
| `mode` | string |
| `regex` | string |

### stun.Rule

| Field | Type |
| --- | --- |
| `autoRetry` | boolean |
| `bind` | `stun.Bind` |
| `disableCheck` | boolean |
| `disableForward` | boolean |
| `enable` | boolean |
| `group` | string |
| `heartbeatInterval` | integer |
| `keepAliveServers` | Array of string |
| `key` | string |
| `localPort` | integer |
| `maxConnections` | integer |
| `name` | string |
| `natpmp` | boolean |
| `natpmpGateway` | string |
| `protocol` | string |
| `remark` | string |
| `retryInterval` | integer |
| `script` | string |
| `scriptEnable` | boolean |
| `stunServers` | Array of string |
| `stunTimeout` | integer |
| `targetAddresses` | Array of string |
| `targetPort` | integer |
| `udpSessionTimeout` | integer |
| `upnp` | boolean |
| `upnpControlUrl` | string |
| `upnpGateway` | string |
| `upnpInternalPort` | integer |
| `upnpLocalHost` | string |
| `useGlobalServers` | boolean |
| `useGlobalWebhook` | boolean |
| `webhook` | `webhook.Config` |
| `webhookOnlyOnChange` | boolean |

### thirdauth.Provider

| Field | Type |
| --- | --- |
| `clientId` | string |
| `clientSecret` | string |
| `issuer` | string |
| `key` | string |
| `name` | string |
| `proxy` | string |
| `type` | string |

### waf.Instance

| Field | Type |
| --- | --- |
| `bodyLimitAction` | string |
| `bodyLimitMB` | integer |
| `enable` | boolean |
| `group` | string |
| `inboundThreshold` | integer |
| `key` | string |
| `mode` | string |
| `name` | string |
| `outboundThreshold` | integer |
| `paranoia` | integer |
| `remark` | string |
| `ruleSets` | Array of `waf.RuleSet` |
| `skipBodyPaths` | string |
| `webdav` | boolean |

### waf.RuleSet

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `param` | string |
| `remark` | string |
| `type` | string |

### webhook.Config

| Field | Type |
| --- | --- |
| `body` | string |
| `enable` | boolean |
| `headers` | Array of string |
| `insecureSkipVerify` | boolean |
| `method` | string |
| `successContent` | Array of string |
| `timeout` | integer |
| `url` | string |

### webservice.AuthUser

| Field | Type |
| --- | --- |
| `displayName` | string |
| `email` | string |
| `groups` | Array of string |
| `id` | string |
| `name` | string |
| `password` | string |
| `remark` | string |
| `totpSecret` | string |

### webservice.BanInfo

| Field | Type |
| --- | --- |
| `key` | string |
| `manual` | boolean |
| `permanent` | boolean |
| `reason` | string |
| `source` | string |
| `until` | Time (RFC 3339) |

### webservice.CacheConfig

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `maxMb` | integer |
| `ttl` | integer |

### webservice.ForwardAuth

| Field | Type |
| --- | --- |
| `copyHeaders` | Array of string |
| `loginUrl` | string |
| `url` | string |

### webservice.HealthCheck

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `fails` | integer |
| `interval` | integer |
| `mode` | string |
| `passes` | integer |
| `path` | string |
| `timeout` | integer |

### webservice.IPList

| Field | Type |
| --- | --- |
| `count` | integer |
| `enable` | boolean |
| `key` | string |
| `name` | string |
| `refreshHours` | integer |
| `remark` | string |
| `updatedAt` | Time (RFC 3339) |
| `url` | string |

### webservice.Listener

| Field | Type |
| --- | --- |
| `acceptProxyProtocol` | boolean |
| `default` | `webservice.Site` |
| `enable` | boolean |
| `http3` | boolean |
| `key` | string |
| `listenIp` | string |
| `minTls` | string |
| `name` | string |
| `network` | string |
| `plainHttp` | string |
| `port` | integer |
| `remark` | string |
| `sites` | Array of `webservice.Site` |
| `tls` | boolean |

### webservice.Location

| Field | Type |
| --- | --- |
| `prefix` | string |
| `requestHeaders` | Array of string |
| `targets` | Array of string |

### webservice.OIDCClient

| Field | Type |
| --- | --- |
| `allow` | Array of string |
| `id` | string |
| `name` | string |
| `public` | boolean |
| `redirectUris` | Array of string |
| `remark` | string |
| `secret` | string |

### webservice.OIDCSettings

| Field | Type |
| --- | --- |
| `clients` | Array of `webservice.OIDCClient` |
| `enable` | boolean |
| `issuer` | string |
| `signingKey` | string |

### webservice.PathRule

| Field | Type |
| --- | --- |
| `auth` | string |
| `ipList` | Array of string |
| `ipMode` | string |
| `ipSets` | Array of string |
| `prefix` | string |

### webservice.Point

| Field | Type |
| --- | --- |
| `bytes` | integer |
| `requests` | integer |
| `s4xx` | integer |
| `s5xx` | integer |
| `t` | integer |

### webservice.RateLimit

| Field | Type |
| --- | --- |
| `burst` | integer |
| `enable` | boolean |
| `maxConns` | integer |
| `rate` | number |

### webservice.Rewrite

| Field | Type |
| --- | --- |
| `code` | integer |
| `pattern` | string |
| `redirect` | boolean |
| `to` | string |

### webservice.Settings

| Field | Type |
| --- | --- |
| `authSecret` | string |
| `authUsers` | Array of `webservice.AuthUser` |
| `autoBanSeconds` | integer |
| `enable` | boolean |
| `oauthCallbackBase` | string |
| `oauthProviders` | Array of `thirdauth.Provider` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `ssoDomain` | string |
| `trustedProxies` | Array of string |

### webservice.ShareLink

| Field | Type |
| --- | --- |
| `expiresAt` | Time (RFC 3339) |
| `key` | string |
| `maxUses` | integer |
| `name` | string |
| `pathPrefix` | string |
| `token` | string |

### webservice.Site

| Field | Type |
| --- | --- |
| `accessLog` | boolean |
| `authOptional` | boolean |
| `basicAuth` | boolean |
| `cache` | `webservice.CacheConfig` |
| `cleanUrls` | boolean |
| `compress` | boolean |
| `contentType` | string |
| `cors` | boolean |
| `countries` | Array of string |
| `countryMode` | string |
| `denyClose` | boolean |
| `disableKeepAlives` | boolean |
| `domains` | Array of string |
| `enable` | boolean |
| `forwardAuth` | `webservice.ForwardAuth` |
| `forwardedHeaders` | boolean |
| `group` | string |
| `health` | `webservice.HealthCheck` |
| `insecureSkipVerify` | boolean |
| `ipList` | Array of string |
| `ipMode` | string |
| `ipSets` | Array of string |
| `keepPath` | boolean |
| `key` | string |
| `lbPolicy` | string |
| `listDir` | boolean |
| `locations` | Array of `webservice.Location` |
| `name` | string |
| `notFoundPage` | string |
| `oauthUsers` | Array of string |
| `passAuthCookies` | boolean |
| `rateLimit` | `webservice.RateLimit` |
| `redirectCode` | integer |
| `redirectTo` | string |
| `remark` | string |
| `requestHeaders` | Array of string |
| `responseHeaders` | Array of string |
| `rewrites` | Array of `webservice.Rewrite` |
| `root` | string |
| `rules` | Array of `webservice.PathRule` |
| `sendProxyProtocol` | boolean |
| `shareLinks` | Array of `webservice.ShareLink` |
| `sharedUsers` | boolean |
| `statusCode` | integer |
| `targets` | Array of string |
| `text` | string |
| `type` | string |
| `upstreamSniFromHost` | boolean |
| `useTargetHost` | boolean |
| `userHeader` | string |
| `userRoutes` | Array of `webservice.UserRoute` |
| `users` | Array of string |
| `waf` | string |
| `webAuth` | boolean |

### webservice.SiteSharedUsers

| Field | Type |
| --- | --- |
| `listener` | string |
| `sharedUsers` | boolean |
| `site` | string |

### webservice.SiteStatsView

| Field | Type |
| --- | --- |
| `bytesOut` | integer |
| `series` | Array of `webservice.Point` |
| `status` | Map of integer |
| `topCountries` | Array of `webservice.TopItem` |
| `topIps` | Array of `webservice.TopItem` |
| `upstreams` | Array of `webservice.UpstreamStatus` |

### webservice.TopItem

| Field | Type |
| --- | --- |
| `count` | integer |
| `key` | string |

### webservice.UpstreamStatus

| Field | Type |
| --- | --- |
| `backup` | boolean |
| `conns` | integer |
| `healthy` | boolean |
| `url` | string |
| `weight` | integer |

### webservice.UserRoute

| Field | Type |
| --- | --- |
| `targets` | Array of string |
| `users` | Array of string |

### webservice.UserSite

| Field | Type |
| --- | --- |
| `basicAuth` | boolean |
| `default` | boolean |
| `domains` | Array of string |
| `enable` | boolean |
| `forwardAuth` | boolean |
| `group` | string |
| `listener` | string |
| `listenerEnable` | boolean |
| `listenerName` | string |
| `name` | string |
| `otherAuth` | boolean |
| `port` | integer |
| `sharedUsers` | boolean |
| `site` | string |
| `tls` | boolean |
| `type` | string |
| `webAuth` | boolean |

### webservice.UsersRequest

| Field | Type |
| --- | --- |
| `authUsers` | Array of `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | Array of `webservice.SiteSharedUsers` |
| `ssoDomain` | string |

### webservice.UsersView

| Field | Type |
| --- | --- |
| `authUsers` | Array of `webservice.AuthUser` |
| `oidc` | `webservice.OIDCSettings` |
| `selfServicePassword` | boolean |
| `sites` | Array of `webservice.UserSite` |
| `ssoDomain` | string |

### webservice.banRequest

| Field | Type |
| --- | --- |
| `key` | string |
| `seconds` | integer |

### webservice.geoipDownload

| Field | Type |
| --- | --- |
| `url` | string |

### webservice.totpRequest

| Field | Type |
| --- | --- |
| `name` | string |

### webservice.totpResponse

| Field | Type |
| --- | --- |
| `secret` | string |
| `url` | string |

### webterminal.Connection

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `local` | `webterminal.LocalConfig` |
| `name` | string |
| `quickAccessDirs` | Array of string |
| `remark` | string |
| `ssh` | `webterminal.SSHConfig` |
| `telnet` | `webterminal.TelnetConfig` |
| `type` | string |

### webterminal.Credential

| Field | Type |
| --- | --- |
| `authType` | string |
| `enable` | boolean |
| `key` | string |
| `keyFile` | string |
| `name` | string |
| `passphrase` | string |
| `password` | string |
| `privateKey` | string |
| `remark` | string |
| `username` | string |

### webterminal.LocalConfig

| Field | Type |
| --- | --- |
| `env` | Map of string |
| `shell` | string |
| `workDir` | string |

### webterminal.Proxy

| Field | Type |
| --- | --- |
| `enabled` | boolean |
| `host` | string |
| `password` | string |
| `port` | integer |
| `type` | string |
| `username` | string |

### webterminal.SSHConfig

| Field | Type |
| --- | --- |
| `authType` | string |
| `credentialKey` | string |
| `host` | string |
| `hostKey` | string |
| `keepAlive` | integer |
| `keyFile` | string |
| `passphrase` | string |
| `password` | string |
| `port` | integer |
| `privateKey` | string |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |
| `workDir` | string |

### webterminal.TelnetConfig

| Field | Type |
| --- | --- |
| `host` | string |
| `password` | string |
| `port` | integer |
| `proxy` | `webterminal.Proxy` |
| `timeout` | integer |
| `username` | string |

### wol.Device

| Field | Type |
| --- | --- |
| `broadcastIps` | Array of string |
| `checkAddr` | string |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `macs` | Array of string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `repeat` | integer |

