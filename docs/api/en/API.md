# Gately Admin API

The Gately Admin API supports scripting and bulk automation.

Authentication: create a token on the "API Tokens" page of the admin console and send the header `Authorization: Bearer gtl_xxxx`.

Permissions: each token has only the permissions selected at creation, in the form `<module>:read` or `<module>:write`; write permission includes read. Account, two-factor authentication and token management endpoints, as well as backup, restore, restart and system settings endpoints, can be used by login sessions only.

Send the request body with `Content-Type: application/json`; errors are returned uniformly as `{"error": "message"}`.

Modules turned off under "Settings → Modules" serve no API: their endpoints return 403 and no longer appear in the API docs of the running instance. Modules not compiled into a custom build (build.gately.cn) return 404 on their endpoints, saying the module is not included in this build, and are likewise left out of the API docs of the running instance.

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
| `netbird` | NetBird network | `netbird:read`、`netbird:write` |
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
| `perf` | Performance | `perf:read`、`perf:write` |
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
| GET | `/api/portforward/logs/sources` | `portforward:read` | List all sources of module logs |
| GET | `/api/portforward/logs/export` | `portforward:read` | Export module logs |
| DELETE | `/api/portforward/logs` | `portforward:write` | Clear module runtime logs |
| GET | `/api/portforward/rules` | `portforward:read` | List Port forwarding rule |
| POST | `/api/portforward/rules` | `portforward:write` | Create Port forwarding rule |
| POST | `/api/portforward/rules/{key}/copy` | `portforward:write` | Duplicate Port forwarding rule |
| PUT | `/api/portforward/rules/{key}` | `portforward:write` | Update Port forwarding rule |
| DELETE | `/api/portforward/rules/{key}` | `portforward:write` | Delete Port forwarding rule |
| PUT | `/api/portforward/rules/{key}/enable` | `portforward:write` | Enable or disable Port forwarding rule |
| PUT | `/api/portforward/rules-order` | `portforward:write` | Reorder Port forwarding rule |
| PUT | `/api/portforward/rules-enable` | `portforward:write` | Enable or disable in bulk: Port forwarding rule |
| POST | `/api/portforward/rules-delete` | `portforward:write` | Delete in bulk: Port forwarding rule |
| PUT | `/api/portforward/rules-group` | `portforward:write` | Set group in bulk: Port forwarding rule |
| GET | `/api/portforward/settings` | `portforward:read` | Read port forwarding global settings |
| PUT | `/api/portforward/settings` | `portforward:write` | Save port forwarding global settings |

## Protocol Routing

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/protoroute/logs` | `protoroute:read` | View module runtime logs |
| GET | `/api/protoroute/logs/sources` | `protoroute:read` | List all sources of module logs |
| GET | `/api/protoroute/logs/export` | `protoroute:read` | Export module logs |
| DELETE | `/api/protoroute/logs` | `protoroute:write` | Clear module runtime logs |
| GET | `/api/protoroute/rules` | `protoroute:read` | List protocol routing rule |
| POST | `/api/protoroute/rules` | `protoroute:write` | Create protocol routing rule |
| POST | `/api/protoroute/rules/{key}/copy` | `protoroute:write` | Duplicate protocol routing rule |
| PUT | `/api/protoroute/rules/{key}` | `protoroute:write` | Update protocol routing rule |
| DELETE | `/api/protoroute/rules/{key}` | `protoroute:write` | Delete protocol routing rule |
| PUT | `/api/protoroute/rules/{key}/enable` | `protoroute:write` | Enable or disable protocol routing rule |
| PUT | `/api/protoroute/rules-order` | `protoroute:write` | Reorder protocol routing rule |
| PUT | `/api/protoroute/rules-enable` | `protoroute:write` | Enable or disable in bulk: protocol routing rule |
| POST | `/api/protoroute/rules-delete` | `protoroute:write` | Delete in bulk: protocol routing rule |
| PUT | `/api/protoroute/rules-group` | `protoroute:write` | Set group in bulk: protocol routing rule |
| GET | `/api/protoroute/settings` | `protoroute:read` | Get protocol routing settings |
| PUT | `/api/protoroute/settings` | `protoroute:write` | Save protocol routing settings |

## Dynamic DNS

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/ddns/logs` | `ddns:read` | View module runtime logs |
| GET | `/api/ddns/logs/sources` | `ddns:read` | List all sources of module logs |
| GET | `/api/ddns/logs/export` | `ddns:read` | Export module logs |
| DELETE | `/api/ddns/logs` | `ddns:write` | Clear module runtime logs |
| GET | `/api/ddns/tasks` | `ddns:read` | List DDNS task |
| POST | `/api/ddns/tasks` | `ddns:write` | Create DDNS task |
| POST | `/api/ddns/tasks/{key}/copy` | `ddns:write` | Duplicate DDNS task |
| PUT | `/api/ddns/tasks/{key}` | `ddns:write` | Update DDNS task |
| DELETE | `/api/ddns/tasks/{key}` | `ddns:write` | Delete DDNS task |
| PUT | `/api/ddns/tasks/{key}/enable` | `ddns:write` | Enable or disable DDNS task |
| PUT | `/api/ddns/tasks-order` | `ddns:write` | Reorder DDNS task |
| PUT | `/api/ddns/tasks-enable` | `ddns:write` | Enable or disable in bulk: DDNS task |
| POST | `/api/ddns/tasks-delete` | `ddns:write` | Delete in bulk: DDNS task |
| PUT | `/api/ddns/tasks-group` | `ddns:write` | Set group in bulk: DDNS task |
| GET | `/api/ddns/tasks/{key}/secrets` | `ddns:write` | View secrets in plain text |
| POST | `/api/ddns/tasks/{key}/sync` | `ddns:write` | Sync a DDNS task immediately |
| DELETE | `/api/ddns/tasks/{key}/records/{rec}` | `ddns:write` | Delete one record from a DDNS task |
| POST | `/api/ddns/tasks/{key}/remote-delete` | `ddns:write` | Delete the DNS records of a DDNS task at the DNS provider |
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
| GET | `/api/stun/logs/sources` | `stun:read` | List all sources of module logs |
| GET | `/api/stun/logs/export` | `stun:read` | Export module logs |
| DELETE | `/api/stun/logs` | `stun:write` | Clear module runtime logs |
| GET | `/api/stun/rules` | `stun:read` | List STUN rule |
| POST | `/api/stun/rules` | `stun:write` | Create STUN rule |
| POST | `/api/stun/rules/{key}/copy` | `stun:write` | Duplicate STUN rule |
| PUT | `/api/stun/rules/{key}` | `stun:write` | Update STUN rule |
| DELETE | `/api/stun/rules/{key}` | `stun:write` | Delete STUN rule |
| PUT | `/api/stun/rules/{key}/enable` | `stun:write` | Enable or disable STUN rule |
| PUT | `/api/stun/rules-order` | `stun:write` | Reorder STUN rule |
| PUT | `/api/stun/rules-enable` | `stun:write` | Enable or disable in bulk: STUN rule |
| POST | `/api/stun/rules-delete` | `stun:write` | Delete in bulk: STUN rule |
| PUT | `/api/stun/rules-group` | `stun:write` | Set group in bulk: STUN rule |
| POST | `/api/stun/rules/{key}/restart` | `stun:write` | Restart a STUN rule |
| GET | `/api/stun/netinterfaces` | `stun:read` | List network interfaces and addresses on this machine |
| GET | `/api/stun/settings` | `stun:read` | Read STUN global settings |
| PUT | `/api/stun/settings` | `stun:write` | Save STUN global settings |

