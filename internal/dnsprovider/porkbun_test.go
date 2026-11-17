package dnsprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakePorkbun 模拟 Porkbun 接口的行为（issue #99）：
// 保存记录时名称转成小写；retrieveByNameType 按名称区分大小写匹配；
// 同名同类型同值的记录已存在时 create 返回 400 与 “A DNS record with this name, type, and content already exists.”。
type fakePorkbun struct {
	mu   sync.Mutex
	recs []porkbunRecord
	// hideByName 让 retrieveByNameType 总是返回空，模拟按名称查不到已有记录的其他情况
	hideByName bool
	// hideList 让 retrieve（列出整个域名）也返回空
	hideList bool
	paths    []string
}

func (f *fakePorkbun) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := readJSON(t, r)
		if body["apikey"] != "k" || body["secretapikey"] != "s" {
			t.Errorf("凭据错误: %v", body)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.paths = append(f.paths, r.URL.Path)
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/dns/"), "/")
		write := func(v any) {
			_ = json.NewEncoder(w).Encode(v)
		}
		fqdn := func(zone, sub string) string {
			if sub == "" {
				return zone
			}
			return sub + "." + zone
		}
		switch parts[0] {
		case "retrieveByNameType":
			out := []porkbunRecord{}
			sub := ""
			if len(parts) > 3 {
				sub = parts[3]
			}
			if !f.hideByName {
				for _, rec := range f.recs {
					// 区分大小写：带大写字母的主机记录查不到小写保存的记录
					if rec.Name == fqdn(parts[1], sub) && rec.Type == parts[2] {
						out = append(out, rec)
					}
				}
			}
			write(map[string]any{"status": "SUCCESS", "records": out})
		case "retrieve":
			out := f.recs
			if f.hideList {
				out = []porkbunRecord{}
			}
			write(map[string]any{"status": "SUCCESS", "records": out})
		case "create":
			name := strings.ToLower(fqdn(parts[1], body["name"].(string)))
			for _, rec := range f.recs {
				if rec.Name == name && rec.Type == body["type"] && rec.Content == body["content"] {
					w.WriteHeader(http.StatusBadRequest)
					write(map[string]any{"status": "ERROR", "message": "A DNS record with this name, type, and content already exists."})
					return
				}
			}
			f.recs = append(f.recs, porkbunRecord{ID: "n" + name, Name: name, Type: body["type"].(string), Content: body["content"].(string), TTL: body["ttl"].(string)})
			write(map[string]any{"status": "SUCCESS"})
		case "editByNameType":
			sub := ""
			if len(parts) > 3 {
				sub = parts[3]
			}
			for i, rec := range f.recs {
				if rec.Name == fqdn(parts[1], sub) && rec.Type == parts[2] {
					f.recs[i].Content = body["content"].(string)
				}
			}
			write(map[string]any{"status": "SUCCESS"})
		case "delete":
			for i, rec := range f.recs {
				if rec.ID == parts[2] {
					f.recs = append(f.recs[:i], f.recs[i+1:]...)
					break
				}
			}
			write(map[string]any{"status": "SUCCESS"})
		default:
			t.Errorf("意外路径 %s", r.URL.Path)
		}
	}
}

func newFakePorkbun(t *testing.T, f *fakePorkbun) Provider {
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	p, _ := New("porkbun", Credentials{ID: "k", Secret: "s"}, srv.Client())
	p.(*porkbun).endpoint = srv.URL
	return p
}

// 主机记录带大写字母：已有记录（小写保存）应识别为无需更新，不再每轮创建报错。
func TestPorkbunMixedCaseSub(t *testing.T) {
	f := &fakePorkbun{recs: []porkbunRecord{{ID: "1", Name: "nas.example.xyz", Type: "AAAA", Content: "2001:db8::1", TTL: "600"}}}
	p := newFakePorkbun(t, f)
	rec := Record{Zone: "example.xyz", Sub: "NAS", Type: "AAAA", Value: "2001:db8::1"}
	for i := range 3 {
		got, err := p.Upsert(context.Background(), rec)
		if err != nil || got != ActionUnchanged {
			t.Fatalf("第 %d 轮: got %q err %v，应无需更新", i, got, err)
		}
	}
	// 值变化时按小写名称更新已有记录
	rec.Value = "2001:db8::2"
	if got, err := p.Upsert(context.Background(), rec); err != nil || got != ActionUpdated {
		t.Fatalf("got %q err %v，应更新", got, err)
	}
	if len(f.recs) != 1 || f.recs[0].Content != "2001:db8::2" {
		t.Fatalf("记录应原地更新: %+v", f.recs)
	}
}

// 按名称查不到时列出整个域名核对；IPv6 以展开写法返回也视为相同。
func TestPorkbunFallbackListAndIPv6Form(t *testing.T) {
	f := &fakePorkbun{hideByName: true, recs: []porkbunRecord{
		{ID: "1", Name: "example.xyz", Type: "AAAA", Content: "2001:db8::9", TTL: "600"},
		{ID: "2", Name: "nas.example.xyz", Type: "A", Content: "203.0.113.1", TTL: "600"},
		{ID: "3", Name: "nas.example.xyz", Type: "AAAA", Content: "2001:0DB8:0000:0000:0000:0000:0000:0001", TTL: "600"},
	}}
	p := newFakePorkbun(t, f)
	got, err := p.Upsert(context.Background(), Record{Zone: "example.xyz", Sub: "nas", Type: "AAAA", Value: "2001:db8::1"})
	if err != nil || got != ActionUnchanged {
		t.Fatalf("got %q err %v，应无需更新", got, err)
	}
	// 删除时同样能找到这条记录
	n, err := Delete(context.Background(), p, Record{Zone: "example.xyz", Sub: "nas", Type: "AAAA", Value: "2001:db8::1"})
	if err != nil || n != 1 {
		t.Fatalf("删除: n=%d err=%v", n, err)
	}
	if len(f.recs) != 2 {
		t.Fatalf("只应删掉 nas 的 AAAA 记录: %+v", f.recs)
	}
}

// 两次查询都没找到、创建时提示同值记录已存在：按无需更新处理。
func TestPorkbunCreateDuplicateIsUnchanged(t *testing.T) {
	f := &fakePorkbun{}
	p := newFakePorkbun(t, f)
	rec := Record{Zone: "example.xyz", Sub: "nas", Type: "AAAA", Value: "2001:db8::1"}
	if got, err := p.Upsert(context.Background(), rec); err != nil || got != ActionCreated {
		t.Fatalf("首次应创建: %q %v", got, err)
	}
	// 之后让两种查询都看不到这条记录，模拟服务商列表延迟等情况
	f.mu.Lock()
	f.hideByName, f.hideList = true, true
	f.mu.Unlock()
	got, err := p.Upsert(context.Background(), rec)
	if err != nil || got != ActionUnchanged {
		t.Fatalf("got %q err %v，应无需更新", got, err)
	}
	if len(f.recs) != 1 {
		t.Fatalf("不应重复创建: %+v", f.recs)
	}
	// 其他创建错误照常返回
	if !porkbunDuplicate(errString("Porkbun: A DNS record with this name, type, and content already exists.")) ||
		porkbunDuplicate(errString("Porkbun: Invalid domain.")) || porkbunDuplicate(nil) {
		t.Fatal("porkbunDuplicate 判断错误")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
