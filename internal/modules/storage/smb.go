package storage

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/cloudsoda/go-smb2"
)

// smbDialTimeout 限制建立 TCP 连接与完成认证的时间，避免地址不可达时请求长时间挂起。
const smbDialTimeout = 15 * time.Second

// parseSMB 解析 smb://主机[:端口]/共享名[/子目录]，返回连接地址、共享名与共享内子目录（不含首尾斜杠）。
func parseSMB(raw string) (addr, share, sub string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, "smb") || u.Hostname() == "" {
		return "", "", "", errors.New("SMB 地址格式为 smb://主机/共享名[/子目录]")
	}
	segs := strings.SplitN(strings.Trim(path.Clean("/"+u.Path), "/"), "/", 2)
	if segs[0] == "" {
		return "", "", "", errors.New("SMB 地址缺少共享名，格式为 smb://主机/共享名[/子目录]")
	}
	share = segs[0]
	if len(segs) == 2 {
		sub = segs[1]
	}
	port := u.Port()
	if port == "" {
		port = "445"
	}
	return net.JoinHostPort(u.Hostname(), port), share, sub, nil
}

// smbBackend 通过 SMB2/3 访问 Windows 共享、NAS 或 Samba。连接惰性建立，
// 断开（服务端空闲超时、NAS 重启等）后在下一次操作时自动重连一次。
type smbBackend struct {
	addr, share, sub       string
	user, password, domain string

	mu   sync.Mutex
	conn net.Conn
	sess *smb2.Session
	fs   *smb2.Share
}

func newSMB(rawURL, user, password, domain string) (*smbBackend, error) {
	addr, share, sub, err := parseSMB(rawURL)
	if err != nil {
		return nil, err
	}
	return &smbBackend{addr: addr, share: share, sub: sub, user: user, password: password, domain: domain}, nil
}

// rel 把挂载内路径转换为共享内的相对路径；go-smb2 不接受开头的分隔符，共享根目录为空串。
func (s *smbBackend) rel(p string) string {
	return strings.TrimPrefix(path.Join("/", s.sub, cleanPath(p)), "/")
}

// get 返回已挂载的共享，未连接时建立连接。
func (s *smbBackend) get() (*smb2.Share, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fs != nil {
		return s.fs, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), smbDialTimeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return nil, err
	}
	// 未填写用户名时以 guest 身份连接，与 Windows 资源管理器的匿名访问一致
	user := s.user
	if user == "" {
		user = "guest"
	}
	dialer := &smb2.Dialer{Initiator: &smb2.NTLMInitiator{User: user, Password: s.password, Domain: s.domain}}
	sess, err := dialer.DialConn(ctx, conn, s.addr)
	if err != nil {
		conn.Close()
		return nil, smbErr(err)
	}
	// 拨号用的 ctx 在函数返回后取消，会话后续操作（如注销）改用不会过期的 ctx
	sess = sess.WithContext(context.Background())
	fs, err := sess.Mount(s.share)
	if err != nil {
		_ = sess.Logoff()
		conn.Close()
		return nil, smbErr(err)
	}
	s.conn, s.sess, s.fs = conn, sess, fs
	return fs, nil
}

// drop 关闭连接；仅当 fs 仍是当前连接时才关闭，避免误关其他请求刚建立的新连接。
func (s *smbBackend) drop(fs *smb2.Share) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fs != nil && s.fs != fs {
		return
	}
	s.closeLocked()
}

func (s *smbBackend) closeLocked() {
	if s.fs != nil {
		_ = s.fs.Umount()
	}
	if s.sess != nil {
		_ = s.sess.Logoff()
	}
	if s.conn != nil {
		_ = s.conn.Close()
	}
	s.conn, s.sess, s.fs = nil, nil, nil
}

