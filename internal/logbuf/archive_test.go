package logbuf

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// archiveBuf 创建开启归档落盘的缓冲，测试结束时等待后台压缩与清理完成，避免临时目录删除失败。
func archiveBuf(t *testing.T, size int, opt PersistOptions) (*Buffer, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "webservice.log")
	b := New(size, nil)
	opt.Archive = true
	if err := b.PersistWith(path, opt); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s := b.sink
		b.ClosePersist()
		if s != nil {
			s.bg.Wait()
		}
	})
	return b, path
}

// settle 把待写盘数据写入文件，并等待后台压缩与清理完成。
func settle(b *Buffer) {
	b.sink.flush()
	b.sink.bg.Wait()
}

// dirNames 返回目录中的文件名（排序后）。
func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range ents {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

func day(d, h int) time.Time {
	return time.Date(2026, 11, d, h, 0, 0, 0, time.Local)
}

func TestArchiveRotatesDailyAndCompresses(t *testing.T) {
	b, path := archiveBuf(t, 3, PersistOptions{FileBytes: 1 << 20, KeepBytes: 64 << 20})
	for i := 1; i <= 4; i++ {
		b.addAt(day(1, 10+i), LevelInfo, "HTTPS", fmt.Sprintf("一号第 %d 条", i), nil)
	}
	settle(b)
	for i := 1; i <= 3; i++ {
		b.addAt(day(2, 8+i), LevelInfo, "HTTPS", fmt.Sprintf("二号第 %d 条", i), nil)
	}
	settle(b)
	b.addAt(day(3, 9), LevelWarn, "HTTP", "三号", nil)
	settle(b)

	names := dirNames(t, filepath.Dir(path))
	want := []string{
		"webservice." + day(1, 11).UTC().Format(archiveTimeLayout) + ".log.gz",
		"webservice." + day(2, 9).UTC().Format(archiveTimeLayout) + ".log.gz",
		"webservice.log",
		"webservice.sources",
	}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatalf("归档文件不对:\n得到 %v\n期望 %v", names, want)
	}
	// 压缩文件可以正常解压，内容是当天的全部日志
	data := gunzip(t, filepath.Join(filepath.Dir(path), want[0]))
	if n := bytes.Count(data, []byte{'\n'}); n != 4 {
		t.Fatalf("一号归档应有 4 行，得到 %d", n)
	}
	st, err := os.Stat(filepath.Join(filepath.Dir(path), want[0]))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("归档权限应为 0600: %v %v", st.Mode(), err)
	}
	// 缓冲只有 3 条，查询全部 8 条要跨当前文件与两个压缩归档
	if got := fmt.Sprint(seqs(b.Query(0, 0, 100, nil))); got != "[1 2 3 4 5 6 7 8]" {
		t.Fatalf("应跨归档查到全部日志，得到 %s", got)
	}
	// 分页加载更早的日志
	if got := fmt.Sprint(seqs(b.Query(0, 6, 3, nil))); got != "[3 4 5]" {
		t.Fatalf("before=6 limit=3 应得到 3..5，得到 %s", got)
	}
	// 过滤条件在压缩归档里同样生效
	f := &Filter{Terms: []string{"一号"}}
	if got := fmt.Sprint(seqs(b.Query(0, 0, 2, f))); got != "[3 4]" {
		t.Fatalf("应返回最新的两条命中，得到 %s", got)
	}
}

