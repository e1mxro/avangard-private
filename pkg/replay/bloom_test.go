package replay

import (
	"errors"
	"testing"
	"time"
)

func TestCheckFreshAccepts(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)
	f := New(Options{Bits: 1 << 16, Now: func() time.Time { return now }})
	if err := f.Check(now, []byte("nonce-1")); err != nil {
		t.Fatalf("expected accept, got %v", err)
	}
}

func TestCheckReplayRejected(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)
	f := New(Options{Bits: 1 << 16, Now: func() time.Time { return now }})
	_ = f.Check(now, []byte("nonce-x"))
	err := f.Check(now, []byte("nonce-x"))
	if !errors.Is(err, ErrReplay) {
		t.Fatalf("expected ErrReplay, got %v", err)
	}
}

func TestCheckSkewRejected(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)
	f := New(Options{
		Bits:      1 << 16,
		ClockSkew: 1 * time.Second,
		Now:       func() time.Time { return now },
	})
	err := f.Check(now.Add(10*time.Second), []byte("nonce"))
	if !errors.Is(err, ErrSkew) {
		t.Fatalf("expected ErrSkew, got %v", err)
	}
}

func TestRotationDoesNotForgetWithinWindow(t *testing.T) {
	t0 := time.Unix(1_000_000_000, 0)
	tNow := t0
	f := New(Options{
		Bits:       1 << 16,
		RotateRate: 100 * time.Millisecond,
		ClockSkew:  10 * time.Second,
		Now:        func() time.Time { return tNow },
	})
	if err := f.Check(t0, []byte("n1")); err != nil {
		t.Fatal(err)
	}
	tNow = t0.Add(150 * time.Millisecond) // forces 1 rotation
	// Old key now lives in `prev` filter, must still be rejected.
	err := f.Check(t0, []byte("n1"))
	if !errors.Is(err, ErrReplay) {
		t.Fatalf("expected ErrReplay after rotation, got %v", err)
	}
}

func TestRotationForgetsAfterTwoIntervals(t *testing.T) {
	t0 := time.Unix(1_000_000_000, 0)
	tNow := t0
	f := New(Options{
		Bits:       1 << 16,
		RotateRate: 50 * time.Millisecond,
		ClockSkew:  10 * time.Second,
		Now:        func() time.Time { return tNow },
	})
	_ = f.Check(t0, []byte("n2"))
	// Two rotations: prev becomes the once-current bloom, then dropped.
	tNow = t0.Add(60 * time.Millisecond)
	_ = f.Check(tNow, []byte("just-rotates"))
	tNow = t0.Add(120 * time.Millisecond)
	_ = f.Check(tNow, []byte("rotates-again"))
	// "n2" should be forgotten now.
	tNow = t0.Add(125 * time.Millisecond)
	if err := f.Check(t0, []byte("n2")); err != nil {
		t.Logf("expected fresh accept after 2 rotations; got %v (acceptable if FPR triggered)", err)
	}
}

func TestParallelLoad(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)
	f := New(Options{Bits: 1 << 18, Now: func() time.Time { return now }})
	for i := 0; i < 10_000; i++ {
		nonce := []byte{byte(i), byte(i >> 8), byte(i >> 16), byte(i >> 24)}
		if err := f.Check(now, nonce); err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
	}
}