// do 执行一次共享操作；连接已失效时重连并重试一次。fn 必须可安全重复执行。
func (s *smbBackend) do(fn func(fs *smb2.Share) error) error {
	for attempt := 0; ; attempt++ {
		fs, err := s.get()
		if err != nil {
			return err
		}
		err = fn(fs)
		if err != nil && isConnErr(err) {
			s.drop(fs)
			if attempt == 0 {
				continue
			}
		}
		return smbErr(err)
	}
}

// isConnErr 判断错误是否由连接断开或会话失效引起，此时需要重连。
func isConnErr(err error) bool {
	var te *smb2.TransportError
	if errors.As(err, &te) || errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var re *smb2.ResponseError
	if errors.As(err, &re) {
		switch re.Code {
		// STATUS_USER_SESSION_DELETED、STATUS_NETWORK_NAME_DELETED、STATUS_NETWORK_SESSION_EXPIRED
		case 0xC0000203, 0xC00000C9, 0xC000035C:
			return true
		}
	}
	return false
}

// smbErr 把常见的认证与共享错误转换为易懂的提示，路径类错误保留原值以便映射 HTTP 状态码。
func smbErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return errNotFound
	}
	var re *smb2.ResponseError
	if errors.As(err, &re) {
		switch re.Code {
		// STATUS_LOGON_FAILURE、STATUS_ACCOUNT_DISABLED、STATUS_PASSWORD_EXPIRED
		case 0xC000006D, 0xC0000072, 0xC0000071:
			return errors.New("SMB 认证失败，请检查用户名与密码")
		// STATUS_BAD_NETWORK_NAME
		case 0xC00000CC:
			return errors.New("SMB 共享不存在")
		}
	}
	return err
}

func (s *smbBackend) List(p string) ([]Entry, error) {
	var out []Entry
	err := s.do(func(fs *smb2.Share) error {
		infos, err := fs.ReadDir(s.rel(p))
		if err != nil {
			return err
		}
		out = make([]Entry, 0, len(infos))
		for _, fi := range infos {
			out = append(out, Entry{Name: fi.Name(), IsDir: fi.IsDir(), Size: fi.Size(), ModTime: fi.ModTime()})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortEntries(out)
	return out, nil
}

func (s *smbBackend) Open(p string) (io.ReadCloser, int64, time.Time, error) {
	var f *smb2.File
	var st os.FileInfo
	err := s.do(func(fs *smb2.Share) error {
		var err error
		if f, err = fs.Open(s.rel(p)); err != nil {
			return err
		}
		if st, err = f.Stat(); err != nil {
			f.Close()
			return err
		}
		return nil
	})
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	if st.IsDir() {
		f.Close()
		return nil, 0, time.Time{}, errors.New("目录无法下载")
	}
	return f, st.Size(), st.ModTime(), nil
}

func (s *smbBackend) Put(p string, r io.Reader, _ int64) error {
	var f *smb2.File
	// 只有打开文件这一步可以重试，数据开始写入后 r 已被消费，无法重放
	err := s.do(func(fs *smb2.Share) error {
		var err error
		f, err = fs.OpenFile(s.rel(p), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		return err
	})
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return smbErr(err)
	}
	return smbErr(f.Close())
}

func (s *smbBackend) Mkdir(p string) error {
	return s.do(func(fs *smb2.Share) error {
		return fs.Mkdir(s.rel(p), 0o755)
	})
}

func (s *smbBackend) Remove(p string) error {
	if cleanPath(p) == "/" {
		return errors.New("不能删除挂载根目录")
	}
	return s.do(func(fs *smb2.Share) error {
		return fs.RemoveAll(s.rel(p))
	})
}

func (s *smbBackend) Rename(from, to string) error {
	if cleanPath(from) == "/" || cleanPath(to) == "/" {
		return errors.New("不能重命名挂载根目录")
	}
	return s.do(func(fs *smb2.Share) error {
		return fs.Rename(s.rel(from), s.rel(to))
	})
}

func (s *smbBackend) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked()
	return nil
}
