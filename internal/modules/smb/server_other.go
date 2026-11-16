//go:build !linux && !darwin

package smb

import (
	"errors"

	"github.com/wangwanjie/gately/internal/fileusers"
	"github.com/wangwanjie/gately/internal/logbuf"
)

// supported 表示当前系统可以运行 SMB 服务。内置的 SMB 实现只支持 Linux 与 macOS。
const supported = false

type server struct{}

func startServer(Settings, []account, *logbuf.Buffer, *fileusers.Throttle) (*server, error) {
	return nil, errors.New("当前系统不支持 SMB 服务，仅支持 Linux 与 macOS")
}

func (s *server) Addr() string { return "" }

func (s *server) Close() {}

// updatePasswords 在不支持的系统上不会被调用（服务不会启动）。
func (s *server) updatePasswords([]account, []string) int { return 0 }

// Status 在不支持的系统上恒为未运行。
func (s *server) Status() (bool, error) { return false, nil }
