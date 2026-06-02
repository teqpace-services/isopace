// SPDX-License-Identifier: AGPL-3.0-or-later
//
// Copyright (C) 2026 Teqpace Services Ltd.
//
// This file is part of Isopace, a financial transaction framework.
//
// Isopace is dual-licensed:
//   - under the GNU Affero General Public License v3.0 or later (see LICENSE); or
//   - under a commercial license from Teqpace Services Ltd. (see COMMERCIAL-LICENSE.md).
//
// Authorship is recorded in the AUTHORS file.

package iso8583

import (
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
)

// This file is the built-in diagnostic renderer: a schema-aware, one-call dump
// of a message for logs and the playground. It reads only the View, so it never
// re-parses the wire and never touches a codec.
//
// Because Isopace carries cardholder data, the renderer masks PCI-sensitive
// fields (PAN, track data, PIN) BY DEFAULT. Showing them in full is an explicit,
// auditable opt-in via Unmasked — the safe default is to never leak a primary
// account number into a log file.

// maskKind classifies how a field's value is redacted in diagnostic output.
type maskKind uint8

const (
	maskNone maskKind = iota // shown verbatim
	maskPAN                  // keep leading six + trailing four digits
	maskFull                 // value replaced entirely; only its size survives
)

// sensitiveDE maps a top-level data element to its default redaction. These are
// the PCI-DSS "sensitive authentication data" / PAN carriers in the ISO 8583
// directory; the set is representation-independent (it keys on DE number, which
// every variant shares).
var sensitiveDE = map[int]maskKind{
	2:  maskPAN,  // Primary Account Number
	35: maskFull, // Track 2 Data
	36: maskFull, // Track 3 Data
	45: maskFull, // Track 1 Data
	52: maskFull, // PIN Data (PIN block)
}

// sensitiveTag maps EMV (DE 55) BER-TLV tags to their default redaction, so a
// PAN or track-equivalent buried in ICC data is masked just like the top-level
// field.
var sensitiveTag = map[string]maskKind{
	"5A": maskPAN,  // Application PAN
	"57": maskFull, // Track 2 Equivalent Data
	"99": maskFull, // Transaction PIN Data
}

// describeConfig holds resolved render options.
type describeConfig struct {
	unmask bool   // show sensitive fields in full
	raw    bool   // append a raw-hex column
	color  bool   // ANSI colour the output
	title  string // header title override ("" = default)
}

// DescribeOption configures Describe / Dump / LogValuer. Options compose in the
// same functional style as the render packages.
type DescribeOption func(*describeConfig)

// Unmasked shows PCI-sensitive fields (PAN, track data, PIN) in full. Use it
// only for local debugging of test data — never on a wire from a real card.
func Unmasked() DescribeOption { return func(c *describeConfig) { c.unmask = true } }

// WithRaw appends the raw canonical bytes (hex) of each scalar field as an extra
// column, for byte-level debugging. Sensitive fields stay masked unless Unmasked
// is also set.
func WithRaw() DescribeOption { return func(c *describeConfig) { c.raw = true } }

// WithTitle overrides the header title line (default "ISO 8583 message").
func WithTitle(s string) DescribeOption { return func(c *describeConfig) { c.title = s } }

// WithColor renders the dump with ANSI colour: a bold title, cyan DE keys, dim
// structural labels, and green field values. Off by default so piped output
// stays clean; enable it for a terminal.
func WithColor() DescribeOption { return func(c *describeConfig) { c.color = true } }

// ANSI styles used when WithColor is set.
const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiDim   = "\x1b[2m"
	ansiCyan  = "\x1b[36m"
	ansiGreen = "\x1b[32m"
)

// paint wraps s in code when colour is enabled (the padding stays outside, since
// callers pad the visible text before painting to preserve column alignment).
func (c *describeConfig) paint(code, s string) string {
	if !c.color {
		return s
	}
	return code + s + ansiReset
}

func resolve(opts []DescribeOption) describeConfig {
	c := describeConfig{title: "ISO 8583 message"}
	for _, o := range opts {
		o(&c)
	}
	return c
}

