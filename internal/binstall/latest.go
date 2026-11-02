package binstall

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// latestTimeout 是一次“检查更新”查询上游最新版本的时限
const latestTimeout = 45 * time.Second

// Latest 是最近一次查询到的上游最新版本。
type Latest struct {
	// Version 为最新正式版的版本号，查询失败时保留上一次成功的结果
	Version   string    `json:"version,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
	// Error 为最近一次查询的错误，成功时为空
	Error string `json:"error,omitempty"`
	// HasUpdate 表示最新版本比 bin 下安装的版本新，由 View 按当前安装的版本计算
	HasUpdate bool `json:"hasUpdate"`
}

// ErrChecking 表示已有检查更新在进行。
var ErrChecking = errors.New("正在检查更新，请稍候")

// CheckLatest 查询上游最新正式版并缓存结果，供界面显示与定时检查使用；不下载、不改动已安装的程序。
// 使用该程序单独设置的下载代理，未设置时沿用全局“更新代理”。
func (in *Installer) CheckLatest(ctx context.Context) (Latest, error) {
	if in.spec.Check != nil {
		if err := in.spec.Check(in.plat); err != nil {
			return Latest{}, err
		}
	}
	// 同一时间只查询一次：定时检查与界面点击同时发生时，后到的一方直接返回
	if !in.checkMu.TryLock() {
		return in.LatestInfo(), ErrChecking
	}
	defer in.checkMu.Unlock()
	proxy := ""
	if in.opt.GetProxy != nil {
		proxy = in.opt.GetProxy()
	}
	client, err := in.client(proxy)
	if err != nil {
		return in.LatestInfo(), err
	}
	ctx, cancel := context.WithTimeout(ctx, latestTimeout)
	defer cancel()
	rel, err := in.spec.Resolve(ctx, client, in.plat, "")
	in.mu.Lock()
	in.latest.CheckedAt = time.Now()
	if err != nil {
		in.latest.Error = err.Error()
	} else {
		in.latest.Version, in.latest.Error = rel.Version, ""
	}
	in.mu.Unlock()
	return in.LatestInfo(), err
}

// LatestInfo 返回缓存的最新版本，并按 bin 下当前安装的版本计算是否有更新；从未检查过时 CheckedAt 为零值。
func (in *Installer) LatestInfo() Latest {
	in.mu.Lock()
	l := in.latest
	in.mu.Unlock()
	if l.Version != "" {
		if cur := in.Installed(); cur.Present {
			l.HasUpdate = VersionNewer(l.Version, cur.Version)
		}
	}
	return l
}

// Name 返回程序名，供定时检查生成通知。
func (in *Installer) Name() string { return in.spec.Name }

// Key 返回程序标识（旧版本目录名），定时检查以它区分不同程序。
func (in *Installer) Key() string { return in.spec.Key }

// versionNumsRe 取版本号开头的数字段，例如 v0.61.0、1.90.2、2025.10.0、v1.1.0-stable 中的数字部分
var versionNumsRe = regexp.MustCompile(`^v?(\d+(?:\.\d+)*)`)

// VersionNewer 判断 latest 是否比 installed 新：只比较开头的数字段（缺少的段按 0 处理），
// 数字相同即视为不更新，避免 -stable、-beta 之类的后缀在上游与探测结果之间写法不一致时误报；
// 任一版本号无法解析时返回 false。
func VersionNewer(latest, installed string) bool {
	a, okA := versionNums(latest)
	b, okB := versionNums(installed)
	if !okA || !okB {
		return false
	}
	for i := 0; i < max(len(a), len(b)); i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func versionNums(s string) ([]int, bool) {
	m := versionNumsRe.FindStringSubmatch(strings.ToLower(strings.TrimSpace(s)))
	if m == nil {
		return nil, false
	}
	parts := strings.Split(m[1], ".")
	// 防御异常输入：正常的版本号不会超过 8 段
	if len(parts) > 8 {
		return nil, false
	}
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false
		}
		nums[i] = n
	}
	return nums, true
}
