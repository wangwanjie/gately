---
video: true
time: 15 分钟
description: 在局域网里用 http://nas.example.com 这样的域名访问 NAS 或其他应用：添加 HTTP 监听与反向代理站点，用路由器 DNS 或 hosts 让域名指向 Gately，验证访问，并查看站点统计与访问日志。
---

# 第一个反向代理站点

家里的 NAS、Home Assistant、Jellyfin 各占一个端口，记 `192.168.1.10:5000`、`192.168.1.11:8123` 很麻烦。反向代理让你用域名访问它们：浏览器访问 `http://nas.example.com`，Gately 按域名把请求转给 `192.168.1.10:5000`，再把响应送回来。本文在局域网里完成第一个站点，讲清楚「监听」和「站点」是什么关系，最后看看统计与访问日志在哪里。做完之后再接着[申请 HTTPS 证书](/docs/tutorials/https-certificate)，把站点升级成 HTTPS。

::: tip 可以先在演示里看看
在线演示 <https://demo.gately.cn> 的「Web → Web 服务」里有一批示例监听与站点，点「演示账号一键进入」即可浏览。
:::

## 适用场景与前提

- Gately 已安装并登录过管理界面，还没做的话先看[安装后的第一步](/docs/tutorials/getting-started)；
- Gately 所在设备的局域网 IP，本文用 `192.168.1.2`；要代理的应用地址，本文用群晖 DSM 的 `http://192.168.1.10:5000`；
- 一个域名。只在局域网用时随便起一个也行，本文用 `nas.example.com`，请换成你自己的域名；
- Gately 能监听 80 端口：Docker 部署使用 `--network host`；二进制部署用 root 运行，或使用自带的 systemd 服务文件（已授予监听低位端口的能力）。80 端口被其他程序占用时改用 8080 等端口，访问时带上端口号。

## 结果是什么样

```text
浏览器 ── http://nas.example.com ──▶ Gately 192.168.1.2:80
                                       │ 监听「局域网 HTTP」:80
                                       │   └─ 站点 nas.example.com（反向代理）
                                       ▼
                                  NAS 192.168.1.10:5000
```

Web 服务的配置分三层：

- **监听**：Gately 打开的一个端口，决定协议是 HTTP 还是 HTTPS。一个监听可以挂很多站点；
- **站点**：监听下按域名匹配的一条规则，决定请求交给谁处理。类型可以是「反向代理」「重定向」「静态文件」「导航面板」「固定文本」「断开连接」；
- **默认站点**：每个监听都有一个，处理没有匹配到任何站点的请求。没有配置或未启用时返回 404，也可以设成「断开连接」，让访问未知域名的请求得不到任何响应。

以后想加 `ha.example.com`、`jellyfin.example.com`，在同一个监听里再加站点就行，端口还是 80。

## 步骤

### 1. 添加 HTTP 监听

1. 打开侧边栏「Web → Web 服务」，点页头的「添加监听」。

   ![Web 服务页头的「添加监听」](/tutorials/first-site-toolbar.zh-CN.webp)

2. 填写：
   - 名称：`局域网 HTTP`；
   - 监听地址：留空，表示监听本机所有地址；
   - 端口：`80`；
   - HTTPS：关闭。HTTPS 留到[下一篇](/docs/tutorials/https-certificate)再开；
   - 启用：保持开启。

   ![添加监听：端口 80，关闭 HTTPS](/tutorials/first-site-listener.zh-CN.webp)

3. 点「保存」。列表里出现「局域网 HTTP」卡片，标签是 `http://*:80` 和「双栈」，右侧显示「运行中」。

「高级设置」里的「网络」可以选「双栈」「仅 IPv4」「仅 IPv6」，局域网使用保持「双栈」即可。保存后提示「端口 80 已被其他程序占用」时，把端口改成 `8080` 再保存，或者先停掉占用 80 端口的程序（可以用 `ss -ltnp | grep :80` 查看）。

### 2. 添加反向代理站点

1. 在「局域网 HTTP」卡片底部点「添加站点」。
2. 在「基本」标签填写：
   - 域名：`nas.example.com`。一个站点可以写多个域名，每行一个；也支持 `*.example.com` 这样的通配；
   - 名称：`NAS`，用于列表、统计与日志中显示；
   - 类型：「反向代理」；
   - 后端地址：`http://192.168.1.10:5000`，写完整的协议、主机与端口；
   - 选项：保持勾选「添加 X-Forwarded-*」，后端能从请求头里拿到访客的真实 IP 和访问用的域名；
   - 访问日志：打开。新建的站点默认不记访问日志，打开后才能在第 5 步看到每一次请求。

   ![添加站点：nas.example.com 反向代理到 192.168.1.10:5000](/tutorials/first-site-site.zh-CN.webp)

