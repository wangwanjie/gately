package fileusers

import (
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/events"
)

// 文件服务（WebDAV、FTP、SMB）的登录失败发布为 events.FileAuthFailed 事件，入侵防护可按来源计数封禁。

// reportInterval 为同一来源两次失败事件之间的最短间隔：客户端带着错误密码并发多个请求或自动重试时只计一次，
// 持续猜测密码的来源每个间隔计一次，入侵防护按次数封禁也足以在几分钟内生效
const reportInterval = 5 * time.Second

// reportMaxKeys 为记录的来源数量上限，超出时先清理过期项，仍满则不再发布（分布式来源过多时避免事件刷屏）
const reportMaxKeys = 4096

var (
	reportMu   sync.Mutex
	reportSeen = map[netip.Addr]time.Time{}
	// reportNow 为当前时间，测试中可替换
	reportNow = time.Now
)

// ReportAuthFailure 报告一次文件服务登录失败（含因失败过多被直接拒绝）。service 为服务名称（WebDAV、FTP、SMB），
// ip 为来源地址（不能来自客户端可伪造的字段），user 为提交的用户名（可为空，写入事件前会截断）。
// 同一来源每 reportInterval 最多发布一个事件。
func ReportAuthFailure(service, ip, user string) {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return
	}
	a = a.Unmap()
	now := reportNow()
	reportMu.Lock()
	if last, ok := reportSeen[a]; ok && now.Sub(last) < reportInterval {
		reportMu.Unlock()
		return
	}
	if len(reportSeen) >= reportMaxKeys {
		for k, t := range reportSeen {
			if now.Sub(t) >= reportInterval {
				delete(reportSeen, k)
			}
		}
	}
	if len(reportSeen) >= reportMaxKeys {
		reportMu.Unlock()
		return
	}
	reportSeen[a] = now
	reportMu.Unlock()

	msg := fmt.Sprintf("来自 %s 的 %s 登录失败", a, service)
	fields := map[string]string{"ip": a.String(), "service": service}
	if user != "" {
		user = ClipName(user)
		msg = fmt.Sprintf("来自 %s 的 %s 登录失败（用户 %q）", a, service, user)
		fields["user"] = user
	}
	events.Publish(events.Event{
		Type:    events.FileAuthFailed,
		Source:  service,
		Level:   events.LevelWarn,
		Title:   fmt.Sprintf("%s 登录失败：%s", service, a),
		Message: msg,
		Fields:  fields,
	})
}

// ResetFailureReports 清空各来源的发布间隔记录，供其他包的测试在同一来源上验证事件发布。
func ResetFailureReports() {
	reportMu.Lock()
	reportSeen = map[netip.Addr]time.Time{}
	reportMu.Unlock()
}
