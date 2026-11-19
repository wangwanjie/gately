package notify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/wangwanjie/gately/internal/events"
	"github.com/wangwanjie/gately/internal/fsperm"
	"github.com/wangwanjie/gately/internal/httpx"
	"github.com/wangwanjie/gately/internal/i18n"
)

// 失败重试与补推的默认值与上限。
const (
	defaultRetries       = 3
	maxRetries           = 10
	defaultBackfillHours = 24
	maxBackfillHours     = 168
	// maxPendingPerChannel 为每个渠道最多积压的待补推消息，超出时丢弃最早的
	maxPendingPerChannel = 100
	// backfillMinWait / backfillMaxWait 为补推失败后的退避区间，每次失败等待时间翻倍
	backfillMinWait = time.Minute
	backfillMaxWait = 15 * time.Minute
	// backfillTick 为补推协程检查队列的间隔
	backfillTick = 20 * time.Second
)

// retryDelays 为第 n 次失败后等待的时间，超出部分沿用最后一项；测试可替换。
var retryDelays = []time.Duration{10 * time.Second, 30 * time.Second, 60 * time.Second}

func retryDelay(attempt int) time.Duration {
	i := attempt - 1
	if i >= len(retryDelays) {
		i = len(retryDelays) - 1
	}
	if i < 0 {
		i = 0
	}
	return retryDelays[i]
}

// retries 返回失败后的重试次数。
func (s Settings) retries() int {
	if s.Retries == nil {
		return defaultRetries
	}
	return *s.Retries
}

// backfill 返回重试用尽后等待补推的最长时间，0 为不补推。
func (s Settings) backfill() time.Duration {
	h := defaultBackfillHours
	if s.BackfillHours != nil {
		h = *s.BackfillHours
	}
	return time.Duration(h) * time.Hour
}

// sleepCtx 等待 d，期间 ctx 取消则提前返回 false。
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// pendingItem 是一条等待补推的消息，落盘保存，重启后继续补推。
type pendingItem struct {
	ID          string       `json:"id"`
	Channel     string       `json:"channel"`
	ChannelName string       `json:"channelName"`
	Rule        string       `json:"rule"`
	Event       events.Event `json:"event"`
	Title       string       `json:"title"`
	Text        string       `json:"text"`
	Server      string       `json:"server"`
	// Queued 为进入队列的时间，超过补推保留时长后放弃
	Queued time.Time `json:"queued"`
	// Attempts 为进入队列后的补推尝试次数
	Attempts int    `json:"attempts"`
	Error    string `json:"error,omitempty"`
}

func (p pendingItem) message() Message {
	return Message{Event: p.Event, Title: p.Title, Text: p.Text, Server: p.Server}
}

// pendingQueue 是待补推队列。next 与 wait 为各渠道下次补推时间与当前退避，只在内存中。
type pendingQueue struct {
	mu    sync.Mutex
	path  string
	seq   int64
	items []pendingItem
	next  map[string]time.Time
	wait  map[string]time.Duration
}

func newPendingQueue() pendingQueue {
	return pendingQueue{next: map[string]time.Time{}, wait: map[string]time.Duration{}}
}

// pendingPath 返回队列文件位置：配置目录下的 notify 子目录，不进配置历史版本。
func pendingPath(dir string) string {
	return filepath.Join(dir, "notify", "pending.json")
}

