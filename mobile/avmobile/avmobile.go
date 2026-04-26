// Package avmobile is the gomobile-friendly entry point for embedding the
// AVANGARD client in Android (.aar) and iOS (.xcframework) apps.
//
// It exposes a small, foreign-friendly API:
//
//	Start(uri, listenAddr, transport string) error
//	Stop() error
//	IsRunning() bool
//	Version() string
//
// All public functions take and return only types that gomobile supports
// (string, int, bool, error). The package keeps a single global tunnel —
// suitable for typical mobile usage where one VPN session is active at a time.
package avmobile

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/avangard/avangard/pkg/socks5"
	quictransport "github.com/avangard/avangard/pkg/transport/quic"
	tcptransport "github.com/avangard/avangard/pkg/transport/tcp"
	"github.com/avangard/avangard/pkg/tunnel"
	"github.com/avangard/avangard/pkg/uri"
	"github.com/avangard/avangard/pkg/transport"
)

// Version is bumped per release of the mobile binding.
const Version = "0.1.0-alpha"

var (
	mu       sync.Mutex
	running  bool
	cancel   context.CancelFunc
	listener net.Listener
)

// Start parses a `avangard://...` URI, opens an AVANGARD tunnel using the
// requested transport (`tcp` or `quic`), and starts a SOCKS5 listener on
// listenAddr (e.g. "127.0.0.1:18964"). The function returns nil immediately
// after the listener is bound; the SOCKS5 loop runs in a background goroutine.
//
// Calling Start while another tunnel is active returns an error. Use Stop()
// first.
func Start(uriStr, listenAddr, transportName string) error {
	mu.Lock()
	defer mu.Unlock()
	if running {
		return errors.New("avmobile: already running, call Stop first")
	}
	if uriStr == "" {
		return errors.New("avmobile: empty URI")
	}
	if listenAddr == "" {
		listenAddr = "127.0.0.1:18964"
	}
	if transportName == "" {
		transportName = "tcp"
	}

	u, err := uri.Parse(uriStr)
	if err != nil {
		return fmt.Errorf("parse uri: %w", err)
	}

	ctx, cancelFn := context.WithCancel(context.Background())

	cfg := transport.ClientConfig{
		Addr:               net.JoinHostPort(u.Host, fmt.Sprintf("%d", u.Port)),
		UUID:               u.UUID,
		SNI:                u.SNI,
		ServerStaticPub:    u.ServerPub,
		InsecureSkipVerify: u.TOFUHash != "" || u.Fingerprint == "insecure",
		Fingerprint:        u.Fingerprint,
	}
	var dialer transport.Dialer
	switch transportName {
	case "tcp":
		dialer, err = tcptransport.NewClient(cfg)
	case "quic":
		dialer, err = quictransport.NewClient(ctx, cfg)
	default:
		err = fmt.Errorf("unknown transport %q", transportName)
	}
	if err != nil {
		cancelFn()
		return err
	}

	tc := tunnel.NewClient(dialer, u.UUID)
	ln, lnErr := net.Listen("tcp", listenAddr)
	if lnErr != nil {
		cancelFn()
		return fmt.Errorf("socks listen: %w", lnErr)
	}
	srv := socks5.New(tc, nil)
	go func() {
		_ = srv.Serve(ctx, ln)
	}()

	cancel = cancelFn
	listener = ln
	running = true
	return nil
}

// Stop tears down the active tunnel. It is safe to call when no tunnel is
// active (returns nil).
func Stop() error {
	mu.Lock()
	defer mu.Unlock()
	if !running {
		return nil
	}
	if cancel != nil {
		cancel()
	}
	if listener != nil {
		_ = listener.Close()
	}
	running = false
	cancel = nil
	listener = nil
	return nil
}

// IsRunning reports whether a tunnel is currently active.
func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return running
}

// ListenAddr returns the bound SOCKS5 address, or "" if not running.
func ListenAddr() string {
	mu.Lock()
	defer mu.Unlock()
	if listener == nil {
		return ""
	}
	return listener.Addr().String()
}
