package luckyimport_test

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/module"
	"github.com/wangwanjie/gately/internal/modules/portforward"
	"github.com/wangwanjie/gately/internal/modules/stun"
)

// loadSampleDocs 读取一个 Lucky 样本（整站备份 zip 或解压后的目录），返回 文件名 -> 文档。
func loadSampleDocs(t *testing.T, path string) map[string]luckyimport.Document {
	t.Helper()
	var entries []luckyimport.Entry
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		files, _ := filepath.Glob(filepath.Join(path, "*.lkcf"))
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			entries = append(entries, luckyimport.Entry{Name: filepath.Base(f), Data: data})
		}
	} else {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if entries, err = luckyimport.ExpandArchive(filepath.Base(path), data); err != nil {
			t.Fatal(err)
		}
	}
	out := map[string]luckyimport.Document{}
	for _, e := range entries {
		doc, err := luckyimport.ParseAny(e.Data)
		if err != nil {
			t.Fatalf("%s: %v", e.Name, err)
		}
		out[filepath.Base(e.Name)] = doc
	}
	return out
}

type router struct {
	mux    *http.ServeMux
	prefix string
}

func (r router) HandleFunc(p string, h http.HandlerFunc) {
	m, path, _ := strings.Cut(p, " ")
	r.mux.HandleFunc(m+" "+r.prefix+path, h)
}

// setSafeMode 把规则列表中每条规则的 Options.SafeMode 改为 mode，其余字段保持样本原样。
func setSafeMode(t *testing.T, raw json.RawMessage, mode string) json.RawMessage {
	t.Helper()
	var rules []map[string]any
	if err := json.Unmarshal(raw, &rules); err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		opts, _ := r["Options"].(map[string]any)
		if opts == nil {
			opts = map[string]any{}
			r["Options"] = opts
		}
		opts["SafeMode"] = mode
	}
	b, _ := json.Marshal(rules)
	return b
}

// injectIPFilter 在样本的 IP 过滤规则里追加 txt 子规则（真实样本的名单均为空），其余字段保持原样。
func injectIPFilter(t *testing.T, doc luckyimport.Document) luckyimport.Document {
	t.Helper()
	var rules []map[string]any
	if err := json.Unmarshal(doc["RuleList"], &rules); err != nil {
		t.Fatal(err)
	}
	add := map[string]string{
		"whitelist":       "192.168.18.0/24\n2001:db8::/32 # 公司\n10.0.0.1-10.0.0.9",
		"blacklist":       "203.0.113.9",
		"globalblacklist": "198.51.100.7",
	}
	for _, r := range rules {
		if text, ok := add[r["Key"].(string)]; ok {
			subs, _ := r["SubRuleList"].([]any)
			r["SubRuleList"] = append(subs, map[string]any{"Key": "t", "Remark": "test", "Enable": true, "Type": "txt", "LongTermValid": true, "IPTextSets": text})
		}
	}
	b, _ := json.Marshal(rules)
	out := luckyimport.Document{}
	for k, v := range doc {
		out[k] = v
	}
	out["RuleList"] = b
	return out
}

