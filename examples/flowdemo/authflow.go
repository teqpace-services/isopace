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

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/teqpace-services/isopace/examples/posdemo"
	"github.com/teqpace-services/isopace/flow"
	"github.com/teqpace-services/isopace/iso8583"
)

// Response codes (ISO 8583 DE 39) used by the demo issuer.
const (
	respApproved     = posdemo.RespApproved          // "00"
	respDoNotHonor   = "05"                          // blocked / no route
	respInsufficient = posdemo.RespInsufficientFunds // "51"
	respSystemError  = "96"                          // downstream malfunction
	respFormatError  = posdemo.RespFormatError       // "30"
)

// Decline causes carried as the abort reason for each non-approval path.
var (
	errInsufficient = errors.New("insufficient funds")
	errBlocked      = errors.New("card blocked")
	errDownstream   = errors.New("downstream loyalty service unreachable")
)

// Exchange property keys passed between stages.
const (
	keyPAN     = "pan"
	keyAmount  = "amount"
	keyNetwork = "network"
)

// ledger is a tiny in-memory cardholder ledger. Hold reserves funds reversibly
// (the reservable work of the prepare phase); Capture finalises a hold in the
// commit phase; Release returns a hold during rollback. It is safe for
// concurrent use so one ledger can back many exchanges.
type ledger struct {
	mu        sync.Mutex
	available map[string]int64 // PAN -> spendable minor units
	held      map[string]int64 // PAN -> reserved-but-uncaptured minor units
}

func newLedger() *ledger {
	return &ledger{available: map[string]int64{}, held: map[string]int64{}}
}

// seed sets a card's opening available balance.
func (l *ledger) seed(pan string, minor int64) {
	l.mu.Lock()
	l.available[pan] = minor
	l.mu.Unlock()
}

// Hold reserves amt against pan, moving it from available to held. It fails with
// errInsufficient (leaving the ledger untouched) when the balance is too low.
func (l *ledger) Hold(pan string, amt int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.available[pan] < amt {
		return fmt.Errorf("%w: pan=%s need=%d have=%d", errInsufficient, mask(pan), amt, l.available[pan])
	}
	l.available[pan] -= amt
	l.held[pan] += amt
	return nil
}

// Capture finalises a previously placed hold: the held funds leave for good.
func (l *ledger) Capture(pan string, amt int64) {
	l.mu.Lock()
	l.held[pan] -= amt
	l.mu.Unlock()
}

// Release returns a previously placed hold to the available balance.
func (l *ledger) Release(pan string, amt int64) {
	l.mu.Lock()
	l.held[pan] -= amt
	l.available[pan] += amt
	l.mu.Unlock()
}

// Available reports a card's current spendable balance.
func (l *ledger) Available(pan string) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.available[pan]
}

// pipeline holds the demo issuer's shared state and builds the transaction flow.
type pipeline struct {
	led     *ledger
	idem    *flow.MemoryStore
	blocked map[string]bool // PANs the risk stage declines outright
	flaky   map[string]bool // PANs whose post-auth step fails (forces rollback)
	log     *slog.Logger
	authSeq atomic.Uint64
}

// newPipeline builds an issuer pipeline with an empty ledger; seed balances and
// the blocked/flaky sets on the returned value before calling [pipeline.build].
func newPipeline(log *slog.Logger) *pipeline {
	return &pipeline{
		led:     newLedger(),
		idem:    flow.NewMemoryStore(),
		blocked: map[string]bool{},
		flaky:   map[string]bool{},
		log:     log,
	}
}

// build assembles the two-phase authorization flow. The entry group validates
// and routes; each network group reserves funds (the two-phase participant) and
// runs a post-auth step that can fail to demonstrate rollback.
func (p *pipeline) build() *flow.Flow {
	f := flow.New(
		flow.WithLogger(p.log),
		flow.WithJournal(flow.SlogJournal{Log: p.log}),
		flow.WithProfiler(true),
		flow.WithIdempotency(p.idem, idemKey),
	)
	f.Group("authorize", p.parse(), p.risk(), p.route())
	f.Group("visa", networkStage("visa"), reserveStage{p.led, &p.authSeq}, p.postAuth())
	f.Group("mastercard", networkStage("mastercard"), reserveStage{p.led, &p.authSeq}, p.postAuth())
	return f
}

// parse decodes and validates the inbound 0200, stashing the fields later stages
// need. It is read-only, so it Skips the commit phase; a malformed request is
// declined with a format-error response and aborts the transaction.
func (p *pipeline) parse() flow.Stage {
	return flow.Func("parse", func(_ context.Context, ex *flow.Exchange) (flow.Result, error) {
		mti, _ := iso8583.Get[string](ex.Request, 0)
		pan, _ := iso8583.Get[string](ex.Request, 2)
		amt, _ := iso8583.Get[int64](ex.Request, 4)
		if mti != "0200" || pan == "" || amt <= 0 {
			ex.Response = decline(ex.Request, respFormatError)
			ex.Abort(fmt.Errorf("malformed 0200: mti=%q pan=%q amount=%d", mti, mask(pan), amt))
			return flow.Skip(), nil
		}
		ex.Set(keyPAN, pan)
		ex.Set(keyAmount, amt)
		return flow.Skip(), nil
	})
}

