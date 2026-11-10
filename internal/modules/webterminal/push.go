package webterminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/pkg/sftp"
)

// PushFile 是要经 SSH 上传到远程主机的文件。
type PushFile struct {
	Name string
	Data []byte
	Mode os.FileMode
}

// SSHTarget 是可供其他模块选择的 SSH 连接。
type SSHTarget struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Host string `json:"host"`
	// Trusted 表示已记录主机密钥，可用于无人值守的推送
	Trusted bool `json:"trusted"`
}

// SSHTargets 返回全部 SSH 连接，供证书部署等功能选择目标主机。
func (m *Module) SSHTargets() []SSHTarget {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []SSHTarget{}
	for _, c := range m.cfg.Connections {
		if c.Type != TypeSSH {
			continue
		}
		host := c.SSH.Host
		if c.SSH.Port != 0 && c.SSH.Port != 22 {
			host = fmt.Sprintf("%s:%d", host, c.SSH.Port)
		}
		if c.SSH.Username != "" {
			host = c.SSH.Username + "@" + host
		}
		out = append(out, SSHTarget{Key: c.Key, Name: c.Name, Host: host, Trusted: c.SSH.HostKey != ""})
	}
	return out
}

// Push 经指定 SSH 连接把文件写到远程目录（不存在时创建），全部写入后执行 command（可为空），返回命令输出。
// 无人值守时不能确认主机身份，因此要求该连接已记录主机密钥（在 Web 终端中连接过一次）。
func (m *Module) Push(ctx context.Context, connKey, dir string, files []PushFile, command string) (string, error) {
	conn, ok := m.findConn(connKey)
	if !ok || conn.Type != TypeSSH {
		return "", errors.New("SSH 连接不存在，可能已被删除")
	}
	if conn.SSH.HostKey == "" {
		return "", fmt.Errorf("SSH 连接「%s」尚未记录主机密钥，请先在 Web 终端中连接一次以确认主机身份", conn.Name)
	}
	client, release, err := m.acquireSSH(ctx, connKey)
	if err != nil {
		return "", fmt.Errorf("连接 SSH 失败: %w", err)
	}
	defer release()
	if len(files) > 0 {
		sc, err := sftp.NewClient(client)
		if err != nil {
			return "", fmt.Errorf("打开 SFTP 子系统失败: %w", err)
		}
		defer sc.Close()
		if err := sc.MkdirAll(dir); err != nil {
			return "", fmt.Errorf("创建远程目录 %s 失败: %w", dir, err)
		}
		for _, f := range files {
			if err := pushFile(sc, path.Join(dir, f.Name), f.Data, f.Mode); err != nil {
				return "", err
			}
		}
	}
	if strings.TrimSpace(command) == "" {
		return "", nil
	}
	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("打开 SSH 会话失败: %w", err)
	}
	defer sess.Close()
	// 命令执行受 ctx 约束，超时或取消时关闭会话让 CombinedOutput 返回
	stop := context.AfterFunc(ctx, func() { _ = sess.Close() })
	defer stop()
	out, err := sess.CombinedOutput(command)
	if ctx.Err() != nil {
		return string(out), fmt.Errorf("远程命令执行超时: %w", ctx.Err())
	}
	if err != nil {
		return string(out), fmt.Errorf("远程命令执行失败: %w", err)
	}
	return string(out), nil
}

// pushFile 先写临时文件并设好权限，再改名为目标文件，避免远程服务读到写了一半的证书或权限过宽的私钥。
func pushFile(sc *sftp.Client, target string, data []byte, mode os.FileMode) error {
	tmp := path.Join(path.Dir(target), ".gately-"+path.Base(target)+".tmp")
	f, err := sc.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("写入 %s 失败: %w", target, err)
	}
	// 先收紧权限再写内容，私钥不会以默认权限短暂存在
	if err := f.Chmod(mode); err != nil {
		f.Close()
		_ = sc.Remove(tmp)
		return fmt.Errorf("设置 %s 权限失败: %w", target, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = sc.Remove(tmp)
		return fmt.Errorf("写入 %s 失败: %w", target, err)
	}
	if err := f.Close(); err != nil {
		_ = sc.Remove(tmp)
		return fmt.Errorf("写入 %s 失败: %w", target, err)
	}
	// 优先用 posix-rename 原子替换；服务器不支持该扩展时退回先删除再改名
	if err := sc.PosixRename(tmp, target); err != nil {
		_ = sc.Remove(target)
		if err := sc.Rename(tmp, target); err != nil {
			_ = sc.Remove(tmp)
			return fmt.Errorf("替换 %s 失败: %w", target, err)
		}
	}
	return nil
}

// execOutputLimit 为 Exec 保留的命令输出上限，超出部分丢弃，避免远程程序大量输出占用内存。
const execOutputLimit = 16 << 10

// limitedBuffer 只保留前 max 个字节的输出，写入始终报告成功，命令不会因输出被截断而中断。
// ssh 库为 stdout、stderr 各起一个 goroutine 复制数据，两路共用同一个缓冲时会并发写入，因此需要加锁。
type limitedBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.max - len(b.buf); room > 0 {
		if len(p) > room {
			b.buf = append(b.buf, p[:room]...)
		} else {
			b.buf = append(b.buf, p...)
		}
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

// Exec 经指定 SSH 连接执行一条命令并返回输出（最多保留 16 KB），供网络唤醒关机等功能使用。
// 每次新建连接、执行完关闭，不经连接池：目标设备可能刚重启过，池中的旧连接已失效但还没被发现，
// 用它打开会话会一直等到超时。无人值守时不能确认主机身份，因此要求该连接已记录主机密钥。
// 命令执行失败时返回的错误包装了 ssh 库的原始错误（如 *ssh.ExitError、*ssh.ExitMissingError），可用 errors.As 判断。
func (m *Module) Exec(ctx context.Context, connKey, command string) (string, error) {
	conn, ok := m.findConn(connKey)
	if !ok || conn.Type != TypeSSH {
		return "", errors.New("SSH 连接不存在，可能已被删除")
	}
	if conn.SSH.HostKey == "" {
		return "", fmt.Errorf("SSH 连接「%s」尚未记录主机密钥，请先在 Web 终端中连接一次以确认主机身份", conn.Name)
	}
	if strings.TrimSpace(command) == "" {
		return "", errors.New("命令不能为空")
	}
	client, hk, err := m.dialSSH(ctx, &conn)
	if err != nil {
		return "", fmt.Errorf("连接 SSH 失败: %w", err)
	}
	defer client.Close()
	m.recordHostKey(connKey, hk)
	// 超时或取消时关闭整个连接，NewSession 与命令执行都会随之返回
	stop := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stop()
	sess, err := client.NewSession()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("远程命令执行超时: %w", ctx.Err())
		}
		return "", fmt.Errorf("打开 SSH 会话失败: %w", err)
	}
	defer sess.Close()
	out := &limitedBuffer{max: execOutputLimit}
	sess.Stdout, sess.Stderr = out, out
	err = sess.Run(command)
	text := out.String()
	if ctx.Err() != nil {
		return text, fmt.Errorf("远程命令执行超时: %w", ctx.Err())
	}
	if err != nil {
		return text, fmt.Errorf("远程命令执行失败: %w", err)
	}
	return text, nil
}
