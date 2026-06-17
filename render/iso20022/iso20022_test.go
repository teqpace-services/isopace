// SPDX-License-Identifier: AGPL-3.0-or-later
//
// Copyright (C) 2026 Teqpace Services Ltd.
//
// This file is part of Isopace, a financial transaction package.
//
// Isopace is dual-licensed:
//   - under the GNU Affero General Public License v3.0 or later (see LICENSE); or
//   - under a commercial license from Teqpace Services Ltd. (see COMMERCIAL-LICENSE.md).
//
// Authorship is recorded in the AUTHORS file.

package iso20022_test

import (
	"strings"
	"testing"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/packager"
	"github.com/teqpace-services/isopace/render/iso20022"
)

func TestISO20022RoundTrip(t *testing.T) {
	s := packager.ISO87A()
	m := iso8583.New(s)
	for _, kv := range []struct {
		de int
		v  any
	}{
		{2, "4111111111111111"},
		{4, iso8583.NewDecimal(1099, 2)},
		{7, "1231235959"},
		{11, int64(123)},
		{37, "RRN000000001"},
		{49, "840"},
	} {
		if err := m.Set(kv.de, kv.v); err != nil {
			t.Fatalf("Set(%d): %v", kv.de, err)
		}
	}

	xmlb, err := iso20022.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	doc := string(xmlb)
	for _, want := range []string{"FIToFICstmrCdtTrf", "IntrBkSttlmAmt", `Ccy="840"`, "pacs.008"} {
		if !strings.Contains(doc, want) {
			t.Errorf("XML missing %q:\n%s", want, doc)
		}
	}

	m2, err := iso20022.Unmarshal(xmlb, s)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// The mapped subset must survive the bridge.
	if got, _ := iso8583.Get[string](m2, 2); got != "4111111111111111" {
		t.Errorf("DE2 = %q", got)
	}
	if got, _ := iso8583.Get[string](m2, 37); got != "RRN000000001" {
		t.Errorf("DE37 = %q", got)
	}
	if got, _ := iso8583.Get[string](m2, 49); got != "840" {
		t.Errorf("DE49 = %q", got)
	}
	amt, _ := iso8583.Get[iso8583.Decimal](m2, 4)
	if amt.String() != "10.99" {
		t.Errorf("DE4 amount = %s want 10.99 (scale recovered from XML)", amt.String())
	}
}
