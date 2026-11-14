// Package webauthntest 提供软件模拟的 WebAuthn 验证器，仅供测试使用。
package webauthntest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"

	"github.com/wangwanjie/gately/internal/webauthn"
)

// SoftKey 是一个内存中的 ES256 验证器。
type SoftKey struct {
	priv   *ecdsa.PrivateKey
	credID []byte
	count  uint32
	// zero 为 true 时断言中的签名计数恒为 0，模拟同步型 Passkey（iCloud 钥匙串、Google 密码管理器等）
	zero bool
}

// New 创建验证器。
func New() *SoftKey {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return &SoftKey{priv: priv, credID: id}
}

func enc(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func cborBytes(b []byte) []byte {
	return append([]byte{0x58, byte(len(b))}, b...)
}

func cborText(s string) []byte {
	return append([]byte{0x60 | byte(len(s))}, s...)
}

func authData(rpID string, flags byte, count uint32, attested []byte) []byte {
	h := sha256.Sum256([]byte(rpID))
	b := append([]byte{}, h[:]...)
	b = append(b, flags)
	b = binary.BigEndian.AppendUint32(b, count)
	return append(b, attested...)
}

func clientData(typ, challenge, origin string) string {
	b, _ := json.Marshal(map[string]string{"type": typ, "challenge": challenge, "origin": origin})
	return string(b)
}

// 标志位：用户在场 | 用户已验证
const (
	flagsAssert   = 0x01 | 0x04
	flagsRegister = flagsAssert | 0x40
)

// Register 生成注册响应。
func (k *SoftKey) Register(rpID, challenge, origin string) webauthn.RegistrationResponse {
	pub, _ := k.priv.PublicKey.ECDH()
	raw := pub.Bytes()
	cose := []byte{0xa5, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21}
	cose = append(cose, cborBytes(raw[1:33])...)
	cose = append(cose, 0x22)
	cose = append(cose, cborBytes(raw[33:65])...)
	attested := append(make([]byte, 16), 0, byte(len(k.credID)))
	attested = append(attested, k.credID...)
	attested = append(attested, cose...)
	var obj []byte
	obj = append(obj, 0xa3)
	obj = append(obj, cborText("fmt")...)
	obj = append(obj, cborText("none")...)
	obj = append(obj, cborText("attStmt")...)
	obj = append(obj, 0xa0)
	obj = append(obj, cborText("authData")...)
	obj = append(obj, cborBytes(authData(rpID, flagsRegister, 0, attested))...)
	var r webauthn.RegistrationResponse
	r.ID, r.Type = enc(k.credID), "public-key"
	r.Response.ClientDataJSON = enc([]byte(clientData("webauthn.create", challenge, origin)))
	r.Response.AttestationObject = enc(obj)
	return r
}

// SetCount 设置验证器内部的签名计数，用于模拟被克隆的凭据（计数落后于服务端记录）。
func (k *SoftKey) SetCount(n uint32) {
	k.count = n
}

// SetZeroCounter 让验证器的签名计数恒为 0。
func (k *SoftKey) SetZeroCounter(on bool) {
	k.zero = on
}

// ID 返回凭据 ID（base64url）。
func (k *SoftKey) ID() string {
	return enc(k.credID)
}

// Assert 生成登录断言，每次调用签名计数加一。
func (k *SoftKey) Assert(rpID, challenge, origin string) webauthn.AssertionResponse {
	k.count++
	count := k.count
	if k.zero {
		count = 0
	}
	ad := authData(rpID, flagsAssert, count, nil)
	cd := clientData("webauthn.get", challenge, origin)
	cdHash := sha256.Sum256([]byte(cd))
	digest := sha256.Sum256(append(append([]byte{}, ad...), cdHash[:]...))
	sig, _ := ecdsa.SignASN1(rand.Reader, k.priv, digest[:])
	var r webauthn.AssertionResponse
	r.ID, r.Type = enc(k.credID), "public-key"
	r.Response.ClientDataJSON = enc([]byte(cd))
	r.Response.AuthenticatorData = enc(ad)
	r.Response.Signature = enc(sig)
	return r
}
