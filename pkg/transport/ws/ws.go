// Package wstransport implements the AVANGARD WebSocket carrier ("mode B" in
// the spec — colloquially "Госуслуги" mode). The bytestream is wrapped in
// WebSocket binary frames over TLS, indistinguishable from a normal browser-
// to-API session. Suitable for environments that DPI-block QUIC and
// suspicious-looking ALPNs but allow generic HTTPS/WebSocket.
package wstransport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/avangard/avangard/pkg/auth"
	"github.com/avangard/avangard/pkg/transport"
	"github.com/avangard/avangard/pkg/utlsx"
	"github.com/gorilla/websocket"
)

// Path is the default WebSocket upgrade path. Override per-deployment to fit
// the chosen camouflage decoy (e.g. "/api/v1/notifications").
const DefaultPath = "/ws"

// Server upgrades incoming HTTPS connections to WebSocket and forwards raw
// frames to the AVANGARD upper layer (Noise NK + framing).
type Server struct {
	cfg       transport.ServerConfig
	path      string
	httpSrv   *http.Server
	logger    *slog.Logger
	upgrader  websocket.Upgrader
	out       chan *acceptResult
	done      chan struct{}
	closeOnce sync.Once
}

type acceptResult struct {
	s   transport.TunnelStream
	err error
}

// ListenTLS starts a WS server over TLS on cfg.Addr. The path is the upgrade
// endpoint (DefaultPath if empty). Non-WS requests are answered with a generic
// 404 — production deployments should route them to a real decoy via a front
// reverse-proxy.
func ListenTLS(cfg transport.ServerConfig, path string) (*Server, error) {
	if cfg.TLSCert.Certificate == nil {
		return nil, errors.New("ws: TLSCert is required")
	}
	if path == "" {
		path = DefaultPath
	}
	if len(cfg.Prologue) == 0 {
		cfg.Prologue = transport.DefaultPrologue
	}

	s := &Server{
		cfg:    cfg,
		path:   path,
		logger: transport.EnsureLogger(cfg.Logger),
		out:    make(chan *acceptResult, 32),
		done:   make(chan struct{}),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc(path, s.handleWS)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cfg.TLSCert},
		MinVersion:   tls.VersionTLS13,
		NextProtos:   []string{"http/1.1"}, // WS requires HTTP/1.1
	}
	ln, err := tls.Listen("tcp", cfg.Addr, tlsConf)
	if err != nil {
		return nil, fmt.Errorf("ws tls listen: %w", err)
	}
	s.httpSrv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("ws serve", "err", err)
		}
	}()
	s.logger.Info("ws listening", "addr", ln.Addr(), "path", path)
	return s, nil
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Debug("ws upgrade", "err", err)
		return
	}
	stream := newWSStream(conn)
	hr, err := auth.ServerHandshake(stream, s.cfg.NoiseStatic, s.cfg.Prologue, []byte("avangard-srv-hello"))
	if err != nil {
		s.logger.Debug("noise handshake (ws)", "err", err)
		_ = conn.Close()
		return
	}
	stream.hash = hr.Hash
	select {
	case s.out <- &acceptResult{s: stream}:
	case <-s.done:
		_ = conn.Close()
	}
}

// Accept returns the next authenticated stream.
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

// Close shuts down the HTTP server.
func (s *Server) Close() error {
	s.closeOnce.Do(func() { close(s.done) })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.httpSrv.Shutdown(ctx)
}