## Tailscale networking

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/tailscale/logs` | `tailscale:read` | View module runtime logs |
| GET | `/api/tailscale/logs/sources` | `tailscale:read` | List all sources of module logs |
| GET | `/api/tailscale/logs/export` | `tailscale:read` | Export module logs |
| DELETE | `/api/tailscale/logs` | `tailscale:write` | Clear module runtime logs |
| GET | `/api/tailscale/instances` | `tailscale:read` | List Tailscale instance |
| POST | `/api/tailscale/instances` | `tailscale:write` | Add a Tailscale instance |
| POST | `/api/tailscale/instances/{key}/copy` | `tailscale:write` | Duplicate Tailscale instance |
| PUT | `/api/tailscale/instances/{key}` | `tailscale:write` | Update Tailscale instance |
| DELETE | `/api/tailscale/instances/{key}` | `tailscale:write` | Delete Tailscale instance |
| PUT | `/api/tailscale/instances/{key}/enable` | `tailscale:write` | Enable or disable Tailscale instance |
| PUT | `/api/tailscale/instances-order` | `tailscale:write` | Reorder Tailscale instance |
| PUT | `/api/tailscale/instances-enable` | `tailscale:write` | Enable or disable in bulk: Tailscale instance |
| POST | `/api/tailscale/instances-delete` | `tailscale:write` | Delete in bulk: Tailscale instance |
| PUT | `/api/tailscale/instances-group` | `tailscale:write` | Set group in bulk: Tailscale instance |
| GET | `/api/tailscale/instances/{key}/secrets` | `tailscale:write` | View secrets in plain text |
| GET | `/api/tailscale/status` | `tailscale:read` | View the connection status of all Tailscale instances |
| GET | `/api/tailscale/instances/{key}/status` | `tailscale:read` | View the connection status of a Tailscale instance |
| POST | `/api/tailscale/instances/{key}/restart` | `tailscale:write` | Restart a Tailscale instance |
| POST | `/api/tailscale/instances/{key}/logout` | `tailscale:write` | Sign a Tailscale instance out |
| GET | `/api/tailscale/settings` | `tailscale:read` | Read Tailscale program settings |
| PUT | `/api/tailscale/settings` | `tailscale:write` | Save Tailscale program settings |
| GET | `/api/tailscale/install` | `tailscale:read` | View the install info and progress of a managed program |
| POST | `/api/tailscale/install` | `tailscale:write` | Install or update a managed program online |
| POST | `/api/tailscale/install/check` | `tailscale:write` | Check a managed program for updates |
| POST | `/api/tailscale/install/rollback` | `tailscale:write` | Switch to a saved previous version |
| DELETE | `/api/tailscale/install` | `tailscale:write` | Uninstall a managed program |
| DELETE | `/api/tailscale/install/backups/{id}` | `tailscale:write` | Delete a saved previous version |
| POST | `/api/tailscale/reset` | `tailscale:write` | Restore the module default settings |

## NetBird network

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/netbird/logs` | `netbird:read` | View module runtime logs |
| GET | `/api/netbird/logs/sources` | `netbird:read` | List all sources of module logs |
| GET | `/api/netbird/logs/export` | `netbird:read` | Export module logs |
| DELETE | `/api/netbird/logs` | `netbird:write` | Clear module runtime logs |
| GET | `/api/netbird/instances` | `netbird:read` | List NetBird instance |
| POST | `/api/netbird/instances` | `netbird:write` | Add a NetBird instance |
| POST | `/api/netbird/instances/{key}/copy` | `netbird:write` | Duplicate NetBird instance |
| PUT | `/api/netbird/instances/{key}` | `netbird:write` | Update NetBird instance |
| DELETE | `/api/netbird/instances/{key}` | `netbird:write` | Delete NetBird instance |
| PUT | `/api/netbird/instances/{key}/enable` | `netbird:write` | Enable or disable NetBird instance |
| PUT | `/api/netbird/instances-order` | `netbird:write` | Reorder NetBird instance |
| PUT | `/api/netbird/instances-enable` | `netbird:write` | Enable or disable in bulk: NetBird instance |
| POST | `/api/netbird/instances-delete` | `netbird:write` | Delete in bulk: NetBird instance |
| PUT | `/api/netbird/instances-group` | `netbird:write` | Set group in bulk: NetBird instance |
| GET | `/api/netbird/instances/{key}/secrets` | `netbird:write` | View secrets in plain text |
| GET | `/api/netbird/status` | `netbird:read` | View the connection status of all NetBird instances |
| GET | `/api/netbird/instances/{key}/status` | `netbird:read` | View the connection status of a NetBird instance |
| POST | `/api/netbird/instances/{key}/restart` | `netbird:write` | Restart a NetBird instance |
| POST | `/api/netbird/instances/{key}/logout` | `netbird:write` | Log out of a NetBird instance |
| GET | `/api/netbird/settings` | `netbird:read` | Get NetBird program settings |
| PUT | `/api/netbird/settings` | `netbird:write` | Save NetBird program settings |
| GET | `/api/netbird/install` | `netbird:read` | View the install info and progress of a managed program |
| POST | `/api/netbird/install` | `netbird:write` | Install or update a managed program online |
| POST | `/api/netbird/install/check` | `netbird:write` | Check a managed program for updates |
| POST | `/api/netbird/install/rollback` | `netbird:write` | Switch to a saved previous version |
| DELETE | `/api/netbird/install` | `netbird:write` | Uninstall a managed program |
| DELETE | `/api/netbird/install/backups/{id}` | `netbird:write` | Delete a saved previous version |
| POST | `/api/netbird/reset` | `netbird:write` | Restore the module default settings |

## frp tunneling

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/frp/logs` | `frp:read` | View module runtime logs |
| GET | `/api/frp/logs/sources` | `frp:read` | List all sources of module logs |
| GET | `/api/frp/logs/export` | `frp:read` | Export module logs |
| DELETE | `/api/frp/logs` | `frp:write` | Clear module runtime logs |
| GET | `/api/frp/clients` | `frp:read` | List frp client |
| POST | `/api/frp/clients` | `frp:write` | Create frp client |
| POST | `/api/frp/clients/{key}/copy` | `frp:write` | Duplicate frp client |
| PUT | `/api/frp/clients/{key}` | `frp:write` | Update frp client |
| DELETE | `/api/frp/clients/{key}` | `frp:write` | Delete frp client |
| PUT | `/api/frp/clients/{key}/enable` | `frp:write` | Enable or disable frp client |
| PUT | `/api/frp/clients-order` | `frp:write` | Reorder frp client |
| PUT | `/api/frp/clients-enable` | `frp:write` | Enable or disable in bulk: frp client |
| POST | `/api/frp/clients-delete` | `frp:write` | Delete in bulk: frp client |
| PUT | `/api/frp/clients-group` | `frp:write` | Set group in bulk: frp client |
| GET | `/api/frp/clients/{key}/secrets` | `frp:write` | View secrets in plain text |
| GET | `/api/frp/servers` | `frp:read` | List frp server |
| POST | `/api/frp/servers` | `frp:write` | Create frp server |
| POST | `/api/frp/servers/{key}/copy` | `frp:write` | Duplicate frp server |
| PUT | `/api/frp/servers/{key}` | `frp:write` | Update frp server |
| DELETE | `/api/frp/servers/{key}` | `frp:write` | Delete frp server |
| PUT | `/api/frp/servers/{key}/enable` | `frp:write` | Enable or disable frp server |
| PUT | `/api/frp/servers-order` | `frp:write` | Reorder frp server |
| PUT | `/api/frp/servers-enable` | `frp:write` | Enable or disable in bulk: frp server |
| POST | `/api/frp/servers-delete` | `frp:write` | Delete in bulk: frp server |
| PUT | `/api/frp/servers-group` | `frp:write` | Set group in bulk: frp server |
| GET | `/api/frp/servers/{key}/secrets` | `frp:write` | View secrets in plain text |
| GET | `/api/frp/proxies` | `frp:read` | List frp proxies |
| POST | `/api/frp/proxies` | `frp:write` | Create frp proxies |
| POST | `/api/frp/proxies/{key}/copy` | `frp:write` | Duplicate frp proxies |
| PUT | `/api/frp/proxies/{key}` | `frp:write` | Update frp proxies |
| DELETE | `/api/frp/proxies/{key}` | `frp:write` | Delete frp proxies |
| PUT | `/api/frp/proxies/{key}/enable` | `frp:write` | Enable or disable frp proxies |
| PUT | `/api/frp/proxies-order` | `frp:write` | Reorder frp proxies |
| PUT | `/api/frp/proxies-enable` | `frp:write` | Enable or disable in bulk: frp proxies |
| POST | `/api/frp/proxies-delete` | `frp:write` | Delete in bulk: frp proxies |
| PUT | `/api/frp/proxies-group` | `frp:write` | Set group in bulk: frp proxies |
| GET | `/api/frp/proxies/{key}/secrets` | `frp:write` | View secrets in plain text |
| POST | `/api/frp/import` | `frp:write` | Import an existing frpc / frps configuration (legacy ini and new toml are supported); preview only when apply is false |
| GET | `/api/frp/install` | `frp:read` | View the install info and progress of a managed program |
| POST | `/api/frp/install` | `frp:write` | Install or update a managed program online |
| POST | `/api/frp/install/check` | `frp:write` | Check a managed program for updates |
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
| GET | `/api/cloudflared/logs/sources` | `cloudflared:read` | List all sources of module logs |
| GET | `/api/cloudflared/logs/export` | `cloudflared:read` | Export module logs |
| DELETE | `/api/cloudflared/logs` | `cloudflared:write` | Clear module runtime logs |
| GET | `/api/cloudflared/tunnels` | `cloudflared:read` | List Cloudflare Tunnel |
| POST | `/api/cloudflared/tunnels` | `cloudflared:write` | Create Cloudflare Tunnel |
| POST | `/api/cloudflared/tunnels/{key}/copy` | `cloudflared:write` | Duplicate Cloudflare Tunnel |
| PUT | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Update Cloudflare Tunnel |
| DELETE | `/api/cloudflared/tunnels/{key}` | `cloudflared:write` | Delete Cloudflare Tunnel |
| PUT | `/api/cloudflared/tunnels/{key}/enable` | `cloudflared:write` | Enable or disable Cloudflare Tunnel |
| PUT | `/api/cloudflared/tunnels-order` | `cloudflared:write` | Reorder Cloudflare Tunnel |
| PUT | `/api/cloudflared/tunnels-enable` | `cloudflared:write` | Enable or disable in bulk: Cloudflare Tunnel |
| POST | `/api/cloudflared/tunnels-delete` | `cloudflared:write` | Delete in bulk: Cloudflare Tunnel |
| PUT | `/api/cloudflared/tunnels-group` | `cloudflared:write` | Set group in bulk: Cloudflare Tunnel |
| GET | `/api/cloudflared/tunnels/{key}/secrets` | `cloudflared:write` | View secrets in plain text |
| POST | `/api/cloudflared/tunnels/{key}/restart` | `cloudflared:write` | Restart a Cloudflare Tunnel |
| GET | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:read` | Read the routes of a token tunnel |
| PUT | `/api/cloudflared/tunnels/{key}/ingress` | `cloudflared:write` | Save the routes of a token tunnel |
| GET | `/api/cloudflared/settings` | `cloudflared:read` | Read Cloudflare Tunnel settings |
| PUT | `/api/cloudflared/settings` | `cloudflared:write` | Save Cloudflare Tunnel settings |
| GET | `/api/cloudflared/install` | `cloudflared:read` | View the install info and progress of a managed program |
| POST | `/api/cloudflared/install` | `cloudflared:write` | Install or update a managed program online |
| POST | `/api/cloudflared/install/check` | `cloudflared:write` | Check a managed program for updates |
| POST | `/api/cloudflared/install/rollback` | `cloudflared:write` | Switch to a saved previous version |
| DELETE | `/api/cloudflared/install` | `cloudflared:write` | Uninstall a managed program |
| DELETE | `/api/cloudflared/install/backups/{id}` | `cloudflared:write` | Delete a saved previous version |
| POST | `/api/cloudflared/reset` | `cloudflared:write` | Restore the module default settings |

