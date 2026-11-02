package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/binstall"
	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
	"github.com/wangwanjie/gately/internal/version"
)

func mustVersion(t *testing.T, s string) parsedVersion {
	t.Helper()
	v, ok := parseVersion(s)
	if !ok {
		t.Fatalf("版本号 %q 应能解析", s)
	}
	return v
}

func TestVersionInRange(t *testing.T) {
	cases := []struct {
		ver, rng string
		in, ok   bool
	}{
		{"0.6.0.4", "<= 0.6.0.4", true, true},
		{"v0.6.0.3", "<= 0.6.0.4", true, true},
		{"0.6.1", "<= 0.6.0.4", false, true},
		{"0.5.9", ">= 0.5.0, < 0.6.1", true, true},
		{"0.6.1", ">= 0.5.0, < 0.6.1", false, true},
		{"0.4.7", ">= 0.5, < 0.6.1", false, true},
		{"0.5.0", ">= 0.5, < 0.6.1", true, true},
		{"0.6.0", "= 0.6.0", true, true},
		{"0.6.0", "0.6.0", true, true},
		{"0.6.0.1", "== 0.6.0", false, true},
		{"0.6.0.5", "> 0.6.0.4", true, true},
		{"0.6.1-rc1", "< 0.6.1", true, true},
		{"0.6.1", "< 0.6.1-rc1", false, true},
		{"0.6.1", "", false, false},
		{"0.6.1", "<= latest", false, false},
		{"0.6.1", ">= 0.5.0, <", false, false},
	}
	for _, c := range cases {
		in, ok := versionInRange(mustVersion(t, c.ver), c.rng)
		if in != c.in || ok != c.ok {
			t.Errorf("%s 在 %q 中: 得到 (%v, %v)，应为 (%v, %v)", c.ver, c.rng, in, ok, c.in, c.ok)
		}
	}
}

func TestAdvisoryAffects(t *testing.T) {
	adv := securityAdvisory{Ranges: []advisoryRange{
		{Vulnerable: ">= 0.3.0, < 0.4.0", Patched: "0.4.0"},
		{Vulnerable: ">= 0.5.0, <= 0.6.0.4", Patched: "0.6.1"},
	}}
	for ver, want := range map[string]bool{"0.3.5": true, "0.4.2": false, "0.6.0.4": true, "0.6.1": false, "0.2.0": false} {
		if got := adv.affects(mustVersion(t, ver)); got != want {
			t.Errorf("%s 受影响应为 %v", ver, want)
		}
	}
	// 范围无法解析时按修复版本判断
	bad := securityAdvisory{Ranges: []advisoryRange{{Vulnerable: "all versions", Patched: "0.6.1"}}}
	if !bad.affects(mustVersion(t, "0.6.0")) || bad.affects(mustVersion(t, "0.6.1")) {
		t.Fatal("范围无法解析时应以修复版本为界")
	}
	// 两者都无法解析时不判定为受影响，避免误报
	if (securityAdvisory{Ranges: []advisoryRange{{Vulnerable: "?", Patched: ""}}}).affects(mustVersion(t, "0.1.0")) {
		t.Fatal("无法判断时不应视为受影响")
	}
}

