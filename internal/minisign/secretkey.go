package minisign

import (
	"bytes"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"strings"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/scrypt"
)

// 私钥文件（minisign.key）格式：
//
//	untrusted comment: <说明>
//	base64(签名算法 "Ed" 2 || KDF 算法 2 || 校验算法 "B2" 2 || 盐 32 || opslimit 8 || memlimit 8 || 加密部分 104)
//
// 加密部分为「密钥 ID 8 || Ed25519 私钥 64 || 校验值 32」与 scrypt 派生的同长度密钥流异或；
// KDF 算法为 "Sc"（scrypt，有口令）或两个零字节（无口令，不加密）。
// 校验值为 BLAKE2b-256(签名算法 || 密钥 ID || 私钥)，解密后据此判断口令是否正确。
// opslimit / memlimit 沿用 libsodium crypto_pwhash_scryptsalsa208sha256 的参数，按 pickScryptParams 换算成 scrypt 的 N、r、p。

const (
	kdfScrypt   = "Sc"
	kdfNone     = "\x00\x00"
	chkBlake2b  = "B2"
	saltLen     = 32
	encLen      = keyIDLen + ed25519.PrivateKeySize + 32
	secretTotal = 2 + 2 + 2 + saltLen + 8 + 8 + encLen
)

// KDFParams 为私钥加密使用的 libsodium 口令哈希参数。
type KDFParams struct {
	OpsLimit uint64
	MemLimit uint64
}

// DefaultKDF 为 minisign -G 的默认参数（OPSLIMIT_SENSITIVE / MEMLIMIT_SENSITIVE），换算为 scrypt N=2^20、r=8、p=1，约需 1 GiB 内存。
var DefaultKDF = KDFParams{OpsLimit: 33554432, MemLimit: 1073741824}

// PrivateKey 为 minisign 私钥。
type PrivateKey struct {
	ID  KeyID
	Key ed25519.PrivateKey
}

// GenerateKey 生成新的密钥对，密钥 ID 随机。
func GenerateKey(rand io.Reader) (*PrivateKey, error) {
	_, sk, err := ed25519.GenerateKey(rand)
	if err != nil {
		return nil, err
	}
	k := &PrivateKey{Key: sk}
	if _, err := io.ReadFull(rand, k.ID[:]); err != nil {
		return nil, err
	}
	return k, nil
}

// Public 返回对应的公钥。
func (k *PrivateKey) Public() *PublicKey {
	return &PublicKey{ID: k.ID, Key: bytes.Clone(k.Key.Public().(ed25519.PublicKey))}
}

// Sign 生成预哈希（ED）签名文件内容，与 minisign -S 的默认输出相同。
// untrusted 与 trusted 为两行说明，不能包含换行。
func (k *PrivateKey) Sign(msg []byte, untrusted, trusted string) ([]byte, error) {
	if strings.ContainsAny(untrusted+trusted, "\r\n") {
		return nil, errors.New("签名说明不能包含换行")
	}
	s := &Signature{
		Algorithm:        AlgPrehashed,
		KeyID:            k.ID,
		UntrustedComment: untrusted,
		TrustedComment:   trusted,
	}
	s.Sig = ed25519.Sign(k.Key, signedMessage(AlgPrehashed, msg))
	s.GlobalSig = ed25519.Sign(k.Key, append(bytes.Clone(s.Sig), trusted...))
	return s.Encode(), nil
}

// checksum 计算私钥文件中的校验值。
func (k *PrivateKey) checksum() []byte {
	h, _ := blake2b.New256(nil)
	h.Write([]byte(AlgLegacy))
	h.Write(k.ID[:])
	h.Write(k.Key)
	return h.Sum(nil)
}

// pickScryptParams 按 libsodium 的 pickparams 把 opslimit / memlimit 换算为 scrypt 参数（返回 N 的以 2 为底的对数）。
func pickScryptParams(opslimit, memlimit uint64) (logN uint, r, p int) {
	if opslimit < 32768 {
		opslimit = 32768
	}
	r = 8
	var maxN uint64
	if opslimit < memlimit/32 {
		p = 1
		maxN = opslimit / uint64(r*4)
	} else {
		maxN = memlimit / uint64(r*128)
	}
	for logN = 1; logN < 63; logN++ {
		if uint64(1)<<logN > maxN/2 {
			break
		}
	}
	if opslimit >= memlimit/32 {
		maxrp := (opslimit / 4) / (uint64(1) << logN)
		if maxrp > 0x3fffffff {
			maxrp = 0x3fffffff
		}
		p = int(uint32(maxrp)) / r
	}
	return logN, r, p
}

