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

// Package posdemo is the shared logic behind the Isopace example programs (the
// acquirer client, the issuer host, and the simulator). It builds ISO 8583:1987
// variant-A authorization messages, implements a tiny issuer authorization
// decision, and decodes responses — enough to exercise the codec and transport
// layers end to end. It is example/demonstration code, not part of the shipped
// framework API.
package posdemo

import (
	"fmt"
	"sync/atomic"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/link"
	"github.com/teqpace-services/isopace/packager"
)

// Response codes (ISO 8583 DE 39).
const (
	RespApproved          = "00"
	RespInsufficientFunds = "51"
	RespFormatError       = "30"
)

// Schema returns the ISO 8583:1987 variant-A profile used by the examples.
func Schema() *iso8583.Schema { return packager.ISO87A() }

// AuthRequest is the small set of fields the demo acquirer populates in a 0200.
type AuthRequest struct {
	STAN        int    // DE 11, system trace audit number
	PAN         string // DE 2, primary account number
	Terminal    string // DE 41, card acceptor terminal ID (8 chars)
	AmountMinor int64  // DE 4, transaction amount in minor units
	Currency    string // DE 49, ISO 4217 numeric currency code (e.g. "840")
}

// Encode builds and marshals a 0200 authorization request. It returns an error
// for invalid field values (the inputs are caller/flag-supplied, not trusted).
func (r AuthRequest) Encode(c *iso8583.Codec) ([]byte, error) {
	m := iso8583.New(Schema())
	fields := []struct {
		de int
		v  any
	}{
		{0, "0200"},
		{2, r.PAN},
		{3, "000000"}, // processing code: purchase
		{4, r.AmountMinor},
		{11, int64(r.STAN)},
		{41, r.Terminal},
		{49, r.Currency},
	}
	for _, f := range fields {
		if err := m.Set(f.de, f.v); err != nil {
			return nil, fmt.Errorf("posdemo: set DE %d: %w", f.de, err)
		}
	}
	return c.Marshal(m, nil)
}

// Response is the decoded outcome of an authorization.
type Response struct {
	MTI          string
	STAN         int
	ResponseCode string
	AuthID       string
}

// DecodeResponse decodes a 0210 response frame.
func DecodeResponse(c *iso8583.Codec, frame []byte) (Response, error) {
	m, err := c.Unmarshal(frame)
	if err != nil {
		return Response{}, err
	}
	mti, _ := iso8583.Get[string](m, 0)
	stan, _ := iso8583.Get[int64](m, 11)
	rc, _ := iso8583.Get[string](m, 39)
	auth, _ := iso8583.Get[string](m, 38)
	return Response{MTI: mti, STAN: int(stan), ResponseCode: rc, AuthID: auth}, nil
}

// Issuer is the demo authorizing party: it answers 0200 requests with 0210
// responses, approving up to ApproveLimitMinor (0 means approve everything).
type Issuer struct {
	codec   *iso8583.Codec
	limit   int64
	authSeq atomic.Uint64
}

// NewIssuer builds an issuer that approves transactions up to approveLimitMinor
// (0 = approve all).
func NewIssuer(approveLimitMinor int64) *Issuer {
	return &Issuer{codec: iso8583.NewCodec(Schema()), limit: approveLimitMinor}
}

// Respond decodes a request frame and produces a response frame. It echoes the
// correlation fields (DE 11 STAN, DE 41 terminal) so the acquirer's switch can
// match the response, and sets DE 39 (and DE 38 on approval).
func (i *Issuer) Respond(req []byte) ([]byte, error) {
	m, err := i.codec.Unmarshal(req)
	if err != nil {
		return i.errorResponse(0, "")
	}
	stan, _ := iso8583.Get[int64](m, 11)
	term, _ := iso8583.Get[string](m, 41)

	resp := iso8583.New(Schema())
	_ = resp.Set(0, "0210")
	_ = resp.Set(11, stan)
	_ = resp.Set(41, term)

	code := RespApproved
	if i.limit > 0 {
		if amt, err := iso8583.Get[int64](m, 4); err == nil && amt > i.limit {
			code = RespInsufficientFunds
		}
	}
	_ = resp.Set(39, code)
	if code == RespApproved {
		_ = resp.Set(38, i.nextAuthID())
	}
	return i.codec.Marshal(resp, nil)
}

// errorResponse builds a 0210 carrying a format-error response code.
func (i *Issuer) errorResponse(stan int64, term string) ([]byte, error) {
	resp := iso8583.New(Schema())
	_ = resp.Set(0, "0210")
	_ = resp.Set(11, stan)
	if term != "" {
		_ = resp.Set(41, term)
	}
	_ = resp.Set(39, RespFormatError)
	return i.codec.Marshal(resp, nil)
}

func (i *Issuer) nextAuthID() string {
	return fmt.Sprintf("%06d", i.authSeq.Add(1)%1_000_000)
}

// Handle serves one connection: it answers each received request until the link
// closes. It satisfies listener.Handler.
func (i *Issuer) Handle(l *link.Link) {
	for {
		req, err := l.Receive()
		if err != nil {
			return
		}
		resp, err := i.Respond(req)
		if err != nil {
			return
		}
		if err := l.Send(resp); err != nil {
			return
		}
	}
}
