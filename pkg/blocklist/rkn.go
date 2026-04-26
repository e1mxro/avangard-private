// Package blocklist parses Roskomnadzor (РКН) "Единый реестр" XML dumps and
// stores blocked-host data in compact in-memory bloom filters for O(1) lookup
// during route selection.
//
// The official dump format is a list of <content/> records. Each record can
// contain multiple <decision/> entries that each list <domain/> and <ip/>
// elements. AVANGARD currently uses the union of all domains and IPv4
// addresses, deduplicated.
//
// To stay license-clean, this package never embeds the dump itself — only
// parses an externally-provided io.Reader.
package blocklist

import (
	"encoding/xml"
	"errors"
	"hash/fnv"
	"io"
	"strings"
	"sync"
	"time"
)

// Set is an immutable, thread-safe set of blocked hosts (domain or IP literal).
type Set struct {
	mu       sync.RWMutex
	bits     []uint64
	m        uint64
	k        int
	numItems int
	loadedAt time.Time
}

// NewSet allocates a bloom filter with m bits and k hashes. m is rounded up
// to a multiple of 64.
func NewSet(m uint64, k int) *Set {
	if m%64 != 0 {
		m += 64 - (m % 64)
	}
	return &Set{bits: make([]uint64, m/64), m: m, k: k}
}

// LoadXML parses the official register format from r and inserts every domain
// and IP into the set.
//
// The schema is intentionally permissive — we only extract <domain> and <ip>
// inside any nested element.
func (s *Set) LoadXML(r io.Reader) error {
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch strings.ToLower(se.Name.Local) {
		case "domain", "ip", "ipv6", "url":
			var v string
			if err := dec.DecodeElement(&v, &se); err != nil {
				return err
			}
			v = normalize(strings.TrimSpace(v))
			if v != "" {
				s.add(v)
			}
		}
	}
	s.mu.Lock()
	s.loadedAt = time.Now()
	s.mu.Unlock()
	return nil
}

// LoadDomains adds an iterable of plain hostnames to the set (one per line);
// useful for dev/test fixtures.
func (s *Set) LoadDomains(r io.Reader) error {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	for _, line := range strings.Split(string(buf), "\n") {
		v := normalize(strings.TrimSpace(line))
		if v != "" && !strings.HasPrefix(v, "#") {
			s.add(v)
		}
	}
	s.mu.Lock()
	s.loadedAt = time.Now()
	s.mu.Unlock()
	return nil
}

// Contains returns true if host is (probably) in the blocklist. False
// positives can occur (bloom filter); false negatives never do.
//
// host is normalized (lowercased, trailing dot stripped) before the lookup,
// so "Yandex.RU" and "yandex.ru." both match "yandex.ru".
func (s *Set) Contains(host string) bool {
	v := normalize(host)
	if v == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := 0; i < s.k; i++ {
		idx := s.idx(v, i)
		if s.bits[idx>>6]&(1<<(idx&63)) == 0 {
			return false
		}
	}
	return true
}

// Len returns the number of items added (not the bloom filter capacity).
func (s *Set) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.numItems
}

// LoadedAt returns the time of the last successful Load*.
func (s *Set) LoadedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadedAt
}

func (s *Set) add(v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := 0; i < s.k; i++ {
		idx := s.idx(v, i)
		s.bits[idx>>6] |= 1 << (idx & 63)
	}
	s.numItems++
}

func (s *Set) idx(v string, i int) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte{byte(i)})
	_, _ = io.WriteString(h, v)
	return h.Sum64() % s.m
}

func normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSuffix(s, ".")
	// Strip "http://" or "https://" prefix if present (URL records).
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	// Strip path/query.
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	// Strip port.
	if i := strings.Index(s, ":"); i >= 0 {
		s = s[:i]
	}
	return s
}
