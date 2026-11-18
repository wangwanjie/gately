package spa

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
)

type vectorFile struct {
	EncKey  string `json:"encKey"`
	HMACKey string `json:"hmacKey"`
	Vectors []struct {
		Name      string `json:"name"`
		Packet    string `json:"packet"`
		HMAC      string `json:"hmac"`
		Type      int    `json:"type"`
		User      string `json:"user"`
		Timestamp int64  `json:"timestamp"`
		Message   string `json:"message"`
		Timeout   int    `json:"timeout"`
		NAT       string `json:"nat"`
	} `json:"vectors"`
}

func loadVectors(t *testing.T) (vectorFile, Key) {
	t.Helper()
	raw, err := os.ReadFile("testdata/fwknop-2.6.11.json")
	if err != nil {
		t.Fatal(err)
	}
	var f vectorFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	enc, _ := base64.StdEncoding.DecodeString(f.EncKey)
	mac, _ := base64.StdEncoding.DecodeString(f.HMACKey)
	return f, Key{Enc: enc, HMAC: mac}
}

// 真实 fwknop 客户端生成的报文（各种摘要、HMAC 算法、客户端超时、命令与 NAT 消息）都能正确解析。
func TestDecodeFwknopVectors(t *testing.T) {
	f, k := loadVectors(t)
	if len(f.Vectors) < 10 {
		t.Fatalf("测试向量太少: %d", len(f.Vectors))
	}
	for _, v := range f.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			key := k
			key.HMACType = v.HMAC
			m, err := Decode([]byte(v.Packet), key)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if m.Type != v.Type || m.User != v.User || m.Timestamp != v.Timestamp || m.ClientTimeout != v.Timeout {
				t.Fatalf("字段不符: %+v", m)
			}
			addr, rest, _ := strings.Cut(v.Message, ",")
			if m.Addr.String() != addr {
				t.Fatalf("地址 %s，期望 %s", m.Addr, addr)
			}
			switch m.Type {
			case CommandMsg:
				if m.Command != rest {
					t.Fatalf("命令 %q，期望 %q", m.Command, rest)
				}
			default:
				var got []string
				for _, a := range m.Access {
					got = append(got, a.String())
				}
				if strings.Join(got, ",") != rest {
					t.Fatalf("端口 %v，期望 %s", got, rest)
				}
			}
			if v.NAT != "<NULL>" && m.NAT != v.NAT {
				t.Fatalf("NAT %q，期望 %q", m.NAT, v.NAT)
			}
			if len(m.Random) != 16 || m.Digest == "" || m.Version != Version {
				t.Fatalf("随机数/摘要/版本异常: %+v", m)
			}
		})
	}
}

// HMAC 算法不一致、密钥错误、报文被篡改时在解密前拒绝；加密密钥错误时 HMAC 通过但解密失败。
func TestDecodeRejects(t *testing.T) {
	f, k := loadVectors(t)
	pkt := []byte(f.Vectors[0].Packet)
	if f.Vectors[0].HMAC != HMACSHA256 {
		t.Fatal("第一个向量应为 sha256")
	}
	bad := k
	bad.HMACType = HMACSHA512
	if _, err := Decode(pkt, bad); !errors.Is(err, ErrHMAC) && !errors.Is(err, ErrFormat) {
		t.Fatalf("HMAC 算法不一致应失败: %v", err)
	}
	bad = k
	bad.HMAC = bytes.Repeat([]byte{1}, 64)
	if _, err := Decode(pkt, bad); !errors.Is(err, ErrHMAC) {
		t.Fatalf("HMAC 密钥错误应为 ErrHMAC: %v", err)
	}
	bad = k
	bad.Enc = bytes.Repeat([]byte{2}, 32)
	if _, err := Decode(pkt, bad); !errors.Is(err, ErrDecrypt) && !errors.Is(err, ErrMessage) {
		t.Fatalf("加密密钥错误应解密失败: %v", err)
	}
	for i := range pkt {
		mod := bytes.Clone(pkt)
		if mod[i] == 'A' {
			mod[i] = 'B'
		} else {
			mod[i] = 'A'
		}
		if _, err := Decode(mod, k); err == nil {
			t.Fatalf("篡改第 %d 个字符后仍能解析", i)
		}
	}
	for _, p := range []string{"", "hello", strings.Repeat("A", MaxPacketLen+1), saltedB64 + string(pkt), string(pkt) + "\n", string(pkt[:len(pkt)-1]) + "="} {
		if _, err := Decode([]byte(p), k); err == nil {
			t.Fatalf("非法报文被接受: %q", p)
		}
	}
}

