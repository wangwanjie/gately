package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/demo"
	"github.com/wangwanjie/gately/internal/httpx"
)

// 演示模式下数据面服务都不启动，各模块的运行状态与统计要由这里补上：
// 对 GET 接口的响应做改写，让界面看起来像一个正常运行的实例。数值由规则的 key 与当前时间确定地推出，
// 同一条规则在不同页面里看到的数字一致，随时间平滑变化。

type demoMap = map[string]any

// demoXforms 改写模块自己的响应（保留其配置字段，只补运行状态），demoFull 整个响应都由这里生成。
var (
	demoXforms = map[string]func(a *App, r *http.Request, v any) any{}
	demoFull   = map[string]func(a *App, r *http.Request) any{}
)

func init() {
	demoXforms["/api/portforward/rules"] = demoRules
	demoXforms["/api/protoroute/rules"] = demoRules
	demoXforms["/api/webservice/listeners"] = demoListeners
	demoXforms["/api/webservice/topology"] = demoTopology
	demoXforms["/api/ddns/tasks"] = demoDDNS
	demoXforms["/api/stun/rules"] = demoSTUN
	demoXforms["/api/frp/clients"] = demoProcState
	demoXforms["/api/frp/servers"] = demoProcState
	demoXforms["/api/tailscale/instances"] = demoProcState
	demoXforms["/api/netbird/instances"] = demoProcState
	demoXforms["/api/easytier/instances"] = demoProcState
	demoXforms["/api/tailscale/status"] = demoTailscaleStatus
	demoXforms["/api/netbird/status"] = demoNetbirdStatus
	demoXforms["/api/guard/jails"] = demoJails
	demoXforms["/api/guard/status"] = demoGuardStatus
	demoXforms["/api/knock/status"] = demoKnockStatus
	demoXforms["/api/waf/instances"] = demoWAF
	demoXforms["/api/cron/tasks"] = demoCron
	demoXforms["/api/wol/status"] = demoWOLStatus
	demoFull["/api/notify/history"] = demoNotifyHistory
	demoFull["/api/webservice/stats"] = demoSiteStats
	demoFull["/api/webservice/connections"] = demoConns
	demoFull["/api/perf/overview"] = demoPerfOverview
	demoFull["/api/perf/sites"] = demoPerfSites
	demoFull["/api/perf/processes"] = demoPerfProcesses
}

// ---- 中间件 ----

// bufferedResponse 暂存下游响应，改写后再输出。
type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header         { return b.header }
func (b *bufferedResponse) Write(p []byte) (int, error) { return b.body.Write(p) }
func (b *bufferedResponse) WriteHeader(code int)        { b.status = code }

// demoOverlay 在演示模式下改写 GET 响应。放在鉴权与脱敏之内、模块处理器之外。
func (a *App) demoOverlay(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !demo.Enabled() || r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		if full, ok := demoFull[r.URL.Path]; ok {
			writeDemoJSON(w, full(a, r))
			return
		}
		xf, ok := demoXforms[r.URL.Path]
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		// 内容随时间变化：不让客户端用 ETag 拿到 304
		r.Header.Del("If-None-Match")
		rec := &bufferedResponse{header: http.Header{}, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		var v any
		if rec.status != http.StatusOK || json.Unmarshal(rec.body.Bytes(), &v) != nil {
			for k, vs := range rec.header {
				w.Header()[k] = vs
			}
			w.WriteHeader(rec.status)
			_, _ = w.Write(rec.body.Bytes())
			return
		}
		writeDemoJSON(w, xf(a, r, v))
	})
}

func writeDemoJSON(w http.ResponseWriter, v any) {
	httpx.OK(w, v)
}

// ---- 确定性的数值 ----

func demoHash(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// demoWave 返回在 [lo, hi] 之间随时间平滑起伏的值，不同 seed 的相位不同。
func demoWave(seed string, period float64, lo, hi float64) float64 {
	phase := float64(demoHash(seed)%628) / 100
	x := math.Sin(2*math.Pi*float64(time.Now().Unix())/period + phase)
	return lo + (hi-lo)*(0.5+0.5*x)
}

// demoTotal 是随时间累计增长的计数：已“运行”数天的基数加上本进程启动以来按速率的增量。
func (a *App) demoTotal(seed string, perSec float64) int64 {
	days := 2 + float64(demoHash(seed+"/d")%5)
	return int64(perSec*86400*days + perSec*time.Since(a.startTime).Seconds())
}

// demoRate 是由 seed 确定的稳定速率（每秒），范围 [lo, hi]。
func demoRate(seed string, lo, hi float64) float64 {
	return lo + (hi-lo)*float64(demoHash(seed)%1000)/1000
}

func asMaps(v any) []demoMap {
	var out []demoMap
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			if m, ok := e.(demoMap); ok {
				out = append(out, m)
			}
		}
	case demoMap:
		out = append(out, x)
	}
	return out
}

func str(m demoMap, k string) string {
	s, _ := m[k].(string)
	return s
}

