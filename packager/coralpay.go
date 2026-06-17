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
	"github.com/teqpace-services/isopace/fieldcodec/subfield"
	"github.com/teqpace-services/isopace/iso8583"
)

// CoralPay is an ISO 8583:1987 profile for the CoralPay acquirer link — a
// clean-room layout composed from public ISO 8583:1987 field semantics and
// CoralPay's published field tables. It reuses the postField/p* shorthands from
// postilion.go.
func CoralPay() *iso8583.Schema {
	sub := iso8583.NewSchema("coralpay-127").
		Headerless().
		Bitmap(iso8583.BitmapSpec{Codec: fieldcodec.BitmapHex, Levels: 1})
	for _, f := range coralpaySub127 {
		sub.Field(f.de, f.name, f.value, f.length, iso8583.MaxLen(f.max))
	}
	subSchema := sub.MustBuild()

	b := iso8583.NewSchema("coralpay").
		MTI(fieldcodec.ASCII).
		Bitmap(iso8583.BitmapSpec{Codec: fieldcodec.BitmapHex, Levels: 2})
	for _, f := range coralpayFields {
		b.Field(f.de, f.name, f.value, f.length, iso8583.MaxLen(f.max))
	}
	b.Composite(127, "RESERVED PRIVATE USE", subSchema, pLLLLLL, iso8583.WithCodec(subfield.Packager))
	return b.MustBuild()
}

// coralpayFields is the top-level data-element directory (DE 1 bitmap and DE 127
// composite are wired separately).
var coralpayFields = []postField{
	{2, "PRIMARY ACCOUNT NUMBER", pNum, pLL, 19},
	{3, "PROCESSING CODE", pChr, pFix, 6},
	{4, "AMOUNT, TRANSACTION", pNum, pFix, 12},
	{5, "AMOUNT, SETTLEMENT", pNum, pFix, 12},
	{7, "TRANSMISSION DATE AND TIME", pChr, pFix, 10},
	{9, "CONVERSION RATE, SETTLEMENT", pChr, pFix, 8},
	{11, "SYSTEM TRACE AUDIT NUMBER", pNum, pFix, 6},
	{12, "TIME, LOCAL TRANSACTION", pChr, pFix, 6},
	{13, "DATE, LOCAL TRANSACTION", pChr, pFix, 4},
	{14, "DATE, EXPIRATION", pChr, pFix, 4},
	{15, "DATE, SETTLEMENT", pChr, pFix, 4},
	{18, "MERCHANT TYPE", pChr, pFix, 4},
	{22, "POS ENTRY MODE", pChr, pFix, 3},
	{23, "CARD SEQUENCE NUMBER", pChr, pFix, 3},
	{25, "POS CONDITION CODE", pChr, pFix, 2},
	{26, "POS PIN CAPTURE CODE", pChr, pFix, 2},
	{28, "AMOUNT, TRANSACTION FEE", pChr, pFix, 9},
	{29, "AMOUNT, SETTLEMENT FEE", pChr, pFix, 9},
	{30, "AMOUNT, TRANSACTION PROCESSING FEE", pChr, pFix, 9},
	{31, "AMOUNT, SETTLEMENT PROCESSING FEE", pChr, pFix, 9},
	{32, "ACQUIRING INSTITUTION IDENTIFICATION CODE", pChr, pLL, 11},
	{33, "FORWARDING INSTITUTION IDENTIFICATION CODE", pNum, pLL, 11},
	{35, "TRACK 2 DATA", pChr, pLL, 37},
	{37, "RETRIEVAL REFERENCE NUMBER", pChr, pFix, 12},
	{38, "AUTHORIZATION CODE", pChr, pFix, 6},
	{39, "RESPONSE CODE", pChr, pFix, 2},
	{40, "SERVICE RESTRICTION CODE", pChr, pFix, 3},
	{41, "CARD ACCEPTOR TERMINAL IDENTIFICATION", pChr, pFix, 8},
	{42, "CARD ACCEPTOR IDENTIFICATION CODE", pChr, pFix, 15},
	{43, "CARD ACCEPTOR NAME/LOCATION", pChr, pFix, 40},
	{48, "ADDITIONAL DATA", pChr, pLLL, 999},
	{49, "CURRENCY CODE", pChr, pFix, 3},
	{52, "PIN DATA", pHex, pFix, 8},
	{53, "SECURITY RELATED CONTROL INFORMATION", pChr, pFix, 96},
	{54, "ADDITIONAL AMOUNT", pChr, pLLL, 120},
	{55, "INTEGRATED CIRCUIT CARD SYSTEM RELATED DATA", pChr, pLLL, 510},
	{56, "MESSAGE REASON CODE", pChr, pLLL, 4},
	{59, "ECHO DATA", pChr, pLLL, 255},
	{60, "PAYMENT INFORMATION", pChr, pLLL, 999},
	{61, "PRIVATE FIELD, MANAGEMENT DATA 1", pChr, pLLL, 999},
	{62, "PRIVATE FIELD, MANAGEMENT DATA 1", pChr, pLLL, 999},
	{63, "PRIVATE FIELD, MANAGEMENT DATA 2", pChr, pLLLL, 9999},
	{64, "PRIMARY MESSAGE HASH VALUE", pChr, pFix, 64},
	{70, "Network Management Information Code", pChr, pFix, 3},
	{90, "ORIGINAL DATA ELEMENTS", pNum, pFix, 42},
	{95, "REPLACEMENT AMOUNTS", pNum, pFix, 42},
	{98, "PAYEE", pChr, pFix, 25},
	{100, "RECEIVING INSTITUTION IDENT CODE", pNum, pLL, 11},
	{102, "ACCOUNT IDENTIFICATION 1", pChr, pLL, 28},
	{103, "ACCOUNT IDENTIFICATION 2", pChr, pLL, 28},
	{123, "POS DATA CODE", pChr, pLLL, 15},
	{128, "SECONDARY MESSAGE HASH VALUE", pChr, pFix, 64},
}

