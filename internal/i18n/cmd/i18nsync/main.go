// i18nsync 把源码里的中文字面量同步到 internal/i18n/locales/<语言>.json：
// 新增的键写入空值等待翻译，已有译文保持不变，源码里已不存在的键默认保留（加 -prune 删除）。
// 同时生成二进制内嵌用的 <语言>.json.gz。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wangwanjie/gately/internal/i18n"
	"github.com/wangwanjie/gately/internal/i18n/extract"
)

func main() {
	root := flag.String("root", ".", "仓库根目录")
	prune := flag.Bool("prune", false, "删除源码中已不存在的键")
	flag.Parse()

	keys, err := extract.Literals(filepath.Join(*root, "internal"), filepath.Join(*root, "cmd"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dir := filepath.Join(*root, "internal", "i18n", "locales")
	_ = os.MkdirAll(dir, 0o755)
	for _, lang := range i18n.AllLangs {
		if lang == i18n.SourceLang {
			continue
		}
		path := filepath.Join(dir, lang+".json")
		cur := map[string]string{}
		if raw, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(raw, &cur)
		}
		next := map[string]string{}
		missing := 0
		for _, k := range keys {
			v := cur[k]
			if v == "" {
				missing++
			}
			next[k] = v
		}
		if !*prune {
			for k, v := range cur {
				if _, ok := next[k]; !ok {
					next[k] = v
				}
			}
		}
		raw, _ := json.MarshalIndent(next, "", "  ")
		raw = append(raw, '\n')
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		// 二进制只嵌入压缩版本，JSON 改动后必须同步重新生成
		if err := os.WriteFile(path+".gz", i18n.CompressLocale(raw), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("%s: %d 个键，待翻译 %d\n", lang, len(next), missing)
	}
}
