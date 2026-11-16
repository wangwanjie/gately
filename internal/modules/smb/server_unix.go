//go:build linux || darwin

package smb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/wangwanjie/gately/internal/banlist"
	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/fsperm"
	"github.com/wangwanjie/gately/internal/gosamba/config"
	"github.com/wangwanjie/gately/internal/gosamba/discovery"
	"github.com/wangwanjie/gately/internal/gosamba/parent"
	"github.com/wangwanjie/gately/internal/gosamba/transport"
	"github.com/wangwanjie/gately/internal/gosamba/userdb"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// supported 表示当前系统可以运行 SMB 服务。
const supported = true

// durableTimeout 是断线后可续用的文件句柄保留时长，与 gosamba 默认值一致。
const durableTimeout = 60 * time.Second

// 认证完成前的连接限制。未登录的连接不需要凭据就能建立，不加限制时空连接会被一直持有，
// 大量连接还会耗尽文件描述符与内存，连带同一进程中的管理后台与其他服务。
// 登录成功后恢复原有的 5 分钟空闲超时与 16 MiB 帧上限，大文件传输不受影响。
const (
	// preAuthIdleTimeout 为登录前等待下一个请求的最长时间：访达、资源管理器在保存了密码时一秒内完成登录
	preAuthIdleTimeout = 30 * time.Second
	// preAuthDeadline 为从连接到完成登录的最长时间，给输错密码后重新输入留出余量
	preAuthDeadline = 60 * time.Second
	// preAuthMaxFrame 为登录前单个请求的上限：SESSION_SETUP 的安全缓冲区长度只有 16 位，正常请求不超过约 64 KiB
	preAuthMaxFrame = 128 << 10
	// preAuthMaxConns 为同时处于登录前的连接总数上限，preAuthMaxConnsPerIP 为单个来源（IPv6 按 /64）的上限；
	// 已登录的连接不计入。正常客户端每次连接只在登录前停留不到一秒，家庭与小团队远用不到这些名额
	preAuthMaxConns      = 256
	preAuthMaxConnsPerIP = 32
)

// server 是一次启动的 SMB 服务，持有监听与全部连接。
type server struct {
	ln     net.Listener
	cancel context.CancelFunc
	// done 在服务循环（含全部连接）结束后关闭
	done chan struct{}
	// durable 为断线续用的句柄表，服务结束后需要主动关闭其中保留的句柄
	durable *parent.DurableTable
	// serveErr 为服务循环异常退出的错误，只在 done 关闭前写入一次，done 关闭后可以无锁读取
	serveErr error
	// watcher 记录监听的不可恢复错误，出错后服务立即视为停止
	watcher *acceptWatcher
	// users 为认证时使用的账号列表；共享用户只改了密码时就地替换，不必重启服务
	users atomic.Pointer[[]config.UserConfig]
	// conns 记录每条连接已登录的用户名，修改某个用户的密码后只断开该用户的连接
	connsMu sync.Mutex
	conns   map[net.Conn]map[string]bool
}

// userConfigs 把账号转换为 gosamba 的用户配置。
func userConfigs(accounts []account, logs *logbuf.Buffer, warn bool) []config.UserConfig {
	uid, gid := os.Getuid(), os.Getgid()
	users := make([]config.UserConfig, 0, len(accounts))
	for _, u := range accounts {
		shares := []config.ShareConfig{}
		for _, sh := range u.Shares() {
			// 目录不存在时只记录警告，不影响其他共享；客户端访问该共享时会收到错误
			if st, err := os.Stat(sh.Path); warn && (err != nil || !st.IsDir()) {
				logs.Add(logbuf.LevelWarn, "", fmt.Sprintf("用户 %s 的共享 %s 目录不可用: %s", u.Name, sh.Name, sh.Path))
			}
			share := config.ShareConfig{Name: sh.Name, Path: sh.Path, ReadOnly: sh.ReadOnly}
			// 共享目录包含 Gately 的配置目录时隐藏配置目录（目录就是配置目录或位于其中的，已在 buildAccounts 中换成不存在的路径）。
			// 只在需要时设置，nil 的 *Guard 放进接口后不是 nil
			if g := fsperm.NewGuard(sh.Path); g != nil {
				share.Guard = g
			}
			shares = append(shares, share)
		}
		users = append(users, config.UserConfig{
			Name:   u.Name,
			NTHash: userdb.NTHash(u.Password),
			// 文件操作以 Gately 进程身份进行，不切换系统用户
			SystemUID:   uid,
			SystemGID:   gid,
			AllowShares: []string{"*"},
			Shares:      shares,
		})
	}
	return users
}