// risk is a read-only velocity/blocklist check; a blocked card is declined
// do-not-honor and the transaction aborts before any funds are reserved.
func (p *pipeline) risk() flow.Stage {
	return flow.Func("risk", func(_ context.Context, ex *flow.Exchange) (flow.Result, error) {
		if p.blocked[strProp(ex, keyPAN)] {
			ex.Response = decline(ex.Request, respDoNotHonor)
			ex.Abort(errBlocked)
			return flow.Skip(), nil
		}
		return flow.Skip(), nil
	})
}

// route selects the issuer network by BIN and appends that group's stages. The
// routing stage itself joins the transaction (it has no rollback of its own).
func (p *pipeline) route() flow.Stage {
	return flow.Func("route", func(_ context.Context, ex *flow.Exchange) (flow.Result, error) {
		pan := strProp(ex, keyPAN)
		switch pan[0] {
		case '4':
			return flow.Route("visa"), nil
		case '5':
			return flow.Route("mastercard"), nil
		default:
			ex.Response = decline(ex.Request, respDoNotHonor)
			ex.Abort(fmt.Errorf("no issuer route for BIN %q", pan[:1]))
			return flow.Skip(), nil
		}
	})
}

// networkStage records which network handled the transaction. It is read-only.
func networkStage(name string) flow.Stage {
	return flow.Func("net:"+name, func(_ context.Context, ex *flow.Exchange) (flow.Result, error) {
		ex.Set(keyNetwork, name)
		return flow.Skip(), nil
	})
}

// reserveStage is the two-phase participant: Prepare reserves funds (reversible),
// Commit captures them, Abort releases them if a later stage fails.
type reserveStage struct {
	led *ledger
	seq *atomic.Uint64
}

func (reserveStage) Name() string { return "reserve" }

func (s reserveStage) Prepare(_ context.Context, ex *flow.Exchange) (flow.Result, error) {
	pan, amt := strProp(ex, keyPAN), intProp(ex, keyAmount)
	if err := s.led.Hold(pan, amt); err != nil {
		// No hold was placed, so nothing to roll back: decline and Skip rather
		// than join the transaction.
		ex.Response = decline(ex.Request, respInsufficient)
		ex.Abort(err)
		return flow.Skip(), nil
	}
	// Tentative approval; made durable in Commit, undone in Abort.
	ex.Response = approve(ex.Request, s.authID())
	return flow.Continue(), nil
}

func (s reserveStage) Commit(_ context.Context, ex *flow.Exchange) error {
	s.led.Capture(strProp(ex, keyPAN), intProp(ex, keyAmount))
	return nil
}

func (s reserveStage) Abort(_ context.Context, ex *flow.Exchange) error {
	s.led.Release(strProp(ex, keyPAN), intProp(ex, keyAmount))
	return nil
}

func (s reserveStage) authID() string {
	return fmt.Sprintf("%06d", s.seq.Add(1)%1_000_000)
}

// postAuth simulates a downstream step (loyalty/notification) that runs after the
// hold. For a flaky PAN it fails, which aborts the transaction and triggers the
// reserve stage's rollback — the canonical two-phase release demonstration.
func (p *pipeline) postAuth() flow.Stage {
	return flow.Func("postauth", func(_ context.Context, ex *flow.Exchange) (flow.Result, error) {
		if p.flaky[strProp(ex, keyPAN)] {
			ex.Response = decline(ex.Request, respSystemError)
			return flow.Skip(), errDownstream
		}
		return flow.Skip(), nil
	})
}

// idemKey makes a request idempotent on its terminal (DE 41) + STAN (DE 11), so
// a retransmission replays the stored response instead of reprocessing.
func idemKey(ex *flow.Exchange) (string, bool) {
	stan, err1 := iso8583.Get[int64](ex.Request, 11)
	term, err2 := iso8583.Get[string](ex.Request, 41)
	if err1 != nil || err2 != nil {
		return "", false
	}
	return fmt.Sprintf("%s:%d", term, stan), true
}

// base0210 starts a response that echoes the request's correlation fields.
func base0210(req *iso8583.Message) *iso8583.Message {
	m := iso8583.New(req.Schema())
	_ = m.Set(0, "0210")
	if stan, err := iso8583.Get[int64](req, 11); err == nil {
		_ = m.Set(11, stan)
	}
	if term, err := iso8583.Get[string](req, 41); err == nil {
		_ = m.Set(41, term)
	}
	return m
}

// decline builds a 0210 carrying response code rc.
func decline(req *iso8583.Message, rc string) *iso8583.Message {
	m := base0210(req)
	_ = m.Set(39, rc)
	return m
}

// approve builds an approved 0210 with the authorization id in DE 38.
func approve(req *iso8583.Message, auth string) *iso8583.Message {
	m := base0210(req)
	_ = m.Set(39, respApproved)
	_ = m.Set(38, auth)
	return m
}

// strProp / intProp read a typed property left by an earlier stage.
func strProp(ex *flow.Exchange, key string) string {
	v, _ := ex.Get(key)
	s, _ := v.(string)
	return s
}

func intProp(ex *flow.Exchange, key string) int64 {
	v, _ := ex.Get(key)
	n, _ := v.(int64)
	return n
}

// mask hides all but the last four PAN digits for logs and errors.
func mask(pan string) string {
	if len(pan) <= 4 {
		return pan
	}
	return "****" + pan[len(pan)-4:]
}