func isOn(m demoMap, k string) bool {
	b, _ := m[k].(bool)
	return b
}

// demoConfig 读取模块的配置文件（解析成通用结构），文件不存在时返回空。
func (a *App) demoConfig(id string) demoMap {
	b, err := os.ReadFile(filepath.Join(a.opts.ConfigDir, id+".json"))
	if err != nil {
		return demoMap{}
	}
	var m demoMap
	if json.Unmarshal(b, &m) != nil {
		return demoMap{}
	}
	return m
}

func (a *App) demoEnabledCount(id, field string) int {
	n := 0
	for _, it := range asMaps(a.demoConfig(id)[field]) {
		if isOn(it, "enable") {
			n++
		}
	}
	return n
}

// ---- 各模块的运行状态 ----

func demoRules(a *App, r *http.Request, v any) any {
	for _, it := range asMaps(v) {
		if !isOn(it, "enable") {
			continue
		}
		key := str(it, "key")
		it["running"] = true
		perSec := demoRate(key, 0.01, 0.4)
		active := int64(demoWave(key, 120, 0, 3+perSec*40))
		stats := demoMap{
			"activeConns": active, "totalConns": a.demoTotal(key, perSec),
			"bytesIn": a.demoTotal(key+"/in", 3000+perSec*20000), "bytesOut": a.demoTotal(key+"/out", 9000+perSec*60000),
			"denied": a.demoTotal(key+"/deny", 0.0002),
		}
		if rts, ok := it["routes"].([]any); ok {
			counts := make([]any, len(rts))
			var sum int64
			for i := range rts {
				c := a.demoTotal(fmt.Sprintf("%s/r%d", key, i), perSec/float64(i+1))
				counts[i] = c
				sum += c
			}
			stats["routeConns"] = counts
			stats["defaultConns"] = a.demoTotal(key+"/def", perSec/4)
			stats["rejected"] = int64(2)
			stats["knockDenied"] = int64(0)
		}
		it["stats"] = stats
	}
	return v
}

// demoSiteRate 是站点的稳定请求速率（次/秒）：对外站点较高，内网较低，默认站点极低。
func demoSiteRate(listener, site string) float64 {
	key := listener + "/" + site
	switch {
	case site == "default":
		return demoRate(key, 0.01, 0.05)
	case listener == "l-pub":
		return demoRate(key, 0.8, 7)
	}
	return demoRate(key, 0.1, 1.2)
}

func parseTarget(raw string) (url string, weight int, backup bool) {
	weight = 1
	f := strings.Fields(raw)
	if len(f) == 0 {
		return raw, 1, false
	}
	url = f[0]
	for _, o := range f[1:] {
		switch {
		case o == "backup":
			backup = true
		case strings.HasPrefix(o, "weight="):
			if n, err := strconv.Atoi(strings.TrimPrefix(o, "weight=")); err == nil {
				weight = n
			}
		}
	}
	return
}

func demoListeners(a *App, r *http.Request, v any) any {
	for _, l := range asMaps(v) {
		if !isOn(l, "enable") {
			continue
		}
		lk := str(l, "key")
		l["running"] = true
		stats := demoMap{}
		ups := demoMap{}
		sites := asMaps(l["sites"])
		if d, ok := l["default"].(demoMap); ok {
			sites = append(sites, d)
		}
		for _, s := range sites {
			if !isOn(s, "enable") {
				continue
			}
			sk := str(s, "key")
			rate := demoSiteRate(lk, sk)
			stats[sk] = demoMap{"requests": a.demoTotal(lk+"/"+sk, rate), "active": int64(demoWave(lk+sk, 90, 0, rate*1.5)), "errors": a.demoTotal(lk+"/"+sk+"/e", rate/200)}
			if ts, ok := s["targets"].([]any); ok && str(s, "type") == "proxy" {
				var list []any
				for i, t := range ts {
					u, w, b := parseTarget(fmt.Sprint(t))
					share := rate / float64(len(ts))
					list = append(list, demoMap{"url": u, "weight": w, "backup": b, "healthy": true, "conns": int64(demoWave(lk+sk+u, 70, 0, 2+share*2)),
						"requests": a.demoTotal(fmt.Sprintf("%s/%s/u%d", lk, sk, i), share), "bytes": a.demoTotal(fmt.Sprintf("%s/%s/ub%d", lk, sk, i), share*52000)})
				}
				ups[sk] = list
			}
		}
		l["stats"] = stats
		l["upstreams"] = ups
	}
	return v
}