func testKey(t *testing.T, typ string) Key {
	t.Helper()
	k := Key{Enc: make([]byte, 32), HMAC: make([]byte, 64), HMACType: typ}
	rand.Read(k.Enc)
	rand.Read(k.HMAC)
	return k
}

// Encode 生成的报文能被 Decode 解析，各 HMAC 算法与 IPv6 地址都可往返。
func TestEncodeRoundTrip(t *testing.T) {
	for _, typ := range []string{"", HMACSHA256, HMACSHA384, HMACSHA512} {
		k := testKey(t, typ)
		in := Message{User: "bob", Timestamp: 1700000000, Type: ClientTimeoutAccessMsg, Addr: netip.MustParseAddr("2001:db8::7"),
			Access: []Access{{"tcp", 22}, {"udp", 51820}}, ClientTimeout: 45}
		pkt, err := Encode(in, k, rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if Prefilter(pkt) != nil {
			t.Fatalf("生成的报文未通过预检: %s", pkt)
		}
		m, err := Decode(pkt, k)
		if err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		if m.User != "bob" || m.ClientTimeout != 45 || m.Addr != in.Addr || len(m.Access) != 2 || m.Access[1] != (Access{"udp", 51820}) {
			t.Fatalf("往返不一致: %+v", m)
		}
		other := testKey(t, typ)
		if _, err := Decode(pkt, other); err == nil {
			t.Fatal("其他密钥不应能解析")
		}
	}
}

// 密钥长度与算法校验。
func TestKeyValidate(t *testing.T) {
	cases := []struct {
		k  Key
		ok bool
	}{
		{Key{Enc: make([]byte, 32), HMAC: make([]byte, 64)}, true},
		{Key{Enc: make([]byte, 16), HMAC: make([]byte, 16), HMACType: HMACSHA384}, true},
		{Key{Enc: make([]byte, 15), HMAC: make([]byte, 64)}, false},
		{Key{Enc: make([]byte, 33), HMAC: make([]byte, 64)}, false},
		{Key{Enc: make([]byte, 32), HMAC: make([]byte, 8)}, false},
		{Key{Enc: make([]byte, 32), HMAC: make([]byte, 129)}, false},
		{Key{Enc: make([]byte, 32), HMAC: make([]byte, 64), HMACType: "md5"}, false},
	}
	for i, c := range cases {
		if err := c.k.Validate(); (err == nil) != c.ok {
			t.Fatalf("#%d: %v", i, err)
		}
	}
}

func TestParseAccess(t *testing.T) {
	for _, s := range []string{"tcp/22", "UDP/51820", " tcp/65535 "} {
		if _, err := ParseAccess(s); err != nil {
			t.Fatalf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"icmp/1", "tcp/0", "tcp/65536", "tcp/", "tcp22", "tcp/-1", "tcp/1e3", "tcp/000022x"} {
		if _, err := ParseAccess(s); err == nil {
			t.Fatalf("%q 应被拒绝", s)
		}
	}
}

// 任意输入都不会让解析崩溃，且不知道密钥时无法构造出能通过的报文。
func FuzzDecode(f *testing.F) {
	raw, _ := os.ReadFile("testdata/fwknop-2.6.11.json")
	var vf vectorFile
	_ = json.Unmarshal(raw, &vf)
	for _, v := range vf.Vectors {
		f.Add([]byte(v.Packet))
	}
	f.Add([]byte(strings.Repeat("A", 200)))
	enc, _ := base64.StdEncoding.DecodeString(vf.EncKey)
	mac, _ := base64.StdEncoding.DecodeString(vf.HMACKey)
	k := Key{Enc: enc, HMAC: mac}
	f.Fuzz(func(t *testing.T, b []byte) {
		m, err := Decode(b, k)
		if err == nil && m == nil {
			t.Fatal("成功时消息不能为空")
		}
	})
}

// DecodeAny 返回第一个 HMAC 通过的密钥下标；算法不同的密钥混在一起时按各自的 HMAC 长度拆分报文。
func TestDecodeAny(t *testing.T) {
	k256, k512, other := testKey(t, HMACSHA256), testKey(t, HMACSHA512), testKey(t, HMACSHA256)
	in := Message{User: "alice", Timestamp: 1700000000, Type: AccessMsg, Addr: netip.MustParseAddr("203.0.113.5"), Access: []Access{{"tcp", 22}}}
	pkt, err := Encode(in, k512, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	i, m, err := DecodeAny(pkt, []Key{other, k256, k512})
	if err != nil || i != 2 || m == nil || m.User != "alice" {
		t.Fatalf("应匹配第 3 个密钥: i=%d m=%v err=%v", i, m, err)
	}
	// 没有密钥匹配
	if i, _, err := DecodeAny(pkt, []Key{other, k256}); i != -1 || !errors.Is(err, ErrHMAC) {
		t.Fatalf("不应匹配: i=%d err=%v", i, err)
	}
	// 空列表与预检失败
	if i, _, err := DecodeAny(pkt, nil); i != -1 || !errors.Is(err, ErrFormat) {
		t.Fatalf("空密钥列表: i=%d err=%v", i, err)
	}
	if i, _, err := DecodeAny([]byte("not spa"), []Key{k512}); i != -1 || !errors.Is(err, ErrFormat) {
		t.Fatalf("非 SPA 报文: i=%d err=%v", i, err)
	}
	// HMAC 密钥相同、加密密钥不同：返回该下标与 ErrDecrypt
	bad := k512
	bad.Enc = other.Enc
	if i, _, err := DecodeAny(pkt, []Key{bad}); i != 0 || !errors.Is(err, ErrDecrypt) {
		t.Fatalf("加密密钥不一致: i=%d err=%v", i, err)
	}
	// 末尾 HMAC 值改动一个字符（含只改动多余比特的写法）不能通过
	for _, c := range []byte{'A', 'B', 'z'} {
		mut := append([]byte{}, pkt...)
		if mut[len(mut)-1] == c {
			continue
		}
		mut[len(mut)-1] = c
		if i, _, _ := DecodeAny(mut, []Key{k512}); i != -1 {
			t.Fatalf("篡改的 HMAC 值被接受: %q", mut[len(mut)-4:])
		}
	}
}

// DecodeSet 复用各密钥的 HMAC 对象：多个协程交替校验匹配与不匹配的报文，结果始终与 DecodeAny 相同。
func TestDecodeSetReuse(t *testing.T) {
	k256, k512, other := testKey(t, HMACSHA256), testKey(t, HMACSHA512), testKey(t, HMACSHA256)
	keys := []Key{other, k256, k512}
	set := NewKeySet(keys)
	in := Message{User: "alice", Timestamp: 1700000000, Type: AccessMsg, Access: []Access{{"tcp", 22}}}
	var pkts [][]byte
	for _, k := range []Key{k256, k512, other} {
		pkt, err := Encode(in, k, rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		pkts = append(pkts, pkt)
	}
	pkts = append(pkts, []byte(strings.Repeat("A", 200)))
	wants := make([]int, len(pkts))
	for j, p := range pkts {
		wants[j], _, _ = DecodeAny(p, keys)
	}
	if wants[0] != 1 || wants[1] != 2 || wants[2] != 0 || wants[3] != -1 {
		t.Fatalf("参照结果不符: %v", wants)
	}
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range 200 {
				j := (g + n) % len(pkts)
				i, m, err := DecodeSet(pkts[j], set)
				if i != wants[j] {
					t.Errorf("报文 %d 匹配到 %d，应为 %d", j, i, wants[j])
					return
				}
				if i >= 0 && (err != nil || m.User != "alice") {
					t.Errorf("报文 %d 解码失败: %v", j, err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if i, _, err := DecodeSet(pkts[0], nil); i != -1 || !errors.Is(err, ErrFormat) {
		t.Fatalf("nil KeySet: i=%d err=%v", i, err)
	}
}

// BenchmarkDecodeSetForged 对比伪造报文（HMAC 全部不匹配）逐个密钥校验时，每次新建与复用 HMAC 对象的开销。
func BenchmarkDecodeSetForged(b *testing.B) {
	keys := make([]Key, 64)
	for i := range keys {
		keys[i] = Key{Enc: bytes.Repeat([]byte{byte(i + 1)}, 32), HMAC: bytes.Repeat([]byte{byte(i + 2)}, 64), HMACType: HMACSHA256}
	}
	pkt := []byte(strings.Repeat("A", 200))
	b.Run("DecodeAny", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_, _, _ = DecodeAny(pkt, keys)
		}
	})
	b.Run("DecodeSet", func(b *testing.B) {
		set := NewKeySet(keys)
		b.ReportAllocs()
		for range b.N {
			_, _, _ = DecodeSet(pkt, set)
		}
	})
}

// refDecode 是改为 DecodeAny 之前的单密钥解码实现（HMAC 值按 base64 字符串比较），作为模糊测试的参照。
func refDecode(packet []byte, k Key) (*Message, error) {
	if err := Prefilter(packet); err != nil {
		return nil, err
	}
	algo, ok := hmacAlgos[k.hmacType()]
	if !ok || len(k.Enc) == 0 || len(k.Enc) > MaxEncKeyLen || len(k.HMAC) == 0 {
		return nil, ErrHMAC
	}
	if len(packet) <= algo.b64ln+MinPacketLen/2 {
		return nil, ErrFormat
	}
	body, tag := packet[:len(packet)-algo.b64ln], packet[len(packet)-algo.b64ln:]
	full := append([]byte(saltedB64), body...)
	mac := hmac.New(algo.fn, k.HMAC)
	mac.Write(full)
	if subtle.ConstantTimeCompare([]byte(base64.RawStdEncoding.EncodeToString(mac.Sum(nil))), tag) != 1 {
		return nil, ErrHMAC
	}
	return decrypt(full, k)
}

// DecodeAny 与逐个调用原实现的结果一致（模糊测试）。
func FuzzDecodeAny(f *testing.F) {
	k1 := Key{Enc: []byte("0123456789abcdef0123456789abcdef"), HMAC: bytes.Repeat([]byte{7}, 64), HMACType: HMACSHA256}
	k2 := Key{Enc: []byte("fedcba9876543210fedcba9876543210"), HMAC: bytes.Repeat([]byte{9}, 64), HMACType: HMACSHA512}
	for _, k := range []Key{k1, k2} {
		pkt, _ := Encode(Message{User: "u", Timestamp: 1700000000, Type: AccessMsg, Access: []Access{{"tcp", 22}}}, k, rand.Reader)
		f.Add(pkt)
	}
	f.Add([]byte(strings.Repeat("A", 200)))
	f.Fuzz(func(t *testing.T, b []byte) {
		keys := []Key{k1, k2}
		i, _, err := DecodeAny(b, keys)
		// 复用 HMAC 对象的 DecodeSet 结果相同（同一 KeySet 连用两次，确认重置后的对象与新建的一致）
		set := NewKeySet(keys)
		for range 2 {
			if si, _, serr := DecodeSet(b, set); si != i || (serr == nil) != (err == nil) {
				t.Fatalf("DecodeSet 与 DecodeAny 不一致: %d/%v，%d/%v", si, serr, i, err)
			}
		}
		want := -1
		var wantErr error
		for j, k := range keys {
			_, e := refDecode(b, k)
			if errors.Is(e, ErrHMAC) || errors.Is(e, ErrFormat) {
				continue
			}
			want, wantErr = j, e
			break
		}
		if i != want {
			t.Fatalf("下标 %d，逐个解码为 %d", i, want)
		}
		if want >= 0 && (err == nil) != (wantErr == nil) {
			t.Fatalf("错误不一致: %v / %v", err, wantErr)
		}
	})
}
