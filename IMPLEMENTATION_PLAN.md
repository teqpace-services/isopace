# Isopace — Implementation Plan & Progress Tracker

> **Living document.** Status colours are updated as work lands. The detailed
> design each phase implements lives in [`ARCHITECTURE.md`](ARCHITECTURE.md).
>
> **Last updated:** 2026-06-01 (**v0.1.0 released** — Phases 0–12 folded into a single first release on `main`: concrete site packagers `zone`/`fields`/`switch` with DE 127 subfields, hardened `SealedVault`, and the commercial-agreement draft; tag pushed and GitHub release published)

---

## Status legend

| Marker | Meaning |
|:------:|---------|
| 🟢 | **Done** — implemented, `gofmt`'d, `go vet` clean, tested, merged |
| 🟡 | **In progress** — actively being worked on |
| 🔵 | **Next** — unblocked and queued to start |
| ⚪ | **Not started** |
| 🔴 | **Blocked** — needs a decision or an external input (see [Open decisions](#open-decisions)) |

**Definition of Done (per task):** code compiles, `gofmt`/`go vet` clean, unit
tests (and fuzz/bench where noted) pass, SPDX header present, public API matches
`ARCHITECTURE.md` (incl. §10 corrections), no copyleft dependency introduced.

---

## Milestone overview

| # | Phase | Progress | % |
|:-:|-------|----------|:-:|
| 0 | Project foundation | 🟩🟩🟩🟩🟩🟩🟩🟩🟩🟩 | 100 |
| 1 | ISO-8583 core (`iso8583/`) | 🟩🟩🟩🟩🟩🟩🟩🟩🟩🟩 | 100 |
| 2 | Codec catalogs (`fieldcodec/`, `lengthcodec/`) | 🟩🟩🟩🟩🟩🟩🟩🟩🟩🟩 | 100 |
| 3 | Packager profiles (`packager/`) | 🟩🟩🟩🟩🟩🟩🟩🟩🟩🟩 | 100 |
| 4 | Alternate renderings (`render/`) | 🟩🟩🟩🟩🟩🟩🟩🟩🟩🟩 | 100 |
| 5 | Conformance & QA harness | 🟩🟩🟩🟩🟩🟩🟩🟩🟩🟩 | 100 |
| 6 | Transport (`link/`, `listener/`, `mux/`) | 🟩🟩🟩🟩🟩🟩🟩🟩🟩🟩 | 100 |
| 7 | Runtime (`runtime/`) | 🟩🟩🟩🟩🟩🟩🟩🟩🟩🟩 | 100 |
| 8 | Processing / TX manager (`flow/`) | 🟩🟩🟩🟩🟩🟩🟩🟩🟩🟩 | 100 |
| 9 | Coordination (`space/`) | 🟩🟩🟩🟩🟩🟩🟩🟩🟩🟩 | 100 |
| 10 | Security / HSM (`vault/`) | 🟩🟩🟩🟩🟩🟩🟩🟩🟩🟩 | 100 |
| 11 | Enterprise / Ops (`rbac/`, `store/`, `ops/`) | 🟩🟩🟩🟩🟩🟩🟩🟩🟩🟩 | 100 |
| 12 | Release engineering (v0.1.0) | 🟩🟩🟩🟩🟩🟩🟩🟩🟩🟩 | 100 |

**Legend for bars:** 🟩 done · 🟨 in progress · ⬜ remaining (each phase = 10 cells).

---

## Phase 0 — Project foundation

| Status | Task | Artifact |
|:------:|------|----------|
| 🟢 | Git repo + Go module (Go 1.26) | `go.mod` |
| 🟢 | Dual-license governance (CLA, commercial, notice, authors, security) | `CLA.md`, `COMMERCIAL-LICENSE.md`, `NOTICE`, `AUTHORS`, `SECURITY.md` |
| 🟢 | Contributor guide + SPDX header convention + clean-room rule | `CONTRIBUTING.md` |
| 🟢 | CI: fmt / vet / build / `-race` test + `go-licenses` copyleft gate | `.github/workflows/ci.yml` |
| 🟢 | Core architecture spec (+ §10 review corrections) | `ARCHITECTURE.md` |
| 🟢 | Implementation plan / progress tracker | `IMPLEMENTATION_PLAN.md` |
| 🟢 | Self-verifying license fetch script | `scripts/fetch-license.sh` |
| 🟢 | Populate `LICENSE` with verbatim AGPL-3.0 (fetched + verified) | `LICENSE` |
| 🟢 | Initial commit ✓ (note: 2 local commits ahead of `origin/main`; push is yours to run) | — |
| 🟢 | Contact mailboxes (`licensing@`/`security@`) + employee IP-assignment confirmed | — |
| 🟢 | Repo-hardening files: CODEOWNERS, dependabot, PR template, CLA-bot workflow, issue config | `.github/` |
| 🟢 | GitHub settings: branch protection, private vuln reporting, CLA-bot secret | GitHub UI |

---

## Phase 1 — ISO-8583 core (`iso8583/`)

The dependency-free heart. Built in slices so each compiles and tests green.

| Status | Task | File |
|:------:|------|------|
| 🟢 | `FieldPath` grammar — parse/format, inline array, LRU cache, `DE`/`MustPath` | `path.go` |
| 🟢 | `Decimal` / `Amount` fixed-point money (never `float64`) | `decimal.go` |
| 🟢 | `Bitmap` — 192-bit set, popcount, `Range`, engine-derived only | `bitmap.go` |
| 🟢 | Structured errors — `FieldError`, `Violation`, `ValidationError` | `errors.go` |
| 🟢 | Codec **interfaces** `FieldCodec`/`LengthCodec`/`BitmapCodec` (§10 C1) | `codec_iface.go` |
| 🟢 | `Value` — zero-copy view, `Kind`, `Bytes`/`String`/`Int`/`Decimal` | `value.go` |
| 🟢 | `Schema` / `FieldDef` / `BitmapSpec` + build-time validation | `schema.go` |
| 🟢 | `SchemaBuilder` — `Field`/`Composite`/`Tag` (§10 C4), `Derive`/`Override` | `schema_builder.go` |
| 🟢 | `Message` — slots, copy-on-write, `Seal`/`Clone`, dynamic API (§10 C3) | `message.go` |
| 🟢 | `Get[T]`/`GetS`/`GetP` union-constraint generics (§10 C2) | `get.go` |
| 🟢 | `Codec` engine — lazy `Unmarshal`, append `Marshal` (derived bitmap, fast path), `Validate` | `codec.go` |
| 🟢 | `Validator` interface + combinators (`Luhn`, `Digits`, `LenBetween`, …) | `validate.go` |
| 🟢 | `Binder[T]` struct-binding, cached plan, init-time tag check | `bind.go` |
| 🟢 | `View` read-only projection | `view.go` |
| 🟢 | Unit tests + fuzz (parse, marshal/unmarshal round-trip, COW) | `*_test.go` |

> **Done:** `go build`/`go vet`/`gofmt` clean, `go test -race` green, both fuzz
> targets run clean (a 13-digit-DE int32-overflow bug was caught and fixed; its
> regression seed lives in `iso8583/testdata/fuzz/`). The package is
> stdlib-only (no copyleft dependency). Engine tests are driven by minimal
> in-package stub codecs (`codecs_test.go`); the shipped codec catalog is Phase 2.
>
> **Carried into Phase 2 (by design, not a gap):** the *runtime* decode/encode
> of nested composites & BER-TLV. The `FieldPath` grammar, `Composite`/`Tag`
> schema builders, `Sub`-schema lookup, and `Value.Composite()` plumbing are all
> in place and validated at build time, but the actual TLV/subfield wire codec
> (and therefore nested `SetP`/binding writes) lands with `tlv.ber` below.

---

## Phase 2 — Codec catalogs (`fieldcodec/`, `lengthcodec/`)

| Status | Task | File |
|:------:|------|------|
| 🟢 | EBCDIC CP037 / CP1047 translation tables | `internal/ebcdic/` |
| 🟢 | Packed-decimal (BCD) pack/unpack primitives (left + right aligned) | `internal/bcd/` |
| 🟢 | Char codecs — ASCII, EBCDIC037, EBCDIC1047, UTF-8, binary | `fieldcodec/char.go` |
| 🟢 | Numeric codecs — ASCII, EBCDIC, BCD, rBCD, binary | `fieldcodec/numeric.go` |
| 🟢 | Amount codecs — ASCII, BCD, binary → `Decimal` | `fieldcodec/amount.go` |
| 🟢 | Bitmap codecs — binary, hex, EBCDIC | `fieldcodec/bitmap.go` |
| 🟢 | Length codecs — Fixed, LL/LLL/LLLL × ASCII/BCD/Binary/EBCDIC | `lengthcodec/length.go` |
| 🟢 | BER-TLV composite (EMV DE 55), hex-tag addressing | `fieldcodec/tlv/bertlv.go` |
| 🟢 | `Registry` + `DefaultRegistry()` (explicit population) | `fieldcodec/registry.go` |
| 🟢 | Per-codec conformance tests (round-trip + edge cases) | `*_test.go` |

> **Done:** all packages build, `go vet`/`gofmt` clean, `go test -race` green,
> stdlib-only. Enabling core refinements landed in `iso8583/`: exported `Value`
> constructors (so external codec packages can build canonical values), a
> logical-unit length contract with the optional `WidthCodec` (correct packed-BCD
> odd-digit widths), verbatim clean-field copy on Marshal, an `Amount` `Scale`
> field, and the tag-addressed composite runtime (`NewTLV`/`PutTag`/`TagSeq`,
> `GetP`/`SetP` tag routing) that makes `"55.9F26"` work end to end.

---

## Phase 3 — Packager profiles (`packager/`)

| Status | Task | File |
|:------:|------|------|
| 🟢 | ISO 8583:1987 — variants A / B / C | `iso87.go` |
| 🟢 | ISO 8583:1993 — variants A / B | `iso93.go` |
| 🟢 | Visa Base I (overlay on iso93) | `visa.go` |
| 🟢 | Mastercard (overlay) | `mastercard.go` |
| 🟢 | Generic JSON schema loader (codec-by-name); YAML is a drop-in | `generic.go` |
| 🟢 | Embedded declarative schema sources (`packager/schemadef/*.json`) | `schemadef/*.json` |
| 🟢 | Profile round-trip + overlay + EMV tests | `*_test.go` |

> **Done:** profiles assemble from a shared, representation-independent DE
> directory; A/B/C and 93 A/B differ only by a `rep` (codec/length/bitmap set).
> Scheme dialects are `Derive`/`Override` deltas (Visa DE 62/63 binary;
> Mastercard DE 48/63). The generic loader resolves codecs by name against
> `DefaultRegistry()` and produces an identical `*Schema`, including BER-TLV
> composites. All 7 profiles round-trip (incl. EMV `55.9F26`); `go test -race`
> green; stdlib-only. Notes: the DE directory is a representative working subset
> (not all 128 DEs); EBCDIC amounts use the ASCII amount codec (no `amount.ebcdic`
> in the catalog yet); `schemadef/` lives under `packager/` for `go:embed`; YAML
> config is a drop-in once a permissive YAML dependency is added.

---

## Phase 4 — Alternate renderings (`render/`)

| Status | Task | File |
|:------:|------|------|
| 🟢 | JSON render/parse (schema-aware, PAN masking, TLV nesting) | `render/jsonio/` |
| 🟢 | protobuf render (DE-keyed wire codec, stdlib) | `render/protobuf/` |
| 🟢 | ISO 20022 bridge (representative pacs.008 subset) | `render/iso20022/` |
| 🟢 | Cross-format round-trip tests (one `Message`, many backends) | `*_test.go` |

> **Done:** every renderer consumes only the read-only `View`, so the same
> `*Message` projects losslessly to ISO-8583 wire, JSON and protobuf
> (cross-format test proves all three reconstruct a wire-identical message),
> and to a pacs.008 subset for the mapped DEs. All stdlib — no protobuf/YAML/XML
> third-party dependency. `go test -race` green across all 10 packages.
> Notes: protobuf field number = DE (field 1 = MTI), descriptor-compatible
> without a protobuf runtime; the ISO 20022 bridge is a representative pacs.008
> mapping (amount/currency, identifiers, datetime, debtor account) — DEs outside
> the map are not carried, and currency rides as the numeric ISO-8583 code
> pending a numeric→alpha ISO 4217 table.

---

## Phase 5 — Conformance & QA harness

| Status | Task | Notes |
|:------:|------|-------|
| 🟢 | Black-box vector generator (separate tool, **never shipped/linked**) | `tools/vectorgen/` — captures wire bytes only; no jPOS import |
| 🟢 | Golden conformance vectors (hand-derived from the standard) | `conformance/testdata/*.hex` |
| 🟢 | Fuzz suite (unmarshal/marshal, path parser, TLV, profile round-trip) | `go test -fuzz` |
| 🟢 | Allocation/throughput benchmarks (zero-copy hot path) | `conformance/bench_test.go` |
| 🟢 | Race + fuzz-smoke CI jobs (+ goroutine-leak checks land with transport) | CI matrix |

> **Done:** clean-room golden vectors (ISO 87-A 0200/0210, hand-derived MTI +
> bitmap + fields, byte-exact decode/re-encode) under `conformance/`; fuzz
> targets for the path parser, unmarshal/marshal, full profile round-trip and the
> BER-TLV parser (all survive millions of execs, no panics); benchmarks confirm
> the design — **Marshal clean fast-path 1.4 ns/op, 0 allocs** (store-and-forward
> verbatim), structural Unmarshal ~2 allocs/op. CI gains a fuzz-smoke job and a
> benchmark compile/smoke step alongside `-race`. The `vectorgen` tool is a
> standalone TCP wire-capture client that never imports jPOS or the Isopace
> library. (Goroutine-leak checks live in the Phase 6 transport tests.)

---

## Phase 6 — Transport (`link/`, `listener/`, `mux/`)

| Status | Task |
|:------:|------|
| 🟢 | `Link` — TCP connection, length framing, keep-alive, filters | `link/` |
| 🟢 | `Listener` — server, accept loop, per-conn lifecycle, graceful shutdown | `listener/` |
| 🟢 | `Switch` → `mux/` (renamed: `switch` is a Go keyword) — request↔response matching, timeouts | `mux/` |
| 🟢 | Connection pool — health filtering, reconnect (backoff) | `link.Pool` |
| 🟢 | TLS / mutual TLS | `link.WithTLS` / `listener.WithTLS` |
| 🟢 | Transport tests + loopback integration (+ goroutine-leak checks) | `*_test.go` |

> **Done:** `link.Link` is a framed connection (pluggable `Framer`, default
> big-endian length-prefix; ordered `Filter`s; TLS via `WithTLS`; concurrent-safe
> `Send`, single-reader `Receive`). `listener` runs the accept loop with
> graceful shutdown (closes active links, waits for handlers). `mux` (named so
> because `switch` is a Go keyword) correlates request/response over one link by
> a `Keyer` (default `FieldKeyer(codec, 11, 41)` = STAN + terminal), with
> per-request timeout/context, a background reader, and an unsolicited handler.
> `link.Pool` round-robins healthy links and reconnects failed slots with
> exponential backoff. Loopback integration tests over real localhost TCP cover
> concurrent correlation, timeout, close-unblocks-pending, and **goroutine-leak
> checks**; all green under `-race`. Stdlib-only (`net`/`crypto/tls`).

---

## Phase 7 — Runtime (`runtime/`)

| Status | Task | File |
|:------:|------|------|
| 🟢 | Component host + lifecycle (ordered start, reverse stop, partial-start unwind, Deploy/Undeploy, Run) | `host.go`, `component.go` |
| 🟢 | Deploy descriptors + hot (re)deploy (Descriptor/Factory/Registry + Deployer dir reconcile) | `deploy.go` |
| 🟢 | Config loading (JSON + env overrides, dotted-path getters, struct Unmarshal, atomic hot reload) | `config.go`, `watch.go` |
| 🟢 | Structured logging (`slog`) — text/JSON handlers, level parsing | `log.go` |
| 🟢 | Observability — `Observer` facade (spans/counters/histograms), no-op + slog backends | `observe.go` |

> **Done:** the component host starts in registration order and stops in
> reverse, unwinds a partial start (joining stop errors), and supports
> Deploy/Undeploy while running; `Run` blocks on context then stops within a
> shutdown timeout. The `Deployer` reconciles a directory of JSON descriptors
> onto the host and rescans for hot redeploy (building a new component before
> tearing down the old). Config is a JSON tree with env-prefix overrides
> (skipping malformed scalar collisions) and atomic `Reload` via `ConfigWatcher`.
> `go test -race` green; goroutine-leak-checked. **Dependency note:** YAML is a
> documented drop-in over the same tree, and **OpenTelemetry is abstracted
> behind `Observer`** with no-op/slog defaults — a real OTel exporter is a
> drop-in adapter, so the module stays **stdlib-only** (zero third-party deps).

---

## Phase 8 — Processing / transaction manager (`flow/`)

| Status | Task | File |
|:------:|------|------|
| 🟢 | `Flow` + `Stage` pipeline, `Exchange` state | `flow.go`, `stage.go`, `exchange.go` |
| 🟢 | prepare / commit / abort semantics (two-phase, reverse rollback) | `flow.go`, `stage.go` |
| 🟢 | Group selectors / conditional routing (`Route`, worklist, loop cap) | `flow.go`, `stage.go` |
| 🟢 | Journaling + retry + idempotency | `journal.go`, `retry.go`, `idempotency.go` |
| 🟢 | Profiler / per-stage timing | `exchange.go`, `flow.go` |

> **Done:** transactions run through named groups in two phases over the
> immutable copy-on-write `Message`: a prepare pass (with conditional routing)
> then a commit pass over joined stages in order, or an abort pass over them in
> reverse if any stage fails (a stage that reserved work and then aborts still
> joins, so its rollback runs). `Exchange` carries request/response, a property
> bag, abort state and per-stage timings (`Profile()`). `Journal` records the
> lifecycle (basis for store-and-forward recovery); `WithIdempotency` replays the
> stored response for a duplicate (clone-on-store/lookup); `Retry` re-runs
> Prepare with ctx-aware backoff; `WithMaxStages` guards routing loops. `go test
> -race` green; stdlib-only.

---

## Phase 9 — Coordination (`space/`)

| Status | Task | File |
|:------:|------|------|
| 🟢 | `Space` interface (Out / In = take / Rd = read / Inp / Rdp), context-bounded blocking | `space.go` |
| 🟢 | In-process backend (mutex map + close-and-replace broadcast wakeups) | `local.go` |
| 🟢 | Distributed backend — **NATS / JetStream** (abstracted: implements `Space`; adapter is a drop-in module) | `doc.go` |
| 🟢 | Persistent store-and-forward (crash-safe append log, replay, compaction) | `store.go` |

> **Done:** a keyed tuple space where each key is a FIFO bag (a queue), with
> `Out`/`In`(take)/`Rd`(read) plus non-blocking `Inp`/`Rdp`; blocking calls are
> context-cancelable and waiters wake via a close-and-replace broadcast (no
> per-call goroutine). `Local` is the in-process backend; `Store` is durable
> store-and-forward — a `[]byte` queue over a crash-safe append-only log
> (tombstone fsync'd before an entry leaves memory; truncated tail ignored on
> replay; `Compact` rewrites live entries with dir-fsync'd atomic rename).
> `go test -race` green; concurrent exactly-once + goroutine-leak checked.
> **Dependency note:** the **NATS/JetStream backend is a drop-in `Space`
> adapter** in a separate optional module, keeping the core **stdlib-only**.

---

## Phase 10 — Security / HSM (`vault/`)

| Status | Task | File |
|:------:|------|------|
| 🟢 | `Vault` crypto façade + `SoftVault` + hardened `SealedVault` | `vault.go`, `sealed.go` |
| 🟢 | PIN block formats (ISO 9564 0/1/3) + encrypt/translate + pad validation | `pinblock.go`, `cipher.go` |
| 🟢 | MAC generation/verification (ISO 9797-1 alg 1 & 3) + CMAC | `mac.go`, `cmac.go` |
| 🟢 | Key management + TR-31 version B key blocks (wrap/unwrap) | `keyblock.go` |
| 🟢 | PKCS#11 / HSM access (abstracted: HSM is a drop-in `Vault` adapter module) | `vault.go`, `doc.go` |
| 🟢 | DUKPT (ANSI X9.24-1 TDES) key derivation — validated vs public vector | `dukpt.go` |
| 🟢 | EMV ARQC / ARPC (Common Session Key) | `emv.go` |

> **Done:** stdlib-only payment cryptography (`crypto/des`, `crypto/aes`,
> `crypto/cipher`, `crypto/subtle`) behind a `Vault` façade. PIN blocks (ISO
> 9564 formats 0/1/3) with encode/decode/translate and decode-time pad-integrity
> checks; MAC (ISO 9797-1 algorithm 1 and the X9.19 retail MAC, padding methods
> 1/2) with constant-time verify, plus CMAC; DUKPT (X9.24-1 TDES) IPEK and
> per-transaction derivation **validated against the canonical public X9.24
> vector**; TR-31 version B key blocks (CMAC-derived keys, MAC-as-IV, tamper
> detection); EMV Common Session Key + ARQC/ARPC. `SoftVault` holds keys in
> memory; `go test -race` green; zero third-party deps.
>
> Two software backends implement the `Vault` interface: `SoftVault` (keys in
> memory, for tests) and **`SealedVault`** — hardened: working keys are encrypted
> at rest under a caller-supplied KEK (AES-GCM), decrypted only transiently and
> zeroized after each operation (including the cleartext PIN block), with key
> check values, per-key usage enforcement, and an audit hook.
>
> **SECURITY / dependency notes:** the software backends — even `SealedVault` —
> are for development, testing, and non-PIN crypto; **production PIN and key
> handling must use a certified HSM**, a drop-in `Vault` adapter (e.g. PKCS#11
> via cgo) in a separate optional module, so the core stays stdlib-only (the
> `miekg/crypto11` route would be that adapter, not a core dependency). PCI PIN
> Security / P2PE require a tamper-responsive Secure Cryptographic Device that
> software on a general-purpose host cannot be. AES successors (ISO 9564 format
> 4, TR-31 version D) are noted as future. TR-31 wrap/unwrap is self-consistent
> and tamper-evident; external-vector interop validation is pending a published
> version-B vector.

---

## Phase 11 — Enterprise / Ops (`rbac/`, `store/`, `ops/`)

| Status | Task | File |
|:------:|------|------|
| 🟢 | User / Role / RBAC (wildcard permissions, PBKDF2 credentials) | `rbac/rbac.go`, `rbac/credentials.go` |
| 🟢 | Persistence / DB layer (collection/key/value `Store` + in-memory) | `store/store.go` |
| 🟢 | System monitor / health endpoints (liveness/readiness) | `ops/health.go`, `ops/api.go` |
| 🟢 | Admin / management API (REST, rbac-guarded) | `ops/api.go` |
| 🟢 | Metrics (+ runtime.Observer bridge, Prometheus text) + clustering | `ops/metrics.go`, `ops/cluster.go` |

> **Done:** stdlib-only (`net/http`, `encoding/json`, `crypto/pbkdf2`). `rbac`
> maps users→roles→permissions with colon-namespaced wildcards (`tx:*`, `*`) and
> stores PBKDF2-HMAC-SHA256 credentials (tunable work factor, constant-time
> verify, **constant-time `Authenticate` so timing cannot enumerate users**).
> `store` is a collection/key/value persistence interface with an in-memory
> backend (copy-on-put/get) and typed JSON helpers. `ops` provides
> liveness/readiness health checks, a metrics registry (counters/gauges/
> summaries by label set that **implements `runtime.Observer`** and renders
> Prometheus text — one kind per name, enforced), a cluster-membership interface
> with a static single-node default, and an HTTP admin API (Go 1.22 method
> routing): `/healthz`, `/readyz`, `/metrics` open; `/info` rbac-guarded. `go
> test -race` green; reviewed (3 issues fixed); zero third-party deps.
>
> **Dependency notes:** a **SQL-backed `Store` over `database/sql`** is a
> drop-in adapter (the driver is the deployment's blank-import choice, so the
> core takes on no driver dependency); a **distributed `Cluster`** (gossip/raft,
> or built on the space NATS adapter) is a drop-in. A public admin API should
> additionally sit behind network controls (mTLS / allow-list).

---

## Phase 12 — Release engineering (v0.1.0)

| Status | Task | Artifact |
|:------:|------|----------|
| 🟢 | Runnable examples (acquirer + issuer) | `examples/acquirer`, `examples/issuer`, `examples/posdemo` |
| 🟢 | Simulator / test host | `examples/simulator` (runtime.Host + listener + ops admin API) |
| 🟢 | Docs + API reference | `README.md`, `docs/getting-started.md`; GoDoc is the API reference |
| 🟢 | SemVer policy + `CHANGELOG.md` | `docs/versioning.md`, `CHANGELOG.md` |
| 🟢 | Tag `v0.1.0` + GitHub release | annotated tag pushed and release published at `v0.1.0` |

> **Done:** Isopace is assembled into runnable programs — an `issuer` host
> and an `acquirer` client over a real switch, plus a `simulator` test host built
> from `runtime.Host` supervising an ISO-8583 listener and an ops admin endpoint
> (`/healthz`, `/readyz`, `/metrics`) with metrics wired through
> `runtime.Observer`. The shared `examples/posdemo` carries an end-to-end
> loopback test (approve/decline/concurrent correlation); the binaries were
> smoke-tested. README rewritten as the entry point; `docs/getting-started.md`
> and `docs/versioning.md` (SemVer policy) added; `CHANGELOG.md` records v0.1.0.
> The `v0.1.0` tag is **pushed** and the **GitHub release is published**.

---

## Project status

**🟢 v0.1.0 released.** All 13 phases (0–12) are done: 21 importable packages
plus examples, every one `gofmt`/`go vet` clean and green under `go test -race`,
and the module is **stdlib-only** (no third-party dependency in the graph;
optional OTel / NATS / SQL / HSM integrations are drop-in adapters). The whole
project line landed on `main` via a rebase-merge (linear history; the
long-running `phase-1-iso8583-core` branch is retired), with the post-tag
additions (site packagers, `SealedVault`, commercial-agreement draft) folded into
a single clean **`v0.1.0`** rather than a phantom version. The `v0.1.0` tag is
pushed and the GitHub release is published; the example binaries
(`issuer`/`acquirer`/`simulator`) were run end to end (live 0200/0210 approve and
limit-decline, with ops health + Prometheus metrics).

---

## Open decisions

| Status | Decision | Resolution |
|:--:|----------|-----------|
| 🟢 | Contact mailboxes → create `licensing@teqpace.com` **and** `security@teqpace.com` | confirmed |
| 🟢 | Employment IP-assignment vests Isopace copyright in Teqpace | confirmed ✅ |
| 🟢 | Distributed `Space` backend | **NATS / JetStream** (Phase 9) |
| 🟢 | protobuf + ISO 20022 scope | **full** (Phase 4) |
| 🟡 | Commercial-license terms drafted by counsel | draft template prepared (`COMMERCIAL-AGREEMENT.md`); **pending counsel review** before commercial sale |

---

## Update convention

When a task completes, flip its row to 🟢 and recompute the phase's bar/% in the
overview. Use 🟡 for the one task actively in flight, 🔵 for the next queued task.
Keep "Last updated" current. This file is the single source of truth for status.
