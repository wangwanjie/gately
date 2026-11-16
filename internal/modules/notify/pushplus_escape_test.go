package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/events"
)

// PushPlus 正文按 HTML 模板展示：事件中的访客文本（如 WAF 拦截的请求路径）先做 HTML 转义，换行再换成 <br/>。
func TestPushPlusEscapesHTML(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		_, _ = w.Write([]byte(`{"code":200}`))
	}))
	defer srv.Close()
	msg := NewMessage(events.Event{Type: events.WAFBlocked, Title: "WAF 拦截了请求", Level: events.LevelWarn, Time: time.Now(),
		Message: `GET a.test/<img src=x onerror=alert(1)>&"`}, "")
	if err := send(context.Background(), Channel{Kind: KindPushPlus, URL: srv.URL, Secret: "k"}, msg); err != nil {
		t.Fatal(err)
	}
	content, _ := got["content"].(string)
	want := `GET a.test/&lt;img src=x onerror=alert(1)&gt;&amp;&#34;<br/>`
	if len(content) < len(want) || content[:len(want)] != want {
		t.Fatalf("PushPlus 正文应先转义再换行: %q", content)
	}
}
