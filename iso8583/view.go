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

package iso8583

import "iter"

// View is the read-only projection that renderers (JSON, protobuf, ISO 20022)
// consume. Because every renderer depends only on View, adding a format never
// touches the core or any codec. *Message implements View.
type View interface {
	Schema() *Schema
	Bitmap() Bitmap
	Fields() iter.Seq2[FieldPath, Value] // ascending, present fields only
	Get(de int) (Value, bool)
}

// Compile-time assertion that *Message satisfies View.
var _ View = (*Message)(nil)
