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

// Command teq demonstrates the teq container: start it from code, register
// outbound switch connections that stay open and reconnect on their own, and
// send transactions through them by name. Here two in-process issuers stand in
// for upstream interchange switches (Interswitch and UP); in production the
// addresses would be the real hosts and the connections would be long-lived
// (use q.ListenAndServe() to run as a daemon).
//
//	go run ./examples/teq
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/teqpace-services/isopace/connector"
	"github.com/teqpace-services/isopace/examples/posdemo"
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/listener"
	"github.com/teqpace-services/isopace/mux"
	"github.com/teqpace-services/isopace/teq"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "teq demo:", err)
		os.Exit(1)
	}
}

func run() error {
	// Stand up two upstream switches (stand-ins for Interswitch and UP), each an
	// ISO 8583:1987-A issuer with its own approval limit.
	iswAddr, closeISW := startSwitch(100_00) // approves up to 100.00
	defer closeISW()
	upAddr, closeUP := startSwitch(50_00) // approves up to 50.00
	defer closeUP()

	c := iso8583.NewCodec(posdemo.Schema())
	keyer := mux.FieldKeyer(c, 11, 41) // correlate by STAN + terminal

	fmt.Println("== Isopace teq demo: container with self-healing switch links ==")

	// Start the container and register the two switches — a few lines, Q2-style.
	q := teq.New()
	if _, err := q.Switch(connector.Config{Name: "isw", Addr: iswAddr, Keyer: keyer, Timeout: 3 * time.Second}); err != nil {
		return err
	}
	if _, err := q.Switch(connector.Config{Name: "up", Addr: upAddr, Keyer: keyer, Timeout: 3 * time.Second}); err != nil {
		return err
	}

	ctx := context.Background()
	if err := q.Start(ctx); err != nil {
		return err
	}
	defer q.Stop(ctx)

	// The connectors dial in the background; wait for both links to come up.
	for _, name := range q.Switches() {
		waitUp(q.To(name))
	}
	fmt.Printf("\nswitches up: %v\n\n", q.Switches())

	// Route transactions to switches by name.
	send(c, q.To("isw"), 1, 40_00) // ISW approves (<= 100.00)
	send(c, q.To("up"), 2, 75_00)  // UP declines (> 50.00)
	send(c, q.To("up"), 3, 20_00)  // UP approves

	fmt.Println("\n(connections stay open and reconnect automatically; a daemon would call q.ListenAndServe())")
	return nil
}

// startSwitch runs a posdemo issuer behind a listener and returns its address
// plus a closer.
func startSwitch(limitMinor int64) (addr string, closeFn func()) {
	srv, err := listener.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	go srv.Serve(posdemo.NewIssuer(limitMinor).Handle)
	return srv.Addr().String(), func() { srv.Close() }
}

// waitUp blocks briefly until the connector reports connected.
func waitUp(sw *connector.Connector) {
	for i := 0; i < 300 && (sw == nil || !sw.Connected()); i++ {
		time.Sleep(10 * time.Millisecond)
	}
}

// send encodes a purchase, sends it through sw, and prints the response.
func send(c *iso8583.Codec, sw *connector.Connector, stan int, amountMinor int64) {
	req, err := posdemo.AuthRequest{
		STAN: stan, PAN: "4012345678909", Terminal: "TERM0001", AmountMinor: amountMinor, Currency: "840",
	}.Encode(c)
	if err != nil {
		fmt.Printf("stan %d: encode: %v\n", stan, err)
		return
	}
	frame, err := sw.Request(context.Background(), req)
	if err != nil {
		fmt.Printf("stan %d via %s -> error: %v\n", stan, sw.Name(), err)
		return
	}
	resp, err := posdemo.DecodeResponse(c, frame)
	if err != nil {
		fmt.Printf("stan %d: decode: %v\n", stan, err)
		return
	}
	fmt.Printf("stan %d via %-3s -> %s rc=%s auth=%q\n", stan, sw.Name(), resp.MTI, resp.ResponseCode, resp.AuthID)
}
