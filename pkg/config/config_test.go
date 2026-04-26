package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServerOK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	body := []byte(`
listen:
  quic: ":443"
  tcp: ":443"
decoy:
  domain: www.yandex.ru
  addr: www.yandex.ru:443
tls:
  cert_file: /etc/avangard/cert.pem
  key_file: /etc/avangard/key.pem
auth:
  static_privkey_b64: AAAA
  accepted_uuids: ["550e8400-e29b-41d4-a716-446655440000"]
logging:
  level: info
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadServer(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Decoy.Domain != "www.yandex.ru" {
		t.Errorf("decoy = %q", c.Decoy.Domain)
	}
	if len(c.Auth.AcceptedUUIDs) != 1 {
		t.Errorf("uuids = %v", c.Auth.AcceptedUUIDs)
	}
}

func TestLoadServerMissingTLS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	body := []byte(`
listen:
  quic: ":443"
auth:
  static_privkey_b64: AA
  accepted_uuids: ["x"]
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadServer(path); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadClientOK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client.yaml")
	body := []byte(`
server:
  uri: "avangard://uid@h.example:443?sni=ya.ru"
transport:
  preferred: auto
regionshield:
  enabled: true
  region: RU
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadClient(path); err != nil {
		t.Fatalf("load: %v", err)
	}
}
