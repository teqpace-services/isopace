# Isopace

**Isopace** is a financial transaction framework for Go — ISO-8583 messaging and
payment switching, plus the runtime, transaction pipeline, coordination, and
security building blocks needed to run a switch.

It is an independent, clean-room implementation in the spirit of
[jPOS](https://jpos.org), redesigned around idiomatic Go: type-safe field access,
zero-copy decoding, a pluggable codec catalog, and goroutine-native concurrency.
The module is **stdlib-only** — no third-party dependency in the module graph.

> **Status: `v1.0.0-rc.1` — first release candidate.** An **interim,
> feedback-seeking** tag under a "may still change" banner — not the v1.0.0
> contract yet (see the [roadmap to v1](ROADMAP-to-v1.md)). It narrows what v1
> will freeze: the **core** — `iso8583`, `packager`, `fieldcodec`,
> `lengthcodec`, `render` — is a stability candidate and the surface we want
> feedback on; the **higher layers** (`runtime`, `flow`, `space`, `store`,
> transport, `gateway`, `teq`, `vault`, `rbac`, `ops`) remain **experimental**
> until they soak. The software `Vault` backend is for development/testing —
> **production PIN and key handling require a certified HSM, which is not yet
> shipped.** Still stdlib-only; tested under `-race`.

A product of **[Teqpace Services Ltd.](https://teqpace.com)**

📖 **Documentation:** <https://teqpace-services.github.io/isopace/> — getting
started, the full architecture, a package tour, and the changelog. The per-package
API reference is on
[pkg.go.dev](https://pkg.go.dev/github.com/teqpace-services/isopace).

---

## Packages

| Layer | Package | Purpose |
|-------|---------|---------|
| Messaging | `iso8583` | Immutable, zero-copy `Message`; `Codec`; `Schema`; typed `Get[T]` and struct binding; exhaustive validation |
| Codecs | `fieldcodec`, `lengthcodec` | Orthogonal value × length codecs (ASCII/EBCDIC/BCD/binary, BER-TLV) resolvable by name |
| Profiles | `packager` | ISO 8583:1987 A/B/C, 1993 A/B, Visa & Mastercard overlays, declarative JSON loader |
| Rendering | `render/jsonio`, `render/protobuf`, `render/iso20022` | One message, many formats — from a read-only `View` |
| Transport | `link`, `listener`, `mux` | Framed TCP (+TLS), graceful server, connection pool, request/response switch |
| Runtime | `runtime` | Component host & lifecycle, deploy descriptors + hot redeploy, config, `slog`, observability facade |
| Processing | `flow` | Two-phase `Flow`/`Stage`/`Exchange` pipeline: routing, journaling, retry, idempotency, profiling |
| Coordination | `space` | Keyed tuple space + durable, crash-safe store-and-forward |
| Security | `vault` | PIN blocks, MAC/CMAC, DUKPT, TR-31 key blocks, EMV ARQC/ARPC behind a `Vault` façade |
| Enterprise/Ops | `rbac`, `store`, `ops` | RBAC + PBKDF2 auth, persistence, health/metrics/admin API, cluster membership |

The design and layering rules are in [`ARCHITECTURE.md`](ARCHITECTURE.md); status
per phase is tracked in [`IMPLEMENTATION_PLAN.md`](IMPLEMENTATION_PLAN.md).

## Quickstart

```sh
go test ./...

# An issuer that answers authorizations, and an acquirer that sends them:
go run ./examples/issuer   -addr 127.0.0.1:8583 -limit 10000   # terminal 1
go run ./examples/acquirer -addr 127.0.0.1:8583 -n 5 -amount 2500  # terminal 2
```

See [`docs/getting-started.md`](docs/getting-started.md) for the core API and the
`simulator` test host (which exposes `/healthz`, `/readyz`, and `/metrics`).

## Why not just use jPOS?

jPOS is excellent and battle-tested, but it is Java/AGPL and carries 25 years of
design history. Isopace is a **from-scratch Go implementation** that keeps what
makes jPOS great while improving on its data model — compile-time type safety,
zero-copy parsing, an always-derived bitmap, first-class validation, and
multi-format rendering — and operating naturally in cloud-native, high-throughput
Go services. It is **not** a translation or port of jPOS: it is built clean-room
from the ISO-8583 standard and public payments knowledge, with no jPOS source
consulted (see [`CONTRIBUTING.md`](CONTRIBUTING.md)).

## Licensing

Isopace is **dual-licensed**:

- **Open source:** [GNU AGPL v3.0 or later](LICENSE). If you run a modified
  Isopace as a network service, AGPL section 13 requires you to offer your users
  the corresponding source.
- **Commercial:** A proprietary license from Teqpace Services Ltd. removes the
  AGPL copyleft obligations for closed-source or hosted/commercial deployments.
  See [COMMERCIAL-LICENSE.md](COMMERCIAL-LICENSE.md).

Keeping the module stdlib-only is deliberate: it keeps this dual-license model
free of transitive copyleft.

## Contributing

Contributions are welcome. External contributors must sign the
[Contributor License Agreement](CLA.md) so Teqpace can maintain the dual-license
model. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

Please report vulnerabilities responsibly — see [SECURITY.md](SECURITY.md).
