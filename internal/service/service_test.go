package service

import (
	"bytes"
	"strings"
	"testing"
)

func TestSystemdUnit(t *testing.T) {
	u := systemdUnit(Spec{Exe: "/usr/local/bin/gately", ConfigDir: "/etc/gately"})
	for _, want := range []string{
		"ExecStart=/usr/local/bin/gately -c /etc/gately\n",
		"Restart=on-failure",
		"WantedBy=multi-user.target",
		"CAP_NET_BIND_SERVICE",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("单元文件缺少 %q:\n%s", want, u)
		}
	}
	// 路径含空格时需要加引号，管理界面监听地址原样传入
	u = systemdUnit(Spec{Exe: "/opt/my apps/gately", ConfigDir: "/data/my gately", Listen: ":16680"})
	if want := `ExecStart="/opt/my apps/gately" -c "/data/my gately" -listen :16680`; !strings.Contains(u, want) {
		t.Errorf("引号处理不正确:\n%s", u)
	}
}

func TestLaunchdPlist(t *testing.T) {
	p := launchdPlist(Spec{Exe: "/usr/local/bin/gately", ConfigDir: "/usr/local/etc/g&g", Listen: ":16680"}, "/var/log/gately.log")
	for _, want := range []string{
		"<string>" + Label + "</string>",
		"<string>/usr/local/bin/gately</string>",
		"<string>/usr/local/etc/g&amp;g</string>",
		"<string>-listen</string>",
		"<key>RunAtLoad</key>",
		"<key>SuccessfulExit</key>\n\t\t<false/>",
		"<string>/var/log/gately.log</string>",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("plist 缺少 %q:\n%s", want, p)
		}
	}
}

func TestRunArgs(t *testing.T) {
	cases := []struct {
		args []string
		code int
		want string
	}{
		{nil, 2, "用法"},
		{[]string{"bogus"}, 2, "未知的服务操作"},
		{[]string{"start", "extra"}, 2, "多余的参数"},
		{[]string{"--help"}, 0, "用法"},
	}
	for _, c := range cases {
		var out bytes.Buffer
		if got := Run(c.args, &out); got != c.code {
			t.Errorf("%v: 退出码 %d，期望 %d", c.args, got, c.code)
		}
		if !strings.Contains(out.String(), c.want) {
			t.Errorf("%v: 输出缺少 %q:\n%s", c.args, c.want, out.String())
		}
	}
}

// ExecStart 会展开 % 说明符与 $变量，路径中的 % 与 $ 必须转义，单元文件才能原样执行
func TestSystemdUnitEscapes(t *testing.T) {
	u := systemdUnit(Spec{Exe: "/opt/a%20b/gately", ConfigDir: "/data/$HOME/g's", Listen: "[::]:16680"})
	want := `ExecStart=/opt/a%%20b/gately -c "/data/$$HOME/g's" -listen [::]:16680` + "\n"
	if !strings.Contains(u, want) {
		t.Errorf("转义不正确，期望含 %q:\n%s", want, u)
	}
	u = systemdUnit(Spec{Exe: `C:\x "y"`, ConfigDir: "/etc/gately"})
	if !strings.Contains(u, `ExecStart="C:\\x \"y\"" -c /etc/gately`) {
		t.Errorf("反斜杠与双引号转义不正确:\n%s", u)
	}
}

// 换行等控制字符可以在单元文件里另起一行注入指令，安装时直接拒绝
func TestValidateSpecRejectsControlChars(t *testing.T) {
	ok := Spec{Exe: "/usr/local/bin/gately", ConfigDir: "/data/my gately", Listen: ":16680"}
	if err := validateSpec(ok); err != nil {
		t.Fatalf("正常路径被拒绝: %v", err)
	}
	for _, sp := range []Spec{
		{Exe: "/usr/local/bin/gately", ConfigDir: "/etc/gately\nExecStartPre=/bin/sh -c id"},
		{Exe: "/usr/local/bin/gately", ConfigDir: "/etc/gately", Listen: ":16680\r"},
		{Exe: "/tmp/\x00gately", ConfigDir: "/etc/gately"},
		{Exe: "/tmp/gately\t", ConfigDir: "/etc/gately"},
	} {
		if err := validateSpec(sp); err == nil {
			t.Errorf("含控制字符的 %+v 未被拒绝", sp)
		}
	}
	var out bytes.Buffer
	if code := Run([]string{"install", "-c", "/etc/gately\nExecStartPre=/bin/id"}, &out); code != 1 || !strings.Contains(out.String(), "控制字符") {
		t.Errorf("install 未拒绝控制字符: 退出码 %d\n%s", code, out.String())
	}
}
