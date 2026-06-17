# Isopace Core Architecture

> Module `github.com/teqpace-services/isopace` · Go 1.26 · clean-room, idiomatic Go · dual-licensed AGPLv3 + commercial · © Teqpace Services Ltd.

This document is the implementation-ready specification for the **Isopace core**: the ISO-8583 in-memory model (`Message`), the schema (`Schema`/`FieldDef`), the per-field wire engine (`FieldCodec` + `LengthCodec` registry), the message marshal/unmarshal engine (`Codec`), struct-binding, validation, and alternate renderings (JSON/protobuf/ISO 20022). It is the single source of truth for implementing `iso8583/`, `fieldcodec/`, `lengthcodec/`, `packager/`, and `render/`.

The network/switch layers (`Link`, `Listener`, `Switch`, `Runtime`, `Flow`/`Stage`, `Exchange`, `Space`, `Vault`) reference these types but are out of scope here.

This synthesis is built around four hard requirements distilled from the design review:

1. **Ergonomics first (the 90% developer):** a discoverable, hard-to-misuse dual API — both a dynamic map-style surface and typed struct-binding — over one uniform `FieldPath` grammar (`"55.9F26"`) used identically in the dynamic API, struct tags, validation errors, and JSON keys.
2. **Allocation-free hot path:** zero-copy lazy decode (fields as sub-slices of the source buffer), array-indexed field slots (no map lookup), `Value.Bytes()` as the zero-copy primitive, append-style `Marshal(dst)` that reuses the caller's buffer, and a store-and-forward fast path that returns the original wire verbatim when nothing is dirty.
3. **Structural correctness invariants:** the bitmap is **always derived** from present fields at marshal time (desync is impossible); `Message` is immutable with copy-on-write; `Validate()` is exhaustive (returns *all* violations); the `Schema` and struct-binding tags are validated at build/init time.
4. **Orthogonal, multi-format extensibility:** `LengthCodec` × `FieldCodec` compose freely (≈6 length × ≈12 value codecs, no combinatorial `IF*` zoo); a single read-only `View` feeds every renderer; profiles are data with `Derive`/`Override` overlays.

---

## Table of Contents

