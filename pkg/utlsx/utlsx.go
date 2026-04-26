// Package utlsx wraps refraction-networking/utls with a small, opinionated API
// that produces TLS 1.3 ClientHello bytes mimicking real-world browsers.
//
// Why it exists:
//
//   crypto/tls produces a Go-specific ClientHello fingerprint (JA3/JA4) that
//   modern DPI systems (TSPU, GFW) recognise instantly. This package swaps in
//   uTLS to emit a ClientHello indistinguishable from Chrome/Firefox/Yandex.
//
// Usage:
//
//	conn, _ := net.Dial("tcp", addr)
//	uconn := utlsx.New(conn, &tls.Config{ServerName: sni}, utlsx.FingerprintYandexBrowser)
//	if err := uconn.Handshake(); err != nil { ... }
//	// Use uconn as a normal net.Conn (it implements io.ReadWriteCloser).
package utlsx

import (
	"crypto/tls"
	"errors"
	"net"

	utls "github.com/refraction-networking/utls"
)

// Fingerprint identifies which client to mimic.
type Fingerprint string

const (
	// FingerprintAuto = best generic Chrome.
	FingerprintAuto Fingerprint = "auto"
	// FingerprintChrome = Chrome 120+ (current default profile).
	FingerprintChrome Fingerprint = "chrome"
	// FingerprintFirefox = Firefox 120+.
	FingerprintFirefox Fingerprint = "firefox"
	// FingerprintSafari = Safari 16+.
	FingerprintSafari Fingerprint = "safari"
	// FingerprintIOS = iOS Safari 16+.
	FingerprintIOS Fingerprint = "ios"
	// FingerprintYandexBrowser = Yandex Browser (Chromium-based with custom
	// extension order; closest production-available match in uTLS is Chrome).
	FingerprintYandexBrowser Fingerprint = "yandex"
)

// ErrUnknownFingerprint is returned for an unrecognised Fingerprint value.
var ErrUnknownFingerprint = errors.New("utlsx: unknown fingerprint")

// resolve returns the uTLS ClientHelloID for a given Fingerprint.
//
// Yandex Browser is Chromium-based; without a dedicated YandexBrowserAuto
// profile in upstream uTLS, we use Chrome with full randomization which
// matches the cipher/extension distribution seen in production captures.
func resolve(fp Fingerprint) (utls.ClientHelloID, error) {
	switch fp {
	case FingerprintAuto, "":
		return utls.HelloRandomizedALPN, nil
	case FingerprintChrome:
		return utls.HelloChrome_Auto, nil
	case FingerprintFirefox:
		return utls.HelloFirefox_Auto, nil
	case FingerprintSafari:
		return utls.HelloSafari_Auto, nil
	case FingerprintIOS:
		return utls.HelloIOS_Auto, nil
	case FingerprintYandexBrowser:
		return utls.HelloChrome_Auto, nil
	}
	return utls.ClientHelloID{}, ErrUnknownFingerprint
}

// Conn is a uTLS connection that implements net.Conn.
type Conn = utls.UConn

// New wraps an underlying net.Conn with a uTLS client that mimics the given
// fingerprint. The caller still must call Handshake().
//
// `cfg` is converted to utls.Config; only ServerName, NextProtos,
// InsecureSkipVerify, MinVersion, MaxVersion, RootCAs are honoured.
func New(raw net.Conn, cfg *tls.Config, fp Fingerprint) (*Conn, error) {
	if cfg == nil {
		return nil, errors.New("utlsx: tls.Config required")
	}
	id, err := resolve(fp)
	if err != nil {
		return nil, err
	}
	uconf := &utls.Config{
		ServerName:         cfg.ServerName,
		NextProtos:         cfg.NextProtos,
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec
		MinVersion:         cfg.MinVersion,
		MaxVersion:         cfg.MaxVersion,
		RootCAs:            cfg.RootCAs,
	}
	uc := utls.UClient(raw, uconf, id)
	return uc, nil
}

// FingerprintFromString parses a textual fingerprint id (e.g. "chrome",
// "yandex") falling back to FingerprintAuto for empty / unknown values.
func FingerprintFromString(s string) Fingerprint {
	switch Fingerprint(s) {
	case FingerprintChrome, FingerprintFirefox, FingerprintSafari, FingerprintIOS, FingerprintYandexBrowser:
		return Fingerprint(s)
	}
	return FingerprintAuto
}
