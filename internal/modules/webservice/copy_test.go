package webservice

import (
	"strings"
	"testing"
)

func siteKeys(l *Listener) string {
	var out []string
	for _, s := range l.Sites {
		out = append(out, s.Key)
	}
	return strings.Join(out, ",")
}

// 插在中间的新站点不会抢走后面已有站点的 key；重复的 key 保留第一个。
func TestAssignSiteKeys(t *testing.T) {
	l := &Listener{Sites: []Site{{Key: "s1"}, {Key: "s2"}, {}, {Key: "s3"}, {Key: "s4"}}}
	l.assignSiteKeys()
	if got := siteKeys(l); got != "s1,s2,s3_,s3,s4" {
		t.Fatalf("已有站点的 key 应保持不变: %s", got)
	}

	l = &Listener{Sites: []Site{{Key: "x"}, {Key: "x"}, {}, {Key: "s2"}}}
	l.assignSiteKeys()
	got := siteKeys(l)
	parts := strings.Split(got, ",")
	if parts[0] != "x" || parts[3] != "s2" {
		t.Fatalf("第一个重复 key 与已有 key 应保持不变: %s", got)
	}
	seen := map[string]bool{}
	for _, k := range parts {
		if k == "" || seen[k] {
			t.Fatalf("分配后 key 应唯一且非空: %s", got)
		}
		seen[k] = true
	}
}

// 校验监听时分配站点 key 走同一逻辑。
func TestListenerValidateKeepsExistingSiteKeys(t *testing.T) {
	l := &Listener{Port: 8080, Sites: []Site{
		{Key: "s1", Enable: true, Domains: []string{"a.example.com"}, Type: "redirect", RedirectTo: "https://x.example.com"},
		{Enable: true, Domains: []string{"b.example.com"}, Type: "redirect", RedirectTo: "https://x.example.com"},
		{Key: "s2", Enable: true, Domains: []string{"c.example.com"}, Type: "redirect", RedirectTo: "https://x.example.com"},
	}}
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := siteKeys(l); got != "s1,s2_,s2" {
		t.Fatalf("已有站点的 key 应保持不变: %s", got)
	}
}

// 复制监听时清空全部站点与默认站点的分享链接。
func TestDropShareLinks(t *testing.T) {
	l := &Listener{
		Sites:   []Site{{ShareLinks: []ShareLink{{Token: "t1"}}}, {}},
		Default: Site{ShareLinks: []ShareLink{{Token: "t2"}}},
	}
	l.dropShareLinks()
	if len(l.Sites[0].ShareLinks) != 0 || len(l.Default.ShareLinks) != 0 {
		t.Fatalf("分享链接应被清空: %+v", l)
	}
}
