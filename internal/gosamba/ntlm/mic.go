// Gately 修改：报文中的偏移与长度改用 bounds 包在 uint64 中判断边界，32 位平台上不会因 int 转换为负数而越界。

package ntlm

import (
	"crypto/hmac"
	"crypto/md5"
	"encoding/binary"
)

// NTLMSSP message integrity code (MIC) — MS-NLMP §3.1.5.1.2 / §3.2.5.1.2.
//
// A client that supports the MIC computes
//
//	MIC = HMAC_MD5(ExportedSessionKey,
//	               NEGOTIATE_MESSAGE || CHALLENGE_MESSAGE || AUTHENTICATE_MESSAGE)
//
// over the three handshake messages with the AUTHENTICATE_MESSAGE's own MIC
// field zeroed, and stores the result at offset 72 of the AUTHENTICATE_MESSAGE.
//
// Verifying it is what stops an active attacker from editing the parts of the
// NTLM handshake that nothing else covers. NTProofStr authenticates the NTLMv2
// response (and the AV pairs inside it), but not the NEGOTIATE_MESSAGE's
// NegotiateFlags — so without the MIC a downgrade of, say, NEGOTIATE_SEAL or
// NEGOTIATE_EXTENDED_SESSIONSECURITY goes unnoticed by both ends.

// MICFieldOffset is where the 16-byte MIC lives inside an AUTHENTICATE_MESSAGE:
// after the 64-byte fixed part, the 8-byte Version field, at offset 72.
const MICFieldOffset = 72

// AVFlagMICPresent is the MsvAvFlags bit a client sets to say "the
// AUTHENTICATE_MESSAGE carries a MIC" (MS-NLMP §2.2.2.10).
const AVFlagMICPresent uint32 = 0x00000002

// MICRequired reports whether the NTLMv2 response asserts that the
// AUTHENTICATE_MESSAGE carries a MIC, i.e. whether the server must verify one.
//
// The flag is read out of the MsvAvFlags AV pair *inside* the NTLMv2 response
// rather than inferred from a non-zero MIC field, and that distinction is the
// security-relevant part: the AV pairs are covered by NTProofStr, so once
// VerifyNTLMv2 has passed, an attacker cannot have cleared this bit. It can
// still zero the MIC field itself — that just makes verification fail, which
// is exactly what should happen.
func MICRequired(ntResponse []byte) bool {
	flags, ok := msvAvFlags(ntResponse)
	return ok && flags&AVFlagMICPresent != 0
}

// msvAvFlags extracts the MsvAvFlags value from an NTLMv2 response.
//
// NTLMv2_RESPONSE layout (MS-NLMP §2.2.2.8):
//
//	0..16  NTProofStr
//	16..44 NTLMv2_CLIENT_CHALLENGE fixed part — RespType(1) HiRespType(1)
//	       Reserved1(2) Reserved2(4) TimeStamp(8) ChallengeFromClient(8)
//	       Reserved3(4) = 28 bytes
//	44..   AV_PAIR list
func msvAvFlags(ntResponse []byte) (uint32, bool) {
	const avOffset = 16 + 28
	if len(ntResponse) < avOffset {
		return 0, false
	}
	pairs, err := DecodeAVList(ntResponse[avOffset:])
	if err != nil {
		return 0, false
	}
	for _, p := range pairs {
		if p.ID == AVFlags && len(p.Value) >= 4 {
			return binary.LittleEndian.Uint32(p.Value), true
		}
	}
	return 0, false
}

// ComputeMIC returns HMAC_MD5(exportedSessionKey, negotiate||challenge||
// authenticate) with the AUTHENTICATE_MESSAGE's MIC field zeroed. The caller's
// buffer is never modified — the zeroing happens on a copy.
//
// negotiate and challenge must be the exact bytes that crossed the wire, and
// authenticate must be trimmed to the message itself (see AuthenticateLen).
func ComputeMIC(exportedSessionKey, negotiate, challenge, authenticate []byte) [16]byte {
	zeroed := append([]byte(nil), authenticate...)
	if len(zeroed) >= MICFieldOffset+16 {
		for i := MICFieldOffset; i < MICFieldOffset+16; i++ {
			zeroed[i] = 0
		}
	}
	mac := hmac.New(md5.New, exportedSessionKey)
	mac.Write(negotiate)
	mac.Write(challenge)
	mac.Write(zeroed)
	var out [16]byte
	copy(out[:], mac.Sum(nil))
	return out
}

// VerifyMIC reports whether mic matches the MIC computed over the handshake.
// The comparison is constant-time.
func VerifyMIC(exportedSessionKey, negotiate, challenge, authenticate []byte, mic [16]byte) bool {
	want := ComputeMIC(exportedSessionKey, negotiate, challenge, authenticate)
	return hmac.Equal(want[:], mic[:])
}

// AuthenticateLen returns the length of the AUTHENTICATE_MESSAGE that starts at
// b[0], which is not always len(b).
//
// The last leg of a SPNEGO exchange normally carries a mechListMIC *after* the
// responseToken that holds the AUTHENTICATE_MESSAGE. Those trailing DER bytes
// are not part of what the client MIC'd, so they have to be trimmed off before
// the MIC is recomputed — otherwise every MIC-sending client (i.e. every modern
// Windows client) would fail to authenticate. The SPNEGO unwrap now returns the
// responseToken exactly, so this is a second line of defence, and it still
// matters for a security buffer that could not be parsed as SPNEGO and was
// located by scanning for the NTLMSSP signature.
//
// The end of the message is the furthest extent of its payload fields; clients
// lay those out contiguously after the fixed part. A message with no payload at
// all gives nothing to measure, but it also has no NTLMv2 response and hence no
// MsvAvFlags, so its MIC is never verified — returning len(b) there is safe.
func AuthenticateLen(b []byte) int {
	// 在 uint64 中累计：偏移是 uint32，32 位平台上转换为 int 可能变成负数
	var end uint64
	// The six payload field descriptors: LmChallengeResponse,
	// NtChallengeResponse, DomainName, UserName, Workstation,
	// EncryptedRandomSessionKey. Each is len(2) maxlen(2) offset(4).
	for _, off := range []int{12, 20, 28, 36, 44, 52} {
		if len(b) < off+8 {
			return len(b)
		}
		l := uint64(binary.LittleEndian.Uint16(b[off:]))
		o := uint64(binary.LittleEndian.Uint32(b[off+4:]))
		if l == 0 {
			continue
		}
		if o+l > end {
			end = o + l
		}
	}
	if end == 0 || end > uint64(len(b)) {
		return len(b)
	}
	return int(end)
}
