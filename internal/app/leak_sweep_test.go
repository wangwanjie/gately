package app

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// leakMark 是种子数据里所有密钥共有的标记：只读身份的任何响应里都不应出现它。
const leakMark = "LEAKMARK"

// leakHook 是带密钥的 Webhook 设置：地址路径与查询参数里的令牌、请求头的值、请求体。
const leakHook = `{"enable":false,"url":"https://hooks.example.com/services/LEAKMARKabcdef0123456789?token=LEAKMARKq","method":"POST",
 "headers":["Authorization: Bearer LEAKMARKhdr","X-Api-Key: LEAKMARKxk"],"body":"{\"key\":\"LEAKMARKbody\"}"}`

// leakSeeds 在各模块里保存带标记的密钥，覆盖密码、令牌、私钥口令、命令与脚本、附加启动参数、环境变量、
// 请求头与请求体、地址里夹带的凭据等形态。新增能保存密钥的字段时在这里补一条。
var leakSeeds = []struct{ method, path, body string }{
	{"POST", "/api/webterminal/credentials", `{"name":"c1","enable":true,"authType":"password","username":"root","password":"LEAKMARK-credpw","privateKey":"","passphrase":"LEAKMARK-pp"}`},
	{"POST", "/api/webterminal/connections", `{"name":"s1","enable":true,"type":"ssh","ssh":{"host":"127.0.0.1","port":19439,"username":"root","authType":"password","password":"LEAKMARK-sshpw","privateKey":"","passphrase":"LEAKMARK-sshpp"}}`},
	{"POST", "/api/webterminal/connections", `{"name":"l1","enable":true,"type":"local","local":{"shell":"/bin/sh","env":{"SECRET_ENV":"LEAKMARK-localenv"}}}`},
	{"POST", "/api/webterminal/connections", `{"name":"t1","enable":true,"type":"telnet","telnet":{"host":"127.0.0.1","port":19439,"username":"u","password":"LEAKMARK-telnetpw"}}`},
	{"POST", "/api/cron/tasks", `{"name":"c","enable":false,"type":"command","spec":"0 3 1 1 *","command":"curl -H 'Authorization: Bearer LEAKMARKcmd' https://x.example/ ; mysql -pLEAKMARKcmdpw"}`},
	{"POST", "/api/cron/tasks", `{"name":"h","enable":false,"type":"http","spec":"0 3 1 1 *","http":` + leakHook + `}`},
	{"POST", "/api/ddns/tasks", `{"name":"d","enable":false,"provider":"cloudflare","credentials":{"id":"","secret":"LEAKMARK-ddnssecret","extra":{"x":"LEAKMARK-extra"}},
	  "ipv4":{"enable":true,"type":"command","command":"echo LEAKMARK-ipcmd"},"records":[{"enable":true,"zone":"example.com","sub":"a","type":"A","value":"{ipv4Addr}"}],"webhook":` + leakHook + `}`},
	{"POST", "/api/ddns/tasks", `{"name":"cb","enable":false,"provider":"callback","credentials":{"id":"","secret":"","extra":{"url":"https://ddns.example.com/update","method":"POST",
	  "headers":"Authorization: Bearer LEAKMARK-cbhdr\nX-Api-Key: LEAKMARK-cbkey","body":"{\"apikey\":\"LEAKMARK-cbbody\",\"ip\":\"{ip}\"}"}},
	  "ipv4":{"enable":true,"type":"url","urls":["https://api.ipify.org"]},"records":[{"enable":true,"zone":"example.com","sub":"a","type":"A","value":"{ipv4Addr}"}]}`},
	{"POST", "/api/ddns/tasks", `{"name":"ovh","enable":false,"provider":"ovh","credentials":{"id":"appkey","secret":"LEAKMARK-ovhsecret","extra":{"consumerKey":"LEAKMARK-ovhconsumer","endpoint":"ovh-eu"}},
	  "ipv4":{"enable":true,"type":"url","urls":["https://api.ipify.org"]},"records":[{"enable":true,"zone":"example.com","sub":"a","type":"A","value":"{ipv4Addr}"}]}`},
	{"POST", "/api/stun/rules", `{"name":"st","enable":false,"protocol":"tcp4","useGlobalServers":true,"localPort":19438,"targetPort":19437,"targetAddresses":["127.0.0.1"],
	  "script":"echo LEAKMARK-stunscript","scriptEnable":true,"webhook":` + leakHook + `}`},
	{"POST", "/api/notify/channels", `{"name":"n1","enable":false,"kind":"webhook","webhook":` + leakHook + `}`},
	{"POST", "/api/notify/channels", `{"name":"n2","enable":false,"kind":"telegram","secret":"LEAKMARK-tgtoken","target":"12345"}`},
	{"POST", "/api/notify/channels", `{"name":"n3","enable":false,"kind":"smtp","host":"smtp.example.com","port":465,"user":"u@example.com","secret":"LEAKMARK-smtppw","from":"u@example.com","target":"v@example.com"}`},
	{"POST", "/api/notify/channels", `{"name":"n4","enable":false,"kind":"bark","url":"https://api.day.app","secret":"LEAKMARKbarkkey0123456789"}`},
	{"POST", "/api/storage/mounts", `{"name":"m1","enable":false,"type":"webdav","url":"https://u:LEAKMARKurlpw@dav.example.com/","username":"u","password":"LEAKMARK-davpw","path":"/"}`},
	{"POST", "/api/frp/servers", `{"name":"fs","enable":false,"bindPort":19436,"token":"LEAKMARK-frpstoken","dashboardPort":19435,"dashboardUser":"a","dashboardPwd":"LEAKMARK-dashpwd"}`},
	{"POST", "/api/frp/clients", `{"name":"fc","enable":false,"serverAddr":"127.0.0.1","serverPort":19436,"token":"LEAKMARK-frpctoken"}`},
	{"POST", "/api/easytier/instances", `{"name":"e1","enable":false,"networkName":"n","networkSecret":"LEAKMARK-etsecret","extraArgs":"--foo LEAKMARK-etargs","env":"A=LEAKMARK-etenv"}`},
	{"POST", "/api/netbird/instances", `{"name":"nb","enable":false,"setupKey":"LEAKMARK-nbsetup","daemonEnv":"A=LEAKMARK-nbenv","extraUpArgs":"--x LEAKMARK-nbargs","daemonExtraArgs":"--y LEAKMARK-nbdargs"}`},
	{"POST", "/api/tailscale/instances", `{"name":"ts","enable":false,"authKey":"tskey-auth-LEAKMARK-tsauth","daemonEnv":"A=LEAKMARK-tsenv","extraUpArgs":"--x LEAKMARK-tsargs","daemonExtraArgs":"--y LEAKMARK-tsdargs"}`},
	{"POST", "/api/cloudflared/tunnels", `{"name":"cf","enable":false,"mode":"token","token":"LEAKMARK-cftoken","apiToken":"LEAKMARK-cfapi"}`},
	{"POST", "/api/wol/devices", `{"name":"w","enable":true,"macs":["00:11:22:33:44:55"],"agentToken":"LEAKMARK-agenttoken","shutdownMode":"http",
	  "shutdownHttp":{"url":"https://x.example/off?key=LEAKMARKq2","method":"POST","headers":["X-Key: LEAKMARK-wolhdr"],"body":"k=LEAKMARK-wolbody"}}`},
	// 端口敲门的密钥是 Base64：这两个值解码后以 LEAKMARK 开头，编码后的前缀为 TEVBS01BUks
	{"POST", "/api/knock/rules", `{"name":"k","enable":false,"encKey":"TEVBS01BUkstZW5ja2V5LTEyMzQ1Njc4OTA=","hmacKey":"TEVBS01BUkstaG1hY2tleS0xMjM0NTY3ODkw","ports":["tcp/19434"],"timeout":30}`},
	{"POST", "/api/ssl/certs", `{"enable":false,"source":"acme","acme":{"ca":"letsencrypt","challenge":"dns","provider":"cloudflare","credentials":{"id":"","secret":"LEAKMARK-acmesecret"},
	  "domains":["a.example.com"],"email":"a@example.com","eabKid":"kid","eabHmac":"LEAKMARK-eabhmac"},"mapping":{"enable":false,"dir":"/tmp/x","script":"echo LEAKMARK-mapscript"}}`},
	{"PUT", "/api/backup/settings", `{"enable":false,"localEnable":true,"keep":7,"schedule":"0 3 * * *","password":"LEAKMARK-backuppw"}`},
	{"PUT", "/api/docker/settings", `{"autoCheckHours":0,"host":"","mirrors":[],"registries":[{"server":"r.example.com","username":"u","password":"LEAKMARK-regpw"}]}`},
	{"PUT", "/api/fileusers", `{"users":[{"name":"fu","password":"LEAKMARK-fileuser","root":"/srv/leak-sweep"}]}`},
	{"PUT", "/api/webdav/settings", `{"enable":false,"listenIp":"127.0.0.1","port":19433,"users":[{"name":"wd","password":"LEAKMARK-webdav","root":"/srv/leak-sweep"}]}`},
	{"PUT", "/api/ftp/settings", `{"enable":false,"listenIp":"127.0.0.1","port":19432,"users":[{"name":"ft","password":"LEAKMARK-ftp","root":"/srv/leak-sweep"}]}`},
	{"PUT", "/api/smb/settings", `{"enable":false,"listenIp":"127.0.0.1","port":19431,"users":[{"name":"sm","password":"LEAKMARK-smb","root":"/srv/leak-sweep"}]}`},
	{"POST", "/api/webservice/listeners", `{"name":"ws","enable":false,"network":"tcp4","listenIp":"127.0.0.1","port":19430,
	  "default":{"enable":true,"type":"proxy","targets":["http://u:LEAKMARKtargetpw@127.0.0.1:19429"],"basicAuth":true,"users":["alice:LEAKMARK-siteuser"],
	    "requestHeaders":["Authorization: Bearer LEAKMARK-reqhdr"],"shareLinks":[{"name":"sl","token":"LEAKMARKsharetoken0123456789"}]}}`},
}

