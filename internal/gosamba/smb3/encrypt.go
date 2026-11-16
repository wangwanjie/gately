// Gately 修改：OriginalMessageSize 在 uint64 中比较，32 位平台上不会因 int 转换为负数而跳过检查。

package smb3

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// SMB3 Transform Header (MS-SMB2 §2.2.41).
//
// Layout (52 bytes):
//
//	0  ProtocolId            4   = 0xFD 'S' 'M' 'B'
//	4  Signature            16   = AEAD tag
//	20 Nonce                16   = 11 bytes (CCM) or 12 bytes (GCM), zero-padded
//	36 OriginalMessageSize   4
//	40 Reserved              2
//	42 Flags/EncryptionAlgo  2   = 0x0001 = encrypted
//	44 SessionId             8
const (
	TransformHeaderSize    = 52
	TransformFlagEncrypted = 0x0001
	transformAADStart      = 20 // bytes 20..52 are the additional-authenticated-data
	transformAADLen        = 32
)

var (
	transformProtocolID = [4]byte{0xFD, 'S', 'M', 'B'}

	ErrShortTransform     = errors.New("smb3: transform header too short")
	ErrBadTransformID     = errors.New("smb3: bad transform ProtocolId")
	ErrUnknownCipher      = errors.New("smb3: unknown cipher")
	ErrTransformDecrypt   = errors.New("smb3: transform decrypt failed")
	ErrShortTransformBody = errors.New("smb3: transform body shorter than OriginalMessageSize")
)

// Cipher IDs (matching MS-SMB2 SMB2_ENCRYPTION_CAPABILITIES values).
const (
	CipherAES128CCM uint16 = 0x0001
	CipherAES128GCM uint16 = 0x0002
	CipherAES256CCM uint16 = 0x0003
	CipherAES256GCM uint16 = 0x0004
)

// CipherKeyBits returns the length, in bits, of the encryption/decryption key
// the given cipher needs — which is also the L value fed to the SMB3 KDF when
// deriving it (MS-SMB2 §3.1.4.2: L is 256 for the AES-256 ciphers, 128 for
// everything else).
//
// L is not just an output length: it is mixed into the PRF input, so a
// 256-bit key is *not* the 128-bit key plus 16 more bytes — the whole key
// differs. Callers must therefore derive at the right L rather than deriving
// long and truncating. Note this applies only to the cipher keys: the signing
// key and the application key stay 128-bit for every cipher, because SMB3
// signing is always AES-128-CMAC/GMAC.
//
// An unknown or zero cipher id (encryption not negotiated) yields 128 so the
// caller still gets a usable, if unused, key.
func CipherKeyBits(cipherID uint16) uint32 {
	switch cipherID {
	case CipherAES256CCM, CipherAES256GCM:
		return 256
	default:
		return 128
	}
}

// Transform nonces.
//
// AES-GCM and AES-CCM both fail catastrophically when a nonce repeats under a
// given key: two GCM frames sharing a nonce leak the XOR of their plaintexts
// and, worse, expose the GHASH subkey, which lets an attacker forge frames.
// MS-SMB2 §3.1.4.3 accordingly requires the nonce to be unique for every
// invocation with a given key, and both Windows and Samba implement that as a
// monotonically increasing counter rather than a fresh random draw.
//
// Drawing the nonce randomly (what this used to do) is not equivalent: the
// transform nonce is only 12 bytes for GCM and 11 for CCM, so random nonces
// collide by the birthday bound after roughly 2^48 / 2^44 frames on one key,
// and NIST SP 800-38D caps random-IV GCM at 2^32 invocations per key. A large
// copy over a long-lived session is well within shouting distance of those
// numbers; a counter removes the question entirely.
//
// The counter is process-global rather than per-session. That is strictly
// stronger than the per-session counter the spec asks for: no two frames this
// process ever emits share a nonce, whatever key they were encrypted under, so
// it holds even for the interim-plus-final response pairs that an async
// command (CHANGE_NOTIFY) emits under one MessageId. The high bytes carry a
// per-process random prefix so that nonces are not identical across restarts
// either. It also keeps EncryptTransform's signature — and therefore every
// caller — unchanged.
var (
	nonceCounter atomic.Uint64
	// noncePrefix is generated once per process. crypto/rand.Read is
	// documented never to fail as of Go 1.24, so there is no error to
	// propagate; uniqueness rests on the counter regardless.
	noncePrefix = sync.OnceValue(func() [8]byte {
		var p [8]byte
		_, _ = rand.Read(p[:])
		return p
	})
)

// nextNonce fills dst (11 or 12 bytes) with a nonce that this process has
// never emitted before.
func nextNonce(dst []byte) {
	binary.LittleEndian.PutUint64(dst[:8], nonceCounter.Add(1))
	prefix := noncePrefix()
	copy(dst[8:], prefix[:])
}

// IsTransform reports whether buf begins with the SMB3 transform protocol ID.
func IsTransform(buf []byte) bool {
	return len(buf) >= 4 &&
		buf[0] == transformProtocolID[0] && buf[1] == transformProtocolID[1] &&
		buf[2] == transformProtocolID[2] && buf[3] == transformProtocolID[3]
}

