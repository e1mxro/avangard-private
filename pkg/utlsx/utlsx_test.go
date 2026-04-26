package utlsx

import (
	"crypto/tls"
	"testing"
)

func TestResolve(t *testing.T) {
	cases := []struct {
		fp   Fingerprint
		want bool
	}{
		{FingerprintAuto, true},
		{FingerprintChrome, true},
		{FingerprintFirefox, true},
		{FingerprintSafari, true},
		{FingerprintIOS, true},
		{FingerprintYandexBrowser, true},
		{Fingerprint("nonsense"), false},
	}
	for _, c := range cases {
		_, err := resolve(c.fp)
		if (err == nil) != c.want {
			t.Errorf("resolve(%q): got err=%v, want=%v", c.fp, err, c.want)
		}
	}
}

func TestFingerprintFromString(t *testing.T) {
	if got := FingerprintFromString("chrome"); got != FingerprintChrome {
		t.Errorf("got %q", got)
	}
	if got := FingerprintFromString("nonsense"); got != FingerprintAuto {
		t.Errorf("got %q", got)
	}
}

func TestNewRequiresConfig(t *testing.T) {
	_, err := New(nil, nil, FingerprintChrome)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewRejectsUnknownFP(t *testing.T) {
	_, err := New(nil, &tls.Config{}, "nonsense")
	if err == nil {
		t.Fatal("expected error for unknown fp")
	}
}
