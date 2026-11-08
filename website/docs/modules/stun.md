# STUN 穿透

当运营商没有提供固定公网端口时，STUN 穿透通过 NAT 打洞获取公网映射地址与端口，让外网可以访问内网服务。

## 能力

- TCP / UDP 穿透与保活；
- UPnP / NAT-PMP 自动在路由器上创建端口映射；
- 内置转发，把穿透得到的公网端口直接转发到内网目标；
- 公网地址或端口变化时触发 Webhook 与脚本，可用于更新 DNS、通知客户端等；Webhook 可以经代理发送（全局更新代理或自定义代理）。
- 内置转发的来源 IP 访问控制（白名单 / 黑名单）。

## 搭配使用

STUN 得到的公网端口通常会变化，建议配合：

- [动态域名](/docs/modules/ddns) 或 Webhook / 脚本，把新地址同步到解析；动态域名的记录值也可以直接写 `{STUN_规则名_PORT}` 等变量，规则地址变化后立即同步，见 [记录值变量](/docs/modules/ddns#record-vars)；
- [Web 服务](/docs/modules/web) 提供 HTTPS 入口。
- Web 服务的重定向站点可以用 `{STUN_规则名_PORT}` 等变量引用规则当前的公网地址，端口变化后自动跳到新端口，见 [重定向变量](/docs/modules/web#redirect-vars)。

## 限制与多层 NAT

- **运营商级 NAT（CGNAT）**：路由器 WAN 口拿到 `100.64.0.0/10` 等内网地址时，能否穿透取决于运营商那层 NAT 的类型。全锥型（外部任意地址都能访问映射端口）可以穿透；端口受限或对称型无法穿透，此时可改用 [Tailscale、frp 或 Cloudflare 隧道](/docs/modules/tunnels)。
- **多层 NAT 下的 UPnP**：开启 UPnP / NAT-PMP 时，Gately 会在路由器上把通道端口映射到本机；如果路由器的外网地址与 STUN 得到的公网地址不同，说明外层还有一层 NAT，公网端口属于外层，Gately 只保留通道端口的映射，不再按公网端口追加映射。
- **固定本地端口并放行防火墙**：本地端口留空时每次启动随机选择。主机开启了防火墙（例如飞牛 fnOS 的系统防火墙）时，建议给规则固定本地端口，并在防火墙中放行该端口（可预留一段端口给 STUN 规则使用），否则入站连接会被防火墙丢弃，表现为穿透地址无法访问。
- **TCP 与路由器的锥形 NAT**：部分路由器的「NAT1 / 全锥形」只对 UDP 生效，TCP 出站时会改写源端口。TCP 规则需要开启 UPnP，或在路由器上把通道端口映射到本机。

## 自动放行防火墙 {#auto-firewall}

规则开启「自动放行防火墙」后，Gately 会在本机防火墙放行通道端口（填写了 UPnP / NAT-PMP 内部端口时一并放行），规则停止或删除时移除。适合 Gately 运行在 OpenWrt 等路由器上，或运行在开启了防火墙的 Linux 主机上；只支持 Linux。

- nftables：在所有挂在 input 钩子上的过滤链（例如 OpenWrt fw4、firewalld）最前面插入放行规则；
- iptables：在 filter 表的 INPUT 链最前面插入放行规则（例如 OpenWrt fw3、ufw）；
- 放行规则带 `gately-open:` 注释；防火墙重新加载会清掉这些规则，Gately 每 30 秒检查一次并补回；
- 内核没有 nf_tables（如群晖 DSM）时改用 iptables（legacy）：放行规则写在 Gately 专用的 filter 链（`GTLY_OPEN_` 开头）中，INPUT 的第一条规则跳到这条链；系统防火墙把自己的规则插到前面时 30 秒内挪回第一位。Docker 镜像已自带 iptables（legacy）；
- Docker 部署需要 host 网络与 `cap_add: NET_ADMIN`，规则卡片会显示放行结果或失败原因；
- 飞牛 fnOS 系统防火墙等不经过 nftables / iptables 的防火墙需要在系统设置中手动放行。

## 来源 IP 访问控制

使用内置转发时，每条规则可以设置白名单或黑名单，按来源 IP 决定是否转发到目标，名单与订阅 IP 列表的用法和[端口转发](/docs/modules/portforward#来源-ip-访问控制)相同，入侵防护的封禁同样优先于名单。名单只作用于转发，STUN 查询与通道有效性检测不受影响；修改名单不会重新打洞，公网端口保持不变。UDP 按数据包判断。不使用内置转发时外部连接不经过 Gately，名单不起作用。

## 部署提示

STUN 需要直接监听宿主机端口，Docker 部署推荐使用 `--network host`。
