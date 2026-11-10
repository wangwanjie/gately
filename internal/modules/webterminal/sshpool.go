package webterminal

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// poolIdleTimeout 是连接池中无人使用的 SSH 连接的保留时间，测试可调小。
var poolIdleTimeout = 5 * time.Minute

// poolEntry 是连接池中的一条 SSH 连接。
type poolEntry struct {
	client *ssh.Client
	err    error
	// ready 在拨号完成（成功或失败）后关闭
	ready chan struct{}
	refs  int
	timer *time.Timer
	// dead 表示已从池中摘除，引用归零后关闭
	dead bool
}

// sshPool 按连接 key 复用 SSH 连接（引用计数，空闲超时关闭）。
type sshPool struct {
	mu      sync.Mutex
	entries map[string]*poolEntry
}

func newSSHPool() *sshPool { return &sshPool{entries: map[string]*poolEntry{}} }

// closeClient 关闭条目的底层连接。
func (e *poolEntry) closeClient() {
	if e.client != nil {
		_ = e.client.Close()
	}
}

// remove 把条目从池中摘除；无人引用时立即关闭。
func (p *sshPool) remove(key string, e *poolEntry) {
	p.mu.Lock()
	if p.entries[key] == e {
		delete(p.entries, key)
	}
	e.dead = true
	idle := e.refs == 0
	if e.timer != nil {
		e.timer.Stop()
	}
	p.mu.Unlock()
	if idle {
		e.closeClient()
	}
}

// drop 清理指定连接的池内连接（连接配置变更或删除时调用）。
func (p *sshPool) drop(key string) {
	p.mu.Lock()
	e := p.entries[key]
	p.mu.Unlock()
	if e != nil {
		p.remove(key, e)
	}
}

// closeAll 关闭池内全部连接。
func (p *sshPool) closeAll() {
	p.mu.Lock()
	keys := make([]string, 0, len(p.entries))
	for k := range p.entries {
		keys = append(keys, k)
	}
	p.mu.Unlock()
	for _, k := range keys {
		p.drop(k)
	}
}

// acquireSSH 取得指定 SSH 连接的客户端，release 必须在用完后调用。
// 同一连接共用一条 SSH 连接，全部释放 5 分钟后关闭；连接断开时自动从池中移除。
func (m *Module) acquireSSH(ctx context.Context, key string) (client *ssh.Client, release func(), err error) {
	conn, ok := m.findConn(key)
	if !ok || conn.Type != TypeSSH {
		return nil, nil, errors.New("连接不存在或不是 SSH 连接")
	}
	p := m.pool
	for {
		p.mu.Lock()
		e := p.entries[key]
		if e != nil {
			e.refs++
			if e.timer != nil {
				e.timer.Stop()
				e.timer = nil
			}
			p.mu.Unlock()
			select {
			case <-e.ready:
			case <-ctx.Done():
				m.releaseEntry(key, e)
				return nil, nil, ctx.Err()
			}
			if e.err != nil {
				m.releaseEntry(key, e)
				return nil, nil, e.err
			}
			return e.client, func() { m.releaseEntry(key, e) }, nil
		}
		// 池中没有：由当前调用负责拨号，并发调用等待同一个条目
		e = &poolEntry{ready: make(chan struct{}), refs: 1}
		p.entries[key] = e
		p.mu.Unlock()

		c, hk, derr := m.dialSSH(ctx, &conn)
		if derr != nil {
			e.err = derr
			close(e.ready)
			m.pool.remove(key, e)
			return nil, nil, derr
		}
		m.recordHostKey(key, hk)
		e.client = c
		close(e.ready)
		// 底层连接断开后自动摘除，下次获取会重新拨号
		go func() {
			_ = c.Wait()
			m.pool.remove(key, e)
		}()
		return c, func() { m.releaseEntry(key, e) }, nil
	}
}

// releaseEntry 归还引用；归零后启动空闲计时。
func (m *Module) releaseEntry(key string, e *poolEntry) {
	p := m.pool
	p.mu.Lock()
	e.refs--
	if e.refs > 0 {
		p.mu.Unlock()
		return
	}
	if e.dead {
		p.mu.Unlock()
		e.closeClient()
		return
	}
	e.timer = time.AfterFunc(poolIdleTimeout, func() {
		p.mu.Lock()
		stale := e.refs == 0 && !e.dead
		p.mu.Unlock()
		if stale {
			p.remove(key, e)
		}
	})
	p.mu.Unlock()
}
