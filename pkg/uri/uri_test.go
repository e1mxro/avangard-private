package uri

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseFull(t *testing.T) {
	pk := make([]byte, 32)
	if _, err := rand.Read(pk); err != nil {
		t.Fatal(err)
	}
	pkb64 := base64.RawURLEncoding.EncodeToString(pk)

	s := "avangard://550e8400-e29b-41d4-a716-446655440000@example.com:443" +
		"?sni=www.yandex.ru&fp=chrome125&mode=yandex_cdn&region=RU" +
		"&pk=" + pkb64 + "&tofu=abcd1234#prod"
	c, err := Parse(s)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.UUID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("uuid: %q", c.UUID)
	}
	if c.Host != "example.com" || c.Port != 443 {
		t.Errorf("host:port: %s:%d", c.Host, c.Port)
	}
	if c.SNI != "www.yandex.ru" {
		t.Errorf("sni: %q", c.SNI)
	}
	if c.Mode != ModeYandexCDN {
		t.Errorf("mode: %q", c.Mode)
	}
	if c.Region != "RU" {
		t.Errorf("region: %q", c.Region)
	}
	if !bytes.Equal(c.ServerPub, pk) {
		t.Errorf("pk mismatch")
	}
	if c.Name != "prod" {
		t.Errorf("name: %q", c.Name)
	}
}

func TestParseDefaults(t *testing.T) {
	c, err := Parse("avangard://uid@1.2.3.4:8443")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.Mode != ModeAuto {
		t.Errorf("default mode = %q", c.Mode)
	}
	if c.Region != "auto" {
		t.Errorf("default region = %q", c.Region)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []string{
		"https://uid@x:1",
		"avangard://x:1",
		"avangard://uid@:1",
		"avangard://uid@x",
		"avangard://uid@x:abc",
	}
	for _, s := range cases {
		if _, err := Parse(s); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

func TestStringRoundTrip(t *testing.T) {
	c := &Config{
		UUID: "uid", Host: "h.example", Port: 443,
		SNI: "decoy", Fingerprint: "fp", Mode: ModeAuto, Region: "RU",
	}
	round, err := Parse(c.String())
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if round.UUID != c.UUID || round.Host != c.Host || round.Port != c.Port {
		t.Errorf("round-trip mismatch: %+v vs %+v", round, c)
	}
	if !strings.Contains(c.String(), "sni=decoy") {
		t.Errorf("missing sni in encoded URI: %s", c.String())
	}
}