// coralpaySub127 is the DE 127 reserved-private subfield group (field 0 placeholder
// and field 1 bitmap are implicit in the headerless sub-schema).
var coralpaySub127 = []postField{
	{2, "SWITCH KEY", pChr, pLL, 32},
	{3, "ROUTING INFORMATION", pChr, pFix, 48},
	{4, "POS DATA", pChr, pFix, 22},
	{5, "SERVICE STATION DATA", pChr, pFix, 73},
	{6, "AUTHORIZATION PROFILE", pNum, pFix, 2},
	{7, "CHECK DATA", pChr, pLL, 50},
	{8, "RETENTION DATA", pChr, pLLL, 999},
	{9, "ADDITIONAL NODE DATA", pChr, pLLL, 255},
	{10, "CVV2", pNum, pFix, 3},
	{11, "ORIGINAL KEY", pChr, pLL, 32},
	{12, "TERMINAL OWNER", pChr, pLL, 25},
	{13, "POS GEOGRAPHIC DATA", pChr, pFix, 17},
	{14, "SPONSOR BANK", pChr, pFix, 8},
	{15, "AVS REQUEST", pChr, pLL, 29},
	{16, "AVS RESPONSE", pChr, pFix, 1},
	{17, "CARDHOLDER INFORMATION", pChr, pLL, 50},
	{18, "VALIDATION DATA", pChr, pLL, 50},
	{19, "BANK DETAILS", pChr, pFix, 31},
	{20, "AUTHORIZER DATE SETTLEMENT", pNum, pFix, 8},
	{21, "RECORD IDENTIFICATION", pChr, pLL, 12},
	{22, "STRUCTURED DATA", pChr, pLLLLL, 99999},
	{23, "PAYEE NAME AND ADDRESS", pChr, pFix, 253},
	{24, "PAYER ACCOUNT INFORMATION", pChr, pLL, 28},
	{25, "ICC DATA", pChr, pLLLL, 9999},
	{26, "ORIGINAL NODE", pChr, pLL, 20},
	{27, "CARD VERIFICATION RESULT", pChr, pFix, 1},
	{28, "AMERICAN EXPRESS CARD IDENTIFIER", pNum, pFix, 4},
	{29, "3-D SECURE DATA", pHex, pFix, 40},
	{30, "3-D SECURE RESULT", pChr, pFix, 1},
	{31, "ISSUER NETWORK ID", pChr, pLL, 11},
	{32, "UCAF DATA", pChr, pLL, 33},
	{33, "EXTENDED TRANSACTION TYPE", pNum, pFix, 4},
	{34, "ACCOUNT TYPE QUALIFIERS", pNum, pFix, 2},
	{35, "ACQUIRER NETWORK ID", pChr, pLL, 11},
	{36, "CUSTOMER ID", pChr, pLL, 25},
	{37, "EXTENDED RESPONSE CODE", pChr, pFix, 4},
	{38, "ADDITIONAL POS DATA CODE", pChr, pLL, 99},
	{39, "ORIGINAL RESPONSE CODE", pChr, pFix, 2},
	{40, "ACCOUNT TYPE QUALIFIERS", pChr, pLLL, 512},
	{41, "ORIGINATING REMOTE ADDRESS", pChr, pLL, 99},
	{42, "TRANSACTION NUMBER", pChr, pLL, 10},
}
