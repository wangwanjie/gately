package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/wangwanjie/gately/internal/fsperm"
)

// ResetFile 是命令行重置请求在配置目录中的文件名。
// 不使用 .json 后缀，避免被配置备份、诊断包和声明式配置当作普通配置处理。
const ResetFile = "account.reset"

// ResetRequest 是在服务器上通过 gately reset-password / reset-totp 提交的账号重置请求。
//
// 命令行只写请求文件，由运行中的实例（未运行时为下次启动）应用到账号，
// 账号文件始终只有服务进程一个写入方，避免与服务内存中的账号互相覆盖。
type ResetRequest struct {
	// PasswordHash 非空时把主账号密码改为该 bcrypt 哈希
	PasswordHash string `json:"passwordHash,omitempty"`
	// DefaultPassword 表示新密码就是默认密码，登录后继续提醒修改
	DefaultPassword bool `json:"defaultPassword,omitempty"`
	// DisableTOTP 为 true 时关闭主账号的两步验证
	DisableTOTP bool      `json:"disableTOTP,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// NewPasswordReset 生成把主账号密码重置为 password 的请求，password 为空时使用默认密码。
func NewPasswordReset(password string) (ResetRequest, error) {
	if password == "" {
		password = DefaultPassword
	}
	// 重置为默认密码是明确的恢复手段，保留；自定义密码须满足强度要求
	if password != DefaultPassword {
		if err := CheckPasswordStrength("", password); err != nil {
			return ResetRequest{}, err
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return ResetRequest{}, err
	}
	return ResetRequest{PasswordHash: string(hash), DefaultPassword: password == DefaultPassword, CreatedAt: time.Now()}, nil
}

// WriteResetRequest 把请求写入配置目录。已有未处理的请求时合并，先后执行的重置密码与关闭两步验证都会生效。
func WriteResetRequest(dir string, req ResetRequest) error {
	p := filepath.Join(dir, ResetFile)
	if prev, err := readResetRequest(p); err == nil {
		if req.PasswordHash == "" {
			req.PasswordHash, req.DefaultPassword = prev.PasswordHash, prev.DefaultPassword
		}
		req.DisableTOTP = req.DisableTOTP || prev.DisableTOTP
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	// 与配置文件一样先写临时文件再 rename，服务端不会读到写了一半的请求
	tmp := p + ".tmp"
	if err := fsperm.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// ResetPending 返回配置目录中是否有尚未应用的重置请求。
func ResetPending(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ResetFile))
	return err == nil
}

// TakeResetRequest 读取并删除配置目录中的重置请求，没有请求时返回 (nil, nil)。
func TakeResetRequest(dir string) (*ResetRequest, error) {
	p := filepath.Join(dir, ResetFile)
	req, err := readResetRequest(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	// 无论能否解析都删除，避免损坏的请求文件每次轮询都报错
	if rmErr := os.Remove(p); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) && err == nil {
		return nil, rmErr
	}
	if err != nil {
		return nil, err
	}
	return req, nil
}

func readResetRequest(p string) (*ResetRequest, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var req ResetRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("解析重置请求失败: %w", err)
	}
	return &req, nil
}

// ApplyReset 把命令行提交的重置请求应用到主账号：修改密码和/或关闭两步验证，
// 同时轮换会话密钥使全部旧会话失效，并清除登录失败锁定，便于立即用新密码登录。
func (m *Manager) ApplyReset(req ResetRequest) error {
	if req.PasswordHash == "" && !req.DisableTOTP {
		return errors.New("重置请求为空")
	}
	if req.PasswordHash != "" {
		// 只接受 bcrypt 哈希，防止写入无法登录的密码
		if _, err := bcrypt.Cost([]byte(req.PasswordHash)); err != nil {
			return errors.New("重置请求中的密码哈希无效")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.acc
	if next.Username == "" {
		next.Username = "admin"
	}
	if req.PasswordHash != "" {
		next.PasswordHash = req.PasswordHash
		next.UsingDefaultPassword = req.DefaultPassword
		next.WeakPassword = false
		// 命令行重置密码同时重新开启密码登录：这是丢失 Passkey 设备后找回访问权限的途径
		next.PasswordLoginOff = false
	}
	if req.DisableTOTP {
		next.TOTPSecret = ""
	}
	next.SessionSecret = randomString(32)
	if err := m.save(next); err != nil {
		return err
	}
	m.acc = next
	m.fails = map[string]*failState{}
	return nil
}