## SSL certificate

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/ssl/logs` | `ssl:read` | View module runtime logs |
| GET | `/api/ssl/logs/sources` | `ssl:read` | List all sources of module logs |
| GET | `/api/ssl/logs/export` | `ssl:read` | Export module logs |
| DELETE | `/api/ssl/logs` | `ssl:write` | Clear module runtime logs |
| GET | `/api/ssl/certs` | `ssl:read` | List Certificate |
| POST | `/api/ssl/certs` | `ssl:write` | Create Certificate |
| POST | `/api/ssl/certs/{key}/copy` | `ssl:write` | Duplicate Certificate |
| PUT | `/api/ssl/certs/{key}` | `ssl:write` | Update Certificate |
| DELETE | `/api/ssl/certs/{key}` | `ssl:write` | Delete Certificate |
| PUT | `/api/ssl/certs/{key}/enable` | `ssl:write` | Enable or disable Certificate |
| PUT | `/api/ssl/certs-order` | `ssl:write` | Reorder Certificate |
| PUT | `/api/ssl/certs-enable` | `ssl:write` | Enable or disable in bulk: Certificate |
| POST | `/api/ssl/certs-delete` | `ssl:write` | Delete in bulk: Certificate |
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
| GET | `/api/waf/logs/sources` | `waf:read` | List all sources of module logs |
| GET | `/api/waf/logs/export` | `waf:read` | Export module logs |
| DELETE | `/api/waf/logs` | `waf:write` | Clear module runtime logs |
| GET | `/api/waf/instances` | `waf:read` | List WAF instance |
| POST | `/api/waf/instances` | `waf:write` | Create WAF instance |
| POST | `/api/waf/instances/{key}/copy` | `waf:write` | Duplicate WAF instance |
| PUT | `/api/waf/instances/{key}` | `waf:write` | Update WAF instance |
| DELETE | `/api/waf/instances/{key}` | `waf:write` | Delete WAF instance |
| PUT | `/api/waf/instances/{key}/enable` | `waf:write` | Enable or disable WAF instance |
| PUT | `/api/waf/instances-order` | `waf:write` | Reorder WAF instance |
| PUT | `/api/waf/instances-enable` | `waf:write` | Enable or disable in bulk: WAF instance |
| POST | `/api/waf/instances-delete` | `waf:write` | Delete in bulk: WAF instance |
| PUT | `/api/waf/instances-group` | `waf:write` | Set group in bulk: WAF instance |
| POST | `/api/waf/safeline/test` | `waf:write` | Test the SafeLine detection engine |
| GET | `/api/waf/crs` | `waf:read` | List OWASP CRS rule files |
| GET | `/api/waf/crs/{file}` | `waf:read` | Read a CRS rule file |

## Web service

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/webservice/logs` | `webservice:read` | View module runtime logs |
| GET | `/api/webservice/logs/sources` | `webservice:read` | List all sources of module logs |
| GET | `/api/webservice/logs/export` | `webservice:read` | Export module logs |
| DELETE | `/api/webservice/logs` | `webservice:write` | Clear module runtime logs |
| GET | `/api/webservice/listeners` | `webservice:read` | List Web service listening |
| POST | `/api/webservice/listeners` | `webservice:write` | Create Web service listening |
| POST | `/api/webservice/listeners/{key}/copy` | `webservice:write` | Duplicate Web service listening |
| PUT | `/api/webservice/listeners/{key}` | `webservice:write` | Update Web service listening |
| DELETE | `/api/webservice/listeners/{key}` | `webservice:write` | Delete Web service listening |
| PUT | `/api/webservice/listeners/{key}/enable` | `webservice:write` | Enable or disable Web service listening |
| PUT | `/api/webservice/listeners-order` | `webservice:write` | Reorder Web service listening |
| PUT | `/api/webservice/listeners-enable` | `webservice:write` | Enable or disable in bulk: Web service listening |
| POST | `/api/webservice/listeners-delete` | `webservice:write` | Delete in bulk: Web service listening |
| GET | `/api/webservice/iplists` | `webservice:read` | List subscribed IP lists |
| POST | `/api/webservice/iplists` | `webservice:write` | Create Subscribed IP list |
| POST | `/api/webservice/iplists/{key}/copy` | `webservice:write` | Duplicate Subscribed IP list |
| PUT | `/api/webservice/iplists/{key}` | `webservice:write` | Update Subscribed IP list |
| DELETE | `/api/webservice/iplists/{key}` | `webservice:write` | Delete Subscribed IP list |
| PUT | `/api/webservice/iplists/{key}/enable` | `webservice:write` | Enable or disable Subscribed IP list |
| PUT | `/api/webservice/iplists-order` | `webservice:write` | Reorder Subscribed IP list |
| PUT | `/api/webservice/iplists-enable` | `webservice:write` | Enable or disable in bulk: Subscribed IP list |
| POST | `/api/webservice/iplists-delete` | `webservice:write` | Delete in bulk: Subscribed IP list |
| POST | `/api/webservice/iplists/{key}/refresh` | `webservice:write` | Refresh a subscribed IP list now |
| GET | `/api/webservice/errorpages` | `webservice:read` | List custom error pages (without page content) |
| POST | `/api/webservice/errorpages` | `webservice:write` | Add a custom error page: an uploaded page is sanitized before saving; for a file page the file is checked for availability |
| GET | `/api/webservice/errorpages/{key}` | `webservice:read` | Read a custom error page (including page content) |
| PUT | `/api/webservice/errorpages/{key}` | `webservice:write` | Edit a custom error page |
| DELETE | `/api/webservice/errorpages/{key}` | `webservice:write` | Delete a custom error page (rejected while sites still use it) |
| POST | `/api/webservice/errorpages/check` | `webservice:write` | Check a page for safety and usability and generate a preview without saving |
| GET | `/api/webservice/errorpages/styles` | `webservice:read` | Preview of the default style and all built-in styles; ?code= sets the status code (default 502) |
| GET | `/api/webservice/errorpages/{key}/preview` | `webservice:read` | Preview of a custom error page; ?code= sets the status code (default 502) |
| GET | `/api/webservice/panels` | `webservice:read` | List Navigation panels |
| POST | `/api/webservice/panels` | `webservice:write` | Create Navigation panels |
| POST | `/api/webservice/panels/{key}/copy` | `webservice:write` | Duplicate Navigation panels |
| PUT | `/api/webservice/panels/{key}` | `webservice:write` | Update Navigation panels |
| DELETE | `/api/webservice/panels/{key}` | `webservice:write` | Delete Navigation panels |
| PUT | `/api/webservice/panels/{key}/enable` | `webservice:write` | Enable or disable Navigation panels |
| PUT | `/api/webservice/panels-order` | `webservice:write` | Reorder Navigation panels |
| PUT | `/api/webservice/panels-enable` | `webservice:write` | Enable or disable in bulk: Navigation panels |
| POST | `/api/webservice/panels-delete` | `webservice:write` | Delete in bulk: Navigation panels |
| POST | `/api/webservice/panels/preview` | `webservice:write` | Navigation panel preview |
| GET | `/api/webservice/panels/icons` | `webservice:read` | Built-in icons for navigation panels |
| GET | `/api/webservice/users` | `webservice:read` | Read shared users and the OIDC identity provider |
| PUT | `/api/webservice/users` | `webservice:write` | Save shared users and the OIDC identity provider, set sites in bulk |
| POST | `/api/webservice/sites/batch` | `webservice:write` | Bulk site operations: enable, disable, delete, add, remove or replace domains, change some options, or move to another listener; changes are saved only if every site passes validation, and nothing changes if any site fails |
| GET | `/api/webservice/cache` | `webservice:read` | Entry count and size of the site's response cache (?listener=&site=) |
| POST | `/api/webservice/cache/purge` | `webservice:write` | Clear the site's response cache: leave path empty to clear everything, otherwise only entries whose path matches (same syntax as cache rule paths, * supported) |
| POST | `/api/webservice/import/nginx` | `webservice:write` | Import nginx config |
| GET | `/api/webservice/settings` | `webservice:read` | Read Web service global settings |
| GET | `/api/webservice/settings/secrets/totp` | `webservice:write` | View the TOTP secrets of shared users in plain text (indexed by username) |
| GET | `/api/webservice/settings/secrets/oauth` | `webservice:write` | View the Client Secrets of third-party logins in plain text (indexed by login method ID) |
| POST | `/api/webservice/totp-secret` | `webservice:write` | Generate a new TOTP key for a shared user (not saved; takes effect after saving settings) |
| PUT | `/api/webservice/settings` | `webservice:write` | Save Web service global settings |
| GET | `/api/webservice/stats` | `webservice:read` | Per-minute site statistics (?listener=&site=&minutes=) |
| GET | `/api/webservice/metrics` | `webservice:read` | Metrics in Prometheus text format |
| GET | `/api/webservice/topology` | `webservice:read` | Web Services topology snapshot |
| GET | `/api/webservice/visitors` | `webservice:read` | Visitor audit list |
| GET | `/api/webservice/visitors/detail` | `webservice:read` | Audit record of one visitor |
| POST | `/api/webservice/visitors/delete` | `webservice:write` | Delete visitor audit records |
| GET | `/api/webservice/visitors/settings` | `webservice:read` | Read visitor audit settings and storage status |
| PUT | `/api/webservice/visitors/settings` | `webservice:write` | Save visitor audit settings |
| GET | `/api/webservice/topology/replay` | `webservice:read` | Topology replay overview |
| GET | `/api/webservice/topology/replay/frame` | `webservice:read` | Topology replay snapshot at a moment |
| PUT | `/api/webservice/topology/replay/settings` | `webservice:write` | Change topology replay settings |
| GET | `/api/webservice/connections` | `webservice:read` | Requests in progress and recently ended long requests of a site (?listener=&site=&q=) |
| POST | `/api/webservice/connections/close` | `webservice:write` | Close a request in progress on a site, optionally banning its source address (banSeconds -1 means permanent) |
| GET | `/api/webservice/bans` | `webservice:read` | Currently banned IPs |
| POST | `/api/webservice/bans` | `webservice:write` | Manually ban an IP (seconds: 0 means 24 hours, -1 means permanent) |
| DELETE | `/api/webservice/bans` | `webservice:write` | Remove a ban (body is {"key": address}) |
| GET | `/api/webservice/geoip` | `webservice:read` | GeoIP database status |
| POST | `/api/webservice/geoip/upload` | `webservice:write` | Upload a GeoIP database (multipart field file, .mmdb) |
| POST | `/api/webservice/geoip/download` | `webservice:write` | Download a GeoIP database (.mmdb) from a URL |
| DELETE | `/api/webservice/geoip` | `webservice:write` | Remove the GeoIP database |