func TestParseReleases(t *testing.T) {
	list := []ghRelease{
		{TagName: "v0.7.0", Draft: true, Body: "## Security / 安全修复"},
		{TagName: "v0.6.4-rc1", Prerelease: true},
		{TagName: "nightly"},
		{TagName: "v0.6.2.1", HTMLURL: "https://evil.example/x", Body: "## 中文\n### 安全入口改进\n说明"},
		{TagName: "v0.6.3", HTMLURL: repoURL + "/releases/tag/v0.6.3", Body: strings.Repeat("中", maxNotesBytes)},
		{TagName: "v0.6.2", Body: "## New / 新功能\n\n## Security / 安全加固\n- x"},
		{TagName: "v0.6.1", Body: "## Security / 安全修复\n- y"},
		{TagName: "v0.5.5.2", Body: "intro\n## Security fix / 安全修复"},
		{TagName: "v0.5.0", Body: "### 安全修复\n- z"},
		{TagName: "v0.4.0", Body: "提到 security 但不是标题\n#### Securityfix"},
	}
	latest, notes, sec := parseReleases(list)
	if latest == nil || latest.Version != "v0.6.3" || latest.URL != repoURL+"/releases/tag/v0.6.3" {
		t.Fatalf("最新正式版应为 v0.6.3（忽略草稿与预发布）: %+v", latest)
	}
	if len(notes) > maxNotesBytes+len("…") || !strings.HasSuffix(notes, "…") || !strings.HasPrefix(notes, "中") {
		t.Fatalf("更新说明应按字节截断且不切断字符，长度 %d", len(notes))
	}
	var got []string
	for _, r := range sec {
		got = append(got, r.Version)
	}
	if strings.Join(got, ",") != "v0.6.2,v0.6.1,v0.5.5.2,v0.5.0" {
		t.Fatalf("安全修复版本识别错误: %v", got)
	}
	// 非本仓库的发布地址被替换
	_, _, sec = parseReleases([]ghRelease{{TagName: "v0.6.2.1", HTMLURL: "https://evil.example/x", Body: "## Security"}})
	if len(sec) != 1 || sec[0].URL != repoURL+"/releases/tag/v0.6.2.1" {
		t.Fatalf("外部链接应被替换: %+v", sec)
	}
}

func TestParseAdvisories(t *testing.T) {
	raw := `[
	 {"ghsa_id":"GHSA-m6h7-mm5r-r53q","cve_id":null,"html_url":"https://evil.example","summary":"  Docker read-only leak  ","severity":"medium","state":"published",
	  "vulnerabilities":[{"package":{"ecosystem":"go","name":"github.com/wangwanjie/gately"},"vulnerable_version_range":"<= 0.6.0.4","patched_versions":"0.6.1"}]},
	 {"ghsa_id":"GHSA-aaaa-bbbb-cccc","cve_id":"CVE-2026-1","summary":"withdrawn","withdrawn_at":"2026-10-01T00:00:00Z",
	  "vulnerabilities":[{"package":{"name":""},"vulnerable_version_range":"< 1.0.0"}]},
	 {"ghsa_id":"<script>","summary":"bad id","vulnerabilities":[{"vulnerable_version_range":"< 1.0.0"}]},
	 {"ghsa_id":"GHSA-2345-6789-cfgh","summary":"other package","vulnerabilities":[{"package":{"name":"github.com/other/x"},"vulnerable_version_range":"< 1.0.0"}]},
	 {"ghsa_id":"GHSA-wxwx-wxwx-wxwx","cve_id":"CVE-2026-12345","summary":"` + strings.Repeat("x", 400) + `","severity":"<b>","state":"published",
	  "vulnerabilities":[{"package":{"name":""},"vulnerable_version_range":">= 0.5.0, < 0.6.3","patched_versions":"0.6.3"}]}
	]`
	var list []ghAdvisory
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatal(err)
	}
	out := parseAdvisories(list)
	if len(out) != 2 {
		t.Fatalf("应只保留 2 条公告: %+v", out)
	}
	a := out[0]
	if a.ID != "GHSA-m6h7-mm5r-r53q" || a.Summary != "Docker read-only leak" || a.Severity != "medium" ||
		a.URL != repoURL+"/security/advisories/GHSA-m6h7-mm5r-r53q" || a.CVE != "" || len(a.Ranges) != 1 || a.Ranges[0].Patched != "0.6.1" {
		t.Fatalf("公告解析错误: %+v", a)
	}
	b := out[1]
	if b.Severity != "" || b.CVE != "CVE-2026-12345" || len([]rune(b.Summary)) != maxAdvisorySummary+1 {
		t.Fatalf("严重程度应只接受已知取值，标题应截断: %+v", b)
	}
}

