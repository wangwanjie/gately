package wol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
	"golang.org/x/crypto/ssh"
)

// shutdownTimeout 为一次远程关机（建立 SSH 连接并执行命令、发送 HTTP 请求）的最长时间，测试可调小。
var shutdownTimeout = 30 * time.Second

// errShutdownBusy 表示同一设备的关机命令还在执行。
var errShutdownBusy = errors.New("该设备的关机命令正在执行，请稍候")

// errNoShutdown 表示设备没有配置远程关机。
var errNoShutdown = errors.New("该设备未配置远程关机")

// 电源操作：关机三种方式都支持；重启与睡眠只有客户端方式支持（其他方式只配置了一条关机命令或请求）
const (
	ActionShutdown = "shutdown"
	ActionReboot   = "reboot"
	ActionSleep    = "sleep"
)

// validAction 判断电源操作是否有效。
func validAction(a string) bool {
	return a == ActionShutdown || a == ActionReboot || a == ActionSleep
}

// actionText 是一种电源操作的各条文案。每条都是整句，便于按句翻译（拼接的片段无法翻译）。
type actionText struct {
	// failed 为执行失败的错误前缀，unsupported 为非客户端方式不支持该操作的提示
	failed, unsupported string
	// acked 为客户端确认指令的日志，agentFailed 为客户端报告执行失败的日志
	acked, agentFailed string
}

var actionTexts = map[string]actionText{
	ActionShutdown: {failed: "关机失败: %v", acked: "客户端已确认关机指令", agentFailed: "客户端关机失败: %s"},
	ActionReboot:   {failed: "重启失败: %v", unsupported: "只有客户端方式支持重启", acked: "客户端已确认重启指令", agentFailed: "客户端重启失败: %s"},
	ActionSleep:    {failed: "睡眠失败: %v", unsupported: "只有客户端方式支持睡眠", acked: "客户端已确认睡眠指令", agentFailed: "客户端睡眠失败: %s"},
}

// shutdown 按设备配置的方式关机，管理界面、接口与巴法云共用。
func (m *Module) shutdown(ctx context.Context, d Device) (string, error) {
	return m.power(ctx, d, ActionShutdown)
}

// power 按设备配置的方式执行电源操作。同一设备同时只执行一次；成功关机、睡眠后把巴法云开关状态写为关。
func (m *Module) power(ctx context.Context, d Device, action string) (string, error) {
	if d.ShutdownMode == "" {
		return "", errNoShutdown
	}
	if !validAction(action) {
		return "", fmt.Errorf("不支持的操作 %q", action)
	}
	if action != ActionShutdown && d.ShutdownMode != ModeAgent {
		return "", errors.New(actionTexts[action].unsupported)
	}
	if d.ShutdownMode == ModeSSH && m.deps.SSHExec == nil {
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
	var out string
	var err error
	switch d.ShutdownMode {
	case ModeSSH:
		out, err = m.sshShutdown(ctx, d)
	case ModeAgent:
		err = m.agentPower(ctx, d, action)
	case ModeHTTP:
		out, err = m.httpShutdown(ctx, d)
	default:
		return "", errNoShutdown
	}
	if err == nil && action != ActionReboot {
		m.bemfa.publishState(d.BemfaTopic, "off")
	}
	return out, err
}

// sshShutdown 经设备配置的 SSH 连接执行关机命令。
// 关机命令常使 SSH 连接在返回退出码前断开，这种情况视为已开始关机。
func (m *Module) sshShutdown(ctx context.Context, d Device) (string, error) {
	log := m.logs.Logger(d.Name)
	out, err := m.deps.SSHExec(ctx, d.ShutdownConn, d.ShutdownCommand)
	out = strings.TrimSpace(out)
	var missing *ssh.ExitMissingError
	if errors.As(err, &missing) {
		log.Infof("已发送关机命令，连接在命令返回前断开，设备应已开始关机")
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
	return out, nil
}

// shutdownResult 为远程关机接口的返回：命令输出或 HTTP 响应内容（最多 16 KB，去掉首尾空白），客户端方式为空。
type shutdownResult struct {
	Output string `json:"output"`
}

// powerRequest 为远程关机接口的请求体，可省略（默认关机）。
type powerRequest struct {
	// Action 为 shutdown（关机）、reboot（重启）或 sleep（睡眠）；重启与睡眠只有客户端方式支持
	Action string `json:"action"`
}

func (m *Module) handleShutdown(w http.ResponseWriter, r *http.Request) {
	d, ok := m.find(r.PathValue("key"))
	if !ok {
		httpx.Error(w, http.StatusNotFound, "设备不存在")
		return
	}
	// 请求体可省略（旧版管理界面与脚本不带请求体），省略或为 null 时关机
	var req powerRequest
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "请求体过大")
		return
	}
	if body := bytes.TrimSpace(raw); len(body) > 0 && !bytes.Equal(body, []byte("null")) {
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			httpx.Error(w, http.StatusBadRequest, "请求体格式错误: %v", err)
			return
		}
	}
	if req.Action == "" {
		req.Action = ActionShutdown
	}
	if !validAction(req.Action) {
		httpx.Error(w, http.StatusBadRequest, "不支持的操作 %q", req.Action)
		return
	}
	if d.ShutdownMode == "" {
		httpx.Error(w, http.StatusBadRequest, "%v", errNoShutdown)
		return
	}
	if req.Action != ActionShutdown && d.ShutdownMode != ModeAgent {
		httpx.Error(w, http.StatusBadRequest, "%s", actionTexts[req.Action].unsupported)
		return
	}
	// 请求断开后关机仍执行完，避免命令执行到一半被取消
	out, err := m.power(context.WithoutCancel(r.Context()), d, req.Action)
	switch {
	case errors.Is(err, errNoShutdown), errors.Is(err, errShutdownBusy):
		httpx.Error(w, http.StatusBadRequest, "%v", err)
	case errors.Is(err, errAgentOffline):
		httpx.Error(w, http.StatusConflict, "%v", err)
	case err != nil:
		httpx.Error(w, http.StatusBadGateway, actionTexts[req.Action].failed, err)
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
