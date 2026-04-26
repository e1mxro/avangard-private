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

// Client is an AVANGARD QUIC dialer.
type Client struct {
	cfg    transport.ClientConfig
	conn   *quic.Conn
	logger *slog.Logger
}

// NewClient connects (immediately) to the server and returns a ready dialer.
func NewClient(ctx context.Context, cfg transport.ClientConfig) (*Client, error) {
	if cfg.Addr == "" {
		return nil, errors.New("quic: addr required")
	}
	if len(cfg.ServerStaticPub) != 32 {
		return nil, errors.New("quic: ServerStaticPub must be 32 bytes")
	}
	if len(cfg.Prologue) == 0 {
		cfg.Prologue = transport.DefaultPrologue
	}

	tlsConf := &tls.Config{
		ServerName:         cfg.SNI,
		NextProtos:         []string{ALPN},
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // configurable
		MinVersion:         tls.VersionTLS13,
	}
	qConf := &quic.Config{
		MaxIdleTimeout:  120e9,
		KeepAlivePeriod: 30e9,
	}
	conn, err := quic.DialAddr(ctx, cfg.Addr, tlsConf, qConf)
	if err != nil {
		return nil, fmt.Errorf("quic dial: %w", err)
	}
	return &Client{
		cfg:    cfg,
		conn:   conn,
		logger: transport.EnsureLogger(cfg.Logger),
	}, nil
}

// Dial opens a fresh QUIC stream and runs Noise NK over it.
func (c *Client) Dial(ctx context.Context) (transport.TunnelStream, error) {
	stream, err := c.conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}
	hr, err := auth.ClientHandshake(stream, c.cfg.ServerStaticPub, c.cfg.Prologue, []byte("avangard-cli-hello"))
	if err != nil {
		_ = stream.Close()
		return nil, fmt.Errorf("noise handshake: %w", err)
	}
	return &quicStream{
		Stream: stream,
		conn:   c.conn,
		hash:   hr.Hash,
	}, nil
}

// Close tears down the QUIC connection.
func (c *Client) Close() error {
	return c.conn.CloseWithError(0, "client close")
}

// LocalAddr is exposed for diagnostics.
func (c *Client) LocalAddr() net.Addr { return c.conn.LocalAddr() }
