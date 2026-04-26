package smuxtcp

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
	"sync"
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

func TestSmuxMultipleStreams(t *testing.T) {
	cert := selfSignedCert(t)
	staticKey, err := auth.GenerateStatic()
	if err != nil {
		t.Fatal(err)
	}

	srv, err := Listen(transport.ServerConfig{
		Addr:          "127.0.0.1:0",
		AcceptedUUIDs: []string{"x"},
		NoiseStatic:   staticKey,
		TLSCert:       cert,
	})
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

	cli, err := NewClient(transport.ClientConfig{
		Addr:               srv.Addr().String(),
		UUID:               "x",
		SNI:                "localhost",
		ServerStaticPub:    staticKey.Public,
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()

	ctx := context.Background()
	const N = 8
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			s, err := cli.Dial(ctx)
			if err != nil {
				t.Errorf("dial %d: %v", i, err)
				return
			}
			defer s.Close()
			payload := []byte("smux-stream-X")
			payload[len(payload)-1] = byte('0' + i)
			if _, err := s.Write(payload); err != nil {
				t.Errorf("write %d: %v", i, err)
				return
			}
			got := make([]byte, len(payload))
			if _, err := io.ReadFull(s, got); err != nil {
				t.Errorf("read %d: %v", i, err)
				return
			}
			if string(got) != string(payload) {
				t.Errorf("mismatch %d: %q != %q", i, got, payload)
			}
		}()
	}
	wg.Wait()
}