func TestUpdateView(t *testing.T) {
	c := updateCache{
		CheckedAt:        time.Now(),
		Latest:           &releaseBrief{Version: "v0.6.3", URL: repoURL + "/releases/tag/v0.6.3"},
		SecurityReleases: []releaseBrief{{Version: "v0.6.2"}, {Version: "v0.6.1"}, {Version: "v0.5.5.2"}},
		Advisories: []securityAdvisory{
			{ID: "GHSA-m6h7-mm5r-r53q", Ranges: []advisoryRange{{Vulnerable: "<= 0.6.0.4", Patched: "0.6.1"}}},
		},
	}
	v := c.view("v0.6.0", true)
	if !v.HasUpdate || !v.Security || len(v.Advisories) != 1 || len(v.SecurityReleases) != 2 || !v.AutoCheck {
		t.Fatalf("v0.6.0 应提示安全更新（1 条公告、2 个安全修复版本）: %+v", v)
	}
	if keys := v.securityKeys(); strings.Join(keys, ",") != "GHSA-m6h7-mm5r-r53q,v0.6.1,v0.6.2" {
		t.Fatalf("安全条目: %v", keys)
	}
	v = c.view("v0.6.1.1", false)
	if !v.HasUpdate || !v.Security || len(v.Advisories) != 0 || len(v.SecurityReleases) != 1 {
		t.Fatalf("v0.6.1.1 只差 v0.6.2 的安全加固: %+v", v)
	}
	v = c.view("v0.6.3", true)
	if v.HasUpdate || v.Security {
		t.Fatalf("已是最新版本不应提示: %+v", v)
	}
	v = c.view("dev", true)
	if v.HasUpdate || v.Security || v.Comparable {
		t.Fatalf("开发版无法比较: %+v", v)
	}
	if v := (updateCache{}).view("v0.6.0", true); v.Latest != "" || v.CheckedAt != nil || v.Advisories == nil {
		t.Fatalf("从未检查过: %+v", v)
	}
}

func TestNextUpdateCheck(t *testing.T) {
	now := time.Now()
	if d := nextUpdateCheck(updateCache{}, now); d != updateCheckDelay {
		t.Fatalf("从未检查过应在 %v 后检查，实际 %v", updateCheckDelay, d)
	}
	if d := nextUpdateCheck(updateCache{CheckedAt: now.Add(-time.Hour)}, now); d != updateCheckInterval-time.Hour {
		t.Fatalf("上次检查 1 小时前，应再等 %v，实际 %v", updateCheckInterval-time.Hour, d)
	}
	if d := nextUpdateCheck(updateCache{CheckedAt: now.Add(-24 * time.Hour)}, now); d != updateCheckDelay {
		t.Fatalf("超过间隔应尽快检查，实际 %v", d)
	}
	c := updateCache{CheckedAt: now.Add(-24 * time.Hour), FailedAt: now.Add(-10 * time.Minute)}
	if d := nextUpdateCheck(c, now); d != updateCheckRetry-10*time.Minute {
		t.Fatalf("失败后按重试间隔，实际 %v", d)
	}
	if d := nextUpdateCheck(updateCache{CheckedAt: now.Add(48 * time.Hour)}, now); d != updateCheckInterval {
		t.Fatalf("系统时间回调时不超过一个间隔，实际 %v", d)
	}
}

// fakeReleaseAPI 模拟发布列表与安全公告接口，返回值可在测试中修改。
type fakeReleaseAPI struct {
	mu         sync.Mutex
	releases   string
	advisories string
	failRel    bool
	failAdv    bool
	hits       atomic.Int32
	srv        *httptest.Server
}

