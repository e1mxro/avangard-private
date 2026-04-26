// Package config loads and validates AVANGARD YAML configurations.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ServerConfig is the on-disk YAML for a server install.
type ServerConfig struct {
	Listen struct {
		QUIC string `yaml:"quic"` // ":443"
		TCP  string `yaml:"tcp"`  // ":443"
	} `yaml:"listen"`

	Decoy struct {
		Domain string `yaml:"domain"` // SNI we accept ("magic")
		Addr   string `yaml:"addr"`   // host:port to forward non-magic SNIs
	} `yaml:"decoy"`

	TLS struct {
		CertFile string `yaml:"cert_file"`
		KeyFile  string `yaml:"key_file"`
	} `yaml:"tls"`

	Auth struct {
		StaticPrivKeyB64 string   `yaml:"static_privkey_b64"` // X25519 32 bytes
		AcceptedUUIDs    []string `yaml:"accepted_uuids"`
	} `yaml:"auth"`

	Logging struct {
		Level string `yaml:"level"`
	} `yaml:"logging"`
}

// ClientConfig is the on-disk YAML for a client install.
type ClientConfig struct {
	Server struct {
		URI  string `yaml:"uri"`  // avangard://...
		TOFU string `yaml:"tofu"` // sha256 hex
	} `yaml:"server"`

	Transport struct {
		Preferred string `yaml:"preferred"` // auto|quic|tcp
		Ports     []int  `yaml:"ports"`
	} `yaml:"transport"`

	RegionShield struct {
		Enabled       bool     `yaml:"enabled"`
		Region        string   `yaml:"region"`
		FallbackChain []string `yaml:"fallback_chain"`
	} `yaml:"regionshield"`

	Logging struct {
		Level     string `yaml:"level"`
		File      string `yaml:"file"`
		RedactSNI bool   `yaml:"redact_sni"`
	} `yaml:"logging"`
}

// LoadServer reads & validates a server YAML.
func LoadServer(path string) (*ServerConfig, error) {
	var c ServerConfig
	if err := loadYAML(path, &c); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// LoadClient reads & validates a client YAML.
func LoadClient(path string) (*ClientConfig, error) {
	var c ClientConfig
	if err := loadYAML(path, &c); err != nil {
		return nil, err
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate checks required fields.
func (c *ServerConfig) Validate() error {
	if c.Listen.QUIC == "" && c.Listen.TCP == "" {
		return errors.New("config: at least one of listen.quic / listen.tcp must be set")
	}
	if c.TLS.CertFile == "" || c.TLS.KeyFile == "" {
		return errors.New("config: tls.cert_file and tls.key_file are required")
	}
	if len(c.Auth.AcceptedUUIDs) == 0 {
		return errors.New("config: auth.accepted_uuids must list at least one uuid")
	}
	if c.Auth.StaticPrivKeyB64 == "" {
		return errors.New("config: auth.static_privkey_b64 is required")
	}
	return nil
}

// Validate checks required fields.
func (c *ClientConfig) Validate() error {
	if c.Server.URI == "" {
		return errors.New("config: server.uri required")
	}
	return nil
}

func loadYAML(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}
