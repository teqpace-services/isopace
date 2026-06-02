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

package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"testing"

	"github.com/teqpace-services/isopace/flow"
	"github.com/teqpace-services/isopace/iso8583"
)

func quietPipeline(t *testing.T) *pipeline {
	t.Helper()
	return newPipeline(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// runAuth drives one authorization through a freshly built flow and returns the
// exchange plus the run error.
func runAuth(t *testing.T, p *pipeline, stan int, pan string, amt int64) (*flow.Exchange, error) {
	t.Helper()
	ex := flow.NewExchange("TERM0001:"+strconv.Itoa(stan), authReq(stan, pan, amt))
	err := p.build().Run(context.Background(), ex)
	return ex, err
}

func respCode(t *testing.T, ex *flow.Exchange) string {
	t.Helper()
	if ex.Response == nil {
		t.Fatal("exchange has no response")
	}
	rc, _ := iso8583.Get[string](ex.Response, 39)
	return rc
}

// TestApproveCaptures: a transaction within balance commits and the held funds
// are captured (available drops by the amount, exactly once).
func TestApproveCaptures(t *testing.T) {
	p := quietPipeline(t)
	p.led.seed("4012345678909", 100_00)

	ex, err := runAuth(t, p, 1, "4012345678909", 40_00)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rc := respCode(t, ex); rc != respApproved {
		t.Errorf("rc = %q want %q", rc, respApproved)
	}
	if auth, _ := iso8583.Get[string](ex.Response, 38); auth == "" {
		t.Error("approved response has no auth id")
	}
	if got := p.led.Available("4012345678909"); got != 60_00 {
		t.Errorf("available = %d want 6000 (captured)", got)
	}
}

// TestInsufficientFundsDeclines: the hold fails, nothing is captured, and the
// balance is untouched.
func TestInsufficientFundsDeclines(t *testing.T) {
	p := quietPipeline(t)
	p.led.seed("4012345678909", 50_00)

	ex, err := runAuth(t, p, 2, "4012345678909", 999_00)
	if !errors.Is(err, errInsufficient) {
		t.Fatalf("err = %v want errInsufficient", err)
	}
	if rc := respCode(t, ex); rc != respInsufficient {
		t.Errorf("rc = %q want %q", rc, respInsufficient)
	}
	if got := p.led.Available("4012345678909"); got != 50_00 {
		t.Errorf("available = %d want 5000 (untouched)", got)
	}
}

// TestRollbackReleasesHold: a post-auth failure aborts after the hold, and the
// reserve stage's Abort releases it, restoring the opening balance.
func TestRollbackReleasesHold(t *testing.T) {
	p := quietPipeline(t)
	p.led.seed("4999999999999", 100_00)
	p.flaky["4999999999999"] = true

	ex, err := runAuth(t, p, 3, "4999999999999", 25_00)
	if !errors.Is(err, errDownstream) {
		t.Fatalf("err = %v want errDownstream", err)
	}
	if rc := respCode(t, ex); rc != respSystemError {
		t.Errorf("rc = %q want %q", rc, respSystemError)
	}
	if got := p.led.Available("4999999999999"); got != 100_00 {
		t.Errorf("available = %d want 10000 (hold released)", got)
	}
}

// TestRoutingByBIN: a Mastercard BIN routes through the mastercard group, which
// tags the exchange before reserving.
func TestRoutingByBIN(t *testing.T) {
	p := quietPipeline(t)
	p.led.seed("5512345678901", 100_00)

	ex, err := runAuth(t, p, 4, "5512345678901", 30_00)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rc := respCode(t, ex); rc != respApproved {
		t.Errorf("rc = %q want %q", rc, respApproved)
	}
	if net := strProp(ex, keyNetwork); net != "mastercard" {
		t.Errorf("network = %q want mastercard", net)
	}
}

// TestBlockedCardDeclines: the risk stage declines a blocklisted PAN before any
// funds are reserved.
func TestBlockedCardDeclines(t *testing.T) {
	p := quietPipeline(t)
	p.led.seed("4000000000002", 100_00)
	p.blocked["4000000000002"] = true

	ex, err := runAuth(t, p, 5, "4000000000002", 10_00)
	if !errors.Is(err, errBlocked) {
		t.Fatalf("err = %v want errBlocked", err)
	}
	if rc := respCode(t, ex); rc != respDoNotHonor {
		t.Errorf("rc = %q want %q", rc, respDoNotHonor)
	}
	if got := p.led.Available("4000000000002"); got != 100_00 {
		t.Errorf("available = %d want 10000 (untouched)", got)
	}
}

// TestIdempotentRetransmit: a retransmission (same terminal+STAN) replays the
// stored response without reprocessing, so the ledger is debited only once.
func TestIdempotentRetransmit(t *testing.T) {
	p := quietPipeline(t)
	p.led.seed("4012345678909", 100_00)
	f := p.build()

	first := flow.NewExchange("TERM0001:7", authReq(7, "4012345678909", 40_00))
	if err := f.Run(context.Background(), first); err != nil {
		t.Fatalf("first Run: %v", err)
	}

	replay := flow.NewExchange("TERM0001:7", authReq(7, "4012345678909", 40_00))
	if err := f.Run(context.Background(), replay); err != nil {
		t.Fatalf("replay Run: %v", err)
	}
	if rc := respCode(t, replay); rc != respApproved {
		t.Errorf("replay rc = %q want %q", rc, respApproved)
	}
	if got := p.led.Available("4012345678909"); got != 60_00 {
		t.Errorf("available = %d want 6000 (debited once, not twice)", got)
	}
}

// TestMalformedRequestDeclines: a non-0200 request is rejected with a format
// error before routing.
func TestMalformedRequestDeclines(t *testing.T) {
	p := quietPipeline(t)
	m := iso8583.New(authReq(8, "4012345678909", 10_00).Schema())
	_ = m.Set(0, "0800") // network management, not an auth request
	ex := flow.NewExchange("TERM0001:8", m)

	if err := p.build().Run(context.Background(), ex); err == nil {
		t.Fatal("Run err = nil want abort")
	}
	if rc := respCode(t, ex); rc != respFormatError {
		t.Errorf("rc = %q want %q", rc, respFormatError)
	}
}
