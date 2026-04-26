// Command avangard-client connects to an AVANGARD server.
//
// Subcommands:
//   - probe   — run the AutoProbe Engine against a target and print the result.
//   - tunnel  — open a one-shot raw TCP tunnel (for smoke-testing); copies
//     stdin → tunnel → stdout.
//   - socks5  — start a local SOCKS5 listener that pipes through AVANGARD.
//   - region  — print the RegionShield Mode chosen given a probe result.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/avangard/avangard/pkg/autoprobe"
	"github.com/avangard/avangard/pkg/regionshield"
	"github.com/avangard/avangard/pkg/transport"
	quictransport "github.com/avangard/avangard/pkg/transport/quic"
	tcptransport "github.com/avangard/avangard/pkg/transport/tcp"
	"github.com/avangard/avangard/pkg/tunnel"
	"github.com/avangard/avangard/pkg/uri"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "avangard-client",
		Short: "AVANGARD tunnel client",
	}
	root.AddCommand(newProbeCmd(), newTunnelCmd(), newSocksCmd(), newRegionCmd())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newProbeCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "probe <host>",
		Short: "Run the AutoProbe Engine",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			host := args[0]
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			r := autoprobe.Run(ctx, autoprobe.DefaultConfig(host))
			if asJSON {
				b, _ := json.MarshalIndent(r, "", "  ")
				fmt.Println(string(b))
				return nil
			}
			fmt.Printf("AutoProbe(%s)  %s elapsed\n", host, r.FinishedAt.Sub(r.StartedAt))
			fmt.Printf("  UDP/443    OK=%v rtt=%s\n", r.UDP443.OK, r.UDP443.RTT)
			fmt.Printf("  TCP/443    OK=%v rtt=%s\n", r.TCP443.OK, r.TCP443.RTT)
			for p, st := range r.TCPAlt {
				fmt.Printf("  TCP/%-5d OK=%v rtt=%s\n", p, st.OK, st.RTT)
			}
			for ref, rtt := range r.RTTs {
				fmt.Printf("  rtt %-15s %s\n", ref, rtt)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "JSON output")
	return cmd
}

func newRegionCmd() *cobra.Command {
	var region string
	var asn uint32
	cmd := &cobra.Command{
		Use:   "region <host>",
		Short: "Probe + show RegionShield decision",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			r := autoprobe.Run(ctx, autoprobe.DefaultConfig(args[0]))
			var prof regionshield.Profile
			switch region {
			case "RU":
				prof = regionshield.DefaultRUProfile()
			case "IR":
				prof = regionshield.DefaultIRProfile()
			case "CN":
				prof = regionshield.DefaultCNProfile()
			default:
				return fmt.Errorf("unknown region %q (RU|IR|CN)", region)
			}
			d := regionshield.Select(regionshield.SelectorInput{
				Profile: prof, ASN: asn, Probe: r,
			})
			fmt.Printf("region=%s asn=%d -> mode=%s port=%d (reason=%s)\n",
				region, asn, d.Mode, d.Port, d.Reason)
			return nil
		},
	}
	cmd.Flags().StringVar(&region, "region", "RU", "region")
	cmd.Flags().Uint32Var(&asn, "asn", 0, "AS number for ISP override")
	return cmd
}

func newTunnelCmd() *cobra.Command {
	var transportName string
	var insecure bool
	cmd := &cobra.Command{
		Use:   "tunnel <uri> <dest host:port>",
		Short: "Open a one-shot raw TCP tunnel; pipes stdin/stdout",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			u, err := uri.Parse(args[0])
			if err != nil {
				return err
			}
			ctx := context.Background()
			ts, err := dial(ctx, u, transportName, insecure)
			if err != nil {
				return err
			}
			tc := tunnel.NewClient(ts, u.UUID)
			conn, err := tc.DialTunnel(ctx, args[1])
			if err != nil {
				return err
			}
			defer conn.Close()
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); _, _ = io.Copy(conn, os.Stdin) }()
			go func() { defer wg.Done(); _, _ = io.Copy(os.Stdout, conn) }()
			wg.Wait()
			return nil
		},
	}
	cmd.Flags().StringVar(&transportName, "transport", "tcp", "tcp|quic")
	cmd.Flags().BoolVar(&insecure, "insecure", false, "skip TLS verify (dev)")
	return cmd
}

