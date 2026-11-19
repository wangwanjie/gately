package ssl

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wangwanjie/gately/internal/httpx"
)

// fnosCertIndex 是飞牛 fnOS 登记系统证书的文件（设置 → 安全性 → 证书中申请或导入的证书）。
// 证书文件位于 /usr/trim/var/trim_connect/ssls/<域名>/<时间戳>/，每次续期换新目录，
// 所以按域名从这里查当前路径，而不是记录固定路径。测试时替换。
var fnosCertIndex = "/usr/trim/etc/network_cert_all.conf"

// fnosIndexLimit 限制登记文件的读取大小，正常只有几 KB。
const fnosIndexLimit = 4 << 20

// FnOSCert 是飞牛系统中登记的一张证书。
type FnOSCert struct {
	Domain     string   `json:"domain"`
	SAN        []string `json:"san"`
	CertFile   string   `json:"certificate"`
	Fullchain  string   `json:"fullchain"`
	PrivateKey string   `json:"privateKey"`
	// ValidFrom / ValidTo 为毫秒时间戳
	ValidFrom int64 `json:"validFrom"`
	ValidTo   int64 `json:"validTo"`
	// Used 表示飞牛系统自身正在使用这张证书
	Used bool `json:"used"`
}

// chainFile 返回包含中间证书的证书文件：有 fullchain 时用它，否则用 certificate。
func (c FnOSCert) chainFile() string {
	if c.Fullchain != "" {
		return c.Fullchain
	}
	return c.CertFile
}

// readFnOSCerts 读取飞牛系统登记的证书列表；不是飞牛系统（或容器未挂载）时返回 os.ErrNotExist。
func readFnOSCerts() ([]FnOSCert, error) {
	f, err := os.Open(fnosCertIndex)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, fnosIndexLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > fnosIndexLimit {
		return nil, fmt.Errorf("%s 过大", fnosCertIndex)
	}
	var list []FnOSCert
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", fnosCertIndex, err)
	}
	return list, nil
}

// findFnOSCert 按域名查找飞牛系统证书，域名不区分大小写。
func findFnOSCert(domain string) (FnOSCert, error) {
	list, err := readFnOSCerts()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FnOSCert{}, fmt.Errorf("找不到飞牛系统证书列表 %s（Docker 部署需只读挂载该文件与 /usr/trim/var/trim_connect/ssls）", fnosCertIndex)
		}
		return FnOSCert{}, err
	}
	for _, c := range list {
		if strings.EqualFold(c.Domain, domain) {
			if !filepath.IsAbs(c.chainFile()) || !filepath.IsAbs(c.PrivateKey) {
				return FnOSCert{}, fmt.Errorf("飞牛系统证书 %s 的文件路径无效", domain)
			}
			return c, nil
		}
	}
	return FnOSCert{}, fmt.Errorf("飞牛系统中没有域名为 %s 的证书", domain)
}

// handleFnOSCerts 列出飞牛系统证书，供添加证书时选择。
func (m *Module) handleFnOSCerts(w http.ResponseWriter, r *http.Request) {
	type item struct {
		Domain   string    `json:"domain"`
		SAN      []string  `json:"san"`
		NotAfter time.Time `json:"notAfter,omitzero"`
		Used     bool      `json:"used"`
	}
	out := map[string]any{"available": false, "certs": []item{}}
	list, err := readFnOSCerts()
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		out["available"] = true
		out["error"] = err.Error()
	default:
		out["available"] = true
		items := make([]item, 0, len(list))
		for _, c := range list {
			it := item{Domain: c.Domain, SAN: c.SAN, Used: c.Used}
			if c.ValidTo > 0 {
				it.NotAfter = time.UnixMilli(c.ValidTo).UTC()
			}
			items = append(items, it)
		}
		out["certs"] = items
	}
	httpx.OK(w, out)
}