- [1. Package Layout](#1-package-layout)
- [2. Core Types & Interfaces](#2-core-types--interfaces)
- [3. The Dual Field API](#3-the-dual-field-api)
- [4. Zero-Copy Decode](#4-zero-copy-decode)
- [5. Schema & Validation Model](#5-schema--validation-model)
- [6. FieldCodec / LengthCodec Registry & Catalog](#6-fieldcodec--lengthcodec-registry--catalog)
- [7. Packager Profiles](#7-packager-profiles)
- [8. Alternate Renderings (View)](#8-alternate-renderings-view)
- [9. How Isopace differs from jPOS](#9-how-isopace-differs-from-jpos)

---

## 1. Package Layout

```
github.com/teqpace-services/isopace/
├── iso8583/                  # THE CORE public API (depends only on stdlib + internal/)
│   ├── message.go            # Message, slot, copy-on-write, Seal/Clone
│   ├── value.go              # Value (zero-copy view), Kind, typed getters
│   ├── decimal.go            # Decimal/Amount fixed-point money (never float64)
│   ├── bitmap.go             # Bitmap (primary/secondary/tertiary), popcount, Range
│   ├── path.go               # FieldPath grammar ("55.9F26"), parse/format/cache
│   ├── schema.go             # Schema, FieldDef, SchemaBuilder, build-time validation
│   ├── codec.go              # Codec engine: Unmarshal (lazy) / Marshal (append) / Validate
│   ├── view.go               # View interface (read-only projection for renderers)
│   ├── bind.go               # Binder[T] struct-binding, cached plan, init-time tag check
│   ├── validate.go           # Validator interface + built-in rule combinators
│   └── errors.go             # FieldError, Violation, ValidationError aggregate
│
├── fieldcodec/               # the pluggable VALUE FieldCodec catalog
│   ├── registry.go           # Registry, FieldCodec, SpanCodec, DefaultRegistry()
│   ├── char.go               # ASCII, EBCDIC037, EBCDIC1047, UTF8, BINARY
│   ├── numeric.go            # NumASCII, BCD (packed), RBCD, NumBinary
│   ├── amount.go             # AmountASCII, AmountBCD, AmountBinary
│   ├── bitmap.go             # BitmapBinary, BitmapHex, BitmapEBCDIC
│   └── tlv/                  # BER-TLV (EMV field 55) composite sub-codec
│       └── bertlv.go
│
├── lengthcodec/              # the pluggable LENGTH-PREFIX catalog (orthogonal)
│   └── length.go             # Fixed, LLVar/LLLVar/LLLLVar × ASCII/BCD/Binary/EBCDIC
│
├── packager/                 # named PROFILES = assembled immutable Schemas
│   ├── packager.go           # profile registry, Derive/Override helpers
│   ├── iso87.go              # ISO 8583:1987 A / B / C
│   ├── iso93.go              # ISO 8583:1993 A / B
│   ├── visa.go               # Visa Base I (overlay on iso93)
│   ├── mastercard.go         # Mastercard (overlay)
│   └── generic.go            # YAML/JSON schema loader (codec-by-name)
│
├── render/                   # alternate renderings of the SAME Message (consume View)
│   ├── jsonio/               # MarshalJSON / UnmarshalJSON (schema-aware)
│   ├── protobuf/             # descriptor-driven (un)marshal
│   └── iso20022/             # ISO 20022 bridge (pacs/pain/camt)
│
├── schemadef/                # declarative schema sources (embedded via go:embed)
│   ├── iso87a.yaml ...
│
└── internal/
    ├── ebcdic/               # CP037 / CP1047 256-byte translation tables
    └── bcd/                  # packed-decimal nibble pack/unpack primitives
```

**Layering rule (one direction only):**

```
render/ ─┐
         ├─► iso8583/ (core; stdlib + internal only)
packager/ ┤        ▲
          └─► fieldcodec/ ─┐
              lengthcodec/ ┴─► iso8583/ (interfaces only)
```

- `iso8583/` (the model + engine + schema + interfaces) depends on **nothing** in the module except `internal/`. This is the clean *model vs schema vs wire codec* separation: a `Message` knows the ISO-8583 wire format only through the `*Schema` it references; the model itself is wire-agnostic and renders to any format.
- `fieldcodec/` and `lengthcodec/` implement interfaces *declared in* `iso8583/` and are populated into a `Registry` by an **explicit builder** (`DefaultRegistry()`), not by `init()` side effects — no hidden global state, no import-ordering surprises.
- `render/*` consumes only the read-only `View` interface, so adding a format never touches the core or any codec.

---

## 2. Core Types & Interfaces

### 2.1 `FieldPath` — one uniform addressing grammar

A single grammar addresses a field at any depth: plain DEs (`2`), ISO subfields (`48.2.1`), and EMV BER-TLV tags inside DE 55 (`55.9F26`). The **same grammar** is used by the dynamic API, struct tags, validation errors, and JSON keys. Numeric elements are decimal; BER-TLV elements are hex tags (matched case-insensitively, canonicalised to uppercase).

```go
package iso8583

// FieldPath addresses a field at any depth. It is an immutable value type and is
// comparable (usable as a map key) because PathElem is comparable.
//
//   ParsePath("2")        -> [{N:2}]
//   ParsePath("48.2.1")   -> [{N:48} {N:2} {N:1}]
//   ParsePath("55.9F26")  -> [{N:55} {Tag:"9F26"}]
type FieldPath struct {
	elems [maxPathDepth]PathElem // small inline array; no heap for typical depths
	n     uint8
}

// PathElem is one step: either a numeric index (N>=0, Tag=="") or a hex BER-TLV
// tag (Tag!="", N==-1). Exactly one is set.
type PathElem struct {
	N   int32  // numeric DE / subfield index, or -1 when Tag is used
	Tag string // BER-TLV tag in canonical uppercase hex, e.g. "9F26"; "" otherwise
}

const maxPathDepth = 6

// DE constructs a top-level numeric path: DE(2) == ParsePath("2").
func DE(n int) FieldPath

// ParsePath parses the textual grammar. A small LRU cache memoises hot literals
// so repeated ParsePath("55.9F26") in routing code does not re-allocate.
func ParsePath(s string) (FieldPath, error)
func MustPath(s string) FieldPath // panics on error; for package-level constants

func (p FieldPath) Len() int
func (p FieldPath) At(i int) PathElem
func (p FieldPath) Head() PathElem      // first element
func (p FieldPath) Tail() FieldPath     // path without the first element
func (p FieldPath) Push(e PathElem) FieldPath
func (p FieldPath) String() string      // canonical "55.9F26"

// Pather is satisfied by FieldPath, int, and string so the dynamic API accepts
// any of them WITHOUT boxing an int through `any`: the int overloads call DE(n)
// directly and string overloads call ParsePath. See §3.1 for the call sites.
type Pather interface{ ~int | ~string | path() }
```

> **Design note (judges):** Design A's `path any` boxed every hot-path call and lost compile-time safety. We replace it with a typed `FieldPath` plus thin `DE(int)` / `ParsePath(string)` constructors and overloaded entry points (`Get`/`GetP`/`GetS` below), recovering type safety and avoiding `int` boxing while keeping the "just pass `2` or `"55.9F26"`" ergonomics.

### 2.2 `Value` — the zero-copy decoded view

`Value` is a **view** into the owning `Message`'s source buffer plus a tag describing interpretation. It carries no heap allocation in the common path: `raw` aliases the message's `src`. Decoding to a Go `string`/`int64`/`Decimal` is lazy and explicit.

```go
package iso8583

type Kind uint8

const (
	KindInvalid   Kind = iota
	KindString         // text (charset applied lazily on String())
	KindNumeric        // digit string
	KindBytes          // raw octets
	KindAmount         // signed scaled money -> Decimal
	KindComposite      // has children (subfields / BER-TLV)
)

// Value is a zero-copy view into the source buffer. Copying a Value is cheap
// (it is a small header); it never owns or mutates the underlying bytes.
type Value struct {
	raw   []byte    // sub-slice of the owning Message.src (or owned bytes if built)
	kind  Kind
	codec fieldcodec.FieldCodec // how to interpret raw on demand
	off   int                   // absolute byte offset in src (for error reporting)
	sub   *Message              // non-nil for KindComposite (lazy child message)
}

func (v Value) Kind() Kind { return v.kind }
func (v Value) IsZero() bool { return v.kind == KindInvalid }

// Bytes returns the raw view. ZERO allocation, ZERO copy. The returned slice
// ALIASES the source buffer and MUST NOT be mutated. This is the documented
// zero-copy primitive: hot-path routing should compare with bytes.Equal /
// bytes.HasPrefix against Bytes(), never String().
func (v Value) Bytes() []byte { return v.raw }

// BytesCopy returns an owned copy, safe to retain and mutate.
func (v Value) BytesCopy() []byte

// String decodes text lazily. NOTE: Go strings are immutable and cannot alias a
// []byte without a copy, so this is the single unavoidable allocation for text
// (plus transcoding for EBCDIC). Prefer Bytes() on the hot path.
func (v Value) String() (string, error)

// Int / Uint parse digits straight off raw without an intermediate string,
// returning a *FieldError with the byte offset on malformed input.
func (v Value) Int() (int64, error)
func (v Value) Uint() (uint64, error)

// Decimal decodes an amount field to exact fixed-point money (never float64).
func (v Value) Decimal() (Decimal, error)

// Composite returns the lazily-decoded child message for KindComposite fields
// (subfield groups or BER-TLV), enabling nested addressing.
func (v Value) Composite() (*Message, bool)
```

### 2.3 `Decimal` / `Amount` — mandatory fixed-point money

No `float64` ever touches an amount. `Amount` adds currency context for cross-field validation (DE 49/51 vs amount fields).

```go
package iso8583

// Decimal is an exact base-10 fixed-point number: value = Unscaled * 10^-Scale.
type Decimal struct {
	Unscaled int64
	Scale    uint8
}

func NewDecimal(unscaled int64, scale uint8) Decimal
func (d Decimal) String() string                  // "10.99"
func (d Decimal) Rescale(scale uint8) (Decimal, error)
func (d Decimal) Add(o Decimal) (Decimal, error)  // exact; error on scale/overflow

// Amount is a Decimal with an ISO 4217 currency (numeric, e.g. "840").
type Amount struct {
	Decimal
	Currency string
}
```

### 2.4 `Bitmap` — primary / secondary / tertiary, always derived

A fixed 192-bit (3×64) set. Presence test is a single bit op — the per-message hot operation. **The continuation bits (1, 65) are managed entirely by the engine, and the wire bitmap is always derived from the set of present fields at marshal time** — application code never sets bitmap bits, so bitmap/content desync (a real jPOS footgun) is structurally impossible.

```go
package iso8583

// Bitmap is a 192-bit presence set. Field n in 1..192 maps to words[(n-1)/64],
// bit (n-1)%64 counted from the MSB (big-endian bit order on the wire).
type Bitmap struct {
	words [3]uint64
}

func (b Bitmap) IsSet(de int) bool
func (b *Bitmap) Set(de int)     // internal/engine use; app code never calls this
func (b *Bitmap) Clear(de int)
func (b Bitmap) Count() int                  // popcount, excluding continuation bits
func (b Bitmap) Width() int                  // 64 / 128 / 192 per continuation bits
func (b Bitmap) Range(yield func(de int) bool) // ascending present DEs; range-over-func
func (b Bitmap) String() string              // human dump for logs
```

### 2.5 `Message` — lazy, immutable, copy-on-write

The model is a schema-driven, array-indexed sparse field set over a retained source buffer. While read-only, every `Value` aliases the shared `src`, so a decoded `Message` is safe to publish to many goroutines with zero locking and zero copy. The first mutation flips the message to *owned* and clones only the touched field path; everything else keeps aliasing `src`.

```go
package iso8583

type Message struct {
	schema *Schema
	src    []byte        // retained wire buffer; field Values alias INTO this (nil if built fresh)
	bm     Bitmap        // derived presence map
	slots  []slot        // ARRAY-indexed by DE (len == schema.maxDE+1); no map lookup on hot path
	subidx map[FieldPath]*Message // memoised composite children (DE48/55…), lazily built
	owned  bool          // true once any field is mutated (copy-on-write engaged)
	dirty  Bitmap        // fields mutated since decode -> drives selective re-marshal
	sealed bool          // immutable view; mutation goes through Clone()
}

type slot struct {
	v       Value
	present bool // present per bitmap
	decoded bool // Value materialised (memoised) on first typed access
}
```

#### Dynamic map-style API (read)

```go
// Has reports presence WITHOUT decoding (bitmap check only).
func (m *Message) Has(p Pather) bool

// Get returns the zero-copy Value, decoding lazily on first access (memoised).
// Three overloads avoid `any`-boxing: Get(2) takes int, GetS("55.9F26") takes
// string, GetP(path) takes a pre-parsed FieldPath (hottest, no parse).
func (m *Message) Get(de int) (Value, bool)
func (m *Message) GetS(s string) (Value, bool)
func (m *Message) GetP(p FieldPath) (Value, bool)

func (m *Message) MTI() (Value, bool)            // DE 0
func (m *Message) Bitmap() Bitmap
func (m *Message) Schema() *Schema

// Fields iterates present fields in ascending order as a range-over-func.
func (m *Message) Fields() iter.Seq2[FieldPath, Value]
```

#### Typed generic accessor (read) — no interface boxing

```go
// Get[T] is a package-level generic (Go methods cannot have type params). It
// decodes a field directly into a Go type with no boxing, returning a structured
// *FieldError on type mismatch or malformed bytes.
//
//   pan,  _ := iso8583.Get[string](m, 2)
//   stan, _ := iso8583.Get[int64](m, 11)
//   amt,  _ := iso8583.Get[iso8583.Decimal](m, 4)
//   icc,  _ := iso8583.Get[[]byte](m, "55.9F26")   // zero-copy sub-slice
func Get[T FieldType](m *Message, p Pather) (T, error)

// FieldType is the closed set of Go types a field can decode to. Implemented as
// an interface with an unexported method (NOT a type-set union, which cannot mix
// underlying-type approximations with named struct types like Decimal/Bitmap).
// Concrete adapters are provided for string, []byte, int64, uint64, Decimal,
// Amount, time.Time, Bitmap, and *Message.
type FieldType interface{ isFieldType() }
```

> **Design note (judges):** Design A's `FieldType ~string | ~int64 | time.Time | Money | Bitmap` would not compile — a Go type-set union cannot mix `~`-approximation elements with named struct types. We model the closed set as a sealed interface with provided adapters, which compiles and is still fully type-checked at the call site through `Get[T]`'s inference.

#### Dynamic map-style API (write, copy-on-write)

```go
// Set stores a value, triggering copy-on-write on a sealed/decoded message. It
// routes the value to the field's codec by Kind and fails fast with a
// *FieldError on type mismatch. The accepted value types mirror FieldType.
func (m *Message) Set(de int, v any) error
func (m *Message) SetS(s string, v any) error
func (m *Message) SetP(p FieldPath, v any) error

func (m *Message) Unset(p Pather)

// Seal returns an immutable view; subsequent Set on the sealed message clones
// only the touched path (structural sharing). Clone is an explicit deep-ish copy.
func (m *Message) Seal() *Message
func (m *Message) Clone() *Message
```

### 2.6 `Schema` & `FieldDef`

A `Schema` is immutable, shareable, and **validated at build time** (every referenced codec exists, no overlapping subfields, length rules sane). It is built once and reused across all goroutines and messages — never copied per message.

```go
package iso8583

// Schema describes one message format. Immutable; concurrency-safe.
type Schema struct {
	id        string
	mti       *FieldDef
	bitmap    BitmapSpec
	defs      []*FieldDef          // ARRAY indexed by DE (nil = undefined); maxDE+1 long
	resolved  []resolvedField      // codec+length pre-resolved per DE (no map lookup at runtime)
	bindCache sync.Map             // reflect.Type -> *bindPlan (struct-binding plans)
	maxDE     int
}

// FieldDef defines one DE (or subfield). It references codecs from the registry
// by interface (already resolved at Schema build time), not by string at runtime.
type FieldDef struct {
	Path     FieldPath
	Name     string                 // "Primary Account Number"
	Kind     Kind                   // canonical model kind
	Codec    fieldcodec.FieldCodec  // VALUE codec
	Length   lengthcodec.LengthCodec // LENGTH-PREFIX codec (orthogonal); Fixed for fixed-len
	MaxLen   int                    // max value length (digits/chars/octets)
	Pad      PadRule
	Validate []Validator            // ordered field-level rules
	Required RequiredRule           // always | conditional-on-MTI | optional
	Sub      *Schema                // non-nil for composites (DE 48/55/60/63…)
}

// BitmapSpec selects bitmap wire encoding + supported levels for the schema.
type BitmapSpec struct {
	Codec  fieldcodec.BitmapCodec // BitmapBinary | BitmapHex | BitmapEBCDIC
	Levels int                    // 1=primary, 2=+secondary, 3=+tertiary
}

func (s *Schema) ID() string
func (s *Schema) Field(de int) (*FieldDef, bool)
func (s *Schema) Lookup(p FieldPath) (*FieldDef, bool) // walks composites
```

#### Schema builder (build-time validated)

```go
type SchemaBuilder struct{ /* ... */ }

func NewSchema(id string) *SchemaBuilder
func (b *SchemaBuilder) MTI(c fieldcodec.FieldCodec) *SchemaBuilder
func (b *SchemaBuilder) Bitmap(spec BitmapSpec) *SchemaBuilder

// Field wires a DE to a (value codec, length codec) pair plus options. The two
// codecs compose orthogonally: "BCD digits behind an ASCII LLVAR prefix" is just
// Field(2, ..., fieldcodec.BCD, lengthcodec.LLVarASCII, ...).
func (b *SchemaBuilder) Field(
	de int, name string,
	value fieldcodec.FieldCodec, length lengthcodec.LengthCodec,
	opts ...FieldOpt,
) *SchemaBuilder

func (b *SchemaBuilder) Composite(
	de int, name string, sub *Schema, length lengthcodec.LengthCodec, opts ...FieldOpt,
) *SchemaBuilder

func (b *SchemaBuilder) Build(reg *fieldcodec.Registry) (*Schema, error) // validates everything
func (b *SchemaBuilder) MustBuild(reg *fieldcodec.Registry) *Schema

// Derive/Override support card-scheme overlays as deltas over a base profile.
func (s *Schema) Derive(id string) *SchemaBuilder // clone defs, then Override/Field
func (b *SchemaBuilder) Override(de int, opts ...FieldOpt) *SchemaBuilder

// FieldOpt configures a FieldDef.
func MaxLen(n int) FieldOpt
func Pad(p PadRule) FieldOpt
func Optional() FieldOpt
func RequiredOn(mtiClasses ...string) FieldOpt
func Validate(v ...Validator) FieldOpt
```

### 2.7 `Codec` — the message engine

```go
package iso8583

// Codec drives a full message through a Schema. Stateless and concurrency-safe;
// one instance serves all goroutines. Field codecs are pre-resolved by the Schema
// into an array, so the hot path is an array index, not a registry map lookup.
type Codec struct {
	schema *Schema
	opts   CodecOptions
}

func NewCodec(s *Schema, opts ...CodecOption) *Codec

// Unmarshal does a ZERO-COPY structural parse: read MTI + bitmap, then record each
// present field as a (offset,len) span into wire. Field bodies are NOT decoded;
// they decode lazily on first Get and memoise. wire is RETAINED by the Message
// (Values alias into it) — the caller must not mutate wire while the Message lives.
func (c *Codec) Unmarshal(wire []byte) (*Message, error)

// Marshal renders a Message to wire bytes, APPENDING to dst (reuses caller cap).
// The bitmap is DERIVED from present fields here (never hand-set). Fast paths:
//   - if the message is unmodified since Unmarshal (nothing dirty), the original
//     src slice is returned verbatim — a store-and-forward switch hop pays ZERO
//     re-encode/allocation cost (the returned slice ALIASES src; see contract).
//   - otherwise only dirty fields re-encode; clean fields copy straight from src.
func (c *Codec) Marshal(m *Message, dst []byte) ([]byte, error)

// Validate runs every present field's validators plus required/MTI rules and
// returns ALL violations (non-fail-fast) as a *ValidationError, or nil.
func (c *Codec) Validate(m *Message) error

type CodecOptions struct {
	Immutable bool // Unmarshal yields a sealed (copy-on-write) Message
	Strict    bool // fail on unknown DEs / length overruns vs. lenient skip
}
type CodecOption func(*CodecOptions)
```

> **Marshal aliasing contract.** When `Marshal` returns the original `src` verbatim (clean fast path), the returned slice aliases the message's retained buffer. Callers that need an independent buffer must copy, or pass a non-overlapping `dst` and use `CodecOptions`/`Clone` semantics. This is documented prominently because it is the single most important performance lever for a store-and-forward switch.

---

## 3. The Dual Field API

Both APIs operate on the **same** `*Message`. Struct-binding is a thin, cached reflection layer over the same `Get`/`Set` machinery and the same `Value` zero-copy decode — there is no second model.

### 3.1 Dynamic map-style API

**Read (zero-copy, lazy):**

```go
c := iso8583.NewCodec(packager.ISO87A()) // profile = assembled Schema

msg, err := c.Unmarshal(wire) // structural only; no field bodies decoded yet
if err != nil { return err }

mti, _  := iso8583.Get[string](msg, 0)             // "0200"
pan, _  := iso8583.Get[string](msg, 2)             // decoded on demand from a src sub-slice
amt, _  := iso8583.Get[iso8583.Decimal](msg, 4)    // exact fixed-point money
icc, _  := iso8583.Get[[]byte](msg, "55.9F26")     // BER-TLV tag, still a zero-copy sub-slice

// Hot-path routing WITHOUT allocating (compare on the raw view, never String()):
if v, ok := msg.Get(3); ok && bytes.HasPrefix(v.Bytes(), []byte("00")) {
    // purchase
}

if msg.Has(38) { /* approval code present — bitmap check only, no decode */ }

for path, v := range msg.Fields() { // iter.Seq2, ascending
    log.Printf("%s = % x", path, v.Bytes())
}
```

**Write (copy-on-write or fresh build):**

```go
out := iso8583.New(packager.ISO87A())   // empty mutable message
out.Set(0, "0210")
out.Set(2, "4111111111111111")
out.Set(4, iso8583.NewDecimal(1099, 2)) // $10.99
out.SetS("55.9F26", emvCryptogram)      // nested BER-TLV by tag

wire, err := c.Marshal(out, buf[:0])    // bitmap auto-derived; reuses buf
```

**Edit-and-forward (copy-on-write):**

```go
resp := msg.Clone()        // shares src; no field bytes copied yet
resp.Set(0, "0210")        // flips owned=true
resp.Set(39, "00")         // only DE 39 allocates new bytes + marks dirty
resp.Unset(52)             // drop PIN block
out, _ := c.Marshal(resp, buf[:0]) // untouched fields copied straight from src; bitmap re-derived
```

### 3.2 Typed struct-binding API

A struct's `iso:"..."` tags use the **same `FieldPath` grammar**, so flat paths (`iso:"55.9F26"`) and nested structs both work and the developer chooses per use. A `Binder[T]` builds the reflection plan **once per type** and **validates every tag against the schema at construction time** (`NewBinder` fails fast on a tag that names a field the schema does not define or whose Kind is incompatible) — never per message.

```go
type AuthRequest struct {
	MTI    string          `iso:"0"`
	PAN    string          `iso:"2"`
	Proc   string          `iso:"3"`
	Amount iso8583.Decimal `iso:"4"`
	STAN   int64           `iso:"11"`
	Local  time.Time       `iso:"12,format=hhmmss"`
	Term   string          `iso:"41"`
	Track2 []byte          `iso:"35"`         // binds directly to a src sub-slice (zero copy)
	EMV    EMVData         `iso:"55"`         // composite -> nested struct
}

type EMVData struct {
	Cryptogram []byte `iso:"9F26"`            // addressed by BER-TLV tag inside DE 55
	ATC        int64  `iso:"9F36"`
	TVR        []byte `iso:"95"`
}

binder := iso8583.NewBinder[AuthRequest](schema) // builds + caches plan; validates tags now
```

**Read:**

```go
var req AuthRequest
if err := binder.Unmarshal(msg, &req); err != nil {
	var fe *iso8583.FieldError
	if errors.As(err, &fe) {
		log.Printf("bad field %s @byte %d: %v", fe.Path, fe.Offset, fe.Err)
	}
	return err
}
// req.Amount, req.EMV.Cryptogram are fully typed. []byte fields alias src (zero copy);
// only string/Decimal/time fields pay their unavoidable conversion.
```

**Write:**

```go
resp := AuthResponse{ MTI: "0210", PAN: req.PAN, Amount: req.Amount, RespCode: "00", STAN: req.STAN }
m, _ := binder.Marshal(&resp)        // *Message
wire, _ := c.Marshal(m, buf[:0])
```

```go
type Binder[T any] struct{ plan *bindPlan; schema *Schema }

func NewBinder[T any](s *Schema) *Binder[T]            // panics/returns error on tag↔schema mismatch
func (b *Binder[T]) Unmarshal(m *Message, dst *T) error // Message -> struct (lazy: only bound fields)
func (b *Binder[T]) Marshal(src *T) (*Message, error)   // struct -> Message

// Convenience one-shots on Codec that build+cache the binder internally:
func (c *Codec) Bind(wire []byte, dst any) error        // wire -> struct
func (c *Codec) Pack(src any) ([]byte, error)           // struct -> wire
```

A struct may be partial (bind only the DEs you care about); untagged exported fields are ignored. Missing-but-required schema fields surface as a structured `FieldError`.

---

## 4. Zero-Copy Decode

The cornerstone of the hot path. Three layers cooperate:

1. **Structural-only `Unmarshal`.** One pass reads the MTI and bitmap, then for each present DE records `slot{present:true, decoded:false}` with the field's byte span into `src`. **No transcoding, no parsing, no per-field allocation.** A switch that routes on DE 0/11/41 records ~35 spans but decodes 0 field bodies.

2. **Lazy, memoised field decode.** The first `Get(n)` slices `src[off:off+len]` into a `Value` whose `raw` **aliases** `src` (no copy), invokes the field's `FieldCodec.DecodeSpan` to validate/shape it, and memoises the result in the slot. Subsequent reads are free.

3. **`SpanCodec` fast path with graceful fallback.** A `FieldCodec` may implement the optional `SpanCodec` extension to return just the `[start,end)` byte span of a field **without decoding its body** — used by `Unmarshal` to find field boundaries cheaply. Codecs that cannot span (rare) fall back to full `Decode`. This is the most honest zero-copy mechanism: it does not force every codec to implement spanning.

```go
package fieldcodec

// FieldCodec encodes/decodes ONE field's BODY. The length prefix is handled
// separately by a LengthCodec (see §6), so codecs compose orthogonally.
type FieldCodec interface {
	// DecodeBody interprets body (already length-delimited by the engine) into a
	// Value. It MUST return sub-slices of body for raw/binary kinds (no copy); only
	// charset transcoding or digit parsing may allocate, and only lazily.
	DecodeBody(body []byte, def *iso8583.FieldDef) (iso8583.Value, error)

	// EncodeBody appends the wire body of v to dst and returns the grown slice
	// (append-style; reuses dst capacity).
	EncodeBody(dst []byte, v iso8583.Value, def *iso8583.FieldDef) ([]byte, error)

	// Kind is the natural Go kind this codec yields (drives binding + rendering).
	Kind() iso8583.Kind

	// Name is the registry key, e.g. "char.ascii", "num.bcd", "tlv.ber".
	Name() string
}

// SpanCodec is an OPTIONAL extension: codecs that can locate a field's body span
// in src without decoding it. The engine prefers Span; codecs that do not
// implement it fall back to DecodeBody during Unmarshal.
type SpanCodec interface {
	FieldCodec
	Span(body []byte, def *iso8583.FieldDef) (start, end int, err error)
}
```

**Why text cannot be truly zero-copy:** Go strings are immutable and cannot alias a `[]byte` without a copy. We are explicit about this: `Value.Bytes()` is the zero-copy primitive and the documented hot-path tool (`bytes.Equal`/`bytes.HasPrefix` routing); `Value.String()` is the *single, clearly-labelled* allocation (plus EBCDIC transcoding). `[]byte` struct-binding fields and `Get[[]byte]` are fully zero-copy.

**Copy-on-write & store-and-forward.** While read-only, all `Value`s alias the shared `src`; a decoded `Message` is published to many goroutines with no locks and no defensive copy. The first `Set`/`Unset` sets `owned=true`, allocates new bytes for *that field only*, and marks it `dirty`. `Marshal` returns `src` verbatim when `dirty` is empty (pure pass-through), and otherwise re-encodes only dirty fields while copying clean fields straight from `src`.

---

## 5. Schema & Validation Model

### 5.1 Build-time schema validation

`SchemaBuilder.Build` rejects a malformed schema before any message is processed: unknown/unregistered codec, overlapping subfield definitions, a `MaxLen` incompatible with the length codec, a composite without a `Sub` schema, or a BER-TLV path element used under a non-TLV field. This turns whole classes of configuration error into startup failures.

### 5.2 Structured errors with path + byte offset

Every error carries the uniform `FieldPath` and, where known, the absolute byte offset into the wire buffer — invaluable for an operator staring at a hex dump.

```go
package iso8583

// FieldError: a single field could not be decoded or typed.
type FieldError struct {
	Path   FieldPath // "55.9F26"
	Offset int       // absolute byte offset in the source buffer, or -1
	Name   string    // "Primary Account Number"
	Err    error     // underlying cause (luhn failed, truncated, length, type mismatch)
}
func (e *FieldError) Error() string // "field 55.9F26 @byte 214: luhn check failed"
func (e *FieldError) Unwrap() error { return e.Err }

// Violation: a single semantic/validation failure (from Validate).
type Violation struct {
	Path   FieldPath
	Offset int
	Rule   string // "luhn", "len", "required", "currency", "mti"
	Msg    string
}

// ValidationError aggregates ALL violations from one Validate pass (non-fail-fast)
// so a declined message reports every problem at once with precise locations.
type ValidationError struct{ Violations []Violation }
func (e *ValidationError) Error() string
func (e *ValidationError) Unwrap() []error // each Violation as an error, for errors.Is/As
```

### 5.3 Validators

```go
package iso8583

type Validator interface {
	Validate(v Value, def *FieldDef) *Violation // nil == ok
}
type ValidatorFunc func(Value, *FieldDef) *Violation
func (f ValidatorFunc) Validate(v Value, d *FieldDef) *Violation { return f(v, d) }

// Built-in, pure, composable rule combinators:
func Required() Validator
func Digits() Validator
func Luhn() Validator                 // PAN check digit (DE 2)
func LenBetween(min, max int) Validator
func MaxLength(n int) Validator
func OneOf(allowed ...string) Validator
func Regexp(pattern string) Validator
func CurrencyAmount() Validator       // DE 4 well-formed vs DE 49
func MTIClass(allowed ...string) Validator
func All(vs ...Validator) Validator   // AND-combine
```

`Codec.Validate(m)` runs every present field's `Validate` rules plus `Required`/`RequiredOn(MTI)` rules and any schema-level cross-field rules, collecting **all** `Violation`s. This exhaustive, non-fail-fast behaviour is what makes Isopace fuzz/conformance-friendly and lets a switch emit precise ISO response/reason codes for every malformed field in a single pass.

---

## 6. FieldCodec / LengthCodec Registry & Catalog

### 6.1 Two orthogonal codec axes

The defining structural improvement over jPOS: **length handling is a separate, independently-registered `LengthCodec`, composed orthogonally with the value `FieldCodec`.** A field's wire behaviour is the *pair* `(LengthCodec, FieldCodec)`, not a monolithic class. This collapses jPOS's combinatorial `IF*` zoo (`IFA_LLLNUM`, `IFB_LLLBINARY`, `IFE_LLLCHAR`, …) into ≈6 length codecs × ≈12 value codecs that compose freely.

```go
package lengthcodec

// LengthCodec handles the length prefix (fixed / LL / LLL / LLLL) in a chosen
// representation (ASCII / BCD / Binary / EBCDIC). Stateless singleton.
type LengthCodec interface {
	// ReadLen consumes the length prefix from src at off, returning the number of
	// BODY bytes that follow and the offset just past the prefix.
	ReadLen(src []byte, off int, def *iso8583.FieldDef) (bodyLen, next int, err error)
	// WriteLen appends a length prefix for a body of n bytes.
	WriteLen(dst []byte, n int, def *iso8583.FieldDef) ([]byte, error)
	Name() string
}
```

### 6.2 Registries (explicit population — no init() side effects)

```go
package fieldcodec

type Registry struct {
	values  map[string]FieldCodec
	lengths map[string]lengthcodec.LengthCodec
	bitmaps map[string]BitmapCodec
}

func (r *Registry) Register(c FieldCodec)
func (r *Registry) RegisterLength(l lengthcodec.LengthCodec)
func (r *Registry) RegisterBitmap(b BitmapCodec)
func (r *Registry) Lookup(name string) (FieldCodec, bool)
func (r *Registry) LookupLength(name string) (lengthcodec.LengthCodec, bool)

// DefaultRegistry returns a freshly-built registry pre-populated with the FULL
// catalog below. Explicit construction (not import-for-side-effects) keeps global
// state visible and avoids import-ordering surprises. Callers may add custom codecs.
func DefaultRegistry() *Registry
```

Custom codecs (e.g. a scheme-specific date) are added by name and referenced from schemas/YAML — no fork of the engine:

```go
reg := fieldcodec.DefaultRegistry()
reg.Register(myMMDDHHMMSS{}) // Name() == "date.mmddhhmmss.ascii"
```

### 6.3 Catalog — jPOS `IF*` interpreters → Isopace codecs

Isopace splits each jPOS `IF*` class into its two orthogonal parts: a **value `FieldCodec`** (charset/representation) and a **`LengthCodec`** (prefix). The table shows the mapping. "—" in the LengthCodec column means the jPOS class is itself a value interpreter with no length semantics (length comes from a separate `IF*LLNUM`/`IF*LLLCHAR` wrapper, which here is simply a different `LengthCodec`).

#### Value FieldCodecs (charset / numeric / binary)

| jPOS interpreter (representative) | Isopace `FieldCodec` | Registry name | Kind | Notes |
|---|---|---|---|---|
| `IFA_CHAR`, `IF_CHAR` (ASCII char) | `ASCII` | `char.ascii` | String | ASCII text |
| `IFE_CHAR` (EBCDIC char) | `EBCDIC037` | `char.ebcdic.cp037` | String | CP037 table |
| `IFEB_CHAR` / CP1047 usage | `EBCDIC1047` | `char.ebcdic.cp1047` | String | CP1047 table |
| (UTF-8 text, jPOS lacks) | `UTF8` | `char.utf8` | String | extra |
| `IFB_BINARY`, `IF_ECHO`/raw | `BINARY` | `b.raw` | Bytes | zero-copy sub-slice |
| `IFA_BINARY` (hex-text binary) | `HexBINARY` | `b.hex` | Bytes | octets as ASCII hex (2 chars/octet) |
| `IFA_NUMERIC` (ASCII digits) | `NumASCII` | `num.ascii` | Numeric | digit string |
| `IFE_NUMERIC` (EBCDIC digits) | `NumEBCDIC` | `num.ebcdic` | Numeric | EBCDIC digits |
| `IFB_NUMERIC` (BCD / packed) | `BCD` | `num.bcd` | Numeric | 2 digits/byte, left-aligned |
| `IFB_NUMERIC` right-aligned (rBCD) | `RBCD` | `num.rbcd` | Numeric | right-aligned packed |
| `IFB_BINARY` numeric / binary int | `NumBinary` | `num.bin` | Numeric | big-endian integer |
| `IFA_AMOUNT`, `IF_AMOUNT` | `AmountASCII` | `amount.ascii` | Amount | signed, → `Decimal` |
| `IFB_AMOUNT` (packed amount) | `AmountBCD` | `amount.bcd` | Amount | → `Decimal` |
| (binary amount, extra) | `AmountBinary` | `amount.bin` | Amount | → `Decimal` |

#### Bitmap codecs

| jPOS interpreter | Isopace `BitmapCodec` | Registry name | Notes |
|---|---|---|---|
| `IFB_BITMAP` (binary bitmap) | `BitmapBinary` | `bitmap.bin` | 8/16/24 bytes, continuation-bit driven |
| `IFA_BITMAP` (ASCII-hex bitmap) | `BitmapHex` | `bitmap.hex` | 16/32/48 hex chars |
| `IFE_BITMAP` (EBCDIC-hex bitmap) | `BitmapEBCDIC` | `bitmap.ebcdic` | EBCDIC hex |

#### Length-prefix codecs (the orthogonal axis)

| jPOS prefix style (embedded in `IF*LL*`) | Isopace `LengthCodec` | Registry name | Notes |
|---|---|---|---|
| fixed length | `Fixed` | `len.fixed` | length from `FieldDef.MaxLen` |
| `LLNUM`/`LLCHAR` ASCII 2-digit | `LLVarASCII` | `len.ll.ascii` | LLVAR |
| `LLLNUM`/`LLLCHAR` ASCII 3-digit | `LLLVarASCII` | `len.lll.ascii` | LLLVAR |
| 4-digit ASCII (LLLLVAR) | `LLLLVarASCII` | `len.llll.ascii` | LLLLVAR |
| 5-digit ASCII (LLLLLVAR) | `LLLLLVarASCII` | `len.lllll.ascii` | LLLLLVAR |
| 6-digit ASCII (LLLLLLVAR) | `LLLLLLVarASCII` | `len.llllll.ascii` | LLLLLLVAR (e.g. DE 127 container) |
| `IFB_LLNUM` BCD 2-digit prefix | `LLVarBCD` | `len.ll.bcd` | packed length |
| `IFB_LLLNUM` BCD 3-digit prefix | `LLLVarBCD` | `len.lll.bcd` | packed length |
| binary 1-byte length | `LLVarBinary` | `len.ll.bin` | binary count |
| binary 2-byte length | `LLLVarBinary` | `len.lll.bin` | binary count |
| `IFE_LLNUM` EBCDIC 2-digit | `LLVarEBCDIC` | `len.ll.ebcdic` | EBCDIC length |
| `IFE_LLLNUM` EBCDIC 3-digit | `LLLVarEBCDIC` | `len.lll.ebcdic` | EBCDIC length |

**Composition example.** jPOS `IFB_LLLNUM` (BCD digits, BCD 3-digit prefix) = `(value: BCD, length: LLLVarBCD)`. jPOS `IFE_LLLCHAR` (EBCDIC char, EBCDIC 3-digit prefix) = `(value: EBCDIC037, length: LLLVarEBCDIC)`. Neither needs a bespoke class.

#### Composite & extended formats

| Capability | Isopace component | Registry name | Notes |
|---|---|---|---|
| Subfield group (bitmap + positional subfields, e.g. DE 127) | `subfield.Packager` + headerless `Sub *Schema` | `subfield.iso` | nested sub-bitmap; `127.2` addressing |
| EMV BER-TLV (DE 55) | `bertlv.Codec` | `tlv.ber` | hex-tag addressing (`55.9F26`); recursive |
| Visa Base I dialect | `packager/visa` (schema overlay) | — | `Derive`/`Override` on iso93 |
| Mastercard dialect | `packager/mastercard` (overlay) | — | DE 48/124 overrides |
| JSON rendering | `render/jsonio` | — | consumes `View` |
| protobuf rendering | `render/protobuf` | — | descriptor-driven, consumes `View` |
| ISO 20022 bridge | `render/iso20022` | — | DE→`pacs/pain/camt` map, consumes `View` |

---

## 7. Packager Profiles

A profile is **just an assembled, immutable `*Schema`** — there is no special "packager" type, which keeps profiles composable. Each profile wires `FieldDef`s from the two codec axes; scheme dialects are deltas via `Derive`/`Override`.

### 7.1 ISO 8583:1987 — variants A / B / C

| Variant | Charset / value codecs | Length prefixes | Bitmap | Typical use |
|---|---|---|---|---|
| **A** (ASCII) | `char.ascii`, `num.ascii`, `amount.ascii` | `len.*.ascii` | `bitmap.hex` | text-oriented links |
| **B** (binary/BCD) | `num.bcd`, `b.raw`, `amount.bcd` | `len.*.bcd`, `len.fixed` | `bitmap.bin` | compact binary links |
| **C** (EBCDIC) | `char.ebcdic.cp037`, `num.ebcdic` | `len.*.ebcdic` | `bitmap.ebcdic` | mainframe/EBCDIC hosts |

```go
package packager

func ISO87A() *iso8583.Schema {
	reg := fieldcodec.DefaultRegistry()
	return iso8583.NewSchema("iso87-a").
		MTI(must(reg, "num.ascii")).
		Bitmap(iso8583.BitmapSpec{Codec: mustBM(reg, "bitmap.hex"), Levels: 2}).
		Field(2,  "PAN",       fc(reg, "num.ascii"),    ln(reg, "len.ll.ascii"),  iso8583.MaxLen(19), iso8583.Validate(iso8583.Luhn())).
		Field(3,  "Proc Code", fc(reg, "num.ascii"),    ln(reg, "len.fixed"),     iso8583.MaxLen(6)).
		Field(4,  "Amount",    fc(reg, "amount.ascii"), ln(reg, "len.fixed"),     iso8583.MaxLen(12), iso8583.Validate(iso8583.CurrencyAmount())).
		Field(11, "STAN",      fc(reg, "num.ascii"),    ln(reg, "len.fixed"),     iso8583.MaxLen(6)).
		Field(35, "Track2",    fc(reg, "b.raw"),        ln(reg, "len.ll.ascii"),  iso8583.MaxLen(37)).
		Composite(55, "EMV", emvSchema(reg), ln(reg, "len.lll.ascii")).
		MustBuild(reg)
}

// Variant B reuses the SAME field defs, swapping only the codec/length/bitmap axes:
func ISO87B() *iso8583.Schema {
	reg := fieldcodec.DefaultRegistry()
	return ISO87A().Derive("iso87-b").
		Bitmap(iso8583.BitmapSpec{Codec: mustBM(reg, "bitmap.bin"), Levels: 2}).
		Override(2,  withCodec(reg, "num.bcd"),    withLength(reg, "len.ll.bcd")).
		Override(4,  withCodec(reg, "amount.bcd")).
		Override(11, withCodec(reg, "num.bcd")).
		MustBuild(reg)
}

func emvSchema(reg *fieldcodec.Registry) *iso8583.Schema {
	return iso8583.NewSchema("emv-55").
		Bitmap(iso8583.BitmapSpec{Codec: mustBM(reg, "tlv.ber.index")}). // tag-addressed
		Tag("9F26", "App Cryptogram", fc(reg, "b.raw")).
		Tag("9F36", "ATC",            fc(reg, "num.bin")).
		Tag("95",   "TVR",            fc(reg, "b.raw")).
		MustBuild(reg)
}
```

### 7.2 ISO 8583:1993 — variants A / B

| Variant | Value codecs | Length prefixes | Bitmap | Notes |
|---|---|---|---|---|
| **A** (ASCII) | `char.ascii`, `num.ascii` | `len.*.ascii` | `bitmap.hex` | adds DE structure changes vs 87 (response codes, etc.) |
| **B** (binary) | `num.bcd`, `b.raw` | `len.*.bcd` | `bitmap.bin` | compact 1993 |

`iso93` schemas are independent definitions (the 1993 DE catalogue differs from 1987), built the same way; they are the base for card-scheme overlays.

### 7.3 Card-scheme overlays (deltas)

```go
// Visa Base I = iso93 variant A with field 62/63 redefined.
func VisaBaseI() *iso8583.Schema {
	reg := fieldcodec.DefaultRegistry()
	return ISO93A().Derive("visa-base1").
		Override(62, withComposite(visaF62(reg)), withLength(reg, "len.lll.ascii")).
		Override(63, withComposite(visaF63(reg)), withLength(reg, "len.lll.ascii")).
		MustBuild(reg)
}
```

### 7.4 Declarative profiles (YAML, codec-by-name)

The same composition is expressible as config for ops teams; `packager/generic` resolves each `codec:`/`length:` name against the registry, producing an *identical* `*Schema`. Loaded via `go:embed` from `schemadef/`.

```yaml
id: iso87-b
mti: num.bcd
bitmap: { codec: bitmap.bin, levels: 2 }
fields:
  2:  { name: PAN,    codec: num.bcd,    length: len.ll.bcd,  max: 19, validate: [luhn] }
  4:  { name: Amount, codec: amount.bcd, length: len.fixed,   max: 12, validate: [currencyAmount] }
  55: { name: EMV,    composite: emv-55, length: len.lll.bcd }
```

```go
schema, err := generic.LoadFile("schemadef/iso87b.yaml", fieldcodec.DefaultRegistry())
```

### 7.5 Concrete site packager (`postilion`)

Unlike the representation-independent `iso87`/`iso93` directories (which swap whole
codec sets via a `rep`), a deployment often pins each field to one concrete codec
and length discipline. `packager.Postilion()` is such a site profile: an ISO
8583:1987 Postilion layout (ASCII MTI, **binary** primary+secondary bitmap, ASCII
fixed and LL/LLL variable fields) — a table of `(DE, name, value, length, max)`
rows over the two orthogonal axes. Its **DE 127** is a nested reserved-private
subfield group — a headerless **binary** sub-bitmap plus positional subfields
under a 6-digit length prefix, wired with the `subfield` composite codec — so
`127.2`, `127.22`, … are addressable under the uniform path grammar, just like
`55.9F26`. The profile was validated end-to-end against a live switch (a `0800`
sign-on answered `0810`, and a `0200` cashout answered `0210`); on the wire such
links use a 2-byte length prefix and a network sign-on, both handled by the
`link` layer rather than the schema.

---

## 8. Alternate Renderings (View)

Because the model is decoupled from the wire codec, rendering to another format is a visitor over the schema-typed `Value` tree — it never re-parses the wire. Every renderer consumes one read-only interface, so adding a format never touches the core or any codec.

```go
package iso8583

// View is the read-only projection renderers consume. *Message implements it.
type View interface {
	Schema() *Schema
	Bitmap() Bitmap
	Fields() iter.Seq2[FieldPath, Value]  // ascending, present fields only
	Get(de int) (Value, bool)
}
```

```go
package jsonio

// Marshal renders a Message to JSON using schema names as keys and Kind to choose
// the JSON shape. Numerics stay strings to preserve leading zeros and amount scale.
func Marshal(v iso8583.View, opts ...Option) ([]byte, error)
func Unmarshal(data []byte, s *iso8583.Schema) (*iso8583.Message, error)

type Option func(*options) // UseNames(), MaskPAN(), etc.
```

```go
b, _ := jsonio.Marshal(msg, jsonio.UseNames(), jsonio.MaskPAN())
```

```json
{
  "mti": "0200",
  "fields": {
    "2":  { "name": "PAN",    "value": "411111******1111" },
    "4":  { "name": "Amount", "value": { "currency": "840", "scale": 2, "unscaled": 1099 } },
    "55": { "name": "EMV", "tlv": {
        "9F26": "A1B2C3D4E5F60718",
        "9F36": 42
    }}
  }
}
```

The **same `*Message`** round-trips through every backend: `Codec.Marshal` → ISO-8583 wire (any profile), `jsonio.Marshal` → JSON, `protobuf.Marshal` → protobuf (DE→field-number via descriptor), `iso20022.To` → ISO 20022 (DE→`pacs.008`/`pain.001`/`camt.05x` via a declarative `map[FieldPath]xpath` table). No re-modelling, no second source of truth. PAN masking, amount formatting, and tag naming are render options, not baked into the model.

---

## 9. How Isopace differs from jPOS

> Isopace is an independent, clean-room implementation and is **not affiliated
> with, endorsed by, or derived from** the jPOS project. "jPOS" is a trademark of
> its respective owner; the comparisons below are technical and offered for
> design and interoperability context only.

**1. Type-safe access instead of a stringly-typed mutable `Map<Integer,ISOComponent>`.**
jPOS hands you `msg.getString(4)` (a `String` you must parse) and `getComponent(4)` (an `ISOComponent` you downcast), with mistakes surfacing as runtime `ClassCastException`/`NumberFormatException`. Isopace gives `Get[iso8583.Decimal](m, 4)` and tag-bound structs (`Amount iso8583.Decimal \`iso:"4"\``): the type is known at compile time, money is exact fixed-point (never a float or a re-parsed string), and a mismatch is a generic-constraint error or a structured `FieldError` carrying `Path` + `Offset`. The 90% developer never casts.

**2. Zero-copy lazy decode vs eager full-message unpack.**
jPOS `unpack()` eagerly decodes every present field into an `ISOField`/`String` and stuffs them into a map — O(all fields) allocations regardless of what you read. Isopace's `Unmarshal` walks only MTI + bitmap and records `(offset,len)` spans aliasing the original buffer; a field decodes on first `Get` and memoises, `Value.Bytes()` is a true zero-copy view, and `[]byte` struct fields bind with no copy at all. A high-TPS switch routing on 4 of ~35 fields skips decoding the rest, and `Marshal` returns the original buffer **verbatim** when nothing is dirty — a store-and-forward hop is allocation-free end to end.

**3. Composable `(LengthCodec × FieldCodec)` instead of a combinatorial `IF*` class zoo.**
jPOS encodes every length×encoding combination as its own packager class (`IFA_LLLNUM`, `IFB_LLLBINARY`, `IFE_LLLCHAR`, …). Isopace makes the length prefix and the value codec orthogonal, independently-registered components, so ≈6 length × ≈12 value codecs compose into the full catalogue with zero bespoke classes — all resolvable by name from Go, YAML, or a derived profile. Adding EBCDIC to a numeric field is swapping one codec, not authoring a new class.

**4. Structural correctness invariants jPOS lacks.**
The bitmap is **always derived** from present fields at marshal time, so the classic jPOS bitmap/content desync footgun is impossible. `Message` is immutable with copy-on-write, so a decoded request is safely shared across the `Switch`, `Flow` stages, and async logging with zero locking and no defensive copies. `Validate()` is **exhaustive** — it returns *all* `Violation`s in one pass, each with `Path`, `Offset`, and `Rule`, versus jPOS's first-failure `ISOException` with an opaque string — which is exactly what makes Isopace fuzz/conformance-friendly and lets a switch emit precise ISO response codes. The `Schema` and every struct-binding tag are validated at build/init time, turning config mistakes into startup failures.

**5. One model, many renderings — clean model/schema/codec separation.**
In jPOS the packager *is* the format and `ISOMsg` is hard-wired to ISO-8583 wire; producing JSON or another encoding means bespoke traversal per format with no single immutable schema object. Isopace splits `Message` (model) / `Schema` (definitions) / `FieldCodec`+`LengthCodec` (wire), and every renderer consumes one read-only `View`, so the identical `*Message` projects losslessly to ISO-8583 (any profile), JSON, protobuf, and ISO 20022 — and a new acquirer dialect is a `generic.LoadFile` config or a `Derive`/`Override` overlay, not a new packager class.

**6. First-class nested + BER-TLV addressing under one grammar.**
Subfields (DE 48/60/63) and EMV DE 55 are addressed by a single uniform `FieldPath` grammar (`"55.9F26"`, with hex BER-TLV tags as first-class path elements) used identically in the dynamic API, struct tags, validation errors, and JSON keys — versus jPOS's separate sub-`ISOMsg` packagers and manual TLV handling.

---

## 10. Implementation Corrections (post-synthesis review)

The synthesis is sound, but four items in §2–§4 have compile-level defects. The
corrected forms below are **authoritative** for implementation and override the
earlier sections where they conflict.

**C1 — Codec contracts live in `iso8583`; `fieldcodec`/`lengthcodec` only implement them (import-cycle fix).**
As written, `iso8583` references `fieldcodec.FieldCodec` (§2.2, §2.6) while
`fieldcodec.FieldCodec`'s methods take `iso8583.Value` / `*iso8583.FieldDef` (§4) —
a cyclic import. Resolution (this is what the §1 layering prose actually intends):

- Declare the **interfaces** `FieldCodec`, `LengthCodec`, `BitmapCodec` in
  `iso8583`. Therefore `Value.codec`, `FieldDef.Codec`, `FieldDef.Length`, and
  `BitmapSpec.Codec` are the local `iso8583.*` interface types.
- `fieldcodec/` and `lengthcodec/` `import iso8583` and provide the concrete
  **implementations**, the `Registry`, and `DefaultRegistry()`.
- `SchemaBuilder.Field` already receives resolved codec *instances*, so `Build` /
  `MustBuild` take **no** registry argument (`Build() (*Schema, error)`); the
  `Registry` is used only in `packager/` to resolve codec *names* → instances.
  Thus `iso8583` never imports `fieldcodec`, and the dependency graph is acyclic.

**C2 — `Get[T]` uses a union constraint, not a sealed `isFieldType()` interface.**
`FieldType interface{ isFieldType() }` (§2.5) makes `Get[string](m, 2)`
uncompilable — a built-in `string` cannot carry the method. The §2.5 design note's
premise is also wrong: a Go union constraint *may* mix `~`-approximation terms with
exact named-struct terms. Use:

```go
type FieldType interface {
    ~string | ~int64 | ~uint64 | []byte | Decimal | Amount | time.Time | Bitmap | *Message
}
```

(`[]byte` is an exact term; `~[]byte` is invalid because `[]byte` is unnamed.)
Dispatch inside `Get[T]` via `switch any(*new(T)).(type)`. Ergonomic
`Get[string]` / `Get[int64]` / `Get[[]byte]` / `Get[Decimal]` then compile.

**C3 — Replace the `Pather` union with explicit overloads.**
`Pather interface{ ~int | ~string | path() }` is not legal: a union term cannot be
a method, and a union/`~` interface cannot be an ordinary parameter type, so
`Has(p Pather)` / `Unset(p Pather)` will not compile. Drop `Pather`; mirror the
existing read overloads wherever a path is accepted: `Has` / `HasS` / `HasP`,
`Unset` / `UnsetS` / `UnsetP` (alongside the existing `Get`/`GetS`/`GetP` and
`Set`/`SetS`/`SetP`).

**C4 — Add `SchemaBuilder.Tag` for BER-TLV composites.**
§7.1's `emvSchema` calls `.Tag(...)` (and a `tlv.ber.index` bitmap) that §2.6 does
not declare. Add:

```go
func (b *SchemaBuilder) Tag(tag, name string, value FieldCodec, opts ...FieldOpt) *SchemaBuilder
```

A TLV composite addresses children by canonical hex tag (presence by tag, not a
positional bitmap).
