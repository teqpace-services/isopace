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

// Package packager assembles named ISO-8583 profiles — immutable iso8583
// Schemas wired from the orthogonal fieldcodec/lengthcodec catalog. A profile is
// just a *Schema (there is no special packager type), so card-scheme dialects
// are deltas (Derive/Override) over a base, and the same composition is also
// expressible as declarative config via the generic loader.
package packager

import (
	"github.com/teqpace-services/isopace/fieldcodec"
	"github.com/teqpace-services/isopace/fieldcodec/tlv"
	"github.com/teqpace-services/isopace/iso8583"
)

// dataType classifies a data element's value representation.
type dataType uint8

const (
	tNum    dataType = iota // numeric digits
	tAlpha                  // alphanumeric / text
	tBinary                 // raw octets
	tAmount                 // monetary amount -> Decimal
	tEMV                    // BER-TLV composite (DE 55)
)

// lenType classifies a data element's length discipline.
type lenType uint8

const (
	lFixed lenType = iota
	lLL            // LLVAR
	lLLL           // LLLVAR
)

// fieldSpec is one data element in a representation-independent directory. The
// same directory builds every variant; a rep selects the concrete codecs.
type fieldSpec struct {
	de    int
	name  string
	typ   dataType
	ln    lenType
	max   int
	scale uint8 // amount scale (minor-unit digits)
}

// rep is a representation profile: the concrete codecs a variant uses for each
// data type and length discipline, plus the bitmap and MTI codecs.
type rep struct {
	num, alpha, amount, binary iso8583.FieldCodec
	lenLL, lenLLL              iso8583.LengthCodec
	bitmap                     iso8583.BitmapCodec
	levels                     int
	mti                        iso8583.FieldCodec
}

// build assembles a Schema from a directory and a representation. Fixed fields
// use a nil length codec (the engine reads MaxLen wire units); variable fields
// use the rep's LL/LLL codec. DE 55 is wired as a BER-TLV composite.
func build(id string, dir []fieldSpec, r rep) *iso8583.Schema {
	b := iso8583.NewSchema(id).
		MTI(r.mti).
		Bitmap(iso8583.BitmapSpec{Codec: r.bitmap, Levels: r.levels})

	for _, f := range dir {
		opts := []iso8583.FieldOpt{iso8583.MaxLen(f.max)}
		if f.typ == tAmount {
			opts = append(opts, iso8583.Scale(f.scale))
		}
		if f.typ == tEMV {
			b.Composite(f.de, f.name, emvSchema(), lengthFor(r, f.ln), iso8583.WithCodec(tlv.BERTLV))
			continue
		}
		b.Field(f.de, f.name, valueFor(r, f.typ), lengthFor(r, f.ln), opts...)
	}
	return b.MustBuild()
}

func valueFor(r rep, t dataType) iso8583.FieldCodec {
	switch t {
	case tNum:
		return r.num
	case tAlpha:
		return r.alpha
	case tBinary:
		return r.binary
	case tAmount:
		return r.amount
	default:
		return r.alpha
	}
}

// lengthFor returns the length codec for a discipline; fixed fields use a nil
// codec so the engine reads exactly MaxLen units.
func lengthFor(r rep, l lenType) iso8583.LengthCodec {
	switch l {
	case lLL:
		return r.lenLL
	case lLLL:
		return r.lenLLL
	default:
		return nil
	}
}

// emvSchema is a representative EMV (DE 55) BER-TLV sub-schema. Tags are
// addressed by canonical hex under the uniform path grammar ("55.9F26").
func emvSchema() *iso8583.Schema {
	return iso8583.NewSchema("emv-55").
		Tag("9F26", "Application Cryptogram", fieldcodec.BINARY).
		Tag("9F27", "Cryptogram Information Data", fieldcodec.BINARY, iso8583.MaxLen(1)).
		Tag("9F10", "Issuer Application Data", fieldcodec.BINARY).
		Tag("9F36", "Application Transaction Counter", fieldcodec.NumBinary, iso8583.MaxLen(2)).
		Tag("9F37", "Unpredictable Number", fieldcodec.BINARY, iso8583.MaxLen(4)).
		Tag("95", "Terminal Verification Results", fieldcodec.BINARY, iso8583.MaxLen(5)).
		Tag("9A", "Transaction Date", fieldcodec.NumBinary, iso8583.MaxLen(3)).
		Tag("9C", "Transaction Type", fieldcodec.NumBinary, iso8583.MaxLen(1)).
		Tag("5F2A", "Transaction Currency Code", fieldcodec.NumBinary, iso8583.MaxLen(2)).
		Tag("82", "Application Interchange Profile", fieldcodec.BINARY, iso8583.MaxLen(2)).
		Tag("9F1A", "Terminal Country Code", fieldcodec.NumBinary, iso8583.MaxLen(2)).
		Tag("9F03", "Amount Other", fieldcodec.NumBinary, iso8583.MaxLen(6)).
		Tag("9F02", "Amount Authorised", fieldcodec.NumBinary, iso8583.MaxLen(6)).
		MustBuild()
}

// Profiles maps every built-in profile id to its constructor, for discovery and
// name-based lookup (e.g. the generic loader, an admin API).
func Profiles() map[string]func() *iso8583.Schema {
	return map[string]func() *iso8583.Schema{
		"iso87-a":    ISO87A,
		"iso87-b":    ISO87B,
		"iso87-c":    ISO87C,
		"iso93-a":    ISO93A,
		"iso93-b":    ISO93B,
		"visa-base1": VisaBaseI,
		"mastercard": Mastercard,
		"zone":       Zone,
		"fields":     Fields,
		"switch":     Switch,
	}
}

// Profile builds a built-in profile by id.
func Profile(id string) (*iso8583.Schema, bool) {
	if ctor, ok := Profiles()[id]; ok {
		return ctor(), true
	}
	return nil, false
}
