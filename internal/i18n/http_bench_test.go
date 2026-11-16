package i18n

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// benchListeners 生成与“200 个站点的监听器列表”规模相当的配置类响应（约 300 KB）。
// han 为 0 时完全不含汉字；1 时有几个站点带中文备注（用户内容，不翻译）；2 时另有几个站点带程序生成的错误说明。
func benchListeners(han int) []byte {
	type site struct {
		Key      string            `json:"key"`
		Name     string            `json:"name"`
		Remark   string            `json:"remark"`
		Enable   bool              `json:"enable"`
		Domains  []string          `json:"domains"`
		Type     string            `json:"type"`
		Targets  []string          `json:"targets"`
		Headers  map[string]string `json:"headers"`
		Error    string            `json:"error"`
		Requests int64             `json:"requests"`
		Bytes    uint64            `json:"bytes"`
		Options  map[string]any    `json:"options"`
	}
	var sites []site
	for i := 0; i < 200; i++ {
		s := site{Key: fmt.Sprintf("%016x", i*7919), Name: fmt.Sprintf("site%d", i), Enable: true, Domains: []string{fmt.Sprintf("s%d.test.local", i)},
			Type: "proxy", Targets: []string{"http://127.0.0.1:19160"}, Headers: map[string]string{"X-Forwarded-Proto": "https", "X-Real-IP": "$remote_addr"},
			Requests: int64(i) * 1000, Bytes: 9007199254740993 + uint64(i),
			Options: map[string]any{"compress": false, "accessLog": true, "timeout": 30, "retries": 2, "padding": strings.Repeat("x", 900)}}
		if han >= 1 && i%40 == 0 {
			s.Remark = "家里的 NAS，走端口转发"
		}
		if han >= 2 && i%50 == 0 {
			s.Error = "端口 8080 已被占用"
		}
		sites = append(sites, s)
	}
	out, _ := json.Marshal(map[string]any{"listeners": []any{map[string]any{"name": "bench", "port": 19170, "running": true, "sites": sites}}})
	return append(out, '\n')
}

func benchFinish(b *testing.B, path string, body []byte) {
	catalogFor("en")
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		rec.Body = &bytes.Buffer{}
		tw := &transWriter{ResponseWriter: rec, lang: "en", path: path, get: true}
		tw.Header().Set("Content-Type", "application/json; charset=utf-8")
		tw.WriteHeader(http.StatusOK)
		_, _ = tw.Write(body)
		tw.finish()
	}
}

// 非简体中文界面轮询配置类接口：响应里没有汉字。
func BenchmarkFinishListenersNoHan(b *testing.B) {
	benchFinish(b, "/api/webservice/listeners", benchListeners(0))
}

// 同上，但有几个站点带中文备注（用户内容，不需要翻译）。
func BenchmarkFinishListenersUserHan(b *testing.B) {
	benchFinish(b, "/api/webservice/listeners", benchListeners(1))
}

// 同上，另有几个站点带程序生成的错误说明（需要翻译）。
func BenchmarkFinishListenersWithErrors(b *testing.B) {
	benchFinish(b, "/api/webservice/listeners", benchListeners(2))
}

// 完全由程序生成的接口（全部字符串都翻译）：状态接口大小的响应。
func BenchmarkFinishStatusFull(b *testing.B) {
	mods := []any{}
	for i := 0; i < 30; i++ {
		mods = append(mods, map[string]any{"id": fmt.Sprintf("m%d", i), "title": "端口转发", "running": true, "detail": "端口 8080 已被占用", "count": i})
	}
	body, _ := json.Marshal(map[string]any{"version": "0.6.6.6", "modules": mods})
	benchFinish(b, "/api/status", body)
}