func gunzip(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := out.ReadFrom(zr); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestArchiveTimeRangeSkipsFiles(t *testing.T) {
	b, path := archiveBuf(t, 2, PersistOptions{FileBytes: 1 << 20})
	for d := 1; d <= 4; d++ {
		b.addAt(day(d, 12), LevelInfo, "", fmt.Sprintf("第 %d 天", d), nil)
		settle(b)
	}
	// 把 2 号的归档换成损坏的内容：按时间范围只查 4 号时不应打开它，结果也不受影响
	dir := filepath.Dir(path)
	bad := filepath.Join(dir, "webservice."+day(2, 12).UTC().Format(archiveTimeLayout)+".log.gz")
	if err := os.WriteFile(bad, []byte("not gzip"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &Filter{From: day(3, 0), To: day(3, 23)}
	if got := fmt.Sprint(seqs(b.Query(0, 0, 10, f))); got != "[3]" {
		t.Fatalf("只查 3 号应得到第 3 条，得到 %s", got)
	}
	// 查询范围包含损坏的归档时，读到它就停止，不跳过它去读更早的 1 号，避免结果中间缺一段
	f = &Filter{From: day(1, 0)}
	if got := fmt.Sprint(seqs(b.Query(0, 0, 10, f))); got != "[3 4]" {
		t.Fatalf("遇到损坏的归档应停止，得到 %s", got)
	}
}

func TestArchiveRotatesBySize(t *testing.T) {
	b, path := archiveBuf(t, 5, PersistOptions{FileBytes: 300, KeepBytes: 1 << 20})
	for i := 0; i < 20; i++ {
		b.addAt(day(5, 10).Add(time.Duration(i)*time.Second), LevelInfo, "", strings.Repeat("x", 80), nil)
		b.sink.flush()
	}
	settle(b)
	archives := b.sink.listArchives()
	if len(archives) < 5 {
		t.Fatalf("按大小应轮转出多个归档，得到 %d", len(archives))
	}
	for _, a := range archives {
		if a.plain != "" || a.gz == "" {
			t.Fatalf("归档应全部压缩: %+v", a)
		}
	}
	if st, err := os.Stat(path); err != nil || st.Size() > 300 {
		t.Fatalf("当前文件不应超过上限: %v", err)
	}
	got := b.Query(0, 0, 100, nil)
	if len(got) != 20 || got[0].Seq != 1 || got[19].Seq != 20 {
		t.Fatalf("应按顺序查到全部 20 条，得到 %v", seqs(got))
	}
}

func TestArchiveNameCollision(t *testing.T) {
	b, _ := archiveBuf(t, 5, PersistOptions{FileBytes: 100})
	// 同一秒内多次按大小轮转，归档名追加序号，不能互相覆盖
	at := day(6, 10)
	for i := 0; i < 4; i++ {
		b.addAt(at, LevelInfo, "", strings.Repeat("y", 90), nil)
		b.sink.flush()
	}
	settle(b)
	if n := len(b.sink.listArchives()); n != 3 {
		t.Fatalf("应有 3 个归档，得到 %d", n)
	}
	if got := len(b.Query(0, 0, 100, nil)); got != 4 {
		t.Fatalf("应查到 4 条，得到 %d", got)
	}
}

func TestArchivePruneByDaysAndSize(t *testing.T) {
	now := time.Now()
	b, path := archiveBuf(t, 2, PersistOptions{FileBytes: 1 << 20, KeepDays: 3, KeepBytes: 64 << 20})
	// 10 天前、5 天前、2 天前、昨天各一条，然后是今天
	for _, ago := range []int{10, 5, 2, 1, 0} {
		b.addAt(now.AddDate(0, 0, -ago), LevelInfo, "", fmt.Sprintf("%d 天前", ago), nil)
		settle(b)
	}
	// 10 天前的归档在 5 天前结束，5 天前的在 2 天前结束：前者超过 3 天删除，后者结束于保留期内仍保留
	archives := b.sink.listArchives()
	if len(archives) != 3 {
		t.Fatalf("应保留 3 个归档，得到 %d: %v", len(archives), dirNames(t, filepath.Dir(path)))
	}
	if got := fmt.Sprint(seqs(b.Query(0, 0, 100, nil))); got != "[2 3 4 5]" {
		t.Fatalf("10 天前的日志应已清理，得到 %s", got)
	}

	// 改小总大小上限：从最旧的归档开始删除，当前文件保留
	if err := b.PersistWith(path, PersistOptions{Archive: true, FileBytes: 1 << 20, KeepDays: 3, KeepBytes: 1}); err != nil {
		t.Fatal(err)
	}
	b.sink.bg.Wait()
	if n := len(b.sink.listArchives()); n != 0 {
		t.Fatalf("超过总大小时应删除全部归档，剩 %d 个", n)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("当前文件不应删除: %v", err)
	}
}

func TestArchiveRestoreAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "webservice.log")
	b := New(10, nil)
	if err := b.PersistWith(path, PersistOptions{Archive: true}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		b.addAt(day(7, i), LevelInfo, "HTTPS", fmt.Sprintf("第 %d 条", i), nil)
	}
	b.sink.flush()
	b.addAt(day(8, 1), LevelInfo, "HTTPS", "第 6 条", nil)
	b.sink.flush()
	// 当前文件只剩 1 条，其余在刚生成的归档里（压缩前或压缩后都能读）
	s := b.sink
	b.ClosePersist()
	s.bg.Wait()

	b2 := New(10, nil)
	if err := b2.PersistWith(path, PersistOptions{Archive: true}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		s := b2.sink
		b2.ClosePersist()
		s.bg.Wait()
	}()
	if got := fmt.Sprint(seqs(b2.Query(0, 0, 0, nil))); got != "[1 2 3 4 5 6]" {
		t.Fatalf("重启后应从当前文件与归档恢复，得到 %s", got)
	}
	b2.Add(LevelInfo, "", "新日志")
	if got := b2.Query(0, 0, 0, nil); got[len(got)-1].Seq != 7 {
		t.Fatalf("序号应接着递增，得到 %d", got[len(got)-1].Seq)
	}
	if got := fmt.Sprint(b2.Sources()); got != "[HTTPS]" {
		t.Fatalf("来源应从记录文件恢复，得到 %s", got)
	}
}

func TestArchiveCompressLeftovers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webservice.log")
	// 模拟上次压缩到一半退出：一个未压缩的归档、一个残留的临时文件、一个已压缩但未删原文件的归档
	ts1 := day(1, 1).UTC().Format(archiveTimeLayout)
	ts2 := day(2, 1).UTC().Format(archiveTimeLayout)
	line := func(seq int, at time.Time) string {
		return fmt.Sprintf(`{"seq":%d,"time":%q,"level":"info","message":"m%d"}`+"\n", seq, at.Format(time.RFC3339Nano), seq)
	}
	write := func(name, data string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("webservice."+ts1+".log", line(1, day(1, 1)))
	write("webservice."+ts1+".log.gz.tmp", "half")
	write("webservice."+ts2+".log", line(2, day(2, 1)))
	if err := compressFile(filepath.Join(dir, "webservice."+ts2+".log")); err != nil {
		t.Fatal(err)
	}
	write("webservice."+ts2+".log", line(2, day(2, 1)))
	// 其他模块的同前缀文件与符号链接不受影响
	write("webservice2."+ts1+".log", "other")
	if err := os.Symlink("/etc/hosts", filepath.Join(dir, "webservice."+day(3, 1).UTC().Format(archiveTimeLayout)+".log")); err != nil {
		t.Fatal(err)
	}

	b := New(10, nil)
	if err := b.PersistWith(path, PersistOptions{Archive: true}); err != nil {
		t.Fatal(err)
	}
	s := b.sink
	s.bg.Wait()
	defer b.ClosePersist()
	got := dirNames(t, dir)
	want := []string{
		"webservice." + ts1 + ".log.gz",
		"webservice." + ts2 + ".log.gz",
		"webservice." + day(3, 1).UTC().Format(archiveTimeLayout) + ".log",
		"webservice2." + ts1 + ".log",
	}
	sort.Strings(want)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("残留文件处理不对:\n得到 %v\n期望 %v", got, want)
	}
	if got := fmt.Sprint(seqs(b.Query(0, 0, 0, nil))); got != "[1 2]" {
		t.Fatalf("应恢复两个归档中的日志，得到 %s", got)
	}

	// 清空删除本日志的全部文件，其他文件与符号链接指向的目标不受影响
	b.Clear()
	got = dirNames(t, dir)
	want = []string{"webservice." + day(3, 1).UTC().Format(archiveTimeLayout) + ".log", "webservice2." + ts1 + ".log"}
	sort.Strings(want)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("清空后剩余文件不对: %v", got)
	}
	if _, err := os.Stat("/etc/hosts"); err != nil {
		t.Fatalf("符号链接目标不应受影响: %v", err)
	}
}

func TestArchiveLegacyRotatedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webservice.log")
	// 旧版本只保留一份 .1：切换到归档模式后仍能查到，并作为最旧的文件参与清理
	b := New(2, nil)
	if err := b.Persist(path, 0); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		b.addAt(day(1, i), LevelInfo, "", fmt.Sprintf("旧 %d", i), nil)
	}
	b.ClosePersist()
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}

	b2, _ := func() (*Buffer, string) {
		b2 := New(2, nil)
		if err := b2.PersistWith(path, PersistOptions{Archive: true, KeepBytes: 64 << 20}); err != nil {
			t.Fatal(err)
		}
		return b2, path
	}()
	defer func() {
		s := b2.sink
		b2.ClosePersist()
		s.bg.Wait()
	}()
	b2.addAt(day(2, 1), LevelInfo, "", "新 4", nil)
	settle(b2)
	b2.addAt(day(3, 1), LevelInfo, "", "新 5", nil)
	settle(b2)
	if got := fmt.Sprint(seqs(b2.Query(0, 0, 100, nil))); got != "[1 2 3 4 5]" {
		t.Fatalf("应查到 .1、归档与当前文件中的全部日志，得到 %s", got)
	}
	if err := b2.PersistWith(path, PersistOptions{Archive: true, KeepBytes: 1}); err != nil {
		t.Fatal(err)
	}
	b2.sink.bg.Wait()
	if _, err := os.Stat(path + ".1"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("超过上限时应先删除 .1: %v", err)
	}
}

