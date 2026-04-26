// Package replay implements 0-RTT replay protection for Noise NK as described
// in spec §1.4.6.
//
// Each accepted 0-RTT initial payload contains a (timestamp, nonce) pair. The
// server rejects the payload if either:
//
//  1. |client_ts − server_ts| exceeds the configured window (default ±5 s), OR
//  2. The (timestamp, nonce) tuple has been seen in the bloom filter within
//     the active sliding window.
//
// Implementation: a pair of bloom filters that rotate on a fixed cadence. At
// any given time, lookups consult both filters (current + previous), and
// inserts go into the current. When the cadence elapses, the previous filter
// is discarded and a new "current" is allocated.
//
// Defaults (matching spec): m = 2^28 bits ≈ 32 MiB per filter, k = 7 hashes,
// expected throughput up to ~10⁷ entries / window with FPR < 10⁻⁶.
package replay

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sync"
	"time"
)

// Default parameters from the spec.
const (
	DefaultBits       = 1 << 28           // 256 Mb = 32 MiB per filter
	DefaultHashes     = 7                 // optimal for ~5% load factor
	DefaultWindow     = 5 * time.Second   // ±5 s clock skew
	DefaultRotateRate = 60 * time.Second  // rotate filters every 60 s
	DefaultClockSkew  = 5 * time.Second   // accepted client clock drift
)

// ErrReplay is returned by Check if a 0-RTT payload is replayed.
var ErrReplay = errors.New("replay: payload already seen")

// ErrSkew is returned by Check if the timestamp is outside the allowed window.
var ErrSkew = errors.New("replay: timestamp outside allowed skew")

// Filter is a sliding-window double-buffered bloom filter.
type Filter struct {
	bits       uint64
	hashes     int
	skew       time.Duration
	rotateRate time.Duration

	mu      sync.RWMutex
	now     func() time.Time
	curr    *bloom
	prev    *bloom
	rotateAt time.Time
}

// Options configures a Filter; zero values use the defaults above.
type Options struct {
	Bits       uint64
	Hashes     int
	ClockSkew  time.Duration
	RotateRate time.Duration
	Now        func() time.Time
}

// New constructs a Filter.
func New(opts Options) *Filter {
	if opts.Bits == 0 {
		opts.Bits = DefaultBits
	}
	// round up to multiple of 64 for word-aligned storage
	if opts.Bits%64 != 0 {
		opts.Bits += 64 - (opts.Bits % 64)
	}
	if opts.Hashes == 0 {
		opts.Hashes = DefaultHashes
	}
	if opts.ClockSkew == 0 {
		opts.ClockSkew = DefaultClockSkew
	}
	if opts.RotateRate == 0 {
		opts.RotateRate = DefaultRotateRate
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	now := opts.Now()
	return &Filter{
		bits:       opts.Bits,
		hashes:     opts.Hashes,
		skew:       opts.ClockSkew,
		rotateRate: opts.RotateRate,
		now:        opts.Now,
		curr:       newBloom(opts.Bits),
		prev:       newBloom(opts.Bits),
		rotateAt:   now.Add(opts.RotateRate),
	}
}

// Check examines a (clientTimestamp, nonce) tuple. It returns nil if the
// payload is fresh (and records it for future rejection) or an error if it's
// outside the skew window or already seen.
func (f *Filter) Check(clientTS time.Time, nonce []byte) error {
	now := f.now()
	if abs(now.Sub(clientTS)) > f.skew {
		return ErrSkew
	}
	f.maybeRotate(now)

	key := digest(clientTS, nonce)

	f.mu.RLock()
	hit := f.curr.contains(key, f.hashes) || f.prev.contains(key, f.hashes)
	f.mu.RUnlock()
	if hit {
		return ErrReplay
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	// Re-check under the write lock to close the race window.
	if f.curr.contains(key, f.hashes) || f.prev.contains(key, f.hashes) {
		return ErrReplay
	}
	f.curr.add(key, f.hashes)
	return nil
}

// maybeRotate swaps prev<-curr and clears curr if it's time.
func (f *Filter) maybeRotate(now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !now.Before(f.rotateAt) {
		f.prev = f.curr
		f.curr = newBloom(f.bits)
		f.rotateAt = now.Add(f.rotateRate)
	}
}

// digest mixes (timestamp, nonce) into a 32-byte key.
func digest(ts time.Time, nonce []byte) [sha256.Size]byte {
	var tsbuf [8]byte
	binary.BigEndian.PutUint64(tsbuf[:], uint64(ts.UnixNano()))
	h := sha256.New()
	h.Write(tsbuf[:])
	h.Write(nonce)
	var out [sha256.Size]byte
	h.Sum(out[:0])
	return out
}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// bloom is a simple bit-array bloom filter using two SHA-256 halves as the
// hash basis (Kirsch–Mitzenmacher double hashing).
type bloom struct {
	m    uint64
	bits []uint64 // packed 64-bit words
}

func newBloom(m uint64) *bloom {
	return &bloom{m: m, bits: make([]uint64, m/64)}
}

func (b *bloom) add(key [sha256.Size]byte, k int) {
	for i := 0; i < k; i++ {
		idx := bloomIdx(key, i, b.m)
		b.bits[idx>>6] |= 1 << (idx & 63)
	}
}

func (b *bloom) contains(key [sha256.Size]byte, k int) bool {
	for i := 0; i < k; i++ {
		idx := bloomIdx(key, i, b.m)
		if b.bits[idx>>6]&(1<<(idx&63)) == 0 {
			return false
		}
	}
	return true
}

// bloomIdx returns the i'th hash position from a 32-byte digest by treating
// the first 8 bytes as h1 and bytes [8..16) as h2 and computing h1 + i·h2.
func bloomIdx(key [sha256.Size]byte, i int, m uint64) uint64 {
	h1 := binary.BigEndian.Uint64(key[0:8])
	h2 := binary.BigEndian.Uint64(key[8:16])
	return (h1 + uint64(i)*h2) % m
}
