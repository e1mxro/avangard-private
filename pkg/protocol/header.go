// Package protocol implements the AVANGARD wire framing.
//
// Header layout, VLESS-style:
//
//	+---------+--------------+-----+------+--------+------+
//	| 1 byte  |  16 bytes    | 1 b | 1 b  |  N b   | 2 b  |
//	|  ver    |  token_hmac  | cmd | atyp | addr   | port |
//	+---------+--------------+-----+------+--------+------+
//
// For atyp == AtypDomain, addr is `[len][len bytes]` (max 256).
// For atyp == AtypIPv4,  addr is exactly 4 bytes.
// For atyp == AtypIPv6,  addr is exactly 16 bytes.
//
// The IPv4-only case fits in 25 bytes; the spec target of "≤ 32 bytes" applies
// only to IP-based addresses. With a max-length domain (255 bytes) the header
// reaches 277 bytes; this is bounded by MaxHeaderLen.
//
// After the header there is no length prefix; payload is raw stream bytes
// (the underlying transport — QUIC stream or SMUX frame — provides framing).
package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
)

// Wire constants.
const (
	HeaderVersion uint8 = 0x01

	CmdConnect      uint8 = 0x1
	CmdUDPAssociate uint8 = 0x2
	CmdPing         uint8 = 0x3
	CmdBind         uint8 = 0x4

	AtypIPv4   uint8 = 0x1
	AtypDomain uint8 = 0x3
	AtypIPv6   uint8 = 0x4

	TokenHMACLen = 16
	// MaxHeaderLen is the absolute maximum encoded header size (when the
	// destination is a 255-byte domain). The IP-only fast path is ≤ 32 bytes.
	MaxHeaderLen = 1 + TokenHMACLen + 1 + 1 + (1 + 255) + 2 // = 277
)

// Header is the AVANGARD request header sent as the first bytes of the
// authenticated tunnel stream.
type Header struct {
	Version   uint8
	TokenHMAC [TokenHMACLen]byte
	Cmd       uint8
	Atyp      uint8
	Addr      []byte // raw bytes (no length prefix); semantics by Atyp
	Port      uint16
}

// Errors.
var (
	ErrShortHeader  = errors.New("avangard: short header")
	ErrBadVersion   = errors.New("avangard: bad version")
	ErrBadAtyp      = errors.New("avangard: bad address type")
	ErrAddrTooLong  = errors.New("avangard: address too long")
	ErrBadCmd       = errors.New("avangard: bad command")
)

// Encode serialises the header. Returns ErrAddrTooLong if the resulting
// header would exceed MaxHeaderLen.
func (h *Header) Encode() ([]byte, error) {
	if !validCmd(h.Cmd) {
		return nil, ErrBadCmd
	}
	addrBytes, err := encodeAddr(h.Atyp, h.Addr)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 1+TokenHMACLen+1+1+len(addrBytes)+2)
	out = append(out, h.Version)
	out = append(out, h.TokenHMAC[:]...)
	out = append(out, h.Cmd, h.Atyp)
	out = append(out, addrBytes...)
	out = binary.BigEndian.AppendUint16(out, h.Port)
	if len(out) > MaxHeaderLen {
		return nil, ErrAddrTooLong
	}
	return out, nil
}

// Decode parses a header from r. It reads exactly the bytes that make up the
// header (no payload) so that the caller can read the stream payload after.
func Decode(r io.Reader) (*Header, error) {
	// Fixed prefix: ver(1) + token(16) + cmd(1) + atyp(1) = 19 bytes.
	prefix := make([]byte, 1+TokenHMACLen+1+1)
	if _, err := io.ReadFull(r, prefix); err != nil {
		return nil, fmt.Errorf("read prefix: %w", err)
	}
	h := &Header{
		Version: prefix[0],
		Cmd:     prefix[1+TokenHMACLen],
		Atyp:    prefix[1+TokenHMACLen+1],
	}
	copy(h.TokenHMAC[:], prefix[1:1+TokenHMACLen])

	if h.Version != HeaderVersion {
		return nil, ErrBadVersion
	}
	if !validCmd(h.Cmd) {
		return nil, ErrBadCmd
	}

	addr, err := decodeAddr(r, h.Atyp)
	if err != nil {
		return nil, err
	}
	h.Addr = addr

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(r, portBuf); err != nil {
		return nil, fmt.Errorf("read port: %w", err)
	}
	h.Port = binary.BigEndian.Uint16(portBuf)
	return h, nil
}

// HostString returns the textual host part for use with net.Dial.
func (h *Header) HostString() string {
	switch h.Atyp {
	case AtypIPv4, AtypIPv6:
		return net.IP(h.Addr).String()
	case AtypDomain:
		return string(h.Addr)
	default:
		return ""
	}
}

// Destination returns "host:port" for net.Dial.
func (h *Header) Destination() string {
	return net.JoinHostPort(h.HostString(), fmt.Sprintf("%d", h.Port))
}

func validCmd(c uint8) bool {
	switch c {
	case CmdConnect, CmdUDPAssociate, CmdPing, CmdBind:
		return true
	}
	return false
}

func encodeAddr(atyp uint8, addr []byte) ([]byte, error) {
	switch atyp {
	case AtypIPv4:
		if len(addr) != 4 {
			return nil, fmt.Errorf("%w: ipv4 must be 4 bytes", ErrBadAtyp)
		}
		return addr, nil
	case AtypIPv6:
		if len(addr) != 16 {
			return nil, fmt.Errorf("%w: ipv6 must be 16 bytes", ErrBadAtyp)
		}
		return addr, nil
	case AtypDomain:
		if len(addr) == 0 || len(addr) > 255 {
			return nil, fmt.Errorf("%w: domain length 1..255", ErrAddrTooLong)
		}
		out := make([]byte, 0, 1+len(addr))
		out = append(out, byte(len(addr)))
		out = append(out, addr...)
		return out, nil
	default:
		return nil, ErrBadAtyp
	}
}

func decodeAddr(r io.Reader, atyp uint8) ([]byte, error) {
	switch atyp {
	case AtypIPv4:
		buf := make([]byte, 4)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return buf, nil
	case AtypIPv6:
		buf := make([]byte, 16)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return buf, nil
	case AtypDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(r, lenBuf); err != nil {
			return nil, err
		}
		n := int(lenBuf[0])
		if n == 0 {
			return nil, fmt.Errorf("%w: zero-length domain", ErrBadAtyp)
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return buf, nil
	default:
		return nil, ErrBadAtyp
	}
}
