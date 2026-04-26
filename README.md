# AVANGARD — MVP reference implementation

Hybrid tunnel protocol that fuses Hysteria2 (QUIC + BBR) with VLESS+Reality
(SNI camouflage) and adds a `RegionShield` profile system for adapting to
regional censors. This repository contains the **MVP reference
implementation in Go** that accompanies the design document
(`docs/AVANGARD-SPEC.md`).

> Status: **MVP**. Suitable for protocol experimentation, integration tests,
> and as a base for hardening. Not yet production-grade — see
> "What's *not* in this MVP" below.

## Layout

```
.
├── cmd/
│   ├── avangard-server/    # CLI: keygen / selfcert / run
│   └── avangard-client/    # CLI: probe / region / tunnel / socks5
├── pkg/
│   ├── protocol/           # 32-byte VLESS-style framing + tests
│   ├── auth/               # Noise NK wrapper + HMAC token derivation
│   ├── transport/
│   │   ├── quic/           # QUIC server/client (quic-go)
│   │   └── tcp/            # TCP/TLS server/client + SNI peek + decoy fwd
│   ├── tunnel/             # Glue: protocol + transport + auth (+ e2e tests)
│   ├── autoprobe/          # AutoProbe Engine (UDP/TCP/RTT in parallel)
│   ├── regionshield/       # Profile engine + RU/IR/CN profiles + selector
│   ├── uri/                # avangard:// URI parser
│   └── config/             # YAML loader + validator
└── docs/
    └── AVANGARD-SPEC.md    # full RFC-style spec
```

## Build & test

```
go build ./...
go test -race ./...
```

## Quick start (loopback dev)

```bash
# 1. Generate keys + self-signed cert + config
./avangard-server selfcert --out ./dev --host localhost
./avangard-server keygen --host 127.0.0.1 --port 5443 --decoy localhost --out ./dev

# 2. Run the server (foreground)
./avangard-server run --config ./dev/server.yaml &

# 3. From another shell, run an autoprobe
./avangard-client probe 127.0.0.1

# 4. Open a one-shot raw TCP tunnel via TCP transport
echo -n "GET / HTTP/1.0\r\n\r\n" \
    | ./avangard-client tunnel --transport tcp --insecure \
        "<URI from keygen>" example.com:80

# 5. Or run a local SOCKS5 proxy
./avangard-client socks5 --listen 127.0.0.1:1080 --insecure \
        "<URI from keygen>"
curl --socks5 127.0.0.1:1080 https://example.com
```

## Architecture in 30 seconds

```
Client                                              Server
------                                              ------

avangard-client probe / region                      avangard-server run
    |                                                   |
[autoprobe] -> [RegionShield/ModeSelector]              |
    |                                                   |
[tcp.Client | quic.Client]  --TLS 1.3-->  [tcp.Server | quic.Server]
                            (Reality SNI peek + decoy fwd on TCP)
    |                                                   |
            [Noise NK handshake] (e,es / e,ee)
    |                                                   |
            [AVANGARD header: ver|tokenHMAC|cmd|atyp|addr|port]
    |                                                   |
            [raw bytes <-> upstream TCP destination]
```

- **No double encryption.** QUIC/TLS handles transport encryption; Noise NK
  provides per-session authentication and PFS. The AVANGARD framing is
  plaintext on top of the encrypted transport.
- **Token never travels in clear.** What goes on the wire is
  `HMAC(uuid, channel_binding)` truncated to 16 bytes; the Noise transcript
  hash binds the token to the session.

## What's *not* in this MVP

The accompanying design doc (`docs/AVANGARD-SPEC.md`) fully covers these;
the MVP intentionally leaves them as well-defined extension points.

- **uTLS / JA4 fingerprinting** — uses standard `crypto/tls`, so the
  ClientHello looks like a Go program, not Yandex Browser. Production
  builds should swap in a `utls`-fork.
- **Hysteria2 GQUIC mimicry** — Initial Packets are stock IETF QUIC.
- **Reality session_id proof** — TCP-fallback peeks SNI and forwards
  non-magic SNIs to a decoy, but does not yet implement the full
  X25519-proof in `session_id`.
- **WebSocket (mode B), SplitDPI raw-socket (mode C), DNS tunnel (mode E)**
  — `regionshield` selects them, but the actual carriers are not wired up.
- **0-RTT bloom-filter** — design specified, not implemented yet.
- **Automatic Ed25519 7-day rotation** — keypair is static for the MVP.

## Tests

- `pkg/protocol`     — header round-trip (IPv4 / domain), error paths
- `pkg/uri`          — full / minimal URI parsing, error cases
- `pkg/auth`         — token determinism + Noise NK over an in-memory pipe
- `pkg/autoprobe`    — black-hole IP timeout + loopback success
- `pkg/regionshield` — selector decisions for happy path / fallback / overrides
- `pkg/tunnel`       — **end-to-end**: server + client + echo upstream
  over both QUIC and TCP transports

```
go test -race ./...
```

## License

This is a research / reference implementation. See `docs/AVANGARD-SPEC.md`
§7.9 (legal) before deploying anywhere.
