package webservice

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// 缓存命中的响应带 Content-Length，不走分块传输；后端原本分块返回、开启压缩时同样正确。
func TestCacheHitContentLength(t *testing.T) {
	body := strings.Repeat("cached body line\n", 200)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Cache-Control", "max-age=60")
		// 分两次写出并刷新，后端以分块传输返回（没有 Content-Length）
		_, _ = io.WriteString(w, body[:100])
		http.NewResponseController(w).Flush()
		_, _ = io.WriteString(w, body[100:])
	}))
	defer backend.Close()
	for _, compress := range []bool{false, true} {
		site := Site{Enable: true, Domains: []string{"c.test"}, Targets: []string{backend.URL}, Compress: compress, Cache: CacheConfig{Enable: true, TTL: 60}}
		if err := site.validateProxyExtras(); err != nil {
			t.Fatal(err)
		}
		addr, _ := startRunner(t, Listener{Sites: []Site{site}}, nil, nil)
		tr := &http.Transport{DisableCompression: true}
		fetch := func(enc string) *http.Response {
			t.Helper()
			req, _ := http.NewRequest("GET", "http://"+addr+"/doc", nil)
			req.Host = "c.test"
			if enc != "" {
				req.Header.Set("Accept-Encoding", enc)
			}
			resp, err := tr.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			return resp
		}
		miss := fetch("")
		b, _ := io.ReadAll(miss.Body)
		miss.Body.Close()
		if miss.Header.Get("X-Cache") != "MISS" || string(b) != body || len(miss.TransferEncoding) == 0 {
			t.Fatalf("首次应为分块传输的 MISS: %s %v", miss.Header.Get("X-Cache"), miss.TransferEncoding)
		}
		hit := fetch("")
		b, _ = io.ReadAll(hit.Body)
		hit.Body.Close()
		if hit.Header.Get("X-Cache") != "HIT" || string(b) != body {
			t.Fatalf("第二次应命中缓存: %s", hit.Header.Get("X-Cache"))
		}
		if hit.ContentLength != int64(len(body)) || len(hit.TransferEncoding) != 0 || hit.Header.Get("Content-Length") != strconv.Itoa(len(body)) {
			t.Fatalf("命中缓存应带 Content-Length（压缩=%v）: %d %v", compress, hit.ContentLength, hit.TransferEncoding)
		}
		if compress {
			// 开启压缩时，接受 gzip 的访客拿到压缩后的内容，长度不能沿用未压缩的 Content-Length
			gz := fetch("gzip")
			zb, _ := io.ReadAll(gz.Body)
			gz.Body.Close()
			if gz.Header.Get("Content-Encoding") != "gzip" || gz.ContentLength == int64(len(body)) || (gz.ContentLength >= 0 && gz.ContentLength != int64(len(zb))) {
				t.Fatalf("压缩后的长度错误: enc=%q cl=%d 实际 %d", gz.Header.Get("Content-Encoding"), gz.ContentLength, len(zb))
			}
		}
		tr.CloseIdleConnections()
	}
}
