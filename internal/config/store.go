// Package config 提供按模块划分的明文 JSON 配置存储。
//
// 每个模块对应目录下的一个 <name>.json 文件。写入采用"临时文件 + rename"保证原子性，
// 文件权限为 0600，目录权限为 0700。配置保持明文，便于用户审计、备份和纳入 git。
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/wangwanjie/gately/internal/fsperm"
)

// 模块名只允许小写字母、数字、下划线和短横线，防止路径穿越。
var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Store 是配置目录的句柄，可被多个模块并发使用。
type Store struct {
	dir string
	mu  sync.Mutex
	// history 非空时，覆盖配置前保存旧版本
	history *historyConfig
}

// Open 打开（必要时创建）配置目录，并把它登记为受保护目录：目录里有会话签名密钥、令牌与私钥，
// 存储挂载、静态站点与文件服务据此拒绝把它（或包含它的目录）提供出去，见 fsperm.Protect。
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}
	fsperm.Protect(dir)
	return &Store{dir: dir}, nil
}

// Dir 返回配置目录路径。
func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) path(name string) (string, error) {
	if !validName.MatchString(name) {
		return "", fmt.Errorf("非法的配置名: %q", name)
	}
	return filepath.Join(s.dir, name+".json"), nil
}

// Load 读取配置到 v。文件不存在时返回 (false, nil)，v 保持调用方设置的默认值。
func (s *Store) Load(name string, v any) (bool, error) {
	p, err := s.path(name)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return true, fmt.Errorf("解析配置 %s 失败: %w", p, err)
	}
	return true, nil
}

// Save 将 v 以缩进 JSON 原子写入。
func (s *Store) Save(name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return s.WriteRaw(name, append(data, '\n'))
}

// WriteRaw 原子写入已序列化的配置内容（用于从备份恢复）。
func (s *Store) WriteRaw(name string, data []byte) error {
	p, err := s.path(name)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tmp, err := os.CreateTemp(s.dir, "."+name+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// 任何失败路径都清理临时文件
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	// 先落盘再 rename，避免断电后出现空文件
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// 新内容已安全落盘，覆盖前保存旧版本
	s.snapshotLocked(name, p, data)
	return os.Rename(tmpName, p)
}
