package docker

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/module"
)

// summaryTTL 是总览摘要的缓存时长：总览页每 3 秒轮询一次，避免每次都访问 Docker。
const summaryTTL = 10 * time.Second

// summaryTimeout 限制单次探测耗时，远程 Docker 地址不可达时不拖慢总览接口。
const summaryTimeout = 2 * time.Second

// exitedCode 从状态文本（如 "Exited (137) 3 hours ago"）中取出退出码。
var exitedCode = regexp.MustCompile(`^Exited \((-?\d+)\)`)

// summaryCache 缓存最近一次摘要，由 Module.summary 持有。
type summaryCache struct {
	sync.Mutex
	at    time.Time
	items []module.SummaryItem
}

// Summary 实现 module.Summarizer：容器运行 / 停止 / 异常 / 健康检查失败数量与待更新数量；连不上 Docker 时提示未连接。
func (m *Module) Summary() []module.SummaryItem {
	m.summary.Lock()
	defer m.summary.Unlock()
	if m.summary.items != nil && time.Since(m.summary.at) < summaryTTL {
		return m.summary.items
	}
	m.summary.items = m.buildSummary()
	m.summary.at = time.Now()
	return m.summary.items
}

func (m *Module) buildSummary() []module.SummaryItem {
	// 未连接不算故障（很多部署并不使用 Docker），用 info 级别避免拉低整体健康度
	disconnected := []module.SummaryItem{{Label: "状态", Value: "未连接", Level: "info"}}
	c, err := m.client()
	if err != nil {
		return disconnected
	}
	ctx, cancel := context.WithTimeout(context.Background(), summaryTimeout)
	defer cancel()
	list, err := c.Containers(ctx)
	if err != nil {
		return disconnected
	}
	m.mu.Lock()
	updates := make(map[string]UpdateInfo, len(m.updates))
	for k, v := range m.updates {
		updates[k] = v
	}
	m.mu.Unlock()

	running, stopped, bad, unhealthy, upd := 0, 0, 0, 0, 0
	for _, ct := range list {
		if updates[ct.ID].HasUpdate {
			upd++
		}
		switch {
		case ct.State == "restarting" || ct.State == "dead":
			bad++
		case ct.State == "running" || ct.State == "paused":
			// 健康检查失败的容器仍在运行，计入运行中并单独列出，和容器列表的状态保持一致
			running++
			if strings.Contains(ct.Status, "(unhealthy)") {
				unhealthy++
			}
		default:
			// 非 0 退出码视为异常退出，正常停止的容器只计入已停止
			if mt := exitedCode.FindStringSubmatch(ct.Status); mt != nil && mt[1] != "0" {
				bad++
			} else {
				stopped++
			}
		}
	}
	level := "ok"
	if bad > 0 || unhealthy > 0 {
		level = "warn"
	}
	badLevel := "ok"
	if bad > 0 {
		badLevel = "warn"
	}
	items := []module.SummaryItem{
		{Label: "运行中", Value: fmt.Sprintf("%d / %d", running, len(list)), Level: level},
		{Label: "已停止", Value: fmt.Sprint(stopped)},
		{Label: "异常", Value: fmt.Sprint(bad), Level: badLevel},
	}
	if unhealthy > 0 {
		items = append(items, module.SummaryItem{Label: "健康检查失败", Value: fmt.Sprint(unhealthy), Level: "warn"})
	}
	if upd > 0 {
		items = append(items, module.SummaryItem{Label: "有更新", Value: fmt.Sprint(upd), Level: "info"})
	}
	return items
}