## Intrusion Prevention

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/guard/logs` | `guard:read` | View module runtime logs |
| GET | `/api/guard/logs/sources` | `guard:read` | List all sources of module logs |
| GET | `/api/guard/logs/export` | `guard:read` | Export module logs |
| DELETE | `/api/guard/logs` | `guard:write` | Clear module runtime logs |
| GET | `/api/guard/jails` | `guard:read` | List intrusion prevention rule |
| POST | `/api/guard/jails` | `guard:write` | Create intrusion prevention rule |
| POST | `/api/guard/jails/{key}/copy` | `guard:write` | Duplicate intrusion prevention rule |
| PUT | `/api/guard/jails/{key}` | `guard:write` | Update intrusion prevention rule |
| DELETE | `/api/guard/jails/{key}` | `guard:write` | Delete intrusion prevention rule |
| PUT | `/api/guard/jails/{key}/enable` | `guard:write` | Enable or disable intrusion prevention rule |
| PUT | `/api/guard/jails-order` | `guard:write` | Reorder intrusion prevention rule |
| PUT | `/api/guard/jails-enable` | `guard:write` | Enable or disable in bulk: intrusion prevention rule |
| POST | `/api/guard/jails-delete` | `guard:write` | Delete in bulk: intrusion prevention rule |
| PUT | `/api/guard/jails-group` | `guard:write` | Set group in bulk: intrusion prevention rule |
| GET | `/api/guard/presets` | `guard:read` | Jail templates for common services |
| POST | `/api/guard/jails/test` | `guard:write` | Test filters against sample log lines |
| GET | `/api/guard/status` | `guard:read` | Firewall sync status and environment |
| GET | `/api/guard/settings` | `guard:read` | Global settings |
| PUT | `/api/guard/settings` | `guard:write` | Update global settings |
| GET | `/api/guard/bans` | `guard:read` | Currently banned IPs |
| POST | `/api/guard/bans` | `guard:write` | Ban an IP or network manually (seconds: 0 uses the default duration, -1 means permanent) |
| DELETE | `/api/guard/bans` | `guard:write` | Remove a ban (body is {"key": address}) |
| GET | `/api/guard/firewall/rules` | `guard:read` | Rules Gately wrote to the system firewall (text) |

## Wake-on-LAN

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/wol/logs` | `wol:read` | View module runtime logs |
| GET | `/api/wol/logs/sources` | `wol:read` | List all sources of module logs |
| GET | `/api/wol/logs/export` | `wol:read` | Export module logs |
| DELETE | `/api/wol/logs` | `wol:write` | Clear module runtime logs |
| GET | `/api/wol/devices` | `wol:read` | List Wake a device |
| POST | `/api/wol/devices` | `wol:write` | Create Wake a device |
| POST | `/api/wol/devices/{key}/copy` | `wol:write` | Duplicate Wake a device |
| PUT | `/api/wol/devices/{key}` | `wol:write` | Update Wake a device |
| DELETE | `/api/wol/devices/{key}` | `wol:write` | Delete Wake a device |
| PUT | `/api/wol/devices/{key}/enable` | `wol:write` | Enable or disable Wake a device |
| PUT | `/api/wol/devices-order` | `wol:write` | Reorder Wake a device |
| PUT | `/api/wol/devices-enable` | `wol:write` | Enable or disable in bulk: Wake a device |
| POST | `/api/wol/devices-delete` | `wol:write` | Delete in bulk: Wake a device |
| PUT | `/api/wol/devices-group` | `wol:write` | Set group in bulk: Wake a device |
| POST | `/api/wol/devices/{key}/wake` | `wol:write` | Send a Wake-on-LAN magic packet to a device |
| GET | `/api/wol/status` | `wol:read` | View device online status |