// Describe writes a human-readable, schema-aware dump of v to w: the profile,
// MTI, bitmap, and every present field with its DE number, name, and decoded
// value. Composite fields (subfields, BER-TLV) are recursed and indented.
// Sensitive fields are masked unless Unmasked is supplied.
//
//	iso8583.Describe(os.Stdout, msg)               // safe: PAN masked
//	iso8583.Describe(os.Stdout, msg, iso8583.Unmasked(), iso8583.WithRaw())
func Describe(w io.Writer, v View, opts ...DescribeOption) error {
	cfg := resolve(opts)
	rows := collect(v, &cfg)

	// Column widths: key (DE / path) and name, padding the name to align the
	// '=' across every row including indented children.
	keyW, nameW := len("DE"), 0
	for _, r := range rows {
		if l := len(r.key); l > keyW {
			keyW = l
		}
		if l := r.depth*2 + displayWidth(r.name); l > nameW {
			nameW = l
		}
	}
	if nameW > maxNameWidth {
		nameW = maxNameWidth
	}

	bw := newErrWriter(w)
	bm := v.Bitmap()
	fmt.Fprintf(bw, "%s · profile %s\n", cfg.paint(ansiBold, cfg.title), cfg.paint(ansiDim, schemaID(v)))
	if mti, ok := v.Get(0); ok {
		if s, err := mti.String(); err == nil {
			fmt.Fprintf(bw, "  MTI     : %s\n", cfg.paint(ansiBold, s))
		}
	}
	fmt.Fprintf(bw, "  bitmap  : %s  %s\n", bitmapHex(bm), cfg.paint(ansiDim, fmt.Sprintf("(%d field(s) · %s)", bm.Count(), bm)))
	fmt.Fprintf(bw, "  %s\n", cfg.paint(ansiDim, strings.Repeat("─", keyW+nameW+6)))
	for _, r := range rows {
		name := strings.Repeat("  ", r.depth) + r.name
		name = truncate(name, nameW)
		// Pad the visible text first, then colour, so columns stay aligned.
		key := cfg.paint(ansiCyan, fmt.Sprintf("%-*s", keyW, r.key))
		fmt.Fprintf(bw, "  %s  %-*s = %s", key, nameW, name, cfg.paint(ansiGreen, r.value))
		if cfg.raw && r.raw != "" {
			fmt.Fprintf(bw, "  raw=%s", cfg.paint(ansiDim, r.raw))
		}
		fmt.Fprintln(bw)
	}
	return bw.err
}

// Dump returns Describe's output as a string. Any write error (impossible for a
// strings.Builder) is folded into the returned text.
func Dump(v View, opts ...DescribeOption) string {
	var b strings.Builder
	if err := Describe(&b, v, opts...); err != nil {
		fmt.Fprintf(&b, "\n[describe error: %v]\n", err)
	}
	return b.String()
}

// maxNameWidth caps the field-name column so a pathological schema name cannot
// blow out the layout.
const maxNameWidth = 40

// descRow is one rendered line: the key column (DE number or dotted path), the
// schema name, the display value, and nesting depth for indentation.
type descRow struct {
	key   string
	name  string
	value string
	raw   string // raw-hex of the canonical bytes (scalars only)
	depth int
}

// collect flattens a message into render rows, recursing into composites.
func collect(v View, cfg *describeConfig) []descRow {
	var rows []descRow
	s := v.Schema()
	for p, val := range v.Fields() {
		de := int(p.Head().N)
		appendRow(&rows, cfg, p.String(), fieldName(s, de), de2mask(de, cfg), val, 0)
	}
	return rows
}

// appendRow renders one field (recursing if composite) into rows. mk is the
// resolved redaction for this field's own value (ignored for composites).
func appendRow(rows *[]descRow, cfg *describeConfig, key, name string, mk maskKind, val Value, depth int) {
	if child, ok := val.Composite(); ok {
		*rows = append(*rows, descRow{key: key, name: name, value: compositeSummary(child), depth: depth})
		appendChildren(rows, cfg, key, child, depth+1)
		return
	}
	// The raw-hex column must honour masking: dumping the bytes of a masked PAN
	// would defeat the redaction. Only unmasked fields expose their raw bytes.
	raw := "[redacted]"
	if mk == maskNone {
		raw = hexUpper(val.Bytes())
	}
	*rows = append(*rows, descRow{
		key:   key,
		name:  name,
		value: display(val, mk, cfg),
		raw:   raw,
		depth: depth,
	})
}