3. 点「保存」。站点出现在监听卡片里，域名是一个可点击的链接，右侧依次是请求数、错误数与「统计」「连接」「日志」「编辑」「复制站点」「删除站点」按钮。

   ![建好后的监听卡片](/tutorials/first-site-card.zh-CN.webp)

修改站点并保存时，监听保持运行，已有连接（包括 WebSocket）不会断开。WebSocket 自动转发。

后端用 HTTPS 和自签名证书时（例如 DSM 的 5001 端口、Proxmox VE 的 8006 端口），后端地址写 `https://...`，并在「高级选项」里勾选「跳过后端证书校验」。

::: details 也可以用「快速建站」一次完成
页头的「快速建站」把上面两步合成三步向导：填「访问用的域名」→ 填「后端地址」（可以点「测试连通」，也可以先点常见应用模板，自动填好端口与推荐选项）→ 选 HTTP 或 HTTPS。监听会自动复用已有的 80 / 443 监听，没有就新建。完成页给出访问地址以及域名还没指向这台设备时的做法。

![快速建站第二步：后端地址与常见应用模板](/tutorials/first-site-quick.zh-CN.webp)

快速建站同样不打开访问日志，需要时在站点的「编辑」里打开。
:::

### 3. 让域名指向 Gately

浏览器访问 `nas.example.com` 之前要先把它解析成 IP，这一步决定请求能不能到达 Gately。局域网里有两种做法，任选一种：

**做法一：在路由器或局域网 DNS 上添加记录（推荐，全家设备都生效）**

在路由器的「自定义 DNS / 静态域名 / 主机名映射」一类设置里添加：

```text
nas.example.com  →  192.168.1.2
```

OpenWrt 在「网络 → DHCP/DNS → 主机名」里添加；用 AdGuard Home 或 Pi-hole 的，在「DNS 重写」或「本地 DNS 记录」里添加。想让 `*.example.com` 全部指向 Gately，AdGuard Home 的 DNS 重写支持写 `*.example.com`。

**做法二：只改自己电脑的 hosts 文件（适合先试一下）**

在 hosts 文件末尾加一行：

```text
192.168.1.2  nas.example.com
```

hosts 文件的位置：

| 系统 | 路径 | 编辑方式 |
| --- | --- | --- |
| Windows | `C:\Windows\System32\drivers\etc\hosts` | 以管理员身份运行记事本后打开 |
| macOS / Linux | `/etc/hosts` | `sudo nano /etc/hosts` |

保存后刷新 DNS 缓存：Windows 执行 `ipconfig /flushdns`，macOS 执行 `sudo dscacheutil -flushcache; sudo killall -HUP mDNSResponder`。

域名托管在公网 DNS 服务商时，也可以直接添加一条 A 记录指向 `192.168.1.2`。内网地址写进公网 DNS 后，任何人都能查到这条记录，但只有你的局域网能连上。

### 4. 访问站点

在浏览器地址栏输入 `http://nas.example.com`，看到 NAS 的登录页就成功了。也可以直接点监听卡片上的域名链接。

部分浏览器会自动尝试 HTTPS，地址栏里请完整输入 `http://`。

### 5. 查看站点统计与访问日志

**站点统计**：点站点行的「统计」按钮，弹出「站点统计 · NAS」：

- 「1 小时 / 6 小时 / 24 小时」切换时间范围，每分钟一个点，重启 Gately 或修改该监听后重新计数；
- 顶部是请求数、累计响应流量、4xx 与 5xx 数量；
- 下面是请求数、错误（4xx / 5xx）与首字节延迟（P50 / P95）曲线；
- 「后端」显示每个后端的健康状态、连接数与请求数；
- 「来源 IP Top 10」与「国家 Top 10」（国家需要导入 GeoIP 数据库）。

![站点统计](/tutorials/first-site-stats.zh-CN.webp)

**访问日志**：点站点行的「日志」按钮，弹出「访问日志 · NAS」，已经按站点和监听筛选好。每行依次是时间、级别、来源（监听名）、客户端 IP、方法、域名与路径、状态码、耗时、Referer 和 User-Agent。可以按级别、关键词、IP、状态码筛选，点「导出」下载 CSV 或 JSON Lines。

![站点的访问日志](/tutorials/first-site-logs.zh-CN.webp)

