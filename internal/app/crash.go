package app

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/wangwanjie/gately/internal/crashlog"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/version"
)

// startCrashLog 开启崩溃记录，并把上次运行的异常结束写进系统日志。系统日志此时已开启落盘，
// 这条提示重启后依然可见，诊断包里同时带上 crash.log。
func (a *App) startCrashLog() {
	header := fmt.Sprintf("gately %s (%s, built %s) pid %d started %s",
		version.Version, version.Commit, version.BuildDate, os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	rec, prev, err := crashlog.Start(a.opts.ConfigDir, header)
	if errors.Is(err, crashlog.ErrLocked) {
		// 另一个实例正在使用这个配置目录（例如已注册为服务，又手动启动了一次），它的运行标记不能动
		a.sysLogs.Add(logbuf.LevelWarn, "", "同一配置目录已有另一个 Gately 实例在运行，本次不开启崩溃记录")
	} else if err != nil {
		// 记录失败不影响启动
		a.sysLogs.Add(logbuf.LevelWarn, "", "开启崩溃记录失败: "+err.Error())
	}
	a.crash = rec
	at := prev.Time.Local().Format("2006-01-02 15:04:05")
	switch prev.Kind {
	case crashlog.Crashed:
		a.sysLogs.Add(logbuf.LevelError, "", fmt.Sprintf("上次运行在 %s 崩溃退出：%s。完整的崩溃信息已保存到配置目录的 crash.log，导出诊断包时会一并包含", at, prev.Reason))
	case crashlog.Killed:
		a.sysLogs.Add(logbuf.LevelWarn, "", fmt.Sprintf("上次运行意外结束，最后一次存活记录在 %s（之后一分钟内结束），进程没有留下崩溃信息。常见原因是内存不足被系统强制结束（OOM）、被 kill -9 / docker kill 结束或断电", at))
	}
}
