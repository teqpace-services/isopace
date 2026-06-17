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
	"github.com/teqpace-services/isopace/lengthcodec"
)

// iso87Dir is the ISO 8583:1987 data-element directory, representation-
// independent. Each variant (A/B/C) builds it with a different rep. It is a
// representative working subset of the 1987 directory, not the full 128 DEs.
var iso87Dir = []fieldSpec{
	{2, "Primary Account Number", tNum, lLL, 19, 0},
	{3, "Processing Code", tNum, lFixed, 6, 0},
	{4, "Amount, Transaction", tAmount, lFixed, 12, 0},
	{5, "Amount, Settlement", tAmount, lFixed, 12, 0},
	{6, "Amount, Cardholder Billing", tAmount, lFixed, 12, 0},
	{7, "Transmission Date and Time", tNum, lFixed, 10, 0},
	{9, "Conversion Rate, Settlement", tNum, lFixed, 8, 0},
	{10, "Conversion Rate, Cardholder Billing", tNum, lFixed, 8, 0},
	{11, "System Trace Audit Number", tNum, lFixed, 6, 0},
	{12, "Local Transaction Time", tNum, lFixed, 6, 0},
	{13, "Local Transaction Date", tNum, lFixed, 4, 0},
	{14, "Expiration Date", tNum, lFixed, 4, 0},
	{15, "Settlement Date", tNum, lFixed, 4, 0},
	{18, "Merchant Category Code", tNum, lFixed, 4, 0},
	{19, "Acquiring Institution Country Code", tNum, lFixed, 3, 0},
	{22, "POS Entry Mode", tNum, lFixed, 3, 0},
	{23, "Card Sequence Number", tNum, lFixed, 3, 0},
	{25, "POS Condition Code", tNum, lFixed, 2, 0},
	{32, "Acquiring Institution ID Code", tNum, lLL, 11, 0},
	{33, "Forwarding Institution ID Code", tNum, lLL, 11, 0},
	{35, "Track 2 Data", tBinary, lLL, 37, 0},
	{37, "Retrieval Reference Number", tAlpha, lFixed, 12, 0},
	{38, "Authorization ID Response", tAlpha, lFixed, 6, 0},
	{39, "Response Code", tAlpha, lFixed, 2, 0},
	{41, "Card Acceptor Terminal ID", tAlpha, lFixed, 8, 0},
	{42, "Card Acceptor ID Code", tAlpha, lFixed, 15, 0},
	{43, "Card Acceptor Name/Location", tAlpha, lFixed, 40, 0},
	{48, "Additional Data - Private", tAlpha, lLLL, 999, 0},
	{49, "Currency Code, Transaction", tNum, lFixed, 3, 0},
	{52, "PIN Data", tBinary, lFixed, 8, 0},
	{53, "Security Related Control Information", tNum, lFixed, 16, 0},
	{55, "ICC Data", tEMV, lLLL, 999, 0},
	{60, "Reserved Private", tAlpha, lLLL, 999, 0},
	{62, "Reserved Private", tAlpha, lLLL, 999, 0},
	{63, "Reserved Private", tAlpha, lLLL, 999, 0},
	{64, "Message Authentication Code", tBinary, lFixed, 8, 0},
	{70, "Network Management Information Code", tNum, lFixed, 3, 0},
	{90, "Original Data Elements", tNum, lFixed, 42, 0},
	{100, "Receiving Institution ID Code", tNum, lLL, 11, 0},
	{128, "Message Authentication Code", tBinary, lFixed, 8, 0},
}

// repASCII (variant A): ASCII text, ASCII length prefixes, ASCII-hex bitmap.
func repASCII() rep {
	return rep{
		num: fieldcodec.NumASCII, alpha: fieldcodec.ASCII,
		amount: fieldcodec.AmountASCII, binary: fieldcodec.BINARY,
		lenLL: lengthcodec.LLVarASCII, lenLLL: lengthcodec.LLLVarASCII,
		bitmap: fieldcodec.BitmapHex, levels: 2, mti: fieldcodec.NumASCII,
	}
}

// repBCD (variant B): packed BCD numerics/amounts, BCD length prefixes, binary
// bitmap. Alphanumeric fields stay ASCII.
func repBCD() rep {
	return rep{
		num: fieldcodec.BCD, alpha: fieldcodec.ASCII,
		amount: fieldcodec.AmountBCD, binary: fieldcodec.BINARY,
		lenLL: lengthcodec.LLVarBCD, lenLLL: lengthcodec.LLLVarBCD,
		bitmap: fieldcodec.BitmapBinary, levels: 2, mti: fieldcodec.BCD,
	}
}

// repEBCDIC (variant C): EBCDIC text/numerics, EBCDIC length prefixes, EBCDIC
// bitmap. Amounts use the ASCII amount codec (the catalog has no amount.ebcdic;
// an EBCDIC amount codec is a future catalog addition).
func repEBCDIC() rep {
	return rep{
		num: fieldcodec.NumEBCDIC, alpha: fieldcodec.EBCDIC037,
		amount: fieldcodec.AmountASCII, binary: fieldcodec.BINARY,
		lenLL: lengthcodec.LLVarEBCDIC, lenLLL: lengthcodec.LLLVarEBCDIC,
		bitmap: fieldcodec.BitmapEBCDIC, levels: 2, mti: fieldcodec.NumEBCDIC,
	}
}

// ISO87A returns the ISO 8583:1987 variant A (ASCII) profile.
func ISO87A() *iso8583.Schema { return build("iso87-a", iso87Dir, repASCII()) }

// ISO87B returns the ISO 8583:1987 variant B (binary / BCD) profile.
func ISO87B() *iso8583.Schema { return build("iso87-b", iso87Dir, repBCD()) }

// ISO87C returns the ISO 8583:1987 variant C (EBCDIC) profile.
func ISO87C() *iso8583.Schema { return build("iso87-c", iso87Dir, repEBCDIC()) }
