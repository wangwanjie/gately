package app

// 配置变更确认（超时自动回滚）。
//
// 通过域名远程管理时，修改端口转发、协议分流、Web 服务或全局设置可能切断管理界面自身的访问路径，
// 一旦保存就只能登录服务器补救。管理界面保存这些配置时带上 X-Gately-Confirm 请求头，服务端把
// 改动前的配置文件内容记为一次待确认的变更；管理界面随后经新建立的 WebSocket 连接确认访问正常。
// 浏览器总是为 WebSocket 新建 TCP 连接（Go 的 HTTP/2 默认不开启扩展 CONNECT，不会复用已有连接），
// 这条连接经过的是修改后的转发与监听；已有的长连接在热更新后仍连着旧目标，不能用来确认。
//
// 超时未确认时 Gately 重启，并在下次启动、加载任何配置之前恢复改动前的内容：模块运行中随时可能
// 写回内存中的配置，在启动阶段恢复不会与它们竞争。待确认状态保存在配置目录的 pending-confirm.state，
// 进程重启后继续计时；连续启动多次仍未确认（例如新配置让进程无法正常运行）同样回滚。
// 需要重启才生效的设置（管理界面监听地址、功能模块开关）在经管理界面重启时登记，重启后开始计时。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/wangwanjie/gately/internal/applyhold"
	"github.com/wangwanjie/gately/internal/auth"
	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/demo"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/fsperm"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
)

const (
	// confirmHeader 为管理界面在保存需要确认的配置时携带的请求头，值为 1
	confirmHeader = "X-Gately-Confirm"
	// confirmStateName 为待确认状态文件。不以 .json 结尾：配置备份、历史版本与声明式配置目录都不包含它
	confirmStateName = "pending-confirm.state"
	// 确认时限（秒）：设置为 0 时取默认值，负数表示关闭
	defaultConfirmTimeout = 90
	minConfirmTimeout     = 30
	maxConfirmTimeout     = 600
	// confirmBootGrace 为进程重启后至少保留的确认时间，重启本身要花时间，浏览器也要重新连上
	confirmBootGrace = 45 * time.Second
	// confirmMaxBoots 为待确认期间允许的启动次数，超过即回滚：新配置可能让进程反复崩溃或无法监听
	confirmMaxBoots = 3
	// confirmSocketTimeout 为确认连接从接受到写出结果的最长时间
	confirmSocketTimeout = 10 * time.Second
)

// confirmModules 为保存后需要确认的模块：它们的配置决定入站访问链路。
var confirmModules = []string{"portforward", "protoroute", "webservice"}

// 确认结果
const (
	confirmConfirmed   = "confirmed"
	confirmNone        = "none"
	confirmWaitRestart = "waitRestart"
	confirmRollingBack = "rollingBack"
	// confirmHold 表示处于批量修改中，修改尚未生效，无需确认
	confirmHold = "hold"
	// confirmApplying 表示确认请求在修改生效完成之前就已到达，可能经由尚未切换的旧路径，不能据此确认，页面应重试
	confirmApplying = "applying"
)

// pendingConfirm 是一次待确认的配置变更，可能包含多次保存：同一配置只记录第一次保存前的内容。
type pendingConfirm struct {
	ID      string    `json:"id"`
	Created time.Time `json:"created"`
	// Deadline 为确认截止时间；WaitRestart 为 true 时尚未开始计时
	Deadline time.Time `json:"deadline"`
	// Timeout 为登记时的确认时限（秒），重启后按它重新计时（此时全局设置可能正待回滚，不读取设置）
	Timeout int `json:"timeout"`
	// WaitRestart 表示变更需要重启才生效，已请求重启，下次启动时开始计时
	WaitRestart bool `json:"waitRestart,omitempty"`
	// Rollback 表示已决定回滚（超时或手动），下次启动时无条件恢复
	Rollback bool `json:"rollback,omitempty"`
	// Hold 表示处于批量修改中：保存的内容暂缓生效，不计时；结束批量修改（全部生效）时开始计时，
	// 进程在此期间重启时按新配置启动，与 WaitRestart 一样从启动时开始计时
	Hold bool `json:"hold,omitempty"`
	// Applied 为修改开始生效的时间（第一次保存、批量修改全部生效或带着修改启动），据此收集生效后模块报告的错误
	Applied time.Time `json:"applied,omitempty"`
	// Errors 为决定回滚时收集到的模块错误（例如监听启动失败），回滚后告诉管理员原因
	Errors []string `json:"errors,omitempty"`
	// Boots 为登记后经历的启动次数
	Boots int `json:"boots,omitempty"`
	// Files 为配置名 → 改动前的内容；值为 null 表示改动前文件不存在，回滚时删除
	Files map[string]*string `json:"files"`
}

