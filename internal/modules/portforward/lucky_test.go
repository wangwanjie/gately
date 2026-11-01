package portforward

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/wangwanjie/gately/internal/config"
	"github.com/wangwanjie/gately/internal/iplist"
	"github.com/wangwanjie/gately/internal/logbuf"
	"github.com/wangwanjie/gately/internal/luckyimport"
	"github.com/wangwanjie/gately/internal/module"
)

func TestConvertLuckyRules(t *testing.T) {
	raw := json.RawMessage(`[
	  {"Name":"ssh","Enable":true,"ForwardTypes":["tcp","udp"],"ListenAddress":"","ListenPorts":"6024",
	   "TargetAddressList":["192.168.1.8"],"TargetPorts":"22",
	   "Options":{"SingleProxyMaxTCPConnections":256,"UDPSessionTimeout":30000,"SafeMode":"blacklist"}},
	  {"Name":"","Enable":false,"ForwardTypes":["tcp6"],"ListenAddress":"","ListenPorts":"3478",
	   "TargetAddressList":["192.168.1.9"],"TargetPorts":"3578",
	   "Options":{"TCPListenTLS":true,"RuleSpeedLimit":true}},
	  {"Name":"bad","Enable":true,"ForwardTypes":["sctp"],"ListenPorts":"1","TargetAddressList":["x"],"Options":{}}
	]`)
	rules, warnings, err := convertLuckyRules(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("应导入 2 条，实际 %d", len(rules))
	}
	r := rules[0]
	if r.UDPSessionTimeout != 30 || r.MaxConnections != 256 || r.TargetPorts != "22" || len(r.Protocols) != 2 {
		t.Fatalf("字段映射错误: %+v", r)
	}
	joined := strings.Join(warnings, "\n")
	if !rules[0].Enable || rules[1].Enable {
		t.Fatal("转换时应保留 Lucky 中的启用状态")
	}
	for _, want := range []string{"TLS", "限速", "bad: 跳过"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("缺少告警 %q:\n%s", want, joined)
		}
	}
}

// TestImportRealSample 用真实的 lucky_portforward.lkcf 做导入预演。
// 仅在设置环境变量 GATELY_LUCKY_SAMPLE=<文件路径> 时运行，样本不入库。
func TestImportRealSample(t *testing.T) {
	path := os.Getenv("GATELY_LUCKY_SAMPLE")
	if path == "" {
		t.Skip("未设置 GATELY_LUCKY_SAMPLE")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := luckyimport.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	st, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Init(module.Env{Store: st, Logs: logbuf.New(10, log.New(io.Discard, "", 0))}); err != nil {
		t.Fatal(err)
	}
	rep, err := m.ImportLucky(doc, luckyimport.Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep == nil {
		t.Skip("样本中没有端口转发配置")
	}
	t.Logf("found=%d imported=%d", rep.Found, rep.Imported)
	for _, w := range rep.Warnings {
		t.Log("warning:", w)
	}
	if rep.Imported != rep.Found {
		t.Fatalf("有规则未能导入")
	}
}

func TestConvertLuckySafeMode(t *testing.T) {
	raw := json.RawMessage(`[
	  {"Name":"wl","Enable":true,"ForwardTypes":["tcp"],"ListenPorts":"6024","TargetAddressList":["192.168.1.8"],"TargetPorts":"22",
	   "Options":{"SafeMode":"whitelist"}},
	  {"Name":"bl","Enable":true,"ForwardTypes":["tcp"],"ListenPorts":"6025","TargetAddressList":["192.168.1.8"],"TargetPorts":"22",
	   "Options":{"SafeMode":"blacklist"}}
	]`)
	lists := map[string]luckyimport.IPFilterList{
		"whitelist": {Entries: []string{"192.168.1.0/24"}},
		"blacklist": {Entries: []string{"192.0.2.66"}},
	}
	rules, _, err := convertLuckyRules(raw, lists)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].IPMode != iplist.ModeWhitelist || rules[0].IPList[0] != "192.168.1.0/24" ||
		rules[1].IPMode != iplist.ModeBlacklist || rules[1].IPList[0] != "192.0.2.66" {
		t.Fatalf("安全模式转换错误: %+v", rules)
	}
	// 没有上传 IP 过滤配置时白名单规则照常导入并提示
	rules, warnings, _ := convertLuckyRules(raw, nil)
	if len(rules) != 2 || rules[0].IPMode != "" || !strings.Contains(strings.Join(warnings, "\n"), "wl: 原规则启用了白名单模式") {
		t.Fatalf("缺少 IP 过滤配置时的结果错误: %+v %v", rules, warnings)
	}
}
