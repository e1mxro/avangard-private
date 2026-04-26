package regionshield

import (
	"testing"

	"github.com/avangard/avangard/pkg/autoprobe"
	"github.com/avangard/avangard/pkg/uri"
)

func TestSelectorRU_HappyPath(t *testing.T) {
	prof := DefaultRUProfile()
	probe := autoprobe.Result{
		UDP443: autoprobe.ProbeStatus{OK: true},
		TCP443: autoprobe.ProbeStatus{OK: true},
	}
	d := Select(SelectorInput{Profile: prof, Probe: probe})
	if d.Mode != uri.ModeYandexCDN {
		t.Errorf("expected yandex_cdn, got %q (reason=%s)", d.Mode, d.Reason)
	}
}

func TestSelectorRU_TCPOnlyFallsBack(t *testing.T) {
	prof := DefaultRUProfile()
	probe := autoprobe.Result{
		UDP443: autoprobe.ProbeStatus{OK: false},
		TCP443: autoprobe.ProbeStatus{OK: true},
	}
	d := Select(SelectorInput{Profile: prof, Probe: probe})
	if d.Mode == uri.ModeQUICCamouflage {
		t.Errorf("must not pick QUIC when UDP is dead")
	}
	if d.Mode != uri.ModeYandexCDN {
		t.Errorf("expected yandex_cdn first, got %q", d.Mode)
	}
}

func TestSelectorRU_AllBlockedFallsBackToDNS(t *testing.T) {
	prof := DefaultRUProfile()
	probe := autoprobe.Result{
		UDP443: autoprobe.ProbeStatus{OK: false},
		TCP443: autoprobe.ProbeStatus{OK: false},
	}
	d := Select(SelectorInput{Profile: prof, Probe: probe})
	if d.Mode != uri.ModeDNSTunnel {
		t.Errorf("expected dns_tunnel last-resort, got %q", d.Mode)
	}
}

func TestSelectorRU_BeelineOverride(t *testing.T) {
	prof := DefaultRUProfile()
	probe := autoprobe.Result{
		TCP443: autoprobe.ProbeStatus{OK: true},
	}
	d := Select(SelectorInput{Profile: prof, ASN: ASNBeelineRU, Probe: probe})
	if d.Port != 2053 {
		t.Errorf("expected port 2053 for Beeline, got %d", d.Port)
	}
	if d.Mode != uri.ModeYandexCDN {
		t.Errorf("expected yandex_cdn, got %q", d.Mode)
	}
}
