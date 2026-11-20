package app

import (
	"net/http"
	"net/netip"
	"strings"

	"github.com/wangwanjie/gately/internal/geoip"
	"github.com/wangwanjie/gately/internal/httpx"
)

// maxGeoLookup 限制一次查询的地址数量。
const maxGeoLookup = 200

// geoLookupResult 是批量查询 IP 归属地的结果。
type geoLookupResult struct {
	// Loaded 为是否已导入 GeoIP 数据库，City 为是否为城市库
	Loaded bool `json:"loaded"`
	City   bool `json:"city,omitempty"`
	// Items 按查询时的地址索引；内网与本机地址的国家为 LAN，库中未收录的地址不出现
	Items map[string]geoip.Location `json:"items"`
}

// handleGeoLookup 批量查询 IP 归属地。地址由 ip 参数给出（可重复，也可用逗号分隔），
// 可带端口或前缀长度（如 [2001:db8::1]:443、203.0.113.0/24，按其中的地址查询）；lang 为界面语言。
func (a *App) handleGeoLookup(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var ips []string
	for _, v := range q["ip"] {
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				ips = append(ips, s)
			}
		}
	}
	if len(ips) > maxGeoLookup {
		httpx.Error(w, http.StatusBadRequest, "一次最多查询 %d 个地址", maxGeoLookup)
		return
	}
	info := geoip.Default.Info()
	res := geoLookupResult{Loaded: info.Loaded, City: info.City, Items: make(map[string]geoip.Location, len(ips))}
	lang := q.Get("lang")
	for _, s := range ips {
		addr, ok := parseLookupAddr(s)
		if !ok {
			continue
		}
		if geoip.IsLocal(addr) {
			res.Items[s] = geoip.Location{Country: "LAN"}
			continue
		}
		if !info.Loaded {
			continue
		}
		if loc := geoip.Default.Lookup(addr, lang); loc.Country != "" {
			res.Items[s] = loc
		}
	}
	httpx.OK(w, res)
}

// parseLookupAddr 从 IP、IP:端口、[IPv6]:端口 或 CIDR 中取出地址，IPv6 区域标识被忽略。
func parseLookupAddr(s string) (netip.Addr, bool) {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p.Addr(), true
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().WithZone(""), true
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, false
	}
	return a.WithZone(""), true
}
