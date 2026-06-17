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

// Command playground is a hands-on scratch tool for the Isopace ISO-8583 core,
// built on the packager.Postilion() profile (binary bitmap + DE 127 subfield
// group). It can PACK a message, UNPACK a wire string, and SEND to a live host.
// The sample field values below are scrubbed test data, not real card data.
//
// Round-trip the built cashout (pack, then unpack its own bytes):
//
//	go run ./examples/playground
//
// Unpack a message string someone handed you (hex; spaces ignored):
//
//	go run ./examples/playground -msg unpack -hex "30323030..."
//
// Send to a Postilion-style host (sign on first on the same connection):
//
//	go run ./examples/playground -msg cashout -signon -fresh -send 192.168.21.5:9000
//
// Edit reqFields / sub127 / signonFields to change the messages; -lenbytes sets
// the TCP length prefix (2 = Postilion default), -header prepends a channel header.
package main

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/link"
	"github.com/teqpace-services/isopace/packager"
)

// reqFields is the top-level data-element table (DE -> value), taken verbatim
// from the captured outgoing message. Values are strings; numeric fields accept
// digit strings, textual fields are right-space-padded to their fixed width.
var reqFields = []struct {
	de  int
	val string
}{
	{2, "5399990000000000"}, // test PAN (scrubbed)
	{3, "500000"},
	{4, "000000005000"},
	{7, "0601142407"},
	{11, "000029"},
	{12, "142407"},
	{13, "0601"},
	{14, "2901"},
	{15, "0601"},
	{18, "6010"},
	{22, "051"},
	{23, "000"},
	{25, "00"},
	{26, "12"},
	{28, "D00000000"},
	{30, "C00000000"},
	{32, "539941"},
	{33, "111111"},
	{35, "5399990000000000D2901201000000000000"}, // track 2 (scrubbed)
	{37, "320247878921"},
	{40, "201"},
	{41, "2CBR0059"},
	{42, "2CBR0125SL00001"},
	{43, "TEST CARDHOLDER NAME               LA NG"}, // scrubbed
	{49, "566"},
	{56, "1510"},
	{98, "3FAB0001                 "},
	{100, "666035"},
	{103, "0126232372"},
	{123, "510111511344101"},
}

// sub127 is the DE 127 reserved-private subfield group (127.N -> value).
var sub127 = []struct {
	sub int
	val string
}{
	{2, "0200:415495:1207193655:787755594"},
	{3, "AGENCY2scr  TEPSWTsnk   000029000029NOTTEPVPAYMC"},
	{13, "  000000      566"},
	{20, "20260601"},
	{22, "212ORIGINAL_RID235<ORIGINAL_RID>627821</ORIGINAL_RID>"},
	{25, "<IccData><IccRequest><AmountAuthorized>000000005000</AmountAuthorized><AmountOther>000000000000</AmountOther><ApplicationInterchangeProfile>3900</ApplicationInterchangeProfile><ApplicationTransactionCounter>005A</ApplicationTransactionCounter><Cryptogram>0000000000000000</Cryptogram><CryptogramInformationData>80</CryptogramInformationData><CvmResults>440302</CvmResults><IssuerApplicationData>0110A74003020000000000000000000000FF</IssuerApplicationData><TerminalCapabilities>E0F0C8</TerminalCapabilities><TerminalCountryCode>566</TerminalCountryCode><TerminalVerificationResult>0080000000</TerminalVerificationResult><TerminalType>22</TerminalType><TransactionCurrencyCode>566</TransactionCurrencyCode><TransactionDate>260601</TransactionDate><TransactionType>00</TransactionType><UnpredictableNumber>9A99E0E5</UnpredictableNumber></IccRequest></IccData>"},
	{33, "6008"},
	{41, "10.2.103.19,36065"},
}

// signonFields is the 0800 network sign-on (Interswitch: DE 61 = ISW, DE 70 =
// 301). Packed with the same schema as the cashout.
var signonFields = []struct {
	de  int
	val string
}{
	{7, "0601180247"},
	{11, "180247"},
	{12, "180247"},
	{13, "0601"},
	{61, "ISW"},
	{70, "301"},
}

// buildMessage builds the named message (signon=0800, cashout=0200) on schema.
func buildMessage(schema *iso8583.Schema, kind string) *iso8583.Message {
	m := iso8583.New(schema)
	set := func(label string, err error) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ! %s: %v\n", label, err)
		}
	}
	switch kind {
	case "signon":
		set("MTI", m.Set(0, "0800"))
		for _, f := range signonFields {
			set(fmt.Sprintf("DE %d", f.de), m.Set(f.de, f.val))
		}
	case "cashout":
		set("MTI", m.Set(0, "0200"))
		for _, f := range reqFields {
			set(fmt.Sprintf("DE %d", f.de), m.Set(f.de, f.val))
		}
		for _, f := range sub127 {
			set(fmt.Sprintf("DE 127.%d", f.sub), m.SetS(fmt.Sprintf("127.%d", f.sub), f.val))
		}
	}
	return m
}

