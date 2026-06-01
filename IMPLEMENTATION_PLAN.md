# Isopace — Implementation Plan & Progress Tracker

> **Living document.** Status colours are updated as work lands. The detailed
> design each phase implements lives in [`ARCHITECTURE.md`](ARCHITECTURE.md).
>
> **Last updated:** 2026-06-01

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
| 0 | Project foundation | 🟩🟩🟩🟩🟩🟩🟩🟩🟩🟨 | 90 |
| 1 | ISO-8583 core (`iso8583/`) | ⬜⬜⬜⬜⬜⬜⬜⬜⬜⬜ | 0 |
| 2 | Codec catalogs (`fieldcodec/`, `lengthcodec/`) | ⬜⬜⬜⬜⬜⬜⬜⬜⬜⬜ | 0 |
| 3 | Packager profiles (`packager/`) | ⬜⬜⬜⬜⬜⬜⬜⬜⬜⬜ | 0 |
| 4 | Alternate renderings (`render/`) | ⬜⬜⬜⬜⬜⬜⬜⬜⬜⬜ | 0 |
| 5 | Conformance & QA harness | ⬜⬜⬜⬜⬜⬜⬜⬜⬜⬜ | 0 |
| 6 | Transport (`link/`, `listener/`, `switch/`) | ⬜⬜⬜⬜⬜⬜⬜⬜⬜⬜ | 0 |
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
| 🟡 | Initial commit (local) ✓ — push to `origin/main` is yours to run | — |
| 🟢 | Contact mailboxes (`licensing@`/`security@`) + employee IP-assignment confirmed | — |
| 🟢 | Repo-hardening files: CODEOWNERS, dependabot, PR template, CLA-bot workflow, issue config | `.github/` |
| 🟡 | GitHub settings: branch protection, private vuln reporting, CLA-bot secret (yours) | GitHub UI |

---

## Phase 1 — ISO-8583 core (`iso8583/`)

The dependency-free heart. Built in slices so each compiles and tests green.

| Status | Task | File |
|:------:|------|------|
| 🔵 | `FieldPath` grammar — parse/format, inline array, LRU cache, `DE`/`MustPath` | `path.go` |
| ⚪ | `Decimal` / `Amount` fixed-point money (never `float64`) | `decimal.go` |
| ⚪ | `Bitmap` — 192-bit set, popcount, `Range`, engine-derived only | `bitmap.go` |
| ⚪ | Structured errors — `FieldError`, `Violation`, `ValidationError` | `errors.go` |
| ⚪ | Codec **interfaces** `FieldCodec`/`LengthCodec`/`BitmapCodec` (§10 C1) | `codec_iface.go` |
| ⚪ | `Value` — zero-copy view, `Kind`, `Bytes`/`String`/`Int`/`Decimal` | `value.go` |
| ⚪ | `Schema` / `FieldDef` / `BitmapSpec` + build-time validation | `schema.go` |
| ⚪ | `SchemaBuilder` — `Field`/`Composite`/`Tag` (§10 C4), `Derive`/`Override` | `schema_builder.go` |
| ⚪ | `Message` — slots, copy-on-write, `Seal`/`Clone`, dynamic API (§10 C3) | `message.go` |
| ⚪ | `Get[T]` union-constraint generic + typed setters (§10 C2) | `get.go` |
| ⚪ | `Codec` engine — lazy `Unmarshal`, append `Marshal` (derived bitmap, fast path), `Validate` | `codec.go` |
| ⚪ | `Validator` interface + combinators (`Luhn`, `Digits`, `LenBetween`, …) | `validate.go` |
| ⚪ | `Binder[T]` struct-binding, cached plan, init-time tag check | `bind.go` |
| ⚪ | `View` read-only projection | `view.go` |
| ⚪ | Unit tests + fuzz (parse, marshal/unmarshal round-trip, COW) | `*_test.go` |

---

## Phase 2 — Codec catalogs (`fieldcodec/`, `lengthcodec/`)

| Status | Task | File |
|:------:|------|------|
| ⚪ | EBCDIC CP037 / CP1047 translation tables | `internal/ebcdic/` |
| ⚪ | Packed-decimal (BCD) pack/unpack primitives | `internal/bcd/` |
| ⚪ | Char codecs — ASCII, EBCDIC037, EBCDIC1047, UTF-8, binary | `fieldcodec/char.go` |
| ⚪ | Numeric codecs — ASCII, BCD, rBCD, binary, EBCDIC | `fieldcodec/numeric.go` |
| ⚪ | Amount codecs — ASCII, BCD, binary → `Decimal` | `fieldcodec/amount.go` |
| ⚪ | Bitmap codecs — binary, hex, EBCDIC | `fieldcodec/bitmap.go` |
| ⚪ | Length codecs — Fixed, LL/LLL/LLLL × ASCII/BCD/Binary/EBCDIC | `lengthcodec/length.go` |
| ⚪ | BER-TLV composite (EMV DE 55), hex-tag addressing | `fieldcodec/tlv/bertlv.go` |
| ⚪ | `Registry` + `DefaultRegistry()` (explicit population) | `fieldcodec/registry.go` |
| ⚪ | Per-codec conformance tests (round-trip + edge cases) | `*_test.go` |

---

## Phase 3 — Packager profiles (`packager/`)

| Status | Task | File |
|:------:|------|------|
| ⚪ | ISO 8583:1987 — variants A / B / C | `iso87.go` |
| ⚪ | ISO 8583:1993 — variants A / B | `iso93.go` |
| ⚪ | Visa Base I (overlay on iso93) | `visa.go` |
| ⚪ | Mastercard (overlay) | `mastercard.go` |
| ⚪ | Generic YAML/JSON schema loader (codec-by-name) | `generic.go` |
| ⚪ | Embedded declarative schema sources | `schemadef/*.yaml` |
| ⚪ | Profile round-trip tests | `*_test.go` |

---

## Phase 4 — Alternate renderings (`render/`)

| Status | Task | File |
|:------:|------|------|
| ⚪ | JSON render/parse (schema-aware, PAN masking) | `render/jsonio/` |
| ⚪ | protobuf render (descriptor-driven) | `render/protobuf/` |
| ⚪ | ISO 20022 bridge (pacs/pain/camt) | `render/iso20022/` |
| ⚪ | Cross-format round-trip tests (one `Message`, many backends) | `*_test.go` |

---

## Phase 5 — Conformance & QA harness

| Status | Task | Notes |
|:------:|------|-------|
| ⚪ | jPOS black-box vector generator (separate tool, **never shipped/linked**) | clean-room: capture wire bytes only |
| ⚪ | Golden conformance vectors (from standard + network specs) | `testdata/` |
| ⚪ | Fuzz suite (unmarshal/marshal, path parser, TLV) | `go test -fuzz` |
| ⚪ | Allocation/throughput benchmarks (zero-copy hot path) | `*_test.go` |
| ⚪ | Race + leak checks in CI | CI matrix |

---

## Phase 6 — Transport (`link/`, `listener/`, `switch/`)

| Status | Task |
|:------:|------|
| ⚪ | `Link` — TCP connection, length framing, keep-alive, filters |
| ⚪ | `Listener` — server, accept loop, per-conn lifecycle |
| ⚪ | `Switch` — MUX: request↔response matching, timeouts |
| ⚪ | Connection pool — health filtering, reconnect (backoff) |
| ⚪ | TLS / mutual TLS |
| ⚪ | Transport tests + loopback integration |

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