// names 返回涉及的配置名（排序后）。
func (p *pendingConfirm) names() []string {
	out := make([]string, 0, len(p.Files))
	for n := range p.Files {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// confirmState 是运行中的待确认变更。
type confirmState struct {
	// mu 保护 tx 与 timer
	mu    sync.Mutex
	tx    *pendingConfirm
	timer *time.Timer
	// writeMu 串行化带确认请求头的保存：保存前读取原内容、保存后比较，期间不能插入另一次受保护的保存。
	// 确认接口也先取得它，等进行中的保存登记完待确认变更：响应先于登记写出，浏览器可能抢先发起确认
	writeMu sync.Mutex
	// effective 为最近一次修改生效完成的时间（受保护的保存登记完毕、批量修改全部生效），由 mu 保护。
	// 在它之前到达的确认请求可能经由尚未切换的旧路径（批量生效时排在后面的规则还没切换，热更新又不断开已有连接），
	// 不能据此确认
	effective time.Time
	// recovered 为启动时对上次遗留状态的处理结果，Run 中写日志、发通知
	recovered confirmRecovery
}

// confirmRecovery 是启动时处理遗留待确认变更的结果。
type confirmRecovery struct {
	// resume 为继续等待确认的变更
	resume *pendingConfirm
	// rolledBack 为已回滚的配置名；reason 为回滚原因：timeout、manual 或 boots
	rolledBack []string
	reason     string
	timeout    int
	// errs 为处理过程中的错误
	errs []string
	// cliMsgs 为启动时处理遗留命令行回滚请求的结果
	cliMsgs []string
	// applyErrs 为回滚掉的变更生效期间模块报告的错误
	applyErrs []string
}

func confirmStatePath(dir string) string {
	return filepath.Join(dir, confirmStateName)
}

func loadPendingConfirm(dir string) (*pendingConfirm, error) {
	data, err := os.ReadFile(confirmStatePath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var p pendingConfirm
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	if p.Files == nil {
		p.Files = map[string]*string{}
	}
	return &p, nil
}

// savePendingConfirm 原子写入待确认状态：先写临时文件再 rename，断电不会留下半个文件。
func savePendingConfirm(dir string, p *pendingConfirm) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	tmp := confirmStatePath(dir) + ".tmp"
	if err := fsperm.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, confirmStatePath(dir)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func removePendingConfirm(dir string) error {
	if err := os.Remove(confirmStatePath(dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// restoreConfirmFiles 把配置文件恢复为改动前的内容，返回失败的说明。
func restoreConfirmFiles(store *config.Store, p *pendingConfirm) []string {
	var errs []string
	for _, name := range p.names() {
		orig := p.Files[name]
		var err error
		if orig == nil {
			err = store.Remove(name)
		} else {
			err = store.WriteRaw(name, []byte(*orig))
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("恢复配置 %s 失败: %v", name, err))
		}
	}
	return errs
}

// recoverPendingConfirm 在启动时、加载任何配置之前处理上次运行遗留的待确认变更：
// 已决定回滚、已超时或启动次数过多时恢复改动前的内容；否则继续计时（至少保留 confirmBootGrace）。
func recoverPendingConfirm(store *config.Store, now time.Time) confirmRecovery {
	dir := store.Dir()
	p, err := loadPendingConfirm(dir)
	if err != nil {
		// 状态文件损坏时无法得知改动前的内容，只能放弃
		_ = removePendingConfirm(dir)
		return confirmRecovery{errs: []string{"读取待确认的配置变更失败，已忽略: " + err.Error()}}
	}
	if p == nil {
		return confirmRecovery{}
	}
	if p.Timeout <= 0 {
		p.Timeout = defaultConfirmTimeout
	}
	reason := ""
	switch {
	case p.Rollback && now.Before(p.Deadline):
		reason = "manual"
	case p.Rollback || (!p.WaitRestart && !p.Hold && !now.Before(p.Deadline)):
		reason = "timeout"
	case p.Boots >= confirmMaxBoots:
		reason = "boots"
	}
	if reason != "" {
		rec := confirmRecovery{rolledBack: p.names(), reason: reason, timeout: p.Timeout, applyErrs: p.Errors}
		rec.errs = restoreConfirmFiles(store, p)
		if err := removePendingConfirm(dir); err != nil {
			rec.errs = append(rec.errs, "删除待确认状态失败: "+err.Error())
		}
		return rec
	}
	p.Boots++
	if p.WaitRestart || p.Hold {
		// 需要重启才生效的变更、批量修改中途重启时暂存的修改，都从这次启动开始计时
		p.WaitRestart, p.Hold = false, false
		p.Deadline = now.Add(time.Duration(p.Timeout) * time.Second)
		p.Applied = now
	} else if min := now.Add(confirmBootGrace); p.Deadline.Before(min) {
		p.Deadline = min
	}
	if err := savePendingConfirm(dir, p); err != nil {
		return confirmRecovery{resume: p, errs: []string{"保存待确认状态失败: " + err.Error()}}
	}
	return confirmRecovery{resume: p}
}

// confirmTimeout 返回当前设置下的确认时限（秒），0 表示关闭。
func (s Settings) confirmTimeout() int {
	switch {
	case s.ConfirmTimeout < 0:
		return 0
	case s.ConfirmTimeout == 0:
		return defaultConfirmTimeout
	}
	return s.ConfirmTimeout
}

// confirmEnabled 判断当前是否启用确认：演示模式不启用（不运行数据面，也不允许写配置）。
func (a *App) confirmEnabled() bool {
	return !demo.Enabled() && a.settingsSnapshot().confirmTimeout() > 0
}

// confirmTargets 返回这次请求保存后需要确认的配置名，不需要时返回 nil。
// 只处理管理员登录会话带确认请求头的写请求：令牌无法调用确认接口，脚本不会被意外回滚。
func (a *App) confirmTargets(r *http.Request) []string {
	if r.Header.Get(confirmHeader) != "1" || r.Method == http.MethodGet || r.Method == http.MethodHead {
		return nil
	}
	// 批量修改期间即使关闭了确认也要记录改动前的内容：放弃修改时据此恢复
	if auth.IsToken(r.Context()) || auth.RoleFromContext(r.Context()) != auth.RoleAdmin || demo.Enabled() ||
		(!a.confirmEnabled() && !applyhold.Active()) {
		return nil
	}
	if r.URL.Path == "/api/system/settings" && r.Method == http.MethodPut {
		return []string{settingsName}
	}
	rest, ok := strings.CutPrefix(r.URL.Path, "/api/")
	if !ok {
		return nil
	}
	mod, _, _ := strings.Cut(rest, "/")
	if slices.Contains(confirmModules, mod) && a.moduleEnabled(mod) {
		return []string{mod}
	}
	return nil
}

// confirmGuard 为需要确认的保存登记待确认变更：保存前读取配置文件，保存后内容有变化即登记改动前的内容。
// 只看文件内容是否变化，不改变配置的写入（导出缓存、断开连接等不改配置的写请求不会登记）。
func (a *App) confirmGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		names := a.confirmTargets(r)
		if len(names) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		c := &a.confirm
		c.writeMu.Lock()
		defer c.writeMu.Unlock()
		before := map[string][]byte{}
		for _, n := range names {
			data, err := a.store.ReadRaw(n)
			if err != nil {
				// 读不到原内容就无法回滚，照常保存
				a.sysLogs.Add(logbuf.LevelWarn, "system", fmt.Sprintf("读取配置 %s 失败，本次修改不会在未确认时自动回滚: %v", n, err))
				next.ServeHTTP(w, r)
				return
			}
			before[n] = data
		}
		next.ServeHTTP(w, r)
		changed := map[string]*string{}
		for _, n := range names {
			after, err := a.store.ReadRaw(n)
			if err != nil || string(after) == string(before[n]) {
				continue
			}
			if before[n] == nil {
				changed[n] = nil
			} else {
				s := string(before[n])
				changed[n] = &s
			}
		}
		if len(changed) > 0 {
			a.beginConfirm(changed)
		}
	})
}

// beginConfirm 登记一次保存：并入进行中的待确认变更（同一配置保留最早的原内容）并重新计时。
func (a *App) beginConfirm(changed map[string]*string) {
	hold := applyhold.Active()
	timeout := a.settingsSnapshot().confirmTimeout()
	if timeout <= 0 {
		if !hold {
			return
		}
		// 关闭了确认的批量修改：仍记录原内容供放弃时恢复，全部生效时不计时
		timeout = defaultConfirmTimeout
	}
	c := &a.confirm
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	p := c.tx
	fresh := p == nil
	if fresh {
		p = &pendingConfirm{ID: randomHex(8), Created: now, Files: map[string]*string{}}
	} else {
		// 写盘失败时不改动进行中的变更
		cp := *p
		cp.Files = maps.Clone(p.Files)
		p = &cp
	}
	for n, orig := range changed {
		if _, ok := p.Files[n]; !ok {
			p.Files[n] = orig
		}
	}
	// 已决定回滚或等待重启时不再改动计时，新保存的内容随之回滚；批量修改期间不计时
	if hold {
		p.Hold = true
	}
	if !p.Rollback && !p.WaitRestart && !p.Hold {
		p.Timeout = timeout
		p.Deadline = now.Add(time.Duration(timeout) * time.Second)
		if p.Applied.IsZero() {
			p.Applied = now
		}
	}
	if err := savePendingConfirm(a.store.Dir(), p); err != nil {
		a.sysLogs.Add(logbuf.LevelWarn, "system", "保存待确认状态失败，本次修改不会在未确认时自动回滚: "+err.Error())
		return
	}
	c.tx = p
	if !hold {
		// 保存已生效（模块在处理保存请求时已重启或热更新），此前到达的确认请求不算数
		c.effective = time.Now()
	}
	a.armConfirmTimerLocked()
	if fresh && !p.Hold {
		a.sysLogs.Add(logbuf.LevelInfo, "system", fmt.Sprintf("配置变更（%s）待确认：%d 秒内未在管理界面确认访问正常将自动回滚", a.confirmTitles(p.names()), timeout))
	}
}

// armConfirmTimerLocked 按 tx.Deadline 重新设置超时回滚定时器，调用方需持有 c.mu。
func (a *App) armConfirmTimerLocked() {
	c := &a.confirm
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	p := c.tx
	if p == nil || p.WaitRestart || p.Rollback || p.Hold {
		return
	}
	id := p.ID
	c.timer = time.AfterFunc(time.Until(p.Deadline), func() { a.confirmExpired(id) })
}

// confirmExpired 在确认超时后标记回滚并重启，下次启动时恢复改动前的内容。
func (a *App) confirmExpired(id string) {
	c := &a.confirm
	c.mu.Lock()
	p := c.tx
	if p == nil || p.ID != id || p.WaitRestart || p.Rollback || p.Hold {
		c.mu.Unlock()
		return
	}
	if time.Now().Before(p.Deadline) {
		// 期间截止时间被延后（又保存了一次）：按新的截止时间重新计时
		a.armConfirmTimerLocked()
		c.mu.Unlock()
		return
	}
	cp := *p
	cp.Rollback = true
	cp.Errors = a.applyErrors(p.Applied)
	if err := savePendingConfirm(a.store.Dir(), &cp); err == nil {
		c.tx = &cp
	}
	// 写盘失败时截止时间同样已过，启动时照样回滚
	c.mu.Unlock()
	a.sysLogs.Add(logbuf.LevelWarn, "system", fmt.Sprintf("配置变更（%s）在 %d 秒内未确认，正在重启并回滚到修改前的配置", a.confirmTitles(p.names()), p.Timeout))
	a.scheduleRestart()
}

// confirmPending 确认待确认的变更，返回 confirmConfirmed、confirmNone、confirmWaitRestart、confirmRollingBack
// 或 confirmApplying。arrived 为确认请求到达的时间：早于修改生效完成的请求可能经由旧路径，不能确认。
func (a *App) confirmPending(arrived time.Time) (string, error) {
	c := &a.confirm
	// 等进行中的受保护保存登记完毕、批量修改全部生效
	c.writeMu.Lock()
	c.writeMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.tx
	switch {
	case p == nil:
		return confirmNone, nil
	case p.Rollback:
		return confirmRollingBack, nil
	case p.WaitRestart:
		return confirmWaitRestart, nil
	case p.Hold:
		return confirmHold, nil
	case arrived.Before(c.effective):
		return confirmApplying, nil
	}
	// 状态文件留着会在下次启动时回滚，删除失败就不能算确认
	if err := removePendingConfirm(a.store.Dir()); err != nil {
		return "", err
	}
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.tx = nil
	a.sysLogs.Add(logbuf.LevelInfo, "system", fmt.Sprintf("配置变更（%s）已确认生效", a.confirmTitles(p.names())))
	return confirmConfirmed, nil
}

// discardPendingConfirm 放弃待确认的变更（不回滚），用于从备份或历史版本恢复配置：
// 恢复后的配置由用户明确选择，不应在下次启动时被改回。
func (a *App) discardPendingConfirm() {
	applyhold.Drop()
	c := &a.confirm
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	if c.tx != nil {
		c.tx = nil
		a.sysLogs.Add(logbuf.LevelInfo, "system", "已放弃待确认的配置变更")
	}
	_ = removePendingConfirm(a.store.Dir())
}

// restartRollbackFiles 返回经管理界面重启时需要登记的改动：需要重启才生效的设置（管理界面监听地址、
// 功能模块开关）与本次启动时不同，就把这几项恢复为启动时的值作为回滚内容，其余设置保持当前值。
func (a *App) restartRollbackFiles() map[string]*string {
	var cur Settings
	if _, err := a.store.Load(settingsName, &cur); err != nil {
		return nil
	}
	cur.normalize()
	boot := a.bootSettings
	if !restartRequired(boot, cur) {
		return nil
	}
	cur.Listen, cur.HTTPSListen, cur.DisabledModules = boot.Listen, boot.HTTPSListen, boot.DisabledModules
	data, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		return nil
	}
	s := string(append(data, '\n'))
	return map[string]*string{settingsName: &s}
}

// prepareConfirmRestart 在经管理界面重启前登记：需要重启才生效的设置有变化时并入待确认变更，
// 有待确认变更时改为下次启动后开始计时。
func (a *App) prepareConfirmRestart(r *http.Request) {
	if r.Header.Get(confirmHeader) != "1" || auth.RoleFromContext(r.Context()) != auth.RoleAdmin || !a.confirmEnabled() {
		return
	}
	extra := a.restartRollbackFiles()
	c := &a.confirm
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tx == nil && len(extra) == 0 {
		return
	}
	p := &pendingConfirm{ID: randomHex(8), Created: time.Now(), Files: map[string]*string{}}
	if c.tx != nil {
		cp := *c.tx
		cp.Files = maps.Clone(c.tx.Files)
		p = &cp
	}
	if p.Rollback {
		return
	}
	for n, orig := range extra {
		if _, ok := p.Files[n]; !ok {
			p.Files[n] = orig
		}
	}
	p.Timeout = a.settingsSnapshot().confirmTimeout()
	p.WaitRestart = true
	// 管理员主动重启不算异常启动，重新计数
	p.Boots = 0
	if err := savePendingConfirm(a.store.Dir(), p); err != nil {
		a.sysLogs.Add(logbuf.LevelWarn, "system", "保存待确认状态失败，重启后的配置不会在未确认时自动回滚: "+err.Error())
		return
	}
	c.tx = p
	a.armConfirmTimerLocked()
	a.sysLogs.Add(logbuf.LevelInfo, "system", fmt.Sprintf("配置变更（%s）将在重启后等待确认，%d 秒内未确认将自动回滚", a.confirmTitles(p.names()), p.Timeout))
}

// rollbackPending 立即回滚待确认的变更：标记回滚并重启。没有待确认变更时返回 false。
func (a *App) rollbackPending() (bool, error) {
	c := &a.confirm
	c.writeMu.Lock()
	c.writeMu.Unlock()
	c.mu.Lock()
	p := c.tx
	if p == nil {
		c.mu.Unlock()
		return false, nil
	}
	cp := *p
	cp.Rollback = true
	if !p.Hold {
		cp.Errors = a.applyErrors(p.Applied)
	} else {
		// 批量修改期间没有截止时间（或沿用并入时已过的截止时间）：与放弃批量修改一样补上，
		// 启动时据此识别为手动回滚，否则会按“超时未确认”提示
		if cp.Timeout <= 0 {
			cp.Timeout = defaultConfirmTimeout
		}
		cp.Deadline = time.Now().Add(time.Duration(cp.Timeout) * time.Second)
	}
	if err := savePendingConfirm(a.store.Dir(), &cp); err != nil {
		c.mu.Unlock()
		return false, err
	}
	c.tx = &cp
	a.armConfirmTimerLocked()
	c.mu.Unlock()
	if p.Hold {
		// 暂存的修改不再生效，重启后按恢复的配置启动
		applyhold.Drop()
	}
	a.sysLogs.Add(logbuf.LevelWarn, "system", fmt.Sprintf("已手动回滚配置变更（%s），正在重启", a.confirmTitles(p.names())))
	a.scheduleRestart()
	return true, nil
}

// confirmTitles 把配置名换成界面上的名称。
func (a *App) confirmTitles(names []string) string {
	titles := make([]string, 0, len(names))
	for _, n := range names {
		titles = append(titles, a.confirmTitle(n))
	}
	return strings.Join(titles, "、")
}

func (a *App) confirmTitle(name string) string {
	if name == settingsName {
		return "全局设置"
	}
	for _, e := range a.modules {
		if e.mod.ID() == name {
			return e.mod.Title()
		}
	}
	return name
}

// startConfirm 在启动后报告上次遗留变更的处理结果，并为继续等待确认的变更开始计时。
func (a *App) startConfirm() {
	c := &a.confirm
	rec := c.recovered
	for _, m := range rec.cliMsgs {
		a.sysLogs.Add(logbuf.LevelWarn, "system", m)
	}
	for _, e := range rec.errs {
		a.sysLogs.Add(logbuf.LevelError, "system", e)
	}
	if len(rec.rolledBack) > 0 {
		titles := a.confirmTitles(rec.rolledBack)
		var msg string
		switch rec.reason {
		case "manual":
			msg = fmt.Sprintf("已按管理员要求把配置变更（%s）回滚到修改前的内容", titles)
		case "boots":
			msg = fmt.Sprintf("配置变更（%s）未确认且 Gately 已连续启动 %d 次，已回滚到修改前的内容", titles, confirmMaxBoots)
		default:
			msg = fmt.Sprintf("配置变更（%s）在 %d 秒内未确认，已回滚到修改前的内容；修改后的版本保存在配置历史中", titles, rec.timeout)
		}
		a.sysLogs.Add(logbuf.LevelWarn, "system", msg)
		for _, e := range rec.applyErrs {
			a.sysLogs.Add(logbuf.LevelWarn, "system", "回滚前出现的错误: "+e)
		}
		if len(rec.applyErrs) > 0 {
			msg += "\n" + strings.Join(rec.applyErrs, "\n")
		}
		// 通知中心在模块启动后才订阅事件
		events.Publish(events.Event{Type: events.ConfigRolledBack, Source: "管理后台", Level: events.LevelWarn, Title: "配置已自动回滚", Message: msg})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if p := rec.resume; p != nil {
		c.tx = p
		a.armConfirmTimerLocked()
		a.sysLogs.Add(logbuf.LevelInfo, "system", fmt.Sprintf("配置变更（%s）待确认，%d 秒内未在管理界面确认将自动回滚", a.confirmTitles(p.names()), int(time.Until(p.Deadline).Seconds())))
	}
}

// stopConfirmTimer 在停止服务时取消定时器：重启后由启动流程继续计时，不在关闭过程中再触发重启。
func (a *App) stopConfirmTimer() {
	c := &a.confirm
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}

// confirmView 是待确认变更的状态。
type confirmView struct {
	// Enabled 为是否启用确认，Timeout 为确认时限（秒）
	Enabled bool `json:"enabled"`
	Timeout int  `json:"timeout"`
	// Pending 为进行中的待确认变更，没有时为 null
	Pending *confirmPendingView `json:"pending"`
	// RolledBack 为本次启动时回滚掉的变更，没有时为 null：管理界面据此告诉等待确认的页面配置已回滚
	RolledBack *confirmRolledBackView `json:"rolledBack"`
	// Hold 为进行中的批量修改，没有时为 null
	Hold *holdView `json:"hold"`
}

type confirmRolledBackView struct {
	Configs []string `json:"configs"`
	// Errors 为修改生效期间模块报告的错误（例如监听启动失败），可能就是访问失败的原因
	Errors []string `json:"errors"`
	// Reason 为 timeout（超时未确认）、manual（手动回滚）或 boots（连续启动次数过多）
	Reason string `json:"reason"`
}

type confirmPendingView struct {
	ID string `json:"id"`
	// Configs 为涉及的配置名：模块 ID，全局设置为 gately
	Configs []string `json:"configs"`
	// Remaining 为距自动回滚的剩余秒数，WaitRestart 时为确认时限
	Remaining   int  `json:"remaining"`
	WaitRestart bool `json:"waitRestart"`
	Rollback    bool `json:"rollback"`
	Hold        bool `json:"hold"`
	// Errors 为修改生效以来模块报告的错误（例如监听启动失败）
	Errors []string `json:"errors"`
}

func (a *App) handleConfirmStatus(w http.ResponseWriter, r *http.Request) {
	s := a.settingsSnapshot()
	v := confirmView{Enabled: a.confirmEnabled(), Timeout: s.confirmTimeout()}
	c := &a.confirm
	if rec := c.recovered; len(rec.rolledBack) > 0 {
		v.RolledBack = &confirmRolledBackView{Configs: rec.rolledBack, Reason: rec.reason, Errors: rec.applyErrs}
		if v.RolledBack.Errors == nil {
			v.RolledBack.Errors = []string{}
		}
	}
	c.mu.Lock()
	if p := c.tx; p != nil {
		pv := &confirmPendingView{ID: p.ID, Configs: p.names(), WaitRestart: p.WaitRestart, Rollback: p.Rollback, Hold: p.Hold, Errors: p.Errors}
		if !p.Hold && !p.WaitRestart && !p.Rollback {
			pv.Errors = a.applyErrors(p.Applied)
		}
		if pv.Errors == nil {
			pv.Errors = []string{}
		}
		if p.WaitRestart {
			pv.Remaining = p.Timeout
		} else if left := time.Until(p.Deadline); left > 0 {
			pv.Remaining = int((left + time.Second - 1) / time.Second)
		}
		v.Pending = pv
	}
	c.mu.Unlock()
	if applyhold.Active() {
		v.Hold = a.holdView()
	}
	httpx.OK(w, v)
}

// handleConfirmSocket 经新建立的 WebSocket 连接确认待确认的变更：结果以一条 JSON 消息 {"status": ...} 返回后关闭连接。
// 只接受 WebSocket：普通请求可能经由修改前就建立的长连接到达，不能证明修改后的访问路径可用。
func (a *App) handleConfirmSocket(w http.ResponseWriter, r *http.Request) {
	// 到达时间在等待进行中的保存或生效之前记下，见 confirmPending
	arrived := time.Now()
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer ws.CloseNow()
	ctx, cancel := context.WithTimeout(r.Context(), confirmSocketTimeout)
	defer cancel()
	status, err := a.confirmPending(arrived)
	msg := map[string]string{"status": status}
	if err != nil {
		msg = map[string]string{"status": "error", "error": err.Error()}
		a.sysLogs.Add(logbuf.LevelError, "system", "确认配置变更失败: "+err.Error())
	}
	data, _ := json.Marshal(msg)
	if err := ws.Write(ctx, websocket.MessageText, data); err != nil {
		return
	}
	_ = ws.Close(websocket.StatusNormalClosure, "")
}

func (a *App) handleConfirmRollback(w http.ResponseWriter, r *http.Request) {
	ok, err := a.rollbackPending()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "回滚失败: %v", err)
		return
	}
	if !ok {
		httpx.Error(w, http.StatusConflict, "没有待确认的配置变更")
		return
	}
	httpx.OK(w, map[string]bool{"ok": true})
}

