# 协议分流

多个服务共用同一个公网端口（例如网站、VPN、SSH 和远程桌面都用 443）。Gately 读取客户端连接后最先发送的数据识别协议，TLS 再看握手里的域名（SNI）与 ALPN，HTTP 再看 Host，然后把整条连接透传给对应的后端。这里不解密、不终止任何协议，TLS 证书仍放在后端。作用等同于 haproxy 的 `req_ssl_sni` 规则加上 sslh。

按域名分流（SNI 分流）是其中的一种情况：添加 TLS 路由并填写域名即可。

每条规则包含：

- 监听：协议（仅 TCP，tcp 为双栈，tcp4 / tcp6 为单协议族）、监听地址与端口（支持 `443`、`443,8443` 或端口段）；
- 路由：每条路由有协议、名称、目标地址（多个地址依次尝试，带故障转移）、可选的目标端口（留空与监听端口相同），以及 PROXY 协议设置（不发送、v1、v2）。PROXY 设置属于路由，因为有的后端要求它，有的后端会拒绝它；
- 默认目标：规则自身的目标地址、目标端口与 PROXY 协议设置，未命中路由或无法识别协议时使用；
- 识别超时：等待客户端发来可识别数据的秒数，0 为默认 5 秒。

## 可识别的协议

| 路由协议 | 识别依据 | 进一步匹配 |
| --- | --- | --- |
| TLS | TLS 握手记录（ClientHello） | 域名（SNI）、ALPN |
| HTTP | 请求行的方法名与 `HTTP/` 版本号，包括 CONNECT、WebDAV 方法与 HTTP/2 明文直连（h2c） | 域名（Host 头） |
| SSH | 客户端版本标识 `SSH-` | — |
| RDP 远程桌面 | TPKT 头与 X.224 连接请求 | — |
| SOCKS4 / SOCKS5 | SOCKS4、SOCKS4a 请求与 SOCKS5 方法协商 | — |
| OpenVPN（TCP） | 客户端硬重置包，包括 tls-auth、tls-crypt 与 tls-crypt-v2 | — |
| XMPP | XML 声明或 `<stream:stream` | — |
| MQTT | CONNECT 报文（3.1、3.1.1、5.0） | — |
| PostgreSQL | 启动消息、SSLRequest 与 GSSENCRequest | — |
| Minecraft（Java 版） | 握手包；1.6 及更早版本的服务器列表查询 | 服务器地址 |
| RTSP | 请求行版本为 `RTSP/1.0` 或 `RTSP/2.0`（网络摄像头、NVR） | — |
| SIP | 请求行版本为 `SIP/2.0`（TCP） | — |
| Redis | 以 RESP 数组发送的命令 | — |
| AMQP | 协议头 `AMQP`（0-9-1 与 1.0，RabbitMQ 等） | — |
| SMB | NetBIOS 会话报文中的 SMB1、SMB2、SMB3 协商请求 | — |
| Git | `git://` 协议的首个 pkt-line（git daemon） | — |
| IRC | 客户端先发送的 CAP、PASS、NICK 或 USER 命令 | — |
| ADB | Android 调试桥的 CNXN 消息（adb connect） | — |
| BitTorrent | 未加密的对等握手 | — |
| 等待超时 | 识别超时内客户端没有发送任何数据 | — |

## 匹配规则

TLS、HTTP 与 Minecraft 路由按域名匹配（Minecraft 用握手包中客户端填写的服务器地址，Forge、BungeeCord、TCPShield 追加的内容会被忽略）：精确域名（`vpn.example.com`）优先，其次最长的通配（`*.example.com`，匹配任意级子域名），再次 `*`（匹配任何携带域名的连接），最后是没有填写域名的路由（匹配该协议的全部连接，包括没有 SNI 或 Host 的）。域名不区分大小写。

TLS 路由还可以填写 ALPN：客户端声明的 ALPN 含其中任一项时才匹配。同一个域名下填写了 ALPN 的路由优先，ALPN 不匹配时继续按更宽泛的域名查找。例如把 `acme-tls/1` 交给负责 TLS-ALPN-01 证书验证的程序，把 `postgresql` 交给 PostgreSQL 17 的直接 TLS 连接。同一协议下，同一个域名（及同一个 ALPN）只能属于一条路由；其他协议每种只能有一条路由。

「等待超时」用于 VNC、MySQL、FTP、SMTP、POP3、IMAP 这类由服务端先发送数据的协议：客户端连上后一直等服务端发言，Gately 收不到任何数据，也就分辨不出是哪一种。所以每个端口只能把其中一种交给「等待超时」路由，这类连接要等到识别超时才转发。识别超时可以带一位小数：TLS、SSH、HTTP 等客户端在握手后几毫秒内就会发出首包，有「等待超时」路由时设为 0.3～1 秒即可，VNC 等连接也就只等这么久。

