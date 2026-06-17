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

package packager

import (
	"github.com/teqpace-services/isopace/fieldcodec"
	"github.com/teqpace-services/isopace/iso8583"
)

// VisaBaseI is Visa Base I expressed as a delta over ISO 8583:1993 variant A:
// the private fields DE 62/63 carry Visa-specific binary network data. This is
// a representative overlay demonstrating Derive/Override, not the full Visa
// dialect.
func VisaBaseI() *iso8583.Schema {
	return ISO93A().Derive("visa-base1").
		Override(62, iso8583.WithCodec(fieldcodec.BINARY), iso8583.MaxLen(255)).
		Override(63, iso8583.WithCodec(fieldcodec.BINARY), iso8583.MaxLen(255)).
		MustBuild()
}
