package i18n

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// serveRaw 以语言 xx 经 transWriter 返回 body，返回客户端收到的响应。setup 可在写出前设置响应头。
func serveRaw(t *testing.T, method, path string, status int, ctype, body string, setup func(http.Header)) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	tw := &transWriter{ResponseWriter: rec, lang: "xx", path: path, get: method == http.MethodGet}
	tw.Header().Set("Content-Type", ctype)
	if setup != nil {
		setup(tw.Header())
	}
	tw.WriteHeader(status)
	_, _ = tw.Write([]byte(body))
	tw.finish()
	return rec
}

const jsonType = "application/json; charset=utf-8"

// 响应里没有汉字时原样写出：字节完全相同，字段顺序、空白、大整数与浮点数的写法都不变，
// Content-Length 也保持处理器设置的值。改造前这里会整体解析再重新编码：键被重新排序、2^53 以上的整数变成浮点数。
func TestMiddlewarePassesThroughWithoutHan(t *testing.T) {
	testCatalog(map[string]string{"监听失败": "listen failed"})
	bodies := []string{
		`{"zeta":1,"alpha":{"b":2,"a":1},"id":9007199254740993,"big":18446744073709551615,"f":1.50,"e":1e3,"s":"<a&b>","u":"caf\u00e9"}` + "\n",
		"{\n  \"listeners\": [ { \"name\": \"bench\", \"port\": 19170, \"error\": \"\" } ]\n}\n",
		`[]`,
		`"plain string"`,
		`{"error":"connection refused"}`,
		// 日文假名、韩文、全角标点都不是汉字
		`{"name":"テスト","title":"테스트","remark":"（）"}`,
	}
	for _, path := range []string{"/api/webservice/listeners", "/api/status"} {
		for _, body := range bodies {
			rec := serveRaw(t, http.MethodGet, path, 200, jsonType, body, func(h http.Header) {
				h.Set("Content-Length", fmt.Sprint(len(body)))
			})
			if rec.Body.String() != body {
				t.Errorf("%s 的响应被改写:\n得到 %q\n应为 %q", path, rec.Body.String(), body)
			}
			if got := rec.Header().Get("Content-Length"); got != fmt.Sprint(len(body)) {
				t.Errorf("%s 的 Content-Length 应保持 %d，得到 %q", path, len(body), got)
			}
		}
	}
	// 错误响应同理
	body := `{"error":"not found","code":404}`
	if rec := serveRaw(t, http.MethodPost, "/api/x", 404, jsonType, body, nil); rec.Body.String() != body || rec.Code != 404 {
		t.Errorf("没有汉字的错误响应被改写: %d %q", rec.Code, rec.Body.String())
	}
	if rec := serveRaw(t, http.MethodPost, "/api/x", 500, "text/plain; charset=utf-8", "boom\n", nil); rec.Body.String() != "boom\n" {
		t.Errorf("没有汉字的文本错误被改写: %q", rec.Body.String())
	}
}