// reportingLimiter 在限流的同时把登录失败（含失败过多被拒绝）报告给入侵防护。
// gosamba 只对非匿名的登录调用限流器，Windows 资源管理器先匿名试探的失败不会被报告。
type reportingLimiter struct{ *fileusers.Throttle }

func (l reportingLimiter) Reserve(ip string) bool {
	if !l.Throttle.Reserve(ip) {
		fileusers.ReportAuthFailure("SMB", ip, "")
		return false
	}
	return true
}

func (l reportingLimiter) Done(ip string, ok bool) {
	l.Throttle.Done(ip, ok)
	if !ok {
		fileusers.ReportAuthFailure("SMB", ip, "")
	}
}

// loadUsers 返回当前的账号列表，每次认证时调用。
func (s *server) loadUsers() []config.UserConfig {
	if p := s.users.Load(); p != nil {
		return *p
	}
	return nil
}

// trackConn 登记一条连接，返回连接结束时的注销函数。
func (s *server) trackConn(c net.Conn) func() {
	s.connsMu.Lock()
	if s.conns == nil {
		s.conns = map[net.Conn]map[string]bool{}
	}
	s.conns[c] = map[string]bool{}
	s.connsMu.Unlock()
	return func() {
		s.connsMu.Lock()
		delete(s.conns, c)
		s.connsMu.Unlock()
	}
}

// noteLogin 记录连接 c 上登录成功的用户。SMB 用户名不区分大小写，统一按小写记录。
func (s *server) noteLogin(c net.Conn, name string) {
	s.connsMu.Lock()
	if users := s.conns[c]; users != nil {
		users[strings.ToLower(name)] = true
	}
	s.connsMu.Unlock()
}

// updatePasswords 换用新的账号列表（只有密码不同），并断开 changed 中用户已登录的连接；其他用户的连接与认证失败计数都保留。
// 被断开的客户端重新连接时需要用新密码登录。返回断开的连接数。
func (s *server) updatePasswords(accounts []account, changed []string) int {
	users := userConfigs(accounts, nil, false)
	s.users.Store(&users)
	drop := map[string]bool{}
	for _, n := range changed {
		drop[strings.ToLower(n)] = true
	}
	var list []net.Conn
	s.connsMu.Lock()
	for c, names := range s.conns {
		for n := range names {
			if drop[n] {
				list = append(list, c)
				break
			}
		}
	}
	s.connsMu.Unlock()
	for _, c := range list {
		_ = c.Close()
	}
	return len(list)
}

// acceptWatcher 包装监听，Accept 出现不可恢复的错误时记录错误并取消服务上下文。
// gosamba 的 Serve 遇到这种错误后会等全部连接结束才返回，期间状态仍会显示为运行中；
// 取消上下文让现有连接随之断开，Serve 尽快返回，状态也能立即反映服务已停止。
type acceptWatcher struct {
	net.Listener
	ctx    context.Context
	cancel context.CancelFunc
	failed atomic.Pointer[error]
}

func (w *acceptWatcher) Accept() (net.Conn, error) {
	c, err := w.Listener.Accept()
	if err != nil && w.ctx.Err() == nil && !parent.IsTemporaryAcceptError(err) {
		w.failed.CompareAndSwap(nil, &err)
		w.cancel()
	}
	return c, err
}

// failure 返回记录的不可恢复错误，没有时返回 nil。
func (w *acceptWatcher) failure() error {
	if p := w.failed.Load(); p != nil {
		return *p
	}
	return nil
}

