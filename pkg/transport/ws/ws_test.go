package wstransport

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
	"net"
	"strings"
	"testing"
	"time"

	"github.com/avangard/avangard/pkg/auth"
	"github.com/avangard/avangard/pkg/transport"
)

func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
}

func TestWSEcho(t *testing.T) {
	cert := selfSignedCert(t)
	staticKey, err := auth.GenerateStatic()
	if err != nil {
		t.Fatal(err)
	}

	// Pick a random TCP port.
	probe, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := probe.Addr().String()
	probe.Close()

	srvCfg := transport.ServerConfig{
		Addr:          addr,
		AcceptedUUIDs: []string{"x"},
		NoiseStatic:   staticKey,
		TLSCert:       cert,
	}
	srv, err := ListenTLS(srvCfg, "")
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	// Server-side echo loop.
	go func() {
		for {
			ts, err := srv.Accept(context.Background())
			if err != nil {
				return
			}
			go func(ts transport.TunnelStream) {
				defer ts.Close()
				_, _ = io.Copy(ts, ts)
			}(ts)
		}
	}()

	// Give server a moment to start accepting.
	time.Sleep(100 * time.Millisecond)

	cli, err := NewClient(transport.ClientConfig{
		Addr:               addr,
		UUID:               "x",
		SNI:                "localhost",
		ServerStaticPub:    staticKey.Public,
		InsecureSkipVerify: true,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := cli.Dial(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	want := strings.Repeat("hello-ws", 32)
	if _, err := conn.Write([]byte(want)); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("echo mismatch")
	}
}
