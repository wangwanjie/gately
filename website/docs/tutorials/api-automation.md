---
time: 20 分钟
description: 用脚本和 AI 助手管理 Gately：创建只带所需权限、会过期的 API 令牌，用 curl 列出站点、触发 DDNS 同步、签发证书，在界面里查接口文档与 OpenAPI，用 gately ctl 命令行调用，再通过 gately mcp 让 Claude、Cursor 等 AI 客户端读取和修改配置。
---

# API、命令行与 MCP 自动化

管理界面里能做的事，都可以通过 HTTP 接口完成：家里宽带重拨后立刻同步 DDNS、CI 部署完新服务后加一条反向代理、每天把证书到期时间推到监控面板，或者直接对 AI 助手说「看看哪个证书快到期了」。本文先创建一个权限最小的 API 令牌，用 curl 跑通几个常用接口，然后换成内置的 `gately ctl` 命令行，最后接入 MCP，让 AI 客户端调用同一批接口。

::: tip 先看看接口文档
在线演示 [demo.gately.cn](https://demo.gately.cn) 点「演示账号一键进入」，从右上角用户菜单打开「API 文档」，可以浏览全部接口。演示实例只读，不能创建令牌。
:::

## 适用场景与前提

- Gately 已经在运行，你有管理员账号；
- 运行脚本的机器能访问 Gately 的管理端口（默认 `16680`）。下文用 `http://192.168.1.2:16680` 作为示例地址；
- 系统里有 `curl`，解析 JSON 时用到 `jq`（可选）。

## 结果是什么样

```text
脚本 / CI ──curl──────────────┐
终端 ──gately ctl─────────────┼──Authorization: Bearer gtl_…──▶ Gately 管理 API（/api/...）
AI 客户端 ──gately mcp（stdio）┘                                   └─ 按令牌权限放行或返回 403
```

三种方式用的是同一套接口、同一种令牌，权限由令牌决定。

## 步骤

### 1. 创建 API 令牌

1. 打开「系统 → 设置」，在「账号与安全」的「API 令牌」卡片点「管理 API 令牌」；也可以从右上角用户菜单进入（地址 `#/api-tokens`）。
2. 点「新建令牌」，填写：
   - 「名称」：写清用途，例如 `DDNS 与证书脚本`；
   - 「过期时间」：可选「永不过期」、7 天、30 天、90 天、1 年或自定义日期，建议设置；
   - 「权限」：每个功能模块一行，选「无」「只读」或「读写」。本例给「动态域名 ddns」与「SSL 证书 ssl」选「读写」，「Web 服务 webservice」选「只读」，再到下方「系统」分组给「系统状态」选「只读」。

   ![新建令牌：名称、过期时间与权限](/tutorials/api-automation-create.zh-CN.webp)

3. 点「新建」。弹窗「令牌仅显示这一次」给出明文令牌（`gtl_` 加 48 位十六进制字符）和两条现成的 curl 示例。点「复制」保存到密码管理器或脚本的密钥配置里，再点「我已保存」。

   ![令牌只显示一次](/tutorials/api-automation-secret.zh-CN.webp)

磁盘上只保存令牌的哈希，关掉弹窗后无法再查看明文；丢了只能「重新生成」，旧值立即失效。

![令牌列表](/tutorials/api-automation-tokens.zh-CN.webp)

令牌列表显示每个令牌的权限、过期时间、最近使用时间与来源 IP。「编辑」可以随时改名称、权限与过期时间，修改立即生效，令牌明文不变；开关可以临时停用。

**选权限的原则：**

- 写权限包含读权限。顶部的「全部只读」「全部读写」「清空」可以一键设置；
- 带「可执行命令」标记的模块（如 `ddns`、`ssl`、`cron`、`docker`、`webterminal`），写权限可以借脚本、命令在服务器上执行任意命令；带「可读写文件」标记的模块（如 `webservice`、`storage`、`backup`），写权限可以读写服务器上的文件。只给完全信任的脚本；
- 一个脚本一个令牌，只勾它用到的模块。泄露时删掉或重新生成那一个即可。

**令牌碰不到的接口。** 账号与两步验证、令牌管理、操作审计与配置历史、系统设置、重启与在线升级、诊断包、配置备份下载与恢复都只接受登录会话，令牌访问一律返回 403。

### 2. 用 curl 调用

先把地址和令牌放进环境变量，后面的命令都用它们：

```bash
export GATELY_URL=http://192.168.1.2:16680
export GATELY_TOKEN=gtl_xxxx   # 换成你的令牌
```

**确认令牌可用。** 读取接口描述不需要任何权限，令牌有效就返回：

```bash
curl -s -H "Authorization: Bearer $GATELY_TOKEN" "$GATELY_URL/api/openapi.json" | jq '.info'
```

**查看系统状态**（需要 `status:read`）：

```bash
curl -s -H "Authorization: Bearer $GATELY_TOKEN" "$GATELY_URL/api/status"
```

**列出全部站点**（需要 `webservice:read`）。Web 服务的配置按监听组织，每个监听下有若干站点：

```bash
curl -s -H "Authorization: Bearer $GATELY_TOKEN" "$GATELY_URL/api/webservice/listeners" \
  | jq -r '.[] | .name as $l | .sites[] | [$l, .name, (.domains | join(",")), .type, (.enable | tostring)] | @tsv'
```

输出类似：

```text
公网 HTTPS 入口	博客	blog.example.com,www.example.com	proxy	true
公网 HTTPS 入口	导航面板	home.example.com	panel	true
```

**立即同步一个 DDNS 任务**（需要 `ddns:write`）。先列出任务拿到 `key`：

```bash
curl -s -H "Authorization: Bearer $GATELY_TOKEN" "$GATELY_URL/api/ddns/tasks" \
  | jq -r '.[] | [.key, .name, .state.ipv4 // "-", .state.ipv6 // "-"] | @tsv'
```

再触发同步，返回 `{"ok":true}` 表示已经排队执行，结果在 DDNS 页面和模块日志里看：

```bash
curl -s -X POST -H "Authorization: Bearer $GATELY_TOKEN" "$GATELY_URL/api/ddns/tasks/d-cf/sync"

# 加 force=1：IP 没有变化也向服务商重写一次解析
curl -s -X POST -H "Authorization: Bearer $GATELY_TOKEN" "$GATELY_URL/api/ddns/tasks/d-cf/sync?force=1"
```

任务停用时返回 400「任务未运行」。

**签发或续期证书**（需要 `ssl:write`）。先看证书与到期时间：

```bash
curl -s -H "Authorization: Bearer $GATELY_TOKEN" "$GATELY_URL/api/ssl/certs" \
  | jq -r '.[] | [.key, .remark, ((.info.domains // []) | join(",")), (.info.notAfter // "未签发")] | @tsv'
```

对某张 ACME 证书立即签发或续期：

```bash
curl -s -X POST -H "Authorization: Bearer $GATELY_TOKEN" "$GATELY_URL/api/ssl/certs/c-wild/renew"
```

签发在后台进行，返回 `{"ok":true}`；正在签发中返回 409，手动上传或按文件路径添加的证书返回 400「只有 ACME 证书可以续签」。之后再查 `.state.obtaining` 与 `.info.notAfter` 确认结果。

**写操作的请求体。** 新增与修改用 JSON，带 `Content-Type: application/json`。例如停用一个 DDNS 任务：

```bash
curl -s -X PUT -H "Authorization: Bearer $GATELY_TOKEN" -H "Content-Type: application/json" \
  -d '{"enable":false}' "$GATELY_URL/api/ddns/tasks/d-cf/enable"
```

错误统一返回 `{"error": "说明"}`。权限不足时返回 403「令牌缺少权限 ddns:write」，系统日志里同时记一条警告。加 `-H "Accept-Language: zh-CN"` 可以让错误说明固定用中文。

::: tip 宽带重拨后立即同步
很多路由器支持拨号成功后执行脚本或调用 URL，把上面的 `sync` 命令放进去，比等 DDNS 的检查间隔更快。令牌只给 `ddns:write`。
:::

### 3. 在接口文档里找接口

不知道某个操作对应哪个接口时，打开「API 文档」（API 令牌页右上角的「API 文档」，或用户菜单，地址 `#/api-docs`）：

1. 左侧按功能分组列出全部接口，上方「搜索路径或摘要」可以按路径或中文摘要过滤；
2. 点开一个接口，看到「所需权限」、参数、请求体字段与「请求示例」（可复制的 curl）；
3. 标记「仅登录会话」的接口令牌无法调用，也不给 curl 示例。

![API 文档：展开「立即同步一个 DDNS 任务」](/tutorials/api-automation-docs.zh-CN.webp)

右上角「下载 openapi.json」得到 OpenAPI 3.0 描述，可以导入 Postman、Bruno 等工具，或用来生成客户端代码。每个接口带扩展字段 `x-required-scope`（所需权限）与 `x-token-allowed`（令牌能否访问）。全部接口也可以在官网 [API 接口清单](/docs/reference/api-reference) 查看。

### 4. 用 gately ctl 命令行

`gately` 程序自带 `ctl` 子命令，省去拼 curl 的麻烦：

```bash
# 保存地址与令牌（会先验证令牌），之后不用再带
gately ctl login http://192.168.1.2:16680 gtl_xxxx

# 列出令牌能访问的接口，按关键字过滤
gately ctl ls ddns

# 读
gately ctl get /api/status
gately ctl get /api/ddns/tasks

# 写：请求体可以是 JSON、@文件 或 -（标准输入）
gately ctl post /api/ddns/tasks/d-cf/sync
gately ctl put /api/ddns/tasks/d-cf/enable '{"enable":false}'
gately ctl post /api/ddns/tasks @task.json
```

- `login` 把配置写到 `~/.config/gately/ctl.json`（macOS 为 `~/Library/Application Support/gately/ctl.json`）；也可以用环境变量 `GATELY_URL`、`GATELY_TOKEN`，或在子命令前加 `-url`、`-token`；
- 管理界面用自签名 HTTPS 时加 `-insecure`；想要原始响应加 `-raw`，默认输出格式化的 JSON；
- 成功退出码为 0，接口返回非 2xx 时输出 `错误: HTTP 状态码: 说明` 并以 1 退出，适合写进脚本判断。

Docker 部署时程序在容器里，可以直接在宿主机上执行：

```bash
docker exec gately gately ctl -token gtl_xxxx get /api/status
```

完整用法见 [命令行与 MCP](/docs/guide/cli-mcp)。

### 5. 接入 MCP，让 AI 助手管理 Gately

`gately mcp` 把令牌能访问的接口变成 MCP 工具，由 AI 客户端作为子进程启动，通过标准输入输出通信。

1. **为 AI 单独建一个令牌。** 先给只读权限（「全部只读」），确认用法后再按需加写权限。
2. **在客户端里登记。** Claude Desktop、Cursor 等使用 `mcpServers` 配置的客户端：

   ```json
   {
     "mcpServers": {
       "gately": {
         "command": "gately",
         "args": ["mcp"],
         "env": {
           "GATELY_URL": "http://192.168.1.2:16680",
           "GATELY_TOKEN": "gtl_xxxx"
         }
       }
     }
   }
   ```

   `command` 可以写 `gately` 的绝对路径。Claude Code 用一条命令添加：

   ```bash
   claude mcp add gately -e GATELY_URL=http://192.168.1.2:16680 -e GATELY_TOKEN=gtl_xxxx -- gately mcp
   ```

3. **重启客户端**，工具列表里出现 `get_ddns_tasks`、`get_ssl_certs`、`get_webservice_listeners` 这类工具，每个接口一个，名称由方法与路径组成。

**默认只读。** 不带 `--write` 时只提供 GET 类工具，也不提供取回明文密码的 `.../secrets` 接口。要让 AI 修改配置，把 `args` 改为 `["mcp", "--write"]`，令牌也要有对应的写权限，两者同时满足才能写。

**对话示例。** 接好以后可以直接这样问：

- 「列出 Gately 里所有站点和它们的后端地址，标出没有开启认证的对外站点。」AI 调用 `get_webservice_listeners` 后整理成表格；
- 「哪些证书 30 天内到期？」AI 调用 `get_ssl_certs`，比较 `info.notAfter`；
- 「DDNS 任务现在解析到哪个 IP？最近一次同步是什么时候？」AI 读取任务的 `state`；
- 开启写入后：「把 Jellyfin 站点的后端改成 `http://192.168.1.21:8096`。」AI 先读取监听配置，再调用修改监听的工具提交。

::: warning 开启写入前想清楚
`--write` 允许 AI 调用令牌权限范围内的全部写接口，包括删除。只给它需要的模块写权限，并保留客户端的「调用前确认」。改坏了可以在「日志 → 配置历史」里对比并回滚，见 [配置历史、备份与迁移](/docs/tutorials/history-backup)。所有令牌发起的写操作都记在「操作审计」里，操作者显示为 `api-token:<令牌名>`。
:::

## 验证

- `curl .../api/openapi.json` 返回 JSON，说明令牌有效；
- 用一个只有 `ddns:read` 的令牌调用 `sync`，返回 403「令牌缺少权限 ddns:write」，说明权限隔离生效；
- 触发 DDNS 同步后，DDNS 页面的「最近同步」时间更新；
- 「API 令牌」列表里该令牌的「最近使用」时间与来源 IP 更新；
- 「日志 → 操作审计」里能看到 `api-token:DDNS 与证书脚本` 发起的写操作。

## 常见问题

**返回 401 未登录。** 令牌写错、已过期、被停用或已删除。令牌无效时请求按登录会话校验，所以报的是未登录。检查 `Authorization: Bearer gtl_…` 的写法。

**返回 403。** 令牌缺少该接口要求的权限，看错误里写的权限名去「编辑」令牌；或者接口只接受登录会话（备份恢复、令牌管理等）；或者对应模块在「功能模块」里关闭了。

**某些 GET 接口要写权限。** 证书下载、`.../secrets`、容器内执行命令、文件读取这类会返回敏感内容或建立交互通道的接口按写权限处理，只读令牌拿不到。

**`gately ctl` 连不上。** 默认地址是 `http://127.0.0.1:16680`，在其他机器上运行时要用 `login`、环境变量或 `-url` 指定；管理界面改过端口或加了安全入口时同样要写对地址。

**AI 客户端里没有出现工具。** 确认 `gately` 在客户端的 PATH 里（或写绝对路径），地址和令牌正确；工具列表在客户端第一次请求时从实例的 OpenAPI 生成，只包含令牌能访问的接口，令牌权限少时工具也少。

## 下一步

- [配置告警通知](/docs/tutorials/notify-alerts)：用 Webhook 把事件推给自己的自动化系统；
- [申请 HTTPS 证书](/docs/tutorials/https-certificate)、[DDNS 入门](/docs/tutorials/ddns-basics)：脚本调用的对象从哪来；
- [API 令牌与 OpenAPI](/docs/reference/api)、[API 接口清单](/docs/reference/api-reference)、[命令行与 MCP](/docs/guide/cli-mcp)：完整参考。
