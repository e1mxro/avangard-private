package auth

import (
	"bytes"
	"io"
	"sync"
	"testing"
)

// pipePair implements two duplex pipes connected to each other.
type pipePair struct {
	a, b *bytePipe
}

type bytePipe struct {
	mu   sync.Mutex
	cond *sync.Cond
	buf  bytes.Buffer
	closed bool
}

func newBytePipe() *bytePipe {
	bp := &bytePipe{}
	bp.cond = sync.NewCond(&bp.mu)
	return bp
}

func (p *bytePipe) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, io.ErrClosedPipe
	}
	n, err := p.buf.Write(b)
	p.cond.Broadcast()
	return n, err
}

func (p *bytePipe) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for p.buf.Len() == 0 {
		if p.closed {
			return 0, io.EOF
		}
		p.cond.Wait()
	}
	return p.buf.Read(b)
}

type duplex struct {
	r io.Reader
	w io.Writer
}

func (d duplex) Read(b []byte) (int, error)  { return d.r.Read(b) }
func (d duplex) Write(b []byte) (int, error) { return d.w.Write(b) }

func TestNoiseNKHandshake(t *testing.T) {
	cliToSrv := newBytePipe()
	srvToCli := newBytePipe()

	cliRW := duplex{r: srvToCli, w: cliToSrv}
	srvRW := duplex{r: cliToSrv, w: srvToCli}

	srvStatic, err := GenerateStatic()
	if err != nil {
		t.Fatal(err)
	}

	prologue := []byte("avangard-v1")
	cliInitPayload := []byte("hello-from-client")
	srvReplyPayload := []byte("welcome-bw=1Gbps")

	type result struct {
		hr  *HandshakeResult
		err error
	}
	cliCh := make(chan result, 1)
	srvCh := make(chan result, 1)

	go func() {
		hr, err := ClientHandshake(cliRW, srvStatic.Public, prologue, cliInitPayload)
		cliCh <- result{hr, err}
	}()
	go func() {
		hr, err := ServerHandshake(srvRW, srvStatic, prologue, srvReplyPayload)
		srvCh <- result{hr, err}
	}()

	cli := <-cliCh
	srv := <-srvCh

	if cli.err != nil || srv.err != nil {
		t.Fatalf("handshake errors: cli=%v srv=%v", cli.err, srv.err)
	}
	if !bytes.Equal(cli.hr.Payload, srvReplyPayload) {
		t.Fatalf("client got wrong payload: %q", cli.hr.Payload)
	}
	if !bytes.Equal(srv.hr.Payload, cliInitPayload) {
		t.Fatalf("server got wrong payload: %q", srv.hr.Payload)
	}
	if !bytes.Equal(cli.hr.Hash, srv.hr.Hash) {
		t.Fatalf("transcript hashes differ")
	}

	// Encrypt-decrypt smoke
	pt := []byte("the quick brown fox")
	ct, err := cli.hr.Send.Encrypt(nil, nil, pt)
	if err != nil {
		t.Fatal(err)
	}
	got, err := srv.hr.Recv.Decrypt(nil, nil, ct)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pt) {
		t.Fatalf("aead mismatch: %q vs %q", got, pt)
	}
}
