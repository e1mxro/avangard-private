// Package tunnel ties protocol + transport + auth together: it runs an
// AVANGARD server that accepts authenticated streams, parses the AVANGARD
// header, dials the requested destination, and shovels bytes bi-directionally.
package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/avangard/avangard/pkg/auth"
	"github.com/avangard/avangard/pkg/protocol"
	"github.com/avangard/avangard/pkg/transport"
)

// Server runs the inbound side of the tunnel.
type Server struct {
	listener      transport.Listener
	acceptedUUIDs []string
	logger        *slog.Logger
}

// NewServer constructs a Server.
func NewServer(ln transport.Listener, acceptedUUIDs []string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{listener: ln, acceptedUUIDs: acceptedUUIDs, logger: logger}
}

// Serve runs accept-loop until ctx is done.
func (s *Server) Serve(ctx context.Context) error {
	for {
		ts, err := s.listener.Accept(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			s.logger.Error("accept", "err", err)
			return err
		}
		go s.handle(ts)
	}
}

func (s *Server) handle(ts transport.TunnelStream) {
	defer ts.Close()

	// Bound the time we wait for the AVANGARD header.
	type readResult struct {
		h   *protocol.Header
		err error
	}
	rc := make(chan readResult, 1)
	go func() {
		h, err := protocol.Decode(ts)
		rc <- readResult{h, err}
	}()
	var header *protocol.Header
	select {
	case r := <-rc:
		if r.err != nil {
			s.logger.Debug("header decode", "remote", ts.RemoteAddr(), "err", r.err)
			return
		}
		header = r.h
	case <-time.After(10 * time.Second):
		s.logger.Debug("header timeout", "remote", ts.RemoteAddr())
		return
	}

	if !s.tokenValid(header.TokenHMAC, ts.Channel()) {
		s.logger.Warn("token rejected", "remote", ts.RemoteAddr())
		return
	}

	switch header.Cmd {
	case protocol.CmdConnect:
		s.handleConnect(ts, header)
	case protocol.CmdPing:
		_, _ = ts.Write([]byte("PONG"))
	default:
		s.logger.Debug("unsupported cmd", "cmd", header.Cmd)
	}
}

func (s *Server) handleConnect(ts transport.TunnelStream, h *protocol.Header) {
	dest := h.Destination()
	upstream, err := net.DialTimeout("tcp", dest, 10*time.Second)
	if err != nil {
		s.logger.Debug("upstream dial", "dest", dest, "err", err)
		return
	}
	defer upstream.Close()

	s.logger.Info("tunnel open", "remote", ts.RemoteAddr(), "dest", dest)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(upstream, ts) }()
	go func() { defer wg.Done(); _, _ = io.Copy(ts, upstream) }()
	wg.Wait()
}

// tokenValid checks whether `got` matches HMAC(uuid, channel) for any of the
// accepted UUIDs. Constant-time per UUID.
func (s *Server) tokenValid(got [protocol.TokenHMACLen]byte, channel []byte) bool {
	for _, u := range s.acceptedUUIDs {
		if auth.VerifyToken(u, channel, got) {
			return true
		}
	}
	return false
}

// Close shuts the server down.
func (s *Server) Close() error {
	return s.listener.Close()
}

// CompositeListener fans Accept across multiple underlying listeners (e.g.
// QUIC + TCP fallback in parallel).
type CompositeListener struct {
	listeners []transport.Listener
	out       chan acceptItem
	done      chan struct{}
	closeOnce sync.Once
}

type acceptItem struct {
	s   transport.TunnelStream
	err error
}

// NewComposite combines multiple listeners.
func NewComposite(lns ...transport.Listener) *CompositeListener {
	c := &CompositeListener{
		listeners: lns,
		out:       make(chan acceptItem, len(lns)*4),
		done:      make(chan struct{}),
	}
	for _, ln := range lns {
		ln := ln
		go func() {
			for {
				ts, err := ln.Accept(context.Background())
				select {
				case c.out <- acceptItem{ts, err}:
				case <-c.done:
					return
				}
				if err != nil {
					return
				}
			}
		}()
	}
	return c
}

func (c *CompositeListener) Accept(ctx context.Context) (transport.TunnelStream, error) {
	select {
	case item := <-c.out:
		return item.s, item.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, net.ErrClosed
	}
}

func (c *CompositeListener) Close() error {
	c.closeOnce.Do(func() { close(c.done) })
	var firstErr error
	for _, l := range c.listeners {
		if err := l.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (c *CompositeListener) Addr() net.Addr {
	if len(c.listeners) == 0 {
		return nil
	}
	return c.listeners[0].Addr()
}

// Sanity fmt import.
var _ = fmt.Sprintf
