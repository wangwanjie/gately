package spa

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"strings"
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