// startServer 按配置快照监听并开始服务。
// accounts 为本地用户与共享用户，见 buildAccounts；throttle 为认证失败限流，由模块持有，服务重启后失败次数不清零。
func startServer(s Settings, accounts []account, logs *logbuf.Buffer, throttle *fileusers.Throttle) (*server, error) {
	log := slog.New(&logHandler{logs: logs, debug: os.Getenv(debugEnv) != ""})
	users := userConfigs(accounts, logs, true)
	opts := parent.ConnOptions{
		RequireEncryption: s.RequireEncryption,
		RequireSigning:    true,
		MaxIOSize:         8 << 20,
		Users:             users,
		// 全局共享为空：每个用户只能看到并连接自己的共享
		Shares:         []config.ShareConfig{},
		DurableTimeout: durableTimeout,
		// 按来源地址限制认证失败次数（与 WebDAV、FTP 相同：15 分钟内失败 10 次后拒绝）；
		// 限流器由模块持有，修改设置重启服务后失败次数仍然保留
		AuthLimiter: reportingLimiter{throttle},
		PreAuth: &parent.PreAuthLimits{
			IdleTimeout: preAuthIdleTimeout,
			Deadline:    preAuthDeadline,
			MaxFrame:    preAuthMaxFrame,
			Slots:       &parent.PreAuthSlots{MaxTotal: preAuthMaxConns, MaxPerIP: preAuthMaxConnsPerIP},
		},
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(s.ListenIP, strconv.Itoa(s.Port)))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	// 断线续用的句柄表与会话索引在整个服务内共享，客户端换新连接后仍能找回原来的句柄与会话
	durable := parent.NewDurableTable()
	watcher := &acceptWatcher{Listener: banlist.Default.Listener(ln), ctx: ctx, cancel: cancel}
	srv := &server{ln: ln, cancel: cancel, done: make(chan struct{}), durable: durable, watcher: watcher}
	srv.users.Store(&users)
	durable.StartSweeper(ctx, 30*time.Second)
	sessions := parent.NewSessionIndex()
	l := &parent.Listener{
		Log:      log,
		MaxFrame: transport.MaxFrameSize,
		Handler: func(ctx context.Context, c net.Conn, lg *slog.Logger, maxFrame uint32) {
			o := opts
			o.Durable = durable
			o.Sessions = sessions
			// 账号列表每次认证时读取，修改密码后无需重启
			o.UsersFunc = srv.loadUsers
			untrack := srv.trackConn(c)
			defer untrack()
			o.OnAuthenticated = func(sess *parent.Session) error {
				srv.noteLogin(c, sess.User.Name)
				return nil
			}
			parent.ServeConn(ctx, c, lg, maxFrame, o)
		},
	}
	if s.MDNS {
		advertise(ctx, ln, log, logs)
	}
	go func() {
		defer close(srv.done)
		// 被封禁的地址在 Accept 时直接断开
		err := l.Serve(ctx, watcher)
		if err == nil {
			// 监听失败时 watcher 已取消上下文，Serve 会按正常停止返回 nil，错误以 watcher 记录的为准
			err = watcher.failure()
		}
		if err != nil && !errors.Is(err, net.ErrClosed) {
			srv.serveErr = err
			logs.Add(logbuf.LevelWarn, "", "SMB 服务退出: "+err.Error())
		}
	}()
	return srv, nil
}

// advertise 通过 mDNS 广播 _smb._tcp 服务，失败只记录日志。
func advertise(ctx context.Context, ln net.Listener, log *slog.Logger, logs *logbuf.Buffer) {
	port := ln.Addr().(*net.TCPAddr).Port
	host, _ := os.Hostname()
	instance, host := mdnsNames(host)
	closer, err := discovery.Advertise(ctx, instance, host, port, log)
	if err != nil {
		logs.Add(logbuf.LevelWarn, "", "Bonjour 广播失败: "+err.Error())
		return
	}
	go func() {
		<-ctx.Done()
		_ = closer.Close()
	}()
}

// mdnsNames 由系统主机名生成 Bonjour 服务实例名与主机名：主机名只取第一段，
// DNS 标签不超过 63 字节，实例名形如「主机名 (Gately)」，便于与 NAS 自带的 SMB 服务区分。
func mdnsNames(hostname string) (instance, host string) {
	host, _, _ = strings.Cut(hostname, ".")
	if host == "" {
		host = "gately"
	}
	host = truncateBytes(host, 63)
	return truncateBytes(host, 63-len(" (Gately)")) + " (Gately)", host
}

// truncateBytes 按字节截断字符串，不截断多字节字符。
func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// Addr 返回实际监听地址。
func (s *server) Addr() string { return s.ln.Addr().String() }

// Close 关闭监听与全部连接，最多等待 5 秒让连接收尾。
// 连接全部结束后再关闭为断线续用保留的句柄：旧服务的句柄表不会再被使用，不关闭的话文件与锁要等 GC 才释放。
// 5 秒内没有收尾完的连接仍可能在使用这些句柄，此时改为在后台等它们结束后再关闭。
func (s *server) Close() {
	s.cancel()
	select {
	case <-s.done:
		s.durable.CloseAll()
	case <-time.After(5 * time.Second):
		go func() {
			<-s.done
			s.durable.CloseAll()
		}()
	}
}

// Status 返回服务循环是否仍在运行，以及异常退出时的错误。
// 监听出现不可恢复的错误后服务循环会退出，此时不应再显示为运行中。
func (s *server) Status() (running bool, err error) {
	if err := s.watcher.failure(); err != nil {
		return false, err
	}
	select {
	case <-s.done:
		return false, s.serveErr
	default:
		return true, nil
	}
}

// tooManyFailures 是 gosamba 因来源认证失败过多而拒绝登录时的错误前缀。
const tooManyFailures = "too many failed logins"

// failedUser 匹配 gosamba 认证失败错误中的用户名。
var failedUser = regexp.MustCompile(`user ("(?:[^"\\]|\\.)*")`)

