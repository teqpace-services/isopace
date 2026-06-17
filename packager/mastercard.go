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

// Mastercard is a representative Mastercard dialect over ISO 8583:1993 variant
// A, redefining the additional-data private fields DE 48 and DE 63. It
// demonstrates a scheme overlay via Derive/Override.
func Mastercard() *iso8583.Schema {
	return ISO93A().Derive("mastercard").
		Override(48, iso8583.MaxLen(999)).
		Override(63, iso8583.WithCodec(fieldcodec.BINARY), iso8583.MaxLen(512)).
		MustBuild()
}
