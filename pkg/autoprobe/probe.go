// Package autoprobe runs the AutoProbe Engine described in spec §6.2.1.
//
// At first connect (and every cache TTL afterwards) it concurrently checks
// what kind of network the client is on:
//   - Is UDP/443 to the AVANGARD server reachable?
//   - Is TCP/443 reachable? Are alternative ports?
//   - What's the RTT to known reference points?
//
// The result feeds the RegionShield ModeSelector.
package autoprobe

import (
	"context"
	"net"
	"sync"
	"time"
)

// Result is the probe outcome.
type Result struct {
	UDP443 ProbeStatus
	TCP443 ProbeStatus
	TCPAlt map[int]ProbeStatus // {8443: ok, 2053: timeout, ...}
	RTTs   map[string]time.Duration

	StartedAt  time.Time
	FinishedAt time.Time
}

// ProbeStatus indicates the outcome of an individual probe.
type ProbeStatus struct {
	OK     bool
	RTT    time.Duration
	ErrMsg string
}

// Config controls which probes run.
type Config struct {
	ServerHost string
	// AltTCPPorts to test in addition to 443. e.g. {8443, 2053, 2087, 2096}.
	AltTCPPorts []int
	// RTTRefs is a list of "host:port" baselines to ping (TCP-connect).
	RTTRefs []string
	// Timeout per individual probe.
	Timeout time.Duration
}

// DefaultConfig returns sensible defaults for the RU profile.
func DefaultConfig(serverHost string) Config {
	return Config{
		ServerHost:  serverHost,
		AltTCPPorts: []int{8443, 2053, 2087, 2096},
		RTTRefs:     []string{"1.1.1.1:443", "ya.ru:443"},
		Timeout:     800 * time.Millisecond,
	}
}

// Run executes all probes in parallel and returns when the slowest finishes
// (bounded by ctx).
func Run(ctx context.Context, cfg Config) Result {
	if cfg.Timeout == 0 {
		cfg.Timeout = 800 * time.Millisecond
	}
	r := Result{
		TCPAlt:    make(map[int]ProbeStatus, len(cfg.AltTCPPorts)),
		RTTs:      make(map[string]time.Duration, len(cfg.RTTRefs)),
		StartedAt: time.Now(),
	}
	var mu sync.Mutex
	var wg sync.WaitGroup

	probe := func(label, network, addr string, store func(ProbeStatus)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st := tcpUDPProbe(ctx, network, addr, cfg.Timeout)
			mu.Lock()
			store(st)
			mu.Unlock()
			_ = label
		}()
	}

	probe("udp443", "udp", net.JoinHostPort(cfg.ServerHost, "443"), func(st ProbeStatus) { r.UDP443 = st })
	probe("tcp443", "tcp", net.JoinHostPort(cfg.ServerHost, "443"), func(st ProbeStatus) { r.TCP443 = st })
	for _, p := range cfg.AltTCPPorts {
		p := p
		probe("tcpalt", "tcp", net.JoinHostPort(cfg.ServerHost, itoa(p)), func(st ProbeStatus) {
			r.TCPAlt[p] = st
		})
	}
	for _, ref := range cfg.RTTRefs {
		ref := ref
		probe("rtt:"+ref, "tcp", ref, func(st ProbeStatus) {
			r.RTTs[ref] = st.RTT
		})
	}
	wg.Wait()
	r.FinishedAt = time.Now()
	return r
}

func tcpUDPProbe(ctx context.Context, network, addr string, timeout time.Duration) ProbeStatus {
	start := time.Now()
	d := &net.Dialer{Timeout: timeout}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c, err := d.DialContext(cctx, network, addr)
	if err != nil {
		return ProbeStatus{OK: false, RTT: time.Since(start), ErrMsg: err.Error()}
	}
	_ = c.Close()
	return ProbeStatus{OK: true, RTT: time.Since(start)}
}

func itoa(i int) string {
	// minimal allocator-light int-to-string for non-negative i.
	if i == 0 {
		return "0"
	}
	var buf [10]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[n:])
}
