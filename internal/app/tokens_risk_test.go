package app

import "testing"

// 令牌权限界面如实标出写权限能读写服务器文件或执行命令的模块。
func TestScopeDomainRiskFlags(t *testing.T) {
	a := newDocApp(t)
	files, exec := map[string]bool{}, map[string]bool{}
	for _, d := range a.scopeDomains() {
		if d.CanFiles {
			files[d.ID] = true
		}
		if d.CanExec {
			exec[d.ID] = true
		}
	}
	for id := range fileDomains {
		if !files[id] {
			t.Errorf("fileDomains 中的 %s 不是已注册的模块", id)
		}
	}
	for _, id := range []string{"storage", "webservice", "webdav", "ftp", "smb", "backup", "fileusers"} {
		if !files[id] {
			t.Errorf("%s 的写权限可以读写服务器文件，应有提示", id)
		}
	}
	for _, id := range []string{"tailscale", "netbird"} {
		if !exec[id] {
			t.Errorf("%s 的写权限可以追加守护进程参数与环境变量，应提示可执行命令", id)
		}
	}
	if files["portforward"] || files["status"] || files["logs"] {
		t.Fatalf("不涉及文件的权限域不应提示: %v", files)
	}
}
