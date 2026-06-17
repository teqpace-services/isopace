# Changelog

All notable changes to Isopace are recorded here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) as described in
the [versioning policy](https://teqpace-services.github.io/isopace/versioning/).

## [Unreleased]

## [1.0.0] - 2026-06-17

The first **stable** release. With the release-candidate soak complete, the
**core** ISO 8583 surface is now **frozen under [Semantic Versioning](https://semver.org/spec/v2.0.0.html)**;
the higher layers and the production-crypto (HSM) path ship alongside it but
remain **preview** and are explicitly **not** covered by the v1 stability
guarantee:

- **Stable — frozen.** `iso8583`, `packager`, `fieldcodec`, `lengthcodec`, and
  `render` are the v1 API contract. Breaking changes to these now require a
  major-version bump. This surface soaked through `v1.0.0-rc.1` in production.
- **Experimental — may still change.** The higher layers (`runtime`, `flow`,
  `space`, `store`, `mux`, `link`, `listener`, `connector`, `gateway`, `teq`,
  `vault`, `rbac`, `ops`) ship but are **not** yet covered by the stability
  guarantee and may change in a minor release until they soak.
- **Preview — production crypto (HSM).** The HSM path (roadmap B1) ships for the
  first time: `vault` capability interfaces plus the `adapters/pkcs11` and
  `adapters/payshield` modules. The PKCS#11 MAC path is cross-checked against a
  real token under SoftHSM2 in CI; **`adapters/payshield` is a scaffold
  validated only against an in-repo simulator — not against real payShield
  hardware or LMK key schemes — and is pending device validation and security
  review.** Software vault backends remain development/testing only; production
  PIN and key handling require a certified HSM.

### Added

- **`vault` capability interfaces** for HSM adapters. The `vault.Vault` façade is
  now composed from `PINEncryptor`, `PINTranslator`, and `Macer`, so a hardware
  adapter can implement exactly the operations its device supports — a
  general-purpose PKCS#11 HSM provides `Macer` (and possibly `PINEncryptor`),
  while a payment HSM additionally provides `PINTranslator`. `Vault` keeps the
  same method set, so this is **source-compatible**. `PINTranslator` documents
  the PCI PIN Security contract: a conforming hardware implementation must
  re-encipher atomically inside the device so the clear PIN never leaves it, and
  an adapter that cannot do so (e.g. stock PKCS#11) must not implement it.
- **`adapters/pkcs11` — HSM-backed MAC via a PKCS#11 token.** The adapter now
  implements `vault.Macer` (`GenerateMAC` / `VerifyMAC`) for ISO 9797-1
  algorithm 1 (single-DES CBC-MAC) and algorithm 3 (ANSI X9.19 retail MAC),
  composed from the token's 3DES primitives so the key never leaves the device.
  The output is byte-for-byte identical to `vault.GenerateMAC`, **cross-checked
  against a real token under SoftHSM2 in CI**. Per the `PINTranslator` contract
  the type deliberately advertises **only** `Macer` — it does not implement PIN
  translate (no PCI-secure stock-PKCS#11 mechanism exists) or PIN-block encrypt
  (a clear-PIN, issuer-context operation). Separate cgo module; the core stays
  stdlib-only.
- **`adapters/payshield` — Thales payShield payment-HSM adapter (scaffold).** A
  reference adapter that exposes a payShield over its host-command protocol as the
  **full `vault.Vault`**: `vault.PINEncryptor` (issuer-context clear-PIN encrypt),
  `vault.PINTranslator` (PCI-secure PIN translate — the operation a general-purpose
  PKCS#11 HSM cannot do), and `vault.Macer` (ISO 9797-1 MAC). A payment HSM
  performs every one of these inside the device, so the adapter satisfies the whole
  façade. Keys are named by the device's LMK key token; the clear key never leaves
  the device. Ships with an in-repo `Simulator` — a protocol **test double** whose
  cryptography is the Isopace software vault — so the command flow (framing, command
  building, response/error-code parsing, capability surface) is exercised end to end
  in CI without hardware. **Scaffold:** the on-wire field layout is a simplified
  stand-in for payShield's positional fields and it has not been validated against
  real hardware/LMK schemes — pending device validation and security review (B1).
  Separate, stdlib-only module.

## [1.0.0-rc.1] - 2026-06-09

First release candidate for `v1.0.0`. This is an **interim, feedback-seeking
tag under a "may still change" banner** — not the v1.0.0 contract itself. Per
the [roadmap to v1](https://github.com/teqpace-services/isopace/blob/main/ROADMAP-to-v1.md),
it deliberately **narrows** what v1 will
freeze and is published to gather integration feedback before the API freeze:

- **Stable candidate — the core.** `iso8583`, `packager`, `fieldcodec`,
  `lengthcodec`, and `render` are proposed for the v1 API freeze. Feedback is
  solicited against these surfaces specifically.
- **Experimental — may still change.** The higher layers (`runtime`, `flow`,
  `space`, `store`, `mux`, `link`, `listener`, `connector`, `gateway`, `teq`,
  `vault`, `rbac`, `ops`) remain explicitly experimental and are **not** covered
  by the rc's stability intent until they have soaked.
- **Production crypto (HSM) is not yet shipped.** The only `vault` backends are
  software (`SoftVault`, `SealedVault`); the certified-HSM / PKCS#11 path
  (roadmap B1) is outstanding. Do not treat the crypto path as production-ready.

No code change since `0.3.0` in the root module; the OpenTelemetry and SQL
adapters ship as separate modules (`adapters/otel`, `adapters/sql`) so the core
dependency graph stays empty. Still **stdlib-only**; every package is
`gofmt`/`go vet` clean and tested under `go test -race`.

### Added

- **`adapters/otel/` and `adapters/sql/`** — the first production integrations as
  standalone modules (each its own `go.mod`): a `runtime.Observer` over the
  OpenTelemetry SDK, and a `store.Store` over `database/sql` (driver chosen by the
  integrator, so none enters the core graph). The root module remains
  dependency-free.

### Changed

- **Docs:** corrected the `CoralPay` / `Zone` profile descriptions to state their
  actual provenance — clean-room layouts composed from public ISO 8583:1987 field
  semantics and the acquirers' published field tables — replacing earlier wording
  that implied derivation from a jPOS packager definition. No code change.

## [0.3.0] - 2026-06-02

The acquirer-profiles release: two more ISO 8583:1987 switch profiles —
`CoralPay` and `Zone` — each a clean-room layout composed from public
ISO 8583:1987 field semantics and the acquirer's published field tables, plus
the two wire primitives those links need: a
prefix-keyed SHA-256 message MAC and a fixed-width tag-length-value codec. Still
**stdlib-only**; every package is `gofmt`/`go vet` clean and tested under
`go test -race`.

### Added

- **`packager.CoralPay()` — the CoralPay acquirer profile.** An ISO 8583:1987
  layout for the CoralPay acquirer link: ASCII MTI, an **ASCII-hex**
  primary+secondary bitmap, hex-on-wire (ASCII-hex) DE 52 PIN data, ASCII
  message-hash fields
  DE 64 / DE 128, and the DE 127 reserved-private subfield group carried under a
  6-digit length prefix over a 1-level sub-bitmap. Registered in `Profiles()` as
  `"coralpay"`. Round-trip tested (MTI, ASCII fields, 8-byte DE 52). A clean-room
  composition from public ISO 8583:1987 field semantics.
- **`packager.Zone()` — the Zone acquirer profile.** An ISO 8583:1987 layout
  for the Zone acquirer link: numeric-ASCII MTI, a **binary** primary+secondary
  bitmap, raw-binary DE 52 PIN data, and the same DE 127 reserved-private
  subfield group under a 6-digit length prefix. Registered in `Profiles()` as
  `"zone"`. This reintroduces a `Zone()` constructor and the `"zone"` profile id
  — distinct from the representative "site" layout removed in 0.2.0 — a
  clean-room composition from public ISO 8583:1987 field semantics. Round-trip
  tested.
- **`vault.SHA256MAC` / `vault.VerifySHA256MAC` — prefix-keyed SHA-256 message
  MAC.** Computes `SHA-256(key ‖ data)`, the message-hash MAC used by acquirer
  links such as CoralPay (DE 128 message hash, and the parameter-download
  DE 64) — **not HMAC**, a plain prefix-keyed digest. `VerifySHA256MAC`
  constant-time compares the leftmost `len(mac)` bytes. Callers hex-encode the
  32-byte digest to the 64-character on-wire field.
- **`tlv` — a fixed-width tag-length-value codec.** A new package with `Decode`
  / `Encode` / `Get` for streams of (`TagWidth`-character tag +
  `LenWidth`-decimal-digit length + value) elements — for example CoralPay's
  terminal-parameter DE 62 (`"03"+"015"+<15-char MID>`, `"05"+"003"+<currency>`,
  …). Distinct from the BER-TLV composite in `fieldcodec`, which uses encoded
  tag/length octets.

## [0.2.0] - 2026-06-02

The switching-layer release: a jPOS Q2-style container (`teq`) that keeps switch
connections open, routes and transforms transactions, and traces each one — plus
XML rendering and a built-in message describer. Still **stdlib-only**; every
package is `gofmt`/`go vet` clean and tested under `go test -race`.

### Added

- **`teq` + `cmd/teq` — the container (jPOS Q2 analog), assembled.** Wraps
  `runtime.Host` with first-class switch connections (`Q.Switch` / `Q.To`), a
  name registrar (`Q.Get` / `Q.Put` / `Q.Names`), and a shared tuple space
  (`Q.Space`). Start from code (`Start` / `Run` / `ListenAndServe`) or run
  `cmd/teq` as a daemon that boots from a directory of JSON component descriptors
  and hot-(re)deploys them. Switch and routing-gateway descriptors are fully
  declarative — packager, framer, TLS, sign-on, echo, reconnect — so adding a
  switch needs no code.
- **`connector` — supervised, self-healing outbound switch links.** Keeps a
  `link.Link` dialled and a `mux.Mux` running over it, reconnecting with bounded
  backoff; optional `OnConnect` (sign-on / key exchange) and `Keepalive` (0800
  echo) hooks; routes server-initiated frames to a space queue. Satisfies
  `runtime.Component`. (The jPOS ChannelAdaptor + QMUX analog.)
- **`gateway` — the inbound routing/transforming switch server.** Receives a
  transaction, routes it to an upstream `Forwarder`, edits the request
  (`BeforeRequest`) and the response (`BeforeResponse`) en route, and replies —
  with an `OnError` decline path. Handles messages on a link concurrently;
  satisfies `runtime.Component`.
- **`trace` — per-transaction lifecycle.** Records one request/response cycle as
  a single correlated unit (timestamped steps + PCI-masked message dumps),
  carried in the request context so every participant annotates the same trace.
  Renders with optional ANSI colour (`WithColor`) and a toggleable timestamp
  column (`NoTimestamps`).
- **`render/xmlio` and an XML channel framer (`link`).** Schema-aware ISO-8583
  XML (un)marshal, plus a matching XML wire framer.
- **`iso8583.Describe` / `Dump` / `LogValuer`.** A built-in, schema-aware message
  describer with PCI masking (PAN / track / PIN), an optional raw-hex column, and
  `WithColor()` for ANSI output.
- **`mux.Mux.Done()` / `Err()`.** Observe a multiplexer's link death (or clean
  close) without issuing a request — the basis for the reconnecting connector.
- **`packager.Postilion()`** — an ISO 8583:1987 profile for Postilion-style
  switch links (e.g. agency/Interswitch acquirers): ASCII MTI, a **binary**
  primary+secondary bitmap, ASCII fixed and LL/LLL variable fields, and a DE 127
  reserved-private subfield group carried under a 6-digit length prefix over a
  binary sub-bitmap. Registered in `Profiles()` as `"postilion"`. Validated
  end-to-end against a live host (0800 sign-on → 0810, 0200 → 0210). A
  clean-room composition from public ISO 8583:1987 field semantics.
- **Examples** — `flowdemo` (two-phase transaction flow), `runtimehost`
  (component lifecycle + hot deploy), `teq` (named switch connections), and
  `teqswitch` (the routing + transforming gateway, with `-serve`).

### Removed

- **`packager.Switch()`, `packager.Fields()`, `packager.Zone()`** and their
  embedded `schemadef/{switch,fields,zone}.json` — the representative "site"
  layouts, superseded by the validated `Postilion()` profile. **Breaking:**
  code referencing those constructors or the `"switch"` / `"fields"` / `"zone"`
  profile ids must move to `Postilion()` / `"postilion"`.

## [0.1.0] - 2026-06-01

First tagged release. A feature-complete, clean-room Go package for ISO-8583
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
  Mastercard overlays, three concrete site layouts (`zone`, `fields`, `switch` —
  the last with **DE 127** as a headerless positional-subfield group, so `127.2`,
  `127.22`, … address like `55.9F26`), and a declarative JSON loader (`go:embed`)
  whose schemas are generated from the same field tables so the two forms cannot
  drift. Supporting catalog primitives: the `subfield.iso` positional-subfield
  composite codec, the `b.hex` ASCII-hex binary codec, 5-/6-digit length prefixes
  (`len.lllll.ascii` / `len.llllll.ascii`), and `SchemaBuilder.Headerless()`.
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
  TR-31 version B key blocks, and EMV ARQC/ARPC, behind a `Vault` façade. Two
  software backends: `SoftVault` (keys in memory) and the hardened `SealedVault`
  (working keys encrypted at rest under a caller-supplied KEK via AES-GCM,
  decrypted only transiently and zeroized after each operation, with key check
  values, per-key usage enforcement, and an audit hook); HSM/PKCS#11 is a drop-in
  adapter.
- **Enterprise / ops (`rbac`, `store`, `ops`)** — role-based access control with
  PBKDF2 credentials and constant-time authentication; a collection/key/value
  persistence `Store`; and an operational surface with health checks, a metrics
  registry (Prometheus text; implements `runtime.Observer`), cluster membership,
  and an rbac-guarded HTTP admin API.
- **Examples** — runnable `acquirer` and `issuer` programs, a `simulator` test
  host assembled from `runtime` + `ops` + transport, and the shared `posdemo`
  library with an end-to-end loopback test.
- **Project governance & tooling** — dual-license scaffolding (AGPLv3 +
  commercial CLA, `NOTICE`, `AUTHORS`, `SECURITY.md`), a draft commercial-license
  template (`COMMERCIAL-AGREEMENT.md`, pending counsel review), and an `.air.toml`
  live-reload config for the `simulator` (dev tooling, not a module dependency).

### Security notes

- `vault.SoftVault` is for development, testing, and conformance only. Production
  PIN and key handling must use a certified HSM behind the `Vault` interface.
- The admin API distinguishes 401 (unauthenticated) from 403 (forbidden) and
  authenticates in constant time; expose it only behind appropriate network
  controls.

[Unreleased]: https://github.com/teqpace-services/isopace/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/teqpace-services/isopace/compare/v1.0.0-rc.1...v1.0.0
[1.0.0-rc.1]: https://github.com/teqpace-services/isopace/compare/v0.3.0...v1.0.0-rc.1
[0.3.0]: https://github.com/teqpace-services/isopace/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/teqpace-services/isopace/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/teqpace-services/isopace/releases/tag/v0.1.0