var leakPattern = regexp.MustCompile(`LEAKMARK[-A-Za-z0-9]*|TEVBS01BUks[A-Za-z0-9+/=]*`)

// leakSkip 列出遍历时不请求的接口：它们会访问外网或建立交互通道，与本测试关心的“响应里是否带出已保存的密钥”无关。
func leakSkip(path string) bool {
	for _, seg := range strings.Split(path, "/") {
		switch seg {
		case "ws", "update", "install", "search", "stream", "events":
			return true
		}
	}
	return false
}

// 只读账号、只读令牌与单模块只读令牌遍历全部 GET 接口，任何响应里都不能出现已保存的密钥明文、
// 会话签名密钥或密码哈希。
func TestReadOnlyIdentitiesNeverSeeSecrets(t *testing.T) {
	a, srv, c := fullApp(t, t.TempDir(), nil)
	// 遍历中个别接口可能访问外网（检查更新等）：把更新代理指向本机一个不监听的端口，这类请求立即失败
	a.settingsMu.Lock()
	a.settings.UpdateProxy = "http://127.0.0.1:9"
	a.settingsMu.Unlock()
	h := a.Handler()

	for _, s := range leakSeeds {
		if code, body := doJSON(t, c, s.method, srv.URL+s.path, json.RawMessage(s.body)); code != 200 {
			t.Errorf("写入种子数据 %s %s 失败（接口变了就更新 leakSeeds）: %d %s", s.method, s.path, code, body)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	if code, body := doJSON(t, c, "POST", srv.URL+"/api/account/users", map[string]string{"username": "viewer", "password": "tide-Lamp-47-oak"}); code != 200 {
		t.Fatalf("创建只读账号失败 %d %s", code, body)
	}
	cookieOf := func(user, pass string) string {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"username":"`+user+`","password":"`+pass+`"}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rec, req)
		for _, ck := range rec.Result().Cookies() {
			if ck.Value != "" {
				return ck.Name + "=" + ck.Value
			}
		}
		t.Fatalf("%s 登录失败: %d %s", user, rec.Code, rec.Body.String())
		return ""
	}
	adminCookie, viewerCookie := cookieOf("admin", "test-password"), cookieOf("viewer", "tide-Lamp-47-oak")
	get := func(path string, hdr map[string]string) (int, string) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path, nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		h.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	// 展开路径参数：{key} 等取自管理员读到的列表，其余参数用占位值
	keys := map[string][]string{}
	for _, r := range a.routes {
		if r.Method != "GET" || strings.Contains(r.Path, "{") || leakSkip(r.Path) {
			continue
		}
		_, body := get(r.Path, map[string]string{"Cookie": adminCookie})
		var list []map[string]any
		if json.Unmarshal([]byte(body), &list) != nil {
			continue
		}
		for _, it := range list {
			if k, _ := it["key"].(string); k != "" {
				keys[r.Path] = append(keys[r.Path], k)
			}
		}
	}
	param := regexp.MustCompile(`\{[A-Za-z]+\}`)
	pathSet := map[string]bool{}
	for _, r := range a.routes {
		if r.Method != "GET" || r.Public || leakSkip(r.Path) {
			continue
		}
		if !strings.Contains(r.Path, "{") {
			pathSet[r.Path] = true
			continue
		}
		base := r.Path[:strings.Index(r.Path, "/{")]
		for _, k := range append(keys[base], "x") {
			first := true
			pathSet[param.ReplaceAllStringFunc(r.Path, func(string) string {
				if first {
					first = false
					return k
				}
				return "x"
			})] = true
		}
	}
	paths := make([]string, 0, len(pathSet))
	for p := range pathSet {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	if len(paths) < 150 {
		t.Fatalf("遍历到的 GET 接口数量异常: %d", len(paths))
	}

	// 管理员确实能读到这些标记：证明种子数据已保存、遍历覆盖到了返回它们的接口
	seen := map[string]bool{}
	for _, p := range paths {
		_, body := get(p, map[string]string{"Cookie": adminCookie})
		for _, m := range leakPattern.FindAllString(body, -1) {
			seen[m] = true
		}
	}
	if len(seen) < 20 {
		t.Fatalf("管理员应能在这些接口里读到大部分种子密钥，实际只有 %d 个: %v", len(seen), seen)
	}

	secrets := []string{a.auth.EntrySecret()}
	forbidden := func(body string) []string {
		var hits []string
		hits = append(hits, leakPattern.FindAllString(body, -1)...)
		for _, s := range secrets {
			if s != "" && strings.Contains(body, s) {
				hits = append(hits, "会话签名密钥")
			}
		}
		if strings.Contains(body, "$2a$") || strings.Contains(body, "$2b$") {
			hits = append(hits, "bcrypt 哈希")
		}
		return hits
	}

	identities := map[string]map[string]string{"只读账号": {"Cookie": viewerCookie}}
	_, all := createToken(t, srv.URL, c, map[string]any{"name": "all-read", "scopes": []string{"*:read"}})
	identities["*:read 令牌"] = map[string]string{"Authorization": "Bearer " + all}
	for _, d := range a.scopeDomains() {
		if d.WriteOnly {
			continue
		}
		_, tok := createToken(t, srv.URL, c, map[string]any{"name": "r-" + d.ID, "scopes": []string{d.ID + ":read"}})
		identities[d.ID+":read 令牌"] = map[string]string{"Authorization": "Bearer " + tok}
	}
	leaks := 0
	for name, hdr := range identities {
		for _, p := range paths {
			code, body := get(p, hdr)
			if hits := forbidden(body); len(hits) > 0 {
				leaks++
				if leaks <= 40 {
					t.Errorf("%s 读取 %s（%d）带出了密钥: %v", name, p, code, hits)
				}
			}
		}
	}
	if leaks > 40 {
		t.Errorf("共 %d 处泄露，以上只列出前 40 处", leaks)
	}
}
