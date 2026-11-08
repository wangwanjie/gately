package fwopen

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeBins 在临时目录写入假的 nft / iptables 脚本并只让 PATH 指向它，候选路径也清空，避免用到宿主机上的真实命令
func fakeBins(t *testing.T, scripts map[string]string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("需要 shell 脚本")
	}
	dir := t.TempDir()
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	oldN, oldI, oldI6 := nftPaths, iptablesPaths, ip6tablesPaths
	nftPaths, iptablesPaths, ip6tablesPaths = nil, nil, nil
	t.Cleanup(func() { nftPaths, iptablesPaths, ip6tablesPaths = oldN, oldI, oldI6 })
}

// 群晖 DSM 的 nft 报错（issue #74）：内核没有 nf_tables
const dsmNFT = `echo "netlink: Error: cache initialization failed: Invalid argument" >&2; exit 1
`

// iptables-legacy：-V 报版本，-S INPUT 输出空链
const legacyIPT = `case "$1" in
-V) echo "iptables v1.8.3 (legacy)";;
*) echo "-P INPUT ACCEPT";;
esac
`

// 内核没有 nf_tables 但有 iptables-legacy 时改用 iptables 放行
func TestTargetsFallBackToIPTablesWithoutNFTables(t *testing.T) {
	fakeBins(t, map[string]string{"nft": dsmNFT, "iptables": legacyIPT})
	b := &cmdBackend{}
	list, _, err := b.targets(context.Background())
	if err != nil {
		t.Fatalf("应退回 iptables: %v", err)
	}
	if len(list) != 1 || list[0].Kind != "iptables" {
		t.Fatalf("目标应只有 iptables 的 INPUT 链: %+v", list)
	}
	if b.nft != "" {
		t.Fatal("nft 不可用时写入也不应再使用 nft")
	}
}

// 内核没有 nf_tables 且没有 iptables 时给出明确说明
func TestTargetsNoNFTablesNoIPTables(t *testing.T) {
	fakeBins(t, map[string]string{"nft": dsmNFT})
	_, _, err := (&cmdBackend{}).targets(context.Background())
	if err == nil || !strings.Contains(err.Error(), "不支持 nftables") {
		t.Fatalf("应说明内核不支持: %v", err)
	}
}

// nft 的其他错误（如权限不足）仍原样报告，不退回 iptables
func TestTargetsNFTPermissionErrorNotFallback(t *testing.T) {
	fakeBins(t, map[string]string{
		"nft":      `echo "netlink: Error: cache initialization failed: Operation not permitted" >&2; exit 1` + "\n",
		"iptables": legacyIPT,
	})
	_, _, err := (&cmdBackend{}).targets(context.Background())
	if err == nil || !strings.Contains(err.Error(), "没有修改防火墙的权限") {
		t.Fatalf("权限错误应原样报告: %v", err)
	}
}

func TestKernelNoNFT(t *testing.T) {
	if !kernelNoNFT("netlink: Error: cache initialization failed: Invalid argument") || kernelNoNFT("Operation not permitted") {
		t.Fatal("kernelNoNFT 判断不符")
	}
}