// 历史教训：配置类接口只翻译 messageKeys / stringMessageKeys 登记的字段，用户自己填写的内容绝不能被改写——
// 客户端读出后原样保存，改写过的译文就会覆盖用户的数据。这里要求更严：除了被翻译的那几个字符串，其余字节逐字节不变。
func TestMiddlewareRewritesOnlyMessageFields(t *testing.T) {
	testCatalog(map[string]string{"端口转发": "Port forwarding", "监听失败": "listen failed", "未找到程序": "program not found", "失败": "failed", "映射失败": "mapping failed"})
	in := `{"zeta":"端口转发","id":9007199254740993,"name":"端口转发","remark":"由端口转发 SNI 分流送达","error":"监听失败",` +
		`"sites":[{"remark":"端口转发","details":["监听失败",{"x":"端口转发"}],"state":{"lastError":"监听失败","value":"端口转发","mapping":"UPnP 映射失败"}}],` +
		`"cert":{"mapping":{"dir":"端口转发","script":"echo 失败"}},"firewall":{"note":"失败"},"unsupported":"失败","warnings":[],"hint":null,"label":123,` +
		`"script":"echo \"监听失败\" > /tmp/端口转发","端口转发":"端口转发","title":"a\u7aef\u53e3\u8f6c\u53d1"}` + "\n"
	want := `{"zeta":"端口转发","id":9007199254740993,"name":"端口转发","remark":"由端口转发 SNI 分流送达","error":"listen failed",` +
		`"sites":[{"remark":"端口转发","details":["listen failed",{"x":"Port forwarding"}],"state":{"lastError":"listen failed","value":"端口转发","mapping":"UPnP mapping failed"}}],` +
		`"cert":{"mapping":{"dir":"端口转发","script":"echo 失败"}},"firewall":{"note":"失败"},"unsupported":"failed","warnings":[],"hint":null,"label":123,` +
		`"script":"echo \"监听失败\" > /tmp/端口转发","端口转发":"端口转发","title":"aPort forwarding"}` + "\n"
	for _, path := range []string{"/api/webservice/listeners", "/api/ddns/tasks", "/api/cron/tasks", "/api/history"} {
		rec := serveRaw(t, http.MethodGet, path, 200, jsonType, in, func(h http.Header) { h.Set("Content-Length", fmt.Sprint(len(in))) })
		if got := rec.Body.String(); got != want {
			t.Errorf("%s:\n得到 %s\n应为 %s", path, got, want)
		}
		if rec.Header().Get("Content-Length") != "" {
			t.Errorf("%s: 内容改写后应去掉原来的 Content-Length", path)
		}
	}
	// 完全由程序生成的接口与错误响应翻译全部字符串值，对象的键不翻译
	// （“echo 失败”里的“失败”太短，不做句中替换，保持原样）
	full := strings.NewReplacer(`"zeta":"端口转发"`, `"zeta":"Port forwarding"`, `"name":"端口转发"`, `"name":"Port forwarding"`,
		`"remark":"由端口转发 SNI 分流送达"`, `"remark":"由Port forwarding SNI 分流送达"`, `"remark":"端口转发"`, `"remark":"Port forwarding"`,
		`"value":"端口转发"`, `"value":"Port forwarding"`, `"dir":"端口转发"`, `"dir":"Port forwarding"`,
		`"note":"失败"`, `"note":"failed"`, `"script":"echo \"监听失败\" > /tmp/端口转发"`, `"script":"echo \"listen failed\" \u003e /tmp/Port forwarding"`,
		`"端口转发":"端口转发"`, `"端口转发":"Port forwarding"`).Replace(want)
	if got := serveRaw(t, http.MethodGet, "/api/status", 200, jsonType, in, nil).Body.String(); got != full {
		t.Errorf("状态接口:\n得到 %s\n应为 %s", got, full)
	}
	if got := serveRaw(t, http.MethodPut, "/api/webservice/listeners/x", 400, jsonType, in, nil).Body.String(); got != full {
		t.Errorf("错误响应:\n得到 %s\n应为 %s", got, full)
	}
	// 非 GET 的成功响应不处理
	if got := serveRaw(t, http.MethodPut, "/api/webservice/listeners/x", 200, jsonType, in, nil).Body.String(); got != in {
		t.Errorf("PUT 的成功响应被改写: %s", got)
	}
}

// 不合法的 JSON、被截断的响应一个字节也不改（与改造前解析失败时的处理相同）。
func TestMiddlewareLeavesInvalidJSONAlone(t *testing.T) {
	testCatalog(map[string]string{"监听失败": "listen failed"})
	for _, body := range []string{
		`{"error":"监听失败"`,
		`{"error":"监听失败"} trailing`,
		`{"error":"监听失败"}{"error":"监听失败"}`,
		`{"error":"监听失败","bad":"\x"}`,
		`{"error":"监听失败","bad":tru}`,
		`{"error":"监听失败",}`,
		`{"error" "监听失败"}`,
		`{error:"监听失败"}`,
		`["监听失败"`,
		`{"error":"监听失败` + "\n" + `"}`,
		`{"error":"监听失败","n":01}`,
		`{"error":"监听失败","n":1.}`,
		`监听失败`,
		strings.Repeat("[", 20000) + `"监听失败"` + strings.Repeat("]", 20000),
	} {
		if got := serveRaw(t, http.MethodGet, "/api/status", 200, jsonType, body, nil).Body.String(); got != body {
			t.Errorf("不合法的 JSON 被改写:\n输入 %.80q\n输出 %.80q", body, got)
		}
	}
}