func TestScanForwardLongAndBrokenLines(t *testing.T) {
	long := strings.Repeat("z", maxLineBytes+10)
	data := "a\n\n" + long + "\nb\nc"
	var got []string
	err := scanForward(context.Background(), strings.NewReader(data), func(line []byte) bool {
		got = append(got, string(line))
		return true
	}, 0)
	if err != nil || fmt.Sprint(got) != "[a b c]" {
		t.Fatalf("超长行应跳过，末尾无换行的行也要读到: %v %v", got, err)
	}
	mid := strings.Repeat("m", tailChunk*2)
	got = nil
	_ = scanForward(context.Background(), strings.NewReader("x\n"+mid+"\ny\n"), func(line []byte) bool {
		got = append(got, string(line[:1]))
		return true
	}, 2)
	if fmt.Sprint(got) != "[x m]" {
		t.Fatalf("比缓冲长但未超限的行应完整读出，maxLines 应生效: %v", got)
	}
}

func TestSameDay(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*3600)
	a := time.Date(2026, 11, 6, 15, 59, 0, 0, time.UTC) // 东八区 23:59
	b := time.Date(2026, 11, 6, 23, 59, 59, 0, loc)
	c := time.Date(2026, 11, 7, 0, 0, 1, 0, loc)
	if !sameDay(a, b) || sameDay(a, c) || !sameDay(time.Time{}, c) {
		t.Fatal("应按后一个时间的时区比较日期")
	}
}

