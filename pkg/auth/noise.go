package auth

import (
	"errors"
	"fmt"
	"io"

	"github.com/flynn/noise"
	"golang.org/x/crypto/curve25519"
)

// CipherSuite is the AVANGARD Noise cipher-suite:
// X25519 + ChaCha20-Poly1305 + BLAKE2s.
//
// Per-spec we keep ChaChaPoly here; the AEAD used for the post-handshake
// stream is selected separately (AES-GCM on AES-NI desktops, ChaCha20 on
// mobile). The handshake itself is small enough that the choice is moot.
var CipherSuite = noise.NewCipherSuite(
	noise.DH25519,
	noise.CipherChaChaPoly,
	noise.HashBLAKE2s,
)

// StaticKey is a long-term X25519 keypair (the server's identity).
type StaticKey = noise.DHKey

// GenerateStatic creates a fresh server static keypair.
func GenerateStatic() (StaticKey, error) {
	return CipherSuite.GenerateKeypair(nil)
}

// StaticFromPriv reconstructs a Noise X25519 keypair from a 32-byte private
// scalar (e.g. one produced by GenerateStatic and persisted to disk).
func StaticFromPriv(priv []byte) (StaticKey, error) {
	if len(priv) != 32 {
		return StaticKey{}, fmt.Errorf("noise: priv must be 32 bytes, got %d", len(priv))
	}
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return StaticKey{}, fmt.Errorf("derive pub: %w", err)
	}
	out := StaticKey{Public: pub, Private: append([]byte(nil), priv...)}
	return out, nil
}

// HandshakeResult carries the post-split cipherstates and a transcript hash.
type HandshakeResult struct {
	// Send is the cipherstate that encrypts data we send to the peer.
	Send *noise.CipherState
	// Recv decrypts data the peer sends to us.
	Recv *noise.CipherState
	// Hash is the handshake transcript hash (for channel-binding).
	Hash []byte
	// Payload is the plaintext payload received in the final handshake msg
	// (server-initiated value sent to the client in NK).
	Payload []byte
}

// ClientHandshake performs the Noise NK initiator side over an already-open
// duplex stream `rw`.
//
// Wire format used here is simple length-prefixed messages
// (`[2-byte BE len][body]`); in production the QUIC stream itself can carry
// these as-is and the framing isn't strictly needed, but this keeps the
// auth package transport-agnostic.
func ClientHandshake(rw io.ReadWriter, serverStaticPub []byte, prologue, initialPayload []byte) (*HandshakeResult, error) {
	if len(serverStaticPub) != 32 {
		return nil, errors.New("noise: server pubkey must be 32 bytes")
	}
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite: CipherSuite,
		Pattern:     noise.HandshakeNK,
		Initiator:   true,
		Prologue:    prologue,
		PeerStatic:  serverStaticPub,
	})
	if err != nil {
		return nil, fmt.Errorf("noise init: %w", err)
	}

	// -> e, es
	msg1, _, _, err := hs.WriteMessage(nil, initialPayload)
	if err != nil {
		return nil, fmt.Errorf("noise write1: %w", err)
	}
	if err := writeFrame(rw, msg1); err != nil {
		return nil, err
	}

	// <- e, ee, payload
	msg2, err := readFrame(rw)
	if err != nil {
		return nil, err
	}
	payload, cs1, cs2, err := hs.ReadMessage(nil, msg2)
	if err != nil {
		return nil, fmt.Errorf("noise read2: %w", err)
	}
	if cs1 == nil || cs2 == nil {
		return nil, errors.New("noise: split missing")
	}
	return &HandshakeResult{
		Send:    cs1, // initiator's send
		Recv:    cs2, // initiator's recv
		Hash:    append([]byte(nil), hs.ChannelBinding()...),
		Payload: payload,
	}, nil
}

// ServerHandshake performs the Noise NK responder side.
func ServerHandshake(rw io.ReadWriter, serverStatic StaticKey, prologue, replyPayload []byte) (*HandshakeResult, error) {
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:   CipherSuite,
		Pattern:       noise.HandshakeNK,
		Initiator:     false,
		Prologue:      prologue,
		StaticKeypair: serverStatic,
	})
	if err != nil {
		return nil, fmt.Errorf("noise init: %w", err)
	}

	// <- e, es
	msg1, err := readFrame(rw)
	if err != nil {
		return nil, err
	}
	clientPayload, _, _, err := hs.ReadMessage(nil, msg1)
	if err != nil {
		return nil, fmt.Errorf("noise read1: %w", err)
	}

	// -> e, ee, payload
	msg2, cs1, cs2, err := hs.WriteMessage(nil, replyPayload)
	if err != nil {
		return nil, fmt.Errorf("noise write2: %w", err)
	}
	if err := writeFrame(rw, msg2); err != nil {
		return nil, err
	}
	if cs1 == nil || cs2 == nil {
		return nil, errors.New("noise: split missing")
	}
	return &HandshakeResult{
		Send:    cs2, // responder's send (initiator's recv)
		Recv:    cs1, // responder's recv (initiator's send)
		Hash:    append([]byte(nil), hs.ChannelBinding()...),
		Payload: clientPayload,
	}, nil
}

func writeFrame(w io.Writer, b []byte) error {
	if len(b) > 65535 {
		return errors.New("noise: frame too large")
	}
	hdr := []byte{byte(len(b) >> 8), byte(len(b))}
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}

func readFrame(r io.Reader) ([]byte, error) {
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, err
	}
	n := int(hdr[0])<<8 | int(hdr[1])
	if n == 0 || n > 65535 {
		return nil, fmt.Errorf("noise: bad frame size %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