// randomJSON 生成随机的 JSON 值：键与字符串取自 words，嵌套对象、数组、数字、布尔、null 混合。
func randomJSON(rnd *rand.Rand, words []string, depth int) any {
	pick := func() string { return words[rnd.Intn(len(words))] }
	switch k := rnd.Intn(10); {
	case depth <= 0 || k < 4:
		switch rnd.Intn(8) {
		case 0:
			return float64(rnd.Intn(1000))
		case 1:
			return rnd.Intn(2) == 0
		case 2:
			return nil
		default:
			return pick()
		}
	case k < 7:
		m := map[string]any{}
		for i := rnd.Intn(5); i > 0; i-- {
			m[pick()] = randomJSON(rnd, words, depth-1)
		}
		return m
	default:
		a := []any{}
		for i := rnd.Intn(4); i > 0; i-- {
			a = append(a, randomJSON(rnd, words, depth-1))
		}
		return a
	}
}

// 流式改写与改造前“解析成 any、TrDeep / TrFields、重新编码”的结果在语义上完全相同：
// 随机生成的大量 JSON（紧凑、缩进、不转义 HTML 三种写法）解析后逐值比较。
func TestRewriteJSONMatchesTreeTranslation(t *testing.T) {
	testCatalog(map[string]string{"端口转发": "Port forwarding", "监听失败": "listen failed", "失败": "failed",
		"加载 %s 失败: %v": "failed to load %[1]s: %[2]v", "含 <b> & \"引号\" 的文案": "text with <b> & \"quotes\"", "换行\n文案": "multi\nline"})
	words := []string{"error", "errors", "lastError", "detail", "details", "message", "reason", "warnings", "hint", "title", "label", "issues",
		"mapping", "unsupported", "firewall", "name", "remark", "group", "sites", "state", "value", "x",
		"端口转发", "监听失败", "失败", "加载 a.json 失败: 监听失败", "用户写的备注：端口转发很好用", "含 <b> & \"引号\" 的文案", "换行\n文案", "plain", "", "a\\b\"c", "\u2028 é 😀"}
	rnd := rand.New(rand.NewSource(7))
	changedCount := 0
	for i := 0; i < 3000; i++ {
		v := randomJSON(rnd, words, 5)
		var src []byte
		switch i % 3 {
		case 0:
			src, _ = json.Marshal(v)
		case 1:
			src, _ = json.MarshalIndent(v, "", "  ")
		default:
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			_ = enc.Encode(v)
			src = buf.Bytes()
		}
		for _, deep := range []bool{false, true} {
			var tree any
			if err := json.Unmarshal(src, &tree); err != nil {
				t.Fatal(err)
			}
			if deep {
				tree = TrDeep("xx", tree)
			} else {
				tree = TrFields("xx", tree)
			}
			out, changed := rewriteJSON("xx", src, deep)
			if !changed {
				out = src
			} else {
				changedCount++
			}
			var got any
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("改写后不是合法 JSON: %v\n输入 %s\n输出 %s", err, src, out)
			}
			if !reflect.DeepEqual(got, tree) {
				t.Fatalf("deep=%v 时结果与整树翻译不同:\n输入 %s\n输出 %s", deep, src, out)
			}
			if !changed && mayHaveHanJSON(src) {
				// 没有改动时必须真的没有可翻译的内容
				var orig any
				_ = json.Unmarshal(src, &orig)
				if !reflect.DeepEqual(orig, tree) {
					t.Fatalf("漏翻: %s", src)
				}
			}
		}
	}
	if changedCount < 1000 {
		t.Fatalf("随机样本里只有 %d 个发生了改写，样本没有覆盖到翻译路径", changedCount)
	}
}
