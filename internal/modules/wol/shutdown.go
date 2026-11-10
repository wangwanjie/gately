package wol

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
	"golang.org/x/crypto/ssh"
)

// shutdownTimeout 为一次远程关机（建立 SSH 连接并执行命令）的最长时间，测试可调小。
var shutdownTimeout = 30 * time.Second

// errShutdownBusy 表示同一设备的关机命令还在执行。
var errShutdownBusy = errors.New("该设备的关机命令正在执行，请稍候")

// errNoShutdown 表示设备没有配置远程关机。
var errNoShutdown = errors.New("该设备未配置远程关机")

// shutdown 经设备配置的 SSH 连接执行关机命令。同一设备同时只执行一次；
// 关机命令常使 SSH 连接在返回退出码前断开，这种情况视为已开始关机。
func (m *Module) shutdown(ctx context.Context, d Device) (string, error) {
	if d.ShutdownConn == "" {
		return "", errNoShutdown
	}
	if m.deps.SSHExec == nil {
		return "", errors.New("Web 终端模块未包含在此构建中，无法远程关机")
	}
	m.mu.Lock()
	if m.busy[d.Key] {
		m.mu.Unlock()
		return "", errShutdownBusy
	}
	m.busy[d.Key] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.busy, d.Key)
		m.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()
	log := m.logs.Logger(d.Name)
	out, err := m.deps.SSHExec(ctx, d.ShutdownConn, d.ShutdownCommand)
	out = strings.TrimSpace(out)
	var missing *ssh.ExitMissingError
	if errors.As(err, &missing) {
		log.Infof("已发送关机命令，连接在命令返回前断开，设备应已开始关机")
		m.bemfa.publishState(d.BemfaTopic, "off")
		return out, nil
	}
	if err != nil {
		if out != "" {
			log.Errorf("关机失败: %v，输出: %s", err, out)
		} else {
			log.Errorf("关机失败: %v", err)
		}
		return out, err
	}
	log.Infof("已执行关机命令")
	m.bemfa.publishState(d.BemfaTopic, "off")
	return out, nil
}

// shutdownResult 为远程关机接口的返回：命令输出（最多 16 KB，去掉首尾空白）。
type shutdownResult struct {
	Output string `json:"output"`
}

func (m *Module) handleShutdown(w http.ResponseWriter, r *http.Request) {
	d, ok := m.find(r.PathValue("key"))
	if !ok {
		httpx.Error(w, http.StatusNotFound, "设备不存在")
		return
	}
	// 请求断开后关机仍执行完，避免命令执行到一半被取消
	out, err := m.shutdown(context.WithoutCancel(r.Context()), d)
	switch {
	case errors.Is(err, errNoShutdown), errors.Is(err, errShutdownBusy):
		httpx.Error(w, http.StatusBadRequest, "%v", err)
	case err != nil:
		httpx.Error(w, http.StatusBadGateway, "关机失败: %v", err)
	default:
		httpx.OK(w, shutdownResult{Output: out})
	}
}

// handleSSHTargets 列出 Web 终端中的 SSH 连接，供设备选择关机所用的连接；Web 终端未编译时返回空列表。
func (m *Module) handleSSHTargets(w http.ResponseWriter, r *http.Request) {
	out := []SSHTarget{}
	if m.deps.SSHTargets != nil {
		out = m.deps.SSHTargets()
	}
	httpx.OK(w, out)
}