// logHandler 把 gosamba 的结构化日志转换为模块运行日志：
// 登录成功与认证失败改写为易读的中文记录，其余只保留警告与错误，连接建立、协商等调试信息丢弃。
type logHandler struct {
	logs  *logbuf.Buffer
	attrs []slog.Attr
	// debug 为 true 时原样记录 gosamba 的全部日志（含调试级别），用于排查客户端兼容问题
	debug bool
}

// debugEnv 为非空时开启 SMB 协议调试日志。
const debugEnv = "GATELY_SMB_DEBUG"

func (h *logHandler) Enabled(_ context.Context, l slog.Level) bool {
	return h.debug || l >= slog.LevelInfo
}

func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &logHandler{logs: h.logs, attrs: append(append([]slog.Attr{}, h.attrs...), attrs...), debug: h.debug}
}

// WithGroup 不区分分组，属性仍按名称读取。
func (h *logHandler) WithGroup(string) slog.Handler { return h }

func (h *logHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := map[string]string{}
	var extra []string
	collect := func(a slog.Attr) bool {
		v := a.Value.Resolve().String()
		attrs[a.Key] = v
		if a.Key != "remote" {
			extra = append(extra, a.Key+"="+v)
		}
		return true
	}
	for _, a := range h.attrs {
		collect(a)
	}
	r.Attrs(collect)
	remote := attrs["remote"]
	if h.debug {
		h.logs.Add(logbuf.LevelDebug, "", strings.TrimSpace(remote+" "+r.Message+" "+strings.Join(extra, " ")))
	}
	switch r.Message {
	case "session authenticated":
		h.logs.Add(logbuf.LevelInfo, "", fmt.Sprintf("%s 登录（用户 %q）", remote, attrs["smb_user"]))
		return nil
	case "auth failed":
		// 来源失败次数过多被限流：不再校验密码，直接拒绝
		if strings.HasPrefix(attrs["err"], tooManyFailures) {
			if m := failedUser.FindStringSubmatch(attrs["err"]); m != nil {
				if name, err := strconv.Unquote(m[1]); err == nil {
					h.logs.Add(logbuf.LevelWarn, "", fmt.Sprintf("%s 登录失败次数过多，已拒绝（用户 %q）", remote, name))
					return nil
				}
			}
			h.logs.Add(logbuf.LevelWarn, "", fmt.Sprintf("%s 登录失败次数过多，已拒绝", remote))
			return nil
		}
		// 错误形如 unknown user "x" 或 user "x": ...，取出用户名，与 FTP 的认证失败记录格式一致
		if m := failedUser.FindStringSubmatch(attrs["err"]); m != nil {
			if name, err := strconv.Unquote(m[1]); err == nil {
				h.logs.Add(logbuf.LevelWarn, "", fmt.Sprintf("%s 认证失败（用户 %q）", remote, name))
				return nil
			}
		}
		h.logs.Add(logbuf.LevelWarn, "", fmt.Sprintf("%s 认证失败: %s", remote, attrs["err"]))
		return nil
	case "session-setup error":
		// 认证失败已由 auth failed 记录，这里只是同一错误的重复
		if strings.Contains(attrs["err"], "authentication failed") || strings.Contains(attrs["err"], "unknown user") || strings.HasPrefix(attrs["err"], tooManyFailures) {
			return nil
		}
	case "delete-on-close failed":
		// 访达删除文件夹时会先对非空目录设置关闭时删除，随后逐个删除内容，属于正常现象
		if strings.Contains(attrs["err"], "directory not empty") {
			return nil
		}
	case "too many unauthenticated connections; refusing new ones":
		// 同一时刻最多记录一条（gosamba 内部按 30 秒限频），上限数值写入日志便于判断是否需要排查
		h.logs.Add(logbuf.LevelWarn, "", fmt.Sprintf("%s 未登录的连接过多（全部来源上限 %s，单个来源上限 %s），已拒绝新连接", remote, attrs["max_total"], attrs["max_per_ip"]))
		return nil
	case "negotiate failed":
		// 客户端常同时尝试多个地址，再关掉多余的连接，协商前断开属于正常现象
		if strings.HasSuffix(attrs["err"], "EOF") || strings.Contains(attrs["err"], "connection reset") {
			return nil
		}
	}
	if r.Level < slog.LevelWarn {
		return nil
	}
	level := logbuf.LevelWarn
	if r.Level >= slog.LevelError {
		level = logbuf.LevelError
	}
	msg := r.Message
	if len(extra) > 0 {
		msg += " " + strings.Join(extra, " ")
	}
	if remote != "" {
		msg = remote + " " + msg
	}
	h.logs.Add(level, "", msg)
	return nil
}
