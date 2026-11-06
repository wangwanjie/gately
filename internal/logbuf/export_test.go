package logbuf

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestExportOrderFilterAndDedupe(t *testing.T) {
	b, _ := archiveBuf(t, 3, PersistOptions{FileBytes: 1 << 20})
	for d := 1; d <= 3; d++ {
		for i := 0; i < 2; i++ {
			lv := LevelInfo
			if i == 1 {
				lv = LevelWarn
			}
			b.addAt(day(d, 10+i), lv, "HTTPS", fmt.Sprintf("%d 号 %s", d, lv), Fields{FieldStatus: "200"})
		}
		settle(b)
	}
	// 最后一条还在待写盘数据里，导出时也要包含
	b.addAt(day(3, 20), LevelError, "HTTPS", "3 号 error", nil)

	collect := func(f *Filter) string {
		var got []uint64
		err := b.Export(context.Background(), f, func(e *Entry) error {
			got = append(got, e.Seq)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(got)
	}
	if got := collect(nil); got != "[1 2 3 4 5 6 7]" {
		t.Fatalf("应按时间顺序导出全部日志且不重复，得到 %s", got)
	}
	if got := collect(&Filter{MinLevel: LevelWarn}); got != "[2 4 6 7]" {
		t.Fatalf("按级别过滤导出，得到 %s", got)
	}
	if got := collect(&Filter{From: day(2, 0), To: day(2, 23)}); got != "[3 4]" {
		t.Fatalf("按时间范围导出，得到 %s", got)
	}
	// fn 返回错误时停止
	stop := errors.New("stop")
	n := 0
	if err := b.Export(context.Background(), nil, func(*Entry) error {
		n++
		if n == 2 {
			return stop
		}
		return nil
	}); !errors.Is(err, stop) || n != 2 {
		t.Fatalf("fn 出错时应停止: %v %d", err, n)
	}
	// 已取消的导出返回 ctx 的错误
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.Export(ctx, nil, func(*Entry) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消后应返回 context.Canceled: %v", err)
	}
}

func TestExportWithoutPersist(t *testing.T) {
	b := New(3, nil)
	for i := 1; i <= 5; i++ {
		b.Add(LevelInfo, "", fmt.Sprintf("m%d", i))
	}
	var got []uint64
	if err := b.Export(context.Background(), nil, func(e *Entry) error {
		got = append(got, e.Seq)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != "[3 4 5]" {
		t.Fatalf("未落盘时只导出缓冲内的日志，得到 %v", got)
	}
}
