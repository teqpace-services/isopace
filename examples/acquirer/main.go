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

// Command acquirer is a minimal Isopace acquirer client: it connects to an
// issuer, sends a number of ISO 8583:1987 variant-A authorization requests over
// a request/response switch, and prints each response. Pair it with the issuer
// example.
//
//	go run ./examples/acquirer -addr 127.0.0.1:8583 -n 5 -amount 2500
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/teqpace-services/isopace/examples/posdemo"
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/link"
	"github.com/teqpace-services/isopace/mux"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8583", "issuer TCP address")
	count := flag.Int("n", 3, "number of authorizations to send")
	amount := flag.Int64("amount", 1000, "transaction amount in minor units")
	flag.Parse()

	c := iso8583.NewCodec(posdemo.Schema())
	cl, err := link.Dial("tcp", *addr)
	if err != nil {
		log.Fatalf("acquirer: dial: %v", err)
	}
	// Correlate responses to requests by DE 11 (STAN) + DE 41 (terminal).
	m := mux.New(cl, mux.FieldKeyer(c, 11, 41), mux.WithTimeout(5*time.Second))
	defer m.Close()

	for stan := 1; stan <= *count; stan++ {
		req, err := posdemo.AuthRequest{
			STAN:        stan,
			PAN:         "4012345678909",
			Terminal:    "TERM0001",
			AmountMinor: *amount,
			Currency:    "840",
		}.Encode(c)
		if err != nil {
			log.Fatalf("acquirer: encode: %v", err)
		}
		frame, err := m.Request(context.Background(), req)
		if err != nil {
			log.Printf("stan %d: request failed: %v", stan, err)
			continue
		}
		resp, err := posdemo.DecodeResponse(c, frame)
		if err != nil {
			log.Printf("stan %d: decode failed: %v", stan, err)
			continue
		}
		log.Printf("stan %d -> %s rc=%s auth=%q", resp.STAN, resp.MTI, resp.ResponseCode, resp.AuthID)
	}
}
