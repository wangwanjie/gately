// Package geoip 读取 MaxMind 格式（GeoLite2 / DB-IP 等）的国家或城市库，按 IP 查询国家代码。
package geoip

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
)

// maxDBSize 限制下载或上传的数据库大小，城市库约 70MB。
const maxDBSize = 128 << 20

// Info 描述已加载的数据库。
type Info struct {
	Loaded bool   `json:"loaded"`
	Type   string `json:"type,omitempty"`
	// BuildTime 为数据库构建时间
	BuildTime time.Time `json:"buildTime,omitzero"`
	Size      int64     `json:"size,omitempty"`
}

// DB 是线程安全的国家查询库。零值可用，未加载时查询均返回未知。
type DB struct {
	mu   sync.RWMutex
	r    *maxminddb.Reader
	size int64
}

// Default 是进程级共享库，由 Web 服务模块管理，其他模块可直接查询。
var Default = &DB{}

type record struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	// 部分库（如 DB-IP 国家库）使用 registered_country，作为国家缺失时的回退
	Registered struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"registered_country"`
}

// Load 打开数据库文件，成功后替换并关闭旧库。
func (d *DB) Load(path string) error {
	r, err := maxminddb.Open(path)
	if err != nil {
		return fmt.Errorf("读取 GeoIP 数据库失败: %w", err)
	}
	st, _ := os.Stat(path)
	d.mu.Lock()
	old := d.r
	d.r = r
	d.size = 0
	if st != nil {
		d.size = st.Size()
	}
	d.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

// Close 关闭数据库。
func (d *DB) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.r != nil {
		_ = d.r.Close()
		d.r = nil
	}
}

// Info 返回数据库概况。
func (d *DB) Info() Info {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.r == nil {
		return Info{}
	}
	m := d.r.Metadata
	return Info{Loaded: true, Type: m.DatabaseType, BuildTime: time.Unix(int64(m.BuildEpoch), 0), Size: d.size}
}

// Country 返回 IP 所属国家的 ISO 代码（大写）。未加载或未收录时返回空串。
func (d *DB) Country(addr netip.Addr) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.r == nil {
		return ""
	}
	res := d.r.Lookup(addr.Unmap())
	if !res.Found() {
		return ""
	}
	var rec record
	if res.Decode(&rec) != nil {
		return ""
	}
	code := rec.Country.ISOCode
	if code == "" {
		code = rec.Registered.ISOCode
	}
	return strings.ToUpper(code)
}

// IsLocal 判断地址是否为本机或内网地址，这类地址不会出现在国家库中。
func IsLocal(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsUnspecified()
}

// Save 把数据库内容原子写入 path 并校验可读，返回写入的字节数。
// 校验失败时不会覆盖已有文件。
func Save(path string, r io.Reader) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".geoip-*.tmp")
	if err != nil {
		return 0, err
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, io.LimitReader(r, maxDBSize+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, err
	}
	if n > maxDBSize {
		return 0, errors.New("数据库文件过大")
	}
	check, err := maxminddb.Open(tmp.Name())
	if err != nil {
		return 0, errors.New("不是有效的 MaxMind 数据库（.mmdb）")
	}
	_ = check.Close()
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return 0, err
	}
	return n, os.Rename(tmp.Name(), path)
}

// Download 用 client 从 url 下载数据库到 path；client 为 nil 时使用 5 分钟超时的默认客户端。
func Download(client *http.Client, url, path string) (int64, error) {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}
	return Save(path, resp.Body)
}
