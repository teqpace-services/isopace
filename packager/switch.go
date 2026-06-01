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
	"github.com/teqpace-services/isopace/fieldcodec"
	"github.com/teqpace-services/isopace/iso8583"
)

// The "fields" and "switch" profiles share one sparse switch layout; only the
// profile id differs. It uses an ASCII text MTI, an ASCII-hex message bitmap, a
// PIN block carried as ASCII hex, and the DE 127 reserved-private subfield group.

// switchPackager builds the shared layout under the given ids.
func switchPackager(id, sub127ID string) concretePackager {
	return concretePackager{
		id:     id,
		mti:    fieldcodec.ASCII,
		bitmap: fieldcodec.BitmapHex,
		fields: switchFields,
		sub127: &subPackager{
			id:     sub127ID,
			name:   "RESERVED PRIVATE USE",
			bitmap: fieldcodec.BitmapHex,
			fields: switchSub127Fields,
		},
	}
}

var switchFields = []field{
	{2, "PRIMARY ACCOUNT NUMBER", numeric, llVar, 19},
	{3, "PROCESSING CODE", textual, fixedWidth, 6},
	{4, "AMOUNT, TRANSACTION", numeric, fixedWidth, 12},
	{5, "AMOUNT, SETTLEMENT", numeric, fixedWidth, 12},
	{7, "TRANSMISSION DATE AND TIME", textual, fixedWidth, 10},
	{9, "CONVERSION RATE, SETTLEMENT", textual, fixedWidth, 8},
	{11, "SYSTEM TRACE AUDIT NUMBER", numeric, fixedWidth, 6},
	{12, "TIME, LOCAL TRANSACTION", textual, fixedWidth, 6},
	{13, "DATE, LOCAL TRANSACTION", textual, fixedWidth, 4},
	{14, "DATE, EXPIRATION", textual, fixedWidth, 4},
	{15, "DATE, SETTLEMENT", textual, fixedWidth, 4},
	{18, "MERCHANT TYPE", textual, fixedWidth, 4},
	{22, "POS ENTRY MODE", textual, fixedWidth, 3},
	{23, "CARD SEQUENCE NUMBER", textual, fixedWidth, 3},
	{25, "POS CONDITION CODE", textual, fixedWidth, 2},
	{26, "POS PIN CAPTURE CODE", textual, fixedWidth, 2},
	{28, "AMOUNT, TRANSACTION FEE", textual, fixedWidth, 9},
	{29, "AMOUNT, SETTLEMENT FEE", textual, fixedWidth, 9},
	{30, "AMOUNT, TRANSACTION PROCESSING FEE", textual, fixedWidth, 9},
	{31, "AMOUNT, SETTLEMENT PROCESSING FEE", textual, fixedWidth, 9},
	{32, "ACQUIRING INSTITUTION IDENTIFICATION CODE", textual, llVar, 11},
	{33, "FORWARDING INSTITUTION IDENTIFICATION CODE", numeric, llVar, 11},
	{35, "TRACK 2 DATA", textual, llVar, 37},
	{37, "RETRIEVAL REFERENCE NUMBER", textual, fixedWidth, 12},
	{38, "AUTHORIZATION CODE", textual, fixedWidth, 6},
	{39, "RESPONSE CODE", textual, fixedWidth, 2},
	{40, "SERVICE RESTRICTION CODE", textual, fixedWidth, 3},
	{41, "CARD ACCEPTOR TERMINAL IDENTIFICATION", textual, fixedWidth, 8},
	{42, "CARD ACCEPTOR IDENTIFICATION CODE", textual, fixedWidth, 15},
	{43, "CARD ACCEPTOR NAME/LOCATION", textual, fixedWidth, 40},
	{48, "ADDITIONAL DATA", textual, lllVar, 999},
	{49, "CURRENCY CODE", textual, fixedWidth, 3},
	{52, "PIN DATA", hexBin, fixedWidth, 8},
	{53, "SECURITY RELATED CONTROL INFORMATION", textual, fixedWidth, 96},
	{54, "ADDITIONAL AMOUNT", textual, lllVar, 120},
	{55, "INTEGRATED CIRCUIT CARD SYSTEM RELATED DATA", textual, lllVar, 510},
	{56, "MESSAGE REASON CODE", textual, fixedWidth, 4},
	{59, "ECHO DATA", textual, lllVar, 255},
	{60, "PAYMENT INFORMATION", textual, lllVar, 999},
	{61, "PRIVATE FIELD, MANAGEMENT DATA 1", textual, lllVar, 999},
	{62, "PRIVATE FIELD, MANAGEMENT DATA 1", textual, lllVar, 999},
	{63, "PRIVATE FIELD, MANAGEMENT DATA 2", textual, llllVar, 9999},
	{64, "PRIMARY MESSAGE HASH VALUE", textual, fixedWidth, 64},
	{70, "Network Management Information Code", textual, fixedWidth, 3},
	{90, "ORIGINAL DATA ELEMENTS", numeric, fixedWidth, 42},
	{95, "REPLACEMENT AMOUNTS", numeric, fixedWidth, 42},
	{98, "PAYEE", textual, fixedWidth, 25},
	{100, "RECEIVING INSTITUTION IDENT CODE", numeric, llVar, 11},
	{101, "FILE NAME", textual, llVar, 17},
	{102, "ACCOUNT IDENTIFICATION 1", textual, llVar, 28},
	{103, "ACCOUNT IDENTIFICATION 2", textual, llVar, 28},
	{123, "POS DATA CODE", textual, fixedWidth, 15},
	{128, "SECONDARY MESSAGE HASH VALUE", textual, fixedWidth, 64},
}

