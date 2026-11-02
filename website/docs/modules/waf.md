# WAF 防火墙

基于 [Coraza](https://coraza.io/) 与 OWASP CRS v4 的 Web 应用防火墙，可挂接到 [Web 服务](/docs/modules/web) 的站点。

## 能力

- 内置 OWASP CRS v4 规则集，可按规则文件启用；
- 支持自定义 SecLang 规则；
- 规则组可调整顺序；命中日志记录客户端 IP、域名、路径与规则 ID，可在日志中按域名、IP 或关键词筛选；
- 拦截模式与仅检测模式；
- 请求体检查上限可调，超过上限时拒绝请求或只检查前面部分；可按路径跳过请求体检查；
- WebDAV 兼容模式：放行网盘、WebDAV 用到的方法、请求头与任意上传内容类型；
- 可接入自行部署的雷池（SafeLine）检测引擎，单独使用或与规则组同时使用；
- 多个实例，站点按需引用。

## 使用建议

1. 新站点先使用「仅检测」，观察一段时间的日志，确认没有误报；
2. 针对误报调整规则文件或添加自定义例外；
3. 再切换为「拦截」。

## 规则组顺序

实例里的规则组从上到下加载，可用上移、下移按钮调整。同一阶段内排在前面的规则先执行：

- 拦截规则（deny 等）放在哪里都会生效；
- 运行期的放行或排除规则（`allow`、`ctl:ruleRemoveById`、`ctl:ruleRemoveTargetById` 等）要放在 OWASP CRS 规则组之前，否则请求可能已被 CRS 拦截；
- `SecRuleRemoveById`、`SecRuleUpdateTargetById` 这类配置期指令只作用于已经加载的规则，要放在 CRS 规则组之后，放在之前不起作用。

## 上传大文件的站点

WAF 默认只检查不超过 12.5 MB 的请求体，更大的请求会被拒绝（413），网盘、WebDAV、文件管理这类站点因此无法上传大文件。在实例的「请求体检查」中可以调整：

- **检查上限**：0 表示默认的 12.5 MB，最大 1024 MB；
- **超过上限时**：「拒绝请求」与之前一致；「只检查前面部分」只检查不超过上限的部分，其余内容原样转发给后端，适合上传大文件的站点；
- **兼容 WebDAV 与上传**：OWASP CRS 默认只允许 GET、HEAD、POST、OPTIONS，并拦截 `Lock-Token`、`If`、`Content-Range` 请求头与白名单以外的内容类型，WebDAV 客户端因此无法列目录、上传和加锁。开启后放行 PUT、DELETE、PROPFIND、PROPPATCH、MKCOL、COPY、MOVE、LOCK、UNLOCK 等方法与这些请求头，不再限制上传的内容类型；PUT 与 `application/octet-stream` 的请求体是文件内容，不再按表单参数检查。注入、路径穿越等攻击检查照常生效；
- **跳过请求体的路径**：每行一个路径前缀（如 `/api/upload`、`/remote.php/dav/`），匹配的请求不检查请求体，请求头、URL 与参数仍照常检查。

推荐为网盘、WebDAV 这类站点单独建一个实例：开启「兼容 WebDAV 与上传」并选择「只检查前面部分」，必要时把上传接口加入跳过路径，其他站点继续使用默认设置。

curl、rclone 等客户端上传较大的内容时会带 `Expect: 100-continue` 请求头，CRS 4 默认会拦截它。Gately 的所有实例都放行该请求头（由 Gately 自行处理 100-continue），不需要额外设置。

## 接入雷池（SafeLine）

[雷池](https://waf-ce.chaitin.cn/)是长亭科技的 Web 应用防火墙，靠语义分析引擎识别攻击，误报较少。Gately 可以把请求送到你自己部署的雷池检测引擎（safeline-detector）检测，按结果放行或拦截；入口、证书、认证与封禁仍由 Gately 负责，雷池不必再当一层反向代理。

1. 按雷池官方文档安装社区版，默认安装目录为 `/data/safeline`。
2. 选择连接检测引擎的方式：
   - **Unix 套接字**（Gately 与雷池在同一台机器上）：检测引擎默认监听 `/data/safeline/resources/detector/snserver.sock`，地址填 `unix:/data/safeline/resources/detector/snserver.sock`。Gately 运行在 Docker 中时，把 `/data/safeline/resources/detector` 目录挂载进 Gately 容器，地址填容器内的路径。
   - **TCP**：在 `/data/safeline/resources/detector/detector.yml` 中写入下面两行，再执行 `docker restart safeline-detector`。改为 TCP 后检测引擎不再监听 Unix 套接字。

     ```yaml
     bind_addr: 0.0.0.0
     listen_port: 8000
     ```

     地址填 safeline-detector 容器的 IP 与端口，例如 `172.22.222.5:8000`，容器 IP 可以用 `docker inspect safeline-detector | grep IPAddress` 查看。Gately 运行在 Docker 的 bridge 网络中时，用 `docker network connect safeline-ce <Gately 容器名>` 把它接入雷池的网络。检测引擎没有认证，不要把 8000 端口暴露到公网。
3. 在 WAF 实例中打开「接入雷池」，填写地址后点「测试连接」。Gately 先发送心跳，再送检一个示例 SQL 注入请求，提示「示例攻击已被识别」说明检测引擎工作正常。这条示例攻击会出现在雷池控制台的攻击记录中，来源为 127.0.0.1，域名为 gately-test.invalid。
4. 在 Web 服务的站点中引用这个实例。

说明：

- 接入雷池后规则组可以留空；同时保留规则组时，请求先经雷池检测，再经规则组检查。「仅检测」模式下，雷池识别出的攻击只写日志，请求照常放行。
- 被拦截的请求：浏览器看到带事件 ID 的拦截页，其他客户端得到与雷池官方插件相同的 JSON（含 `event_id`）。状态码采用检测引擎给出的值，通常为 403。拦截计入访问日志（`deny=waf`）、站点统计与 WAF 拦截事件，入侵防护的「WAF 拦截」预设同样生效。
- 攻击记录也会写入雷池控制台，可以按事件 ID 查到；这些记录不归属雷池中的任何站点。
- 送检的请求体默认最多 1024 KB，超出部分不送检、原样转发给后端。「跳过请求体的路径」与「兼容 WebDAV 与上传」对雷池同样生效；经 HTTP/2 传输、长度未知的 gRPC 请求体不送检。
- 检测超时默认 1 秒。检测引擎不可用（连接失败、超时）时默认放行请求，实例显示「雷池不可用」并写入错误日志；选择「拒绝请求」后，这类请求返回 503。检测引擎刚启动的几秒内可能返回无法识别的结果，同样按不可用处理。
- 只检测请求，不送检响应。

## 验证

WAF 使用真实攻击载荷测试，覆盖路径穿越等攻击的拦截。
