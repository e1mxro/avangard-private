// Package tcptransport implements the AVANGARD TCP-fallback server and client,
// with optional Reality-style SNI-based decoy forwarding.
//
// On Accept the server peeks the inbound TLS ClientHello SNI:
//   - if it matches one of `MagicSNIs`, the server terminates TLS itself and
//     attempts a Noise NK handshake;
//   - otherwise, raw bytes are bidirectionally proxied to `DecoyAddr` so that
//     active probers see exactly what the real decoy site would return.
//
// This is a simplified MVP of the Reality scheme — see spec §1.3.1.
package tcptransport

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"slices"
	"time"

	"github.com/avangard/avangard/pkg/auth"
	"github.com/avangard/avangard/pkg/transport"
)

// ALPN advertised over TLS. Must match the client.
const ALPN = "avangard/1"

// Server is an AVANGARD TCP/TLS listener.
type Server struct {
	cfg     transport.ServerConfig
	mlsConf *tls.Config
	ln      net.Listener
	logger  *slog.Logger

	// MagicSNIs are the SNIs that route to AVANGARD termination. Anything
	// else gets proxied to DecoyAddr (if set).
	magicSNIs []string

	out  chan *acceptResult
	done chan struct{}
}

type acceptResult struct {
	s   transport.TunnelStream
	err error
}

// Listen starts a TCP+TLS listener.
//
// magicSNIs, if non-empty, restricts which SNIs are routed to AVANGARD. Empty
// means "accept any SNI" (decoy forwarding disabled).
func Listen(cfg transport.ServerConfig, magicSNIs []string) (*Server, error) {
	if cfg.TLSCert.Certificate == nil {
		return nil, errors.New("tcp: TLSCert is required")
	}
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cfg.TLSCert},
		NextProtos:   []string{ALPN},
		MinVersion:   tls.VersionTLS13,
	}
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("tcp listen: %w", err)
	}
	if len(cfg.Prologue) == 0 {
		cfg.Prologue = transport.DefaultPrologue
	}
	s := &Server{
		cfg:       cfg,
		mlsConf:   tlsConf,
		ln:        ln,
		logger:    transport.EnsureLogger(cfg.Logger),
		magicSNIs: magicSNIs,
		out:       make(chan *acceptResult, 32),
		done:      make(chan struct{}),
	}
	go s.acceptLoop()
	return s, nil
}

func (s *Server) acceptLoop() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
			}
			s.logger.Error("tcp accept", "err", err)
			return
		}
		go s.handleConn(c)
	}
}

func (s *Server) handleConn(c net.Conn) {
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))

	// Peek ClientHello to extract SNI.
	pre := &peekConn{Conn: c}
	sni, raw, err := peekClientHelloSNI(pre)
	if err != nil && len(raw) == 0 {
		// Read failure or non-handshake bytes — drop silently.
		_ = c.Close()
		return
	}
	pre.replay = raw // injected into subsequent reads

	if !s.isMagicSNI(sni) {
		// Forward raw bytes to decoy (Reality-style fallback).
		s.forwardDecoy(pre)
		return
	}

	// Reset deadline for TLS+Noise handshake.
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))

	tlsConn := tls.Server(pre, s.mlsConf)
	if err := tlsConn.HandshakeContext(context.Background()); err != nil {
		s.logger.Debug("tls handshake failed", "remote", c.RemoteAddr(), "err", err)
		_ = c.Close()
		return
	}

	// Run Noise NK handshake on the inner TLS stream.
	hr, err := auth.ServerHandshake(tlsConn, s.cfg.NoiseStatic, s.cfg.Prologue, []byte("avangard-srv-hello"))
	if err != nil {
		s.logger.Debug("noise handshake failed", "remote", c.RemoteAddr(), "err", err)
		_ = tlsConn.Close()
		return
	}
	_ = c.SetDeadline(time.Time{})

	ts := &tcpStream{
		raw:  c,
		conn: tlsConn,
		hash: hr.Hash,
	}
	select {
	case s.out <- &acceptResult{s: ts}:
	case <-s.done:
		_ = tlsConn.Close()
	}
}

func (s *Server) forwardDecoy(c net.Conn) {
	defer c.Close()
	if s.cfg.DecoyAddr == "" {
		// No decoy configured — close silently. Better than emitting RST.
		return
	}
	d, err := net.DialTimeout("tcp", s.cfg.DecoyAddr, 5*time.Second)
	if err != nil {
		s.logger.Debug("decoy dial failed", "addr", s.cfg.DecoyAddr, "err", err)
		return
	}
	defer d.Close()
	_ = c.SetDeadline(time.Time{})

	// Bi-directional copy.
	errc := make(chan error, 2)
	go func() { _, err := io.Copy(d, c); errc <- err }()
	go func() { _, err := io.Copy(c, d); errc <- err }()
	<-errc
}

func (s *Server) isMagicSNI(sni string) bool {
	if len(s.magicSNIs) == 0 {
		// No SNI gating configured: accept everything. (No decoy mode.)
		return true
	}
	return slices.Contains(s.magicSNIs, sni)
}

// Accept returns the next authenticated tunnel stream.
func (s *Server) Accept(ctx context.Context) (transport.TunnelStream, error) {
	select {
	case r := <-s.out:
		return r.s, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return nil, net.ErrClosed
	}
}

// Close shuts down the listener.
func (s *Server) Close() error {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	return s.ln.Close()
}

// Addr returns the TCP listening address.
func (s *Server) Addr() net.Addr { return s.ln.Addr() }

// peekConn returns previously-peeked bytes on Read and then falls back to the
// underlying connection.
type peekConn struct {
	net.Conn
	replay []byte
}

func (p *peekConn) Read(b []byte) (int, error) {
	if len(p.replay) > 0 {
		n := copy(b, p.replay)
		p.replay = p.replay[n:]
		return n, nil
	}
	return p.Conn.Read(b)
}

// We rely on type assertions in tls.Server to read NetConn-like methods.
var _ net.Conn = (*peekConn)(nil)

// tcpStream wraps a TLS connection (std crypto/tls or uTLS) as a TunnelStream.
type tcpStream struct {
	raw  net.Conn
	conn net.Conn // tls.Conn or *utls.UConn — both implement net.Conn
	hash []byte
}

func (t *tcpStream) Read(b []byte) (int, error)  { return t.conn.Read(b) }
func (t *tcpStream) Write(b []byte) (int, error) { return t.conn.Write(b) }
func (t *tcpStream) Close() error                { return t.conn.Close() }
func (t *tcpStream) RemoteAddr() net.Addr        { return t.raw.RemoteAddr() }
func (t *tcpStream) Channel() []byte             { return t.hash }

// Compile-time interface check.
var _ transport.TunnelStream = (*tcpStream)(nil)

// Used by tests / dev to make the unused-import linter happy.
var _ = bytes.Equal
