package dnsprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Cloudflare 错误信息带 HTTP 状态码，且 5xx 仍可被识别为可重试、4xx 不重试
func TestCloudflareErrorCarriesHTTPStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":0,"message":"unhandled server error"}]}`))
	}))
	defer srv.Close()
	c := &cloudflare{hc: srv.Client(), endpoint: srv.URL, creds: Credentials{Secret: "t"}}
	err := c.call(context.Background(), "GET", "/zones", nil, nil)
	if err == nil || err.Error() != "Cloudflare: HTTP 500: 0 unhandled server error" {
		t.Fatalf("文案: %v", err)
	}
	if !IsRetryable(err) {
		t.Fatal("500 应可重试")
	}

	srv4 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":9109,"message":"Invalid access token"}]}`))
	}))
	defer srv4.Close()
	c.endpoint, c.hc = srv4.URL, srv4.Client()
	err = c.call(context.Background(), "GET", "/zones", nil, nil)
	if err == nil || IsRetryable(err) {
		t.Fatalf("403 不应重试: %v", err)
	}
}
