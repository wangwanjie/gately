// Package webauthn 实现 Passkey（WebAuthn）登录所需的最小协议：注册（attestation 为 none）与断言校验。
//
// 只支持 ES256（ECDSA P-256 + SHA-256）凭据，这是所有平台验证器与安全密钥都支持的算法。
// 协议依赖少、代码量小，便于审计；密钥只保存公钥，私钥始终留在用户的设备里。
package webauthn

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"
)

// 认证器数据标志位
const (
	flagUserPresent  = 0x01
	flagUserVerified = 0x04
	flagAttested     = 0x40
)

// coseES256 是 COSE 算法标识：ECDSA w/ SHA-256。
const coseES256 = -7

// Credential 是一个已注册的 Passkey，只含公钥信息。
type Credential struct {
	// ID 为凭据 ID（base64url）
	ID   string `json:"id"`
	Name string `json:"name"`
	// RPID 为注册时使用的 Relying Party ID；管理员 Passkey 留空（取访问域名），站点访客 Passkey 记录它，
	// 单点登录域变化后旧凭据不再匹配，不会被其他域名误用
	RPID string `json:"rpId,omitempty"`
	// X、Y 为 P-256 公钥坐标（base64url）
	X         string    `json:"x,omitempty"`
	Y         string    `json:"y,omitempty"`
	SignCount uint32    `json:"signCount,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	LastUsed  time.Time `json:"lastUsed,omitzero"`
}

// RegistrationResponse 是浏览器 navigator.credentials.create 的结果（二进制字段为 base64url）。
type RegistrationResponse struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Response struct {
		ClientDataJSON    string `json:"clientDataJSON"`
		AttestationObject string `json:"attestationObject"`
	} `json:"response"`
}

// AssertionResponse 是浏览器 navigator.credentials.get 的结果。
type AssertionResponse struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Response struct {
		ClientDataJSON    string `json:"clientDataJSON"`
		AuthenticatorData string `json:"authenticatorData"`
		Signature         string `json:"signature"`
	} `json:"response"`
}

// NewChallenge 生成 32 字节随机挑战（base64url）。
func NewChallenge() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// RPID 从请求的 Host 提取 Relying Party ID（去掉端口）。
func RPID(host string) string {
	if h, _, ok := strings.Cut(host, "]"); ok && strings.HasPrefix(h, "[") {
		return strings.TrimPrefix(h, "[")
	}
	h, _, _ := strings.Cut(host, ":")
	return strings.ToLower(h)
}

type clientData struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Origin    string `json:"origin"`
}

// checkClientData 校验 clientDataJSON 的类型、挑战与来源，返回其 SHA-256。
func checkClientData(raw, wantType, challenge, host string) ([]byte, error) {
	var cd clientData
	if err := json.Unmarshal([]byte(raw), &cd); err != nil {
		return nil, errors.New("clientData 无法解析")
	}
	if cd.Type != wantType {
		return nil, errors.New("clientData 类型不符")
	}
	if cd.Challenge != challenge {
		return nil, errors.New("挑战不匹配或已过期")
	}
	u, err := url.Parse(cd.Origin)
	if err != nil || !strings.EqualFold(u.Host, host) || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("来源与当前访问地址不一致")
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:], nil
}

func b64(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

// authData 是解析后的认证器数据头部。
type authData struct {
	rpIDHash  []byte
	flags     byte
	signCount uint32
	// rest 为头部之后的内容（注册时是 attestedCredentialData）
	rest []byte
}

func parseAuthData(b []byte) (*authData, error) {
	if len(b) < 37 {
		return nil, errors.New("认证器数据过短")
	}
	return &authData{rpIDHash: b[:32], flags: b[32], signCount: binary.BigEndian.Uint32(b[33:37]), rest: b[37:]}, nil
}

func (a *authData) check(rpID string, needUV bool) error {
	want := sha256.Sum256([]byte(rpID))
	if !bytes.Equal(a.rpIDHash, want[:]) {
		return errors.New("RP ID 不匹配")
	}
	if a.flags&flagUserPresent == 0 {
		return errors.New("缺少用户在场确认")
	}
	if needUV && a.flags&flagUserVerified == 0 {
		return errors.New("缺少用户验证（指纹、面容或 PIN）")
	}
	return nil
}

// ParseRegistration 校验注册响应并提取公钥。host 为请求的 Host（含端口），用于校验来源与 RP ID。
func ParseRegistration(resp RegistrationResponse, challenge, host, name string) (*Credential, error) {
	return ParseRegistrationRP(resp, challenge, host, RPID(host), name)
}

// ParseRegistrationRP 与 ParseRegistration 相同，但 RP ID 由调用方指定（例如单点登录的父域）。
// 调用方负责保证 rpID 是 host 的域名本身或其父域。
func ParseRegistrationRP(resp RegistrationResponse, challenge, host, rpID, name string) (*Credential, error) {
	if resp.Type != "public-key" {
		return nil, errors.New("凭据类型无效")
	}
	cdRaw, err := b64(resp.Response.ClientDataJSON)
	if err != nil {
		return nil, errors.New("clientData 编码无效")
	}
	if _, err := checkClientData(string(cdRaw), "webauthn.create", challenge, host); err != nil {
		return nil, err
	}
	attRaw, err := b64(resp.Response.AttestationObject)
	if err != nil {
		return nil, errors.New("attestationObject 编码无效")
	}
	v, n, err := cborDecode(attRaw)
	if err != nil || n != len(attRaw) {
		return nil, errors.New("attestationObject 无法解析")
	}
	obj, ok := v.(map[any]any)
	if !ok {
		return nil, errors.New("attestationObject 结构无效")
	}
	adBytes, ok := obj["authData"].([]byte)
	if !ok {
		return nil, errors.New("缺少 authData")
	}
	ad, err := parseAuthData(adBytes)
	if err != nil {
		return nil, err
	}
	if err := ad.check(rpID, true); err != nil {
		return nil, err
	}
	if ad.flags&flagAttested == 0 || len(ad.rest) < 18 {
		return nil, errors.New("缺少凭据数据")
	}
	idLen := int(binary.BigEndian.Uint16(ad.rest[16:18]))
	if len(ad.rest) < 18+idLen {
		return nil, errors.New("凭据 ID 长度无效")
	}
	credID := ad.rest[18 : 18+idLen]
	keyVal, _, err := cborDecode(ad.rest[18+idLen:])
	if err != nil {
		return nil, errors.New("公钥无法解析")
	}
	x, y, err := coseES256Key(keyVal)
	if err != nil {
		return nil, err
	}
	if idLen == 0 || idLen > 1023 {
		return nil, errors.New("凭据 ID 长度无效")
	}
	return &Credential{
		ID:        base64.RawURLEncoding.EncodeToString(credID),
		Name:      name,
		RPID:      rpID,
		X:         base64.RawURLEncoding.EncodeToString(x),
		Y:         base64.RawURLEncoding.EncodeToString(y),
		SignCount: ad.signCount,
		CreatedAt: time.Now(),
	}, nil
}

// coseES256Key 从 COSE_Key 中取出 P-256 公钥坐标，并校验算法、曲线与点在曲线上。
func coseES256Key(v any) (x, y []byte, err error) {
	m, ok := v.(map[any]any)
	if !ok {
		return nil, nil, errors.New("公钥结构无效")
	}
	if kty, _ := m[int64(1)].(int64); kty != 2 {
		return nil, nil, errors.New("仅支持 EC2 公钥")
	}
	if alg, _ := m[int64(3)].(int64); alg != coseES256 {
		return nil, nil, fmt.Errorf("仅支持 ES256 算法，实际为 %d", alg)
	}
	if crv, _ := m[int64(-1)].(int64); crv != 1 {
		return nil, nil, errors.New("仅支持 P-256 曲线")
	}
	x, _ = m[int64(-2)].([]byte)
	y, _ = m[int64(-3)].([]byte)
	if len(x) != 32 || len(y) != 32 {
		return nil, nil, errors.New("公钥坐标长度无效")
	}
	if _, err := publicKey(x, y); err != nil {
		return nil, nil, err
	}
	return x, y, nil
}

func publicKey(x, y []byte) (*ecdsa.PublicKey, error) {
	pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
	// 用 ECDH 转换检查点是否在曲线上
	if _, err := pub.ECDH(); err != nil {
		return nil, errors.New("公钥不在曲线上")
	}
	return pub, nil
}

// VerifyAssertion 校验登录断言，成功时返回认证器报告的新签名计数。
// 签名计数非零时必须严格递增，否则视为凭据被克隆。
func VerifyAssertion(cred *Credential, resp AssertionResponse, challenge, host string) (uint32, error) {
	return VerifyAssertionRP(cred, resp, challenge, host, RPID(host))
}

// VerifyAssertionRP 与 VerifyAssertion 相同，但 RP ID 由调用方指定。
func VerifyAssertionRP(cred *Credential, resp AssertionResponse, challenge, host, rpID string) (uint32, error) {
	if resp.Type != "public-key" {
		return 0, errors.New("凭据类型无效")
	}
	cdRaw, err := b64(resp.Response.ClientDataJSON)
	if err != nil {
		return 0, errors.New("clientData 编码无效")
	}
	cdHash, err := checkClientData(string(cdRaw), "webauthn.get", challenge, host)
	if err != nil {
		return 0, err
	}
	adBytes, err := b64(resp.Response.AuthenticatorData)
	if err != nil {
		return 0, errors.New("认证器数据编码无效")
	}
	ad, err := parseAuthData(adBytes)
	if err != nil {
		return 0, err
	}
	if err := ad.check(rpID, true); err != nil {
		return 0, err
	}
	sig, err := b64(resp.Response.Signature)
	if err != nil {
		return 0, errors.New("签名编码无效")
	}
	x, err1 := b64(cred.X)
	y, err2 := b64(cred.Y)
	if err1 != nil || err2 != nil {
		return 0, errors.New("已保存的公钥损坏")
	}
	pub, err := publicKey(x, y)
	if err != nil {
		return 0, err
	}
	digest := sha256.Sum256(append(append([]byte{}, adBytes...), cdHash...))
	if !ecdsa.VerifyASN1(pub, digest[:], sig) {
		return 0, errors.New("签名校验失败")
	}
	if (ad.signCount != 0 || cred.SignCount != 0) && ad.signCount <= cred.SignCount {
		return 0, errors.New("签名计数未递增，凭据可能被克隆")
	}
	return ad.signCount, nil
}
