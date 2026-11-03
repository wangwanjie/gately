package webservice

import (
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/geoip"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/proxyx"
)

type banRequest struct {
	// Key 为 IP 地址；解除封禁时也可使用列表返回的键（IPv6 为 /64 网段）
	Key string `json:"key"`
	// Seconds 为手动封禁时长，0 表示 24 小时，-1 表示永久，其余最长 30 天
	Seconds int `json:"seconds"`
}

type geoipDownload struct {
	URL string `json:"url"`
	// Proxy 为下载使用的代理：留空遵循环境变量，update 为全局更新代理，也可以填写代理地址
	Proxy string `json:"proxy,omitempty"`
}

func (m *Module) handleListBans(w http.ResponseWriter, _ *http.Request) {
	httpx.OK(w, m.auth.listBans())
}

func (m *Module) handleAddBan(w http.ResponseWriter, r *http.Request) {
	var req banRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(req.Key))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "IP 地址无效")
		return
	}
	if req.Seconds < -1 || time.Duration(req.Seconds)*time.Second > maxBanDuration {
		httpx.Error(w, http.StatusBadRequest, "封禁时长不能超过 30 天（-1 表示永久）")
		return
	}
	// -1 原样传给 ban 表示永久封禁，0 沿用 24 小时
	d := time.Duration(req.Seconds) * time.Second
	if req.Seconds == 0 {
		d = 24 * time.Hour
	}
	if p := httpx.ClientIP(r); p != "" && limitKey(p) == limitKey(addr.Unmap().String()) {
		httpx.Error(w, http.StatusBadRequest, "不能封禁你当前使用的地址")
		return
	}
	// 与从实时连接封禁相同：回环地址与可信代理封禁后会拦截一大批无关访客（永久封禁时后果更重），一律拒绝
	if msg := m.unbannableReason(addr); msg != "" {
		httpx.Error(w, http.StatusBadRequest, "%s", msg)
		return
	}
	key := m.auth.ban(addr.Unmap().String(), d, "手动封禁", true)
	httpx.OK(w, map[string]string{"key": key})
}

func (m *Module) handleRemoveBan(w http.ResponseWriter, r *http.Request) {
	var req banRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	key := strings.TrimSpace(req.Key)
	// 也接受未规范化的 IPv6 单个地址
	if a, err := netip.ParseAddr(key); err == nil {
		key = limitKey(a.String())
	}
	m.auth.unban(key)
	httpx.OK(w, map[string]bool{"ok": true})
}

func (m *Module) geoipPath() string {
	return filepath.Join(m.store.Dir(), "geoip", "country.mmdb")
}

func (m *Module) handleGeoIPInfo(w http.ResponseWriter, _ *http.Request) {
	httpx.OK(w, geoip.Default.Info())
}

// installGeoIP 保存数据库并加载；写入前已校验为有效的 mmdb。
func (m *Module) loadGeoIP(w http.ResponseWriter) {
	if err := geoip.Default.Load(m.geoipPath()); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	m.logs.Logger("GeoIP").Infof("已加载 GeoIP 数据库（%s）", geoip.Default.Info().Type)
	httpx.OK(w, geoip.Default.Info())
}

func (m *Module) handleGeoIPUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 129<<20)
	f, _, err := r.FormFile("file")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "请上传 .mmdb 文件")
		return
	}
	defer f.Close()
	if _, err := geoip.Save(m.geoipPath(), f); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	m.loadGeoIP(w)
}

func (m *Module) handleGeoIPDownload(w http.ResponseWriter, r *http.Request) {
	var req geoipDownload
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	u := strings.TrimSpace(req.URL)
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		httpx.Error(w, http.StatusBadRequest, "下载地址需以 http(s):// 开头")
		return
	}
	if err := proxyx.Validate(req.Proxy); err != nil {
		httpx.Error(w, http.StatusBadRequest, "%v", err)
		return
	}
	if _, err := geoip.Download(proxyx.Client(req.Proxy, 5*time.Minute, false), u, m.geoipPath()); err != nil {
		httpx.Error(w, http.StatusBadGateway, "%v", err)
		return
	}
	m.loadGeoIP(w)
}

func (m *Module) handleGeoIPDelete(w http.ResponseWriter, _ *http.Request) {
	geoip.Default.Close()
	_ = os.Remove(m.geoipPath())
	httpx.OK(w, geoip.Default.Info())
}
