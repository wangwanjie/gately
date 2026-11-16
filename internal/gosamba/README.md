# gosamba（内置副本）

本目录是 [ahmetozer/gosamba](https://github.com/ahmetozer/gosamba) 的源码副本，供 SMB 服务模块（`internal/modules/smb`）使用。上游把实现放在 `internal/` 下，无法作为依赖引入，因此按 Apache-2.0 许可证复制到这里，许可证原文见同目录 `LICENSE`。

- 上游版本：commit `607ed4b`（2026-09-16）
- 复制范围：`internal/` 下的 config、discovery、inotify、ntlm、parent、smb2、smb3、transport、userdb、vfs，未包含 `cmd/` 与 `internal/logging`
- 导入路径从 `github.com/ahmetozer/gosamba/internal/` 改为 `github.com/wangwanjie/gately/internal/gosamba/`

Gately 的改动（改动过的文件开头有注明）：

- `config.UserConfig` 新增 `Shares`：非 nil 时为该用户专属的共享列表
- `parent/gately.go`：新增 `Dispatcher.sharesFor`，TREE_CONNECT 查找共享与 srvsvc 枚举共享改为按登录用户解析，使不同用户的同名共享可以指向不同目录
- `parent/session.go`：SESSION_SETUP 按客户端请求的格式回复。Linux 内核 cifs 客户端默认发送不带 SPNEGO 的裸 NTLMSSP 消息，上游一律回复 SPNEGO，内核报 `blob signature incorrect` 无法挂载
- 签名密钥（`parent/session.go`、`parent/dispatch.go`、`parent/compound.go`、`parent/gately.go`）：SMB 3.1.1 协商出 AES-256 加密算法时，macOS 派生签名密钥用 L=128，Linux 内核 cifs 用 L=256 再取前 16 字节。上游只按 L=128 派生，内核挂载时签名校验失败。现在两种都备好，按客户端第一个签名请求锁定会话使用的密钥
- `discovery/mdns.go`：每个网卡只通告该网卡自己的 IPv4 地址。上游在所有网卡上通告本机全部地址，局域网客户端可能拿到 Docker 网桥等不可达的地址
- 签名密钥加锁：会话签名密钥的写入（`parent/session.go` 经 `setSigningKeys`）与读取（`parent/dispatch.go`、`parent/compound.go`、`parent/session.go` 经 `signingKey()`）都经过 `signingMu`，修复数据竞争
- `parent/gately.go`：新增 `DurableTable.CloseAll`，服务重启时关闭旧句柄表中为断线续用保留的句柄；导出 `IsTemporaryAcceptError` 供判断监听是否不可恢复地失败
- `vfs/path.go`：共享根目录为 `/` 时前缀判断不再拼成 `//`，根目录为 `/` 的共享可以正常使用
- 构建约束：`parent` 包的源文件（含测试）加上 `//go:build linux || darwin`，另有不带约束的 `parent/doc.go`，其他系统上 `go build/vet ./...` 可以通过
- 报文边界（新增 `bounds` 包）：客户端给出的 uint32 偏移与长度（NEGOTIATE 上下文、NTLMSSP 字段、CREATE 上下文、WRITE、SET_INFO、IOCTL、QUERY_INFO、报文链 NextCommand、改名文件名长度、扩展属性链表等）一律在 uint64 中判断边界后再转换为 int，32 位平台上不会因转换为负数而越过检查。涉及 `smb2/`、`ntlm/`、`smb3/encrypt.go`、`parent/conn.go`、`parent/dispatch.go`、`parent/xattr.go`
- 连接隔离（`parent/gately_recover.go`）：连接的读循环、工作池协程、写协程、CHANGE_NOTIFY 与阻塞锁等待协程、监听的连接处理协程都捕获 panic，记录错误日志（含调用栈）后只断开该连接，不让进程退出。涉及 `parent/conn.go`、`parent/listener.go`、`parent/writer.go`、`parent/dispatch.go`、`parent/lock.go`
- 认证前限制（`parent/gately_preauth.go`，`ConnOptions.PreAuth`）：认证完成前（含协商阶段，上游这里不设截止时间）按每帧超时与总时限设置读截止时间，单帧上限可调小（`NegotiatorOptions.MaxFrame`），并按全局与单个来源（IPv6 按 /64）限制同时处于认证前的连接数；认证成功后恢复原有的空闲超时与帧大小。认证前的超时断开按 Info 记录。为空时与上游行为一致

上游只支持 Linux 与 macOS 作为服务端，Gately 在其他系统上不编译这些包（见 `internal/modules/smb/server_other.go`）。

更新上游时：重新复制上述目录、替换导入路径，再按上面的列表重新应用改动，并运行 `go test ./internal/gosamba/... ./internal/modules/smb/`。
