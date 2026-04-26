// Package transport defines the AVANGARD transport interfaces shared by the
// QUIC and TCP-fallback implementations.
package transport

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"

	"github.com/avangard/avangard/pkg/auth"
)

// TunnelStream is an authenticated bi-directional byte stream produced by a
// successful server-accept or client-dial. AVANGARD framing (header + raw
// payload) is multiplexed on top.
type TunnelStream interface {
	io.ReadWriteCloser
	// RemoteAddr returns the peer's network address.
	RemoteAddr() net.Addr
	// Channel returns the Noise transcript-hash, suitable for channel binding.
	Channel() []byte
}

// Listener is the server side of a transport.
type Listener interface {
	Accept(ctx context.Context) (TunnelStream, error)
	Close() error
	Addr() net.Addr
}

// Dialer is the client side of a transport.
type Dialer interface {
	Dial(ctx context.Context) (TunnelStream, error)
}

// ServerConfig is shared between transports.
type ServerConfig struct {
	// Addr is the listen address, e.g. ":443".
	Addr string

	// AcceptedUUIDs is the set of UUIDs the server will recognise. The client
	// proves possession via the AVANGARD header HMAC token (see pkg/auth).
	AcceptedUUIDs []string

	// NoiseStatic is the long-term server identity (X25519 keypair).
	NoiseStatic auth.StaticKey

	// TLSCert is the certificate served on the outer transport (decoy cert
	// in production; self-signed for tests).
	TLSCert tls.Certificate

	// DecoyAddr is the "host:port" forwarded to when an inbound TCP
	// connection presents a non-AVANGARD SNI. Empty => no decoy forwarding.
	DecoyAddr string

	// Prologue is mixed into the Noise handshake. Empty defaults to "avangard-v1".
	Prologue []byte

	// Logger receives diagnostic logs. Nil => slog.Default().
	Logger *slog.Logger
}

// ClientConfig is shared between transports.
type ClientConfig struct {
	Addr               string
	UUID               string
	SNI                string
	ServerStaticPub    []byte
	InsecureSkipVerify bool // dev only
	Prologue           []byte
	Logger             *slog.Logger

	// Fingerprint, if non-empty, makes the client use uTLS to mimic the
	// named browser ClientHello (e.g. "chrome", "yandex"). Empty defaults
	// to standard crypto/tls (Go fingerprint).
	Fingerprint string
}

// DefaultPrologue is mixed into the Noise handshake when none is configured.
var DefaultPrologue = []byte("avangard-v1")

// EnsureLogger returns l or slog.Default if nil.
func EnsureLogger(l *slog.Logger) *slog.Logger {
	if l != nil {
		return l
	}
	return slog.Default()
}
