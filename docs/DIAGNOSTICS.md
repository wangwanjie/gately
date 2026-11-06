# 诊断包

遇到问题时，在 **设置 → 诊断** 中导出诊断包，附在 [GitHub issue](https://github.com/wangwanjie/gately/issues/new) 里，维护者就能直接定位原因，不必再向你索要截图和日志。

诊断包在你自己的机器上生成，不会上传到任何地方。下载前点击 **预览内容**，可以看到将要导出的全部文件；zip 内只有纯文本和 JSON，解压后也能逐个检查。

## 会导出什么

| 文件 | 内容 |
| --- | --- |
| `README.txt` | 诊断包说明（中英对照） |
| `summary.json` | Gately 版本与构建信息、系统与架构、CPU 型号与核数、内存与磁盘容量、运行时长、Go 运行时统计、管理界面端口、网卡**名称**及其地址**类别**（如 `private: 1, public: 2`） |
| `modules.json` | 模块列表与总览页的状态摘要 |
| `config/*.json` | 配置的**结构**：字段名、数字、开关，以及协议、模式等枚举值 |
| `logs/*.log` | 系统与各模块最近的运行日志，包括重启前已落盘的部分（可在导出时关闭） |
| `logs/crash.log` | 异常退出记录：Go 运行时崩溃（panic、fatal error）的信息与堆栈，以及进程被强制结束（如内存不足被 OOM 终止）的大致时间；只有发生过异常退出才有（可在导出时关闭） |

## 绝不会导出什么

- 密码、密码哈希、API 令牌、两步验证密钥、会话密钥、OAuth 密钥、DNS 服务商密钥、SSH 密码与私钥、证书与私钥。`account.json` 与 `tokens.json` 不会被读取。
- 域名、主机名、真实 IP 与 MAC 地址，统一替换为 `<domain>`、`<ipv4:private>`、`<ipv4:public>`、`<mac>` 等占位符，端口予以保留。
- 配置目录与用户主目录的路径（替换为 `<config-dir>`、`<home>`）。
- 你在配置里填写的名称、备注等文字：凡不是已知枚举值的字符串，一律替换为 `<redacted>`。
- 文件内容、Docker 容器 / 镜像 / 环境变量、终端会话与流量内容。

## 脱敏方式

- **配置**：默认移除所有取值，只保留字段名、数字、开关、固定的枚举字段（`protocol`、`mode`、`provider` 等）与时间戳。地址类字段只保留端口和 IP 类别。
- **日志**：替换 IP、域名、邮箱、URL、`password=…` 形式的键值、`Bearer` 令牌、疑似密钥的长字符串、MAC 地址与用户目录。`github.com`、`letsencrypt.org` 等公共服务域名予以保留，用来判断故障是否为网络问题。
- 你自填的名称（如规则备注）若出现在日志里，且不像 IP 或域名，仍可能留在日志行中。发布前请快速浏览 `logs/*.log`，删掉不想公开的内容。

## 排查异常退出

Gately 运行时会把崩溃信息写进配置目录的 `crash.log`。进程意外结束后，下次启动时系统日志会出现一条提示，说明上次是崩溃退出还是被强制结束：

- **崩溃退出**：`crash.log` 中有崩溃原因与各协程堆栈，导出诊断包附在 issue 里即可。`concurrent map writes` 这类致命错误的 `fatal error: …` 一行 Go 只输出到标准错误，可在 `docker logs` 中查看。
- **意外结束**：进程没有留下任何信息，通常是内存不足被系统终止（OOM）、被 `kill -9` / `docker kill` 结束或断电。可以在宿主机上用 `dmesg -T | grep -i -E "oom|killed process"` 确认是否为 OOM。

## 接口

`GET /api/system/diagnostics`，仅登录会话可用，API 令牌无权访问。`?logs=0` 不含日志，`?preview=1` 以 JSON 返回各文件内容。
