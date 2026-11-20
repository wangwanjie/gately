package geoip

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

// buildDB 生成含两个网段的测试库：1.0.0.0/24 → AU，2a00:1450::/32 → JP（使用 registered_country 回退）。
func buildDB(t *testing.T) []byte {
	t.Helper()
	w, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: "Test-Country", RecordSize: 24})
	if err != nil {
		t.Fatal(err)
	}
	_, n1, _ := net.ParseCIDR("1.0.0.0/24")
	if err := w.Insert(n1, mmdbtype.Map{"country": mmdbtype.Map{"iso_code": mmdbtype.String("au")}}); err != nil {
		t.Fatal(err)
	}
	_, n2, _ := net.ParseCIDR("2a00:1450::/32")
	if err := w.Insert(n2, mmdbtype.Map{"registered_country": mmdbtype.Map{"iso_code": mmdbtype.String("JP")}}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := w.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestLookupAndFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.mmdb")
	if _, err := Save(path, bytes.NewReader(buildDB(t))); err != nil {
		t.Fatal(err)
	}
	d := &DB{}
	if d.Country(netip.MustParseAddr("1.0.0.5")) != "" {
		t.Fatal("未加载时应返回空")
	}
	if err := d.Load(path); err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	cases := map[string]string{
		"1.0.0.5":        "AU",
		"::ffff:1.0.0.9": "AU",
		"2a00:1450::1":   "JP",
		"8.8.8.8":        "",
	}
	for ip, want := range cases {
		if got := d.Country(netip.MustParseAddr(ip)); got != want {
			t.Errorf("%s: got %q want %q", ip, got, want)
		}
	}
	if info := d.Info(); !info.Loaded || info.Type != "Test-Country" {
		t.Fatalf("Info 不符: %+v", info)
	}
}

func TestSaveRejectsInvalidAndKeepsOld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.mmdb")
	good := buildDB(t)
	if _, err := Save(path, bytes.NewReader(good)); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(path, bytes.NewReader([]byte("garbage"))); err == nil {
		t.Fatal("无效文件应被拒绝")
	}
	data, _ := os.ReadFile(path)
	if !bytes.Equal(data, good) {
		t.Fatal("校验失败不应覆盖旧文件")
	}
}

func TestIsLocal(t *testing.T) {
	for ip, want := range map[string]bool{"127.0.0.1": true, "192.168.1.1": true, "fd00::1": true, "8.8.8.8": false, "::ffff:10.0.0.1": true} {
		if IsLocal(netip.MustParseAddr(ip)) != want {
			t.Errorf("IsLocal(%s) != %v", ip, want)
		}
	}
}

func TestSaveCompressed(t *testing.T) {
	raw := buildDB(t)
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = zw.Write(raw)
	_ = zw.Close()

	var tgz bytes.Buffer
	zw = gzip.NewWriter(&tgz)
	tw := tar.NewWriter(zw)
	readme := []byte("readme")
	_ = tw.WriteHeader(&tar.Header{Name: "GeoLite2-City_20261118/README.txt", Mode: 0o644, Size: int64(len(readme)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(readme)
	_ = tw.WriteHeader(&tar.Header{Name: "GeoLite2-City_20261118/GeoLite2-City.mmdb", Mode: 0o644, Size: int64(len(raw)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(raw)
	_ = tw.Close()
	_ = zw.Close()

	var noMMDB bytes.Buffer
	zw = gzip.NewWriter(&noMMDB)
	tw = tar.NewWriter(zw)
	_ = tw.WriteHeader(&tar.Header{Name: "README.txt", Mode: 0o644, Size: int64(len(readme)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(readme)
	_ = tw.Close()
	_ = zw.Close()

	for name, data := range map[string][]byte{"gz": gz.Bytes(), "tar.gz": tgz.Bytes()} {
		path := filepath.Join(t.TempDir(), "c.mmdb")
		n, err := Save(path, bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got, _ := os.ReadFile(path)
		if n != int64(len(raw)) || !bytes.Equal(got, raw) {
			t.Fatalf("%s: 解压结果不符", name)
		}
	}
	if _, err := Save(filepath.Join(t.TempDir(), "c.mmdb"), bytes.NewReader(noMMDB.Bytes())); err == nil {
		t.Fatal("不含 .mmdb 的压缩包应被拒绝")
	}
	if _, err := Save(filepath.Join(t.TempDir(), "c.mmdb"), bytes.NewReader([]byte{0x1f, 0x8b, 0, 0})); err == nil {
		t.Fatal("损坏的 gzip 应被拒绝")
	}
}
