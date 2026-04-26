// Package smuxtcp wraps the TCP+TLS+Noise transport in SMUX v2 multiplexing
// (xtaci/smux). One outer connection carries up to 256 concurrent tunnel
// streams, dramatically reducing handshake overhead for short-lived flows
// (web browsing, DNS resolves, telegram messenger).
//
// All multiplexed streams share the same Noise transcript hash, so the
// AVANGARD HMAC-token check is performed once per substream against the same
// channel binding.
package smuxtcp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/avangard/avangard/pkg/auth"
	"github.com/avangard/avangard/pkg/transport"
	"github.com/avangard/avangard/pkg/utlsx"
	"github.com/xtaci/smux"
)

// ALPN advertised over TLS.
const ALPN = "avangard/1"

// smuxConfig returns the AVANGARD SMUX v2 settings (spec §1.5.3).
func smuxConfig() *smux.Config {
	c := smux.DefaultConfig()
	c.Version = 2
	c.KeepAliveInterval = 30 * time.Second
	c.KeepAliveTimeout = 60 * time.Second
	c.MaxFrameSize = 32 * 1024
	c.MaxReceiveBuffer = 4 * 1024 * 1024
	c.MaxStreamBuffer = 256 * 1024
	return c
}

// Server is the multiplexed AVANGARD server.
type Server struct {
	cfg      transport.ServerConfig
	tlsConf  *tls.Config
	listener net.Listener
	logger   *slog.Logger

	out  chan *acceptResult
	done chan struct{}
	once sync.Once
}

type acceptResult struct {
	s   transport.TunnelStream
	err error
}

// Listen starts a multiplexed AVANGARD listener.
func Listen(cfg transport.ServerConfig) (*Server, error) {
	if cfg.TLSCert.Certificate == nil {
		return nil, errors.New("smuxtcp: TLSCert is required")
	}
	if len(cfg.Prologue) == 0 {
		cfg.Prologue = transport.DefaultPrologue
	}
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cfg.TLSCert},
		NextProtos:   []string{ALPN},
		MinVersion:   tls.VersionTLS13,
	}
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("smuxtcp listen: %w", err)
	}
	s := &Server{
		cfg:      cfg,
		tlsConf:  tlsConf,
		listener: ln,
		logger:   transport.EnsureLogger(cfg.Logger),
		out:      make(chan *acceptResult, 64),
		done:     make(chan struct{}),
	}
	go s.acceptLoop()
	return s, nil
}

func (s *Server) acceptLoop() {
	for {
		raw, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
			}
			s.logger.Error("smuxtcp accept", "err", err)
			return
		}
		go s.handleOuter(raw)
	}
}

func (s *Server) handleOuter(raw net.Conn) {
	_ = raw.SetDeadline(time.Now().Add(15 * time.Second))
	tlsConn := tls.Server(raw, s.tlsConf)
	if err := tlsConn.HandshakeContext(context.Background()); err != nil {
		s.logger.Debug("smuxtcp tls handshake", "err", err)
		_ = raw.Close()
		return
	}
	hr, err := auth.ServerHandshake(tlsConn, s.cfg.NoiseStatic, s.cfg.Prologue, []byte("avangard-srv-hello"))
	if err != nil {
		s.logger.Debug("smuxtcp noise", "err", err)
		_ = tlsConn.Close()
		return
	}
	_ = raw.SetDeadline(time.Time{})

	session, err := smux.Server(tlsConn, smuxConfig())
	if err != nil {
		s.logger.Debug("smux session", "err", err)
		_ = tlsConn.Close()
		return
	}
	go s.acceptStreams(session, raw, hr.Hash)
}

func (s *Server) acceptStreams(session *smux.Session, raw net.Conn, hash []byte) {
	defer session.Close()
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			return
		}
		ts := &smuxStream{
			Stream: stream,
			raw:    raw,
			hash:   hash,
		}
		select {
		case s.out <- &acceptResult{s: ts}:
		case <-s.done:
			_ = stream.Close()
			return
		}
	}
}

// Accept returns the next authenticated substream.
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

// Close shuts the listener.
func (s *Server) Close() error {
	s.once.Do(func() { close(s.done) })
	return s.listener.Close()
}