var switchSub127Fields = []field{
	{2, "SWITCH KEY", textual, fixedWidth, 32},
	{3, "ROUTING INFORMATION", textual, fixedWidth, 48},
	{4, "POS DATA", textual, fixedWidth, 22},
	{5, "SERVICE STATION DATA", textual, fixedWidth, 73},
	{6, "AUTHORIZATION PROFILE", numeric, fixedWidth, 2},
	{7, "CHECK DATA", textual, llVar, 50},
	{8, "RETENTION DATA", textual, lllVar, 999},
	{9, "ADDITIONAL NODE DATA", textual, lllVar, 255},
	{10, "CVV2", numeric, fixedWidth, 3},
	{11, "ORIGINAL KEY", textual, fixedWidth, 32},
	{12, "TERMINAL OWNER", textual, llVar, 25},
	{13, "POS GEOGRAPHIC DATA", textual, fixedWidth, 17},
	{14, "SPONSOR BANK", textual, fixedWidth, 8},
	{15, "AVS REQUEST", textual, llVar, 29},
	{16, "AVS RESPONSE", textual, fixedWidth, 1},
	{17, "CARDHOLDER INFORMATION", textual, llVar, 50},
	{18, "VALIDATION DATA", textual, llVar, 50},
	{19, "BANK DETAILS", textual, fixedWidth, 31},
	{20, "AUTHORIZER DATE SETTLEMENT", numeric, fixedWidth, 8},
	{21, "RECORD IDENTIFICATION", textual, llVar, 12},
	{22, "STRUCTURED DATA", textual, lllllVar, 99999},
	{23, "PAYEE NAME AND ADDRESS", textual, fixedWidth, 253},
	{24, "PAYER ACCOUNT INFORMATION", textual, llVar, 28},
	{25, "ICC DATA", textual, llllVar, 9999},
	{26, "ORIGINAL NODE", textual, llVar, 20},
	{27, "CARD VERIFICATION RESULT", textual, fixedWidth, 1},
	{28, "AMERICAN EXPRESS CARD IDENTIFIER", numeric, fixedWidth, 4},
	{29, "3-D SECURE DATA", hexBin, fixedWidth, 40},
	{30, "3-D SECURE RESULT", textual, fixedWidth, 1},
	{31, "ISSUER NETWORK ID", textual, llVar, 11},
	{32, "UCAF DATA", textual, llVar, 33},
	{33, "EXTENDED TRANSACTION TYPE", numeric, fixedWidth, 4},
	{34, "ACCOUNT TYPE QUALIFIERS", numeric, fixedWidth, 2},
	{35, "ACQUIRER NETWORK ID", textual, llVar, 11},
	{36, "CUSTOMER ID", textual, llVar, 25},
	{37, "EXTENDED RESPONSE CODE", textual, fixedWidth, 4},
	{38, "ADDITIONAL POS DATA CODE", textual, llVar, 99},
	{39, "ORIGINAL RESPONSE CODE", textual, fixedWidth, 2},
	{40, "ACCOUNT TYPE QUALIFIERS", textual, lllVar, 512},
	{41, "ORIGINATING REMOTE ADDRESS", textual, llVar, 99},
	{42, "TRANSACTION NUMBER", textual, llVar, 10},
}

// Fields returns the "fields" packager profile.
func Fields() *iso8583.Schema { return switchPackager("fields", "fields-127").schema() }

// Switch returns the "switch" packager profile (the same layout as Fields under
// a distinct id).
func Switch() *iso8583.Schema { return switchPackager("switch", "switch-127").schema() }