## Web terminal

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/webterminal/logs` | `webterminal:read` | View module runtime logs |
| GET | `/api/webterminal/logs/sources` | `webterminal:read` | List all sources of module logs |
| GET | `/api/webterminal/logs/export` | `webterminal:read` | Export module logs |
| DELETE | `/api/webterminal/logs` | `webterminal:write` | Clear module runtime logs |
| GET | `/api/webterminal/connections` | `webterminal:read` | List Terminal connection |
| POST | `/api/webterminal/connections` | `webterminal:write` | Create Terminal connection |
| POST | `/api/webterminal/connections/{key}/copy` | `webterminal:write` | Duplicate Terminal connection |
| PUT | `/api/webterminal/connections/{key}` | `webterminal:write` | Update Terminal connection |
| DELETE | `/api/webterminal/connections/{key}` | `webterminal:write` | Delete Terminal connection |
| PUT | `/api/webterminal/connections/{key}/enable` | `webterminal:write` | Enable or disable Terminal connection |
| PUT | `/api/webterminal/connections-order` | `webterminal:write` | Reorder Terminal connection |
| PUT | `/api/webterminal/connections-enable` | `webterminal:write` | Enable or disable in bulk: Terminal connection |
| POST | `/api/webterminal/connections-delete` | `webterminal:write` | Delete in bulk: Terminal connection |
| PUT | `/api/webterminal/connections-group` | `webterminal:write` | Set group in bulk: Terminal connection |
| GET | `/api/webterminal/connections/{key}/secrets` | `webterminal:write` | View secrets in plain text |
| DELETE | `/api/webterminal/connections/{key}/hostkey` | `webterminal:write` | Clear the recorded host key |
| POST | `/api/webterminal/connections/{key}/test` | `webterminal:write` | Test a saved connection |
| POST | `/api/webterminal/connections/test` | `webterminal:write` | Test temporary connection parameters |
| PUT | `/api/webterminal/connections/{key}/quickaccess` | `webterminal:write` | Set the quick access for a connection |
| GET | `/api/webterminal/credentials` | `webterminal:read` | List Terminal credential preset |
| POST | `/api/webterminal/credentials` | `webterminal:write` | Create Terminal credential preset |
| POST | `/api/webterminal/credentials/{key}/copy` | `webterminal:write` | Duplicate Terminal credential preset |
| PUT | `/api/webterminal/credentials/{key}` | `webterminal:write` | Update Terminal credential preset |
| DELETE | `/api/webterminal/credentials/{key}` | `webterminal:write` | Delete Terminal credential preset |
| PUT | `/api/webterminal/credentials/{key}/enable` | `webterminal:write` | Enable or disable Terminal credential preset |
| PUT | `/api/webterminal/credentials-order` | `webterminal:write` | Reorder Terminal credential preset |
| PUT | `/api/webterminal/credentials-enable` | `webterminal:write` | Enable or disable in bulk: Terminal credential preset |
| POST | `/api/webterminal/credentials-delete` | `webterminal:write` | Delete in bulk: Terminal credential preset |
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
| GET | `/api/cron/logs/sources` | `cron:read` | List all sources of module logs |
| GET | `/api/cron/logs/export` | `cron:read` | Export module logs |
| DELETE | `/api/cron/logs` | `cron:write` | Clear module runtime logs |
| GET | `/api/cron/tasks` | `cron:read` | List Scheduled task |
| POST | `/api/cron/tasks` | `cron:write` | Create Scheduled task |
| POST | `/api/cron/tasks/{key}/copy` | `cron:write` | Duplicate Scheduled task |
| PUT | `/api/cron/tasks/{key}` | `cron:write` | Update Scheduled task |
| DELETE | `/api/cron/tasks/{key}` | `cron:write` | Delete Scheduled task |
| PUT | `/api/cron/tasks/{key}/enable` | `cron:write` | Enable or disable Scheduled task |
| PUT | `/api/cron/tasks-order` | `cron:write` | Reorder Scheduled task |
| PUT | `/api/cron/tasks-enable` | `cron:write` | Enable or disable in bulk: Scheduled task |
| POST | `/api/cron/tasks-delete` | `cron:write` | Delete in bulk: Scheduled task |
| PUT | `/api/cron/tasks-group` | `cron:write` | Set group in bulk: Scheduled task |
| POST | `/api/cron/tasks/{key}/run` | `cron:write` | Run a scheduled task immediately |
| POST | `/api/cron/parse` | `cron:write` | Parse and preview a Cron expression |

## Docker management

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/docker/logs` | `docker:read` | View module runtime logs |
| GET | `/api/docker/logs/sources` | `docker:read` | List all sources of module logs |
| GET | `/api/docker/logs/export` | `docker:read` | Export module logs |
| DELETE | `/api/docker/logs` | `docker:write` | Clear module runtime logs |
| GET | `/api/docker/tasks` | `docker:read` | List background tasks |
| GET | `/api/docker/tasks/{id}` | `docker:read` | View background task progress and output |
| POST | `/api/docker/tasks/{id}/cancel` | `docker:write` | Cancel a running background task |
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
| GET | `/api/webdav/logs/sources` | `webdav:read` | List all sources of module logs |
| GET | `/api/webdav/logs/export` | `webdav:read` | Export module logs |
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
| GET | `/api/ftp/logs/sources` | `ftp:read` | List all sources of module logs |
| GET | `/api/ftp/logs/export` | `ftp:read` | Export module logs |
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
| GET | `/api/smb/logs/sources` | `smb:read` | List all sources of module logs |
| GET | `/api/smb/logs/export` | `smb:read` | Export module logs |
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
| GET | `/api/filebrowser/logs/sources` | `filebrowser:read` | List all sources of module logs |
| GET | `/api/filebrowser/logs/export` | `filebrowser:read` | Export module logs |
| DELETE | `/api/filebrowser/logs` | `filebrowser:write` | Clear module runtime logs |
| GET | `/api/filebrowser/instances` | `filebrowser:read` | List File Browser instance |
| POST | `/api/filebrowser/instances` | `filebrowser:write` | Create File Browser instance |
| POST | `/api/filebrowser/instances/{key}/copy` | `filebrowser:write` | Duplicate File Browser instance |
| PUT | `/api/filebrowser/instances/{key}` | `filebrowser:write` | Update File Browser instance |
| DELETE | `/api/filebrowser/instances/{key}` | `filebrowser:write` | Delete File Browser instance |
| PUT | `/api/filebrowser/instances/{key}/enable` | `filebrowser:write` | Enable or disable File Browser instance |
| PUT | `/api/filebrowser/instances-order` | `filebrowser:write` | Reorder File Browser instance |
| PUT | `/api/filebrowser/instances-enable` | `filebrowser:write` | Enable or disable in bulk: File Browser instance |
| POST | `/api/filebrowser/instances-delete` | `filebrowser:write` | Delete in bulk: File Browser instance |
| PUT | `/api/filebrowser/instances-group` | `filebrowser:write` | Set group in bulk: File Browser instance |
| GET | `/api/filebrowser/instances/{key}/secrets` | `filebrowser:write` | View secrets in plain text |
| POST | `/api/filebrowser/instances/{key}/restart` | `filebrowser:write` | Restart a File Browser instance |
| GET | `/api/filebrowser/settings` | `filebrowser:read` | Read File Browser program settings |
| PUT | `/api/filebrowser/settings` | `filebrowser:write` | Save File Browser program settings |
| GET | `/api/filebrowser/install` | `filebrowser:read` | View the install info and progress of a managed program |
| POST | `/api/filebrowser/install` | `filebrowser:write` | Install or update a managed program online |
| POST | `/api/filebrowser/install/check` | `filebrowser:write` | Check a managed program for updates |
| POST | `/api/filebrowser/install/rollback` | `filebrowser:write` | Switch to a saved previous version |
| DELETE | `/api/filebrowser/install` | `filebrowser:write` | Uninstall a managed program |
| DELETE | `/api/filebrowser/install/backups/{id}` | `filebrowser:write` | Delete a saved previous version |
| POST | `/api/filebrowser/reset` | `filebrowser:write` | Restore the module default settings |

## Storage Mounts

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/storage/logs` | `storage:read` | View module runtime logs |
| GET | `/api/storage/logs/sources` | `storage:read` | List all sources of module logs |
| GET | `/api/storage/logs/export` | `storage:read` | Export module logs |
| DELETE | `/api/storage/logs` | `storage:write` | Clear module runtime logs |
| GET | `/api/storage/mounts` | `storage:read` | List Storage Mounts |
| POST | `/api/storage/mounts` | `storage:write` | Create Storage Mounts |
| POST | `/api/storage/mounts/{key}/copy` | `storage:write` | Duplicate Storage Mounts |
| PUT | `/api/storage/mounts/{key}` | `storage:write` | Update Storage Mounts |
| DELETE | `/api/storage/mounts/{key}` | `storage:write` | Delete Storage Mounts |
| PUT | `/api/storage/mounts/{key}/enable` | `storage:write` | Enable or disable Storage Mounts |
| PUT | `/api/storage/mounts-order` | `storage:write` | Reorder Storage Mounts |
| PUT | `/api/storage/mounts-enable` | `storage:write` | Enable or disable in bulk: Storage Mounts |
| POST | `/api/storage/mounts-delete` | `storage:write` | Delete in bulk: Storage Mounts |
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
| GET | `/api/notify/logs/sources` | `notify:read` | List all sources of module logs |
| GET | `/api/notify/logs/export` | `notify:read` | Export module logs |
| DELETE | `/api/notify/logs` | `notify:write` | Clear module runtime logs |
| GET | `/api/notify/channels` | `notify:read` | List Notification channel |
| POST | `/api/notify/channels` | `notify:write` | Create Notification channel |
| POST | `/api/notify/channels/{key}/copy` | `notify:write` | Duplicate Notification channel |
| PUT | `/api/notify/channels/{key}` | `notify:write` | Update Notification channel |
| DELETE | `/api/notify/channels/{key}` | `notify:write` | Delete Notification channel |
| PUT | `/api/notify/channels/{key}/enable` | `notify:write` | Enable or disable Notification channel |
| PUT | `/api/notify/channels-order` | `notify:write` | Reorder Notification channel |
| PUT | `/api/notify/channels-enable` | `notify:write` | Enable or disable in bulk: Notification channel |
| POST | `/api/notify/channels-delete` | `notify:write` | Delete in bulk: Notification channel |
| PUT | `/api/notify/channels-group` | `notify:write` | Set group in bulk: Notification channel |
| GET | `/api/notify/channels/{key}/secrets` | `notify:write` | View secrets in plain text |
| GET | `/api/notify/rules` | `notify:read` | List Notification rule |
| POST | `/api/notify/rules` | `notify:write` | Create Notification rule |
| POST | `/api/notify/rules/{key}/copy` | `notify:write` | Duplicate Notification rule |
| PUT | `/api/notify/rules/{key}` | `notify:write` | Update Notification rule |
| DELETE | `/api/notify/rules/{key}` | `notify:write` | Delete Notification rule |
| PUT | `/api/notify/rules/{key}/enable` | `notify:write` | Enable or disable Notification rule |
| PUT | `/api/notify/rules-order` | `notify:write` | Reorder Notification rule |
| PUT | `/api/notify/rules-enable` | `notify:write` | Enable or disable in bulk: Notification rule |
| POST | `/api/notify/rules-delete` | `notify:write` | Delete in bulk: Notification rule |
| PUT | `/api/notify/rules-group` | `notify:write` | Set group in bulk: Notification rule |
| GET | `/api/notify/settings` | `notify:read` | Get notification settings |
| PUT | `/api/notify/settings` | `notify:write` | Save notification settings |
| GET | `/api/notify/event-types` | `notify:read` | Event types that can be subscribed to |
| GET | `/api/notify/history` | `notify:read` | Recent delivery records |
| POST | `/api/notify/channels/{key}/test` | `notify:write` | Send a test notification to a channel |

## Performance

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/perf/logs` | `perf:read` | View module runtime logs |
| GET | `/api/perf/logs/sources` | `perf:read` | List all sources of module logs |
| GET | `/api/perf/logs/export` | `perf:read` | Export module logs |
| DELETE | `/api/perf/logs` | `perf:write` | Clear module runtime logs |
| GET | `/api/perf/rules` | `perf:read` | List Performance alert rules |
| POST | `/api/perf/rules` | `perf:write` | Create Performance alert rules |
| POST | `/api/perf/rules/{key}/copy` | `perf:write` | Duplicate Performance alert rules |
| PUT | `/api/perf/rules/{key}` | `perf:write` | Update Performance alert rules |
| DELETE | `/api/perf/rules/{key}` | `perf:write` | Delete Performance alert rules |
| PUT | `/api/perf/rules/{key}/enable` | `perf:write` | Enable or disable Performance alert rules |
| PUT | `/api/perf/rules-order` | `perf:write` | Reorder Performance alert rules |
| PUT | `/api/perf/rules-enable` | `perf:write` | Enable or disable in bulk: Performance alert rules |
| POST | `/api/perf/rules-delete` | `perf:write` | Delete in bulk: Performance alert rules |
| PUT | `/api/perf/rules-group` | `perf:write` | Set group in bulk: Performance alert rules |
| GET | `/api/perf/metrics` | `perf:read` | Metrics available for alerts |
| GET | `/api/perf/overview` | `perf:read` | Host and process history (?range=60s|1h|24h) |
| GET | `/api/perf/sites` | `perf:read` | Each site's metrics over roughly the last 60 seconds |
| GET | `/api/perf/alerts` | `perf:read` | Alerts in progress (pending and firing) |
| GET | `/api/perf/processes` | `perf:read` | Managed processes (frpc, tailscaled and so on) with their current and last 60 seconds of resource usage |

