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
	"github.com/teqpace-services/isopace/fieldcodec/subfield"
	"github.com/teqpace-services/isopace/iso8583"
)

// Zone is an ISO 8583:1987 profile for the Zone acquirer link — a clean-room
// layout composed from public ISO 8583:1987 field semantics and Zone's published
// field tables. It reuses the postField/p* shorthands from postilion.go.
func Zone() *iso8583.Schema {
	sub := iso8583.NewSchema("zone-127").
		Headerless().
		Bitmap(iso8583.BitmapSpec{Codec: fieldcodec.BitmapBinary, Levels: 1})
	for _, f := range zoneSub127 {
		sub.Field(f.de, f.name, f.value, f.length, iso8583.MaxLen(f.max))
	}
	subSchema := sub.MustBuild()

	b := iso8583.NewSchema("zone").
		MTI(fieldcodec.NumASCII).
		Bitmap(iso8583.BitmapSpec{Codec: fieldcodec.BitmapBinary, Levels: 2})
	for _, f := range zoneFields {
		b.Field(f.de, f.name, f.value, f.length, iso8583.MaxLen(f.max))
	}
	b.Composite(127, "RESERVED PRIVATE USE", subSchema, pLLLLLL, iso8583.WithCodec(subfield.Packager))
	return b.MustBuild()
}

