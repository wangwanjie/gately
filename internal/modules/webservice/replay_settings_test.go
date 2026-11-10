package webservice

import "testing"

// 配置中的回放容量无效（手工编辑）时，概览接口上报的设置与容量与实际录制使用的默认值一致。
func TestReplayIndexInvalidSettingsReportsEffectiveLimit(t *testing.T) {
	m, _ := newConnsModule(t, []Site{{Key: "a", Enable: true, Domains: []string{"a.test"}, Type: TypeText, StatusCode: 200, Text: "ok"}}, Site{})
	m.mu.Lock()
	m.cfg.Replay = ReplaySettings{Enable: true, SizeMB: replayMaxMB + 100}
	m.syncReplayLocked()
	st := m.replay.store
	m.mu.Unlock()
	t.Cleanup(func() {
		m.mu.Lock()
		m.cfg.Replay = ReplaySettings{}
		m.syncReplayLocked()
		m.mu.Unlock()
	})
	want := int64(replayDefaultMB) << 20
	st.mu.Lock()
	got := st.limit
	st.mu.Unlock()
	if got != want {
		t.Fatalf("录制应改用默认容量: %d", got)
	}
	out := m.replayIndex(0, 0, 0, 0)
	if out.Limit != want || out.Settings.SizeMB != replayDefaultMB || !out.Settings.Enable {
		t.Fatalf("概览上报的容量与实际不符: limit=%d settings=%+v", out.Limit, out.Settings)
	}

	// 没有存储时按生效设置计算
	if s := effectiveReplaySettings(ReplaySettings{SizeMB: -5}); s.SizeMB != replayDefaultMB {
		t.Fatalf("无效容量应回退默认值: %+v", s)
	}
}
