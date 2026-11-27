package sysstat

import "testing"

func TestDetectContainer(t *testing.T) {
	none := func(string) bool { return false }
	cases := []struct {
		name string
		env  containerEnv
		want bool
	}{
		{"宿主机 systemd", containerEnv{MarkerExists: none, Pid1Comm: "systemd\n", Cgroup: "0::/init.scope\n", ExeName: "gately"}, false},
		{"宿主机 OpenWrt procd", containerEnv{MarkerExists: none, Pid1Comm: "procd\n", ExeName: "gately"}, false},
		{"宿主机 macOS 无 proc", containerEnv{MarkerExists: none, ExeName: "gately"}, false},
		{"Docker 标记文件", containerEnv{MarkerExists: func(p string) bool { return p == "/.dockerenv" }}, true},
		{"Podman 标记文件", containerEnv{MarkerExists: func(p string) bool { return p == "/run/.containerenv" }}, true},
		{"container 环境变量", containerEnv{MarkerExists: none, EnvContainer: "lxc"}, true},
		{"cgroup 含 docker", containerEnv{MarkerExists: none, Cgroup: "12:cpu:/docker/abc\n"}, true},
		{"cgroup 含 kubepods", containerEnv{MarkerExists: none, Cgroup: "0::/kubepods/besteffort/pod1\n"}, true},
		{"RouterOS：PID 1 就是 gately", containerEnv{MarkerExists: none, Pid1Comm: "gately\n", Cgroup: "0::/\n", ExeName: "gately"}, true},
		{"RouterOS：入口为 sh 脚本", containerEnv{MarkerExists: none, Pid1Comm: "sh\n", Cgroup: "0::/\n", ExeName: "gately"}, true},
		{"极简 init tini", containerEnv{MarkerExists: none, Pid1Comm: "tini\n"}, true},
		{"进程名截断仍匹配", containerEnv{MarkerExists: none, Pid1Comm: "gately-linux-am\n", ExeName: "gately-linux-amd64"}, true},
	}
	for _, c := range cases {
		if got := detectContainer(c.env); got != c.want {
			t.Errorf("%s：得到 %v，期望 %v", c.name, got, c.want)
		}
	}
}