// maxApplyErrors 为收集的错误条数上限
const maxApplyErrors = 5

// applyErrors 收集修改生效以来三个模块报告的错误（例如监听启动失败：地址不是本机地址、端口被占用），
// 访问失败时它们往往就是原因。since 为零时返回空。
func (a *App) applyErrors(since time.Time) []string {
	if since.IsZero() {
		return nil
	}
	from := since.Add(-time.Second)
	type item struct {
		t   time.Time
		msg string
	}
	var all []item
	for _, e := range a.modules {
		if !slices.Contains(confirmModules, e.mod.ID()) {
			continue
		}
		for _, en := range e.logs.Range(0, 0, 500) {
			if en.Level != logbuf.LevelError || en.Time.Before(from) {
				continue
			}
			msg := en.Message
			if en.Source != "" {
				msg = en.Source + ": " + msg
			}
			all = append(all, item{en.Time, a.confirmTitle(e.mod.ID()) + " › " + msg})
		}
	}
	slices.SortFunc(all, func(x, y item) int { return x.t.Compare(y.t) })
	if len(all) > maxApplyErrors {
		all = all[len(all)-maxApplyErrors:]
	}
	out := make([]string, 0, len(all))
	for _, it := range all {
		out = append(out, it.msg)
	}
	return out
}