## Network diagnostics

| Method | Path | Required permission | Description |
| --- | --- | --- | --- |
| GET | `/api/nettools/logs` | `nettools:read` | View module runtime logs |
| GET | `/api/nettools/logs/sources` | `nettools:read` | List all sources of module logs |
| GET | `/api/nettools/logs/export` | `nettools:read` | Export module logs |
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
| GET | `/api/backup/logs/sources` | `backup:read` | List all sources of module logs |
| GET | `/api/backup/logs/export` | `backup:read` | Export module logs |
| DELETE | `/api/backup/logs` | `backup:write` | Clear module runtime logs |
| GET | `/api/backup/targets` | `backup:read` | List Backup destinations |
| POST | `/api/backup/targets` | `backup:write` | Create Backup destinations |
| POST | `/api/backup/targets/{key}/copy` | `backup:write` | Duplicate Backup destinations |
| PUT | `/api/backup/targets/{key}` | `backup:write` | Update Backup destinations |
| DELETE | `/api/backup/targets/{key}` | `backup:write` | Delete Backup destinations |
| PUT | `/api/backup/targets/{key}/enable` | `backup:write` | Enable or disable Backup destinations |
| PUT | `/api/backup/targets-order` | `backup:write` | Reorder Backup destinations |
| PUT | `/api/backup/targets-enable` | `backup:write` | Enable or disable in bulk: Backup destinations |
| POST | `/api/backup/targets-delete` | `backup:write` | Delete in bulk: Backup destinations |
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
| GET | `/api/logs/sources` | `logs:read` | List all sources of system logs |
| GET | `/api/logs/export` | `logs:read` | Export system logs |
| GET | `/api/system/settings` | Login sessions only | Read global settings |
| PUT | `/api/system/settings` | Login sessions only | Save global settings |
| GET | `/api/system/settings/secrets` | Login sessions only | View third-party login Client Secrets in plain text |
| GET | `/api/system/info` | `status:read` | View system information |
| GET | `/api/system/update` | `status:read` | Check for new versions and security advisories |
| GET | `/api/system/update/state` | `status:read` | View the result of the last update check |
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

### crud.batchDeleteRequest

| Field | Type |
| --- | --- |
| `keys` | Array of string |

### crud.batchEnableRequest

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `keys` | Array of string |

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

### ddns.RemoteDeleteResult

| Field | Type |
| --- | --- |
| `deleted` | integer |
| `domain` | string |
| `error` | string |
| `key` | string |
| `type` | string |

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

### ddns.remoteDeleteRequest

| Field | Type |
| --- | --- |
| `records` | Array of string |
| `task` | `ddns.Task` |

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
| `access` | `frp.ClientAccess` |
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

### frp.ClientAccess

| Field | Type |
| --- | --- |
| `httpPort` | integer |
| `httpScheme` | string |
| `httpsPort` | integer |
| `subdomainHost` | string |

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
| `ipList` | Array of string |
| `ipMode` | string |
| `ipSets` | Array of string |
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
| `ipList` | Array of string |
| `ipMode` | string |
| `ipSets` | Array of string |
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
| `backend` | string |
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
| `ipv6Strict` | boolean |
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

### netbird.ConnView

| Field | Type |
| --- | --- |
| `connected` | boolean |
| `error` | string |
| `url` | string |

### netbird.Instance

| Field | Type |
| --- | --- |
| `acceptDns` | boolean |
| `acceptRoutes` | boolean |
| `blockInbound` | boolean |
| `daemonAddr` | string |
| `daemonEnv` | string |
| `daemonExtraArgs` | string |
| `enable` | boolean |
| `extraUpArgs` | string |
| `group` | string |
| `hostname` | string |
| `key` | string |
| `legacyStateDir` | boolean |
| `localForwarding` | boolean |
| `manageDaemon` | boolean |
| `managementUrl` | string |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `serverRoutes` | boolean |
| `setupKey` | string |
| `socks5` | string |
| `stateDir` | string |
| `userspace` | boolean |

### netbird.PeerView

| Field | Type |
| --- | --- |
| `connType` | string |
| `fqdn` | string |
| `ips` | Array of string |
| `lastSeen` | Time (RFC 3339) |
| `latencyMs` | number |
| `name` | string |
| `online` | boolean |
| `relay` | string |
| `routes` | Array of string |
| `rxBytes` | integer |
| `status` | string |
| `txBytes` | integer |

### netbird.SelfView

| Field | Type |
| --- | --- |
| `fqdn` | string |
| `ips` | Array of string |
| `kernel` | boolean |
| `name` | string |
| `wgPort` | integer |

### netbird.Settings

| Field | Type |
| --- | --- |
| `installProxy` | string |
| `netbirdPath` | string |

### netbird.SettingsView

| Field | Type |
| --- | --- |
| `cli` | string |
| `defaultManagementUrl` | string |
| `settings` | `netbird.Settings` |

### netbird.StatusView

| Field | Type |
| --- | --- |
| `authCode` | string |
| `authUrl` | string |
| `cli` | string |
| `daemon` | `proc.State` |
| `daemonStatus` | string |
| `enable` | boolean |
| `error` | string |
| `key` | string |
| `management` | `netbird.ConnView` |
| `name` | string |
| `peers` | Array of `netbird.PeerView` |
| `peersConnected` | integer |
| `relaysTotal` | integer |
| `relaysUp` | integer |
| `self` | `netbird.SelfView` |
| `signal` | `netbird.ConnView` |
| `version` | string |

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
| `proxy` | string |
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

### notify.Settings

| Field | Type |
| --- | --- |
| `proxy` | string |
| `serverName` | string |

### notify.settingsView

| Field | Type |
| --- | --- |
| `hostname` | string |
| `proxy` | string |
| `serverName` | string |

### perf.AlertState

| Field | Type |
| --- | --- |
| `firedAt` | integer |
| `level` | string |
| `name` | string |
| `rule` | string |
| `since` | integer |
| `state` | string |
| `subject` | string |
| `target` | string |
| `value` | number |

### perf.MetricDef

| Field | Type |
| --- | --- |
| `id` | string |
| `label` | string |
| `max` | number |
| `scope` | string |
| `unit` | string |
| `window` | boolean |

### perf.Overview

| Field | Type |
| --- | --- |
| `latest` | `perf.Point` |
| `range` | string |
| `series` | Array of `perf.Point` |
| `step` | integer |

### perf.Point

