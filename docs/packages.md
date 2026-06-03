---
title: Packages
description: >-
  A guided tour of the Isopace packages — the messaging core, the codec catalog,
  profiles, renderers, transport, runtime, processing, coordination, and
  security.
---

# Packages

Isopace is layered with a strict, one-directional dependency rule: the core
model knows the wire format only through the `*Schema` it references, codecs
implement interfaces declared in the core, and every renderer consumes one
read-only `View`. The [Architecture guide](architecture.md) covers the design in
depth; this page is a tour of each package and where to read more.

!!! info "Authoritative API reference"

    The complete, always-current symbol reference is the GoDoc on
    **[pkg.go.dev/github.com/teqpace-services/isopace](https://pkg.go.dev/github.com/teqpace-services/isopace)**.
    The pages below summarize each package's purpose and entry points.

| Layer | Package | Purpose |
|-------|---------|---------|
| Messaging | [`iso8583`](#iso8583) | Immutable, zero-copy `Message`; `Codec`; `Schema`; typed `Get[T]` and struct binding |
| Codecs | [`fieldcodec`, `lengthcodec`](#fieldcodec-lengthcodec) | Orthogonal value × length codecs resolvable by name |
| Profiles | [`packager`](#packager) | ISO 8583:1987/1993 profiles, scheme overlays, declarative loader |
| Rendering | [`render/*`](#render) | One message, many formats — from a read-only `View` |
| Transport | [`link`, `listener`, `mux`](#link-listener-mux) | Framed TCP (+TLS), graceful server, request/response switch |
| Runtime | [`runtime`](#runtime) | Component host & lifecycle, deploy descriptors, hot redeploy |
| Processing | [`flow`](#flow) | Two-phase transaction pipeline |
| Coordination | [`space`](#space) | Keyed tuple space + durable store-and-forward |
| Security | [`vault`](#vault) | PIN, MAC/CMAC, DUKPT, TR-31, EMV behind a `Vault` façade |
| Enterprise/Ops | [`rbac`, `store`, `ops`](#rbac-store-ops) | Auth, persistence, health/metrics/admin, clustering |

---

## iso8583 — the messaging core { #iso8583 }

The core public API, depending only on the standard library and `internal/`. It
holds the in-memory model and the marshal/unmarshal engine.

**Key types**

- `Message` — lazy, immutable, copy-on-write field set over a retained wire
  buffer. Safe to publish to many goroutines with zero locking.
- `Value` — a zero-copy view into the source buffer; `Bytes()` is the documented
  hot-path primitive, `String()`/`Int()`/`Decimal()` decode lazily.
- `Codec` — drives a `Message` through a `Schema`: `Unmarshal` (structural,
  zero-copy), `Marshal` (append-style, bitmap auto-derived), `Validate`
  (exhaustive).
- `Schema` / `FieldDef` / `SchemaBuilder` — immutable, build-time-validated
  message definitions.
- `FieldPath` — one grammar (`"55.9F26"`) used by the dynamic API, struct tags,
  validation errors, and JSON keys.
- `Decimal` / `Amount` — exact fixed-point money; no `float64` ever touches an
  amount.
- `Bitmap` — a 192-bit presence set, always derived at marshal time.
- `Binder[T]` — cached struct-binding over `iso:"..."` tags.
- `FieldError` / `Violation` / `ValidationError` — structured errors carrying the
  field path and byte offset.

```go
s := packager.ISO87A()
c := iso8583.NewCodec(s)

m := iso8583.New(s)
_ = m.Set(0, "0200")
_ = m.Set(2, "4111111111111111")
_ = m.Set(4, iso8583.NewDecimal(1099, 2))   // $10.99, exact
wire, _ := c.Marshal(m, nil)

got, _ := c.Unmarshal(wire)                  // structural only; bodies decode on demand
pan, _ := iso8583.Get[string](got, 2)        // no casting
amt, _ := iso8583.Get[iso8583.Decimal](got, 4)
```

[:octicons-arrow-right-24: Core types & the dual field API](architecture.md#2-core-types--interfaces) ·
[:material-language-go: pkg.go.dev](https://pkg.go.dev/github.com/teqpace-services/isopace/iso8583)

---

## fieldcodec & lengthcodec — the codec catalog { #fieldcodec-lengthcodec }

The defining structural improvement over jPOS: length handling is a separate,
independently-registered `LengthCodec`, composed **orthogonally** with the value
`FieldCodec`. A field's wire behaviour is the *pair* `(LengthCodec, FieldCodec)`
— not a monolithic class. This collapses jPOS's combinatorial `IF*` zoo into ≈6
length × ≈12 value codecs that compose freely.

**Value codecs** — `char.ascii`, `char.ebcdic.cp037`, `char.ebcdic.cp1047`,
`char.utf8`, `b.raw`, `b.hex`, `num.ascii`, `num.ebcdic`, `num.bcd`, `num.rbcd`,
`num.bin`, `amount.ascii`, `amount.bcd`, `amount.bin`.

**Bitmap codecs** — `bitmap.bin`, `bitmap.hex`, `bitmap.ebcdic`.

**Length codecs** — `len.fixed`, `len.ll.ascii` … `len.llllll.ascii`,
`len.ll.bcd`, `len.lll.bcd`, `len.ll.bin`, `len.lll.bin`, `len.ll.ebcdic`,
`len.lll.ebcdic`.

Registries are populated by an **explicit builder** — `DefaultRegistry()` — not
by `init()` side effects, so global state stays visible and import order never
matters. Custom codecs are added by name and referenced from schemas or
declarative profiles, with no fork of the engine:

```go
reg := fieldcodec.DefaultRegistry()
reg.Register(myMMDDHHMMSS{})   // Name() == "date.mmddhhmmss.ascii"
```

[:octicons-arrow-right-24: Registry & catalog](architecture.md#6-fieldcodec--lengthcodec-registry--catalog) ·
[:material-language-go: fieldcodec](https://pkg.go.dev/github.com/teqpace-services/isopace/fieldcodec) ·
[:material-language-go: lengthcodec](https://pkg.go.dev/github.com/teqpace-services/isopace/lengthcodec)

---

## packager — profiles { #packager }

A profile is **just an assembled, immutable `*Schema`** — there is no special
"packager" type, which keeps profiles composable. Scheme dialects are deltas
applied with `Derive`/`Override`.

- **ISO 8583:1987** — variants A (ASCII), B (binary/BCD), C (EBCDIC).
- **ISO 8583:1993** — variants A and B.
- **Card-scheme overlays** — Visa Base I and Mastercard, as deltas over the 1993
  base.
- **Site profiles** — concrete, field-pinned layouts validated against live
  switches: `Postilion()`, `CoralPay()`, `Zone()`.
- **Declarative loader** — `packager/generic` resolves codec/length names from
  JSON, producing an identical `*Schema`.
- `Profiles()` returns the registry of named profiles (`"iso87-a"`,
  `"coralpay"`, `"zone"`, …).

```go
s := packager.ISO87A()           // assembled *iso8583.Schema
c := iso8583.NewCodec(s)
```

[:octicons-arrow-right-24: Packager profiles](architecture.md#7-packager-profiles) ·
[:material-language-go: pkg.go.dev](https://pkg.go.dev/github.com/teqpace-services/isopace/packager)

---

## render — alternate renderings { #render }

Because the model is decoupled from the wire codec, rendering to another format
is a visitor over the schema-typed `Value` tree — it never re-parses the wire.
Every renderer consumes one read-only `View`, so adding a format never touches
the core or any codec. The **same `*Message`** round-trips through every backend.

- `render/jsonio` — schema-aware JSON; numerics stay strings to preserve leading
  zeros and amount scale. Options like `UseNames()` and `MaskPAN()`.
- `render/protobuf` — descriptor-driven (un)marshal (DE → field number).
- `render/iso20022` — bridge to `pacs` / `pain` / `camt` via a declarative
  `map[FieldPath]xpath` table.

```go
b, _ := jsonio.Marshal(msg, jsonio.UseNames(), jsonio.MaskPAN())
```

[:octicons-arrow-right-24: Alternate renderings (View)](architecture.md#8-alternate-renderings-view) ·
[:material-language-go: pkg.go.dev](https://pkg.go.dev/github.com/teqpace-services/isopace/render)

---

## link, listener & mux — transport { #link-listener-mux }

The wire-facing layer: framed TCP with optional TLS, a graceful server, a
connection pool, and a request/response multiplexer that correlates replies to
requests.

- `link` — a framed connection (`link.Dial`); the framer (e.g. a 2-byte length
  prefix), TLS, and MAC filters are link options.
- `listener` — a graceful TCP server.
- `mux` — request/response switching, correlating by a keyer (e.g. STAN +
  terminal).

```go
cl, _ := link.Dial("tcp", "127.0.0.1:8583")
x := mux.New(cl, mux.FieldKeyer(c, 11, 41), mux.WithTimeout(5*time.Second))
resp, _ := x.Request(context.Background(), wire)
```

[:octicons-arrow-right-24: Core API in a nutshell](getting-started.md#core-api-in-a-nutshell) ·
[:material-language-go: link](https://pkg.go.dev/github.com/teqpace-services/isopace/link) ·
[:material-language-go: mux](https://pkg.go.dev/github.com/teqpace-services/isopace/mux)

---

## runtime — the component host { #runtime }

`runtime.Host` is the component container (the jPOS Q2 analog): it supervises
components with a start-in-order / stop-in-reverse lifecycle, supports live
`Deploy`/`Undeploy`, and a `Deployer` turns a directory of declarative JSON
descriptors into running components and hot-(re)deploys them as the files change.
Config, structured logging (`slog`), and an observability facade are wired in.

The higher-level [`teq`](getting-started.md#teq--the-container-assembled) container
wraps `runtime.Host` and adds self-healing upstream `Switch` connections.

[:octicons-arrow-right-24: Component host](getting-started.md#component-host) ·
[:material-language-go: pkg.go.dev](https://pkg.go.dev/github.com/teqpace-services/isopace/runtime)

---

## flow — the transaction pipeline { #flow }

A two-phase `Flow`/`Stage`/`Exchange` transaction manager. A prepare pass
validates, routes, and reserves; a commit pass captures, or an abort pass
releases the hold and carries a decline. Routing, journaling, retry,
idempotent retransmission, and a per-stage profiler are built in.

[:octicons-arrow-right-24: Transaction flow](getting-started.md#transaction-flow) ·
[:material-language-go: pkg.go.dev](https://pkg.go.dev/github.com/teqpace-services/isopace/flow)

---

## space — coordination { #space }

A keyed tuple space plus a durable, crash-safe store-and-forward queue. Inside
the container, components decouple through a shared space — for example a
connector routes server-initiated advices to a queue via its
`unsolicited_queue`.

[:material-language-go: pkg.go.dev](https://pkg.go.dev/github.com/teqpace-services/isopace/space)

---

## vault — security { #vault }

PIN blocks, message MAC / CMAC, DUKPT, TR-31 key blocks, and EMV ARQC/ARPC behind
a single `Vault` façade. `vault.SHA256MAC` / `VerifySHA256MAC` provide the
prefix-keyed SHA-256 message hash used by some acquirer links (e.g. CoralPay).

!!! danger "Software backend is for development and testing only"

    The built-in software `Vault` is for development and testing. **Production
    PIN and key handling require a certified HSM.** Cryptographic and
    key-management components are security-critical — see the
    [security policy](security.md).

[:material-language-go: pkg.go.dev](https://pkg.go.dev/github.com/teqpace-services/isopace/vault)

---

## rbac, store & ops — enterprise & ops { #rbac-store-ops }

The operational layer that turns the framework into a deployable service.

- `rbac` — role-based access control with PBKDF2 authentication.
- `store` — persistence.
- `ops` — health and readiness checks, metrics, an admin API, and cluster
  membership.

[:material-language-go: rbac](https://pkg.go.dev/github.com/teqpace-services/isopace/rbac) ·
[:material-language-go: store](https://pkg.go.dev/github.com/teqpace-services/isopace/store) ·
[:material-language-go: ops](https://pkg.go.dev/github.com/teqpace-services/isopace/ops)
