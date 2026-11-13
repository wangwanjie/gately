package app

import (
	"fmt"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/loginips"
)

// loginIPsName 是已知登录来源在配置目录中的文件名（运行状态，不记录配置历史）
const loginIPsName = "loginips"

type loginIPsFile struct {
	IPs []loginips.Entry `json:"ips"`
}

// initLoginIPs 载入已知登录来源。文件损坏时从空表开始：下一次登录重新建立基线，不会阻止启动。
func (a *App) initLoginIPs() {
	var f loginIPsFile
	if _, err := a.store.Load(loginIPsName, &f); err != nil {
		a.logger.Printf("读取已知登录来源失败，将重新建立: %v", err)
		f = loginIPsFile{}
	}
	a.loginIPs = loginips.New(f.IPs, func(list []loginips.Entry) error {
		return a.store.Save(loginIPsName, loginIPsFile{IPs: list})
	})
}

// noteLoginIP 在管理后台登录成功后调用：来源地址从未登录过时写日志并发布“新地址登录”事件。
// 通知规则决定是否发送，不配置规则则只留在系统日志里。
func (a *App) noteLoginIP(ip, user, how string) {
	if a.loginIPs == nil || !a.loginIPs.Observe(ip, time.Now()) {
		return
	}
	msg := fmt.Sprintf("%s 通过%s从新地址 %s 登录管理后台，该地址此前没有登录过", user, how, ip)
	a.sysLogs.Add(logbuf.LevelWarn, "auth", msg)
	events.Publish(events.Event{Type: events.LoginNewIP, Source: "管理后台", Level: events.LevelWarn,
		Title: "管理后台新地址登录", Message: msg, Fields: map[string]string{"ip": ip, "user": user}})
}
