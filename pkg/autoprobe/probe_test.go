package autoprobe

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestRunUnreachable(t *testing.T) {
	// 198.18.0.1 is RFC2544 black-hole; should reliably time out.
	cfg := Config{
		ServerHost:  "198.18.0.1",
		AltTCPPorts: []int{8443},
		Timeout:     200 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r := Run(ctx, cfg)
	if r.TCP443.OK {
		t.Errorf("expected TCP/443 to be unreachable")
	}
	if _, ok := r.TCPAlt[8443]; !ok {
		t.Errorf("alt port not measured")
	}
}

func TestRunLocalLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	cfg := Config{
		ServerHost:  "127.0.0.1",
		AltTCPPorts: []int{port},
		Timeout:     500 * time.Millisecond,
	}
	r := Run(context.Background(), cfg)
	if !r.TCPAlt[port].OK {
		t.Errorf("expected port %d ok, got %+v", port, r.TCPAlt[port])
	}
	_ = strconv.Itoa // keep import
}
