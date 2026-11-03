# Quick Start

Deploy, sign in and create your first rule in five minutes.

## 1. Start

```bash
docker run -d --name gately --network host \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

If you do not need Docker management, drop the `docker.sock` mount. Port forwarding, STUN and web services listen on host ports, so `--network host` is recommended.

If GHCR is slow for you, use `vanjayak/gately:latest` from Docker Hub instead; it is the same image.

::: tip fnOS users
Install the native app instead: it opens from the desktop icon through the fnOS unified gateway and can sign fnOS administrators in with their fnOS session. See [fnOS native app](/en/docs/guide/install#fnos).
:::

## 2. Sign in

Open `http://<device-ip>:16680` in a browser. The username is `admin` and the default password is `gately.cn`. The default password is public, so the admin UI keeps showing a warning at the top until you change it.

## 3. Apply basic hardening

Right after signing in, go to "Settings" and:

1. Change the administrator password;
2. Enable two-factor authentication (TOTP);
3. Set a secure entrance path, LAN-only access or admin UI HTTPS as needed.

See the [Hardening Guide](/en/docs/guide/hardening) for more recommendations.

## 4. Add your first rule

Pick a module that fits your need:

| Need | Module |
| --- | --- |
| Your home public IP keeps changing and you want a stable domain name | [Dynamic DNS](/en/docs/modules/ddns) |
| Your ISP gives no fixed public port and you want to reach home services from outside | [STUN Traversal](/en/docs/modules/stun) |
| Forward one port to another device on your LAN | [Port Forwarding](/en/docs/modules/portforward) |
| Reach multiple LAN services through one domain with HTTPS | [SSL Certificates](/en/docs/modules/ssl) + [Web Services](/en/docs/modules/web) |
| Connect networks across locations | [Tailscale / frp](/en/docs/modules/tunnels) |
