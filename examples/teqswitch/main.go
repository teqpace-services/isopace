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

// Command teqswitch demonstrates teq as a switch: a gateway listens for inbound
// transactions, routes each to an upstream host over a supervised connector,
// transforms the request on the way out and the response on the way back, and
// replies to the client. The full path is:
//
//	client -> gateway (transform request) -> connector -> host
//	client <- gateway (transform response) <----------- host
//
// Here the gateway adds a 10.00 fee and an acquirer id before forwarding, and
// stamps the response; the upstream is a posdemo issuer that approves up to
// 50.00, so the fee can flip an otherwise-approved amount to a decline.
//
//	go run ./examples/teqswitch
//
// Use -serve to keep the gateway listening after the samples so you can send your
// own transactions to it and watch the transforms live:
//
//	go run ./examples/teqswitch -serve -addr 127.0.0.1:9000   # terminal 1
//	go run ./examples/acquirer -addr 127.0.0.1:9000 -n 5      # terminal 2
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/teqpace-services/isopace/connector"
	"github.com/teqpace-services/isopace/examples/posdemo"
	"github.com/teqpace-services/isopace/gateway"
	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/link"
	"github.com/teqpace-services/isopace/listener"
	"github.com/teqpace-services/isopace/mux"
	"github.com/teqpace-services/isopace/teq"
	"github.com/teqpace-services/isopace/trace"
)

const feeMinor = 10_00 // gateway surcharge added before forwarding

func main() {
	serve := flag.Bool("serve", false, "keep the gateway listening after the sample transactions (Ctrl+C to stop)")
	addr := flag.String("addr", "127.0.0.1:0", "gateway listen address (pin a port, e.g. 127.0.0.1:9000, when using -serve)")
	flag.Parse()
	if err := run(*serve, *addr); err != nil {
		fmt.Fprintln(os.Stderr, "teqswitch:", err)
		os.Exit(1)
	}
}

func run(serve bool, addr string) error {
	// Upstream host (the issuer/processor), approving up to 50.00.
	hostAddr, closeHost := startHost(50_00)
	defer closeHost()

	c := iso8583.NewCodec(posdemo.Schema())
	keyer := mux.FieldKeyer(c, 11, 41)

	fmt.Println("== Isopace teqswitch demo: routing + transforming gateway ==")

	q := teq.New()

	// Outbound: a supervised connection to the host.
	if _, err := q.Switch(connector.Config{Name: "host", Addr: hostAddr, Keyer: keyer, Timeout: 3 * time.Second}); err != nil {
		return err
	}

	// Inbound: a gateway that routes to the host and transforms in both directions.
	gw, err := q.Gateway(gateway.Config{
		Name:  "pos",
		Addr:  addr,
		Codec: c,
		Route: func(context.Context, *iso8583.Message) (gateway.Forwarder, error) {
			if dest := q.To("host"); dest != nil {
				return dest, nil
			}
			return nil, gateway.ErrNoRoute
		},
		BeforeRequest: func(ctx context.Context, req *iso8583.Message) (*iso8583.Message, error) {
			amt, _ := iso8583.Get[int64](req, 4)
			_ = req.Set(4, amt+feeMinor)  // add a switch fee
			_ = req.Set(32, int64(99001)) // stamp the acquiring institution id
			trace.From(ctx).Step("apply fee", "amount", amt, "fee", feeMinor, "acquirer", 99001)
			return req, nil
		},
		BeforeResponse: func(ctx context.Context, _, resp *iso8583.Message) (*iso8583.Message, error) {
			_ = resp.Set(48, "ROUTED-VIA-TEQ") // stamp the response
			trace.From(ctx).Step("stamp response", "de48", "ROUTED-VIA-TEQ")
			return resp, nil
		},
		OnError: func(_ context.Context, req *iso8583.Message, _ error) (*iso8583.Message, error) {
			return declineFor(req, "91"), nil // issuer/switch inoperative
		},
		// Print the full lifecycle of every transaction (PCI-masked), in colour
		// when writing to a terminal.
		Trace:   func(t *trace.Trace) { fmt.Print(t.Describe(trace.Color(isTerminal(os.Stdout)))) },
		Timeout: 3 * time.Second,
	})
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := q.Start(ctx); err != nil {
		return err
	}
	defer q.Stop(ctx)
	waitUp(q.To("host"))

	// A client (POS/acquirer) connects to the gateway, not the host.
	fmt.Printf("\nclient -> gateway(%s) -> host\n\n", gw.Addr())
	cl, err := link.Dial("tcp", gw.Addr())
	if err != nil {
		return err
	}
	m := mux.New(cl, keyer, mux.WithTimeout(3*time.Second))
	defer m.Close()

	send(c, m, 1, 30_00) // 30.00 + fee = 40.00 <= 50.00 -> approved
	send(c, m, 2, 45_00) // 45.00 + fee = 55.00 >  50.00 -> declined by the fee

	if serve {
		fmt.Printf("\ngateway listening on %s — send your own transactions, e.g.:\n", gw.Addr())
		fmt.Printf("  go run ./examples/acquirer -addr %s -n 5 -amount 2500\n", gw.Addr())
		fmt.Println("press Ctrl+C to stop")
		sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		<-sigCtx.Done()
		fmt.Println("\nshutting down")
	}
	return nil
}

// startHost runs a posdemo issuer behind a listener and returns its address.
func startHost(limitMinor int64) (addr string, closeFn func()) {
	srv, err := listener.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	go srv.Serve(posdemo.NewIssuer(limitMinor).Handle)
	return srv.Addr().String(), func() { srv.Close() }
}

// declineFor builds a 0210 decline echoing the request's correlation fields.
func declineFor(req *iso8583.Message, rc string) *iso8583.Message {
	resp := iso8583.New(posdemo.Schema())
	_ = resp.Set(0, "0210")
	if stan, e := iso8583.Get[int64](req, 11); e == nil {
		_ = resp.Set(11, stan)
	}
	if term, e := iso8583.Get[string](req, 41); e == nil {
		_ = resp.Set(41, term)
	}
	_ = resp.Set(39, rc)
	return resp
}

func waitUp(sw *connector.Connector) {
	for i := 0; i < 300 && (sw == nil || !sw.Connected()); i++ {
		time.Sleep(10 * time.Millisecond)
	}
}

// isTerminal reports whether f is a character device (a terminal), so colour is
// only emitted interactively, not when piped to a file or log. Stdlib-only.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// send issues a purchase to the gateway and prints the (transformed) response.
func send(c *iso8583.Codec, m *mux.Mux, stan int, amountMinor int64) {
	req, err := posdemo.AuthRequest{
		STAN: stan, PAN: "4012345678909", Terminal: "TERM0001", AmountMinor: amountMinor, Currency: "566",
	}.Encode(c)
	if err != nil {
		fmt.Printf("stan %d: encode: %v\n", stan, err)
		return
	}
	frame, err := m.Request(context.Background(), req)
	if err != nil {
		fmt.Printf("stan %d -> error: %v\n", stan, err)
		return
	}
	resp, err := c.Unmarshal(frame)
	if err != nil {
		fmt.Printf("stan %d: decode: %v\n", stan, err)
		return
	}
	mti, _ := iso8583.Get[string](resp, 0)
	rc, _ := iso8583.Get[string](resp, 39)
	auth, _ := iso8583.Get[string](resp, 38)
	stamp, _ := iso8583.Get[string](resp, 48)
	fmt.Printf("stan %d sent=%d -> %s rc=%s auth=%q de48=%q\n", stan, amountMinor, mti, rc, auth, stamp)
}
