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
	"reflect"
	"strings"
	"time"
)

// Binder binds a Go struct to/from a Message using `iso:"path"` tags under the
// same FieldPath grammar. The reflection plan is built and validated against the
// schema once per type (NewBinder), never per message.
type Binder[T any] struct {
	plan   *bindPlan
	schema *Schema
}

// NewBinder builds and caches the binding plan for T, validating every tag
// against the schema. It panics on a tag that names an undefined field or whose
// kind is incompatible — these are construction-time programming errors.
func NewBinder[T any](s *Schema) *Binder[T] {
	rt := reflect.TypeFor[T]()
	for rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	plan, err := planFor(s, rt)
	if err != nil {
		panic(err)
	}
	return &Binder[T]{plan: plan, schema: s}
}

// Unmarshal copies bound fields from a Message into dst (lazy: only bound
// fields decode).
func (b *Binder[T]) Unmarshal(m *Message, dst *T) error {
	rv := reflect.ValueOf(dst).Elem()
	return b.plan.unmarshal(m, rv)
}

// Marshal builds a Message from the bound fields of src.
func (b *Binder[T]) Marshal(src *T) (*Message, error) {
	rv := reflect.ValueOf(src).Elem()
	return b.plan.marshal(b.schema, rv)
}

// bindField is one struct field's binding plan entry.
type bindField struct {
	index  []int        // struct field index (FieldByIndex)
	path   FieldPath    // target field path
	typ    reflect.Type // struct field type
	format string       // time layout option, if any
	nested *bindPlan    // for composite struct fields
}

type bindPlan struct {
	fields []bindField
}

// planFor returns a cached, validated plan binding structType to the schema.
func planFor(s *Schema, structType reflect.Type) (*bindPlan, error) {
	if structType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("iso8583: bind target must be a struct, got %s", structType)
	}
	if cached, ok := s.binds.Load(structType); ok {
		return cached.(*bindPlan), nil
	}
	plan, err := buildPlan(s, structType)
	if err != nil {
		return nil, err
	}
	actual, _ := s.binds.LoadOrStore(structType, plan)
	return actual.(*bindPlan), nil
}

func buildPlan(s *Schema, structType reflect.Type) (*bindPlan, error) {
	plan := &bindPlan{}
	for i := 0; i < structType.NumField(); i++ {
		sf := structType.Field(i)
		tag, ok := sf.Tag.Lookup("iso")
		if !ok || tag == "" {
			continue // untagged exported fields are ignored
		}
		pathStr, format := splitTag(tag)
		path, err := ParsePath(pathStr)
		if err != nil {
			return nil, fmt.Errorf("iso8583: field %s: %w", sf.Name, err)
		}
		def, ok := s.Lookup(path)
		if !ok {
			return nil, fmt.Errorf("iso8583: field %s: tag %q names no schema field", sf.Name, pathStr)
		}

		bf := bindField{index: sf.Index, path: path, typ: sf.Type, format: format}

		if sf.Type.Kind() == reflect.Struct && !isScalarStruct(sf.Type) {
			if def.Sub == nil {
				return nil, fmt.Errorf("iso8583: field %s binds composite %q but schema field is not composite", sf.Name, pathStr)
			}
			nested, err := buildPlan(def.Sub, sf.Type)
			if err != nil {
				return nil, err
			}
			bf.nested = nested
		} else if def.Kind == KindComposite {
			return nil, fmt.Errorf("iso8583: field %s: composite schema field requires a nested struct", sf.Name)
		}

		plan.fields = append(plan.fields, bf)
	}
	return plan, nil
}

// isScalarStruct reports whether a struct type is one the codecs treat as a
// scalar (Decimal, Amount, time.Time) rather than a composite.
func isScalarStruct(t reflect.Type) bool {
	switch t {
	case reflect.TypeFor[Decimal](), reflect.TypeFor[Amount](), reflect.TypeFor[time.Time]():
		return true
	}
	return false
}

func splitTag(tag string) (path, format string) {
	parts := strings.Split(tag, ",")
	path = parts[0]
	for _, opt := range parts[1:] {
		if strings.HasPrefix(opt, "format=") {
			format = strings.TrimPrefix(opt, "format=")
		}
	}
	return path, format
}

func (p *bindPlan) unmarshal(m *Message, dst reflect.Value) error {
	for _, bf := range p.fields {
		fv := dst.FieldByIndex(bf.index)
		if bf.nested != nil {
			v, ok := m.GetP(bf.path)
			if !ok {
				continue
			}
			sub, ok := v.Composite()
			if !ok {
				continue
			}
			if err := bf.nested.unmarshal(sub, fv); err != nil {
				return err
			}
			continue
		}
		v, ok := m.GetP(bf.path)
		if !ok {
			continue // partial structs are allowed; absence is not an error here
		}
		if err := setField(fv, v, bf); err != nil {
			return err
		}
	}
	return nil
}

