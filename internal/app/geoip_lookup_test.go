package app

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"

	"github.com/wangwanjie/gately/internal/geoip"
)

// loadTestCityDB 把只含 1.0.0.0/24（中国广东广州）的城市库加载到 geoip.Default，测试结束后关闭。
func loadTestCityDB(t *testing.T) {
	t.Helper()
	w, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: "Test-City", RecordSize: 24})
	if err != nil {
		t.Fatal(err)
	}
	_, n, _ := net.ParseCIDR("1.0.0.0/24")
	if err := w.Insert(n, mmdbtype.Map{
		"country":      mmdbtype.Map{"iso_code": mmdbtype.String("CN")},
		"subdivisions": mmdbtype.Slice{mmdbtype.Map{"names": mmdbtype.Map{"en": mmdbtype.String("Guangdong"), "zh-CN": mmdbtype.String("广东")}}},
		"city":         mmdbtype.Map{"names": mmdbtype.Map{"en": mmdbtype.String("Guangzhou"), "zh-CN": mmdbtype.String("广州")}},
	}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := w.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "city.mmdb")
	if _, err := geoip.Save(path, &buf); err != nil {
		t.Fatal(err)
	}
	if err := geoip.Default.Load(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(geoip.Default.Close)
}

func decodeLookup(t *testing.T, body []byte) geoLookupResult {
	t.Helper()
	var res geoLookupResult
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("解析结果失败: %v %s", err, body)
	}
	return res
}

func TestGeoLookup(t *testing.T) {
	_, srv, admin := loginApp(t)
	url := srv.URL + "/api/geoip/lookup?lang=zh-CN&ip=1.0.0.1&ip=192.168.1.2,[::1]:443&ip=1.0.0.0/24&ip=8.8.8.8&ip=bad"

	// 未导入数据库时内网地址仍标为 LAN
	code, body := doJSON(t, admin, "GET", url, nil)
	if code != 200 {
		t.Fatalf("查询失败 %d %s", code, body)
	}
	if res := decodeLookup(t, body); res.Loaded || len(res.Items) != 2 || res.Items["192.168.1.2"].Country != "LAN" || res.Items["[::1]:443"].Country != "LAN" {
		t.Fatalf("未导入数据库时结果不符: %s", body)
	}

	loadTestCityDB(t)
	code, body = doJSON(t, admin, "GET", url, nil)
	res := decodeLookup(t, body)
	if code != 200 || !res.Loaded || !res.City {
		t.Fatalf("查询失败 %d %s", code, body)
	}
	want := geoip.Location{Country: "CN", Region: "广东", City: "广州"}
	if res.Items["1.0.0.1"] != want || res.Items["1.0.0.0/24"] != want {
		t.Fatalf("城市库结果不符: %s", body)
	}
	if _, ok := res.Items["8.8.8.8"]; ok {
		t.Fatalf("未收录的地址不应出现: %s", body)
	}
	if _, ok := res.Items["bad"]; ok {
		t.Fatalf("无效地址不应出现: %s", body)
	}
	_, body = doJSON(t, admin, "GET", srv.URL+"/api/geoip/lookup?lang=en&ip=1.0.0.1", nil)
	if got := decodeLookup(t, body).Items["1.0.0.1"]; got.Region != "Guangdong" || got.City != "Guangzhou" {
		t.Fatalf("英文名称不符: %s", body)
	}

	// 数量上限
	many := strings.Repeat("1.0.0.1,", maxGeoLookup) + "1.0.0.2"
	if code, _ := doJSON(t, admin, "GET", srv.URL+"/api/geoip/lookup?ip="+many, nil); code != http.StatusBadRequest {
		t.Fatalf("超过上限应返回 400，实际 %d", code)
	}

	// 只读账号可查询，未登录不可
	if code, body := doJSON(t, admin, "POST", srv.URL+"/api/account/users", map[string]string{"username": "viewer", "password": "tide-Lamp-47-oak"}); code != 200 {
		t.Fatalf("创建只读账号失败 %d %s", code, body)
	}
	jar, _ := cookiejar.New(nil)
	ro := &http.Client{Jar: jar}
	if code, body := doJSON(t, ro, "GET", url, nil); code != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401，实际 %d %s", code, body)
	}
	if code, body := doJSON(t, ro, "POST", srv.URL+"/api/login", map[string]string{"username": "viewer", "password": "tide-Lamp-47-oak"}); code != 200 {
		t.Fatalf("只读账号登录失败 %d %s", code, body)
	}
	if code, body := doJSON(t, ro, "GET", url, nil); code != 200 || decodeLookup(t, body).Items["1.0.0.1"] != want {
		t.Fatalf("只读账号应能查询: %d %s", code, body)
	}
}
