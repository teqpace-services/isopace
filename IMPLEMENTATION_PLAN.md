# Isopace — Implementation Plan & Progress Tracker

> **Living document.** Status colours are updated as work lands. The detailed
> design each phase implements lives in [`ARCHITECTURE.md`](ARCHITECTURE.md).
>
> **Last updated:** 2026-06-01 (Phases 0–6 landed: core, codecs, profiles, renderings, QA, transport)

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
| 7 | Runtime (`runtime/`) | ⬜⬜⬜⬜⬜⬜⬜⬜⬜⬜ | 0 |
| 8 | Processing / TX manager (`flow/`) | ⬜⬜⬜⬜⬜⬜⬜⬜⬜⬜ | 0 |
| 9 | Coordination (`space/`) | ⬜⬜⬜⬜⬜⬜⬜⬜⬜⬜ | 0 |
| 10 | Security / HSM (`vault/`) | ⬜⬜⬜⬜⬜⬜⬜⬜⬜⬜ | 0 |
| 11 | Enterprise / Ops | ⬜⬜⬜⬜⬜⬜⬜⬜⬜⬜ | 0 |
| 12 | Release engineering (v0.1.0) | ⬜⬜⬜⬜⬜⬜⬜⬜⬜⬜ | 0 |

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

| Status | Task |
|:------:|------|
| ⚪ | Component host + lifecycle (start/stop hooks) |
| ⚪ | Deploy descriptors + hot (re)deploy |
| ⚪ | Config loading (YAML/env, hot reload) |
| ⚪ | Structured logging (`slog`) |
| ⚪ | OpenTelemetry traces/metrics |

---

## Phase 8 — Processing / transaction manager (`flow/`)

| Status | Task |
|:------:|------|
| ⚪ | `Flow` + `Stage` pipeline, `Exchange` state |
| ⚪ | prepare / commit / abort semantics |
| ⚪ | Group selectors / conditional routing |
| ⚪ | Journaling + retry + idempotency |
| ⚪ | Profiler / per-stage timing |

---

## Phase 9 — Coordination (`space/`)

| Status | Task |
|:------:|------|
| ⚪ | `Space` interface (in / out / rd / take) |
| ⚪ | In-process backend (channels) |
| ⚪ | Distributed backend — **NATS / JetStream** |
| ⚪ | Persistent store-and-forward |

---

## Phase 10 — Security / HSM (`vault/`)

| Status | Task |
|:------:|------|
| ⚪ | `Vault` crypto façade |
| ⚪ | PIN block formats + translation |
| ⚪ | MAC generation / verification |
| ⚪ | Key management + TR-31 key blocks |
| ⚪ | PKCS#11 / HSM access (miekg + crypto11) |
| ⚪ | DUKPT (X9.24) key derivation |
| ⚪ | EMV ARQC / ARPC |

---

## Phase 11 — Enterprise / Ops

| Status | Task |
|:------:|------|
| ⚪ | User / Role / RBAC |
| ⚪ | Persistence / DB layer |
| ⚪ | System monitor / health endpoints |
| ⚪ | Admin / management API (REST) |
| ⚪ | Metrics + clustering |

---

## Phase 12 — Release engineering (v0.1.0)

| Status | Task |
|:------:|------|
| ⚪ | Runnable examples (acquirer + issuer) |
| ⚪ | Simulator / test host |
| ⚪ | Docs site + API reference |
| ⚪ | SemVer policy + `CHANGELOG.md` |
| ⚪ | Tag `v0.1.0` |

---

## Open decisions

| Status | Decision | Resolution |
|:--:|----------|-----------|
| 🟢 | Contact mailboxes → create `licensing@teqpace.com` **and** `security@teqpace.com` | confirmed |
| 🟢 | Employment IP-assignment vests Isopace copyright in Teqpace | confirmed ✅ |
| 🟢 | Distributed `Space` backend | **NATS / JetStream** (Phase 9) |
| 🟢 | protobuf + ISO 20022 scope | **full** (Phase 4) |
| 🔴 | Commercial-license terms drafted by counsel | before commercial sale |

---

## Update convention

When a task completes, flip its row to 🟢 and recompute the phase's bar/% in the
overview. Use 🟡 for the one task actively in flight, 🔵 for the next queued task.
Keep "Last updated" current. This file is the single source of truth for status.
