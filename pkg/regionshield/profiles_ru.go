package regionshield

import "github.com/avangard/avangard/pkg/uri"

// RU AS numbers for the major ISPs (spec §6.4.1).
const (
	ASNMTSRU       uint32 = 8359
	ASNBeelineRU   uint32 = 3216
	ASNMegafonRU   uint32 = 31163
	ASNTele2RU     uint32 = 12061
	ASNRostelRU    uint32 = 12389
	ASNERTelecomRU uint32 = 41733
	ASNYotaRU      uint32 = 31213
	ASNMGTSRU      uint32 = 25513
)

// DefaultRUProfile returns the canonical RU profile from the spec.
func DefaultRUProfile() Profile {
	return Profile{
		Region: "RU",
		FallbackChain: []Mode{
			uri.ModeYandexCDN,
			uri.ModeSplitDPI,
			uri.ModeGosuslugiWS,
			uri.ModeQUICCamouflage,
			uri.ModeDNSTunnel,
		},
		IPv6Priority: true,
		ISPOverrides: map[uint32]ISPRule{
			ASNMTSRU: {
				PreferredMode: uri.ModeSplitDPI,
				ExtraFrag:     true,
				TTLManip:      true,
			},
			ASNBeelineRU: {
				PreferredMode: uri.ModeYandexCDN,
				Port:          2053,
				QUIC:          ptrBool(false),
			},
			ASNMegafonRU: {
				PreferredMode: uri.ModeYandexCDN,
				QUIC:          ptrBool(false),
			},
			ASNRostelRU: {
				PreferredMode: uri.ModeGosuslugiWS,
				TTLManip:      true,
			},
			ASNERTelecomRU: {
				PreferredMode: uri.ModeYandexCDN,
			},
		},
	}
}

// DefaultIRProfile returns a minimal Iran profile.
func DefaultIRProfile() Profile {
	return Profile{
		Region: "IR",
		FallbackChain: []Mode{
			uri.ModeYandexCDN, // ArvanCloud-fronting fits the same shape
			uri.ModeGosuslugiWS,
			uri.ModeQUICCamouflage,
			uri.ModeDNSTunnel,
		},
	}
}

// DefaultCNProfile returns a minimal China profile.
func DefaultCNProfile() Profile {
	return Profile{
		Region: "CN",
		FallbackChain: []Mode{
			uri.ModeYandexCDN, // (Alibaba-front mode reuses the slot)
			uri.ModeGosuslugiWS,
		},
	}
}

func ptrBool(b bool) *bool { return &b }