// withHeader prepends an optional channel header to the packed message.
func withHeader(wire, header []byte) []byte {
	if len(header) == 0 {
		return wire
	}
	return append(append([]byte{}, header...), wire...)
}

// exchange sends payload on cl and returns the next non-empty frame, bounded by
// timeout. Zero-length frames are PostChannel 0x0000 keep-alives (a host sends
// them while forwarding a transaction) — skip them and keep reading.
func exchange(cl *link.Link, payload []byte, timeout time.Duration) ([]byte, error) {
	if err := cl.Send(payload); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	type result struct {
		frame []byte
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		for {
			frame, err := cl.Receive()
			if err != nil {
				ch <- result{nil, err}
				return
			}
			if len(frame) == 0 {
				fmt.Println("  · keep-alive (0x0000), still waiting...")
				continue
			}
			ch <- result{frame, nil}
			return
		}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.frame, r.err
	}
}

// showReply prints a reply frame's hex, decoded fields, and DE 39.
func showReply(codec *iso8583.Codec, frame []byte, dopts []iso8583.DescribeOption) {
	fmt.Printf("got %d bytes\nhex     : %s\n\n", len(frame), strings.ToUpper(hex.EncodeToString(frame)))
	resp, err := codec.Unmarshal(frame)
	if err != nil {
		fmt.Fprintln(os.Stderr, "decode reply:", err)
		return
	}
	_ = iso8583.Describe(os.Stdout, resp, dopts...)
	if rc, err := iso8583.Get[string](resp, 39); err == nil {
		fmt.Printf("\n>>> RESPONSE CODE (DE 39) = %q\n", rc)
	}
}

