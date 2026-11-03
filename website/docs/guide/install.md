# 安装部署

## Docker

镜像地址 `ghcr.io/wangwanjie/gately`，同时发布到 Docker Hub 的 `vanjayak/gately`（两处为同一份镜像，标签一致），支持 amd64、arm64、arm/v7。访问 GHCR 不便时，把下文的镜像名换成 `vanjayak/gately:latest` 即可，也可以配合 Docker 的镜像加速使用。

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
    # 端口转发、STUN 穿透、Web 服务都需要直接监听宿主机端口
    network_mode: host
    # 入侵防护把封禁写入宿主机防火墙（不需要可删除）
    cap_add:
      - NET_ADMIN
    volumes:
      # 配置目录（明文 JSON，可纳入 git 备份；ssl.json 中含证书私钥）
      - ./data:/etc/gately
      # Docker 管理（不需要可删除）
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - TZ=Asia/Shanghai
```

`--cap-add NET_ADMIN` 与 `cap_add` 供[入侵防护](/docs/modules/guard)把封禁写入宿主机的 nftables 防火墙，不需要可以去掉，封禁仍对 Gately 自身的服务生效。

镜像内置 `sh`、`bash` 与 `curl`，常见的通知脚本可以直接运行。

## 二进制

1. 从 [下载页](/download) 获取对应平台的压缩包：Linux（amd64 / arm64 / arm / mips / mipsle / riscv64）、macOS、Windows、FreeBSD。
2. 解压后运行：

```bash
./gately -c /etc/gately
```

### systemd

仓库提供完整的服务文件 [deploy/systemd/gately.service](https://github.com/wangwanjie/gately/blob/main/deploy/systemd/gately.service)，已授予监听低位端口所需的能力：

```bash
sudo install -m 0755 gately /usr/local/bin/gately
sudo install -m 0644 gately.service /etc/systemd/system/gately.service
sudo systemctl daemon-reload && sudo systemctl enable --now gately
```

### OpenWrt {#openwrt}

每个版本附带 OpenWrt 安装包，包含程序、procd 启动脚本和刷写固件时保留配置的规则：

- OpenWrt 24.10 及以前（含 iStoreOS 等基于 24.10 的固件）使用 `.ipk`，执行 `opkg install` 安装；
- OpenWrt 25.12 起使用 `.apk`，执行 `apk add --allow-untrusted` 安装（安装包没有签名，需要加 `--allow-untrusted`）。

先执行 `grep DISTRIB_ARCH /etc/openwrt_release` 查看设备架构，再按下表在 [下载页](/download) 的 OpenWrt 分组中选择安装包：

| DISTRIB_ARCH | 安装包 |
| --- | --- |
| `x86_64` | `gately_<版本>_openwrt_amd64` |
| `aarch64_*` | `gately_<版本>_openwrt_arm64` |
| `arm_cortex-a*_*`（ARMv7，带 VFP / NEON） | `gately_<版本>_openwrt_arm` |
| `mipsel_*` | `gately_<版本>_openwrt_mipsle` |
| `mips_*` | `gately_<版本>_openwrt_mips` |
| `riscv64_*` | `gately_<版本>_openwrt_riscv64` |

```bash
cd /tmp
# OpenWrt 24.10 / iStoreOS（以 x86_64 为例）
opkg install ./gately_<版本>_openwrt_amd64.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./gately_<版本>_openwrt_amd64.apk
```

- 安装后自动启用开机自启并启动，管理界面为 `http://<路由器地址>:16680`。配置目录 `/etc/gately`，刷写固件（sysupgrade）时自动保留；
- 升级：下载新版安装包，同样执行 `opkg install` 或 `apk add --allow-untrusted`，配置保留。停用了开机自启的，升级后保持停用；
- 卸载：`opkg remove gately` 或 `apk del gately`。配置目录会保留，彻底删除再执行 `rm -rf /etc/gately`；
- 安装前会核对设备架构。装错架构时 opkg 会中止安装；OpenWrt 25.12 的 apk 仍会装上但不启动服务，执行 `apk del gately` 后换用正确的安装包；
- 程序约 32 MB，安装需要同等大小的可用空间（`df -h /overlay` 查看），适合 x86 软路由、ARM64 盒子等存储较大的设备，16 MB / 32 MB 闪存的路由器装不下；
- arm 安装包需要 ARMv7 且带硬件浮点，ARMv5 / ARMv6 以及 `arm_cortex-a9` 这类没有浮点单元的设备暂不支持。