func demoDDNS(a *App, r *http.Request, v any) any {
	now := time.Now()
	for _, t := range asMaps(v) {
		if !isOn(t, "enable") {
			continue
		}
		key := str(t, "key")
		interval := 300.0
		if n, ok := t["interval"].(float64); ok && n > 0 {
			interval = n
		}
		offset := float64(demoHash(key) % uint32(interval))
		last := now.Add(-time.Duration(math.Mod(float64(now.Unix())+offset, interval)) * time.Second)
		recs := demoMap{}
		for _, rec := range asMaps(t["records"]) {
			if !isOn(rec, "enable") {
				continue
			}
			val := "203.0.113.10"
			if str(rec, "type") == "AAAA" {
				val = "2001:db8::10"
			}
			recs[str(rec, "key")] = demoMap{"value": val, "lastSync": last.Add(-time.Duration(demoHash(key+str(rec, "key"))%72) * time.Hour), "lastAction": "unchanged", "lastCheck": last}
		}
		st := demoMap{"running": true, "records": recs, "lastRun": last, "nextRun": last.Add(time.Duration(interval) * time.Second)}
		if isOn(t["ipv4"].(demoMap), "enable") {
			st["ipv4"] = "203.0.113.10"
		}
		if isOn(t["ipv6"].(demoMap), "enable") {
			st["ipv6"] = "2001:db8::10"
		}
		t["state"] = st
	}
	return v
}

func demoSTUN(a *App, r *http.Request, v any) any {
	now := time.Now()
	for _, it := range asMaps(v) {
		if !isOn(it, "enable") {
			continue
		}
		key := str(it, "key")
		port := 40000 + int(demoHash(key)%20000)
		perSec := demoRate(key, 0.02, 0.2)
		st := demoMap{
			"status": "ok", "publicAddr": fmt.Sprintf("203.0.113.10:%d", port), "localAddr": fmt.Sprintf("192.0.2.1:%v", it["localPort"]),
			"stunServer": "stun.example.com:3478", "since": now.Add(-time.Duration(3+demoHash(key)%40) * time.Hour),
			"stats":   demoMap{"activeConns": int64(demoWave(key, 100, 0, 5)), "totalConns": a.demoTotal(key, perSec), "bytesIn": a.demoTotal(key+"/i", 8000), "bytesOut": a.demoTotal(key+"/o", 30000), "denied": int64(0)},
			"history": []any{demoMap{"time": now.Add(-26 * time.Hour), "from": "203.0.113.10:41562", "to": fmt.Sprintf("203.0.113.10:%d", port)}},
		}
		it["state"] = st
	}
	return v
}

// demoProcState 把托管进程类实例（frp、Tailscale、NetBird、EasyTier）标为运行中。
func demoProcState(a *App, r *http.Request, v any) any {
	now := time.Now()
	for _, it := range asMaps(v) {
		if !isOn(it, "enable") {
			continue
		}
		st := demoMap{"running": true, "since": now.Add(-time.Duration(48+demoHash(str(it, "key"))%200) * time.Hour), "restarts": int64(0), "pid": 200 + demoHash(str(it, "key"))%800}
		it["state"] = st
		it["error"] = ""
		if d, ok := it["daemon"].(demoMap); ok {
			for k, x := range st {
				d[k] = x
			}
		}
	}
	return v
}

func demoPeers(key string, names ...string) []any {
	now := time.Now()
	var out []any
	for i, n := range names {
		out = append(out, demoMap{
			"name": n, "dnsName": n + ".example.ts.net", "ips": []string{fmt.Sprintf("198.51.100.%d", 10+i), fmt.Sprintf("2001:db8:100::%x", 10+i)},
			"os": []string{"linux", "iOS", "windows", "macOS"}[i%4], "online": i != len(names)-1, "active": i%2 == 0, "relay": []string{"hkg", "tok", "sin"}[i%3],
			"curAddr": fmt.Sprintf("203.0.113.%d:41641", 40+i), "rxBytes": int64(1e8) * int64(i+2), "txBytes": int64(6e7) * int64(i+1), "lastSeen": now.Add(-time.Duration(i*i) * time.Minute),
		})
	}
	return out
}

func demoTailscaleStatus(a *App, r *http.Request, v any) any {
	for _, it := range asMaps(v) {
		if !isOn(it, "enable") {
			continue
		}
		key := str(it, "key")
		it["daemon"] = demoMap{"running": true, "since": time.Now().Add(-9 * 24 * time.Hour), "restarts": int64(0)}
		it["cli"] = "/usr/local/bin/tailscale"
		it["version"] = "1.80.2"
		it["error"] = ""
		it["backendState"] = "Running"
		it["tailnet"] = "demo@example.com"
		it["magicDnsSuffix"] = "example.ts.net"
		it["self"] = demoMap{"name": "gately-home", "dnsName": "gately-home.example.ts.net", "ips": []string{"198.51.100.4", "2001:db8:100::4"}, "os": "linux", "online": true, "active": true, "exitNode": false, "exitNodeOption": true}
		it["peers"] = demoPeers(key, "laptop", "phone", "office-pc", "vps-hk")
	}
	return v
}

