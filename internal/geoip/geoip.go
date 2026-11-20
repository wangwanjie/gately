// Package geoip 读取 MaxMind 格式（GeoLite2 / DB-IP 等）的国家或城市库，按 IP 查询国家代码。
package geoip

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
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

	"github.com/wangwanjie/gately/internal/outbound"
)

// maxDBSize 限制下载或上传的数据库（解压后）大小：GeoLite2 城市库约 50MB，DB-IP 城市库约 130MB。
const maxDBSize = 256 << 20

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

// unpack 识别压缩格式：gzip（DB-IP 提供的 .mmdb.gz）解压，tar（MaxMind 官方的 .tar.gz）取出其中的 .mmdb 文件，
// 其他内容原样返回。
func unpack(r io.Reader) (io.Reader, error) {
	br := bufio.NewReader(r)
	if head, _ := br.Peek(2); len(head) == 2 && head[0] == 0x1f && head[1] == 0x8b {
		zr, err := gzip.NewReader(br)
		if err != nil {
			return nil, errors.New("gzip 压缩包损坏")
		}
		br = bufio.NewReaderSize(zr, 1024)
	}
	// tar 头的第 257 字节起为 "ustar"
	if head, _ := br.Peek(262); len(head) == 262 && string(head[257:262]) == "ustar" {
		tr := tar.NewReader(br)
		for {
			h, err := tr.Next()
			if err != nil {
				return nil, errors.New("压缩包中没有 .mmdb 文件")
			}
			if h.Typeflag == tar.TypeReg && strings.HasSuffix(strings.ToLower(h.Name), ".mmdb") {
				return tr, nil
			}
		}
	}
	return br, nil
}

// Save 把数据库内容原子写入 path 并校验可读，返回写入的字节数。内容可以是 .mmdb，
// 也可以是 gzip 压缩的 .mmdb.gz 或含 .mmdb 的 .tar.gz。校验失败时不会覆盖已有文件。
func Save(path string, r io.Reader) (int64, error) {
	r, err := unpack(r)
	if err != nil {
		return 0, err
	}
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
		client = outbound.NewClient(5 * time.Minute)
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
