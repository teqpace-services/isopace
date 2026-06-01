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

package posdemo_test

import (
	"context"
	"testing"
	"time"

	"github.com/teqpace-services/isopace/examples/posdemo"
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/link"
	"github.com/teqpace-services/isopace/listener"
	"github.com/teqpace-services/isopace/mux"
)

// TestAcquirerIssuerRoundTrip wires the demo issuer behind a Listener and the
// demo acquirer through a Mux over loopback TCP, proving the codec + transport +
// switch stack carries an authorization end to end and correlates the response.
func TestAcquirerIssuerRoundTrip(t *testing.T) {
	issuer := posdemo.NewIssuer(100_00) // approve up to 100.00

	srv, err := listener.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(issuer.Handle)
	defer srv.Close()

	c := iso8583.NewCodec(posdemo.Schema())
	cl, err := link.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	m := mux.New(cl, mux.FieldKeyer(c, 11, 41), mux.WithTimeout(2*time.Second))
	defer m.Close()

	send := func(r posdemo.AuthRequest) posdemo.Response {
		t.Helper()
		req, err := r.Encode(c)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		frame, err := m.Request(context.Background(), req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		resp, err := posdemo.DecodeResponse(c, frame)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		return resp
	}

	// Approved: amount within the issuer's limit.
	ok := send(posdemo.AuthRequest{STAN: 1, PAN: "4012345678909", Terminal: "TERM0001", AmountMinor: 50_00, Currency: "840"})
	if ok.MTI != "0210" || ok.ResponseCode != posdemo.RespApproved || ok.STAN != 1 {
		t.Errorf("approved response = %+v", ok)
	}
	if ok.AuthID == "" {
		t.Errorf("approved response has no auth ID")
	}

	// Declined: amount over the limit.
	dec := send(posdemo.AuthRequest{STAN: 2, PAN: "4012345678909", Terminal: "TERM0001", AmountMinor: 500_00, Currency: "840"})
	if dec.ResponseCode != posdemo.RespInsufficientFunds || dec.STAN != 2 {
		t.Errorf("declined response = %+v", dec)
	}

	// Concurrent correlation: each STAN must get its own matching response.
	const n = 20
	results := make(chan posdemo.Response, n)
	for i := 100; i < 100+n; i++ {
		go func(stan int) {
			results <- send(posdemo.AuthRequest{STAN: stan, PAN: "4012345678909", Terminal: "TERM0001", AmountMinor: 10_00, Currency: "840"})
		}(i)
	}
	seen := map[int]bool{}
	for i := 0; i < n; i++ {
		r := <-results
		if r.ResponseCode != posdemo.RespApproved {
			t.Errorf("stan %d not approved: %+v", r.STAN, r)
		}
		seen[r.STAN] = true
	}
	if len(seen) != n {
		t.Errorf("expected %d distinct correlated responses, got %d", n, len(seen))
	}
}
