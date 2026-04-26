// Package regionshield implements the RegionShield engine described in spec
// §6.2.4: profile-driven mode selection that adapts to the user's region and
// ISP based on AutoProbe results.
package regionshield

import (
	"github.com/avangard/avangard/pkg/autoprobe"
	"github.com/avangard/avangard/pkg/uri"
)

// Mode is a transport mimicry strategy. Re-exports uri.Mode for ergonomics.
type Mode = uri.Mode

// Profile is a regional bypass policy.
type Profile struct {
	Region        string             // e.g. "RU"
	FallbackChain []Mode             // ordered preference
	ISPOverrides  map[uint32]ISPRule // by AS number
	Relays        []string           // host:port relay nodes
	IPv6Priority  bool               // prefer AAAA on dual-stack
}

// ISPRule overrides a profile for a specific AS.
type ISPRule struct {
	PreferredMode Mode
	Port          int
	ExtraFrag     bool // SplitDPI fragmentation
	TTLManip      bool // TTL spoofing
	QUIC          *bool
}

// SelectorInput captures the inputs to ModeSelector.
type SelectorInput struct {
	Profile Profile
	ASN     uint32          // 0 if unknown
	Probe   autoprobe.Result
}

// Decision is what the engine decided to do.
type Decision struct {
	Mode   Mode
	Port   int
	Reason string
}

// Select picks a mode based on the configured profile, the ISP override (if
// any), and the AutoProbe result. The algorithm walks the fallback chain and
// returns the first viable entry, then layers ISP-specific overrides.
func Select(in SelectorInput) Decision {
	port := 443

	// 1. ISP-specific override (highest priority).
	if rule, ok := in.Profile.ISPOverrides[in.ASN]; ok && rule.PreferredMode != "" {
		p := port
		if rule.Port != 0 {
			p = rule.Port
		}
		return Decision{Mode: rule.PreferredMode, Port: p, Reason: "isp-override"}
	}

	// 2. Walk the fallback chain in order.
	for _, m := range in.Profile.FallbackChain {
		if isViable(m, in.Probe) {
			return Decision{Mode: m, Port: port, Reason: "chain"}
		}
	}

	// 3. Last-resort fallback.
	return Decision{Mode: uri.ModeDNSTunnel, Port: 443, Reason: "last-resort"}
}

func isViable(m Mode, r autoprobe.Result) bool {
	switch m {
	case uri.ModeQUICCamouflage:
		return r.UDP443.OK
	case uri.ModeYandexCDN, uri.ModeGosuslugiWS, uri.ModeSplitDPI:
		// All TCP/443-based modes.
		return r.TCP443.OK
	case uri.ModeDNSTunnel:
		return true // DoH can always be attempted.
	case uri.ModeAuto:
		// Auto means "let the engine pick"; treat as TCP-based.
		return r.TCP443.OK || r.UDP443.OK
	}
	return false
}
