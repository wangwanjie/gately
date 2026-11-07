//go:build windows

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const supportsUserMode = false

// DefaultConfigDir 返回服务模式下默认的配置目录。
func DefaultConfigDir(user bool) string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "Gately")
}

// IsWindowsService 判断当前进程是否由服务控制管理器启动。
func IsWindowsService() bool {
	ok, err := svc.IsWindowsService()
	return err == nil && ok
}

// RunAsWindowsService 向服务控制管理器报告状态并运行 run，收到停止或关机请求时取消其 ctx。
func RunAsWindowsService(run func(ctx context.Context)) int {
	if err := svc.Run(Name, &handler{run: run}); err != nil {
		return 1
	}
	return 0
}

type handler struct {
	run func(ctx context.Context)
}

func (h *handler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		h.run(ctx)
		close(done)
	}()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				status <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: 30000}
				cancel()
				// 等待各模块停止，超时后由进程退出强制结束
				select {
				case <-done:
				case <-time.After(25 * time.Second):
				}
				return false, 0
			}
		case <-done:
			// 服务主体自行结束视为异常，让恢复策略重新拉起
			return false, 1
		}
	}
}

func platformRun(action string, sp Spec, out io.Writer) error {
	m, err := mgr.Connect()
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return errors.New("需要管理员权限，请以管理员身份运行终端")
		}
		return err
	}
	defer m.Disconnect()

	switch action {
	case "install":
		if err := os.MkdirAll(sp.ConfigDir, 0o700); err != nil {
			return err
		}
		// 服务以 LocalSystem 运行，配置目录里有账号、令牌、DNS 密钥与证书私钥，且托管程序优先从 <配置目录>\bin 查找；
		// ProgramData 默认允许普通用户读取并新建文件，目录必须收紧为只有 SYSTEM 与管理员可访问
		if err := protectDir(sp.ConfigDir); err != nil {
			return fmt.Errorf("设置配置目录权限失败: %w", err)
		}
		if s, err := m.OpenService(Name); err == nil {
			s.Close()
			return errors.New("服务已存在，请先运行 gately service uninstall 再重新安装")
		}
		s, err := m.CreateService(Name, sp.Exe, mgr.Config{
			DisplayName: "Gately",
			Description: "Gately network gateway toolkit (https://gately.cn)",
			StartType:   mgr.StartAutomatic,
			// 延迟自动启动，等网络就绪后再运行
			DelayedAutoStart: true,
		}, sp.args()...)
		if err != nil {
			return err
		}
		defer s.Close()
		// 异常退出（含管理界面“重启”在 Windows 上以退出码 3 结束进程）后 3 秒重新拉起
		actions := []mgr.RecoveryAction{
			{Type: mgr.ServiceRestart, Delay: 3 * time.Second},
			{Type: mgr.ServiceRestart, Delay: 3 * time.Second},
			{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		}
		if err := s.SetRecoveryActions(actions, 86400); err != nil {
			return err
		}
		if err := s.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
			return err
		}
		if sp.NoStart {
			say(out, "已注册开机自启服务 %s", Name)
			return nil
		}
		if err := s.Start(); err != nil {
			return err
		}
		say(out, "已安装并启动服务 %s，开机自动运行；配置目录 %s", Name, sp.ConfigDir)
		say(out, "默认账号 admin / gately.cn，忘记密码使用 gately reset-password")
		return nil
	case "uninstall":
		s, err := m.OpenService(Name)
		if err != nil {
			return errors.New("服务不存在")
		}
		defer s.Close()
		_ = stopService(s)
		if err := s.Delete(); err != nil {
			return err
		}
		say(out, "已移除服务 %s，配置目录保持不变", Name)
		return nil
	}

	s, err := m.OpenService(Name)
	if err != nil {
		return errors.New("服务尚未安装，请先运行 gately service install")
	}
	defer s.Close()
	switch action {
	case "start":
		return s.Start()
	case "stop":
		return stopService(s)
	case "restart":
		if err := stopService(s); err != nil {
			return err
		}
		return s.Start()
	case "status":
		st, err := s.Query()
		if err != nil {
			return err
		}
		say(out, "状态: %s", stateName(st.State))
		return nil
	}
	return nil
}

// protectedDirSDDL 是配置目录的安全描述符：受保护的 DACL（不继承上级目录的权限），
// 只有 SYSTEM 与 Administrators 完全控制，并由子目录与文件继承
const protectedDirSDDL = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

// protectDir 把目录（含已有内容，经继承）设置为只有 SYSTEM 与管理员可访问，目录已存在时同样重设。
func protectDir(dir string) error {
	sd, err := windows.SecurityDescriptorFromString(protectedDirSDDL)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// stopService 发送停止请求并等待服务进入停止状态；服务本来就没有运行时视为成功。
func stopService(s *mgr.Service) error {
	st, err := s.Control(svc.Stop)
	if errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
		return nil
	}
	if err != nil {
		return err
	}
	deadline := time.Now().Add(30 * time.Second)
	for st.State != svc.Stopped {
		if time.Now().After(deadline) {
			return errors.New("等待服务停止超时")
		}
		time.Sleep(300 * time.Millisecond)
		if st, err = s.Query(); err != nil {
			return err
		}
	}
	return nil
}

func stateName(s svc.State) string {
	switch s {
	case svc.Running:
		return "运行中"
	case svc.Stopped:
		return "已停止"
	case svc.StartPending:
		return "正在启动"
	case svc.StopPending:
		return "正在停止"
	}
	return "其他"
}
