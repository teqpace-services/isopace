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

// Command playground is a hands-on scratch tool for the Isopace ISO-8583 core.
// It does both directions:
//
//	PACK   — build a message from fields (flags below) and show the wire bytes.
//	UNPACK — decode an ISO-8583 message string and show every field.
//
// Build a message and round-trip it (PACK, then UNPACK the bytes we just made):
//
//	go run ./examples/playground -pan 4111111111111111 -amount 9900 -stan 42
//
// Unpack a message string someone handed you (hex; spaces are ignored):
//
//	go run ./examples/playground -hex "30323030..."
//
// Edit the set(...) calls in PACK to add more data elements; everything is
// driven by the ISO 8583:1987 variant-A profile (packager.ISO87A()).
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/packager"
	"github.com/teqpace-services/isopace/render/jsonio"
)

func main() {
	hexIn := flag.String("hex", "", "ISO-8583 message as a hex string to UNPACK (spaces ok); empty = round-trip the built message")
	mti := flag.String("mti", "0200", "DE 0  message type indicator")
	pan := flag.String("pan", "4012345678909", "DE 2  primary account number")
	proc := flag.String("proc", "000000", "DE 3  processing code")
	amount := flag.Int64("amount", 2500, "DE 4  amount, minor units")
	stan := flag.Int64("stan", 1, "DE 11 system trace audit number")
	term := flag.String("term", "TERM0001", "DE 41 terminal id (8 chars)")
	currency := flag.String("currency", "840", "DE 49 currency (ISO 4217 numeric)")
	unmask := flag.Bool("unmask", false, "show sensitive fields (PAN, track, PIN) in full instead of masked")
	showRaw := flag.Bool("raw", false, "add a raw-hex column to the field dump")
	flag.Parse()

	// Diagnostic options for the built-in iso8583.Describe dump.
	var dopts []iso8583.DescribeOption
	if *unmask {
		dopts = append(dopts, iso8583.Unmasked())
	}
	if *showRaw {
		dopts = append(dopts, iso8583.WithRaw())
	}

	schema := packager.ISO87A()
	codec := iso8583.NewCodec(schema)

	// ───────── PACK: build a message from fields ─────────
	m := iso8583.New(schema)
	set := func(de int, v any) {
		if err := m.Set(de, v); err != nil {
			fmt.Fprintf(os.Stderr, "set DE %d: %v\n", de, err)
			os.Exit(1)
		}
	}
	set(0, *mti)       // message type indicator
	set(2, *pan)       // PAN
	set(3, *proc)      // processing code
	set(4, *amount)    // amount
	set(11, *stan)     // STAN
	set(41, *term)     // terminal id
	set(49, *currency) // currency

	wire, err := codec.Marshal(m, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal:", err)
		os.Exit(1)
	}

	fmt.Println("════════════════ PACK  (build → wire) ════════════════")
	fmt.Printf("profile : %s\n", schema.ID())
	fmt.Printf("length  : %d bytes\n", len(wire))
	fmt.Printf("hex     : %s\n", strings.ToUpper(hex.EncodeToString(wire)))
	fmt.Printf("ascii   : %q\n", string(wire))
	iso8583.Describe(os.Stdout, m, dopts...)
	if j, err := jsonio.Marshal(m, jsonio.UseNames()); err == nil {
		fmt.Printf("json    : %s\n", j)
	}

	// ───────── UNPACK: decode a message string ─────────
	raw := wire // default: round-trip what we just built
	if s := strings.ReplaceAll(strings.TrimSpace(*hexIn), " ", ""); s != "" {
		raw, err = hex.DecodeString(s)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bad -hex (expected hex bytes):", err)
			os.Exit(1)
		}
	}

	fmt.Println("\n════════════════ UNPACK  (wire → fields) ════════════════")
	fmt.Printf("input   : %s\n", strings.ToUpper(hex.EncodeToString(raw)))
	dec, err := codec.Unmarshal(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "unmarshal:", err)
		os.Exit(1)
	}
	iso8583.Describe(os.Stdout, dec, dopts...)
}
