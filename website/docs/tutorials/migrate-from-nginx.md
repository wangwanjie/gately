---
description: 从 nginx 或 Nginx Proxy Manager 迁移到 Gately：导入 nginx -T 输出、与 nginx 并行验证、逐站切换与回滚。包含 NPM 生成的配置能否导入的实测结论。
---

# 从 nginx 或 Nginx Proxy Manager 迁移

已有的 nginx 站点可以直接导入 Gately，转换成监听与站点，先并行验证再切换，出问题随时退回 nginx。Nginx Proxy Manager（NPM）生成的配置用变量写后端地址，现有导入转换不了，需要按清单手动建站，文中给出了清单与对照表。

## 适用场景与前提

- 现在用 nginx 做反向代理、静态站点或 HTTP 跳转 HTTPS，想换成 Gately 的 Web 服务；
- 可以登录 nginx 所在主机执行 `nginx -T`，Gately 能访问站点的后端；
- 切换时 nginx 与 Gately 不能同时监听同一个端口，所以先用别的端口验证，最后一步才交换端口；
- 证书不随配置导入：用 Gately 重新签发（见[泛域名证书与动态域名](/docs/tutorials/wildcard-cert-ddns)），或让 Gately 读取 nginx 正在用的证书文件。

## 结果是什么样

- 每个 `listen` 端口对应 Gately 的一个监听，`server` 块对应站点，`proxy_pass`、`root`、`return`、`location`、`allow` / `deny`、`add_header` 等常见写法会转换，导入的监听与站点默认停用；
- 切换前后可以回滚：停用 Gately 的监听、启动 nginx 即回到原状。

## 步骤：标准 nginx

### 1. 导出配置

在 nginx 所在主机执行，把完整输出保存下来：

```bash
nginx -T > nginx-T.txt
# nginx 在 Docker 里时
docker exec nginx nginx -T > nginx-T.txt
```

`nginx -T` 的输出包含所有 `include` 展开后的内容，Gately 会像 nginx 一样处理。

### 2. 预览并导入

1. 打开「Web → Web 服务」，点页头的「导入 nginx」。
2. 把 `nginx-T.txt` 的内容粘贴进文本框，或点「从文件读取」。
3. 点「预览」。预览按监听端口分组，列出每个站点的类型、后端或目录、来源文件与行号，以及需要注意的地方：使用了变量的指令、缺少证书的域名、无法转换的原因。

   ![导入预览：4 个站点可以导入，证书需要另行添加](/tutorials/migrate-from-nginx-preview.zh-CN.webp)

4. 保持「导入后立即启用」不勾选，点「导入 N 个站点」。监听以「nginx :端口」命名并标记「从 nginx 配置导入」，监听与站点都是「已停用」。重复导入是安全的，已有的域名会跳过。

