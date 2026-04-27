// Package avmobile is the gomobile-friendly entry point for embedding the
// AVANGARD client in Android (.aar) and iOS (.xcframework) apps.
//
// It exposes a small, foreign-friendly API:
//
//	Start(uri, listenAddr, transport string) error
//	Stop() error
//	IsRunning() bool
//	StartTun(tunFd, mtu int, socksAddr string) error  // v0.2 system-wide
//	StopTun() error
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
	"strconv"
	"sync"

	"github.com/avangard/avangard/pkg/socks5"
	"github.com/avangard/avangard/pkg/transport"
	quictransport "github.com/avangard/avangard/pkg/transport/quic"
	tcptransport "github.com/avangard/avangard/pkg/transport/tcp"
	"github.com/avangard/avangard/pkg/tunnel"
	"github.com/avangard/avangard/pkg/uri"

	"github.com/xjasonlyu/tun2socks/v2/engine"
	t2slog "github.com/xjasonlyu/tun2socks/v2/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Version is bumped per release of the mobile binding.
const Version = "0.2.0-alpha"

func init() {
	// xjasonlyu/tun2socks's engine.Start / engine.Stop call log.Fatalf on
	// unrecoverable errors, which by default invokes os.Exit and would tear
	// down the entire host process (the Android app). Replace the global
	// logger with a no-op zap logger that converts Fatal-level events into
	// goroutine exits so we can recover and surface the error instead.
	nop := zap.NewNop().WithOptions(zap.WithFatalHook(zapcore.WriteThenGoexit))
	t2slog.SetLogger(nop)
}

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

// --- v0.2 system-wide VPN (Android VpnService / iOS NEPacketTunnelProvider) ---
//
// The mobile platform creates a tun-style file descriptor that delivers raw
// IP packets from the OS. We use xjasonlyu/tun2socks to build a userspace
// network stack on top of that fd and forward each TCP/UDP flow to the
// SOCKS5 listener that Start() exposes on 127.0.0.1:18964.

var (
	tunMu      sync.Mutex
	tunRunning bool
)

// StartTun connects a tun-style file descriptor (as produced by Android's
// VpnService.Builder.establish() or iOS's NEPacketTunnelFlow) to the SOCKS5
// proxy address. Once running, every IP packet written by the OS into the
// tun fd is parsed by an internal netstack and forwarded to socksAddr.
//
// tunFd: file descriptor handed over by the platform. Caller is responsible
//        for keeping it open until StopTun() returns.
// mtu:   maximum transmission unit. Pass 0 to use the default (1500).
// socksAddr: address of the SOCKS5 proxy started by Start(). Pass "" for
//        the default 127.0.0.1:18964.
func StartTun(tunFd, mtu int, socksAddr string) error {
	tunMu.Lock()
	defer tunMu.Unlock()
	if tunRunning {
		return errors.New("avmobile: tun already running, call StopTun first")
	}
	if tunFd <= 0 {
		return errors.New("avmobile: invalid tun fd")
	}
	if mtu <= 0 {
		mtu = 1500
	}
	if socksAddr == "" {
		socksAddr = "127.0.0.1:18964"
	}

	key := &engine.Key{
		Device:   "fd://" + strconv.Itoa(tunFd),
		Proxy:    "socks5://" + socksAddr,
		MTU:      mtu,
		LogLevel: "warning",
	}
	engine.Insert(key)

	var startErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				startErr = fmt.Errorf("tun2socks start: %v", r)
			}
		}()
		engine.Start()
	}()
	if startErr != nil {
		return startErr
	}

	tunRunning = true
	return nil
}

// StopTun shuts down the tun2socks netstack started by StartTun. Safe to
// call when no tun is active. Does not close the underlying fd; the
// platform is expected to manage its lifetime.
func StopTun() error {
	tunMu.Lock()
	defer tunMu.Unlock()
	if !tunRunning {
		return nil
	}
	func() {
		defer func() { _ = recover() }()
		engine.Stop()
	}()
	tunRunning = false
	return nil
}

// IsTunRunning reports whether the system-wide VPN packet pump is active.
func IsTunRunning() bool {
	tunMu.Lock()
	defer tunMu.Unlock()
	return tunRunning
}