// TestRealSampleSafeMode 用真实 Lucky 备份验证端口转发与 STUN 规则安全模式的导入（白名单、黑名单、关闭）。
// 仅在 GATELY_LUCKY_SAMPLES 设置为逗号分隔的样本路径（zip 或目录，可混合 2.x 与 3.0 格式）时运行，样本不入库。
func TestRealSampleSafeMode(t *testing.T) {
	env := os.Getenv("GATELY_LUCKY_SAMPLES")
	if env == "" {
		t.Skip("未设置 GATELY_LUCKY_SAMPLES")
	}
	for _, sample := range strings.Split(env, ",") {
		t.Run(filepath.Base(sample), func(t *testing.T) {
			docs := loadSampleDocs(t, sample)
			ipf, ok := docs["lucky_ipfilter.lkcf"]
			if !ok {
				t.Skip("样本中没有 lucky_ipfilter.lkcf")
			}
			// 原样解析真实的 IP 过滤配置：识别出三条内置规则
			orig := luckyimport.IPFilterLists([]luckyimport.Document{ipf}, time.Now())
			for _, k := range []string{"whitelist", "blacklist", "globalblacklist"} {
				if _, ok := orig[k]; !ok {
					t.Fatalf("应识别内置规则 %s: %+v", k, orig)
				}
			}
			t.Logf("样本原有名单条目: whitelist=%d blacklist=%d globalblacklist=%d",
				len(orig["whitelist"].Entries), len(orig["blacklist"].Entries), len(orig["globalblacklist"].Entries))
			lists := luckyimport.IPFilterLists([]luckyimport.Document{injectIPFilter(t, ipf)}, time.Now())

			type target struct {
				file, field, id string
				mod             func() module.Module
			}
			for _, tg := range []target{
				{"lucky_portforward.lkcf", "PortForwardsRuleList", portforward.ID, func() module.Module { return portforward.New() }},
				{"lucky_stun.lkcf", "StunRuleList", stun.ID, func() module.Module { return stun.New() }},
			} {
				doc, ok := docs[tg.file]
				if !ok {
					continue
				}
				raw, ok := doc[tg.field]
				if !ok {
					t.Fatalf("%s 中没有 %s", tg.file, tg.field)
				}
				var origRules []map[string]any
				_ = json.Unmarshal(raw, &origRules)
				if len(origRules) == 0 {
					t.Logf("%s 没有规则，跳过", tg.file)
					continue
				}
				// "" 表示样本原样导入：真实样本的安全模式均为 blacklist 且名单为空，不应设置访问控制
				for _, mode := range []string{"", "whitelist", "blacklist", "disable"} {
					d := luckyimport.Document{}
					for k, v := range doc {
						d[k] = v
					}
					useLists := lists
					if mode == "" {
						useLists = orig
					} else {
						d[tg.field] = setSafeMode(t, raw, mode)
					}
					st, _ := config.Open(t.TempDir())
					m := tg.mod()
					if err := m.Init(module.Env{Store: st, Logs: logbuf.New(10, log.New(io.Discard, "", 0))}); err != nil {
						t.Fatal(err)
					}
					rep, err := m.(luckyimport.Importer).ImportLucky(d, luckyimport.Options{IPFilter: useLists})
					if err != nil {
						t.Fatal(err)
					}
					mux := http.NewServeMux()
					m.Routes(router{mux, "/api/" + tg.id})
					w := httptest.NewRecorder()
					mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/"+tg.id+"/rules", nil))
					var got []struct {
						Name           string
						DisableForward bool     `json:"disableForward"`
						IPMode         string   `json:"ipMode"`
						IPList         []string `json:"ipList"`
					}
					if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
						t.Fatal(err)
					}
					if len(got) != rep.Imported || rep.Imported == 0 {
						t.Fatalf("%s %s: 导入 %d 条，列表 %d 条，样本 %d 条", tg.file, mode, rep.Imported, len(got), rep.Found)
					}
					for _, r := range got {
						var wantMode, wantList string
						switch {
						case r.DisableForward || mode == "disable" || mode == "":
						case mode == "whitelist":
							wantMode, wantList = "whitelist", "192.168.18.0/24,2001:db8::/32"
						case mode == "blacklist":
							wantMode, wantList = "blacklist", "203.0.113.9,198.51.100.7"
						}
						if r.IPMode != wantMode || strings.Join(r.IPList, ",") != wantList {
							t.Errorf("%s %s %q: 名单 %s %v，期望 %s %s", tg.file, mode, r.Name, r.IPMode, r.IPList, wantMode, wantList)
						}
					}
					joined := strings.Join(rep.Warnings, "\n")
					if mode == "whitelist" && !strings.Contains(joined, "1 条 IP 库、自动记录或地址范围条目无法导入") {
						t.Errorf("%s 白名单应提示地址范围无法导入:\n%s", tg.file, joined)
					}
					t.Logf("%s %s: 导入 %d/%d 条，示例 %s %v", tg.file, mode, rep.Imported, rep.Found, got[0].IPMode, got[0].IPList)
				}
			}
		})
	}
}
