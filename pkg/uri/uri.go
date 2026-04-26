// Package uri parses the AVANGARD URI scheme:
//
//	avangard://<uuid>@<host>:<port>?
//	          sni=<decoy>&fp=<fp>&mode=<mode>&region=<region>
//	          &pk=<server_x25519_pub_b64>&tofu=<sha256_hex>
//	          #<name>
package uri

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const Scheme = "avangard"

// Mode is one of the RegionShield transport modes.
type Mode string

const (
	ModeAuto             Mode = "auto"
	ModeYandexCDN        Mode = "yandex_cdn" // RU mode A
	ModeGosuslugiWS      Mode = "gosuslugi_ws"
	ModeSplitDPI         Mode = "splitdpi"
	ModeQUICCamouflage   Mode = "quic_camouflage"
	ModeDNSTunnel        Mode = "dns_tunnel"
)

// Config is the parsed URI form, suitable for the client.
type Config struct {
	UUID        string
	Host        string
	Port        int
	SNI         string // decoy domain
	Fingerprint string
	Mode        Mode
	Region      string
	ServerPub   []byte // X25519 server static pubkey (32 bytes), optional
	TOFUHash    string // sha256 hex of server-static-pub for pinning
	Name        string // fragment / display name
}

// Parse parses an avangard:// URI.
func Parse(s string) (*Config, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	if u.Scheme != Scheme {
		return nil, fmt.Errorf("uri: bad scheme %q", u.Scheme)
	}
	if u.User == nil || u.User.Username() == "" {
		return nil, errors.New("uri: missing uuid")
	}
	host := u.Hostname()
	if host == "" {
		return nil, errors.New("uri: missing host")
	}
	portStr := u.Port()
	if portStr == "" {
		return nil, errors.New("uri: missing port")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("uri: bad port: %w", err)
	}
	q := u.Query()

	cfg := &Config{
		UUID:        u.User.Username(),
		Host:        host,
		Port:        port,
		SNI:         q.Get("sni"),
		Fingerprint: q.Get("fp"),
		Mode:        Mode(getOrDefault(q.Get("mode"), string(ModeAuto))),
		Region:      getOrDefault(q.Get("region"), "auto"),
		TOFUHash:    q.Get("tofu"),
		Name:        u.Fragment,
	}
	if pk := q.Get("pk"); pk != "" {
		raw, err := base64.RawURLEncoding.DecodeString(pk)
		if err != nil {
			// Tolerate std base64 too.
			raw, err = base64.StdEncoding.DecodeString(pk)
			if err != nil {
				return nil, fmt.Errorf("uri: pk base64: %w", err)
			}
		}
		if len(raw) != 32 {
			return nil, fmt.Errorf("uri: pk must be 32 bytes, got %d", len(raw))
		}
		cfg.ServerPub = raw
	}
	return cfg, nil
}

// String reconstructs the URI form.
func (c *Config) String() string {
	q := url.Values{}
	if c.SNI != "" {
		q.Set("sni", c.SNI)
	}
	if c.Fingerprint != "" {
		q.Set("fp", c.Fingerprint)
	}
	if c.Mode != "" {
		q.Set("mode", string(c.Mode))
	}
	if c.Region != "" {
		q.Set("region", c.Region)
	}
	if len(c.ServerPub) == 32 {
		q.Set("pk", base64.RawURLEncoding.EncodeToString(c.ServerPub))
	}
	if c.TOFUHash != "" {
		q.Set("tofu", c.TOFUHash)
	}
	u := url.URL{
		Scheme:   Scheme,
		User:     url.User(c.UUID),
		Host:     net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		RawQuery: q.Encode(),
		Fragment: c.Name,
	}
	out := u.String()
	// Remove trailing '?' if no query.
	out = strings.TrimSuffix(out, "?")
	return out
}

func getOrDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