func newFakeReleaseAPI(t *testing.T) *fakeReleaseAPI {
	t.Helper()
	f := &fakeReleaseAPI{releases: "[]", advisories: "[]"}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			http.Error(w, "不应携带凭据", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/releases":
			if f.failRel {
				http.Error(w, "rate limited", http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(f.releases))
		case "/advisories":
			if f.failAdv {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(f.advisories))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	oldRel, oldAdv, oldVer := releasesAPI, advisoriesAPI, version.Version
	releasesAPI, advisoriesAPI = f.srv.URL+"/releases", f.srv.URL+"/advisories"
	t.Cleanup(func() { releasesAPI, advisoriesAPI, version.Version = oldRel, oldAdv, oldVer })
	return f
}

func (f *fakeReleaseAPI) set(fn func(f *fakeReleaseAPI)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// collectEvents 订阅指定类型的事件。
func collectEvents(t *testing.T, types ...string) func() []events.Event {
	t.Helper()
	var mu sync.Mutex
	var got []events.Event
	unsub := events.Subscribe(func(e events.Event) {
		for _, typ := range types {
			if e.Type == typ {
				mu.Lock()
				got = append(got, e)
				mu.Unlock()
			}
		}
	})
	t.Cleanup(unsub)
	return func() []events.Event {
		// 事件异步投递，稍等片刻
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		out := got
		got = nil
		return out
	}
}

const advisoryJSON = `[{"ghsa_id":"GHSA-m6h7-mm5r-r53q","summary":"Docker read-only leak","severity":"medium","state":"published",
  "vulnerabilities":[{"package":{"name":"github.com/wangwanjie/gately"},"vulnerable_version_range":"<= 0.6.0.4","patched_versions":"0.6.1"}]}]`

func TestCheckUpdatesNotifiesOnce(t *testing.T) {
	gh := newFakeReleaseAPI(t)
	gh.set(func(f *fakeReleaseAPI) {
		f.releases = `[{"tag_name":"v0.6.3"},{"tag_name":"v0.6.1","body":"## Security / 安全修复"}]`
		f.advisories = advisoryJSON
	})
	version.Version = "v0.6.0"
	a, srv, c := loginApp(t)
	got := collectEvents(t, events.SystemUpdate, events.SystemSecurity)

	code, body := doJSON(t, c, "GET", srv.URL+"/api/system/update", nil)
	if code != 200 {
		t.Fatalf("检查更新失败: %d %s", code, body)
	}
	var v updateView
	_ = json.Unmarshal(body, &v)
	if !v.HasUpdate || !v.Security || len(v.Advisories) != 1 || len(v.SecurityReleases) != 1 || v.Latest != "v0.6.3" {
		t.Fatalf("应提示安全更新: %s", body)
	}
	evs := got()
	if len(evs) != 1 || evs[0].Type != events.SystemSecurity || evs[0].Level != events.LevelError ||
		!strings.Contains(evs[0].Message, "GHSA-m6h7-mm5r-r53q") || !strings.Contains(evs[0].Message, "v0.6.3") {
		t.Fatalf("应只发一条安全更新通知（不再另发新版本通知）: %+v", evs)
	}

	// 再次检查（绕过手动检查的最短间隔）不重复通知
	if _, err := a.checkUpdates(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if evs := got(); len(evs) != 0 {
		t.Fatalf("同样的内容不应重复通知: %+v", evs)
	}

	// 发布新版本：没有新的安全问题时发新版本通知
	gh.set(func(f *fakeReleaseAPI) {
		f.releases = `[{"tag_name":"v0.6.4"},{"tag_name":"v0.6.3"},{"tag_name":"v0.6.1","body":"## Security / 安全修复"}]`
	})
	if _, err := a.checkUpdates(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if evs := got(); len(evs) != 1 || evs[0].Type != events.SystemUpdate || evs[0].Fields["latest"] != "v0.6.4" {
		t.Fatalf("新版本应通知一次: %+v", evs)
	}

	// 新的安全公告再次触发安全通知
	gh.set(func(f *fakeReleaseAPI) {
		f.advisories = strings.Replace(advisoryJSON, "}]}]", `}]},{"ghsa_id":"GHSA-2345-6789-cfgh","summary":"new","state":"published","vulnerabilities":[{"package":{"name":""},"vulnerable_version_range":"< 0.6.4"}]}]`, 1)
	})
	if _, err := a.checkUpdates(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if evs := got(); len(evs) != 1 || evs[0].Type != events.SystemSecurity || !strings.Contains(evs[0].Fields["advisories"], "GHSA-2345-6789-cfgh") {
		t.Fatalf("新的安全公告应再次通知: %+v", evs)
	}

	// 重启后读取保存的结果：仍显示升级提示，且不重复通知
	b, err := New(Options{ConfigDir: a.opts.ConfigDir, Modules: []module.Module{portforward.New()}})
	if err != nil {
		t.Fatal(err)
	}
	if st := b.updateSnapshot().view(version.Version, true); !st.Security || st.Latest != "v0.6.4" {
		t.Fatalf("重启后应保留检查结果: %+v", st)
	}
	if _, err := b.checkUpdates(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if evs := got(); len(evs) != 0 {
		t.Fatalf("重启后不应重复通知: %+v", evs)
	}

	// 升级到修复版本后：不再受影响，界面不再提示
	version.Version = "v0.6.4"
	if st := b.updateSnapshot().view(version.Version, true); st.Security || st.HasUpdate {
		t.Fatalf("升级后不应再提示: %+v", st)
	}
	if fi, err := os.Stat(filepath.Join(a.opts.ConfigDir, updateCheckName+".json")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("检查结果应以 0600 保存: %v", err)
	}
}

func TestCheckUpdatesFailures(t *testing.T) {
	gh := newFakeReleaseAPI(t)
	gh.set(func(f *fakeReleaseAPI) {
		f.releases = `[{"tag_name":"v0.6.3"}]`
		f.advisories = advisoryJSON
	})
	version.Version = "v0.6.0"
	a, srv, c := loginApp(t)
	if _, err := a.checkUpdates(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	// 安全公告读取失败：版本检查照常成功，沿用上次的公告
	gh.set(func(f *fakeReleaseAPI) { f.failAdv = true })
	v, err := a.checkUpdates(context.Background(), false)
	if err != nil || len(v.Advisories) != 1 {
		t.Fatalf("公告读取失败时应沿用上次结果: %v %+v", err, v)
	}
	// 发布列表读取失败：记录错误，保留上次结果；同样的错误只记一次日志
	gh.set(func(f *fakeReleaseAPI) { f.failRel = true })
	for range 3 {
		if _, err := a.checkUpdates(context.Background(), false); err == nil || !strings.Contains(err.Error(), "403") {
			t.Fatalf("应返回 GitHub 的错误: %v", err)
		}
	}
	n := 0
	for _, l := range a.sysLogs.Since(0) {
		if strings.Contains(l.Message, "检查更新失败") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("同样的错误应只记录一次，实际 %d 次", n)
	}
	code, body := doJSON(t, c, "GET", srv.URL+"/api/system/update/state", nil)
	var st updateView
	_ = json.Unmarshal(body, &st)
	if code != 200 || st.Error == "" || st.Latest != "v0.6.3" || !st.Security || st.FailedAt == nil {
		t.Fatalf("状态接口应同时给出错误与上次结果: %d %s", code, body)
	}
	// 手动检查失败返回 502
	if code, _ := doJSON(t, c, "GET", srv.URL+"/api/system/update", nil); code != http.StatusBadGateway {
		t.Fatalf("手动检查失败应返回 502，实际 %d", code)
	}
	// 没有正式版本
	gh.set(func(f *fakeReleaseAPI) { f.failRel, f.releases = false, `[{"tag_name":"v1.0.0","draft":true}]` })
	if _, err := a.checkUpdates(context.Background(), false); err == nil {
		t.Fatal("没有正式版本时应报错")
	}
	// 响应过大
	gh.set(func(f *fakeReleaseAPI) {
		f.releases = `[{"tag_name":"v0.6.3","body":"` + strings.Repeat("x", maxReleasesBody) + `"}]`
	})
	if _, err := a.checkUpdates(context.Background(), false); err == nil || !strings.Contains(err.Error(), "过大") {
		t.Fatalf("过大的响应应拒绝: %v", err)
	}
}

// 短时间内重复点击“检查更新”只访问一次 GitHub
func TestManualCheckThrottle(t *testing.T) {
	gh := newFakeReleaseAPI(t)
	gh.set(func(f *fakeReleaseAPI) { f.releases = `[{"tag_name":"v0.6.3"}]` })
	version.Version = "v0.6.3"
	_, srv, c := loginApp(t)
	for range 5 {
		if code, body := doJSON(t, c, "GET", srv.URL+"/api/system/update", nil); code != 200 || !strings.Contains(string(body), `"hasUpdate":false`) {
			t.Fatalf("检查更新失败: %d %s", code, body)
		}
	}
	if n := gh.hits.Load(); n != 2 {
		t.Fatalf("应只请求一次发布列表与一次安全公告，实际 %d 次", n)
	}
	// 状态接口不访问网络
	if code, _ := doJSON(t, c, "GET", srv.URL+"/api/system/update/state", nil); code != 200 || gh.hits.Load() != 2 {
		t.Fatal("状态接口不应访问 GitHub")
	}
}

// 检查失败后同样限流：短时间内重复点击直接返回上次的错误，不再访问 GitHub
func TestManualCheckThrottleAfterFailure(t *testing.T) {
	gh := newFakeReleaseAPI(t)
	gh.set(func(f *fakeReleaseAPI) { f.failRel = true })
	version.Version = "v0.6.3"
	_, srv, c := loginApp(t)
	for range 3 {
		if code, body := doJSON(t, c, "GET", srv.URL+"/api/system/update", nil); code != http.StatusBadGateway || !strings.Contains(string(body), "403") {
			t.Fatalf("失败时应返回 502 与原因: %d %s", code, body)
		}
	}
	if n := gh.hits.Load(); n != 1 {
		t.Fatalf("失败后短时间内不应重复访问 GitHub，实际 %d 次", n)
	}
}

// 调用方取消（程序退出、重启）不算检查失败，不改动保存的结果
func TestCheckCanceledNotRecorded(t *testing.T) {
	gh := newFakeReleaseAPI(t)
	gh.set(func(f *fakeReleaseAPI) { f.releases = `[{"tag_name":"v0.6.3"}]` })
	version.Version = "v0.6.2"
	a, _, _ := loginApp(t)
	if _, err := a.checkUpdates(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.checkUpdates(ctx, true); err == nil {
		t.Fatal("已取消的检查应返回错误")
	}
	if st := a.updateSnapshot(); st.Error != "" || !st.FailedAt.IsZero() || st.Latest == nil {
		t.Fatalf("取消不应记为失败: %+v", st)
	}
}

// 自动检查：按设置开关，关闭时不访问网络，重新开启后立即恢复
func TestRunUpdateCheck(t *testing.T) {
	gh := newFakeReleaseAPI(t)
	gh.set(func(f *fakeReleaseAPI) { f.releases = `[{"tag_name":"v0.6.3"}]` })
	version.Version = "v0.6.2"
	oldDelay, oldInterval := updateCheckDelay, updateCheckInterval
	updateCheckDelay, updateCheckInterval = 20*time.Millisecond, time.Hour
	t.Cleanup(func() { updateCheckDelay, updateCheckInterval = oldDelay, oldInterval })

	a, srv, c := loginApp(t)
	got := collectEvents(t, events.SystemUpdate)
	// 先关闭自动检查
	code, body := doJSON(t, c, "GET", srv.URL+"/api/system/settings", nil)
	if code != 200 {
		t.Fatal(string(body))
	}
	var s map[string]any
	_ = json.Unmarshal(body, &s)
	s["disableUpdateCheck"] = true
	if code, body := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 200 {
		t.Fatalf("保存设置失败: %s", body)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.runUpdateCheck(ctx)
		close(done)
	}()
	time.Sleep(150 * time.Millisecond)
	if n := gh.hits.Load(); n != 0 {
		t.Fatalf("关闭自动检查后不应访问 GitHub，实际 %d 次", n)
	}
	// 重新开启：唤醒检查协程，按启动延迟检查
	s["disableUpdateCheck"] = false
	if code, body := doJSON(t, c, "PUT", srv.URL+"/api/system/settings", s); code != 200 {
		t.Fatalf("保存设置失败: %s", body)
	}
	deadline := time.Now().Add(3 * time.Second)
	for gh.hits.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if evs := got(); len(evs) != 1 {
		t.Fatalf("重新开启后应完成检查并通知新版本: hits=%d %+v", gh.hits.Load(), evs)
	}
	// 一个间隔内不再检查
	time.Sleep(100 * time.Millisecond)
	if n := gh.hits.Load(); n != 2 {
		t.Fatalf("间隔内不应重复检查，实际 %d 次", n)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("取消后检查协程应退出")
	}
}

// fakeProgramModule 是托管一个程序的测试模块。
type fakeProgramModule struct {
	in *binstall.Installer
}

func (m *fakeProgramModule) ID() string                  { return "fakeprog" }
func (m *fakeProgramModule) Title() string               { return "测试程序" }
func (m *fakeProgramModule) Init(module.Env) error       { return nil }
func (m *fakeProgramModule) Start(context.Context) error { return nil }
func (m *fakeProgramModule) Stop() error                 { return nil }
func (m *fakeProgramModule) Routes(module.Router)        {}
func (m *fakeProgramModule) ProgramInstallers() []*binstall.Installer {
	return []*binstall.Installer{m.in}
}

// 自动检查同时检查已安装的托管程序，有新版本时通知一次
func TestCheckProgramUpdates(t *testing.T) {
	gh := newFakeReleaseAPI(t)
	gh.set(func(f *fakeReleaseAPI) { f.releases = `[{"tag_name":"v0.6.3"}]` })
	version.Version = "v0.6.3"
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	var latest atomic.Value
	latest.Store("v1.2.0")
	in := binstall.New(binstall.Spec{
		Key: "fakeprog", Name: "FakeProg", Files: []string{"fakeprog"},
		Resolve: func(ctx context.Context, _ *http.Client, _ binstall.Platform, _ string) (*binstall.Release, error) {
			return &binstall.Release{Version: latest.Load().(string)}, nil
		},
		Probe: func(string) (string, error) { return "v1.1.0", nil },
	}, binstall.Options{BinDir: bin, Logs: logbuf.New(100, nil)})
	mod := &fakeProgramModule{in: in}
	a, err := New(Options{ConfigDir: dir, Modules: []module.Module{mod}})
	if err != nil {
		t.Fatal(err)
	}
	got := collectEvents(t, events.ProgramUpdate)
	// 未安装时不检查
	if _, err := a.checkUpdates(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if evs := got(); len(evs) != 0 || !in.LatestInfo().CheckedAt.IsZero() {
		t.Fatalf("未安装的程序不应检查: %+v", evs)
	}
	if err := os.WriteFile(filepath.Join(bin, binstall.ExeName("fakeprog")), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := a.checkUpdates(context.Background(), true); err != nil {
			t.Fatal(err)
		}
	}
	evs := got()
	if len(evs) != 1 || evs[0].Source != "FakeProg" || evs[0].Fields["latest"] != "v1.2.0" || evs[0].Fields["current"] != "v1.1.0" {
		t.Fatalf("托管程序有新版本应只通知一次: %+v", evs)
	}
	if l := in.LatestInfo(); !l.HasUpdate || l.Version != "v1.2.0" {
		t.Fatalf("安装器应缓存最新版本: %+v", l)
	}
	// 手动检查不检查托管程序
	latest.Store("v1.3.0")
	if _, err := a.checkUpdates(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if evs := got(); len(evs) != 0 {
		t.Fatalf("手动检查不应检查托管程序: %+v", evs)
	}
	if _, err := a.checkUpdates(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if evs := got(); len(evs) != 1 || evs[0].Fields["latest"] != "v1.3.0" {
		t.Fatalf("更新的版本应再次通知: %+v", evs)
	}
	// 卸载后清除通知记录
	if err := os.Remove(filepath.Join(bin, binstall.ExeName("fakeprog"))); err != nil {
		t.Fatal(err)
	}
	if _, err := a.checkUpdates(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if n := a.updateSnapshot().Notified.Programs; len(n) != 0 {
		t.Fatalf("卸载后应清除通知记录: %v", n)
	}
	// 托管程序的检查写回通知记录时不覆盖 Gately 本身的检查结果
	if st := a.updateSnapshot(); st.Latest == nil || st.Latest.Version != "v0.6.3" || st.Notified.Version != "" {
		t.Fatalf("Gately 的检查结果应保留: %+v", st)
	}
}