func demoNetbirdStatus(a *App, r *http.Request, v any) any {
	now := time.Now()
	for _, it := range asMaps(v) {
		if !isOn(it, "enable") {
			continue
		}
		it["daemon"] = demoMap{"running": true, "since": now.Add(-6 * 24 * time.Hour), "restarts": int64(1)}
		it["cli"] = "/usr/local/bin/netbird"
		it["version"] = "0.35.2"
		it["error"] = ""
		it["daemonStatus"] = "Connected"
		it["management"] = demoMap{"url": "https://netbird.example.com:443", "connected": true}
		it["signal"] = demoMap{"url": "https://netbird.example.com:10000", "connected": true}
		it["relaysTotal"], it["relaysUp"] = 2, 2
		it["self"] = demoMap{"name": "gately-home", "fqdn": "gately-home.netbird.example.com", "ips": []string{"203.0.113.5"}, "wgPort": 51820, "kernel": false}
		var peers []any
		for i, n := range []string{"nas", "office", "cabin"} {
			peers = append(peers, demoMap{"name": n, "fqdn": n + ".netbird.example.com", "ips": []string{fmt.Sprintf("203.0.113.%d", 20+i)}, "online": i != 2, "status": []string{"Connected", "Connected", "Idle"}[i],
				"connType": []string{"P2P", "P2P", "Relayed"}[i], "latencyMs": 8.0 + float64(i*21), "rxBytes": int64(2e8) * int64(i+1), "txBytes": int64(9e7) * int64(i+1), "lastSeen": now.Add(-time.Duration(i*i*5) * time.Minute)})
		}
		it["peers"] = peers
		it["peersConnected"] = 2
	}
	return v
}

func demoJails(a *App, r *http.Request, v any) any {
	for _, it := range asMaps(v) {
		if !isOn(it, "enable") {
			continue
		}
		key := str(it, "key")
		it["running"] = true
		it["matches"] = a.demoTotal(key, 0.003)
		it["bans"] = a.demoTotal(key+"/b", 0.00002)
		it["active"] = int64(1 + demoHash(key)%2)
	}
	return v
}

func demoGuardStatus(a *App, r *http.Request, v any) any {
	m, ok := v.(demoMap)
	if !ok {
		return v
	}
	cfg := a.demoConfig("guard")
	set, _ := cfg["settings"].(demoMap)
	trap, _ := set["trap"].(demoMap)
	ports := str(trap, "ports")
	m["firewall"] = demoMap{"active": true, "enabled": true, "dropped": demoMap{"ban": a.demoTotal("guard/ban", 0.01), "trap": a.demoTotal("guard/trap", 0.004)},
		"hits": demoMap{}, "openPorts": 0, "redirect": true, "trapPorts": ports, "trapSkipped": []any{}}
	return m
}

func demoKnockStatus(a *App, r *http.Request, v any) any {
	m, ok := v.(demoMap)
	if !ok {
		return v
	}
	now := time.Now()
	m["listening"] = true
	m["counters"] = demoMap{"accepted": a.demoTotal("knock/acc", 0.0004), "ignored": a.demoTotal("knock/ign", 0.0002), "rejected": a.demoTotal("knock/rej", 0.00005)}
	m["firewall"] = demoMap{"active": true, "allowed": a.demoTotal("knock/allow", 0.0003), "dropped": a.demoTotal("knock/drop", 0.002), "needed": true, "backend": "nftables", "ports": []string{"tcp/22"}}
	m["grants"] = []any{
		demoMap{"addr": "203.0.113.17", "rule": "k-ssh", "ruleName": a.demoName("knock", "rules", "k-ssh"), "until": now.Add(25 * time.Second), "ports": []string{"tcp/22"}, "source": "spa"},
		demoMap{"addr": "198.51.100.64", "rule": "k-ssh", "ruleName": a.demoName("knock", "rules", "k-ssh"), "until": now.Add(12 * time.Second), "ports": []string{"tcp/22"}, "source": "spa"},
	}
	return m
}

func demoWAF(a *App, r *http.Request, v any) any {
	for _, it := range asMaps(v) {
		key := str(it, "key")
		matched := a.demoTotal(key+"/m", 0.002)
		it["matched"] = matched
		if str(it, "mode") == "block" {
			it["blocked"] = matched * 3 / 4
		}
	}
	return v
}

func demoCron(a *App, r *http.Request, v any) any {
	now := time.Now()
	for _, it := range asMaps(v) {
		if !isOn(it, "enable") {
			continue
		}
		key := str(it, "key")
		var hist []any
		for i := 0; i < 8; i++ {
			ok := !(i == 5 && key == "cr-ping")
			out := "OK"
			if !ok {
				out = `Get "https://hc.example.com/ping/demo": context deadline exceeded`
			}
			hist = append(hist, demoMap{"start": now.Add(-time.Duration(i+1) * time.Duration(5+demoHash(key)%50) * time.Minute), "durationMs": 120 + int64(demoHash(fmt.Sprint(key, i))%900), "ok": ok, "output": out, "manual": false})
		}
		it["history"] = hist
	}
	return v
}

