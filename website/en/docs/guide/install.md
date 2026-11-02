# Installation

## Docker

Image: `ghcr.io/wangwanjie/gately`, also published to Docker Hub as `vanjayak/gately` (the same image with the same tags), available for amd64, arm64 and arm/v7. If GHCR is hard to reach, replace the image name below with `vanjayak/gately:latest`; Docker registry mirrors work with it too.

```bash
docker run -d --name gately --network host --cap-add NET_ADMIN \
  -v ./gately-data:/etc/gately \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/wangwanjie/gately:latest
```

### Docker Compose

```yaml
services:
  gately:
    image: ghcr.io/wangwanjie/gately:latest
    container_name: gately
    restart: unless-stopped
    # Port forwarding, STUN traversal and web services all listen on host ports directly
    network_mode: host
    # Intrusion Prevention writes bans to the host firewall (remove if not needed)
    cap_add:
      - NET_ADMIN
    volumes:
      # Config directory (plain JSON, safe to keep in git; ssl.json contains certificate private keys)
      - ./data:/etc/gately
      # Docker management (remove if not needed)
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - TZ=Asia/Shanghai
```

`--cap-add NET_ADMIN` and `cap_add` let [Intrusion Prevention](/en/docs/modules/guard) write bans to the host's nftables firewall. You can drop them; bans then still apply to Gately's own services.

The image ships with `sh`, `bash` and `curl`, so typical notification scripts run out of the box.

## Binary

1. Get the archive for your platform from the [download page](/en/download): Linux (amd64 / arm64 / arm / mips / mipsle / riscv64), macOS, Windows or FreeBSD.
2. Extract it and run:

```bash
./gately -c /etc/gately
```

### systemd

