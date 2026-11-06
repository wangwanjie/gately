package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wangwanjie/gately/internal/module"
)

// getRaw 发出 GET 请求，返回状态码、响应头与响应体。
func getRaw(t *testing.T, c *http.Client, u, lang string) (int, http.Header, []byte) {
	t.Helper()
	req, _ := http.NewRequest("GET", u, nil)
	if lang != "" {
		req.Header.Set("Accept-Language", lang)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, data
}

// sourcesModule 是带配置来源的测试模块，只用于验证来源列表接口。
type sourcesModule struct{ module.Module }

func (sourcesModule) LogSources() []string { return []string{"HTTPS", "", "备用"} }

func TestLogSourcesAPI(t *testing.T) {
	a, srv, c := loginApp(t)
	pf := a.modules[0].logs
	pf.Logger("规则B").Infof("x")
	pf.Logger("规则A").Infof("y")
	// 挤出缓冲后来源仍然列出：来源不依赖当前加载的日志
	pf.Resize(1)
	pf.Logger("").Infof("无来源")
	defer pf.Resize(defaultLogLines)

	code, _, body := getRaw(t, c, srv.URL+"/api/portforward/logs/sources", "zh-CN")
	var list []string
	if code != 200 || json.Unmarshal(body, &list) != nil {
		t.Fatalf("来源接口失败 %d %s", code, body)
	}
	if fmt.Sprint(list) != "[规则A 规则B]" {
		t.Fatalf("来源列表不对: %v", list)
	}
	// 系统日志也有来源列表
	if code, _, body := getRaw(t, c, srv.URL+"/api/logs/sources", "zh-CN"); code != 200 || !bytes.HasPrefix(body, []byte("[")) {
		t.Fatalf("系统日志来源接口失败 %d %s", code, body)
	}

	// 模块配置中的来源与日志中出现过的来源合并、去空、排序
	rec := httptest.NewRecorder()
	a.logSourcesHandler(pf, sourcesModule{}).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	list = nil
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if fmt.Sprint(list) != "[HTTPS 备用 规则A 规则B]" {
		t.Fatalf("应合并配置来源: %v", list)
	}
}