// keystream 用 scrypt 从口令派生与加密部分等长的密钥流。
func keystream(password string, salt []byte, kdf KDFParams) ([]byte, error) {
	logN, r, p := pickScryptParams(kdf.OpsLimit, kdf.MemLimit)
	if logN > 30 || p < 1 {
		return nil, errors.New("私钥的口令哈希参数异常")
	}
	return scrypt.Key([]byte(password), salt, 1<<logN, r, p, encLen)
}

// EncodeSecretKey 输出 minisign 私钥文件；password 为空时不加密（等同 minisign -G -W）。
func EncodeSecretKey(k *PrivateKey, password string, kdf KDFParams, rand io.Reader) ([]byte, error) {
	raw := make([]byte, 0, secretTotal)
	raw = append(raw, AlgLegacy...)
	salt := make([]byte, saltLen)
	if password == "" {
		raw = append(raw, kdfNone...)
	} else {
		raw = append(raw, kdfScrypt...)
		if _, err := io.ReadFull(rand, salt); err != nil {
			return nil, err
		}
	}
	raw = append(raw, chkBlake2b...)
	raw = append(raw, salt...)
	raw = binary.LittleEndian.AppendUint64(raw, kdf.OpsLimit)
	raw = binary.LittleEndian.AppendUint64(raw, kdf.MemLimit)
	enc := make([]byte, 0, encLen)
	enc = append(enc, k.ID[:]...)
	enc = append(enc, k.Key...)
	enc = append(enc, k.checksum()...)
	if password != "" {
		stream, err := keystream(password, salt, kdf)
		if err != nil {
			return nil, err
		}
		subtle.XORBytes(enc, enc, stream)
	}
	raw = append(raw, enc...)
	comment := "minisign secret key"
	if password != "" {
		comment = "minisign encrypted secret key"
	}
	return []byte(untrustedPrefix + comment + "\n" + base64.StdEncoding.EncodeToString(raw) + "\n"), nil
}

// ParseSecretKey 解析并解密 minisign 私钥文件；口令错误时返回错误。
func ParseSecretKey(data []byte, password string) (*PrivateKey, error) {
	line := ""
	for _, l := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, untrustedPrefix) {
			continue
		}
		line = l
		break
	}
	raw, err := base64.StdEncoding.DecodeString(line)
	if err != nil || len(raw) != secretTotal {
		return nil, errors.New("私钥格式无效")
	}
	if string(raw[:2]) != AlgLegacy || string(raw[4:6]) != chkBlake2b {
		return nil, errors.New("私钥的算法不受支持")
	}
	kdfAlg := string(raw[2:4])
	salt := raw[6 : 6+saltLen]
	off := 6 + saltLen
	kdf := KDFParams{
		OpsLimit: binary.LittleEndian.Uint64(raw[off:]),
		MemLimit: binary.LittleEndian.Uint64(raw[off+8:]),
	}
	enc := bytes.Clone(raw[off+16:])
	switch kdfAlg {
	case kdfScrypt:
		if password == "" {
			return nil, errors.New("私钥已加密，需要口令")
		}
		stream, err := keystream(password, salt, kdf)
		if err != nil {
			return nil, err
		}
		subtle.XORBytes(enc, enc, stream)
	case kdfNone:
	default:
		return nil, errors.New("私钥的口令哈希算法不受支持")
	}
	k := &PrivateKey{Key: ed25519.PrivateKey(bytes.Clone(enc[keyIDLen : keyIDLen+ed25519.PrivateKeySize]))}
	copy(k.ID[:], enc[:keyIDLen])
	chk := enc[keyIDLen+ed25519.PrivateKeySize:]
	// minisign 0.12 生成的无口令私钥（-W）校验值全为零，此时只核对下面的公私钥一致性
	unchecked := kdfAlg == kdfNone && bytes.Equal(chk, make([]byte, len(chk)))
	if !unchecked && subtle.ConstantTimeCompare(k.checksum(), chk) != 1 {
		return nil, errors.New("私钥口令错误或文件已损坏")
	}
	// 私钥后 32 字节是公钥，必须与前 32 字节种子推导出的公钥一致
	if !bytes.Equal(ed25519.NewKeyFromSeed(k.Key.Seed()), k.Key) {
		return nil, errors.New("私钥口令错误或文件已损坏")
	}
	return k, nil
}
