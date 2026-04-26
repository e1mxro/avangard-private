// Package quictransport implements the AVANGARD QUIC server and client.
//
// Layering:
//
//	QUIC v1 (RFC 9000, with TLS 1.3 by RFC 9001)
//	  └─ stream (per-tunnel)
//	       └─ Noise NK handshake (length-prefixed)
//	            └─ AVANGARD header + raw payload (no extra encryption)
//
// QUIC handles transport encryption and 0-RTT; Noise handles authentication.
// We do NOT layer a second AEAD on top — see spec §1.4.5 (no double crypto).
package quictransport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/avangard/avangard/pkg/auth"
	"github.com/avangard/avangard/pkg/transport"
	"github.com/quic-go/quic-go"
)

// ALPN identifies the AVANGARD QUIC application.
//
// Production deployments inside RegionShield modes override this with a value
// from the regional pool (e.g. "h3", "h3-29") to dodge ALPN-based DPI.
const ALPN = "avangard/1"

// Server is an AVANGARD QUIC listener.
type Server struct {
	cfg    transport.ServerConfig
	ln     *quic.Listener
	logger *slog.Logger

	// Channel of accepted, post-handshake streams.
	out chan *acceptResult
	// closed when the server shuts down.
	done chan struct{}
}

type acceptResult struct {
	s   transport.TunnelStream
	err error
}

// Listen starts a QUIC listener.
func Listen(cfg transport.ServerConfig) (*Server, error) {
	if cfg.TLSCert.Certificate == nil {
		return nil, errors.New("quic: TLSCert is required")
	}
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cfg.TLSCert},
		NextProtos:   []string{ALPN},
		MinVersion:   tls.VersionTLS13,
	}
	qConf := &quic.Config{
		MaxIdleTimeout:  120e9, // 120 s — see spec §1.6.2
		KeepAlivePeriod: 30e9,
	}
	ln, err := quic.ListenAddr(cfg.Addr, tlsConf, qConf)
	if err != nil {
		return nil, fmt.Errorf("quic listen: %w", err)
	}
	prologue := cfg.Prologue
	if len(prologue) == 0 {
		prologue = transport.DefaultPrologue
	}
	cfg.Prologue = prologue

	s := &Server{
		cfg:    cfg,
		ln:     ln,
		logger: transport.EnsureLogger(cfg.Logger),
		out:    make(chan *acceptResult, 32),
		done:   make(chan struct{}),
	}
	go s.acceptLoop()
	return s, nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.ln.Accept(context.Background())
		if err != nil {
			select {
			case <-s.done:
				return
			default:
			}
			s.logger.Error("quic accept conn", "err", err)
			return
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn *quic.Conn) {
	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			s.logger.Debug("quic stream accept stop", "err", err)
			return
		}
		go s.handleStream(conn, stream)
	}
}

func (s *Server) handleStream(conn *quic.Conn, stream *quic.Stream) {
	hr, err := auth.ServerHandshake(stream, s.cfg.NoiseStatic, s.cfg.Prologue, []byte("avangard-srv-hello"))
	if err != nil {
		s.logger.Debug("noise handshake failed", "remote", conn.RemoteAddr(), "err", err)
		_ = stream.Close()
		return
	}
	ts := &quicStream{
		Stream: stream,
		conn:   conn,
		hash:   hr.Hash,
	}
	select {
	case s.out <- &acceptResult{s: ts}:
	case <-s.done:
		_ = stream.Close()
	}
}

// Accept returns the next authenticated tunnel stream.
func (s *Server) Accept(ctx context.Context) (transport.TunnelStream, error) {
	select {
	case r := <-s.out:
		if r == nil {
			return nil, net.ErrClosed
		}
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

// Addr returns the listening UDP address.
func (s *Server) Addr() net.Addr { return s.ln.Addr() }

type quicStream struct {
	*quic.Stream
	conn *quic.Conn
	hash []byte
}

func (q *quicStream) RemoteAddr() net.Addr { return q.conn.RemoteAddr() }
func (q *quicStream) Channel() []byte      { return q.hash }
