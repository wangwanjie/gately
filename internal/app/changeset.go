package app

// 批量修改：有些修改要几处一起改才能用（例如协议分流转到 Web 服务的端口与 Web 服务监听的端口），
// 只保存第一处就会切断正经由它们访问的管理界面。开始批量修改后，端口转发、协议分流与 Web 服务的保存
// 只写入配置、暂缓生效（见 applyhold 包），旧的访问路径继续可用；全部改好后一次性生效，
// 随后与单次保存一样经新连接确认，确认不了时把整批修改一起回滚。放弃修改则恢复开始前的配置并重启。
// 改动前的内容记在待确认变更中（pendingConfirm.Hold），进程中途重启时按新配置启动并开始计时。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/applyhold"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// holdItem 是批量修改中暂缓生效的一项：模块与规则或监听。
type holdItem struct {
	Module string `json:"module"`
	Key    string `json:"key"`
	// Name 为规则或监听的名称，已删除时为空
	Name string `json:"name,omitempty"`
}

type holdView struct {
	// Items 为暂缓生效的规则与监听（按保存顺序）
	Items []holdItem `json:"items"`
	// Configs 为批量修改以来改动过的配置名
	Configs []string `json:"configs"`
}

func (a *App) holdView() *holdView {
	v := &holdView{Items: []holdItem{}, Configs: []string{}}
	cache := map[string]map[string]json.RawMessage{}
	for _, it := range applyhold.Pending() {
		mod := it[0]
		// 需要重新监听的项带 #relisten 后缀，与同一监听合并显示
		key, _, _ := strings.Cut(it[1], "#")
		if cache[mod] == nil {
			cache[mod] = a.loadPathConfig(mod)
		}
		item := holdItem{Module: mod, Key: key}
		if raw, ok := cache[mod][key]; ok {
			var n namedItem
			_ = json.Unmarshal(raw, &n)
			item.Name = n.Name
		}
		dup := false
		for _, x := range v.Items {
			if x.Module == item.Module && x.Key == item.Key {
				dup = true
			}
		}
		if !dup {
			v.Items = append(v.Items, item)
		}
	}
	c := &a.confirm
	c.mu.Lock()
	if c.tx != nil {
		v.Configs = c.tx.names()
	}
	c.mu.Unlock()
	return v
}

// handleHoldBegin 开始批量修改：此后三个模块的保存暂缓生效。已有未确认的变更时并入这批修改。
func (a *App) handleHoldBegin(w http.ResponseWriter, r *http.Request) {
	c := &a.confirm
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if applyhold.Active() {
		httpx.OK(w, a.holdView())
		return
	}
	c.mu.Lock()
	p := &pendingConfirm{ID: randomHex(8), Created: time.Now(), Timeout: defaultConfirmTimeout, Files: map[string]*string{}}
	if c.tx != nil {
		if c.tx.Rollback {
			c.mu.Unlock()
			httpx.Error(w, http.StatusConflict, "配置正在回滚，请稍后再试")
			return
		}
		cp := *c.tx
		p = &cp
	}
	p.Hold = true
	if err := savePendingConfirm(a.store.Dir(), p); err != nil {
		c.mu.Unlock()
		httpx.Error(w, http.StatusInternalServerError, "保存批量修改状态失败: %v", err)
		return
	}
	c.tx = p
	// 批量修改期间不计时
	a.armConfirmTimerLocked()
	c.mu.Unlock()
	applyhold.Begin()
	a.sysLogs.Add(logbuf.LevelInfo, "system", "已开始批量修改：端口转发、协议分流与 Web 服务的修改暂缓生效，全部改好后一起生效")
	httpx.OK(w, a.holdView())
}

type holdApplyResult struct {
	// Applied 为生效的规则与监听数量
	Applied int `json:"applied"`
	// Confirm 表示需要经新连接确认（未确认时到时自动回滚）
	Confirm bool `json:"confirm"`
	// Timeout 为确认时限（秒）
	Timeout int `json:"timeout"`
}

// holdApplyDelay 为全部生效前等待响应发出的时间：生效可能重启这次请求所经过的监听，先让响应离开
var holdApplyDelay = 300 * time.Millisecond

