// Package auth provides AVANGARD authentication primitives:
//   - HMAC-derived tunnel tokens (16 bytes on the wire).
//   - Noise NK handshake wrappers (client / server).
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
)

// TokenLen is the byte length of the HMAC token carried in the AVANGARD header.
const TokenLen = 16

// DeriveToken returns HMAC-SHA256(uuid, sessionNonce)[:16].
//
// The raw UUID is never sent on the wire; only this short HMAC is, bound to a
// per-session nonce so that captured tokens cannot be replayed verbatim.
func DeriveToken(uuid string, sessionNonce []byte) [TokenLen]byte {
	mac := hmac.New(sha256.New, []byte(uuid))
	mac.Write(sessionNonce)
	sum := mac.Sum(nil)
	var out [TokenLen]byte
	copy(out[:], sum[:TokenLen])
	return out
}

// VerifyToken returns true if `got` equals DeriveToken(uuid, nonce) in
// constant time.
func VerifyToken(uuid string, sessionNonce []byte, got [TokenLen]byte) bool {
	want := DeriveToken(uuid, sessionNonce)
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1
}
