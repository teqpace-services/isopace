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

// Command vectorgen is a clean-room, BLACK-BOX conformance vector capture tool.
// It connects to a reference ISO-8583 endpoint over TCP, sends a caller-supplied
// request frame, and records the response WIRE BYTES as a hex vector file.
//
// Clean-room boundary (see CONTRIBUTING.md): this tool observes only externally
// visible wire bytes. It does NOT import, link, embed, or read the source of
// jPOS (or any other implementation), and it is a standalone dev binary that is
// never linked into the shipped Isopace library. Captured bytes may be used to
// derive golden vectors; reference SOURCE CODE and test fixtures must never be
// copied.
//
// Usage:
//
//	vectorgen -addr host:port -req <hex> -out vector.hex [-prefix 2] [-timeout 5s]
package main

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "", "reference endpoint host:port (required)")
	reqHex := flag.String("req", "", "request body as hex (required)")
	out := flag.String("out", "", "output vector file (hex); stdout if empty")
	prefix := flag.Int("prefix", 2, "length-prefix width in bytes (0 = none)")
	timeout := flag.Duration("timeout", 5*time.Second, "dial + I/O timeout")
	flag.Parse()

	if err := run(*addr, *reqHex, *out, *prefix, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, "vectorgen:", err)
		os.Exit(1)
	}
}

func run(addr, reqHex, out string, prefix int, timeout time.Duration) error {
	if addr == "" || reqHex == "" {
		return errors.New("-addr and -req are required")
	}
	body, err := hex.DecodeString(strings.TrimSpace(reqHex))
	if err != nil {
		return fmt.Errorf("bad -req hex: %w", err)
	}

	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	if err := writeFrame(conn, body, prefix); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	resp, err := readFrame(conn, prefix)
	if err != nil {
		return fmt.Errorf("receive: %w", err)
	}

	enc := hex.EncodeToString(resp) + "\n"
	if out == "" {
		fmt.Print(enc)
		return nil
	}
	return os.WriteFile(out, []byte(enc), 0o644)
}

func writeFrame(w io.Writer, body []byte, prefix int) error {
	if prefix > 0 {
		hdr := make([]byte, prefix)
		switch prefix {
		case 2:
			binary.BigEndian.PutUint16(hdr, uint16(len(body)))
		case 4:
			binary.BigEndian.PutUint32(hdr, uint32(len(body)))
		default:
			return fmt.Errorf("unsupported prefix width %d", prefix)
		}
		if _, err := w.Write(hdr); err != nil {
			return err
		}
	}
	_, err := w.Write(body)
	return err
}

func readFrame(r io.Reader, prefix int) ([]byte, error) {
	if prefix == 0 {
		return io.ReadAll(r)
	}
	hdr := make([]byte, prefix)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, err
	}
	var n int
	switch prefix {
	case 2:
		n = int(binary.BigEndian.Uint16(hdr))
	case 4:
		n = int(binary.BigEndian.Uint32(hdr))
	default:
		return nil, fmt.Errorf("unsupported prefix width %d", prefix)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}