func newSocksCmd() *cobra.Command {
	var addr, transportName string
	var insecure bool
	cmd := &cobra.Command{
		Use:   "socks5 <uri>",
		Short: "Start a local SOCKS5 server tunnelling through AVANGARD",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u, err := uri.Parse(args[0])
			if err != nil {
				return err
			}
			ctx := context.Background()
			d, err := buildDialer(ctx, u, transportName, insecure)
			if err != nil {
				return err
			}
			tc := tunnel.NewClient(d, u.UUID)
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}
			logger := slog.Default()
			logger.Info("socks5 listening", "addr", ln.Addr())
			for {
				c, err := ln.Accept()
				if err != nil {
					return err
				}
				go handleSOCKS(ctx, tc, c, logger)
			}
		},
	}
	cmd.Flags().StringVar(&addr, "listen", "127.0.0.1:1080", "local SOCKS5 address")
	cmd.Flags().StringVar(&transportName, "transport", "tcp", "tcp|quic")
	cmd.Flags().BoolVar(&insecure, "insecure", false, "skip TLS verify (dev)")
	return cmd
}

// buildDialer returns a transport.Dialer for the chosen mode.
func buildDialer(ctx context.Context, u *uri.Config, name string, insecure bool) (transport.Dialer, error) {
	cfg := transport.ClientConfig{
		Addr:               net.JoinHostPort(u.Host, strconv.Itoa(u.Port)),
		UUID:               u.UUID,
		SNI:                u.SNI,
		ServerStaticPub:    u.ServerPub,
		InsecureSkipVerify: insecure,
	}
	switch name {
	case "tcp":
		return tcptransport.NewClient(cfg)
	case "quic":
		return quictransport.NewClient(ctx, cfg)
	default:
		return nil, fmt.Errorf("unknown transport %q", name)
	}
}

func dial(ctx context.Context, u *uri.Config, name string, insecure bool) (transport.Dialer, error) {
	return buildDialer(ctx, u, name, insecure)
}

// SOCKS5 (RFC 1928) — minimal implementation, no auth, CONNECT only.
func handleSOCKS(ctx context.Context, tc *tunnel.Client, c net.Conn, logger *slog.Logger) {
	defer c.Close()

	buf := make([]byte, 262)
	if _, err := io.ReadFull(c, buf[:2]); err != nil {
		return
	}
	if buf[0] != 0x05 {
		return
	}
	nMethods := int(buf[1])
	if _, err := io.ReadFull(c, buf[:nMethods]); err != nil {
		return
	}
	if _, err := c.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// Request: VER CMD RSV ATYP DST.ADDR DST.PORT
	if _, err := io.ReadFull(c, buf[:4]); err != nil {
		return
	}
	if buf[1] != 0x01 { // CONNECT only
		_, _ = c.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	var host string
	switch buf[3] {
	case 0x01:
		if _, err := io.ReadFull(c, buf[:4]); err != nil {
			return
		}
		host = net.IP(buf[:4]).String()
	case 0x03:
		if _, err := io.ReadFull(c, buf[:1]); err != nil {
			return
		}
		n := int(buf[0])
		if _, err := io.ReadFull(c, buf[:n]); err != nil {
			return
		}
		host = string(buf[:n])
	case 0x04:
		if _, err := io.ReadFull(c, buf[:16]); err != nil {
			return
		}
		host = net.IP(buf[:16]).String()
	default:
		return
	}
	if _, err := io.ReadFull(c, buf[:2]); err != nil {
		return
	}
	port := int(buf[0])<<8 | int(buf[1])

	dest := net.JoinHostPort(host, strconv.Itoa(port))
	upstream, err := tc.DialTunnel(ctx, dest)
	if err != nil {
		_, _ = c.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		logger.Debug("socks dial fail", "dest", dest, "err", err)
		return
	}
	defer upstream.Close()

	// Reply: succeeded, BND.ADDR=0, BND.PORT=0
	_, _ = c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(upstream, c) }()
	go func() { defer wg.Done(); _, _ = io.Copy(c, upstream) }()
	wg.Wait()
}
