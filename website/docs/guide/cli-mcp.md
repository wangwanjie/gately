# 命令行与 MCP

`gately` 可执行文件内置两个子命令：`gately ctl` 在终端中调用运行中实例的管理 API，`gately mcp` 把同一批接口以 MCP 工具的形式提供给 AI 客户端。两者都通过 HTTP 与 [API 令牌](/docs/reference/api) 访问实例，接口清单来自实例的 `/api/openapi.json`，权限由令牌决定。

另有 `gately reset-password` 与 `gately reset-totp` 直接读写本机配置目录，不经过 API，用于忘记密码或丢失两步验证器时找回管理员账号，见 [重置密码与两步验证](/docs/guide/install#reset)。

`gately service install|uninstall|start|stop|restart|status` 把 Gately 注册为开机自启的系统服务，见 [开机自启](/docs/guide/install#service)。

## 连接配置

连接信息按以下顺序确定，后面的覆盖前面的：

1. 默认地址 `http://127.0.0.1:16680`
2. 配置文件 `ctl.json`
3. 环境变量 `GATELY_URL`、`GATELY_TOKEN`
4. 命令行选项 `-url`、`-token`

配置文件位于系统用户配置目录下的 `gately/ctl.json`（Linux 为 `~/.config/gately/ctl.json`，macOS 为 `~/Library/Application Support/gately/ctl.json`）。内容示例：

```json
{
  "url": "http://127.0.0.1:16680",
  "token": "gtl_xxxx",
  "insecure": false
}
```

`gately ctl login` 会生成这个文件。

## gately ctl

### 子命令

| 命令 | 说明 |
| --- | --- |
| `gately ctl ls [关键字]` | 列出令牌可访问的接口，输出方法、路径与摘要，可按关键字过滤 |
| `gately ctl get <路径>` | 发送 GET 请求 |
| `gately ctl post <路径> [JSON或@文件]` | 发送 POST 请求 |
| `gately ctl put <路径> [JSON或@文件]` | 发送 PUT 请求 |
| `gately ctl delete <路径> [JSON或@文件]` | 发送 DELETE 请求 |
| `gately ctl login <地址> <令牌>` | 校验连接并保存到配置文件 |

路径必须以 `/api/` 开头。请求体可以是 JSON 字面量、`@文件名`（读取文件）或 `-`（读取标准输入），内容必须是合法 JSON。`patch` 方法同样可用，`ls` 也可写作 `list`。

### 连接选项

选项写在子命令之前：

| 选项 | 说明 |
| --- | --- |
| `-url <地址>` | 管理地址，http 或 https 开头 |
| `-token <令牌>` | API 令牌 |
| `-insecure` | 跳过 HTTPS 证书校验，用于自签名证书的内网实例 |
| `-raw` | 输出响应原文，默认格式化 JSON |

命令成功时退出码为 0，接口返回非 2xx 时输出 `错误: HTTP 状态码: 说明` 并以 1 退出，参数问题以 2 退出。

### 示例

```bash
# 保存连接配置（会先请求 /api/session 验证令牌）
gately ctl login http://192.168.1.2:16680 gtl_xxxx

# 查看令牌可访问的接口，过滤包含 ddns 的行
gately ctl ls ddns

# 读取系统状态
gately ctl get /api/status

# 唤醒设备
gately ctl post /api/wol/devices/abc123/wake

# 停用一个 DDNS 任务
gately ctl put /api/ddns/tasks/abc/enable '{"enable":false}'

# 从文件读取请求体
gately ctl post /api/ddns/tasks @task.json

# 临时使用环境变量
GATELY_URL=https://gw.example.com GATELY_TOKEN=gtl_xxxx gately ctl get /api/status
```

::: tip 提示
`ls` 列出的是令牌可以访问的接口，仅登录会话可用的接口（账号、令牌管理、备份恢复等）不会出现。写操作是否放行取决于令牌权限，权限不足时返回 403。
:::

## gately mcp

`gately mcp` 通过标准输入输出提供 MCP 服务（JSON-RPC，每行一条消息），由 AI 客户端作为子进程启动。

### 选项

| 选项 | 说明 |
| --- | --- |
| `--url <地址>` | 管理地址 |
| `--token <令牌>` | API 令牌 |
| `--write` | 同时暴露写操作工具 |
| `--insecure` | 跳过 HTTPS 证书校验 |

连接信息的来源与 `gately ctl` 一致（配置文件、`GATELY_URL`、`GATELY_TOKEN`、选项）。

### 工具生成规则

- 服务启动后，在客户端第一次请求工具列表时读取实例的 OpenAPI 文档，每个令牌可访问的接口生成一个工具。
- 工具名由方法与路径组成，如 `get_ddns_tasks`、`put_ddns_tasks_key_enable`，最长 64 个字符，重名时追加序号。
- 路径参数成为必填的字符串参数；GET 工具带可选的 `query` 参数（如 `since=10`）；有请求体的接口带 `body` 参数，结构取自 OpenAPI。
- 工具描述包含接口摘要、方法路径和所需权限；工具标注 `readOnlyHint`，DELETE 工具标注 `destructiveHint`。
- 返回内容超过 200000 个字符时截断。

### 默认只读

不带 `--write` 时只暴露 GET 与 HEAD 工具，也不提供 `.../secrets` 这类取回明文密码的接口，加上 `--write` 后才会提供。需要让 AI 修改配置时加上 `--write`。即使开启写入，令牌自身的权限依然生效，所以更稳妥的做法是为 MCP 创建专用令牌，只授予需要的权限。

::: warning 注意
`--write` 允许 AI 客户端调用令牌权限范围内的全部写接口，包括删除。先用只读令牌验证流程，再按需授予写权限。
:::

### 客户端配置

Claude Desktop、Cursor 等使用 `mcpServers` 配置格式的客户端：

```json
{
  "mcpServers": {
    "gately": {
      "command": "gately",
      "args": ["mcp"],
      "env": {
        "GATELY_URL": "http://127.0.0.1:16680",
        "GATELY_TOKEN": "gtl_xxxx"
      }
    }
  }
}
```

开启写入时把 `args` 改为 `["mcp", "--write"]`。`command` 可以写 `gately` 的绝对路径。

Claude Code 使用命令行添加：

```bash
claude mcp add gately -e GATELY_URL=http://127.0.0.1:16680 -e GATELY_TOKEN=gtl_xxxx -- gately mcp
```
