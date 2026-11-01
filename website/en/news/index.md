---
title: Changelog
description: Release announcements and notes for each Gately version.
---

# Changelog

Announcements for major releases are listed below. See [GitHub Releases](https://github.com/wangwanjie/gately/releases) for the full changes and downloads of every version.

## [v0.6.2](/en/news/v0.6.2)

October 31, 2026 · A new Protocol Routing module lets HTTPS, SSH, RDP, OpenVPN and more share one port; every long list can be grouped and sorted; Dynamic DNS can use STUN public addresses; STUN works behind multiple NATs; several security hardening changes.

## [v0.6.1.1](/en/news/v0.6.1.1)

October 30, 2026 · Intrusion Prevention bans web path probing; a new Auth users page manages shared users and OIDC; shared users can change their own password; sites can close the connection when access is denied; Cloudflare token tunnel routes can be edited in Gately; logs get an Exclude filter.

## [v0.6.1](/en/news/v0.6.1)

October 30, 2026 · Security fix: read-only accounts and read-only API tokens can no longer read container environment variables and start commands in plain text; certificates can be issued over IPv4 only or IPv6 only; the Docker page no longer upgrades or recreates Gately's own container; notifications and logs follow the admin interface language.

## [v0.6.0.4](/en/news/v0.6.0.4)

October 29, 2026 · Adds a LuCI app for OpenWrt to see the service state, start and stop the service and set the admin port from LuCI; redirect sites support variables such as `{path}`, `{1}` and `{STUN_RuleName_PORT}`; shared users and OIDC clients in the global settings are shown as summaries and edited in a dialog.

## [v0.6.0.3](/en/news/v0.6.0.3)

October 28, 2026 · Fixes a LAN Gately receiving plain HTTP through a trusted PROXY source not recognizing that visitors used HTTPS, which broke OIDC sign-in with an https issuer, left cookies without Secure and caused plain HTTP redirect loops.

## [v0.6.0.2](/en/news/v0.6.0.2)

October 28, 2026 · Fixes the web terminal hanging at "Connecting" when opened from the fnOS desktop, File Browser failing to start with a relative configuration directory, and Chinese text entered by users being partly translated in API responses for other languages.

## [v0.6.0.1](/en/news/v0.6.0.1)

October 27, 2026 · Dynamic DNS and port forwarding get groups, sorting and search; the Web Service, Dynamic DNS and port forwarding pages can set the group of many items at once, drag several checked items together and rename groups.

## [v0.6.0](/en/news/v0.6.0)

October 27, 2026 · Several frpc and frps instances; the Web Service routes signed-in users to their own backends, groups, sorts and searches sites, and can forward Gately sign-in cookies; certificates detect the DNS zone of delegated subdomains; saved passwords and keys can be shown on request.

## [v0.5.9.1](/en/news/v0.5.9.1)

October 26, 2026 · HTTPS listeners of the Web Service can accept HTTP on the same port (redirect or serve); File Browser instances can list their access URLs; saving an edited File Browser instance or Cloudflare tunnel no longer fails.

## [v0.5.9](/en/news/v0.5.9)

October 26, 2026 · Hosted File Browser; IP address and short-lived certificates; online install, update, rollback and uninstall for frp, cloudflared, Tailscale and File Browser; the file service shared users move to a page of their own.

## [v0.5.8.1](/en/news/v0.5.8.1)

October 26, 2026 · WebDAV, FTP and SMB share one list of shared users; the Web Service gains a Close connection site type and logs the Referer and User-Agent; frps can listen on chosen addresses; copy buttons work over HTTP.

## [v0.5.8](/en/news/v0.5.8)

October 25, 2026 · A built-in OIDC identity provider lets shared users sign in to Gitea, Grafana and other applications; new SMB Service; every log can be filtered; WAF rule groups can be reordered; site headers can be picked from common entries.

## [v0.5.7](/en/news/v0.5.7)

October 24, 2026 · Storage Mounts can connect to SMB shares; themes become 15 palettes with light and dark, custom accent and corners; the management interface is compressed and the binary is about 11% smaller.

## [v0.5.6.4](/en/news/v0.5.6.4)

October 23, 2026 · Certificate auto deploy can copy to several local directories, each with its own command, and write the certificate and key under custom file names.

## [v0.5.6.3](/en/news/v0.5.6.3)

October 23, 2026 · Fixes the overview hiding the br-lan interface on OpenWrt, and the time zone that could not be set and stayed on UTC.

## [v0.5.6.2](/en/news/v0.5.6.2)

October 23, 2026 · Redesigned Web service login and error pages with two-step verification in a popup. Fixes the Docker module failing to connect on Docker 29, the overview flashing before the login page, and Cloudflare credentials and renamed DDNS records imported from Lucky not syncing.

## [v0.5.6.1](/en/news/v0.5.6.1)

October 22, 2026 · Fixes firewall rules written by the Docker image crashing an older nft on the host.

## [v0.5.6](/en/news/v0.5.6)

October 22, 2026 · Adds intrusion prevention: fail2ban-style log jails, trap ports and port-scan detection, with bans synced to nftables.

## [v0.5.5.2](/en/news/v0.5.5.2)

October 22, 2026 · Fixes sensitive files in the config directory being readable by other local users on fnOS and similar filesystems.

## [v0.5.5.1](/en/news/v0.5.5.1)

October 21, 2026 · Runtime logs are written to disk and survive restarts and upgrades.

## [v0.5.5](/en/news/v0.5.5)

October 21, 2026 · Import nginx configs, auto deploy certificates to other hosts and containers, large uploads and WebDAV behind the WAF, subscribed IP lists, and new self-check items for certificate coverage and frp conflicts.

## [v0.5.4](/en/news/v0.5.4)

October 21, 2026 · Online upgrade for Linux binary installs, so PVE LXC containers no longer need to be recreated; new OpenWrt packages (.ipk / .apk).

## [v0.5.3](/en/news/v0.5.3)

October 21, 2026 · Native fnOS app that plugs into the fnOS unified gateway and signs fnOS administrators in with their fnOS session; feature modules can be turned on or off.

## [v0.5.2](/en/news/v0.5.2)

October 20, 2026 · SNI routing lets several services share port 443, Web Services can replace nginx, shared users add TOTP and single sign-on, and a performance comparison with haproxy + nginx.

## [v0.5.0](/en/news/v0.5.0)

October 19, 2026 · Audit log, config history and rollback, Passkey, read-only accounts, ACME HTTP-01, HTTP/3, PROXY protocol, global Prometheus metrics, network self-check, and signed releases.
