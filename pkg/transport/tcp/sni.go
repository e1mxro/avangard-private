package tcptransport

import (
	"encoding/binary"
	"errors"
	"io"
)

// peekClientHelloSNI reads exactly one TLS record from r (without consuming it
// from the caller's perspective), parses the embedded ClientHello, and returns
// the SNI server_name extension along with the raw record bytes so the caller
// can replay them downstream.
//
// This is a deliberately tiny, tolerant parser — we don't validate the full
// ClientHello, just walk to the server_name extension. Anything malformed
// returns ("", raw, nil) so the caller can decide what to do (forward to decoy).
func peekClientHelloSNI(r io.Reader) (sni string, raw []byte, err error) {
	// TLS record header: type(1) + version(2) + length(2)
	hdr := make([]byte, 5)
	if _, err = io.ReadFull(r, hdr); err != nil {
		return "", nil, err
	}
	if hdr[0] != 0x16 { // handshake
		return "", hdr, errors.New("tls: not a handshake record")
	}
	recLen := int(binary.BigEndian.Uint16(hdr[3:5]))
	if recLen < 4 || recLen > 16384 {
		return "", hdr, errors.New("tls: bad record length")
	}
	body := make([]byte, recLen)
	if _, err = io.ReadFull(r, body); err != nil {
		return "", append(hdr, body...), err
	}
	raw = append(hdr, body...)

	// Handshake header: type(1) + length(3)
	if len(body) < 4 || body[0] != 0x01 { // ClientHello
		return "", raw, errors.New("tls: not ClientHello")
	}
	hsLen := int(body[1])<<16 | int(body[2])<<8 | int(body[3])
	if hsLen+4 > len(body) {
		return "", raw, errors.New("tls: hs length oob")
	}
	hs := body[4 : 4+hsLen]

	// ClientHello layout:
	//   client_version (2)
	//   random         (32)
	//   session_id_len (1) + session_id
	//   cipher_suites_len (2) + cipher_suites
	//   compression_methods_len (1) + compression_methods
	//   extensions_len (2) + extensions
	if len(hs) < 2+32+1 {
		return "", raw, errors.New("tls: short hs")
	}
	p := 2 + 32
	sidLen := int(hs[p])
	p++
	if p+sidLen > len(hs) {
		return "", raw, errors.New("tls: bad session_id len")
	}
	p += sidLen
	if p+2 > len(hs) {
		return "", raw, errors.New("tls: bad cs len")
	}
	csLen := int(binary.BigEndian.Uint16(hs[p : p+2]))
	p += 2
	if p+csLen > len(hs) {
		return "", raw, errors.New("tls: bad cs")
	}
	p += csLen
	if p+1 > len(hs) {
		return "", raw, errors.New("tls: bad cm len")
	}
	cmLen := int(hs[p])
	p++
	if p+cmLen > len(hs) {
		return "", raw, errors.New("tls: bad cm")
	}
	p += cmLen
	if p+2 > len(hs) {
		// no extensions
		return "", raw, nil
	}
	extLen := int(binary.BigEndian.Uint16(hs[p : p+2]))
	p += 2
	if p+extLen > len(hs) {
		return "", raw, errors.New("tls: bad ext len")
	}
	exts := hs[p : p+extLen]

	// Walk extensions for server_name (type 0x0000).
	for i := 0; i+4 <= len(exts); {
		t := binary.BigEndian.Uint16(exts[i : i+2])
		l := int(binary.BigEndian.Uint16(exts[i+2 : i+4]))
		if i+4+l > len(exts) {
			return "", raw, errors.New("tls: bad ext")
		}
		if t == 0x0000 {
			data := exts[i+4 : i+4+l]
			// server_name_list_len (2) + entries
			if len(data) < 2 {
				return "", raw, nil
			}
			listLen := int(binary.BigEndian.Uint16(data[:2]))
			if 2+listLen > len(data) {
				return "", raw, nil
			}
			list := data[2 : 2+listLen]
			// type(1) + len(2) + name
			if len(list) < 5 || list[0] != 0x00 {
				return "", raw, nil
			}
			nameLen := int(binary.BigEndian.Uint16(list[1:3]))
			if 3+nameLen > len(list) {
				return "", raw, nil
			}
			return string(list[3 : 3+nameLen]), raw, nil
		}
		i += 4 + l
	}
	return "", raw, nil
}