func demoWOLStatus(a *App, r *http.Request, v any) any {
	m, ok := v.(demoMap)
	if !ok {
		return v
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for n, k := range keys {
		if n%2 == 0 {
			m[k] = "online"
		} else {
			m[k] = "offline"
		}
	}
	return m
}

func demoNotifyHistory(a *App, r *http.Request) any {
	now := time.Now()
	type rec struct {
		ago      time.Duration
		typ, msg string
		ch, rule string
		ok       bool
		err      string
	}
	list := []rec{
		{14 * time.Minute, "guard.banned", "198.51.100.23", "n-tg", "nr-sec", true, ""},
		{14 * time.Minute, "guard.banned", "198.51.100.23", "n-bark", "nr-sec", true, ""},
		{3 * time.Hour, "ddns.changed", "example.com 203.0.113.10", "n-mail", "nr-ops", true, ""},
		{9 * time.Hour, "ssl.expiring", "*.lab.example.com", "n-mail", "nr-ops", true, ""},
		{9 * time.Hour, "ssl.expiring", "*.lab.example.com", "n-tg", "nr-ops", false, "dial tcp 203.0.113.80:443: i/o timeout"},
		{26 * time.Hour, "stun.changed", "203.0.113.10", "n-tg", "nr-ops", true, ""},
		{2 * 24 * time.Hour, "waf.blocked", "git.example.com", "n-bark", "nr-sec", true, ""},
	}
	out := make([]any, 0, len(list))
	for _, e := range list {
		out = append(out, demoMap{"time": now.Add(-e.ago), "type": e.typ, "title": e.typ + ": " + e.msg,
			"rule": a.demoName("notify", "rules", e.rule), "channel": a.demoName("notify", "channels", e.ch), "ok": e.ok, "error": e.err})
	}
	return out
}

// demoName 返回演示配置中某个条目的名称，找不到时返回 key。
func (a *App) demoName(id, field, key string) string {
	for _, it := range asMaps(a.demoConfig(id)[field]) {
		if str(it, "key") == key {
			return str(it, "name")
		}
	}
	return key
}

// ---- 站点统计、连接与拓扑 ----

func (a *App) demoSite(listener, site string) demoMap {
	for _, l := range asMaps(a.demoConfig("webservice")["listeners"]) {
		if str(l, "key") != listener {
			continue
		}
		if site == "default" {
			d, _ := l["default"].(demoMap)
			return d
		}
		for _, s := range asMaps(l["sites"]) {
			if str(s, "key") == site {
				return s
			}
		}
	}
	return nil
}

func demoSiteStats(a *App, r *http.Request) any {
	q := r.URL.Query()
	lk, sk := q.Get("listener"), q.Get("site")
	minutes, _ := strconv.Atoi(q.Get("minutes"))
	if minutes < 1 || minutes > 1440 {
		minutes = 60
	}
	rate := demoSiteRate(lk, sk)
	now := time.Now()
	cur := now.Unix() / 60
	series := make([]any, 0, minutes)
	for i := minutes - 1; i >= 0; i-- {
		min := cur - int64(i)
		// 昼夜起伏叠加小幅波动
		day := 0.6 + 0.4*math.Sin(2*math.Pi*float64(min*60-8*3600)/86400)
		jit := 0.8 + 0.4*float64(demoHash(fmt.Sprint(lk, sk, min))%100)/100
		req := int64(rate * 60 * day * jit)
		p50 := 18 + float64(demoHash(fmt.Sprint(sk, min, "p"))%14)
		series = append(series, demoMap{"t": min * 60, "requests": req, "bytes": req * (30000 + int64(demoHash(sk)%40000)), "s4xx": req / 25, "s5xx": req / 400, "p50": p50, "p95": p50*4 + float64(demoHash(fmt.Sprint(sk, min, "q"))%60)})
	}
	total := a.demoTotal(lk+"/"+sk, rate)
	status := demoMap{"2xx": total * 86 / 100, "3xx": total * 8 / 100, "4xx": total * 55 / 1000, "5xx": total / 200}
	var topIPs []any
	for i := 0; i < 8; i++ {
		topIPs = append(topIPs, demoMap{"key": demoIP(i + int(demoHash(sk)%7)), "count": total / int64(4+i*3)})
	}
	countries := []string{"US", "CN", "DE", "JP", "GB", "SG", "NL", "FR"}
	var topC []any
	for i, c := range countries {
		topC = append(topC, demoMap{"key": c, "count": total / int64(3+i*4)})
	}
	out := demoMap{"series": series, "status": status, "bytesOut": a.demoTotal(lk+sk+"/b", rate*52000), "topIps": topIPs, "topCountries": topC,
		"blocked": demoMap{"ban": total / 900, "ip": total / 1500, "waf": total / 700, "country": int64(0), "rateLimit": total / 4000}}
	var ups []any
	if s := a.demoSite(lk, sk); s != nil {
		if ts, ok := s["targets"].([]any); ok && str(s, "type") == "proxy" {
			for i, t := range ts {
				u, w, b := parseTarget(fmt.Sprint(t))
				share := rate / float64(len(ts))
				ups = append(ups, demoMap{"url": u, "weight": w, "backup": b, "healthy": true, "conns": int64(demoWave(lk+sk+u, 70, 0, 3)), "requests": a.demoTotal(fmt.Sprintf("%s/%s/u%d", lk, sk, i), share), "bytes": a.demoTotal(fmt.Sprintf("%s/%s/ub%d", lk, sk, i), share*52000)})
			}
		}
	}
	out["upstreams"] = ups
	return out
}

// demoIP 返回第 i 个演示访客地址，取自文档保留网段。
func demoIP(i int) string {
	switch i % 4 {
	case 0:
		return fmt.Sprintf("198.51.100.%d", 1+i*7%250)
	case 1:
		return fmt.Sprintf("203.0.113.%d", 1+i*11%250)
	case 2:
		return fmt.Sprintf("192.0.2.%d", 2+i*13%240)
	}
	return fmt.Sprintf("2001:db8:%x::%x", 1+i%30, 1+i)
}

func demoConns(a *App, r *http.Request) any {
	q := r.URL.Query()
	lk, sk := q.Get("listener"), q.Get("site")
	s := a.demoSite(lk, sk)
	host := "example.com"
	if ds, ok := s["domains"].([]any); ok && len(ds) > 0 {
		host = fmt.Sprint(ds[0])
	}
	now := time.Now()
	paths := []string{"/", "/api/v1/items", "/assets/app.js", "/feed.xml", "/login", "/posts/hello-world/"}
	var active []any
	for i := 0; i < 1+int(demoHash(sk)%4); i++ {
		active = append(active, demoMap{"id": strconv.Itoa(1000 + i), "ip": demoIP(i + 3), "port": 40000 + i*113, "proto": "HTTP/2.0", "method": "GET", "host": host,
			"path": paths[(i+int(demoHash(sk)))%len(paths)], "start": now.Add(-time.Duration(2+i*7) * time.Second), "durationMs": int64(2000 + i*7000), "down": int64(40000 + i*9000), "up": int64(1200)})
	}
	var recent []any
	for i := 0; i < 20; i++ {
		end := now.Add(-time.Duration(3+i*11) * time.Second)
		dur := int64(30 + demoHash(fmt.Sprint(sk, i))%900)
		recent = append(recent, demoMap{"id": strconv.Itoa(900 - i), "ip": demoIP(i), "port": 41000 + i*31, "proto": "HTTP/2.0", "method": "GET", "host": host, "path": paths[i%len(paths)],
			"start": end.Add(-time.Duration(dur) * time.Millisecond), "end": end, "durationMs": dur, "down": int64(8000 + demoHash(fmt.Sprint(i, sk))%200000), "up": int64(500), "result": "done"})
	}
	return demoMap{"active": active, "recent": recent, "activeTotal": len(active)}
}

func demoTopology(a *App, r *http.Request, v any) any {
	t, ok := v.(demoMap)
	if !ok {
		return v
	}
	now := time.Now()
	focus := r.URL.Query().Get("focus")
	var totalReq, totalActive int64
	var totalQPS, totalBPS float64
	var ipNodes [][]string
	for _, l := range asMaps(t["listeners"]) {
		if !isOn(l, "enable") {
			continue
		}
		lk := str(l, "key")
		l["running"] = true
		var lReq, lActive int64
		var lQPS, lBPS float64
		for _, s := range asMaps(l["sites"]) {
			if !isOn(s, "enable") {
				continue
			}
			sk := str(s, "key")
			qps := demoSiteRate(lk, sk) * (0.8 + 0.4*demoWave(lk+sk, 37, 0, 1))
			bps := qps * (30000 + float64(demoHash(sk)%40000))
			req := a.demoTotal(lk+"/"+sk, demoSiteRate(lk, sk))
			act := int64(demoWave(lk+sk, 90, 0, qps*1.5))
			s["running"] = true
			s["requests"], s["active"] = req, act
			s["errors"], s["closed"] = req/200, int64(0)
			s["rate"] = demoMap{"qps": qps, "qps60": qps * 0.97, "bps": bps}
			s["status"] = demoMap{"2xx": req * 86 / 100, "3xx": req * 8 / 100, "4xx": req * 55 / 1000, "5xx": req / 200}
			s["blocked"] = demoMap{}
			if sk == "default" {
				s["closed"] = req
			}
			ups := asMaps(s["upstreams"])
			for _, u := range ups {
				u["healthy"] = true
				u["conns"] = act / int64(len(ups))
				u["requests"] = req / int64(len(ups))
				u["bytes"] = req / int64(len(ups)) * 52000
				u["rate"] = demoMap{"qps": qps / float64(len(ups)), "qps60": qps / float64(len(ups)), "bps": bps / float64(len(ups))}
			}
			lReq += req
			lActive += act
			lQPS += qps
			lBPS += bps
			ipNodes = append(ipNodes, []string{str(s, "id")})
		}
		for _, ac := range asMaps(l["actions"]) {
			ac["requests"] = a.demoTotal(lk+"/act", 0.1)
			ac["rate"] = demoMap{"qps": 0.1, "qps60": 0.1, "bps": 2000.0}
		}
		l["requests"], l["active"] = lReq, lActive
		l["rate"] = demoMap{"qps": lQPS, "qps60": lQPS * 0.97, "bps": lBPS}
		totalReq += lReq
		totalActive += lActive
		totalQPS += lQPS
		totalBPS += lBPS
	}
	t["requests"], t["active"] = totalReq, totalActive
	t["rate"] = demoMap{"qps": totalQPS, "qps60": totalQPS * 0.97, "bps": totalBPS}
	t["time"] = now.Unix()
	var ids []string
	for _, n := range ipNodes {
		ids = append(ids, n...)
	}
	var ips []any
	for i := 0; i < 24 && len(ids) > 0; i++ {
		first := ids[i%len(ids)]
		second := ids[(i*3+1)%len(ids)]
		act := 0
		if i%5 == 0 {
			act = 1
		}
		ips = append(ips, demoMap{"ip": demoIP(i), "nodes": []string{first, second}, "active": act, "last": now.Unix() - int64(i*2)})
	}
	t["ips"], t["ipTotal"], t["ipTruncated"] = ips, len(ips), false
	if focus != "" {
		var series []any
		for i := 59; i >= 0; i-- {
			ts := now.Unix() - int64(i)
			q := totalQPS * (0.8 + 0.4*math.Sin(float64(ts)/9))
			series = append(series, demoMap{"t": ts, "requests": int64(q), "bytes": int64(q * 40000)})
		}
		t["focus"], t["series"] = focus, series
	}
	return t
}

// ---- 总览与性能 ----

func demoPerfPoint(t int64) demoMap {
	ts := time.Unix(t, 0)
	f := func(period, phase, lo, hi float64) float64 {
		return lo + (hi-lo)*(0.5+0.5*math.Sin(2*math.Pi*float64(ts.Unix())/period+phase))
	}
	cpu := f(2400, 0, 7, 26) + float64(demoHash(strconv.FormatInt(t, 10))%40)/10
	mem := f(5000, 1, 33, 39)
	return demoMap{"t": t, "cpu": cpu, "cpuMax": cpu * 1.4, "mem": mem, "disk": 29.0, "load1": cpu / 40, "procCpu": cpu / 14, "procCpuMax": cpu / 8,
		"procRss": f(3000, 2, 44, 52), "procFds": 36.0, "goroutines": 70 + float64(demoHash(strconv.FormatInt(t, 10)+"g")%12), "childCpu": cpu / 6, "childCpuMax": cpu / 4, "childRss": 136.0}
}

func demoPerfOverview(a *App, r *http.Request) any {
	rng := r.URL.Query().Get("range")
	now := time.Now().Unix()
	var step, n int64
	switch rng {
	case "1h":
		step, n = 60, 60
	case "24h":
		step, n = 300, 288
	default:
		rng, step, n = "60s", 5, 12
	}
	series := make([]any, 0, n)
	for i := n - 1; i >= 0; i-- {
		series = append(series, demoPerfPoint((now-i*step)/step*step))
	}
	return demoMap{"range": rng, "step": step, "series": series, "latest": demoPerfPoint(now)}
}

func demoPerfSites(a *App, r *http.Request) any {
	var out []any
	for _, l := range asMaps(a.demoConfig("webservice")["listeners"]) {
		if !isOn(l, "enable") {
			continue
		}
		lk := str(l, "key")
		for _, s := range asMaps(l["sites"]) {
			if !isOn(s, "enable") || str(s, "type") == "close" {
				continue
			}
			sk := str(s, "key")
			qps := demoSiteRate(lk, sk) * (0.8 + 0.4*demoWave(lk+sk, 37, 0, 1))
			p95 := 90 + demoWave(lk+sk, 53, 0, 140)
			e5 := demoRate(sk+"e", 0, 0.9)
			row := demoMap{"key": lk + "/" + sk, "name": str(s, "name"), "active": int64(demoWave(lk+sk, 90, 0, qps*1.5)), "qps": qps, "err5xx": e5, "p95": p95,
				"requests": uint64(qps * 60), "latencies": uint64(qps * 60)}
			if ts, ok := s["targets"].([]any); ok && str(s, "type") == "proxy" {
				var ups []any
				for i, t := range ts {
					u, _, _ := parseTarget(fmt.Sprint(t))
					ups = append(ups, demoMap{"key": fmt.Sprintf("%s/%s/%d", lk, sk, i), "url": u, "healthy": true, "conns": int64(demoWave(lk+sk+u, 70, 0, 3))})
				}
				row["upstreams"] = ups
			}
			out = append(out, row)
		}
	}
	return out
}

func demoPerfProcesses(a *App, r *http.Request) any {
	now := time.Now()
	type p struct{ module, name, program string }
	var procs []p
	if cs := asMaps(a.demoConfig("frp")["clients"]); len(cs) > 0 {
		procs = append(procs, p{"frp", str(cs[0], "name"), "frpc"})
	}
	if cs := asMaps(a.demoConfig("tailscale")["instances"]); len(cs) > 0 {
		procs = append(procs, p{"tailscale", str(cs[0], "name"), "tailscaled"})
	}
	if cs := asMaps(a.demoConfig("netbird")["instances"]); len(cs) > 0 {
		procs = append(procs, p{"netbird", str(cs[0], "name"), "netbird"})
	}
	out := []any{}
	for i, pr := range procs {
		var samples []any
		for j := 59; j >= 0; j -= 5 {
			cpu := demoWave(pr.name+strconv.Itoa(j), 80, 0.1, 1.2)
			samples = append(samples, demoMap{"t": now.Unix() - int64(j), "cpu": cpu, "rss": 30.0 + float64(i*14)})
		}
		out = append(out, demoMap{"module": pr.module, "name": pr.name, "program": pr.program, "pid": 211 + i*12, "since": now.Add(-9 * 24 * time.Hour), "restarts": 0,
			"cpu": demoWave(pr.name, 80, 0.1, 1.2), "rss": uint64(30+i*14) << 20, "threads": 11 + i, "samples": samples})
	}
	return out
}

// ---- 概览 ----

// demoSummary 返回模块在总览页的摘要，替换依赖运行状态的内容；返回 nil 表示沿用模块自己的摘要。
func (a *App) demoSummary(id string) []summaryItem {
	switch id {
	case "portforward", "protoroute":
		n := a.demoEnabledCount(id, "rules")
		return []summaryItem{{"运行中", fmt.Sprintf("%d / %d", n, n), "ok"}, {"活动连接", fmt.Sprint(int(demoWave(id, 120, 6, 22))), ""}}
	case "ddns":
		recs := 0
		for _, t := range asMaps(a.demoConfig("ddns")["tasks"]) {
			if isOn(t, "enable") {
				for _, rec := range asMaps(t["records"]) {
					if isOn(rec, "enable") {
						recs++
					}
				}
			}
		}
		return []summaryItem{{"运行中的记录", fmt.Sprint(recs), ""}, {"同步失败", "0", "ok"}, {"IPv4", "203.0.113.10", ""}, {"IPv6", "2001:db8::10", ""}}
	case "stun":
		n := a.demoEnabledCount("stun", "rules")
		return []summaryItem{{"已穿透", fmt.Sprintf("%d / %d", n, n), "ok"}, {"异常", "0", "ok"}}
	case "guard":
		return []summaryItem{{"封禁中", fmt.Sprint(a.demoEnabledBans()), ""}, {"防火墙", "已生效", "ok"}, {"已丢弃", fmt.Sprint(a.demoTotal("guard/ban", 0.01)), ""}}
	case "knock":
		return []summaryItem{{"启用规则", fmt.Sprint(a.demoEnabledCount("knock", "rules")), ""}, {"当前授权", "2", ""}, {"防火墙拦截", "已生效", "ok"}}
	case "waf":
		return []summaryItem{{"实例", fmt.Sprint(len(asMaps(a.demoConfig("waf")["instances"]))), ""}, {"已拦截", fmt.Sprint(a.demoTotal("waf-main/m", 0.002) * 3 / 4), "warn"}}
	case "webservice":
		ls, sites := 0, 0
		var req int64
		for _, l := range asMaps(a.demoConfig("webservice")["listeners"]) {
			if isOn(l, "enable") {
				ls++
				for _, s := range asMaps(l["sites"]) {
					if isOn(s, "enable") {
						sites++
						req += a.demoTotal(str(l, "key")+"/"+str(s, "key"), demoSiteRate(str(l, "key"), str(s, "key")))
					}
				}
			}
		}
		total := len(asMaps(a.demoConfig("webservice")["listeners"]))
		return []summaryItem{{"运行中的监听", fmt.Sprintf("%d / %d", ls, total), "ok"}, {"站点", fmt.Sprint(sites), ""}, {"请求数", fmt.Sprint(req), ""}}
	case "tailscale":
		return []summaryItem{{"状态", "已连接", "ok"}, {"节点", "4", ""}}
	case "netbird":
		return []summaryItem{{"状态", "已连接", "ok"}, {"节点", "2/3", ""}}
	case "frp":
		return []summaryItem{{"客户端", "运行中", "ok"}, {"代理", "3 / 3", ""}, {"服务端", "运行中", "ok"}}
	}
	return nil
}

type summaryItem struct {
	Label string
	Value string
	Level string
}

// demoEnabledBans 返回演示封禁表中未过期的条目数（封禁表在启动时由演示数据装入）。
func (a *App) demoEnabledBans() int {
	b, err := os.ReadFile(filepath.Join(a.opts.ConfigDir, "guard", "bans.json"))
	if err != nil {
		return 0
	}
	var f struct {
		Bans []struct {
			Until time.Time `json:"until"`
		} `json:"bans"`
	}
	if json.Unmarshal(b, &f) != nil {
		return 0
	}
	n := 0
	for _, e := range f.Bans {
		if e.Until.IsZero() || e.Until.After(time.Now()) {
			n++
		}
	}
	return n
}