| Field | Type |
| --- | --- |
| `childCpu` | number |
| `childCpuMax` | number |
| `childRss` | number |
| `cpu` | number |
| `cpuMax` | number |
| `disk` | number |
| `goroutines` | number |
| `load1` | number |
| `mem` | number |
| `procCpu` | number |
| `procCpuMax` | number |
| `procFds` | number |
| `procRss` | number |
| `t` | integer |

### perf.ProcSample

| Field | Type |
| --- | --- |
| `cpu` | number |
| `rss` | number |
| `t` | integer |

### perf.ProcessView

| Field | Type |
| --- | --- |
| `cpu` | number |
| `module` | string |
| `name` | string |
| `pid` | integer |
| `program` | string |
| `restarts` | integer |
| `rss` | integer |
| `samples` | Array of `perf.ProcSample` |
| `since` | Time (RFC 3339) |
| `threads` | integer |

### perf.Rule

| Field | Type |
| --- | --- |
| `duration` | integer |
| `enable` | boolean |
| `group` | string |
| `key` | string |
| `level` | string |
| `metric` | string |
| `minRequests` | integer |
| `name` | string |
| `op` | string |
| `remark` | string |
| `sites` | Array of string |
| `threshold` | number |

### perf.SiteView

| Field | Type |
| --- | --- |
| `active` | integer |
| `err5xx` | number |
| `key` | string |
| `latencies` | integer |
| `name` | string |
| `p95` | number |
| `qps` | number |
| `requests` | integer |
| `upstreams` | Array of `perf.UpstreamView` |

### perf.UpstreamView

| Field | Type |
| --- | --- |
| `conns` | integer |
| `healthy` | boolean |
| `key` | string |
| `route` | string |
| `url` | string |

### portforward.Rule

| Field | Type |
| --- | --- |
| `autoFirewall` | boolean |
| `enable` | boolean |
| `group` | string |
| `ipList` | Array of string |
| `ipMode` | string |
| `ipSets` | Array of string |
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

### proc.State

| Field | Type |
| --- | --- |
| `error` | string |
| `lastExit` | string |
| `pid` | integer |
| `restarts` | integer |
| `running` | boolean |
| `since` | Time (RFC 3339) |

### protoroute.Route

| Field | Type |
| --- | --- |
| `alpn` | Array of string |
| `disabled` | boolean |
| `domains` | Array of string |
| `name` | string |
| `protocol` | string |
| `proxyProtocol` | string |
| `targetAddresses` | Array of string |
| `targetPort` | integer |

### protoroute.Rule

| Field | Type |
| --- | --- |
| `autoFirewall` | boolean |
| `enable` | boolean |
| `group` | string |
| `ipList` | Array of string |
| `ipMode` | string |
| `ipSets` | Array of string |
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
| `disableCname` | boolean |
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
| `autoFirewall` | boolean |
| `autoRetry` | boolean |
| `bind` | `stun.Bind` |
| `disableCheck` | boolean |
| `disableForward` | boolean |
| `enable` | boolean |
| `group` | string |
| `heartbeatInterval` | integer |
| `ipList` | Array of string |
| `ipMode` | string |
| `ipSets` | Array of string |
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

### tailscale.Instance

| Field | Type |
| --- | --- |
| `acceptDns` | boolean |
| `acceptRoutes` | boolean |
| `advertiseExitNode` | boolean |
| `advertiseRoutes` | Array of string |
| `authKey` | string |
| `daemonEnv` | string |
| `daemonExtraArgs` | string |
| `enable` | boolean |
| `extraUpArgs` | string |
| `group` | string |
| `hostname` | string |
| `key` | string |
| `legacyStateDir` | boolean |
| `loginServer` | string |
| `manageDaemon` | boolean |
| `name` | string |
| `port` | integer |
| `remark` | string |
| `socket` | string |
| `socks5` | string |
| `stateDir` | string |
| `userspace` | boolean |

### tailscale.PeerView

| Field | Type |
| --- | --- |
| `active` | boolean |
| `curAddr` | string |
| `dnsName` | string |
| `exitNode` | boolean |
| `exitNodeOption` | boolean |
| `ips` | Array of string |
| `lastSeen` | Time (RFC 3339) |
| `name` | string |
| `online` | boolean |
| `os` | string |
| `relay` | string |
| `routes` | Array of string |
| `rxBytes` | integer |
| `txBytes` | integer |

### tailscale.Settings

| Field | Type |
| --- | --- |
| `installProxy` | string |
| `tailscalePath` | string |
| `tailscaledPath` | string |

### tailscale.SettingsView

| Field | Type |
| --- | --- |
| `cli` | string |
| `settings` | `tailscale.Settings` |

### tailscale.StatusView

| Field | Type |
| --- | --- |
| `authUrl` | string |
| `backendState` | string |
| `cli` | string |
| `daemon` | `proc.State` |
| `enable` | boolean |
| `error` | string |
| `health` | Array of string |
| `key` | string |
| `magicDnsSuffix` | string |
| `name` | string |
| `peers` | Array of `tailscale.PeerView` |
| `self` | `tailscale.PeerView` |
| `tailnet` | string |
| `version` | string |

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
| `safeline` | `waf.SafeLine` |
| `skipBodyPaths` | string |
| `webdav` | boolean |

### waf.RuleSet

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `param` | string |
| `remark` | string |
| `type` | string |

### waf.SafeLine

| Field | Type |
| --- | --- |
| `addr` | string |
| `bodyLimitKB` | integer |
| `enable` | boolean |
| `failClosed` | boolean |
| `timeoutMs` | integer |

### webhook.Config

| Field | Type |
| --- | --- |
| `body` | string |
| `enable` | boolean |
| `headers` | Array of string |
| `insecureSkipVerify` | boolean |
| `method` | string |
| `proxy` | string |
| `successContent` | Array of string |
| `timeout` | integer |
| `url` | string |

### webpage.CheckReport

| Field | Type |
| --- | --- |
| `pageTitle` | string |
| `placeholders` | Array of string |
| `removed` | Array of string |
| `size` | integer |
| `warnings` | Array of string |

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

### webservice.BackendTimeouts

| Field | Type |
| --- | --- |
| `connect` | integer |
| `responseHeader` | integer |
| `tlsHandshake` | integer |

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
| `rules` | Array of `webservice.CacheRule` |
| `ttl` | integer |

### webservice.CacheRule

| Field | Type |
| --- | --- |
| `noCache` | boolean |
| `override` | boolean |
| `path` | string |
| `ttl` | integer |

### webservice.CacheStatsView

| Field | Type |
| --- | --- |
| `bytes` | integer |
| `enabled` | boolean |
| `entries` | integer |
| `maxBytes` | integer |

### webservice.ClientTimeouts

| Field | Type |
| --- | --- |
| `readIdle` | integer |
| `writeIdle` | integer |

### webservice.ConnView

| Field | Type |
| --- | --- |
| `down` | integer |
| `durationMs` | integer |
| `end` | Time (RFC 3339) |
| `host` | string |
| `id` | string |
| `ip` | string |
| `method` | string |
| `path` | string |
| `peer` | string |
| `port` | integer |
| `proto` | string |
| `result` | string |
| `start` | Time (RFC 3339) |
| `up` | integer |
| `websocket` | boolean |

### webservice.ConnsView

| Field | Type |
| --- | --- |
| `active` | Array of `webservice.ConnView` |
| `closed` | integer |
| `history` | Array of `webservice.ConnView` |
| `matched` | integer |
| `total` | integer |

### webservice.ErrorPage

| Field | Type |
| --- | --- |
| `file` | string |
| `html` | string |
| `key` | string |
| `name` | string |
| `remark` | string |
| `source` | string |
| `updatedAt` | Time (RFC 3339) |

### webservice.ErrorPageCheckRequest

| Field | Type |
| --- | --- |
| `code` | integer |
| `file` | string |
| `html` | string |
| `source` | string |

### webservice.ErrorPageCheckResult

| Field | Type |
| --- | --- |
| `preview` | string |
| `report` | `webpage.CheckReport` |

### webservice.ErrorPageOptions

| Field | Type |
| --- | --- |
| `codes` | string |
| `intercept` | boolean |
| `page` | string |
| `style` | string |

### webservice.ErrorPageSaved

| Field | Type |
| --- | --- |
| `page` | `webservice.ErrorPageView` |
| `report` | `webpage.CheckReport` |

### webservice.ErrorPageStyle

| Field | Type |
| --- | --- |
| `page` | string |
| `style` | string |

### webservice.ErrorPageView

| Field | Type |
| --- | --- |
| `error` | string |
| `file` | string |
| `key` | string |
| `name` | string |
| `remark` | string |
| `size` | integer |
| `source` | string |
| `updatedAt` | Time (RFC 3339) |
| `usedBy` | Array of string |

### webservice.ErrorStylePreview

| Field | Type |
| --- | --- |
| `html` | string |
| `id` | string |

### webservice.ForwardAuth

| Field | Type |
| --- | --- |
| `copyHeaders` | Array of string |
| `loginFromLocation` | boolean |
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
| `proxy` | string |
| `refreshHours` | integer |
| `remark` | string |
| `updatedAt` | Time (RFC 3339) |
| `url` | string |

### webservice.IdentityHeader