// appendChildren renders a composite's children: BER-TLV tags for a TLV
// sub-schema, otherwise positional subfields.
func appendChildren(rows *[]descRow, cfg *describeConfig, prefix string, child *Message, depth int) {
	s := child.Schema()
	if s != nil && s.IsTLV() {
		for tag, val := range child.TagSeq() {
			def, _ := s.LookupTag(tag)
			appendRow(rows, cfg, prefix+"."+tag, defName(def), tag2mask(tag, cfg), val, depth)
		}
		return
	}
	for p, val := range child.Fields() {
		de := int(p.Head().N)
		appendRow(rows, cfg, prefix+"."+p.String(), fieldName(s, de), maskNone, val, depth)
	}
}

// de2mask resolves the redaction for a top-level DE, honouring Unmasked.
func de2mask(de int, cfg *describeConfig) maskKind {
	if cfg.unmask {
		return maskNone
	}
	return sensitiveDE[de]
}

func tag2mask(tag string, cfg *describeConfig) maskKind {
	if cfg.unmask {
		return maskNone
	}
	return sensitiveTag[strings.ToUpper(tag)]
}

// display renders a scalar value's canonical form for a human, applying the
// redaction mk.
func display(v Value, mk maskKind, cfg *describeConfig) string {
	switch mk {
	case maskPAN:
		if s, err := v.String(); err == nil {
			return maskPANDigits(s)
		}
		return redacted(v)
	case maskFull:
		return redacted(v)
	}
	switch v.Kind() {
	case KindAmount:
		d, err := v.Decimal()
		if err != nil {
			return "?"
		}
		return d.String()
	case KindBytes:
		return hexUpper(v.Bytes())
	case KindComposite:
		return compositeSummary(compositeOf(v))
	case KindString:
		// Quote text so fixed-width space padding and stray control bytes are
		// visible; digits and amounts read better bare.
		s, err := v.String()
		if err != nil {
			return hexUpper(v.Bytes())
		}
		return strconv.Quote(s)
	default:
		s, err := v.String()
		if err != nil {
			return hexUpper(v.Bytes())
		}
		return s
	}
}

func compositeOf(v Value) *Message {
	c, _ := v.Composite()
	return c
}

// compositeSummary describes a composite parent line without revealing values.
func compositeSummary(child *Message) string {
	if child == nil {
		return "{}"
	}
	n := 0
	s := child.Schema()
	if s != nil && s.IsTLV() {
		for range child.TagSeq() {
			n++
		}
	} else {
		for range child.Fields() {
			n++
		}
	}
	return fmt.Sprintf("{%d element(s)}", n)
}

// redacted renders a fully-masked value, preserving only its size so the log
// still shows the field was present and how big it was.
func redacted(v Value) string {
	return fmt.Sprintf("[redacted %dB]", len(v.Bytes()))
}

// --- slog integration ---

// LogValue implements slog.LogValuer: a Message logs as a structured group of
// schema, MTI, and each present field, with PCI-sensitive fields masked. This
// is the safe default for production logging —
//
//	logger.Info("auth request", "iso", msg)
//
// emits the message as grouped, redacted attributes. For full (unmasked) field
// values in a controlled context, build a value explicitly with LogValuer.
func (m *Message) LogValue() slog.Value { return logValue(m, &describeConfig{}) }

// LogValuer wraps a View as an slog.LogValuer with explicit options, e.g. to
// include raw bytes or (deliberately) unmask a debug build:
//
//	logger.Debug("decoded", "iso", iso8583.LogValuer(msg, iso8583.Unmasked()))
func LogValuer(v View, opts ...DescribeOption) slog.LogValuer {
	cfg := resolve(opts)
	return logValuerFunc(func() slog.Value { return logValue(v, &cfg) })
}

type logValuerFunc func() slog.Value

func (f logValuerFunc) LogValue() slog.Value { return f() }

