---
title: "Download"
description: "Download the latest Gately binaries, fnOS and OpenWrt packages, or deploy with the Docker image."
---

# Download Gately

## Docker image (recommended)

The image is published for amd64, arm64 and arm/v7:

```bash
docker pull ghcr.io/wangwanjie/gately:latest
```

The same image (same tags) is also on Docker Hub:

```bash
docker pull vanjayak/gately:latest
```

See [Installation](/en/docs/guide/install) for the full set of deployment options.

## Binaries

Download the archive for your platform, extract it and run `./gately -c /etc/gately`. Supported platforms: Linux (amd64 / arm64 / arm / mips / mipsle / riscv64), macOS, Windows and FreeBSD.

<DownloadList />

::: tip Slow downloads?
GitHub can be slow on some networks. NAS and soft-router users can pull through a proxy first, or use the Docker image instead.
:::

## fnOS native app {#fnos}

fnOS users can install the native app directly: download the `.fpk` from the fnOS group in the list above (`x86` for Intel / AMD, `arm` for ARM devices), then pick it under "App Center → Manual install" in fnOS.

- Works with the fnOS unified gateway: open it from the desktop icon, also over your remote fnOS domain, with no extra port to open;
- Signs fnOS administrators in with their fnOS session: once enabled, administrators already signed in to fnOS skip the Gately password and two-step code.

See [Installation · fnOS native app](/en/docs/guide/install#fnos).

## OpenWrt packages {#openwrt}

OpenWrt users can download a package from the OpenWrt group in the list above: pick the `.ipk` for OpenWrt 24.10 and earlier (including iStoreOS) and install it with `opkg install`, or the `.apk` for OpenWrt 25.12 and later and install it with `apk add --allow-untrusted`. Choose the architecture by `DISTRIB_ARCH` in `/etc/openwrt_release`. The service starts right after install and keeps its config in `/etc/gately`.

`luci-app-gately_<version>_openwrt_all` in the same group is the LuCI app. It does not depend on the architecture; install it after Gately to manage the service from **Services → Gately** in LuCI.

See [Installation · OpenWrt](/en/docs/guide/install#openwrt).

## Build from source

Requires Go 1.26+, Node.js 24+ and pnpm:

```bash
git clone https://github.com/wangwanjie/gately.git
cd gately
make web      # build the frontend
make build    # outputs bin/gately
```

## Verification

Every release ships with a checksum file. Verify the SHA-256 after downloading and before running.

Since v0.5.0, the checksum file and Docker images carry cosign keyless signatures, archives come with build provenance, and an SBOM is published. See [Verifying release artifacts](/en/docs/guide/verify).
