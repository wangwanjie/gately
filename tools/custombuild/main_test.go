package main

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func loadManifest(t *testing.T) manifest {
	t.Helper()
	raw, err := os.ReadFile("../../modules.json")
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestResolve(t *testing.T) {
	m := loadManifest(t)

	// 选了备份时自动补上存储挂载，核心模块传入时忽略
	r, err := resolve(m, "backup, webservice ,frp", "zh", "linux-arm64,fnos-arm,linux-arm64")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.Modules, []string{"frp", "storage", "backup"}) || !slices.Equal(r.Added, []string{"storage"}) {
		t.Fatalf("依赖补齐结果不对: %+v", r)
	}
	tags := strings.Split(r.Tags, ",")
	if slices.Contains(tags, "no_backup") || slices.Contains(tags, "no_storage") || !slices.Contains(tags, "no_waf") || !slices.Contains(tags, "lang_zh") {
		t.Fatalf("build tags 不对: %s", r.Tags)
	}
	if r.Locales != "zh-CN" || len(r.Targets) != 2 || r.Targets[1].Platform != "arm" {
		t.Fatalf("语言或目标不对: %+v", r)
	}

	// 组合键与模块传入顺序无关，模块或语言不同则不同
	r2, _ := resolve(m, "frp,backup", "zh", "")
	r3, _ := resolve(m, "frp,backup", "all", "")
	if r.Profile != r2.Profile || r.Profile == r3.Profile || len(r.Profile) != 12 {
		t.Fatalf("组合键不对: %s %s %s", r.Profile, r2.Profile, r3.Profile)
	}

	// 全部模块与全部语言不带任何 tag，与发布页的完整版本一致
	full, _ := resolve(m, "all", "all", "")
	if full.Tags != "" {
		t.Fatalf("完整构建不应带 tag: %s", full.Tags)
	}

	// 定制镜像：没有 Docker 管理与入侵防护时不装 docker-cli 与 nftables
	img, _ := resolve(m, "frp", "cjk-en", "docker")
	if img.Targets[0].Kind != "docker" || img.Targets[0].Platforms == "" {
		t.Fatalf("docker 目标不对: %+v", img.Targets)
	}
	want := []string{"GATELY_TAGS=" + img.Tags, "GATELY_LOCALES=zh-CN,zh-TW,en", "WITH_DOCKER_CLI=0", "WITH_NFTABLES=0"}
	if !slices.Equal(img.DockerArgs, want) {
		t.Fatalf("dockerArgs 不对: %v", img.DockerArgs)
	}
	img, _ = resolve(m, "docker,guard", "all", "docker")
	if !slices.Contains(img.DockerArgs, "WITH_DOCKER_CLI=1") || !slices.Contains(img.DockerArgs, "WITH_NFTABLES=1") {
		t.Fatalf("选了 Docker 管理与入侵防护时应安装对应工具: %v", img.DockerArgs)
	}
	// STUN 的自动放行防火墙同样需要 nft
	img, _ = resolve(m, "stun", "zh", "docker")
	if !slices.Contains(img.DockerArgs, "WITH_NFTABLES=1") {
		t.Fatalf("选了 STUN 时应安装 nftables: %v", img.DockerArgs)
	}

	for _, bad := range [][3]string{{"nope", "all", ""}, {"", "jp", ""}, {"", "all", "linux-sparc"}} {
		if _, err := resolve(m, bad[0], bad[1], bad[2]); err == nil {
			t.Fatalf("非法选项应报错: %v", bad)
		}
	}
}
