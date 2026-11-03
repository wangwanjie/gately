package tailscale

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestArgs(t *testing.T) {
	in := Instance{Key: "k1", Name: "nas", Options: DefaultOptions(), LegacyStateDir: true}
	s := &in.Options
	s.AuthKey = "tskey-abc"
	s.Hostname = "gately"
	s.LoginServer = "https://hs.example.com"
	s.AdvertiseRoutes = []string{"192.168.1.5/24"}
	s.AdvertiseExitNode = true
	s.AcceptRoutes = true
	s.Socks5 = "localhost:1055"
	s.ExtraUpArgs = "--ssh  --shields-up"
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	up := s.UpArgs()
	for _, want := range []string{
		"up", "--reset", "--accept-routes=true", "--accept-dns=false", "--auth-key=tskey-abc", "--hostname=gately",
		"--login-server=https://hs.example.com", "--advertise-routes=192.168.1.0/24", "--advertise-exit-node", "--ssh", "--shields-up",
	} {
		if !slices.Contains(up, want) {
			t.Errorf("up 参数缺少 %s: %v", want, up)
		}
	}
	d := in.DaemonArgs("/etc/gately")
	want := []string{"--statedir=/etc/gately/tailscale", "--socket=/etc/gately/tailscale/tailscaled.sock", "--tun=userspace-networking", "--socks5-server=localhost:1055"}
	if !slices.Equal(d, want) {
		t.Errorf("tailscaled 参数错误: %v", d)
	}
	if got := in.CLIArgs("/etc/gately"); !slices.Equal(got, []string{"--socket=/etc/gately/tailscale/tailscaled.sock"}) {
		t.Errorf("CLI 参数错误: %v", got)
	}

	// 使用外部守护进程时不指定套接字，沿用 tailscale 的系统默认值
	ext := Instance{Key: "k2"}
	if got := ext.CLIArgs("/x"); got != nil {
		t.Errorf("外部守护进程默认不应带 --socket: %v", got)
	}
}

func TestValidate(t *testing.T) {
	bad := []Options{
		{StateDir: "relative/dir"},
		{Socks5: "no-port"},
		{Hostname: "bad_name"},
		{LoginServer: "headscale.example.com"},
		{AdvertiseRoutes: []string{"not-a-cidr"}},
		{Socket: "relative.sock"},
	}
	for i, s := range bad {
		if err := s.Validate(); err == nil {
			t.Errorf("用例 %d 应被拒绝: %+v", i, s)
		}
	}
}

func TestPeerViewAndStatusParse(t *testing.T) {
	raw := `{"Version":"1.80.0","BackendState":"Running","AuthURL":"","Health":[],"MagicDNSSuffix":"tail1234.ts.net",
	"CurrentTailnet":{"Name":"me@example.com"},
	"Self":{"HostName":"gately","DNSName":"gately.tail1234.ts.net.","TailscaleIPs":["100.64.0.1"],"Online":true,"OS":"linux"},
	"Peer":{"k1":{"HostName":"nas","DNSName":"nas.tail1234.ts.net.","TailscaleIPs":["100.64.0.2"],"Online":false,"LastSeen":"0001-01-01T00:00:00Z"},
	        "k2":{"HostName":"phone","TailscaleIPs":["100.64.0.3"],"Online":true,"ExitNodeOption":true,"PrimaryRoutes":["10.0.0.0/24"]}}}`
	var st rawStatus
	if err := jsonUnmarshal(raw, &st); err != nil {
		t.Fatal(err)
	}
	if st.BackendState != "Running" || len(st.Peer) != 2 || st.CurrentTailnet.Name != "me@example.com" {
		t.Fatalf("解析错误: %+v", st)
	}
	self := peerView(st.Self)
	if self.DNSName != "gately.tail1234.ts.net" || !strings.HasPrefix(self.IPs[0], "100.") {
		t.Errorf("节点视图错误: %+v", self)
	}
	if !st.Peer["k1"].LastSeen.IsZero() == false && peerView(st.Peer["k1"]).LastSeen.Year() > 1 {
		t.Error("零值时间不应展示")
	}
}

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }

func TestDaemonPortArgsAndEnv(t *testing.T) {
	in := Instance{Key: "k1", Options: DefaultOptions(), LegacyStateDir: true}
	s := &in.Options
	s.Userspace = false
	s.Port = 41641
	s.DaemonExtraArgs = "--no-logs-no-support  --verbose=1"
	s.DaemonEnv = "# 宿主机 iptables 为 nf_tables 后端\nTS_DEBUG_FIREWALL_MODE=nftables\n\n  FOO = bar baz \n"
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	want := []string{"--statedir=/etc/gately/tailscale", "--socket=/etc/gately/tailscale/tailscaled.sock", "--port=41641", "--no-logs-no-support", "--verbose=1"}
	if got := in.DaemonArgs("/etc/gately"); !slices.Equal(got, want) {
		t.Errorf("tailscaled 参数错误: %v", got)
	}
	if got := s.DaemonEnvList(); !slices.Equal(got, []string{"TS_DEBUG_FIREWALL_MODE=nftables", "FOO=bar baz"}) {
		t.Errorf("环境变量错误: %v", got)
	}

	// 默认不带 --port，也没有额外环境变量
	def := Instance{Key: "k1", Options: DefaultOptions()}
	if got := def.DaemonArgs("/x"); slices.ContainsFunc(got, func(a string) bool { return strings.HasPrefix(a, "--port") }) {
		t.Errorf("默认不应固定端口: %v", got)
	}
	if got := def.DaemonEnvList(); len(got) != 0 {
		t.Errorf("默认不应有环境变量: %v", got)
	}

	for i, bad := range []Options{{Port: 70000}, {Port: -1}, {DaemonEnv: "NOEQUALS"}, {DaemonEnv: "1BAD=x"}} {
		if err := bad.Validate(); err == nil {
			t.Errorf("用例 %d 应被拒绝: %+v", i, bad)
		}
	}
}
