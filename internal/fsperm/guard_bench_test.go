package fsperm

import (
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkGuardHidden 衡量共享目录包含配置目录时，文件服务每次访问一个普通文件前的屏蔽判断开销
// （WebDAV、SMB、FTP 的每个操作都要判断一次，SMB 打开一个文件往往要经过多次）。
func BenchmarkGuardHidden(b *testing.B) {
	base := b.TempDir()
	share := mkdir(b, base, "vol1")
	conf := mkdir(b, share, "1000", "docker", "gately")
	media := mkdir(b, share, "1000", "media", "movies", "2024", "film")
	file := filepath.Join(media, "film.mkv")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		b.Fatal(err)
	}
	Protect(conf)
	g := NewGuard(share)
	if g == nil {
		b.Fatal("应有屏蔽规则")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if g.Hidden(file) {
			b.Fatal("普通文件不应被屏蔽")
		}
	}
}
