package tcptransport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/avangard/avangard/pkg/auth"
	"github.com/avangard/avangard/pkg/transport"
	"github.com/avangard/avangard/pkg/utlsx"
)

// Client is an AVANGARD TCP/TLS dialer.
//
// Each Dial opens a fresh TCP connection (no in-Go mux); for production use
// you would layer SMUX v2 (spec §1.5.3) on top of a single connection.
type Client struct {
	cfg    transport.ClientConfig
	logger *slog.Logger
}

// NewClient validates config and returns a stateless dialer.
func NewClient(cfg transport.ClientConfig) (*Client, error) {
	if cfg.Addr == "" {
		return nil, errors.New("tcp: addr required")
	}
	if len(cfg.ServerStaticPub) != 32 {
		return nil, errors.New("tcp: ServerStaticPub must be 32 bytes")
	}
	if len(cfg.Prologue) == 0 {
		cfg.Prologue = transport.DefaultPrologue
	}
	return &Client{
		cfg:    cfg,
		logger: transport.EnsureLogger(cfg.Logger),
	}, nil
}

// Dial opens a new TCP+TLS connection and runs Noise NK over it.
func (c *Client) Dial(ctx context.Context) (transport.TunnelStream, error) {
	d := &net.Dialer{}
	raw, err := d.DialContext(ctx, "tcp", c.cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("tcp dial: %w", err)
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
		return nil, fmt.Errorf("tls handshake: %w", err)
	}
	hr, err := auth.ClientHandshake(tlsConn, c.cfg.ServerStaticPub, c.cfg.Prologue, []byte("avangard-cli-hello"))
	if err != nil {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("noise: %w", err)
	}
	return &tcpStream{
		raw:  raw,
		conn: tlsConn,
		hash: hr.Hash,
	}, nil
}

// dialTLS performs the TLS handshake either via std crypto/tls or via uTLS,
// depending on whether the client requested a browser fingerprint.
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