// zoneFields is the top-level data-element directory (DE 1 bitmap and DE 127
// composite are wired separately).
var zoneFields = []postField{
	{2, "PAN - PRIMARY ACCOUNT NUMBER", pNum, pLL, 19},
	{3, "PROCESSING CODE", pNum, pFix, 6},
	{4, "AMOUNT, TRANSACTION", pNum, pFix, 12},
	{5, "AMOUNT, SETTLEMENT", pNum, pFix, 12},
	{6, "AMOUNT, CARDHOLDER BILLING", pNum, pFix, 12},
	{7, "TRANSMISSION DATE AND TIME", pNum, pFix, 10},
	{8, "Cardholder billing fee amount", pNum, pFix, 8},
	{9, "CONVERSION RATE, SETTLEMENT", pNum, pFix, 8},
	{10, "Cardholder billing conversion rate", pNum, pFix, 8},
	{11, "SYSTEM TRACE AUDIT NUMBER", pNum, pFix, 6},
	{12, "TIME, LOCAL TRANSACTION", pNum, pFix, 6},
	{13, "DATE, LOCAL TRANSACTION", pNum, pFix, 4},
	{14, "DATE, EXPIRATION", pNum, pFix, 4},
	{15, "DATE, SETTLEMENT", pNum, pFix, 4},
	{16, "DATE, CONVERSION", pNum, pFix, 4},
	{17, "DATE, CAPTURE", pNum, pFix, 4},
	{18, "MERCHANTS TYPE", pNum, pFix, 4},
	{19, "Acquiring institution country code", pNum, pFix, 3},
	{20, "Primary account number country code", pNum, pFix, 3},
	{21, "Forwarding institution country code", pNum, pFix, 3},
	{22, "POINT OF SERVICE ENTRY MODE", pNum, pFix, 3},
	{23, "CARD SEQUENCE NUMBER", pNum, pFix, 3},
	{24, "Network international identifier", pNum, pFix, 3},
	{25, "POINT OF SERVICE CONDITION CODE", pNum, pFix, 2},
	{26, "POINT OF SERVICE PIN CAPTURE CODE", pNum, pFix, 2},
	{27, "AUTHORIZATION IDENTIFICATION RESP LEN", pNum, pFix, 1},
	{28, "AMOUNT, TRANSACTION FEE", pChr, pFix, 9},
	{29, "AMOUNT, SETTLEMENT FEE", pChr, pFix, 9},
	{30, "AMOUNT, TRANSACTION PROCESSING FEE", pChr, pFix, 9},
	{31, "AMOUNT, SETTLEMENT PROCESSING FEE", pChr, pFix, 9},
	{32, "ACQUIRING INSTITUTION IDENT CODE", pNum, pLL, 11},
	{33, "FORWARDING INSTITUTION IDENT CODE", pNum, pLL, 11},
	{34, "Extended primary account number", pNum, pLL, 28},
	{35, "TRACK 2 DATA", pNum, pLL, 37},
	{36, "TRACK 3 DATA", pNum, pLLL, 104},
	{37, "RETRIEVAL REFERENCE NUMBER", pChr, pFix, 12},
	{38, "AUTHORIZATION IDENTIFICATION RESPONSE", pChr, pFix, 6},
	{39, "RESPONSE CODE", pChr, pFix, 2},
	{40, "SERVICE RESTRICTION CODE", pChr, pFix, 3},
	{41, "CARD ACCEPTOR TERMINAL IDENTIFICACION", pChr, pFix, 8},
	{42, "CARD ACCEPTOR IDENTIFICATION CODE", pChr, pFix, 15},
	{43, "CARD ACCEPTOR NAME/LOCATION", pChr, pFix, 40},
	{44, "ADITIONAL RESPONSE DATA", pChr, pLL, 25},
	{45, "TRACK 1 DATA", pChr, pLL, 76},
	{46, "Additional data - ISO", pChr, pLLL, 999},
	{47, "Additional data - national", pChr, pLLL, 999},
	{48, "ADITIONAL DATA - PRIVATE", pChr, pLLL, 999},
	{49, "CURRENCY CODE, TRANSACTION", pChr, pFix, 3},
	{50, "CURRENCY CODE, SETTLEMENT", pChr, pFix, 3},
	{51, "Cardholder billing currency code", pChr, pFix, 3},
	{52, "PIN DATA", pBin, pFix, 8},
	{53, "SECURITY RELATED CONTROL INFORMATION", pBin, pFix, 48},
	{54, "ADDITIONAL AMOUNTS", pChr, pLLL, 120},
	{55, "Reserved for ISO use", pChr, pLLL, 999},
	{56, "RESERVED ISO", pChr, pLLL, 999},
	{57, "RESERVED NATIONAL", pChr, pFix, 3},
	{58, "RESERVED NATIONAL", pChr, pLLL, 11},
	{59, "RESERVED NATIONAL", pChr, pLLL, 999},
	{60, "RESERVED PRIVATE", pChr, pLLL, 999},
	{61, "RESERVED PRIVATE", pChr, pLLL, 999},
	{62, "RESERVED PRIVATE", pChr, pLLL, 999},
	{63, "RESERVED PRIVATE", pChr, pLLL, 999},
	{64, "RESERVED PRIVATE", pBin, pFix, 8},
	{65, "RESERVED PRIVATE", pBin, pFix, 8},
	{66, "SETTLEMENT CODE", pNum, pFix, 1},
	{67, "EXTENDED PAYMENT CODE", pNum, pFix, 2},
	{68, "Receiving institution country code", pNum, pFix, 3},
	{69, "SETTLEMENT INSTITUTION COUNTRY CODE", pNum, pFix, 3},
	{70, "NETWORK MANAGEMENT INFORMATION CODE", pNum, pFix, 3},
	{71, "MESSAGE NUMBER", pNum, pFix, 4},
	{72, "LAST MESSAGE NUMBER", pNum, pFix, 4},
	{73, "DATE ACTION", pNum, pFix, 6},
	{74, "CREDITS NUMBER", pNum, pFix, 10},
	{75, "CREDITS REVERSAL NUMBER", pNum, pFix, 10},
	{76, "DEBITS NUMBER", pNum, pFix, 10},
	{77, "DEBITS REVERSAL NUMBER", pNum, pFix, 10},
	{78, "TRANSFER NUMBER", pNum, pFix, 10},
	{79, "TRANSFER REVERSAL NUMBER", pNum, pFix, 10},
	{80, "INQUIRIES NUMBER", pNum, pFix, 10},
	{81, "AUTHORIZATION NUMBER", pNum, pFix, 10},
	{82, "CREDITS, PROCESSING FEE AMOUNT", pNum, pFix, 12},
	{83, "CREDITS, TRANSACTION FEE AMOUNT", pNum, pFix, 12},
	{84, "DEBITS, PROCESSING FEE AMOUNT", pNum, pFix, 12},
	{85, "DEBITS, TRANSACTION FEE AMOUNT", pNum, pFix, 12},
	{86, "CREDITS, AMOUNT", pNum, pFix, 16},
	{87, "CREDITS, REVERSAL AMOUNT", pNum, pFix, 16},
	{88, "DEBITS, AMOUNT", pNum, pFix, 16},
	{89, "DEBITS, REVERSAL AMOUNT", pNum, pFix, 16},
	{90, "ORIGINAL DATA ELEMENTS", pNum, pFix, 42},
	{91, "FILE UPDATE CODE", pChr, pFix, 1},
	{92, "FILE SECURITY CODE", pChr, pFix, 2},
	{93, "RESPONSE INDICATOR", pChr, pFix, 6},
	{94, "SERVICE INDICATOR", pChr, pFix, 7},
	{95, "REPLACEMENT AMOUNTS", pChr, pFix, 42},
	{96, "MESSAGE SECURITY CODE", pHex, pFix, 8},
	{97, "AMOUNT, NET SETTLEMENT", pChr, pFix, 17},
	{98, "PAYEE", pChr, pFix, 25},
	{99, "Settlement institution Id code", pNum, pLL, 11},
	{100, "RECEIVING INSTITUTION IDENT CODE", pNum, pLL, 11},
	{101, "FILE NAME", pChr, pLL, 17},
	{102, "ACCOUNT IDENTIFICATION 1", pChr, pLL, 28},
	{103, "ACCOUNT IDENTIFICATION 2", pChr, pLL, 28},
	{104, "Transaction description", pChr, pLLL, 100},
	{105, "Reserved for ISO use", pChr, pLLL, 999},
	{106, "Reserved for ISO use", pChr, pLLL, 999},
	{107, "Reserved for ISO use", pChr, pLLL, 999},
	{108, "Reserved for ISO use", pChr, pLLL, 999},
	{109, "Reserved for ISO use", pChr, pLLL, 999},
	{110, "Reserved for ISO use", pChr, pLLL, 999},
	{111, "Reserved for ISO use", pChr, pLLL, 999},
	{112, "Reserved for ISO use", pChr, pLLL, 999},
	{113, "Reserved for ISO use", pChr, pLLL, 999},
	{114, "Reserved for ISO use", pChr, pLLL, 999},
	{115, "Reserved for ISO use", pChr, pLLL, 999},
	{116, "Reserved for ISO use", pChr, pLLL, 999},
	{117, "Reserved for ISO use", pChr, pLLL, 999},
	{118, "RESERVED NATIONAL USE", pChr, pFix, 10},
	{119, "RESERVED NATIONAL USE", pChr, pFix, 10},
	{120, "RESERVED PRIVATE USE", pChr, pLLL, 999},
	{121, "RESERVED PRIVATE USE", pChr, pLLL, 999},
	{122, "RESERVED PRIVATE USE", pChr, pLLL, 999},
	{123, "RESERVED PRIVATE USE", pChr, pLLL, 999},
	{124, "RESERVED PRIVATE USE", pChr, pLLL, 999},
	{125, "RESERVED PRIVATE USE", pChr, pLLL, 40},
	{126, "RESERVED PRIVATE USE", pChr, pLLL, 999},
	{128, "MAC EXTENDED", pBin, pFix, 32},
}

// zoneSub127 is the DE 127 reserved-private subfield group (field 0 placeholder
// and field 1 bitmap are implicit in the headerless sub-schema).
var zoneSub127 = []postField{
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
	{36, "CUSTOMER ID", pChr, pLLL, 999},
	{37, "EXTENDED RESPONSE CODE", pChr, pLLL, 999},
	{38, "ADDITIONAL POS DATA CODE", pChr, pLLL, 999},
	{39, "ORIGINAL RESPONSE CODE", pChr, pLLL, 999},
	{40, "ACCOUNT TYPE QUALIFIERS", pChr, pLLL, 999},
	{41, "ORIGINATING REMOTE ADDRESS", pChr, pLLL, 999},
	{42, "TRANSACTION NUMBER", pChr, pLLL, 999},
}
