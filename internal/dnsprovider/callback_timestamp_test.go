package dnsprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCallbackTimestampVariable(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("t") + "|" + r.URL.Query().Get("u")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	p, err := New("callback", Credentials{Extra: map[string]string{"url": srv.URL + "/?t={timestamp}&u=#{timestamp}"}}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	before := time.Now().Unix()
	if _, err := p.Upsert(context.Background(), Record{Zone: "example.com", Sub: "www", Type: "A", Value: "1.1.1.1"}); err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(got, "|")
	if len(parts) != 2 || parts[0] != parts[1] {
		t.Fatalf("两种写法应得到同一个时间戳: %q", got)
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || ts < before || ts > before+5 {
		t.Fatalf("时间戳不合理: %q", got)
	}
}