| Field | Type |
| --- | --- |
| `field` | string |
| `header` | string |

### webservice.Listener

| Field | Type |
| --- | --- |
| `acceptProxyProtocol` | boolean |
| `accessPort` | integer |
| `accessScheme` | string |
| `autoFirewall` | boolean |
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

### webservice.Panel

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `icon` | string |
| `key` | string |
| `lanCidrs` | Array of string |
| `links` | Array of `webservice.PanelLink` |
| `name` | string |
| `remark` | string |
| `subtitle` | string |
| `theme` | string |
| `title` | string |

### webservice.PanelIconView

| Field | Type |
| --- | --- |
| `cat` | string |
| `color` | string |
| `id` | string |
| `name` | string |
| `src` | string |
| `srcDark` | string |

### webservice.PanelLink

| Field | Type |
| --- | --- |
| `color` | string |
| `description` | string |
| `group` | string |
| `hide` | boolean |
| `icon` | string |
| `lanUrl` | string |
| `name` | string |
| `sameTab` | boolean |
| `url` | string |

### webservice.PanelPreview

| Field | Type |
| --- | --- |
| `html` | string |

### webservice.PanelPreviewRequest

| Field | Type |
| --- | --- |
| `lan` | boolean |
| `panel` | `webservice.Panel` |

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
| `p50` | number |
| `p95` | number |
| `requests` | integer |
| `s4xx` | integer |
| `s5xx` | integer |
| `t` | integer |

### webservice.Rate

| Field | Type |
| --- | --- |
| `bps` | number |
| `qps` | number |
| `qps60` | number |

### webservice.RateLimit

| Field | Type |
| --- | --- |
| `burst` | integer |
| `enable` | boolean |
| `maxConns` | integer |
| `rate` | number |

### webservice.RatePoint

| Field | Type |
| --- | --- |
| `bytes` | integer |
| `requests` | integer |
| `t` | integer |

### webservice.ReplayBucket

| Field | Type |
| --- | --- |
| `active` | integer |
| `bps` | number |
| `ips` | integer |
| `issue` | boolean |
| `qps` | number |
| `t` | integer |

### webservice.ReplayIndex

| Field | Type |
| --- | --- |
| `bucket` | integer |
| `buckets` | Array of `webservice.ReplayBucket` |
| `end` | integer |
| `frames` | integer |
| `from` | integer |
| `limit` | integer |
| `samples` | integer |
| `settings` | `webservice.ReplaySettings` |
| `start` | integer |
| `step` | integer |
| `to` | integer |
| `used` | integer |

### webservice.ReplayMeta

| Field | Type |
| --- | --- |
| `end` | integer |
| `frameEnd` | integer |
| `frameStart` | integer |
| `next` | integer |
| `prev` | integer |
| `start` | integer |
| `step` | integer |

### webservice.ReplaySettings

| Field | Type |
| --- | --- |
| `enable` | boolean |
| `sizeMB` | integer |

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
| `realIPFrom` | Array of string |
| `realIPHeaders` | Array of string |
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
| `clientTimeouts` | `webservice.ClientTimeouts` |
| `compress` | boolean |
| `contentType` | string |
| `cors` | boolean |
| `countries` | Array of string |
| `countryMode` | string |
| `denyClose` | boolean |
| `disableKeepAlives` | boolean |
| `domains` | Array of string |
| `enable` | boolean |
| `errorPage` | `webservice.ErrorPageOptions` |
| `forwardAuth` | `webservice.ForwardAuth` |
| `forwardedHeaders` | boolean |
| `group` | string |
| `health` | `webservice.HealthCheck` |
| `identityHeaders` | Array of `webservice.IdentityHeader` |
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
| `panel` | string |
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
| `timeouts` | `webservice.BackendTimeouts` |
| `type` | string |
| `upstreamSniFromHost` | boolean |
| `useTargetHost` | boolean |
| `userHeader` | string |
| `userRoutes` | Array of `webservice.UserRoute` |
| `users` | Array of string |
| `waf` | string |
| `webAuth` | boolean |

### webservice.SiteBatchOptions

| Field | Type |
| --- | --- |
| `accessLog` | boolean |
| `compress` | boolean |
| `cors` | boolean |
| `denyClose` | boolean |
| `errorPage` | `webservice.ErrorPageStyle` |
| `forwardedHeaders` | boolean |
| `waf` | string |

### webservice.SiteBatchRequest

| Field | Type |
| --- | --- |
| `action` | string |
| `domains` | Array of string |
| `from` | string |
| `move` | `webservice.SiteMoveTarget` |
| `options` | `webservice.SiteBatchOptions` |
| `sites` | Array of `webservice.SiteRef` |
| `to` | string |

### webservice.SiteBatchResult

| Field | Type |
| --- | --- |
| `changed` | integer |
| `moved` | Array of `webservice.SiteMoved` |
| `unchanged` | integer |

### webservice.SiteMoveTarget

| Field | Type |
| --- | --- |
| `after` | string |
| `before` | string |
| `group` | string |
| `listener` | string |

### webservice.SiteMoved

| Field | Type |
| --- | --- |
| `from` | `webservice.SiteRef` |
| `to` | `webservice.SiteRef` |

### webservice.SiteRef

| Field | Type |
| --- | --- |
| `listener` | string |
| `site` | string |

### webservice.SiteSharedUsers

| Field | Type |
| --- | --- |
| `disableAuth` | boolean |
| `listener` | string |
| `sharedUsers` | boolean |
| `site` | string |

### webservice.SiteStatsView

| Field | Type |
| --- | --- |
| `blocked` | Map of integer |
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

### webservice.TopoAction

| Field | Type |
| --- | --- |
| `id` | string |
| `kind` | string |
| `rate` | `webservice.Rate` |
| `requests` | integer |

### webservice.TopoIP

| Field | Type |
| --- | --- |
| `active` | integer |
| `activeAt` | Map of integer |
| `ip` | string |
| `last` | integer |
| `nodes` | Array of string |

### webservice.TopoListener

| Field | Type |
| --- | --- |
| `actions` | Array of `webservice.TopoAction` |
| `active` | integer |
| `enable` | boolean |
| `error` | string |
| `http3` | boolean |
| `id` | string |
| `key` | string |
| `listenIp` | string |
| `name` | string |
| `network` | string |
| `plainHttp` | string |
| `port` | integer |
| `proxyProtocol` | boolean |
| `rate` | `webservice.Rate` |
| `requests` | integer |
| `running` | boolean |
| `sites` | Array of `webservice.TopoSite` |
| `tls` | boolean |

### webservice.TopoSite

| Field | Type |
| --- | --- |
| `accessLog` | boolean |
| `active` | integer |
| `blocked` | Map of integer |
| `closed` | integer |
| `default` | boolean |
| `domains` | Array of string |
| `enable` | boolean |
| `errors` | integer |
| `group` | string |
| `guards` | Array of string |
| `id` | string |
| `key` | string |
| `name` | string |
| `rate` | `webservice.Rate` |
| `requests` | integer |
| `running` | boolean |
| `status` | Map of integer |
| `target` | string |
| `type` | string |
| `upstreams` | Array of `webservice.TopoUpstream` |

### webservice.TopoUpstream

| Field | Type |
| --- | --- |
| `backup` | boolean |
| `bytes` | integer |
| `conns` | integer |
| `healthy` | boolean |
| `id` | string |
| `match` | string |
| `rate` | `webservice.Rate` |
| `requests` | integer |
| `scope` | string |
| `url` | string |
| `weight` | integer |

### webservice.TopologyView

| Field | Type |
| --- | --- |
| `active` | integer |
| `enabled` | boolean |
| `focus` | string |
| `ipTotal` | integer |
| `ipTruncated` | boolean |
| `ips` | Array of `webservice.TopoIP` |
| `listeners` | Array of `webservice.TopoListener` |
| `rate` | `webservice.Rate` |
| `replay` | `webservice.ReplayMeta` |
| `requests` | integer |
| `series` | Array of `webservice.RatePoint` |
| `seriesStep` | integer |
| `time` | integer |
| `window` | integer |

### webservice.UpstreamStatus

| Field | Type |
| --- | --- |
| `backup` | boolean |
| `bytes` | integer |
| `conns` | integer |
| `healthy` | boolean |
| `match` | string |
| `requests` | integer |
| `scope` | string |
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

### webservice.cachePurgeRequest

| Field | Type |
| --- | --- |
| `listener` | string |
| `path` | string |
| `site` | string |

### webservice.cachePurgeResult

| Field | Type |
| --- | --- |
| `bytes` | integer |
| `enabled` | boolean |
| `entries` | integer |
| `maxBytes` | integer |
| `removed` | integer |

### webservice.closeConnRequest

| Field | Type |
| --- | --- |
| `banSeconds` | integer |
| `id` | string |
| `listener` | string |
| `site` | string |

### webservice.closeConnResult

| Field | Type |
| --- | --- |
| `banKey` | string |
| `closed` | integer |

### webservice.geoipDownload

| Field | Type |
| --- | --- |
| `proxy` | string |
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
| `hostKeyType` | string |
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