The repository ships a complete service file, [deploy/systemd/gately.service](https://github.com/wangwanjie/gately/blob/main/deploy/systemd/gately.service), which already grants the capability needed to listen on low ports:

```bash
sudo install -m 0755 gately /usr/local/bin/gately
sudo install -m 0644 gately.service /etc/systemd/system/gately.service
sudo systemctl daemon-reload && sudo systemctl enable --now gately
```

### OpenWrt {#openwrt}

Every release ships OpenWrt packages that contain the binary, the procd init script and a rule that keeps your config across firmware upgrades:

- OpenWrt 24.10 and earlier (including iStoreOS and other 24.10-based firmware) use the `.ipk`, installed with `opkg install`;
- OpenWrt 25.12 and later use the `.apk`, installed with `apk add --allow-untrusted` (the packages are unsigned, so `--allow-untrusted` is required).

Run `grep DISTRIB_ARCH /etc/openwrt_release` to find the device architecture, then pick the package from the OpenWrt group on the [download page](/en/download):

| DISTRIB_ARCH | Package |
| --- | --- |
| `x86_64` | `gately_<version>_openwrt_amd64` |
| `aarch64_*` | `gately_<version>_openwrt_arm64` |
| `arm_cortex-a*_*` (ARMv7 with VFP / NEON) | `gately_<version>_openwrt_arm` |
| `mipsel_*` | `gately_<version>_openwrt_mipsle` |
| `mips_*` | `gately_<version>_openwrt_mips` |
| `riscv64_*` | `gately_<version>_openwrt_riscv64` |

```bash
cd /tmp
# OpenWrt 24.10 / iStoreOS (x86_64 shown)
opkg install ./gately_<version>_openwrt_amd64.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./gately_<version>_openwrt_amd64.apk
```

- The service is enabled at boot and started right after install; the admin UI is at `http://<router address>:16680`. Config lives in `/etc/gately` and is kept across firmware upgrades (sysupgrade);
- Upgrade: download the newer package and run `opkg install` or `apk add --allow-untrusted` again; your config is kept. If you disabled the service at boot, it stays disabled after the upgrade;
- Uninstall: `opkg remove gately` or `apk del gately`. The config directory is kept; run `rm -rf /etc/gately` to remove it as well;
- The device architecture is checked before install. opkg aborts when the architecture does not match; the apk in OpenWrt 25.12 still installs the package but leaves the service stopped, so run `apk del gately` and install the right package;
- The binary is about 32 MB and needs the same amount of free space (check with `df -h /overlay`), which suits x86 soft routers, ARM64 boxes and other devices with ample storage; routers with 16 MB / 32 MB of flash are too small;
- The arm package needs ARMv7 with hardware floating point. ARMv5 / ARMv6 devices and FPU-less cores such as `arm_cortex-a9` are not supported yet.

You can also deploy manually: download the Linux archive and use the procd init script [deploy/openwrt/gately.init](https://github.com/wangwanjie/gately/blob/main/deploy/openwrt/gately.init):

```bash
cp gately /usr/bin/gately && chmod +x /usr/bin/gately
cp gately.init /etc/init.d/gately && chmod +x /etc/init.d/gately
/etc/init.d/gately enable && /etc/init.d/gately start
```

#### LuCI app {#openwrt-luci}

Each release also ships an architecture-independent LuCI app, `luci-app-gately_<version>_openwrt_all.ipk` / `.apk`. Install it after Gately:

```bash
# OpenWrt 24.10 / iStoreOS
opkg install ./luci-app-gately_<version>_openwrt_all.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./luci-app-gately_<version>_openwrt_all.apk
```

Then open **Services → Gately** in LuCI to:

- see the service state, version, memory usage and whether it starts at boot;
- start, stop and restart the service, and enable or disable start at boot;
- open the Gately admin interface in one click, with the safe entrance from the global settings already in the link;
- set the admin port. It is stored in `/etc/config/gately` and, when set, replaces the port of the listen address in the Gately global settings (a host part such as a loopback or LAN address is kept), which helps when the admin interface is unreachable because of a port conflict or a wrong port; leave it empty to follow the global settings.

Everything else is still configured in the Gately admin interface. The app follows the LuCI language and is translated into Simplified Chinese, Traditional Chinese, English, Japanese, Korean, German, French, Spanish, Russian and Portuguese (Brazil). To uninstall, remove `luci-app-gately` first (`opkg remove luci-app-gately` or `apk del luci-app-gately`), then gately.

## fnOS native app {#fnos}

Every release ships offline packages for fnOS: `gately_<version>_fnos_x86.fpk` (Intel / AMD) and `gately_<version>_fnos_arm.fpk` (ARM). They are not listed in the official App Center, so install them manually:

1. Get the `.fpk` for your device's architecture from the [download page](/en/download#fnos);
2. In fnOS, open "App Center → Manual install" (the button at the bottom-left), pick the file and finish the wizard;
3. Click the Gately icon on the fnOS desktop to open the admin UI.

### Works with the fnOS unified gateway

The desktop icon opens the admin UI through the fnOS unified gateway at `/app/gately/` on the address you use for fnOS, sharing the same entry point. Both the LAN address and your remote fnOS domain work, with no need to open or forward port 16680.

Access through the gateway is controlled by fnOS, and only fnOS administrators can open it. The secure entrance path and "LAN-only access" apply only to direct access on port 16680. On the LAN, `http://<NAS address>:16680` still works.

### Sign in with your fnOS administrator session

Turn on "Sign in with fnOS" under "Settings → Access control" and administrators who are signed in to fnOS enter Gately straight from the desktop icon, without a password or two-step code:

- Only fnOS administrators are signed in, as the Gately administrator; regular fnOS users still enter a username and password;
- It applies only to access through the unified gateway; direct access on port 16680 still requires a normal sign-in;
- After you sign out, Gately does not sign you back in silently; the login page shows a "Sign in as fnOS user &lt;name&gt;" button instead;
- The setting is off by default and only shown in the fnOS package.

### Notes

- The app runs as `root`, so it can listen on 80/443, manage Docker through `docker.sock` and open a real Web terminal. Install it only on a NAS you control.
- Config lives in the app's `var` directory and is kept on uninstall unless you choose "delete all" in the uninstall wizard.
- The package also adds a `/usr/local/bin/gately` command for `gately ctl`, `gately mcp` and `gately reset-password`.
- Port 16680 is shared with the Docker image. Stop the `gately` container before starting the native app.

## Initial sign-in

The admin UI listens on port `16680` by default. The default account is `admin` and the default password is `gately.cn`. For automated deployments, set the `GATELY_INITIAL_PASSWORD` environment variable to choose your own password instead of the default.

The default password is public; while it is still in use, the admin UI keeps showing a warning at the top asking you to change it. The first sign-in shows an onboarding guide that walks you through changing the password, enabling two-factor authentication or a Passkey, running the network self-check, importing from Lucky and setting up notifications.

## Reset password and two-factor authentication {#reset}

If you forget the administrator password or lose your two-factor authenticator (TOTP), reset it by running a command on the server where Gately runs. The command needs read and write access to the config directory, which is the same as having access to the server, so it cannot be used over the network:

- `gately reset-password`: resets the administrator password to the default `gately.cn`; add `-p <new password>` to set a new password directly;
- `gately reset-totp`: turns off two-factor authentication for the administrator; you can enable it again after signing in.

Before doing anything, the command shows the administrator username and the action it is about to take, and asks you to type `yes` to confirm so a slip of the keyboard does no harm; in scripts and other non-interactive environments, add `-y` to skip the confirmation. A running Gately applies the change within seconds without a restart; if Gately is not running, the change applies on the next start. A reset invalidates all signed-in sessions and clears login lockouts; the username, Passkeys, read-only accounts and other configuration stay unchanged.

| Install method | Command |
| --- | --- |
| Docker | `docker exec -it gately gately reset-password` |
| systemd | `sudo gately reset-password` |
| OpenWrt / iStoreOS | `gately reset-password` |
| fnOS | `sudo gately reset-password` |
| Other | `./gately reset-password -c <config dir>` |

Without `-c`, the command looks in `GATELY_CONFIG_DIR`, `data` in the current directory, `/etc/gately` and the fnOS package config directory `/var/apps/gately/var/config`, in that order, and prints the directory it uses. Resets are recorded in the system log and the audit log.

## Upgrading

### Automatic update checks and security alerts {#update-check}

By default Gately contacts GitHub every 12 hours to read Gately's release list and [published security advisories](https://github.com/wangwanjie/gately/security/advisories), and queries the latest upstream version of managed programs installed online (frp, Cloudflare Tunnel, Tailscale, File Browser). All requests are anonymous GETs and carry no device, configuration or usage information.

- **New version**: a notice appears at the top of the admin UI; you can view the release notes or skip that version;
- **Security update**: when the current version falls within the affected range of an advisory, or a release newer than the current version has a security-fix section in its notes, a red notice at the top lists the advisories and versions. A security notice can only be postponed for 24 hours, and it returns immediately when a new security issue appears;
- **Notifications**: a new version or a new security issue publishes a `system.update` / `system.security` event, and a new version of a managed program publishes `program.update`. Add a rule from the "Updates" template in [Notifications](/en/docs/modules/notify) to receive them. Each version and each security issue is notified once, and restarts do not repeat it.

The result is saved in `update-check.json` in the config directory and is re-evaluated for the new version after an upgrade, without another check. The first check runs about 3 minutes after startup (or waits until 12 hours have passed since the last successful check), and a failed check is retried after an hour; GitHub is reached through the update proxy described below. Automatic checks can be turned off on the About page or in Settings → Network & runtime, after which GitHub is contacted only when you click Check for updates. Automatic checks only notify you; they never download or install anything.

### Online upgrade {#online-upgrade}

Linux binary installs (systemd, PVE LXC containers, manual OpenWrt installs and so on) can upgrade online from the About page of the admin UI: click "Check for Updates", then "Upgrade now" when a new version is found. This also works for LXC containers created in PVE from the OCI image, so there is no need to delete and recreate them. Gately will:

1. Download `gately_<version>_linux_<arch>.tar.gz` for this platform from GitHub;
2. Verify its SHA-256 against the `checksums.txt` of the same release;
3. Test-run the new binary with `-version` to make sure it runs on this machine;
4. Replace the executable and restart with the same arguments, keeping the previous version as `gately.old` in the same directory.

Everything happens in a dialog: it first shows the release notes, then the current step, download progress and speed once the upgrade starts. Until the binary is replaced (fetching, downloading and verifying) you can click "Cancel upgrade" at any time, which leaves the current binary untouched.

If GitHub is hard to reach, set an **update proxy** on the About page or under Settings → Network & Runtime. It supports `http://`, `https://` and `socks5://`, and both the update check and the download go through it. When empty, Gately connects directly and still honors the `HTTPS_PROXY` environment variable.

Without internet access, get the tar.gz from the [download page](/en/download) and click "Upload upgrade package" on the About page. Uploads accept any version, so you can also roll back to an older release.

Online upgrades only run when an admin starts them; Gately never downloads or installs a version on its own. If the new version fails to start, roll back manually with the kept copy (systemd example):

```bash
mv /usr/local/bin/gately.old /usr/local/bin/gately
systemctl restart gately
```

If you granted low-port access to the binary with `setcap`, run `setcap` again after the upgrade.

### Other deployments

- Docker: `docker pull` the new image and recreate the container (keep the config directory mounted);
- fnOS: manually install the newer `.fpk` from the App Center; your config is kept;
- OpenWrt packages: download the newer `.ipk` / `.apk` and run `opkg install` / `apk add --allow-untrusted`; your config is kept (see [OpenWrt](#openwrt));
- Windows, macOS, FreeBSD: replace the executable and restart the service.

Before upgrading, export a configuration backup or keep the config directory in git. Downloaded archives and images can be checked against their signatures as described in [Verifying Releases](/en/docs/guide/verify).
