package protocol

import (
	"bytes"
	"net"
	"testing"
)

func TestHeaderRoundTripIPv4(t *testing.T) {
	h := &Header{
		Version: HeaderVersion,
		Cmd:     CmdConnect,
		Atyp:    AtypIPv4,
		Addr:    net.IPv4(1, 2, 3, 4).To4(),
		Port:    443,
	}
	for i := range h.TokenHMAC {
		h.TokenHMAC[i] = byte(i)
	}
	encoded, err := h.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// IPv4-only path fits the tight 32-byte budget.
	if len(encoded) > 32 {
		t.Fatalf("ipv4 header %d > 32", len(encoded))
	}
	got, err := Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Version != h.Version || got.Cmd != h.Cmd || got.Atyp != h.Atyp || got.Port != h.Port {
		t.Fatalf("mismatch: %+v vs %+v", got, h)
	}
	if !bytes.Equal(got.Addr, h.Addr) {
		t.Fatalf("addr mismatch: %v vs %v", got.Addr, h.Addr)
	}
	if got.TokenHMAC != h.TokenHMAC {
		t.Fatalf("token mismatch")
	}
	if got.Destination() != "1.2.3.4:443" {
		t.Fatalf("dest = %q", got.Destination())
	}
}

func TestHeaderRoundTripDomain(t *testing.T) {
	domain := "www.yandex.ru"
	h := &Header{
		Version: HeaderVersion,
		Cmd:     CmdConnect,
		Atyp:    AtypDomain,
		Addr:    []byte(domain),
		Port:    443,
	}
	encoded, err := h.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(encoded) > MaxHeaderLen {
		t.Fatalf("header %d > max %d", len(encoded), MaxHeaderLen)
	}
	got, err := Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.HostString() != domain {
		t.Fatalf("got %q want %q", got.HostString(), domain)
	}
}

func TestHeaderRejectBadVersion(t *testing.T) {
	buf := make([]byte, 25)
	buf[0] = 0xFF
	if _, err := Decode(bytes.NewReader(buf)); err != ErrBadVersion {
		t.Fatalf("expected ErrBadVersion, got %v", err)
	}
}

func TestHeaderRejectBadCmd(t *testing.T) {
	h := &Header{Version: HeaderVersion, Cmd: 0xF, Atyp: AtypIPv4, Addr: []byte{1, 2, 3, 4}, Port: 1}
	if _, err := h.Encode(); err == nil {
		t.Fatalf("expected error for bad cmd")
	}
}

func TestHeaderDomainTooLong(t *testing.T) {
	long := bytes.Repeat([]byte{'a'}, 256)
	h := &Header{Version: HeaderVersion, Cmd: CmdConnect, Atyp: AtypDomain, Addr: long, Port: 1}
	if _, err := h.Encode(); err == nil {
		t.Fatalf("expected error for long domain")
	}
}
