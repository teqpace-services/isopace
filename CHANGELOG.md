# Changelog

All notable changes to Isopace are recorded here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) as described in
[`docs/versioning.md`](docs/versioning.md).

## [Unreleased]

### Added

- **`vault.SealedVault`** — a hardened software `Vault`: working keys are
  encrypted at rest under a caller-supplied key-encryption key (AES-GCM),
  decrypted only transiently and zeroized after each operation (including the
  cleartext PIN block), with key check values, per-key usage enforcement, and an
  audit hook. It implements the same `Vault` interface as `SoftVault`. It raises
  the bar for software key storage and non-PIN crypto but is still **not** a
  certified HSM substitute for PCI PIN / P2PE.
- **`COMMERCIAL-AGREEMENT.md`** — a draft commercial-license template (pending
  legal-counsel review).
- **`.air.toml`** — live-reload config for the `simulator` example (dev tool;
  not a module dependency).

## [0.1.0] - 2026-06-01

First tagged release. A feature-complete, clean-room Go framework for ISO-8583
messaging and payment switching. The module is **stdlib-only** — no third-party
dependency in the module graph — and every package is `gofmt`/`go vet` clean and
tested under `go test -race`.

### Added

- **ISO-8583 core (`iso8583`)** — immutable, copy-on-write `Message`; zero-copy
  lazy `Codec` (`Unmarshal`/`Marshal`/`Validate`); `Schema`/`SchemaBuilder`;
  fixed-point `Decimal`/`Amount`; `FieldPath` grammar (incl. BER-TLV tags); the
  typed generic accessor `Get[T]` and struct-binding `Binder[T]`; exhaustive
  validation with structured errors.
- **Codec catalog (`fieldcodec`, `lengthcodec`)** — orthogonal value × length
  codecs: ASCII / EBCDIC (CP037/CP1047) / packed-BCD / binary, amount and bitmap
  codecs, and a BER-TLV composite for EMV field 55, all resolvable by name.
- **Packager profiles (`packager`)** — ISO 8583:1987 A/B/C, 1993 A/B, Visa and
  Mastercard overlays, and a declarative JSON loader (`go:embed`).
- **Alternate renderings (`render`)** — schema-aware JSON (PAN masking, TLV
  nesting), a descriptor-driven protobuf wire codec, and an ISO 20022 pacs.008
  bridge — all from one read-only `View`.
- **Conformance & QA (`conformance`)** — hand-derived golden wire vectors, fuzz
  targets, and allocation/throughput benchmarks (allocation-free clean Marshal).
- **Transport (`link`, `listener`, `mux`)** — framed TCP connection (pluggable
  framer, filters, TLS), a graceful-shutdown server, a connection pool with
  backoff, and a request/response switch correlating by a `Keyer`.
- **Runtime (`runtime`)** — component host with ordered start / reverse stop and
  partial-start unwind; declarative deploy descriptors with hot redeploy;
  JSON+env configuration with hot reload; `log/slog` setup; and an `Observer`
  traces/metrics facade (no-op and slog backends; OpenTelemetry as a drop-in).
- **Transaction manager (`flow`)** — two-phase `Flow`/`Stage`/`Exchange`
  pipeline with conditional routing between groups, journaling, idempotent
  replay, per-stage retry, and a profiler.
- **Coordination (`space`)** — a keyed tuple space (`Space`) with an in-process
  backend and a durable, crash-safe store-and-forward `Store`.
- **Security (`vault`)** — ISO 9564 PIN blocks, ISO 9797-1 MAC (incl. retail
  MAC) and CMAC, DUKPT (ANSI X9.24-1, validated against the public test vector),
  TR-31 version B key blocks, and EMV ARQC/ARPC, behind a `Vault` façade
  (`SoftVault` software backend; HSM/PKCS#11 as a drop-in adapter).
- **Enterprise / ops (`rbac`, `store`, `ops`)** — role-based access control with
  PBKDF2 credentials and constant-time authentication; a collection/key/value
  persistence `Store`; and an operational surface with health checks, a metrics
  registry (Prometheus text; implements `runtime.Observer`), cluster membership,
  and an rbac-guarded HTTP admin API.
- **Examples** — runnable `acquirer` and `issuer` programs, a `simulator` test
  host assembled from `runtime` + `ops` + transport, and the shared `posdemo`
  library with an end-to-end loopback test.

### Security notes

- `vault.SoftVault` is for development, testing, and conformance only. Production
  PIN and key handling must use a certified HSM behind the `Vault` interface.
- The admin API distinguishes 401 (unauthenticated) from 403 (forbidden) and
  authenticates in constant time; expose it only behind appropriate network
  controls.

[Unreleased]: https://github.com/teqpace-services/isopace/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/teqpace-services/isopace/releases/tag/v0.1.0
