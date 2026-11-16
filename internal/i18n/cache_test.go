package i18n

import (
	"fmt"
	"strings"
	"testing"
)

// 翻译缓存有上限，写满时只淘汰较旧的一代：反复用到的条目一直留在缓存里，不会随着一次性内容被整体清空。
func TestCacheBoundedAndKeepsHotEntries(t *testing.T) {
	var g genCache
	g.put("常用", "hot")
	for i := 0; i < 10*cacheGenEntries; i++ {
		g.put(fmt.Sprintf("一次性内容 %d", i), "x")
		if i%100 == 0 {
			if v, ok := g.get("常用"); !ok || v != "hot" {
				t.Fatalf("写入 %d 条一次性内容后，常用条目被淘汰", i)
			}
		}
		if n := g.len(); n > 2*cacheGenEntries {
			t.Fatalf("缓存条目数 %d 超过上限 %d", n, 2*cacheGenEntries)
		}
	}
	// 不再访问的条目在两代之内被淘汰
	if _, ok := g.get("一次性内容 0"); ok {
		t.Error("最早的一次性内容应已被淘汰")
	}
	// 按字节数同样有上限：大条目很快写满一代
	var big genCache
	val := strings.Repeat("x", 8<<10)
	for i := 0; i < 2000; i++ {
		big.put(fmt.Sprintf("k%d", i), val)
	}
	if n := big.len(); n > 2*(cacheGenBytes/(8<<10)+1) {
		t.Errorf("大条目占用超出字节上限：%d 条", n)
	}
	// 超长内容不进缓存
	big.put("huge", strings.Repeat("x", cacheMaxEntry+1))
	if _, ok := big.get("huge"); ok {
		t.Error("超长内容不应缓存")
	}
}
