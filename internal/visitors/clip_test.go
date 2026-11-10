package visitors

import "testing"

// 访客可控字符串去掉 Unicode 控制字符、双向控制符、零宽字符与行/段分隔符。
func TestClipStripsInvisibleRunes(t *testing.T) {
	cases := map[string]string{
		"a\u202egnp.exe":           "agnp.exe",
		"x\u2066y\u2069z":          "xyz",
		"ad\u200bmin\u200d":        "admin",
		"\ufeffbom":                "bom",
		"line\u2028sep\u2029":      "linesep",
		"c1\u0085ctl\u009b":        "c1ctl",
		"lrm\u200e\u200frlm":       "lrmrlm",
		"中文 Ünïcode ok":            "中文 Ünïcode ok",
		"tab\tnl\n\x7f":            "tabnl",
		"\u202a\u202b\u202c\u202d": "",
	}
	for in, want := range cases {
		if got := clip(in, 256); got != want {
			t.Fatalf("clip(%q) = %q，应为 %q", in, got, want)
		}
	}
	s, now := newTestStore(t, "")
	a := ip("203.0.113.9")
	s.Record(Hit{Addr: a, Time: *now, Site: "s", Path: "/a\u202eb", UA: "Moz\u200billa", User: "ali\u2066ce", Host: "h\u2028x"})
	v, _ := s.Get(a)
	if v.Paths[0].Path != "/ab" || v.Agents[0].UA != "Mozilla" || v.Identities[0].Name != "alice" || v.LastReq.Host != "hx" {
		t.Fatalf("记录中的字符串应过滤: %+v", v)
	}
}
