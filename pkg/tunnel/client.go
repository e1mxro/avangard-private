package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/avangard/avangard/pkg/auth"
	"github.com/avangard/avangard/pkg/protocol"
	"github.com/avangard/avangard/pkg/transport"
)

// Client opens AVANGARD-tunnelled streams to remote destinations.
type Client struct {
	dialer transport.Dialer
	uuid   string
}

// NewClient constructs a Client.
func NewClient(d transport.Dialer, uuid string) *Client {
	return &Client{dialer: d, uuid: uuid}
}

// DialTunnel opens a tunnel to dest ("host:port"), returning a net.Conn-like
// that pipes through the AVANGARD server.
func (c *Client) DialTunnel(ctx context.Context, dest string) (transport.TunnelStream, error) {
	host, portStr, err := net.SplitHostPort(dest)
	if err != nil {
		return nil, fmt.Errorf("split host:port: %w", err)
	}
	atyp, addr, err := encodeDest(host)
	if err != nil {
		return nil, err
	}
	port, err := parsePort(portStr)
	if err != nil {
		return nil, err
	}

	ts, err := c.dialer.Dial(ctx)
	if err != nil {
		return nil, err
	}

	hdr := &protocol.Header{
		Version:   protocol.HeaderVersion,
		TokenHMAC: auth.DeriveToken(c.uuid, ts.Channel()),
		Cmd:       protocol.CmdConnect,
		Atyp:      atyp,
		Addr:      addr,
		Port:      port,
	}
	encoded, err := hdr.Encode()
	if err != nil {
		_ = ts.Close()
		return nil, err
	}
	if _, err := ts.Write(encoded); err != nil {
		_ = ts.Close()
		return nil, err
	}
	return ts, nil
}

func encodeDest(host string) (uint8, []byte, error) {
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return protocol.AtypIPv4, v4, nil
		}
		v6 := ip.To16()
		if v6 == nil {
			return 0, nil, errors.New("bad ip")
		}
		return protocol.AtypIPv6, v6, nil
	}
	if len(host) == 0 || len(host) > 255 {
		return 0, nil, errors.New("bad domain length")
	}
	return protocol.AtypDomain, []byte(host), nil
}

func parsePort(s string) (uint16, error) {
	var p uint16
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("bad port %q", s)
		}
		p = p*10 + uint16(c-'0')
	}
	return p, nil
}
