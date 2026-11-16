// Gately 修改：按用户解析共享（见 parent/gately.go 与 config.UserConfig.Shares）；ShareConfig.Guard 屏蔽共享目录中的一部分。

package config

import (
	"io/fs"
	"time"
)

// Config is the fully merged, validated runtime configuration.
// All fields are populated; no nil pointers, no zero-value-as-unset.
type Config struct {
	Server ServerConfig
	Log    LogConfig
	Shares []ShareConfig
	Users  []UserConfig
}

type ServerConfig struct {
	Listen string

	// Netbios is accepted by the parser but NOT implemented: no NBSS listener
	// exists, so :139 is never bound. Validate rejects it rather than let the
	// flag imply legacy clients are served. Delete this field, the flag and
	// the Validate check together if NetBIOS is ever really wired up.
	Netbios bool

	MDNS           bool
	Encryption     EncryptionMode
	Signing        SigningMode
	DurableTimeout time.Duration

	// StateDir is parsed, merged and defaulted, but nothing reads it yet —
	// durable handles live in an in-memory table (parent.DurableTable), not on
	// disk. Reserved for when that state is persisted; setting it today has no
	// effect. Unlike Netbios it is harmless, so it is accepted rather than
	// rejected: a stale state_dir in an existing config must not break startup.
	StateDir string

	// PerUserPrivdrop enables the per-connection re-exec worker model: when set
	// (and the process is root), each accepted connection is served by a child
	// process that drops to the authenticated user's uid/gid. Default false —
	// connections are served in-process exactly as before.
	PerUserPrivdrop bool
}

type EncryptionMode string

const (
	EncryptionRequired  EncryptionMode = "required"
	EncryptionPreferred EncryptionMode = "preferred"
	EncryptionOff       EncryptionMode = "off"
)

type SigningMode string

const (
	SigningRequired  SigningMode = "required"
	SigningPreferred SigningMode = "preferred"
)

type LogConfig struct {
	Level  string
	Format string
}

type ShareConfig struct {
	Name     string
	Path     string
	ReadOnly bool
	GuestOK  bool
	// Guard 非 nil 时屏蔽共享目录中的一部分（Gately 补丁：共享目录包含 Gately 的配置目录时隐藏配置目录），
	// 只由调用方在代码中设置，不出现在配置文件里
	Guard PathGuard
}

// PathGuard 决定共享目录中哪些路径对客户端不可见（Gately 补丁）。参数均为本机绝对路径。
type PathGuard interface {
	// Hidden 判断路径是否被屏蔽：打开、创建、作为改名目标一律按不存在处理
	Hidden(path string) bool
	// Touches 判断改名或删除该路径是否会动到被屏蔽的部分（路径在其中，或是它的上级目录）
	Touches(path string) bool
	// HiddenChild 判断目录 dir 中由 fi（条目自身的 Lstat 信息）描述的条目是否从列表中隐藏
	HiddenChild(dir string, fi fs.FileInfo) bool
}

type UserConfig struct {
	Name        string
	NTHash      [16]byte
	SystemUser  string
	SystemUID   int
	SystemGID   int
	AllowShares []string
	// Shares 非 nil 时为该用户专属的共享列表，取代全局 Shares（Gately 补丁：各用户的同名共享可映射到不同目录）
	Shares []ShareConfig
}

func Defaults() Config {
	return Config{
		Server: ServerConfig{
			Listen:         ":445",
			Netbios:        false,
			MDNS:           true,
			Encryption:     EncryptionRequired,
			Signing:        SigningRequired,
			DurableTimeout: 60 * time.Second,
			StateDir:       "/var/run/gosamba",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
	}
}