// DecryptTransform decrypts a frame whose first 4 bytes are 0xFD 'S' 'M' 'B'.
// Returns the cleartext SMB2 message, the SessionId, and an error.
func DecryptTransform(cipherID uint16, key, frame []byte) ([]byte, uint64, error) {
	if len(frame) < TransformHeaderSize {
		return nil, 0, ErrShortTransform
	}
	if !IsTransform(frame) {
		return nil, 0, ErrBadTransformID
	}
	origSize := binary.LittleEndian.Uint32(frame[36:])
	flags := binary.LittleEndian.Uint16(frame[42:])
	sessID := binary.LittleEndian.Uint64(frame[44:])
	if flags&TransformFlagEncrypted == 0 {
		return nil, 0, fmt.Errorf("smb3: transform flags=0x%04x (not encrypted)", flags)
	}
	ciphertext := frame[TransformHeaderSize:]
	// 在 uint64 中比较：32 位平台上 int(origSize) 可能变成负数而让检查失效
	if uint64(len(ciphertext)) < uint64(origSize) {
		return nil, 0, ErrShortTransformBody
	}

	aead, nonceLen, err := aeadFor(cipherID, key)
	if err != nil {
		return nil, 0, err
	}
	nonce := frame[20 : 20+nonceLen]

	// The AEAD wants ciphertext||tag contiguous, but the transform header keeps
	// the tag in its Signature field at offset 4. Rebuilding that by copying the
	// whole body cost two payload-sized buffers per frame (~2x the payload in
	// B/op): one for ciphertext||tag, one for the plaintext Open returned.
	//
	// transport.ReadFrame deliberately leaves 16 bytes of spare capacity on
	// every frame, so the tag can be appended straight after the ciphertext and
	// the plaintext decrypted back over it — no payload-sized allocation at all.
	// Frames that arrive without that headroom (tests, other callers) fall back
	// to the copy.
	//
	// This is an allocation and GC win, not a latency win: Go's GCM Open takes a
	// slower per-byte path when input and output alias, so wall clock is roughly
	// a wash while B/op drops from ~2x the payload to a constant.
	tag := frame[4:20]
	n := len(ciphertext)
	var ct []byte
	if cap(ciphertext) >= n+16 {
		ct = ciphertext[:n+16]
	} else {
		ct = make([]byte, n+16)
		copy(ct, ciphertext)
	}
	copy(ct[n:], tag)

	aad := frame[transformAADStart : transformAADStart+transformAADLen]
	// ct[:0] aliases ct exactly, which cipher.AEAD permits (only *inexact*
	// overlap is rejected), and GCM authenticates before it decrypts so reading
	// the ciphertext it is overwriting is safe.
	pt, err := aead.Open(ct[:0], nonce, ct, aad)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrTransformDecrypt, err)
	}
	return pt, sessID, nil
}

// EncryptTransform produces a transform-wrapped frame for plaintext using key.
// sessID is written into the header so the peer can locate the session.
func EncryptTransform(cipherID uint16, key []byte, sessID uint64, plaintext []byte) ([]byte, error) {
	aead, nonceLen, err := aeadFor(cipherID, key)
	if err != nil {
		return nil, err
	}
	// Both supported nonce lengths (11 for CCM, 12 for GCM) leave room for the
	// 8-byte counter plus prefix; the bounds are asserted so a future cipher
	// with a shorter nonce can't silently truncate the counter and start
	// repeating nonces.
	if nonceLen > 16 || nonceLen < 8 {
		return nil, fmt.Errorf("smb3: unusable nonce length %d", nonceLen)
	}
	nonce := make([]byte, nonceLen)
	nextNonce(nonce)

	// Length is just the header; the capacity covers header + ciphertext + tag
	// so that Seal appends straight into this buffer's spare bytes. Sealing into
	// a fresh buffer instead (Seal(nil, ...)) allocated a second payload-sized
	// slice and then copied the whole ciphertext back — two payload-sized
	// allocations and a full copy per frame.
	out := make([]byte, TransformHeaderSize, TransformHeaderSize+len(plaintext)+16)
	copy(out[:4], transformProtocolID[:])
	// out[4:20] (Signature) stays zero until the tag is moved into it below; it
	// is not part of the AAD, so its value during Seal is irrelevant.
	copy(out[20:20+nonceLen], nonce)
	binary.LittleEndian.PutUint32(out[36:], uint32(len(plaintext)))
	binary.LittleEndian.PutUint16(out[42:], TransformFlagEncrypted)
	binary.LittleEndian.PutUint64(out[44:], sessID)

	aad := out[transformAADStart : transformAADStart+transformAADLen]
	// sealed = header(52) || ciphertext(n) || tag(16), written in place.
	sealed := aead.Seal(out, nonce, plaintext, aad)
	// On the wire the tag lives in the transform header's Signature field, not
	// after the ciphertext, so move the trailing 16 bytes to offset 4 and cut
	// them off the end.
	tagOff := len(sealed) - 16
	copy(sealed[4:20], sealed[tagOff:])
	return sealed[:tagOff], nil
}

func newAEAD(cipherID uint16, rawKey []byte) (cipher.AEAD, int, error) {
	switch cipherID {
	case CipherAES128GCM, CipherAES128CCM:
		if len(rawKey) < 16 {
			return nil, 0, fmt.Errorf("smb3: cipher 0x%04x needs 16-byte key, got %d", cipherID, len(rawKey))
		}
		block, err := aes.NewCipher(rawKey[:16])
		if err != nil {
			return nil, 0, err
		}
		if cipherID == CipherAES128GCM {
			a, err := cipher.NewGCM(block)
			return a, 12, err
		}
		a, err := newCCM(block)
		return a, 11, err
	case CipherAES256GCM, CipherAES256CCM:
		if len(rawKey) < 32 {
			return nil, 0, fmt.Errorf("smb3: cipher 0x%04x needs 32-byte key, got %d", cipherID, len(rawKey))
		}
		block, err := aes.NewCipher(rawKey[:32])
		if err != nil {
			return nil, 0, err
		}
		if cipherID == CipherAES256GCM {
			a, err := cipher.NewGCM(block)
			return a, 12, err
		}
		a, err := newCCM(block)
		return a, 11, err
	}
	return nil, 0, fmt.Errorf("%w: 0x%04x", ErrUnknownCipher, cipherID)
}