也可以手动部署：下载 Linux 压缩包，配合 procd 启动脚本 [deploy/openwrt/gately.init](https://github.com/wangwanjie/gately/blob/main/deploy/openwrt/gately.init)：

```bash
cp gately /usr/bin/gately && chmod +x /usr/bin/gately
cp gately.init /etc/init.d/gately && chmod +x /etc/init.d/gately
/etc/init.d/gately enable && /etc/init.d/gately start
```

#### LuCI 插件 {#openwrt-luci}

每个版本另附与架构无关的 LuCI 插件 `luci-app-gately_<版本>_openwrt_all.ipk` / `.apk`，装好 Gately 后再安装：

```bash
# OpenWrt 24.10 / iStoreOS
opkg install ./luci-app-gately_<版本>_openwrt_all.ipk
# OpenWrt 25.12
apk add --allow-untrusted ./luci-app-gately_<版本>_openwrt_all.apk
```

安装后在 LuCI 的「服务 → Gately」中可以：

- 查看运行状态、版本、内存占用与开机自启状态；
- 启动、停止、重启服务，启用或停用开机自启；
- 一键打开 Gately 管理界面，链接自动带上全局设置中的安全入口；
- 设置管理界面端口。端口保存在 `/etc/config/gately`，填写后替换 Gately 全局设置中监听地址的端口（只监听本机或内网地址的设置保持不变），端口冲突或改错端口打不开管理界面时可以在这里改回；留空则以全局设置为准。

其余功能仍在 Gately 管理界面中配置。插件界面跟随 LuCI 的语言设置，支持简体中文、繁体中文、英语、日语、韩语、德语、法语、西班牙语、俄语和葡萄牙语（巴西）。卸载时先执行 `opkg remove luci-app-gately` 或 `apk del luci-app-gately`，再卸载 gately。

## 飞牛 fnOS 原生应用 {#fnos}

每个版本附带飞牛 fnOS 的离线安装包：`gately_<版本>_fnos_x86.fpk`（Intel / AMD）与 `gately_<版本>_fnos_arm.fpk`（ARM）。安装包没有上架官方应用中心，需要手动安装：

1. 从 [下载页](/download#fnos) 获取与设备架构对应的 `.fpk`；
2. 在飞牛打开「应用中心 → 手动安装」（左下角按钮），选择文件并完成向导；
3. 点击飞牛桌面上的 Gately 图标打开管理界面。

### 支持飞牛统一网关

桌面图标经飞牛统一网关打开管理界面，地址为访问飞牛所用的地址加 `/app/gately/`，与飞牛共用同一个访问入口。局域网地址、远程访问飞牛的域名都能直接使用，无需额外开放或映射 16680 端口。

经网关访问时由飞牛负责访问控制，只有飞牛管理员能打开；安全入口与「仅允许内网访问」只约束直接访问 16680 端口。局域网内仍可直接访问 `http://<NAS 地址>:16680`。

### 支持飞牛管理员登录态直接登录

在「设置 → 访问控制」开启「飞牛登录态自动登录」后，已登录飞牛的管理员点击桌面图标即可直接进入 Gately，无需再输入密码与两步验证码：

- 只有飞牛管理员会以 Gately 管理员身份登录，普通飞牛用户仍需输入账号密码；
- 只对经统一网关的访问生效，直接访问 16680 端口时仍需正常登录；
- 主动退出后不会静默重新登录，登录页会显示「使用飞牛账号 &lt;用户名&gt; 登录」按钮；
- 该开关默认关闭，只在飞牛安装包中显示。

### 运行说明

- 应用以 `root` 身份运行，可以监听 80/443、通过 `docker.sock` 管理 Docker、使用真实的 Web 终端。请只安装在自己掌控的 NAS 上。
- 配置保存在应用的 `var` 目录，卸载时默认保留；在卸载向导中选择「全部删除」才会清除。
- 安装包同时提供命令行入口 `/usr/local/bin/gately`，可使用 `gately ctl`、`gately mcp` 与 `gately reset-password`。
- 16680 端口与 Docker 镜像相同，启动原生应用前请先停止 `gately` 容器。

## 初始登录

管理界面默认监听 `16680` 端口，默认账号 `admin`，默认密码 `gately.cn`。自动化部署时可用环境变量 `GATELY_INITIAL_PASSWORD` 指定初始密码代替默认密码。

默认密码是公开的，仍在使用时管理界面顶部会一直提示修改。首次登录会出现启动引导，依次提示修改密码、开启两步验证或 Passkey、运行网络自检、从 Lucky 导入与配置通知。

## 重置密码与两步验证 {#reset}

忘记管理员密码、丢失两步验证器（TOTP）时，在运行 Gately 的服务器上执行命令重置。命令需要读写配置目录，等同于拥有服务器权限，因此无法通过网络使用：

- `gately reset-password`：把管理员密码重置为默认密码 `gately.cn`，用 `-p <新密码>` 可直接指定新密码；
- `gately reset-totp`：关闭管理员的两步验证，登录后可重新启用。

执行前会显示管理员用户名和将要进行的操作，需要输入 `yes` 确认，防止误操作；脚本等非交互环境加 `-y` 跳过确认。运行中的 Gately 会在几秒内自动生效，无需重启；未在运行时，下次启动时生效。重置后所有已登录会话失效，登录失败锁定一并解除，用户名、Passkey、只读账号与其他配置保持不变。

| 安装方式 | 命令 |
| --- | --- |
| Docker | `docker exec -it gately gately reset-password` |
| systemd | `sudo gately reset-password` |
| OpenWrt / iStoreOS | `gately reset-password` |
| 飞牛 fnOS | `sudo gately reset-password` |
| 其他 | `./gately reset-password -c <配置目录>` |

未指定 `-c` 时依次查找 `GATELY_CONFIG_DIR`、当前目录下的 `data`、`/etc/gately` 与飞牛安装包的配置目录 `/var/apps/gately/var/config`，并显示实际使用的目录。重置操作会记入系统日志与审计日志。

## 升级

### 自动检查更新与安全提醒 {#update-check}

Gately 默认每 12 小时访问一次 GitHub，读取 Gately 的发布列表与[已公开的安全公告](https://github.com/wangwanjie/gately/security/advisories)，并为已在线安装的托管程序（frp、Cloudflare 隧道、Tailscale、NetBird、File Browser）查询上游最新版本。请求都是匿名 GET，不携带设备、配置或使用情况等任何信息。

- **新版本**：管理界面顶部显示提示，可以查看更新内容，或忽略这个版本；
- **安全更新**：当前版本落在某个安全公告的受影响版本范围内，或者比当前版本新的发布说明带有安全修复章节时，顶部以红色提示并列出相关公告与版本。安全提示只能暂缓 24 小时，出现新的安全问题时立即重新提示；
- **通知**：发现新版本或新的安全问题时发布 `system.update` / `system.security` 事件，托管程序有新版本时发布 `program.update`，在[通知中心](/docs/modules/notify)用「版本更新」模板添加规则即可接收。同一版本、同一安全问题只通知一次，重启后也不会重复。

检查结果保存在配置目录的 `update-check.json`，升级后按新的版本重新判断，无需再次检查。启动后约 3 分钟进行首次检查（距上次成功检查不足 12 小时时等到满 12 小时），失败时 1 小时后重试；访问 GitHub 同样经过下文的更新代理。在「关于」页或「设置 → 网络与运行」中可以关闭自动检查，关闭后只在点击「检查更新」时访问 GitHub。自动检查只负责提示，不会下载或安装任何版本。「检查更新」按钮只对管理员显示；只读账号与只有读权限的 API 令牌调用检查接口时直接返回上次的结果，不访问 GitHub。

### 在线升级 {#online-upgrade}

Linux 二进制部署（systemd、PVE LXC 容器、手动部署的 OpenWrt 等）可以在管理界面的「关于」页在线升级：点击「检查更新」，发现新版本后点击「立即升级」。在 PVE 中用 OCI 镜像创建的 LXC 容器同样适用，无需删除重建。Gately 会依次：

1. 从 GitHub 下载与本机平台对应的 `gately_<版本>_linux_<架构>.tar.gz`；
2. 对照同一版本的 `checksums.txt` 校验 SHA-256；
3. 以 `-version` 试运行新程序，确认能在本机执行；
4. 替换可执行文件并以相同参数重启，旧版本保留为同目录下的 `gately.old`。

整个过程在弹窗中进行：先展示该版本的更新内容，开始后显示当前步骤、下载进度与速度。替换程序之前（获取版本、下载、校验阶段）可以随时点击「取消升级」，取消不会改动现有程序。

访问 GitHub 不稳定时，可以在「关于」页或「设置 → 网络与运行」中设置**更新代理**，支持 `http://`、`https://` 与 `socks5://`，检查更新与下载都经过该代理。留空时直连，仍会遵循 `HTTPS_PROXY` 环境变量。

无法联网时，从 [下载页](/download) 获取 tar.gz，在「关于」页点击「上传升级包」即可升级。上传不限制版本高低，也可以用来回退到旧版本。

[定制构建](/docs/modules/#custom-build)不提供在线升级：在线升级下载的是完整版本，会装回去掉的模块。新版本发布后请到 [build.gately.cn](https://build.gately.cn) 用相同选项（「关于」页的链接已带上）重新生成，再上传升级包；Docker 定制镜像直接拉取新标签。

在线升级只由管理员手动触发，Gately 不会自动下载或安装任何版本。新版本启动异常时，可以用保留的旧版本手动回退（以 systemd 部署为例）：

```bash
mv /usr/local/bin/gately.old /usr/local/bin/gately
systemctl restart gately
```

用 `setcap` 给程序授予过低位端口权限的，替换后需要重新执行一次 `setcap`。

### 其他部署方式

- Docker：`docker pull` 新镜像后重建容器（配置目录保持挂载即可）；
- 飞牛 fnOS：在应用中心手动安装新版 `.fpk`，配置自动保留；
- OpenWrt 安装包：下载新版 `.ipk` / `.apk` 后执行 `opkg install` / `apk add --allow-untrusted`，配置自动保留（见 [OpenWrt](#openwrt)）；
- Windows、macOS、FreeBSD：替换可执行文件并重启服务。

升级前建议导出一份配置备份，或直接把配置目录纳入 git。下载的压缩包与镜像可按 [校验发布产物](/docs/guide/verify) 核对签名。
