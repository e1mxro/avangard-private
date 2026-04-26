package tunnel

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/avangard/avangard/pkg/auth"
	"github.com/avangard/avangard/pkg/transport"
	quictransport "github.com/avangard/avangard/pkg/transport/quic"
	tcptransport "github.com/avangard/avangard/pkg/transport/tcp"
)

// generateSelfSigned returns a tls.Certificate suitable for tests.
func generateSelfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "avangard-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"avangard-test", "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(priv)
	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
	}
	_ = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return cert
}

// startEchoServer returns an "echo" TCP server: writes back what it reads.
func startEchoServer(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop = func() { _ = ln.Close() }
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(c)
		}
	}()
	return ln.Addr().String(), stop
}

func TestE2E_QUIC(t *testing.T) {
	echoAddr, stopEcho := startEchoServer(t)
	defer stopEcho()

	cert := generateSelfSigned(t)
	staticKey, err := auth.GenerateStatic()
	if err != nil {
		t.Fatal(err)
	}
	uuid := "550e8400-e29b-41d4-a716-446655440000"

	srvCfg := transport.ServerConfig{
		Addr:          "127.0.0.1:0",
		AcceptedUUIDs: []string{uuid},
		NoiseStatic:   staticKey,
		TLSCert:       cert,
	}
	qln, err := quictransport.Listen(srvCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer qln.Close()

	tunnelSrv := NewServer(qln, srvCfg.AcceptedUUIDs, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tunnelSrv.Serve(ctx)

	cli, err := quictransport.NewClient(ctx, transport.ClientConfig{
		Addr:               qln.Addr().String(),
		UUID:               uuid,
		SNI:                "localhost",
		ServerStaticPub:    staticKey.Public,
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()

	tc := NewClient(cli, uuid)
	conn, err := tc.DialTunnel(ctx, echoAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	payload := []byte("hello-avangard-quic")
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("echo mismatch: got %q want %q", got, payload)
	}
}

func TestE2E_TCP(t *testing.T) {
	echoAddr, stopEcho := startEchoServer(t)
	defer stopEcho()

	cert := generateSelfSigned(t)
	staticKey, err := auth.GenerateStatic()
	if err != nil {
		t.Fatal(err)
	}
	uuid := "tcp-uuid-test"

	srvCfg := transport.ServerConfig{
		Addr:          "127.0.0.1:0",
		AcceptedUUIDs: []string{uuid},
		NoiseStatic:   staticKey,
		TLSCert:       cert,
	}
	// Empty magicSNIs => no SNI gating, accepts everything.
	tln, err := tcptransport.Listen(srvCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tln.Close()

	tunnelSrv := NewServer(tln, srvCfg.AcceptedUUIDs, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tunnelSrv.Serve(ctx)

	cli, err := tcptransport.NewClient(transport.ClientConfig{
		Addr:               tln.Addr().String(),
		UUID:               uuid,
		SNI:                "localhost",
		ServerStaticPub:    staticKey.Public,
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	tc := NewClient(cli, uuid)
	conn, err := tc.DialTunnel(ctx, echoAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	payload := []byte("hello-avangard-tcp")
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("echo mismatch: got %q want %q", got, payload)
	}
	_ = strconv.Itoa
}
