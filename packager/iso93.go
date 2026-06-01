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

import "github.com/teqpace-services/isopace/iso8583"

// iso93Dir is the ISO 8583:1993 data-element directory. The 1993 catalogue is
// an independent definition (it is the base for card-scheme overlays); the most
// visible differences from 1987 are the 3-character response code (DE 39) and
// the message reason code (DE 56).
var iso93Dir = iso93Directory()

func iso93Directory() []fieldSpec {
	dir := make([]fieldSpec, len(iso87Dir))
	copy(dir, iso87Dir)
	for i := range dir {
		if dir[i].de == 39 {
			dir[i].max = 3 // 1993 response/action code is 3 characters
		}
	}
	dir = append(dir,
		fieldSpec{56, "Message Reason Code", tNum, lLLL, 4, 0},
		fieldSpec{95, "Replacement Amounts", tAlpha, lFixed, 42, 0},
	)
	return dir
}

// ISO93A returns the ISO 8583:1993 variant A (ASCII) profile.
func ISO93A() *iso8583.Schema { return build("iso93-a", iso93Dir, repASCII()) }

// ISO93B returns the ISO 8583:1993 variant B (binary / BCD) profile.
func ISO93B() *iso8583.Schema { return build("iso93-b", iso93Dir, repBCD()) }
