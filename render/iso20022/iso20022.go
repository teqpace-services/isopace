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

// Package iso20022 bridges an ISO-8583 message to and from an ISO 20022
// pacs.008 (FIToFICstmrCdtTrf) document via a declarative DE -> element map. It
// consumes the read-only iso8583.View (ARCHITECTURE.md §8).
//
// This is a representative subset of pacs.008, mapping the common credit-
// transfer fields (amount/currency, identifiers, datetime, debtor account); DEs
// outside the map are not carried (the bridge is intentionally lossy). Currency
// is carried as the ISO-8583 numeric code in @Ccy; a numeric->alpha ISO 4217
// table is a future addition.
package iso20022

import (
	"encoding/xml"
	"errors"
	"strconv"
	"strings"

	"github.com/teqpace-services/isopace/iso8583"
)

const namespace = "urn:iso:std:iso:20022:tech:xsd:pacs.008.001.08"

// Document is the pacs.008 root.
type Document struct {
	XMLName xml.Name `xml:"Document"`
	Xmlns   string   `xml:"xmlns,attr"`
	Msg     message  `xml:"FIToFICstmrCdtTrf"`
}

type message struct {
	GrpHdr groupHeader `xml:"GrpHdr"`
	Tx     txInfo      `xml:"CdtTrfTxInf"`
}

type groupHeader struct {
	MsgID   string `xml:"MsgId"`
	CreDtTm string `xml:"CreDtTm,omitempty"`
	NbOfTxs string `xml:"NbOfTxs"`
}

type txInfo struct {
	PmtID    paymentID `xml:"PmtId"`
	Amount   amount    `xml:"IntrBkSttlmAmt"`
	DbtrAcct account   `xml:"DbtrAcct"`
}

type paymentID struct {
	InstrID    string `xml:"InstrId,omitempty"`
	EndToEndID string `xml:"EndToEndId,omitempty"`
}

type amount struct {
	Ccy   string `xml:"Ccy,attr,omitempty"`
	Value string `xml:",chardata"`
}

type account struct {
	ID accountID `xml:"Id"`
}

type accountID struct {
	Other otherID `xml:"Othr"`
}

type otherID struct {
	ID string `xml:"Id"`
}

// ErrNoAmount is returned by Unmarshal when a currency is present without a
// well-formed amount.
var ErrNoAmount = errors.New("iso20022: malformed amount")

// Marshal renders a View to a pacs.008 XML document.
func Marshal(v iso8583.View) ([]byte, error) {
	doc := Document{Xmlns: namespace}
	doc.Msg.GrpHdr.NbOfTxs = "1"

	if stan, ok := str(v, 11); ok {
		doc.Msg.GrpHdr.MsgID = stan
		doc.Msg.Tx.PmtID.InstrID = stan
	}
	if dt, ok := str(v, 7); ok {
		doc.Msg.GrpHdr.CreDtTm = dt
	}
	if rrn, ok := str(v, 37); ok {
		doc.Msg.Tx.PmtID.EndToEndID = rrn
	}
	if pan, ok := str(v, 2); ok {
		doc.Msg.Tx.DbtrAcct.ID.Other.ID = pan
	}
	if amt, ok := v.Get(4); ok {
		if d, err := amt.Decimal(); err == nil {
			doc.Msg.Tx.Amount.Value = d.String()
		}
	}
	if ccy, ok := str(v, 49); ok {
		doc.Msg.Tx.Amount.Ccy = ccy
	}

	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), body...), nil
}

// Unmarshal parses a pacs.008 document into a Message on the given schema,
// setting the mapped data elements.
func Unmarshal(data []byte, s *iso8583.Schema) (*iso8583.Message, error) {
	var doc Document
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	m := iso8583.New(s)
	set := func(de int, val string) error {
		if val == "" {
			return nil
		}
		if _, ok := s.Field(de); !ok {
			return nil
		}
		return m.Set(de, val)
	}
	if err := set(11, doc.Msg.GrpHdr.MsgID); err != nil {
		return nil, err
	}
	if err := set(7, doc.Msg.GrpHdr.CreDtTm); err != nil {
		return nil, err
	}
	if err := set(37, doc.Msg.Tx.PmtID.EndToEndID); err != nil {
		return nil, err
	}
	if err := set(2, doc.Msg.Tx.DbtrAcct.ID.Other.ID); err != nil {
		return nil, err
	}
	if err := set(49, doc.Msg.Tx.Amount.Ccy); err != nil {
		return nil, err
	}
	if v := strings.TrimSpace(doc.Msg.Tx.Amount.Value); v != "" {
		if _, ok := s.Field(4); ok {
			d, err := parseDecimal(v)
			if err != nil {
				return nil, err
			}
			if err := m.Set(4, d); err != nil {
				return nil, err
			}
		}
	}
	return m, nil
}

// str reads a DE as a string from the view.
func str(v iso8583.View, de int) (string, bool) {
	val, ok := v.Get(de)
	if !ok {
		return "", false
	}
	s, err := val.String()
	if err != nil {
		return "", false
	}
	return s, true
}

// parseDecimal parses a plain decimal string into an exact Decimal, recovering
// the scale from the fractional digit count.
func parseDecimal(s string) (iso8583.Decimal, error) {
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	intPart, frac, _ := strings.Cut(s, ".")
	digits := intPart + frac
	if digits == "" {
		return iso8583.Decimal{}, ErrNoAmount
	}
	unscaled, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return iso8583.Decimal{}, ErrNoAmount
	}
	if neg {
		unscaled = -unscaled
	}
	return iso8583.NewDecimal(unscaled, uint8(len(frac))), nil
}
