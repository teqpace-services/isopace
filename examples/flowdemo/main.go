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

// Command flowdemo runs the Isopace transaction manager (the flow package) as a
// small issuer authorization pipeline, in-process and with no network. It shows
// the two-phase model end to end: a prepare pass validates, routes by BIN, and
// reserves funds, then either a commit pass captures the funds or an abort pass
// releases the hold and carries a decline. Journaling, the per-stage profiler,
// and idempotent retransmission are wired in so each appears in the output.
//
//	go run ./examples/flowdemo
//
// The PANs below are scrubbed test values, not real card numbers.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/teqpace-services/isopace/examples/posdemo"
	"github.com/teqpace-services/isopace/flow"
	"github.com/teqpace-services/isopace/iso8583"
)

// Demo card numbers (scrubbed test data) and their behaviours.
const (
	panVisa       = "4012345678909" // approves within balance, routes to visa
	panMastercard = "5512345678901" // approves within balance, routes to mastercard
	panFlaky      = "4999999999999" // post-auth fails -> hold released on rollback
	panBlocked    = "4000000000002" // risk stage declines do-not-honor
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	p := newPipeline(log)
	p.led.seed(panVisa, 100_00)
	p.led.seed(panMastercard, 100_00)
	p.led.seed(panFlaky, 100_00)
	p.led.seed(panBlocked, 100_00)
	p.blocked[panBlocked] = true
	p.flaky[panFlaky] = true

	f := p.build()
	run := func(label string, stan int, pan string, amt int64) {
		ex := flow.NewExchange(fmt.Sprintf("TERM0001:%d", stan), authReq(stan, pan, amt))
		err := f.Run(context.Background(), ex)
		report(label, ex, err)
	}

	fmt.Println("== Isopace flow demo: issuer authorization pipeline ==")
	run("approve (visa)", 1, panVisa, 40_00)           // hold -> capture
	run("decline (no funds)", 2, panVisa, 999_00)      // hold fails, no capture
	run("rollback (postauth)", 3, panFlaky, 25_00)     // hold -> released on abort
	run("route (mastercard)", 4, panMastercard, 30_00) // BIN routes to mastercard
	run("blocked card", 5, panBlocked, 10_00)          // risk declines
	run("retransmit (replay)", 1, panVisa, 40_00)      // same key as #1 -> idempotent hit

	fmt.Println("\n== Ledger balances (minor units) ==")
	for _, pan := range []string{panVisa, panMastercard, panFlaky, panBlocked} {
		fmt.Printf("  %s available=%d\n", mask(pan), p.led.Available(pan))
	}
}

// authReq builds a 0200 purchase authorization for the demo schema.
func authReq(stan int, pan string, amt int64) *iso8583.Message {
	m := iso8583.New(posdemo.Schema())
	_ = m.Set(0, "0200")
	_ = m.Set(2, pan)
	_ = m.Set(3, "000000") // processing code: purchase
	_ = m.Set(4, amt)
	_ = m.Set(11, int64(stan))
	_ = m.Set(41, "TERM0001")
	_ = m.Set(49, "840")
	return m
}

// report prints the outcome of one exchange: the response it carries plus the
// commit/abort disposition and the per-stage profile.
func report(label string, ex *flow.Exchange, err error) {
	mti, rc, auth := "", "", ""
	if ex.Response != nil {
		mti, _ = iso8583.Get[string](ex.Response, 0)
		rc, _ = iso8583.Get[string](ex.Response, 39)
		auth, _ = iso8583.Get[string](ex.Response, 38)
	}
	status := "COMMIT"
	if err != nil {
		status = "ABORT (" + err.Error() + ")"
	}
	fmt.Printf("\n%-20s -> %s rc=%s auth=%-6q %s\n", label, mti, rc, auth, status)
	if pr := ex.Profile(); pr != "" {
		fmt.Printf("    profile: %s\n", pr)
	}
}
