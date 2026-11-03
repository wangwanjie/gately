# FAQ

## Does it need to connect to the project's servers?

No. Gately does not connect to any project servers and does not report device information; by default it anonymously queries GitHub for releases and security advisories, which can be turned off, see [Automatic update checks](/en/docs/guide/install#update-check). Outbound addresses such as public IP lookup use third-party public services and can be changed in the corresponding tasks.

## What are the default username and password?

The default account is `admin` and the default password is `gately.cn`. The default password is public, so change it under "Settings → Account & Security" right after signing in; until you do, the admin UI keeps showing a warning at the top. If the `GATELY_INITIAL_PASSWORD` environment variable was set at deployment, that password applies instead.

## What if I forget the administrator password?

Run `gately reset-password` on the server (`docker exec -it gately gately reset-password` for Docker) and type `yes` to confirm. The password is reset to the default `gately.cn`, and a running instance picks it up within seconds. See [Reset password and two-factor authentication](/en/docs/guide/install#reset).

## What if I lose my two-factor authenticator?

Run `gately reset-totp` on the server (`docker exec -it gately gately reset-totp` for Docker) and confirm. Two-factor authentication is turned off; sign in with your password and enable it again.

## Does it support fnOS?

Yes. Every release ships offline fnOS packages (`.fpk`) that you install manually from the App Center. Gately plugs into the fnOS unified gateway, so it opens from the desktop icon and also works over your remote fnOS domain. Turn on "Sign in with fnOS" under "Settings → Access control" and fnOS administrators who are signed in go straight in, without a password or two-step code. See [fnOS native app](/en/docs/guide/install#fnos).

## Does it support OpenWrt / iStoreOS?

Yes. Every release ships OpenWrt packages: the `.ipk` for OpenWrt 24.10 and earlier (including iStoreOS), installed with `opkg install`, and the `.apk` for OpenWrt 25.12 and later, installed with `apk add --allow-untrusted`. The service starts right after install and keeps its config in `/etc/gately`; to upgrade, install the newer package. The binary is about 32 MB, so a soft router with ample storage is a good fit. See [OpenWrt](/en/docs/guide/install#openwrt).

## Why is `--network host` recommended?

Port forwarding, STUN and web services listen directly on host ports. With bridge networking you have to publish every port in use yourself.

## Where do the Tailscale, frp and cloudflared programs come from?

These modules supervise the official external programs, which Gately does not bundle. The "online install" card on each module page downloads the latest version or a specific version, can use its own download proxy, and lets you roll back to an older version if an upgrade has problems, uninstall the program or restore default settings; see [Online install, update and rollback](/en/docs/modules/tunnels#install). Tailscale online install is available on Linux only. You can also place the programs yourself; see [Configuration & Runtime Options](/en/docs/guide/configuration) for where.

## My STUN rule is enabled but I still cannot connect from outside

If your router's WAN interface sits behind carrier-grade NAT (100.64.0.0/10), outside hosts cannot reach the traversed port. This is a limit of the network itself. Use [Tailscale / frp / Cloudflare Tunnel](/en/docs/modules/tunnels) instead.

## How do I report issues or contribute?

Open an issue on [GitHub Issues](https://github.com/wangwanjie/gately/issues). Development conventions are in [Contributing](/en/docs/guide/contributing).