// handleHoldApply 结束批量修改，全部修改一次性生效；启用了确认且有改动时开始计时。
// 生效在响应发出后进行：这次请求往往正经过要重启的 Web 服务监听，同步重启会让监听等这个请求结束（最长 5 秒）后
// 再强行断开，响应随之丢失。生效完成前一直持有 writeMu，确认接口会等到生效之后才确认，不会经旧路径误判成功。
func (a *App) handleHoldApply(w http.ResponseWriter, r *http.Request) {
	c := &a.confirm
	c.writeMu.Lock()
	released := false
	defer func() {
		if !released {
			c.writeMu.Unlock()
		}
	}()
	if !applyhold.Active() {
		httpx.Error(w, http.StatusConflict, "没有进行中的批量修改")
		return
	}
	res := holdApplyResult{}
	c.mu.Lock()
	if p := c.tx; p != nil && p.Hold {
		cp := *p
		cp.Hold = false
		timeout := a.settingsSnapshot().confirmTimeout()
		if len(cp.Files) == 0 || timeout <= 0 {
			// 没有改动，或关闭了确认：不再需要记录
			_ = removePendingConfirm(a.store.Dir())
			c.tx = nil
		} else {
			cp.Timeout = timeout
			cp.Deadline = time.Now().Add(time.Duration(timeout) * time.Second)
			cp.Applied = time.Now()
			if err := savePendingConfirm(a.store.Dir(), &cp); err != nil {
				c.mu.Unlock()
				httpx.Error(w, http.StatusInternalServerError, "保存待确认状态失败: %v", err)
				return
			}
			c.tx = &cp
			a.armConfirmTimerLocked()
			res.Confirm, res.Timeout = true, timeout
		}
	}
	c.mu.Unlock()
	res.Applied = len(applyhold.Pending())
	httpx.OK(w, res)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	released = true
	go func() {
		defer c.writeMu.Unlock()
		time.Sleep(holdApplyDelay)
		n := applyhold.Release()
		// 全部生效完成：此前到达的确认请求可能经由还没切换的旧路径，不能据此确认（见 confirmPending）
		c.mu.Lock()
		c.effective = time.Now()
		c.mu.Unlock()
		if res.Confirm {
			a.sysLogs.Add(logbuf.LevelInfo, "system", fmt.Sprintf("批量修改已全部生效（%d 项），%d 秒内未在管理界面确认访问正常将整批回滚", n, res.Timeout))
		} else {
			a.sysLogs.Add(logbuf.LevelInfo, "system", fmt.Sprintf("批量修改已全部生效（%d 项）", n))
		}
	}()
}

// handleHoldDiscard 放弃批量修改：恢复开始批量修改前的配置并重启（内存中已是新配置，重启后按恢复的配置加载）。
func (a *App) handleHoldDiscard(w http.ResponseWriter, r *http.Request) {
	c := &a.confirm
	c.writeMu.Lock()
	if !applyhold.Active() {
		c.writeMu.Unlock()
		httpx.Error(w, http.StatusConflict, "没有进行中的批量修改")
		return
	}
	c.mu.Lock()
	p := c.tx
	if p == nil || len(p.Files) == 0 {
		// 还没有任何改动：直接结束
		_ = removePendingConfirm(a.store.Dir())
		c.tx = nil
		c.mu.Unlock()
		applyhold.Drop()
		c.writeMu.Unlock()
		a.sysLogs.Add(logbuf.LevelInfo, "system", "已结束批量修改，没有需要恢复的修改")
		httpx.OK(w, map[string]bool{"restart": false})
		return
	}
	cp := *p
	cp.Rollback = true
	// 批量修改期间没有截止时间：补上，启动时据此识别为手动回滚（截止时间之前标记的回滚）
	if cp.Timeout <= 0 {
		cp.Timeout = defaultConfirmTimeout
	}
	cp.Deadline = time.Now().Add(time.Duration(cp.Timeout) * time.Second)
	if err := savePendingConfirm(a.store.Dir(), &cp); err != nil {
		c.mu.Unlock()
		c.writeMu.Unlock()
		httpx.Error(w, http.StatusInternalServerError, "保存待确认状态失败: %v", err)
		return
	}
	c.tx = &cp
	c.mu.Unlock()
	// 不再生效暂存的修改；重启后启动流程恢复改动前的内容
	applyhold.Drop()
	c.writeMu.Unlock()
	a.sysLogs.Add(logbuf.LevelWarn, "system", fmt.Sprintf("已放弃批量修改（%s），正在重启并恢复修改前的配置", a.confirmTitles(cp.names())))
	httpx.OK(w, map[string]bool{"restart": true})
	a.scheduleRestart()
}

// holdProblem 是批量修改全部生效后会出问题的一项：监听地址在本机不可用，生效后会启动失败。
type holdProblem struct {
	Module string `json:"module"`
	Key    string `json:"key"`
	Name   string `json:"name,omitempty"`
	Addr   string `json:"addr"`
	Error  string `json:"error"`
}

// handleHoldCheck 在全部生效前检查暂存的规则与监听：启用中且监听地址在本机不可用的列出来（不改动任何配置）。
func (a *App) handleHoldCheck(w http.ResponseWriter, r *http.Request) {
	problems := []holdProblem{}
	cache := map[string]map[string]json.RawMessage{}
	seen := map[string]bool{}
	for _, it := range applyhold.Pending() {
		mod := it[0]
		key, _, _ := strings.Cut(it[1], "#")
		if seen[mod+"/"+key] {
			continue
		}
		seen[mod+"/"+key] = true
		if cache[mod] == nil {
			cache[mod] = a.loadPathConfig(mod)
		}
		raw, ok := cache[mod][key]
		if !ok {
			continue
		}
		var item map[string]any
		if json.Unmarshal(raw, &item) != nil || item["enable"] != true {
			continue
		}
		for _, f := range listenFields {
			ip, _ := item[f].(string)
			if ip == "" {
				continue
			}
			if err := listenProblem(ip); err != nil {
				name, _ := item["name"].(string)
				problems = append(problems, holdProblem{Module: mod, Key: key, Name: name, Addr: ip, Error: err.Error()})
			}
		}
	}
	httpx.OK(w, map[string]any{"problems": problems})
}
