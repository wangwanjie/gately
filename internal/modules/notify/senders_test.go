package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/i18n"
)

func testMsg() Message {
	return NewMessage(events.Event{Type: events.DDNSChanged, Title: "解析已更新", Message: "1.2.3.4", Level: events.LevelInfo}, "")
}

func TestTelegramSend(t *testing.T) {
	var path string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	ch := Channel{Kind: KindTelegram, URL: srv.URL, Secret: "TOKEN", Target: "42"}
	if err := send(context.Background(), ch, testMsg()); err != nil {
		t.Fatal(err)
	}
	if path != "/botTOKEN/sendMessage" || body["chat_id"] != "42" {
		t.Fatalf("请求不符: %s %v", path, body)
	}
	if !strings.Contains(body["text"].(string), "解析已更新") {
		t.Fatalf("正文缺少标题: %v", body["text"])
	}
}

func TestWeComBusinessError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":93000,"errmsg":"invalid webhook url"}`))
	}))
	defer srv.Close()
	err := send(context.Background(), Channel{Kind: KindWeCom, URL: srv.URL}, testMsg())
	if err == nil {
		t.Fatal("HTTP 200 但 errcode 非 0 应视为失败")
	}
}

func TestDingTalkSignAppended(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"errcode":0}`))
	}))
	defer srv.Close()
	if err := send(context.Background(), Channel{Kind: KindDingTalk, URL: srv.URL + "?access_token=x", Secret: "s"}, testMsg()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "timestamp=") || !strings.Contains(query, "sign=") || !strings.Contains(query, "access_token=x") {
		t.Fatalf("加签参数缺失: %s", query)
	}
}

func TestWebhookChannelUsesVars(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		got = string(data)
	}))
	defer srv.Close()
	ch := Channel{Kind: KindWebhook}
	ch.Webhook.URL = srv.URL
	ch.Webhook.Method = "POST"
	ch.Webhook.Body = `{"t":"{type}","m":"{message}"}`
	if err := send(context.Background(), ch, testMsg()); err != nil {
		t.Fatal(err)
	}
	if got != `{"t":"ddns.changed","m":"1.2.3.4"}` {
		t.Fatalf("模板渲染结果不符: %s", got)
	}
}

func TestNewChannels(t *testing.T) {
	type got struct {
		path, ctype, auth, title, gkey string
		body                           string
	}
	var g got
	respond := `{}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		g = got{path: r.URL.RequestURI(), ctype: r.Header.Get("Content-Type"), auth: r.Header.Get("Authorization"), title: r.Header.Get("Title"), gkey: r.Header.Get("X-Gotify-Key"), body: string(data)}
		_, _ = w.Write([]byte(respond))
	}))
	defer srv.Close()
	msg := testMsg()

	if err := send(context.Background(), Channel{Kind: KindNtfy, URL: srv.URL, Target: "alerts", Secret: "tk_1"}, msg); err != nil {
		t.Fatal(err)
	}
	if g.path != "/alerts" || g.auth != "Bearer tk_1" || g.title == "" || !strings.Contains(g.body, "1.2.3.4") {
		t.Fatalf("ntfy 请求不符: %+v", g)
	}

	if err := send(context.Background(), Channel{Kind: KindGotify, URL: srv.URL, Secret: "APPTOKEN"}, msg); err != nil {
		t.Fatal(err)
	}
	if g.path != "/message" || g.gkey != "APPTOKEN" || strings.Contains(g.path, "APPTOKEN") {
		t.Fatalf("Gotify 令牌应放在请求头: %+v", g)
	}

	if err := send(context.Background(), Channel{Kind: KindPushover, URL: srv.URL, Secret: "app", Target: "usr"}, msg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(g.body, "token=app") || !strings.Contains(g.body, "user=usr") {
		t.Fatalf("Pushover 表单不符: %+v", g)
	}

	for _, k := range []string{KindDiscord, KindSlack} {
		if err := send(context.Background(), Channel{Kind: k, URL: srv.URL}, msg); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(g.body, "解析已更新") {
			t.Fatalf("%s 正文缺少标题: %s", k, g.body)
		}
	}

	respond = `{"code":40001,"message":"bad key"}`
	if err := send(context.Background(), Channel{Kind: KindServerChan, URL: srv.URL, Secret: "k"}, msg); err == nil {
		t.Fatal("Server酱业务码非 0 应视为失败")
	}
	respond = `{"code":999,"msg":"bad"}`
	if err := send(context.Background(), Channel{Kind: KindPushPlus, URL: srv.URL, Secret: "k"}, msg); err == nil {
		t.Fatal("PushPlus 业务码非 200 应视为失败")
	}
	respond = `{"code":200}`
	if err := send(context.Background(), Channel{Kind: KindPushPlus, URL: srv.URL, Secret: "k"}, msg); err != nil {
		t.Fatal(err)
	}
}

func TestNewChannelValidation(t *testing.T) {
	bad := []Channel{
		{Name: "a", Kind: KindNtfy},
		{Name: "a", Kind: KindGotify, Secret: "x"},
		{Name: "a", Kind: KindPushover, Secret: "x"},
		{Name: "a", Kind: KindDiscord},
		{Name: "a", Kind: KindServerChan},
	}
	for _, c := range bad {
		if c.Validate() == nil {
			t.Errorf("%s 缺少必填项应校验失败", c.Kind)
		}
	}
	ok := Channel{Name: "a", Kind: KindNtfy, Target: "t"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
}

// 非中文时，程序生成的来源（“管理后台”）也要翻译，用户填写的来源名称保持原样。
func TestNewMessageTranslatesSource(t *testing.T) {
	i18n.SetServerLang("en")
	t.Cleanup(func() { i18n.SetServerLang("") })
	msg := NewMessage(events.Event{Type: events.LoginSuccessful, Source: "管理后台", Level: events.LevelInfo, Title: "管理后台登录成功"}, "")
	if !strings.Contains(msg.Text, "Source: Admin console") {
		t.Fatalf("来源未翻译: %q", msg.Text)
	}
	if strings.ContainsFunc(msg.Title+msg.Text, func(r rune) bool { return r >= 0x4e00 && r <= 0x9fff }) {
		t.Fatalf("英文通知中不应残留中文: %q %q", msg.Title, msg.Text)
	}
	msg = NewMessage(events.Event{Type: events.BackupDone, Source: "家里的配置备份", Level: events.LevelInfo, Title: "配置备份完成"}, "")
	if !strings.Contains(msg.Text, "家里的配置备份") {
		t.Fatalf("用户填写的来源应保持原样: %q", msg.Text)
	}
	i18n.SetServerLang("zh-CN")
	msg = NewMessage(events.Event{Type: events.LoginSuccessful, Source: "管理后台", Level: events.LevelInfo, Title: "管理后台登录成功"}, "")
	if msg.Title != "[Gately] 管理后台登录成功" || !strings.Contains(msg.Text, "来源：管理后台") {
		t.Fatalf("中文通知内容不对: %q %q", msg.Title, msg.Text)
	}
}
