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

package packager

import (
	"embed"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/teqpace-services/isopace/fieldcodec"
	"github.com/teqpace-services/isopace/fieldcodec/subfield"
	"github.com/teqpace-services/isopace/fieldcodec/tlv"
	"github.com/teqpace-services/isopace/iso8583"
)

// The generic loader assembles the SAME *iso8583.Schema as a programmatic
// profile from declarative config, resolving every codec/length/bitmap by name
// against a Registry. Config is JSON here (stdlib); a YAML front-end is a
// drop-in via a permissive YAML dependency, kept out to stay dependency-free.
//
// schemadef/ lives under the package directory because go:embed cannot reach a
// sibling directory.
//
//go:embed schemadef/*.json
var schemaFS embed.FS

// schemaConfig is the declarative schema document.
type schemaConfig struct {
	ID         string                     `json:"id"`
	MTI        string                     `json:"mti"`
	Bitmap     bitmapConfig               `json:"bitmap"`
	Composites map[string]compositeConfig `json:"composites"`
	Fields     map[string]fieldConfig     `json:"fields"`
}

type bitmapConfig struct {
	Codec  string `json:"codec"`
	Levels int    `json:"levels"`
}

// compositeConfig declares a composite sub-schema. Type "tlv" (the default)
// addresses children by BER-TLV tag; type "subfields" is a positional
// subfield packager — a headerless bitmap + positional subfields (e.g. DE 127).
type compositeConfig struct {
	Type   string                 `json:"type,omitempty"`   // "tlv" (default) | "subfields"
	Bitmap *bitmapConfig          `json:"bitmap,omitempty"` // subfields: sub-bitmap encoding
	Tags   map[string]tagConfig   `json:"tags,omitempty"`   // tlv: children by hex tag
	Fields map[string]fieldConfig `json:"fields,omitempty"` // subfields: children by index
}

type tagConfig struct {
	Name  string `json:"name"`
	Codec string `json:"codec"`
	Max   int    `json:"max,omitempty"`
}

type fieldConfig struct {
	Name      string `json:"name"`
	Codec     string `json:"codec,omitempty"`
	Length    string `json:"length"`
	Max       int    `json:"max,omitempty"`
	Scale     uint8  `json:"scale,omitempty"`
	Composite string `json:"composite,omitempty"` // sub-schema id for a composite field
}

// Load builds a schema from a JSON document, resolving names against reg.
func Load(data []byte, reg *fieldcodec.Registry) (*iso8583.Schema, error) {
	var cfg schemaConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("packager: parse schema: %w", err)
	}
	return cfg.build(reg)
}

// LoadEmbedded builds a schema from an embedded schemadef/<name> document using
// the default registry.
func LoadEmbedded(name string) (*iso8583.Schema, error) {
	data, err := schemaFS.ReadFile("schemadef/" + name)
	if err != nil {
		return nil, fmt.Errorf("packager: embedded schema %q: %w", name, err)
	}
	return Load(data, fieldcodec.DefaultRegistry())
}

