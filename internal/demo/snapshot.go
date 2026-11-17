package demo

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// 快照目录与重置间隔可用环境变量调整。
const (
	envSnapshot = "GATELY_DEMO_SNAPSHOT"
	envInterval = "GATELY_DEMO_RESET_INTERVAL"
)

// DefaultInterval 是演示数据的重置间隔。
const DefaultInterval = time.Hour

// Interval 返回重置间隔，GATELY_DEMO_RESET_INTERVAL 可用 Go 时长写法覆盖（例如 90s，仅测试使用）。
func Interval() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(envInterval)); err == nil && d >= time.Second {
		return d
	}
	return DefaultInterval
}

// SnapshotDir 返回配置目录对应的快照目录：默认放在系统临时目录（容器里通常是 tmpfs），按配置目录区分。
func SnapshotDir(configDir string) string {
	if d := os.Getenv(envSnapshot); d != "" {
		return d
	}
	sum := sha256.Sum256([]byte(configDir))
	return filepath.Join(os.TempDir(), "gately-demo-snapshot-"+hex.EncodeToString(sum[:6]))
}

// markerFile 标记快照已完整写入：写到一半被中断的快照不会被当成基线使用。
const markerFile = ".snapshot-complete"

// HasSnapshot 返回快照是否存在且完整。
func HasSnapshot(snapDir string) bool {
	_, err := os.Stat(filepath.Join(snapDir, markerFile))
	return err == nil
}

// Snapshot 把配置目录保存为快照，已有快照会先被替换。
func Snapshot(configDir, snapDir string) error {
	if err := os.RemoveAll(snapDir); err != nil {
		return err
	}
	if err := os.MkdirAll(snapDir, 0o700); err != nil {
		return err
	}
	if err := copyTree(configDir, snapDir, nil); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(snapDir, markerFile), nil, 0o600)
}

// Restore 把配置目录恢复为快照内容：先清空配置目录里的全部内容（保留目录本身，它可能是挂载点），再复制快照。
func Restore(configDir, snapDir string) error {
	if !HasSnapshot(snapDir) {
		return errors.New("演示快照不存在")
	}
	entries, err := os.ReadDir(configDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(configDir, e.Name())); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}
	return copyTree(snapDir, configDir, func(rel string) bool { return rel == markerFile })
}

// copyTree 递归复制普通文件与目录，保留权限位；符号链接、套接字等特殊文件跳过。skip 返回 true 的相对路径不复制。
func copyTree(src, dst string, skip func(rel string) bool) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if skip != nil && skip(rel) {
			return nil
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case info.Mode().IsRegular():
			return copyFile(p, target, info.Mode().Perm())
		}
		return nil
	})
}

func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("复制 %s 失败: %w", src, err)
	}
	return out.Close()
}
