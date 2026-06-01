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

// Command issuer is a minimal Isopace issuer host: it accepts ISO 8583:1987
// variant-A authorization requests (0200) and answers them with 0210 responses,
// approving up to an optional limit. Pair it with the acquirer example.
//
//	go run ./examples/issuer -addr 127.0.0.1:8583 -limit 10000
package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/teqpace-services/isopace/examples/posdemo"
	"github.com/teqpace-services/isopace/listener"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8583", "TCP address to listen on")
	limit := flag.Int64("limit", 0, "approval limit in minor units (0 = approve all)")
	flag.Parse()

	issuer := posdemo.NewIssuer(*limit)
	srv, err := listener.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("issuer: listen: %v", err)
	}
	go func() {
		if err := srv.Serve(issuer.Handle); err != nil {
			log.Printf("issuer: serve stopped: %v", err)
		}
	}()
	log.Printf("issuer listening on %s (approve limit %d minor units)", srv.Addr(), *limit)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	log.Print("issuer shutting down")
	if err := srv.Close(); err != nil {
		log.Printf("issuer: close: %v", err)
	}
}
