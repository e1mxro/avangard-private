package auth

import "testing"

func TestDeriveTokenStable(t *testing.T) {
	a := DeriveToken("uuid-1", []byte("nonce"))
	b := DeriveToken("uuid-1", []byte("nonce"))
	if a != b {
		t.Fatalf("non-deterministic")
	}
}

func TestDeriveTokenDifferent(t *testing.T) {
	a := DeriveToken("uuid-1", []byte("nonce"))
	b := DeriveToken("uuid-2", []byte("nonce"))
	if a == b {
		t.Fatalf("collision on different uuid")
	}
	c := DeriveToken("uuid-1", []byte("other"))
	if a == c {
		t.Fatalf("collision on different nonce")
	}
}

func TestVerifyToken(t *testing.T) {
	tok := DeriveToken("u", []byte("n"))
	if !VerifyToken("u", []byte("n"), tok) {
		t.Fatal("verify failed")
	}
	if VerifyToken("u", []byte("x"), tok) {
		t.Fatal("verify accepted wrong nonce")
	}
}
