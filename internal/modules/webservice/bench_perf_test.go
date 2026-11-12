package webservice

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/wangwanjie/gately/internal/visitors"
)

// benchProxyHandler 构造反向代理到本机后端的站点处理器，用于基准测试整条请求路径。
func benchProxyHandler(b *testing.B, accessLog, visits bool) *siteHandler {
	b.Helper()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	b.Cleanup(backend.Close)
	auth := newAuthenticator("secret")
	if visits {
		auth.visits = visitors.New(filepath.Join(b.TempDir(), "v.jsonl.gz"), nil)
	}
	s := Site{Key: "s1", Enable: true, Domains: []string{"a.test"}, Type: TypeProxy, Targets: []string{backend.URL}, AccessLog: accessLog}
	if err := s.validate(false); err != nil {
		b.Fatal(err)
	}
	h, err := newSiteHandler(s, testLogs().Logger("t"), auth, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(h.closeIdle)
	return h
}

func benchServe(b *testing.B, h http.Handler) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			i++
			r := httptest.NewRequest("GET", "http://a.test/index.html?x=1", nil)
			r.RemoteAddr = "203.0.113." + strconv.Itoa(i%200) + ":5000"
			r.Header.Set("User-Agent", "bench/1.0")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 200 {
				b.Fatalf("状态码 %d", w.Code)
			}
		}
	})
}

func BenchmarkProxyPlain(b *testing.B)     { benchServe(b, benchProxyHandler(b, false, false)) }
func BenchmarkProxyAccessLog(b *testing.B) { benchServe(b, benchProxyHandler(b, true, false)) }
func BenchmarkProxyVisits(b *testing.B)    { benchServe(b, benchProxyHandler(b, true, true)) }