// TestArchiveUnknownStartStillRotates 当前文件第一行损坏、开始时间未知时，之后仍按天切分。
func TestArchiveUnknownStartStillRotates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webservice.log")
	if err := os.WriteFile(path, []byte("损坏的行\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	b := New(10, nil)
	if err := b.PersistWith(path, PersistOptions{Archive: true}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		s := b.sink
		b.ClosePersist()
		s.bg.Wait()
	}()
	b.addAt(day(1, 1), LevelInfo, "", "一号", nil)
	settle(b)
	b.addAt(day(2, 1), LevelInfo, "", "二号", nil)
	settle(b)
	if n := len(b.sink.listArchives()); n != 1 {
		t.Fatalf("开始时间未知时也应按天切分，归档数 %d", n)
	}
	if got := fmt.Sprint(seqs(b.Query(0, 0, 10, nil))); got != "[1 2]" {
		t.Fatalf("应查到两条，得到 %s", got)
	}
}

// TestArchivePruneWithGap 中间长时间没有日志时：旧归档的最后一次写入已超过保留期，即使下一个归档很新也要删除。
func TestArchivePruneWithGap(t *testing.T) {
	now := time.Now()
	b, path := archiveBuf(t, 2, PersistOptions{FileBytes: 1 << 20, KeepDays: 3})
	b.addAt(now.AddDate(0, 0, -10), LevelInfo, "", "10 天前", nil)
	settle(b)
	b.addAt(now.AddDate(0, 0, -1), LevelInfo, "", "昨天", nil)
	settle(b)
	archives := b.sink.listArchives()
	if len(archives) != 1 {
		t.Fatalf("应有 1 个归档，得到 %d", len(archives))
	}
	// 把归档的修改时间改回 10 天前（真实场景中它最后一次写入就在那时）
	old := now.AddDate(0, 0, -10)
	for _, p := range archives[0].paths() {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	b.addAt(now, LevelInfo, "", "今天", nil)
	settle(b)
	if got := fmt.Sprint(seqs(b.Query(0, 0, 10, nil))); got != "[2 3]" {
		t.Fatalf("10 天前的归档应按最后写入时间清理，得到 %s；目录 %v", got, dirNames(t, filepath.Dir(path)))
	}
}

// TestArchiveRestoreSkipsExpired 启动时先清理过期归档再恢复，过期的日志不进入内存缓冲。
func TestArchiveRestoreSkipsExpired(t *testing.T) {
	now := time.Now()
	path := filepath.Join(t.TempDir(), "webservice.log")
	b := New(10, nil)
	if err := b.PersistWith(path, PersistOptions{Archive: true}); err != nil {
		t.Fatal(err)
	}
	b.addAt(now.AddDate(0, 0, -10), LevelInfo, "", "10 天前", nil)
	settle(b)
	b.addAt(now, LevelInfo, "", "今天", nil)
	settle(b)
	s := b.sink
	b.ClosePersist()
	s.bg.Wait()
	old := now.AddDate(0, 0, -10)
	for _, a := range s.listArchives() {
		for _, p := range a.paths() {
			_ = os.Chtimes(p, old, old)
		}
	}
	b2 := New(10, nil)
	if err := b2.PersistWith(path, PersistOptions{Archive: true, KeepDays: 3}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		s := b2.sink
		b2.ClosePersist()
		s.bg.Wait()
	}()
	if got := fmt.Sprint(seqs(b2.Query(0, 0, 0, nil))); got != "[2]" {
		t.Fatalf("过期归档中的日志不应恢复到内存，得到 %s", got)
	}
	b2.Add(LevelInfo, "", "新日志")
	if got := b2.Query(0, 0, 0, nil); got[len(got)-1].Seq != 3 {
		t.Fatalf("序号应接着递增，得到 %v", seqs(got))
	}
}

// TestArchiveSplitsLargeFlush 一次写盘的数据跨过多个切分大小时按行拆开，每个文件不超过上限，日志不丢。
func TestArchiveSplitsLargeFlush(t *testing.T) {
	b, path := archiveBuf(t, 10, PersistOptions{FileBytes: 1000, KeepBytes: 64 << 20})
	at := day(6, 10)
	for i := 0; i < 50; i++ {
		b.addAt(at.Add(time.Duration(i)*time.Minute), LevelInfo, "", strings.Repeat("z", 100), nil)
	}
	settle(b)
	archives := b.sink.listArchives()
	if len(archives) < 5 {
		t.Fatalf("应拆成多个归档，得到 %d", len(archives))
	}
	for _, a := range archives {
		if n := len(gunzip(t, a.gz)); n > 1000 {
			t.Fatalf("归档 %s 超过上限: %d", filepath.Base(a.gz), n)
		}
	}
	if st, _ := os.Stat(path); st.Size() > 1000 {
		t.Fatalf("当前文件超过上限: %d", st.Size())
	}
	got := b.Query(0, 0, 100, nil)
	if len(got) != 50 || got[0].Seq != 1 || got[49].Seq != 50 {
		t.Fatalf("应按顺序查到全部 50 条，得到 %d", len(got))
	}
	// 每个归档以其中第一条日志的时间命名
	for _, a := range archives {
		zr := gunzip(t, a.gz)
		first := lineTime(zr, time.Time{})
		if !first.UTC().Truncate(time.Second).Equal(a.start) {
			t.Fatalf("归档名时间 %v 与第一条日志 %v 不一致", a.start, first)
		}
	}
}