func setField(fv reflect.Value, v Value, bf bindField) error {
	switch bf.typ {
	case reflect.TypeFor[Decimal]():
		d, err := v.Decimal()
		if err != nil {
			return withPath(err, bf.path)
		}
		fv.Set(reflect.ValueOf(d))
		return nil
	case reflect.TypeFor[Amount]():
		d, err := v.Decimal()
		if err != nil {
			return withPath(err, bf.path)
		}
		fv.Set(reflect.ValueOf(Amount{Decimal: d}))
		return nil
	case reflect.TypeFor[time.Time]():
		t, err := decodeTimeLayout(v, bf.format)
		if err != nil {
			return withPath(err, bf.path)
		}
		fv.Set(reflect.ValueOf(t))
		return nil
	}
	switch bf.typ.Kind() {
	case reflect.String:
		s, err := v.String()
		if err != nil {
			return withPath(err, bf.path)
		}
		fv.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := v.Int()
		if err != nil {
			return withPath(err, bf.path)
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := v.Uint()
		if err != nil {
			return withPath(err, bf.path)
		}
		fv.SetUint(u)
	case reflect.Slice:
		if bf.typ.Elem().Kind() == reflect.Uint8 {
			fv.SetBytes(v.Bytes())
			return nil
		}
		return &FieldError{Path: bf.path, Offset: -1, Err: errWrongKind}
	default:
		return &FieldError{Path: bf.path, Offset: -1, Err: errWrongKind}
	}
	return nil
}

func (p *bindPlan) marshal(s *Schema, src reflect.Value) (*Message, error) {
	m := New(s)
	if err := p.fill(m, src); err != nil {
		return nil, err
	}
	return m, nil
}

func (p *bindPlan) fill(m *Message, src reflect.Value) error {
	for _, bf := range p.fields {
		fv := src.FieldByIndex(bf.index)
		if bf.nested != nil {
			// Nested composite writes are completed in Phase 2.
			return &FieldError{Path: bf.path, Offset: -1, Err: fmt.Errorf("nested composite write %q not yet supported", bf.path.String())}
		}
		val, err := fieldToSet(fv, bf)
		if err != nil {
			return err
		}
		if err := m.SetP(bf.path, val); err != nil {
			return err
		}
	}
	return nil
}

func fieldToSet(fv reflect.Value, bf bindField) (any, error) {
	if bf.typ == reflect.TypeFor[time.Time]() {
		t := fv.Interface().(time.Time)
		layout := layoutFor(bf.format)
		if layout == "" {
			return nil, &FieldError{Path: bf.path, Offset: -1, Err: fmt.Errorf("time field needs a format= tag to marshal")}
		}
		return t.Format(layout), nil
	}
	return fv.Interface(), nil
}

// decodeTimeLayout parses a time field, honouring an explicit format tag and
// otherwise inferring the layout from the value length.
func decodeTimeLayout(v Value, format string) (time.Time, error) {
	if format != "" {
		layout := layoutFor(format)
		if layout == "" {
			return time.Time{}, &FieldError{Offset: v.off, Err: fmt.Errorf("unknown time format %q", format)}
		}
		t, err := time.Parse(layout, string(v.Bytes()))
		if err != nil {
			return time.Time{}, &FieldError{Offset: v.off, Err: err}
		}
		return t, nil
	}
	return decodeTime(v)
}

// layoutFor maps an ISO-8583 time-format token to a Go reference layout.
func layoutFor(format string) string {
	switch format {
	case "hhmmss":
		return "150405"
	case "hhmm":
		return "1504"
	case "MMDD":
		return "0102"
	case "MMDDhhmmss":
		return "0102150405"
	case "YYMMDDhhmmss":
		return "060102150405"
	case "YYYYMMDDhhmmss":
		return "20060102150405"
	default:
		return ""
	}
}

// Bind unmarshals wire and binds it into dst (a pointer to a struct), building
// and caching the binder internally.
func (c *Codec) Bind(wire []byte, dst any) error {
	m, err := c.Unmarshal(wire)
	if err != nil {
		return err
	}
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("iso8583: Bind dst must be a non-nil pointer, got %T", dst)
	}
	elem := rv.Elem()
	plan, err := planFor(c.schema, elem.Type())
	if err != nil {
		return err
	}
	return plan.unmarshal(m, elem)
}

// Pack builds a Message from src (a struct or pointer to one) and marshals it to
// wire.
func (c *Codec) Pack(src any) ([]byte, error) {
	rv := reflect.ValueOf(src)
	for rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	plan, err := planFor(c.schema, rv.Type())
	if err != nil {
		return nil, err
	}
	m, err := plan.marshal(c.schema, rv)
	if err != nil {
		return nil, err
	}
	return c.Marshal(m, nil)
}