func (cfg schemaConfig) build(reg *fieldcodec.Registry) (*iso8583.Schema, error) {
	subs := make(map[string]*iso8583.Schema, len(cfg.Composites))
	subType := make(map[string]string, len(cfg.Composites))
	for id, cc := range cfg.Composites {
		s, kind, err := cc.build(id, reg)
		if err != nil {
			return nil, err
		}
		subs[id] = s
		subType[id] = kind
	}

	mti, ok := reg.Lookup(cfg.MTI)
	if !ok {
		return nil, fmt.Errorf("packager: unknown MTI codec %q", cfg.MTI)
	}
	bm, ok := reg.LookupBitmap(cfg.Bitmap.Codec)
	if !ok {
		return nil, fmt.Errorf("packager: unknown bitmap codec %q", cfg.Bitmap.Codec)
	}
	b := iso8583.NewSchema(cfg.ID).
		MTI(mti).
		Bitmap(iso8583.BitmapSpec{Codec: bm, Levels: cfg.Bitmap.Levels})

	for deStr, fc := range cfg.Fields {
		de, err := strconv.Atoi(deStr)
		if err != nil {
			return nil, fmt.Errorf("packager: field key %q is not a DE number", deStr)
		}
		length, err := resolveLength(reg, fc.Length)
		if err != nil {
			return nil, err
		}
		if fc.Composite != "" {
			sub, ok := subs[fc.Composite]
			if !ok {
				return nil, fmt.Errorf("packager: DE %d references unknown composite %q", de, fc.Composite)
			}
			cc := tlv.BERTLV
			if subType[fc.Composite] == "subfields" {
				cc = subfield.Packager
			}
			b.Composite(de, fc.Name, sub, length, iso8583.WithCodec(cc))
			continue
		}
		vc, ok := reg.Lookup(fc.Codec)
		if !ok {
			return nil, fmt.Errorf("packager: DE %d unknown codec %q", de, fc.Codec)
		}
		opts := []iso8583.FieldOpt{iso8583.MaxLen(fc.Max)}
		if fc.Scale > 0 {
			opts = append(opts, iso8583.Scale(fc.Scale))
		}
		b.Field(de, fc.Name, vc, length, opts...)
	}
	return b.Build()
}

// build assembles a composite sub-schema, returning the schema and its kind
// ("tlv" or "subfields") so the parent field can be wired with the matching
// composite value codec.
func (cc compositeConfig) build(id string, reg *fieldcodec.Registry) (*iso8583.Schema, string, error) {
	switch cc.Type {
	case "", "tlv":
		sb := iso8583.NewSchema(id)
		for tag, tc := range cc.Tags {
			vc, ok := reg.Lookup(tc.Codec)
			if !ok {
				return nil, "", fmt.Errorf("packager: composite %s tag %s: unknown codec %q", id, tag, tc.Codec)
			}
			opts := []iso8583.FieldOpt{}
			if tc.Max > 0 {
				opts = append(opts, iso8583.MaxLen(tc.Max))
			}
			sb.Tag(tag, tc.Name, vc, opts...)
		}
		s, err := sb.Build()
		return s, "tlv", err
	case "subfields":
		if cc.Bitmap == nil {
			return nil, "", fmt.Errorf("packager: subfield composite %s has no bitmap", id)
		}
		bm, ok := reg.LookupBitmap(cc.Bitmap.Codec)
		if !ok {
			return nil, "", fmt.Errorf("packager: subfield composite %s: unknown bitmap codec %q", id, cc.Bitmap.Codec)
		}
		sb := iso8583.NewSchema(id).
			Headerless().
			Bitmap(iso8583.BitmapSpec{Codec: bm, Levels: cc.Bitmap.Levels})
		for deStr, fc := range cc.Fields {
			de, err := strconv.Atoi(deStr)
			if err != nil {
				return nil, "", fmt.Errorf("packager: subfield composite %s: field key %q is not an index", id, deStr)
			}
			length, err := resolveLength(reg, fc.Length)
			if err != nil {
				return nil, "", err
			}
			vc, ok := reg.Lookup(fc.Codec)
			if !ok {
				return nil, "", fmt.Errorf("packager: subfield composite %s sub %d: unknown codec %q", id, de, fc.Codec)
			}
			opts := []iso8583.FieldOpt{iso8583.MaxLen(fc.Max)}
			if fc.Scale > 0 {
				opts = append(opts, iso8583.Scale(fc.Scale))
			}
			sb.Field(de, fc.Name, vc, length, opts...)
		}
		s, err := sb.Build()
		return s, "subfields", err
	default:
		return nil, "", fmt.Errorf("packager: composite %s: unknown type %q", id, cc.Type)
	}
}

// resolveLength maps a length name to a codec; "", "len.fixed" -> nil (the
// engine's native fixed-width path).
func resolveLength(reg *fieldcodec.Registry, name string) (iso8583.LengthCodec, error) {
	if name == "" || name == "len.fixed" {
		return nil, nil
	}
	l, ok := reg.LookupLength(name)
	if !ok {
		return nil, fmt.Errorf("packager: unknown length codec %q", name)
	}
	return l, nil
}