// Addr returns the listening address.
func (s *Server) Addr() net.Addr {
	if s.httpSrv == nil {
		return nil
	}
	// http.Server doesn't expose Addr; document this limitation.
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

// Client dials wss:// + Noise NK.
type Client struct {
	cfg    transport.ClientConfig
	path   string
	logger *slog.Logger
}

// NewClient validates config.
func NewClient(cfg transport.ClientConfig, path string) (*Client, error) {
	if cfg.Addr == "" {
		return nil, errors.New("ws: addr required")
	}
	if len(cfg.ServerStaticPub) != 32 {
		return nil, errors.New("ws: ServerStaticPub must be 32 bytes")
	}
	if path == "" {
		path = DefaultPath
	}
	if len(cfg.Prologue) == 0 {
		cfg.Prologue = transport.DefaultPrologue
	}
	return &Client{cfg: cfg, path: path, logger: transport.EnsureLogger(cfg.Logger)}, nil
}

// Dial opens a wss:// connection and runs Noise NK.
func (c *Client) Dial(ctx context.Context) (transport.TunnelStream, error) {
	tlsConf := &tls.Config{
		ServerName:         c.cfg.SNI,
		InsecureSkipVerify: c.cfg.InsecureSkipVerify, //nolint:gosec
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{"http/1.1"},
	}
	netDialer := &net.Dialer{}
	rawConn, err := netDialer.DialContext(ctx, "tcp", c.cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("ws dial tcp: %w", err)
	}
	tlsConn, err := dialTLS(ctx, rawConn, tlsConf, c.cfg.Fingerprint)
	if err != nil {
		_ = rawConn.Close()
		return nil, fmt.Errorf("ws tls: %w", err)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		// We've already established the TLS layer; gorilla expects a NetDialContext
		// that gives back the (already-handshaked) connection.
		NetDialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return tlsConn, nil
		},
	}
	host := c.cfg.SNI
	if host == "" {
		host = c.cfg.Addr
	}
	u := &url.URL{Scheme: "wss", Host: host, Path: c.path}
	hdr := http.Header{}
	conn, _, err := dialer.DialContext(ctx, u.String(), hdr)
	if err != nil {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("ws upgrade: %w", err)
	}

	stream := newWSStream(conn)
	hr, err := auth.ClientHandshake(stream, c.cfg.ServerStaticPub, c.cfg.Prologue, []byte("avangard-cli-hello"))
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("noise: %w", err)
	}
	stream.hash = hr.Hash
	return stream, nil
}

// dialTLS is identical to the helper in tcp/client.go; duplicated here to keep
// the package self-contained without cross-package internals.
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

// wsStream adapts a *websocket.Conn to the AVANGARD TunnelStream interface.
//
// Each Write produces exactly one BinaryMessage; each Read returns the next
// pending message body (re-buffering across Read calls if the message is
// larger than the caller's buffer).
type wsStream struct {
	c       *websocket.Conn
	hash    []byte
	readBuf []byte
	mu      sync.Mutex
}

func newWSStream(c *websocket.Conn) *wsStream { return &wsStream{c: c} }

func (s *wsStream) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.readBuf) == 0 {
		_, msg, err := s.c.ReadMessage()
		if err != nil {
			return 0, err
		}
		s.readBuf = msg
	}
	n := copy(p, s.readBuf)
	s.readBuf = s.readBuf[n:]
	return n, nil
}

func (s *wsStream) Write(p []byte) (int, error) {
	if err := s.c.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *wsStream) Close() error          { return s.c.Close() }
func (s *wsStream) RemoteAddr() net.Addr  { return s.c.RemoteAddr() }
func (s *wsStream) Channel() []byte       { return s.hash }

// Compile-time interface check.
var _ transport.TunnelStream = (*wsStream)(nil)

// Read deadline support (used by some upper layers).
func (s *wsStream) SetReadDeadline(t time.Time) error  { return s.c.SetReadDeadline(t) }
func (s *wsStream) SetWriteDeadline(t time.Time) error { return s.c.SetWriteDeadline(t) }
func (s *wsStream) SetDeadline(t time.Time) error {
	if err := s.c.SetReadDeadline(t); err != nil {
		return err
	}
	return s.c.SetWriteDeadline(t)
}

// LocalAddr is provided to satisfy net.Conn-shaped consumers.
func (s *wsStream) LocalAddr() net.Addr {
	return s.c.LocalAddr()
}

// Use io.EOF for short reads if WS message has been fully drained and peer closed.
var _ io.Reader = (*wsStream)(nil)