能转换的写法包括：`proxy_pass`（含 upstream 的权重、备用与负载均衡策略）、`root` / `alias` 静态目录、`return` 跳转与固定内容、`add_header`、`proxy_set_header`、`gzip`、`allow` / `deny`、按 location 分路径的后端与访问控制、`auth_request`（转成转发认证）、`auth_basic`（转成共享用户的基本认证，需要补用户）、`listen ... proxy_protocol` 与 `quic`。`stream` 里的 TCP / UDP 转发请在[端口转发](/docs/modules/portforward)里配置。完整清单见 [Web 服务 · 导入 nginx 配置](/docs/modules/web#import-nginx)。

### 3. 准备证书

预览会列出 nginx 使用的证书文件。两种做法：

- 在「SSL 证书」重新签发，推荐 DNS-01 的泛域名证书；
- 继续使用 nginx 的证书文件：「添加证书」，来源选「读取文件」，填证书路径与私钥路径。Gately 定期检查文件变化并自动加载，适合 certbot 继续维护证书的场景。Gately 在 Docker 中时，把证书目录挂载进容器。

### 4. 先在另一个端口验证

nginx 还占着 80 与 443 时，让 Gately 先监听别的端口：

1. 在 Web 服务页找到「nginx :443」监听，点「编辑监听」，把端口改成 `8443`，打开「启用」后保存。
2. 点页头的「排序与批量」，勾选要验证的站点，点工具栏的「启用」，再点「完成排序」。
3. 用 `curl` 指定域名直接访问 Gately 的端口，不需要改 DNS：

```bash
curl -I --resolve nas.example.com:8443:127.0.0.1 https://nas.example.com:8443/
```

逐个站点核对页面、登录、WebSocket、上传下载。有差异的地方在站点编辑里调整，例如后端是自签名证书时勾选「跳过后端证书校验」，后端需要原始 Host 时保持「Host 使用后端地址」关闭。

### 5. 切换

1. 停止 nginx（`systemctl stop nginx` 或 `docker stop nginx`），释放 80 与 443；
2. 把 Gately 的「nginx :443」监听端口改回 `443`、「nginx :80」监听的开关打开；
3. 访问线上域名确认；
4. 确认无误后再决定是否卸载 nginx。

### 回滚

把 Gately 的监听开关关掉，再启动 nginx（`systemctl start nginx`）。Gately 的配置会保留，修复问题后可以再次切换；Gately 的[配置历史](/docs/guide/audit)也可以恢复到修改之前。

## 逐站切换：不停机、一个域名一个域名搬

站点很多、不想一次切完时，可以让 Gately 占用 443，把还没搬的域名交给 nginx：

1. 把 nginx 改为只监听本机端口，并接收 PROXY 协议以保留访客真实 IP（把每个 `listen 443 ssl;` 改为 `listen 127.0.0.1:8443 ssl proxy_protocol;`，80 端口同理改为 `listen 127.0.0.1:8080;`），在 `http` 或各 `server` 里加上：

   ```nginx
   set_real_ip_from 127.0.0.1;
   real_ip_header proxy_protocol;
   ```

   然后重新加载 nginx；
2. 在 Gately 里导入配置（步骤 2），保持「nginx :443」监听端口为 `443`；
3. 在这个监听上点「设置默认站点」：类型「反向代理」，后端地址 `https://127.0.0.1:8443`，勾选「跳过后端证书校验」「向后端发送 PROXY v2」「使用访问域名作为 SNI」，保存。没有匹配 Gately 站点的域名都会落到这里，由 nginx 继续服务；
4. 开启监听，之后每搬一个域名，就在 Gately 里启用对应站点。启用的站点由 Gately 处理，其余仍由 nginx 处理；
5. 站点出问题时，停用这个站点，它会回到 nginx，不影响其他域名。

## 步骤：Nginx Proxy Manager

NPM 在 `/data/nginx/proxy_host/*.conf` 里用 `set $server`、`set $port` 与 `proxy_pass $forward_scheme://$server:$port$request_uri;` 写后端。导入预览会把这样的站点标为「不导入：没有可转换的反向代理…」，并提示 `proxy_pass … 使用了变量，无法转换`，所以 NPM 的代理主机需要手动建站。重定向主机（`return 301 $forward_scheme://…`）同样含变量。

1. 在 NPM 所在主机列出每个域名与后端，路径是 NPM 数据卷里的 `nginx/proxy_host`：

   ```bash
   for f in /data/nginx/proxy_host/*.conf; do
     awk '/^[[:space:]]*server_name/{sub(/^[[:space:]]*server_name[[:space:]]+/,"");sub(/;.*/,"");d=$0}
          /set \$forward_scheme/{gsub(";","");s=$3}
          /set \$server[[:space:]]/{gsub(/[";]/,"");h=$3}
          /set \$port/{gsub(";","");p=$3}
          END{print d" -> "s"://"h":"p}' "$f"
   done
   ```

   输出类似 `nas.example.com -> http://192.168.1.10:5000`。NPM 在 Docker 里时，先用 `docker exec` 进容器执行，或把数据卷挂到宿主机后在宿主机执行；
2. 按每一行在「Web 服务」里添加监听与站点：域名填输出左侧，类型「反向代理」，后端地址填右侧；
3. 对照下表补上 NPM 页面里勾选过的项目；
4. 之后的验证、切换与回滚与上面完全相同。

| NPM 里的设置 | Gately 里的做法 |
| --- | --- |
| Scheme / Forward Hostname / Port | 站点的「后端地址」，如 `http://192.168.1.10:5000`；后端是 https 且证书自签名时勾选「跳过后端证书校验」 |
| Websockets Support | 反向代理自动支持，无需设置 |
| Block Common Exploits | 站点「安全」页选一个 [WAF 实例](/docs/modules/waf)，先用「仅检测」观察 |
| Force SSL、HTTP/2 | 监听的「HTTP 端口」填 80 并选「跳转到 HTTPS」；HTTP/2 在 HTTPS 上自动协商 |
| HSTS | 站点「请求头」页添加响应头 `Strict-Transport-Security: max-age=63072000` |
| Access List 的 IP 规则 | 站点「安全」页的「IP 访问控制」白名单或黑名单 |
| Access List 的用户名密码 | 「基本认证」或「网页认证」，用户需要在 Gately 里重新设置密码 |
| Redirection Host | 类型「重定向」的站点 |
| Streams | [端口转发](/docs/modules/portforward) |
| 404 Host | 默认站点，类型「固定文本」或「断开连接」 |
| Advanced 里的自定义 nginx 配置 | 逐条看是否有对应的站点选项，没有的先不迁移该站点 |
| SSL 证书（Let's Encrypt） | 在 Gately 的「SSL 证书」重新签发，或用「读取文件」指向 `/etc/letsencrypt/live/npm-N/` 下的文件 |

NPM 的数据卷里还有 `letsencrypt` 目录，证书文件在 `live/npm-N/` 下，N 是 NPM 内部编号。

## 验证

- 预览通过后，每个站点用 `curl --resolve` 测试返回码与内容；
- 切换后打开站点，看浏览器里的证书是否有效、WebSocket 应用（如 Jellyfin、Home Assistant）是否正常；
- 站点卡片显示请求数与错误数，在站点统计里可以看到访客 IP 与状态码分布；
- 导入的监听与站点启用后，运行日志里出现 `https 监听 :443（N 个站点）`。

## 常见问题

**预览里「不导入」，原因是没有可转换的内容。** 站点只用了变量写后端（NPM 的典型写法）、正则 `location` 或 Lua 等。手动建站，参考上面的对照表。

**监听启动失败，提示端口被占用。** nginx 还在占用端口。先停 nginx，或把 Gately 的监听改到别的端口。

**导入时提示缺少证书。** 证书不随配置导入，按步骤 3 添加。

**站点启用后返回 502。** 后端地址在 Gately 所在的网络里不可达（Docker 部署要用 `--network host` 或可路由的地址），或后端是 https 自签名证书，勾选「跳过后端证书校验」。

**后端应用看不到访客的真实 IP。** 站点的「添加 X-Forwarded-*」默认开启，应用要信任 Gately 的地址作为代理；逐站切换方案里 nginx 一侧需要按上面配置 `real_ip_header proxy_protocol`。

## 相关文档

- [Web 服务](/docs/modules/web)：导入 nginx 配置、替换 haproxy 与 nginx 的对照表；
- [一个 443 端口跑所有服务](/docs/tutorials/one-port-443)；
- [云服务器上替换 haproxy + nginx + fail2ban + acme.sh](/docs/tutorials/replace-haproxy-nginx-fail2ban)。