func (q *pendingQueue) load() error {
	if q.path == "" {
		return nil
	}
	data, err := os.ReadFile(q.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var items []pendingItem
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	q.mu.Lock()
	q.items = items
	q.mu.Unlock()
	return nil
}

// saveLocked 写入队列文件；队列为空时删除文件。调用方持有 q.mu。
func (q *pendingQueue) saveLocked() error {
	if q.path == "" {
		return nil
	}
	if len(q.items) == 0 {
		if err := os.Remove(q.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(q.path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(q.items)
	if err != nil {
		return err
	}
	tmp := q.path + ".tmp"
	if err := fsperm.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, q.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (q *pendingQueue) has(channel string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, it := range q.items {
		if it.Channel == channel {
			return true
		}
	}
	return false
}

// enqueue 把重试用尽的消息加入补推队列。
func (m *Module) enqueue(rule string, ch Channel, msg Message) bool {
	q := &m.pend
	now := m.now()
	q.mu.Lock()
	defer q.mu.Unlock()
	q.seq++
	q.items = append(q.items, pendingItem{
		ID:      strconv.FormatInt(now.UnixNano(), 36) + "-" + strconv.FormatInt(q.seq, 36),
		Channel: ch.Key, ChannelName: ch.Name, Rule: rule,
		Event: msg.Event, Title: msg.Title, Text: msg.Text, Server: msg.Server,
		Queued: now,
	})
	// 每个渠道最多积压 maxPendingPerChannel 条，超出时丢弃该渠道最早的
	n := 0
	for i := len(q.items) - 1; i >= 0; i-- {
		if q.items[i].Channel != ch.Key {
			continue
		}
		n++
		if n > maxPendingPerChannel {
			m.log.Warnf("渠道 %s 待补推的通知超过 %d 条，丢弃最早的「%s」", ch.Name, maxPendingPerChannel, q.items[i].Title)
			q.items = append(q.items[:i], q.items[i+1:]...)
		}
	}
	// 刚刚重试失败，至少等一个退避周期再补推
	if _, ok := q.next[ch.Key]; !ok {
		q.next[ch.Key] = now.Add(backfillMinWait)
	}
	if err := q.saveLocked(); err != nil {
		m.log.Warnf("保存待补推的通知失败: %v", err)
	}
	return true
}

// kick 让补推协程立即检查该渠道。
func (m *Module) kick(channel string) {
	m.pend.mu.Lock()
	delete(m.pend.next, channel)
	delete(m.pend.wait, channel)
	m.pend.mu.Unlock()
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// backfillLoop 定期补推积压的消息，直到 ctx 取消。
func (m *Module) backfillLoop(ctx context.Context) {
	t := time.NewTicker(backfillTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-m.wake:
		}
		m.runBackfill(ctx)
	}
}

// backfillNote 为补推消息正文末尾的说明。
const backfillNote = "补推：首次推送失败，渠道恢复后重新发送"

// runBackfill 处理一轮补推：先放弃过期、渠道已删除或停用的消息，再按渠道依次补发到期的消息；
// 某渠道补发失败时停止该渠道本轮的补发并加长退避，其余渠道不受影响。
func (m *Module) runBackfill(ctx context.Context) {
	m.mu.Lock()
	window := m.cfg.Settings.backfill()
	chans := map[string]Channel{}
	for _, ch := range m.cfg.Channels {
		if ch.Enable {
			ch.Proxy = effectiveProxy(ch, m.cfg.Settings)
			chans[ch.Key] = ch
		}
	}
	m.mu.Unlock()

	q := &m.pend
	now := m.now()
	q.mu.Lock()
	var keep []pendingItem
	var dropped []Record
	for _, it := range q.items {
		reason := ""
		switch {
		case window <= 0:
			reason = "补推已关闭，放弃补推"
		case now.Sub(it.Queued) > window:
			reason = "超过补推保留时长，放弃补推"
		default:
			if _, ok := chans[it.Channel]; !ok {
				reason = "渠道已删除或停用，放弃补推"
			}
		}
		if reason == "" {
			keep = append(keep, it)
			continue
		}
		m.log.Warnf("经渠道 %s 补推「%s」: %s", it.ChannelName, it.Title, reason)
		dropped = append(dropped, Record{Time: now, Type: it.Event.Type, Title: it.Title, Rule: it.Rule, Channel: it.ChannelName,
			Error: i18n.Tr(i18n.ServerLang(), reason), Attempts: it.Attempts, Backfill: true})
	}
	changed := len(keep) != len(q.items)
	q.items = keep
	// 本轮到期的渠道，按积压顺序处理
	var due []string
	seen := map[string]bool{}
	for _, it := range q.items {
		if seen[it.Channel] {
			continue
		}
		seen[it.Channel] = true
		if next, ok := q.next[it.Channel]; !ok || !now.Before(next) {
			due = append(due, it.Channel)
		}
	}
	if changed {
		if err := q.saveLocked(); err != nil {
			m.log.Warnf("保存待补推的通知失败: %v", err)
		}
	}
	q.mu.Unlock()
	for _, r := range dropped {
		m.record(r)
	}
	for _, key := range due {
		if ctx.Err() != nil {
			return
		}
		m.backfillChannel(ctx, chans[key])
	}
}

// backfillChannel 按顺序补发一个渠道积压的消息，失败即停止并退避。
func (m *Module) backfillChannel(ctx context.Context, ch Channel) {
	q := &m.pend
	note := "\n" + i18n.Tr(i18n.ServerLang(), backfillNote)
	for ctx.Err() == nil {
		q.mu.Lock()
		idx := -1
		for i, it := range q.items {
			if it.Channel == ch.Key {
				idx = i
				break
			}
		}
		if idx < 0 {
			delete(q.next, ch.Key)
			delete(q.wait, ch.Key)
			q.mu.Unlock()
			return
		}
		it := q.items[idx]
		q.mu.Unlock()

		msg := it.message()
		msg.Text += note
		err := m.sendOnce(ch, msg)

		q.mu.Lock()
		// 发送期间队列可能被清空或修改，按 ID 重新定位
		idx = -1
		for i := range q.items {
			if q.items[i].ID == it.ID {
				idx = i
				break
			}
		}
		if err == nil {
			if idx >= 0 {
				q.items = append(q.items[:idx], q.items[idx+1:]...)
			}
			delete(q.wait, ch.Key)
		} else {
			if idx >= 0 {
				q.items[idx].Attempts++
				q.items[idx].Error = err.Error()
			}
			w := q.wait[ch.Key] * 2
			if w < backfillMinWait {
				w = backfillMinWait
			}
			if w > backfillMaxWait {
				w = backfillMaxWait
			}
			q.wait[ch.Key] = w
			q.next[ch.Key] = m.now().Add(w)
		}
		if serr := q.saveLocked(); serr != nil {
			m.log.Warnf("保存待补推的通知失败: %v", serr)
		}
		q.mu.Unlock()

		if err != nil {
			m.log.Debugf("经渠道 %s 补推「%s」失败，稍后再试: %v", ch.Name, it.Title, err)
			return
		}
		m.log.Infof("经渠道 %s 补推「%s」成功", ch.Name, it.Title)
		m.record(Record{Time: m.now(), Type: it.Event.Type, Title: it.Title, Rule: it.Rule, Channel: ch.Name, OK: true,
			Attempts: it.Attempts + 1, Backfill: true})
	}
}

// pendingView 是待补推消息的接口输出。
type pendingView struct {
	ID       string    `json:"id"`
	Channel  string    `json:"channel"`
	Rule     string    `json:"rule"`
	Type     string    `json:"type"`
	Title    string    `json:"title"`
	Queued   time.Time `json:"queued"`
	Attempts int       `json:"attempts"`
	Error    string    `json:"error,omitempty"`
	// NextTry 为该渠道下次补推的时间，为空表示下一轮检查时就会尝试
	NextTry *time.Time `json:"nextTry,omitempty"`
}

func (m *Module) handlePending(w http.ResponseWriter, _ *http.Request) {
	q := &m.pend
	q.mu.Lock()
	out := make([]pendingView, 0, len(q.items))
	for _, it := range q.items {
		v := pendingView{ID: it.ID, Channel: it.ChannelName, Rule: it.Rule, Type: it.Event.Type, Title: it.Title,
			Queued: it.Queued, Attempts: it.Attempts, Error: it.Error}
		if next, ok := q.next[it.Channel]; ok {
			v.NextTry = &next
		}
		out = append(out, v)
	}
	q.mu.Unlock()
	httpx.OK(w, out)
}

func (m *Module) handleClearPending(w http.ResponseWriter, _ *http.Request) {
	q := &m.pend
	q.mu.Lock()
	n := len(q.items)
	q.items = nil
	q.next = map[string]time.Time{}
	q.wait = map[string]time.Duration{}
	err := q.saveLocked()
	q.mu.Unlock()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "%v", err)
		return
	}
	if n > 0 {
		m.log.Infof("已清空 %d 条待补推的通知", n)
	}
	httpx.OK(w, map[string]int{"cleared": n})
}