func logValue(v View, cfg *describeConfig) slog.Value {
	attrs := make([]slog.Attr, 0, 8)
	attrs = append(attrs, slog.String("schema", schemaID(v)))
	if mti, ok := v.Get(0); ok {
		if s, err := mti.String(); err == nil {
			attrs = append(attrs, slog.String("mti", s))
		}
	}
	for p, val := range v.Fields() {
		de := int(p.Head().N)
		attrs = append(attrs, slog.Any("de"+strconv.Itoa(de), logField(val, de2mask(de, cfg), cfg)))
	}
	return slog.GroupValue(attrs...)
}

// logField produces the slog value for one field, recursing into composites.
func logField(v Value, mk maskKind, cfg *describeConfig) slog.Value {
	if child, ok := v.Composite(); ok {
		s := child.Schema()
		sub := make([]slog.Attr, 0, 8)
		if s != nil && s.IsTLV() {
			for tag, cv := range child.TagSeq() {
				sub = append(sub, slog.Any(tag, logField(cv, tag2mask(tag, cfg), cfg)))
			}
		} else {
			for p, cv := range child.Fields() {
				sub = append(sub, slog.Any(p.String(), logField(cv, maskNone, cfg)))
			}
		}
		return slog.GroupValue(sub...)
	}
	return slog.StringValue(display(v, mk, cfg))
}

// --- compact one-liner ---

// String returns a compact one-line summary of the message for %v / %s
// formatting and quick logging: profile, MTI, field count, and the present-DE
// bitmap. It never reveals field values.
func (m *Message) String() string {
	mti := "----"
	if v, ok := m.MTI(); ok {
		if s, err := v.String(); err == nil {
			mti = s
		}
	}
	return fmt.Sprintf("iso8583.Message{%s mti=%s %s}", schemaID(m), mti, m.bm)
}

// --- helpers ---

func schemaID(v View) string {
	if s := v.Schema(); s != nil {
		return s.ID()
	}
	return "?"
}

func fieldName(s *Schema, de int) string {
	if s == nil {
		return ""
	}
	if def, ok := s.Field(de); ok {
		return def.Name
	}
	return ""
}

func defName(def *FieldDef) string {
	if def == nil {
		return ""
	}
	return def.Name
}

// bitmapHex renders the present bitmap as uppercase hex, sized to the highest
// populated word so a not-yet-marshalled message with secondary/tertiary fields
// still shows them (the wire continuation bits are only set at marshal time).
func bitmapHex(b Bitmap) string {
	n := 1
	switch {
	case b.Word(2) != 0:
		n = 3
	case b.Word(1) != 0:
		n = 2
	}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "%016X", b.Word(i))
	}
	return sb.String()
}

// maskPANDigits keeps the leading six and trailing four digits, masking the
// middle — the PCI-DSS display rule. Short values keep only the last four.
func maskPANDigits(pan string) string {
	switch {
	case len(pan) <= 4:
		return pan
	case len(pan) <= 10:
		return stars(len(pan)-4) + pan[len(pan)-4:]
	default:
		return pan[:6] + stars(len(pan)-10) + pan[len(pan)-4:]
	}
}

func stars(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("*", n)
}

func hexUpper(b []byte) string {
	const hexdigits = "0123456789ABCDEF"
	if len(b) == 0 {
		return ""
	}
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2] = hexdigits[c>>4]
		out[i*2+1] = hexdigits[c&0x0f]
	}
	return string(out)
}

// displayWidth returns the rune width of s for column alignment (names are
// ASCII; this stays correct if one ever carries a multi-byte glyph).
func displayWidth(s string) int { return len([]rune(s)) }

// truncate clamps s to at most w runes, marking elision with an ellipsis.
func truncate(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 1 {
		return string(r[:w])
	}
	return string(r[:w-1]) + "…"
}

// errWriter swallows io writes after the first error, so the Describe body stays
// free of per-Fprintf error checks while still surfacing a failed writer.
type errWriter struct {
	w   io.Writer
	err error
}

func newErrWriter(w io.Writer) *errWriter { return &errWriter{w: w} }

func (e *errWriter) Write(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	n, err := e.w.Write(p)
	e.err = err
	return n, err
}