Web 服务页底部的「运行日志」卡片显示所有监听与站点的日志，监听启动失败等错误也记在这里。点日志里的「站点=NAS」标签可以只看这个站点。筛选写法见[运行日志的查看与筛选](/docs/guide/configuration#log-filter)。

**实时连接**：「连接」按钮列出当前正在进行的请求与 WebSocket，可以单个断开，详见[实时连接](/docs/modules/web#connections)。

### 6. 多个后端（可选）

同一个应用跑了多份时，在「后端地址」里每行写一个，Gately 默认轮询分发：

```text
http://192.168.1.10:8080
http://192.168.1.11:8080
```

「流量」标签里的「负载均衡」可以改策略、设置权重与备用后端，「健康检查」会自动摘除不可用的后端。家用场景一个后端就够了，细节见 [Web 服务 · 反向代理增强](/docs/modules/web)。

## 验证

不改 DNS 也能先确认 Gately 这边配好了，用 `Host` 头模拟域名访问：

```bash
# 返回 NAS 的响应（200 或 30x），说明站点匹配成功
curl -sI -H 'Host: nas.example.com' http://192.168.1.2/ | head -n 1

# 换一个没有配置的域名，返回 404，说明请求落到了默认站点
curl -sI -H 'Host: unknown.example.com' http://192.168.1.2/ | head -n 1

# 改好 DNS 或 hosts 之后，确认域名解析到 Gately
ping -c 1 nas.example.com
curl -sI http://nas.example.com/ | head -n 1
```

界面上：监听卡片右上角是「运行中」，站点行的请求数随访问增加，「日志」里能看到自己电脑 IP 的请求。

## 常见问题

**监听卡片显示「未运行」。** 保存时会提示「配置已保存，但监听没有启动」并附上原因，页面底部的运行日志里也有记录。常见原因有三个：「端口 80 已被其他程序占用」，换端口或停掉占用的程序；「没有权限监听端口 80」，以 root 运行、授予 `CAP_NET_BIND_SERVICE`，或者换成 1024 以上的端口；「本机没有这个监听地址」，把「监听地址」留空。

**访问域名得到 404。** 请求到了 Gately，但没有匹配到站点，交给了默认站点。检查站点的域名有没有拼错，是否用了站点所在监听的端口（例如站点挂在 8080 监听上，就要访问 `http://nas.example.com:8080`），以及站点和监听是否都已启用。

**访问域名得到 502「无法连接到后端服务」。** Gately 连不上后端。在 Gately 所在设备上执行 `curl -I http://192.168.1.10:5000` 确认能通；后端是 HTTPS 时地址要写 `https://`；Docker 部署没有用 host 网络时，容器里的 `127.0.0.1` 指的是容器自己，后端地址要写宿主机的局域网 IP。

**浏览器提示找不到服务器。** 域名没有解析到 Gately。用 `ping nas.example.com` 看解析结果，确认 hosts 或路由器记录已保存、DNS 缓存已刷新；手机等设备可能使用了自己的加密 DNS，需要在系统设置里关掉「私人 DNS」或改用路由器 DNS。

**登录后页面跳回了 `192.168.1.10:5000` 这样的原始地址。** 后端应用按自己的配置生成跳转地址。在应用里把「外部访问地址」「服务器地址」一类设置改成 `http://nas.example.com`，具体看应用的反向代理说明。

**看不到访问日志。** 站点默认不记录访问日志，在站点的「编辑 → 基本」里打开「访问日志」。日志工具栏的「隐藏后台自身请求」默认开启，用浏览器访问 Gately 后台自身产生的请求不显示。

## 下一步

<TutorialVideo name="first-site" caption="演示：用「快速建站」发布第一个站点，再查看、编辑与统计（约 1 分钟）" />

- [申请 HTTPS 证书](/docs/tutorials/https-certificate)：给 `nas.example.com` 换上 HTTPS，并让 HTTP 自动跳转；
- [动态域名入门](/docs/tutorials/ddns-basics)：从外网用域名访问时，让域名跟着家里的公网 IP 走；
- [给站点加上访问控制](/docs/tutorials/site-access-control)：只允许内网访问或加登录；
- [给 Jellyfin、Immich、Home Assistant、Vaultwarden 加 HTTPS 和登录](/docs/tutorials/https-and-login-for-apps)：常见应用的推荐设置；
- 参考文档：[Web 服务](/docs/modules/web)、[快速建站](/docs/modules/web#quick-site)、[站点统计](/docs/modules/web#site-stats)。