// Addr returns the bound address.
func (s *Server) Addr() net.Addr { return s.listener.Addr() }

// Client opens one underlying TCP+TLS+Noise connection and multiplexes
// many tunnel streams over it.
type Client struct {
	cfg     transport.ClientConfig
	mu      sync.Mutex
	session *smux.Session
	raw     net.Conn
	logger  *slog.Logger
	hash    []byte
}

// NewClient prepares a Client (does not yet dial — the underlying connection
// is established lazily on the first Dial).
func NewClient(cfg transport.ClientConfig) (*Client, error) {
	if cfg.Addr == "" {
		return nil, errors.New("smuxtcp: addr required")
	}
	if len(cfg.ServerStaticPub) != 32 {
		return nil, errors.New("smuxtcp: ServerStaticPub must be 32 bytes")
	}
	if len(cfg.Prologue) == 0 {
		cfg.Prologue = transport.DefaultPrologue
	}
	return &Client{cfg: cfg, logger: transport.EnsureLogger(cfg.Logger)}, nil
}

// Dial opens a fresh substream, establishing the underlying connection on
// first use (or after the previous one has been torn down).
func (c *Client) Dial(ctx context.Context) (transport.TunnelStream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == nil || c.session.IsClosed() {
		if err := c.dialOuter(ctx); err != nil {
			return nil, err
		}
	}
	stream, err := c.session.OpenStream()
	if err != nil {
		// Outer session might have been killed mid-Dial; drop it and retry once.
		c.session = nil
		if err2 := c.dialOuter(ctx); err2 != nil {
			return nil, fmt.Errorf("reconnect: %w", err2)
		}
		stream, err = c.session.OpenStream()
		if err != nil {
			return nil, err
		}
	}
	return &smuxStream{Stream: stream, raw: c.raw, hash: c.hash}, nil
}

func (c *Client) dialOuter(ctx context.Context) error {
	d := &net.Dialer{}
	raw, err := d.DialContext(ctx, "tcp", c.cfg.Addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	tlsConf := &tls.Config{
		ServerName:         c.cfg.SNI,
		NextProtos:         []string{ALPN},
		InsecureSkipVerify: c.cfg.InsecureSkipVerify, //nolint:gosec
		MinVersion:         tls.VersionTLS13,
	}
	tlsConn, err := dialTLS(ctx, raw, tlsConf, c.cfg.Fingerprint)
	if err != nil {
		_ = raw.Close()
		return fmt.Errorf("tls: %w", err)
	}
	hr, err := auth.ClientHandshake(tlsConn, c.cfg.ServerStaticPub, c.cfg.Prologue, []byte("avangard-cli-hello"))
	if err != nil {
		_ = tlsConn.Close()
		return fmt.Errorf("noise: %w", err)
	}
	session, err := smux.Client(tlsConn, smuxConfig())
	if err != nil {
		_ = tlsConn.Close()
		return fmt.Errorf("smux: %w", err)
	}
	c.session = session
	c.raw = raw
	c.hash = hr.Hash
	return nil
}

// Close tears down the outer connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		_ = c.session.Close()
	}
	if c.raw != nil {
		_ = c.raw.Close()
	}
	return nil
}

// dialTLS is duplicated here (instead of being shared via internal/) to keep
// the package fully self-contained.
func dialTLS(ctx context.Context, raw net.Conn, cfg *tls.Config, fp string) (net.Conn, error) {
	if fp == "" {
		c := tls.Client(raw, cfg)
		if err := c.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		return c, nil
	}
	uconn, err := utlsx.New(raw, cfg, utlsx.FingerprintFromString(fp))
	if err != nil {
		return nil, err
	}
	if err := uconn.HandshakeContext(ctx); err != nil {
		return nil, err
	}
	return uconn, nil
}

// smuxStream adapts an *smux.Stream as transport.TunnelStream.
type smuxStream struct {
	*smux.Stream
	raw  net.Conn
	hash []byte
}

func (s *smuxStream) RemoteAddr() net.Addr { return s.raw.RemoteAddr() }
func (s *smuxStream) Channel() []byte      { return s.hash }

var _ transport.TunnelStream = (*smuxStream)(nil)
