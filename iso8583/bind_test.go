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
	"reflect"
	"testing"
)

type authReq struct {
	MTI      string  `iso:"0"`
	PAN      string  `iso:"2"`
	Proc     string  `iso:"3"`
	Amount   Decimal `iso:"4"`
	STAN     int64   `iso:"11"`
	Resp     string  `iso:"39"`
	Ignored  int     // untagged -> ignored
	internal string  // unexported -> ignored
}

func TestBinderRoundTrip(t *testing.T) {
	s := testSchema()
	c := NewCodec(s)
	binder := NewBinder[authReq](s)

	in := authReq{
		MTI:    "0200",
		PAN:    validPAN,
		Proc:   "000000",
		Amount: NewDecimal(1099, 2),
		STAN:   123,
		Resp:   "00",
	}

	m, err := binder.Marshal(&in)
	if err != nil {
		t.Fatalf("binder.Marshal: %v", err)
	}
	wire, err := c.Marshal(m, nil)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	m2, err := c.Unmarshal(wire)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	var out authReq
	if err := binder.Unmarshal(m2, &out); err != nil {
		t.Fatalf("binder.Unmarshal: %v", err)
	}

	if out.MTI != in.MTI || out.PAN != in.PAN || out.Proc != in.Proc ||
		out.STAN != in.STAN || out.Resp != in.Resp || out.Amount != in.Amount {
		t.Errorf("round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestCodecBindPackConvenience(t *testing.T) {
	s := testSchema()
	c := NewCodec(s)

	in := authReq{MTI: "0210", PAN: validPAN, Proc: "000000", Amount: NewDecimal(250, 2), STAN: 7, Resp: "00"}
	wire, err := c.Pack(&in)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	var out authReq
	if err := c.Bind(wire, &out); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if out.Amount.String() != "2.50" || out.STAN != 7 || out.MTI != "0210" {
		t.Errorf("Bind/Pack round-trip wrong: %+v", out)
	}
}

func TestNewBinderRejectsUnknownTag(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("NewBinder with a tag for an undefined DE should panic")
		}
	}()
	type bad struct {
		X string `iso:"99"`
	}
	_ = NewBinder[bad](testSchema())
}

func TestBinderPlanCached(t *testing.T) {
	s := testSchema()
	p1, err := planFor(s, reflect.TypeOf(authReq{}))
	if err != nil {
		t.Fatalf("planFor: %v", err)
	}
	p2, _ := planFor(s, reflect.TypeOf(authReq{}))
	if p1 != p2 {
		t.Errorf("plan not cached: %p vs %p", p1, p2)
	}
}
