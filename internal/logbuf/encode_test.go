package logbuf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// 序列化移到锁外之后，写进文件的每一行仍与直接 json.Marshal 这条日志逐字节相同（包括转义方式与字段顺序）。
func TestPersistedLineIdenticalToMarshal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.log")
	b := New(100, nil)
	if err := b.Persist(path, 0); err != nil {
		t.Fatal(err)
	}
	l := b.Logger("来源 <a&b>")
	l.Infof("普通 %s", "内容")
	l.With(Fields{FieldIP: "1.2.3.4", FieldStatus: "200", FieldPath: "/a?b=<c>&d=\"e\""}).Warnf("%s %q", "GET", "ua   \x00 \xff 换行\n制表\t")
	b.Add(LevelError, "", "")
	b.AddFields("notice", "s", "未知级别", Fields{})
	b.addAt(time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.FixedZone("x", 8*3600)), LevelInfo, "", "指定时间", nil)
	for i := 0; i < 30; i++ {
		// 序号从一位数增长到两位数
		b.Add(LevelInfo, "", fmt.Sprintf("第 %d 条", i))
	}
	want := b.Since(0)
	b.ClosePersist()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n"))
	if len(lines) != len(want) {
		t.Fatalf("文件有 %d 行，应为 %d", len(lines), len(want))
	}
	for i, e := range want {
		exp, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(lines[i], exp) {
			t.Errorf("第 %d 行与 json.Marshal 不同:\n文件 %s\n应为 %s", i, lines[i], exp)
		}
		if strings.Contains(string(lines[i]), "Format") || strings.Contains(string(lines[i]), "Args") {
			t.Errorf("格式串不应写入文件: %s", lines[i])
		}
	}
}

// 多个协程同时写日志时，文件中的顺序与序号一致（序号逐条加一），与内存缓冲的内容相同。
func TestPersistOrderUnderConcurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.log")
	const workers, per = 8, 3000
	b := New(workers*per, nil)
	if err := b.Persist(path, 64<<20); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			l := b.Logger(fmt.Sprintf("w%d", w)).With(Fields{FieldIP: fmt.Sprintf("10.0.0.%d", w)})
			for i := 0; i < per; i++ {
				l.Infof("协程 %d 第 %d 条", w, i)
			}
		}(w)
	}
	wg.Wait()
	mem := b.Since(0)
	b.ClosePersist()
	file := readTail(path, workers*per+10)
	if len(file) != workers*per || len(mem) != workers*per {
		t.Fatalf("文件 %d 条，内存 %d 条，应各为 %d", len(file), len(mem), workers*per)
	}
	raw, _ := os.ReadFile(path)
	if n := bytes.Count(raw, []byte("\n")); n != workers*per {
		t.Fatalf("文件有 %d 行", n)
	}
	for i := range file {
		if file[i].Seq != uint64(i+1) {
			t.Fatalf("文件第 %d 条的序号为 %d，顺序与序号不一致", i, file[i].Seq)
		}
		if file[i].Message != mem[i].Message || file[i].Source != mem[i].Source || file[i].Fields[FieldIP] != mem[i].Fields[FieldIP] {
			t.Fatalf("文件第 %d 条与内存不一致: %+v / %+v", i, file[i], mem[i])
		}
	}
}

// 预先序列化的内容不是预期的形状时不写入，由调用方改用锁内序列化；待写数据的上限判断与原来相同。
func TestAddEncodedRejectsUnexpectedLine(t *testing.T) {
	s := &fileSink{path: filepath.Join(t.TempDir(), "x.log"), opt: PersistOptions{}.normalized(), sources: map[string]struct{}{}}
	now := time.Now()
	for _, line := range []string{`{"seq":1,"time":"x"}` + "\n", `{"seq":0,"time":"x"}`, `{"time":"x","seq":0}` + "\n", "\n"} {
		if s.addEncoded(7, now, []byte(line)) {
			t.Errorf("形状不对的内容 %q 不应被接受", line)
		}
	}
	if len(s.pending) != 0 {
		t.Fatalf("不应写入任何内容: %q", s.pending)
	}
	if !s.addEncoded(18446744073709551615, now, []byte(`{"seq":0,"time":"x","message":"m"}`+"\n")) {
		t.Fatal("正常内容应被接受")
	}
	if got := string(s.pending); got != `{"seq":18446744073709551615,"time":"x","message":"m"}`+"\n" || !s.pendingFirst.Equal(now) {
		t.Fatalf("写入的内容不对: %q", got)
	}
	s.flush()
}