func main() {
	hexIn := flag.String("hex", "", "ISO-8583 message as a hex string to UNPACK (spaces ok); empty = round-trip the built message")
	send := flag.String("send", "", "host:port to SEND the built message to (e.g. 192.168.21.5:9000); empty = don't send")
	lenbytes := flag.Int("lenbytes", 2, "TCP length-prefix width in bytes (2 or 4)")
	timeout := flag.Duration("timeout", 120*time.Second, "reply-wait timeout (default 120s)")
	dialTimeout := flag.Duration("dialtimeout", 10*time.Second, "TCP dial timeout (fail fast if unreachable)")
	header := flag.String("header", "", "hex channel header prepended before the MTI (inside the length frame); empty = none")
	useTLS := flag.Bool("tls", false, "connect over TLS instead of plain TCP")
	tlsInsecure := flag.Bool("tls-insecure", false, "skip TLS certificate verification (self-signed/test hosts)")
	tlsServerName := flag.String("tls-servername", "", "TLS server name / SNI (default: host from -send)")
	msg := flag.String("msg", "cashout", "which message: signon (0800), cashout (0200), or unpack (decode -hex only)")
	signon := flag.Bool("signon", false, "before sending, sign on (0800) and wait for the 0810 on the SAME connection")
	fresh := flag.Bool("fresh", false, "cashout: set DE 7/11/12/13/37 to current STAN/RRN/timestamps so it isn't a replay")
	unmask := flag.Bool("unmask", false, "show sensitive fields (PAN, track, PIN) in full instead of masked")
	showRaw := flag.Bool("raw", false, "add a raw-hex column to the field dump")
	flag.Parse()

	var dopts []iso8583.DescribeOption
	if *unmask {
		dopts = append(dopts, iso8583.Unmasked())
	}
	if *showRaw {
		dopts = append(dopts, iso8583.WithRaw())
	}

	schema := packager.Postilion()
	codec := iso8583.NewCodec(schema)

	// ───────── UNPACK-only mode: decode a -hex string and stop ─────────
	if *msg == "unpack" {
		s := strings.ReplaceAll(strings.TrimSpace(*hexIn), " ", "")
		if s == "" {
			fmt.Fprintln(os.Stderr, "-msg unpack needs -hex <message string>")
			os.Exit(1)
		}
		raw, derr := hex.DecodeString(s)
		if derr != nil {
			fmt.Fprintln(os.Stderr, "bad -hex (expected hex bytes):", derr)
			os.Exit(1)
		}
		fmt.Printf("════════════════ UNPACK  (wire → fields) ════════════════\nprofile : %s | input %d bytes\n", schema.ID(), len(raw))
		dec, derr := codec.Unmarshal(raw)
		if derr != nil {
			fmt.Fprintln(os.Stderr, "unmarshal:", derr)
			os.Exit(1)
		}
		_ = iso8583.Describe(os.Stdout, dec, dopts...)
		return
	}

	// ───────── PACK: build the selected message ─────────
	if *msg != "signon" && *msg != "cashout" {
		fmt.Fprintf(os.Stderr, "unknown -msg %q (use signon, cashout, or unpack)\n", *msg)
		os.Exit(1)
	}
	m := buildMessage(schema, *msg)
	if *fresh && *msg == "cashout" {
		now := time.Now()
		_ = m.Set(7, now.Format("0102150405"))    // DE 7  transmission MMDDhhmmss
		_ = m.Set(11, now.Format("150405"))       // DE 11 STAN (hhmmss)
		_ = m.Set(12, now.Format("150405"))       // DE 12 local time hhmmss
		_ = m.Set(13, now.Format("0102"))         // DE 13 local date MMDD
		_ = m.Set(37, now.Format("060102150405")) // DE 37 RRN yyMMddhhmmss (12)
	}

	wire, err := codec.Marshal(m, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal:", err)
		os.Exit(1)
	}

	fmt.Println("════════════════ PACK  (build → wire) ════════════════")
	fmt.Printf("profile : %s | msg %s\n", schema.ID(), *msg)
	fmt.Printf("length  : %d bytes\n", len(wire))
	fmt.Printf("hex     : %s\n", strings.ToUpper(hex.EncodeToString(wire)))
	fmt.Printf("ascii   : %q\n\n", string(wire))
	if err := iso8583.Describe(os.Stdout, m, dopts...); err != nil {
		fmt.Fprintln(os.Stderr, "describe:", err)
	}

	// ───────── UNPACK (round-trip self-check) — only when NOT sending ─────────
	if *send == "" {
		raw := wire
		if s := strings.ReplaceAll(strings.TrimSpace(*hexIn), " ", ""); s != "" {
			raw, err = hex.DecodeString(s)
			if err != nil {
				fmt.Fprintln(os.Stderr, "bad -hex (expected hex bytes):", err)
				os.Exit(1)
			}
		}
		fmt.Println("\n════════════════ UNPACK  (wire → fields) ════════════════")
		fmt.Printf("input   : %d bytes\n", len(raw))
		dec, derr := codec.Unmarshal(raw)
		if derr != nil {
			fmt.Fprintln(os.Stderr, "unmarshal:", derr)
			os.Exit(1)
		}
		if derr := iso8583.Describe(os.Stdout, dec, dopts...); derr != nil {
			fmt.Fprintln(os.Stderr, "describe:", derr)
		}
		return
	}

	// ───────── SEND: dial a host and read the reply ─────────
	scheme := "plain TCP"
	dialOpts := []link.Option{link.WithFramer(link.LengthPrefix(*lenbytes)), link.WithDialTimeout(*dialTimeout)}
	if *useTLS {
		scheme = "TLS"
		name := *tlsServerName
		if name == "" {
			if h, _, err := net.SplitHostPort(*send); err == nil {
				name = h
			}
		}
		dialOpts = append(dialOpts, link.WithTLS(&tls.Config{
			ServerName:         name,
			InsecureSkipVerify: *tlsInsecure, //nolint:gosec // -tls-insecure is an explicit opt-in for self-signed test hosts
		}))
		if *tlsInsecure {
			scheme = "TLS (no cert verify)"
		}
	}
	var headerBytes []byte
	if h := strings.ReplaceAll(strings.TrimSpace(*header), " ", ""); h != "" {
		hb, herr := hex.DecodeString(h)
		if herr != nil {
			fmt.Fprintln(os.Stderr, "bad -header (expected hex):", herr)
			os.Exit(1)
		}
		headerBytes = hb
	}

	fmt.Printf("\n════════════════ SEND  → %s (%s, len-prefix %dB, header %dB) ════════════════\n", *send, scheme, *lenbytes, len(headerBytes))
	// Dial on a short, separate budget so an unreachable host fails fast; the
	// -timeout budget is reserved for waiting on each reply.
	dialCtx, dialCancel := context.WithTimeout(context.Background(), *dialTimeout)
	cl, err := link.DialContext(dialCtx, "tcp", *send, dialOpts...)
	dialCancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		os.Exit(1)
	}
	defer cl.Close()

	// Optional sign-on (0800) first, on the SAME connection, to open a session.
	if *signon && *msg != "signon" {
		sonWire, e := codec.Marshal(buildMessage(schema, "signon"), nil)
		if e != nil {
			fmt.Fprintln(os.Stderr, "marshal sign-on:", e)
			os.Exit(1)
		}
		fmt.Printf("\n──── sign-on 0800 → %d bytes; waiting up to %s ────\n", len(withHeader(sonWire, headerBytes)), *timeout)
		frame, e := exchange(cl, withHeader(sonWire, headerBytes), *timeout)
		if e != nil {
			fmt.Fprintln(os.Stderr, "sign-on:", e)
			os.Exit(1)
		}
		showReply(codec, frame, dopts)
	}

	// Send the selected message and read its reply on the same connection.
	fmt.Printf("\n──── %s %s → %d bytes; waiting up to %s ────\n", *msg, mtiOf(*msg), len(withHeader(wire, headerBytes)), *timeout)
	frame, err := exchange(cl, withHeader(wire, headerBytes), *timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "send:", err)
		os.Exit(1)
	}
	showReply(codec, frame, dopts)
}

// mtiOf returns the MTI label for a message kind, for display.
func mtiOf(kind string) string {
	if kind == "signon" {
		return "0800"
	}
	return "0200"
}
