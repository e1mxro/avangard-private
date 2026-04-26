package tcptransport

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	mrand "math/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/avangard/avangard/pkg/auth"
	"github.com/avangard/avangard/pkg/transport"
)

// startDecoyHTTPS launches a tiny TLS-terminating HTTPS server that always
// answers "HTTP/1.1 200 OK\r\n\r\nDECOY". It plays the role of the legitimate
// site that probes are bounced to.
func startDecoyHTTPS(t *testing.T) (addr string, hits *int64, stop func()) {
	t.Helper()
	cert := makeCert(t, "decoy.test")
	tlsConf := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsConf)
	if err != nil {
		t.Fatal(err)
	}
	var counter int64
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddInt64(&counter, 1)
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, _ = c.Read(buf)
				_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nDECOY")
			}(c)
		}
	}()
	return ln.Addr().String(), &counter, func() { _ = ln.Close() }
}

func makeCert(t *testing.T, cn string) tls.Certificate {
	t.Helper()
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{cn, "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
}

// TestActiveProbeForwarding verifies that ClientHellos with non-magic SNIs
// are transparently proxied to the decoy server. This is what makes AVANGARD
// indistinguishable from a regular HTTPS site to active probers.
func TestActiveProbeForwarding(t *testing.T) {
	decoyAddr, decoyHits, stopDecoy := startDecoyHTTPS(t)
	defer stopDecoy()

	cert := makeCert(t, "magic.local")
	staticKey, _ := auth.GenerateStatic()
	srv, err := Listen(transport.ServerConfig{
		Addr:          "127.0.0.1:0",
		AcceptedUUIDs: []string{"x"},
		NoiseStatic:   staticKey,
		TLSCert:       cert,
		DecoyAddr:     decoyAddr,
	}, []string{"magic.local"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	srvAddr := srv.Addr().String()

	const N = 32
	var wg sync.WaitGroup
	wg.Add(N)
	var ok int64
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			// Random non-magic SNI from a pool.
			pool := []string{"yandex.ru", "www.gosuslugi.ru", "vk.com", "telegram.org", "google.com", "ok.ru"}
			sni := pool[mrand.Intn(len(pool))]
			d := &net.Dialer{Timeout: 3 * time.Second}
			c, err := d.Dial("tcp", srvAddr)
			if err != nil {
				t.Errorf("probe %d dial: %v", i, err)
				return
			}
			defer c.Close()
			tlsConf := &tls.Config{ServerName: sni, InsecureSkipVerify: true} //nolint:gosec
			tc := tls.Client(c, tlsConf)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := tc.HandshakeContext(ctx); err != nil {
				t.Errorf("probe %d tls: %v", i, err)
				return
			}
			// Send something to make sure data reaches decoy and round-trips.
			_, _ = io.WriteString(tc, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
			buf := make([]byte, 256)
			n, _ := tc.Read(buf)
			if strings.Contains(string(buf[:n]), "DECOY") {
				atomic.AddInt64(&ok, 1)
			}
		}()
	}
	wg.Wait()

	hits := atomic.LoadInt64(decoyHits)
	if hits < int64(N) {
		t.Fatalf("decoy received %d hits, want %d", hits, N)
	}
	if atomic.LoadInt64(&ok) < int64(N) {
		t.Fatalf("only %d/%d probes saw the DECOY response", ok, N)
	}
}

// TestMagicSNIBypassesDecoy verifies that a client that knows the magic SNI
// reaches the AVANGARD code path (TLS termination + Noise handshake fails
// because it's not a real client) and is NOT forwarded to decoy.
func TestMagicSNIBypassesDecoy(t *testing.T) {
	decoyAddr, decoyHits, stopDecoy := startDecoyHTTPS(t)
	defer stopDecoy()

	cert := makeCert(t, "magic.local")
	staticKey, _ := auth.GenerateStatic()
	srv, err := Listen(transport.ServerConfig{
		Addr:          "127.0.0.1:0",
		AcceptedUUIDs: []string{"x"},
		NoiseStatic:   staticKey,
		TLSCert:       cert,
		DecoyAddr:     decoyAddr,
	}, []string{"magic.local"})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	d := &net.Dialer{Timeout: 3 * time.Second}
	c, err := d.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	tlsConf := &tls.Config{ServerName: "magic.local", InsecureSkipVerify: true} //nolint:gosec
	tc := tls.Client(c, tlsConf)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := tc.HandshakeContext(ctx); err != nil {
		t.Fatal(err)
	}
	// Send garbage; Noise handshake will reject.
	_, _ = tc.Write([]byte("garbage-not-noise"))
	_ = tc.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 256)
	_, _ = tc.Read(buf)
	if atomic.LoadInt64(decoyHits) != 0 {
		t.Fatalf("decoy was hit %d times, expected 0 for magic SNI", *decoyHits)
	}
}