以下连接交给默认目标：没有匹配的路由；数据不属于上表任何协议（例如 Shadowsocks、VMess、启用加密的 BitTorrent 这类没有特征的协议，以及 MongoDB、DNS over TCP 这类首包只有长度字段、容易和其他协议混淆的协议）；ClientHello 无法解析；没有「等待超时」路由时客户端一直不发数据。没有设置默认目标时，这类连接会被断开，并计入卡片上的「已断开」。客户端在识别完成前断开时直接丢弃。

## 说明

- 只处理 TCP。HTTP/3（QUIC）、WireGuard 等 UDP 流量不会被分流，请为它单独建一条规则；
- 上表中 SIP、Redis、AMQP、IRC 等协议的加密版本走 TLS，用 TLS 路由按域名或 ALPN 分流；
- ClientHello 可以被拆成多个 TLS 记录，最大支持 64 KB；HTTP、RTSP、SIP 的请求行最长 8 KB，HTTP 最多读取 32 KB 请求头查找 Host；
- 每个连接只识别一次，同一条 HTTP 长连接上的后续请求都发往第一次选中的后端；
- 规则卡片上会显示每条路由与默认目标收到的连接数；运行日志的调试级别记录每个连接识别出的协议、SNI、ALPN 或 Host；
- 修改路由、默认目标与识别超时后立即生效，监听不中断，已建立的连接不受影响。

## 示例：替换 haproxy

这样的 haproxy 配置：

```text
frontend https
    bind 192.0.2.10:443
    mode tcp
    tcp-request inspect-delay 5s
    tcp-request content accept if { req_ssl_hello_type 1 }
    use_backend ocserv if { req_ssl_sni -i tz.example.com }
    default_backend web
backend ocserv
    server ocserv 127.0.0.1:443 send-proxy-v2
backend web
    server web 127.0.0.1:8443 send-proxy-v2
```

对应一条 Gately 规则：

| 设置 | 值 |
| --- | --- |
| 协议 | TCP |
| 监听地址 / 端口 | `192.0.2.10` / `443` |
| 路由 `ocserv` | 协议 TLS，域名 `tz.example.com`，目标 `127.0.0.1`，端口 `443`，PROXY v2 |
| 默认目标 | `127.0.0.1`，目标端口 `8443`，PROXY v2 |

监听地址请填服务器的公网地址，避免与监听 `127.0.0.1:443` 的后端冲突。要用 Gately 自己提供网站，可以再添加一个监听在 `127.0.0.1:8443` 的 [Web 服务](/docs/modules/web) 监听，开启「接收 PROXY 协议」并把 `127.0.0.0/8` 加入可信 PROXY 来源。启用规则前请先停止 haproxy，同一个端口只能由一个程序监听。

## 示例：替换 sslh

这样的 sslh 启动参数：

```text
sslh --listen 0.0.0.0:443 --timeout 2 \
     --ssh 127.0.0.1:22 --openvpn 127.0.0.1:1194 --tls 127.0.0.1:8443
```

对应一条 Gately 规则：

| 设置 | 值 |
| --- | --- |
| 协议 | TCP |
| 监听地址 / 端口 | 留空 / `443` |
| 识别超时 | `2` |
| 路由 `ssh` | 协议 SSH，目标 `127.0.0.1`，端口 `22` |
| 路由 `vpn` | 协议 OpenVPN，目标 `127.0.0.1`，端口 `1194` |
| 路由 `web` | 协议 TLS，域名留空，目标 `127.0.0.1`，端口 `8443` |
| 路由 `vnc`（可选） | 协议「等待超时」，目标 `127.0.0.1`，端口 `5900` |

sslh 在超时后默认把连接交给 SSH；Gately 按 `SSH-` 版本标识立即识别 SSH，不必等待，超时的连接可以交给 VNC 这类服务端先发言的服务。还可以继续添加 RDP、SOCKS、HTTP 等路由，或按域名把 TLS 分给不同的后端。

## 从端口转发升级

协议分流原先是端口转发规则的「SNI 分流」模式。升级后首次启动时，端口转发中的 SNI 分流规则会自动移到协议分流模块，规则的 key、名称、启用状态、分组和全部路由保持不变，原路由都成为 TLS 路由；端口转发模块被关闭的，协议分流模块也一并关闭；拥有端口转发权限的 API 令牌会同时获得协议分流的同级权限。

迁移后请改用 `/api/protoroute/rules` 管理这些规则：路由字段由 `sniRoutes` 改名为 `routes`，不再使用 `mode` 字段；向 `/api/portforward/rules` 提交分流规则会被拒绝。监控指标也改为 `gately_protoroute_*`，见[监控](/docs/reference/monitoring)。

## 部署提示

协议分流直接监听宿主机端口，Docker 部署推荐使用 `--network host`。

## 分组与排序

规则可以填写「分组」，列表的分组、排序与搜索用法和[端口转发](/docs/modules/portforward#分组与排序)相同；搜索框可按名称、分组、监听端口、目标、域名或路由协议过滤。
