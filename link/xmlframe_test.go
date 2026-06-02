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

package link_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/teqpace-services/isopace/link"
)

// nested has an inner <isomsg id="48"> whose closing tag must NOT end the frame.
const nested = `<isomsg>
  <field id="0" value="0800"/>
  <isomsg id="48">
    <field id="1" value="ABC"/>
  </isomsg>
</isomsg>`

const flat = `<isomsg><field id="0" value="0810"/></isomsg>`

func TestISOXMLReadBalancesNesting(t *testing.T) {
	f := link.ISOXML()
	// Two frames back-to-back, the writer's newline separating them.
	stream := nested + "\n" + flat + "\n"
	r := strings.NewReader(stream)

	got1, err := f.ReadFrame(r)
	if err != nil {
		t.Fatalf("ReadFrame 1: %v", err)
	}
	if string(got1) != nested {
		t.Errorf("frame 1 mismatch:\n got %q\nwant %q", got1, nested)
	}
	got2, err := f.ReadFrame(r)
	if err != nil {
		t.Fatalf("ReadFrame 2: %v", err)
	}
	if string(got2) != flat {
		t.Errorf("frame 2 mismatch:\n got %q\nwant %q", got2, flat)
	}
	if _, err := f.ReadFrame(r); err != io.EOF {
		t.Errorf("ReadFrame at end = %v want io.EOF", err)
	}
}

func TestISOXMLWriteFrameAppendsNewline(t *testing.T) {
	f := link.ISOXML()
	var buf bytes.Buffer
	if err := f.WriteFrame(&buf, []byte(flat)); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	if buf.String() != flat+"\n" {
		t.Errorf("WriteFrame = %q want %q", buf.String(), flat+"\n")
	}
}

func TestISOXMLPartialFrameUnexpectedEOF(t *testing.T) {
	f := link.ISOXML()
	// An open element with no closing tag is a truncated frame.
	if _, err := f.ReadFrame(strings.NewReader(`<isomsg><field id="0" value="0800"/>`)); err != io.ErrUnexpectedEOF {
		t.Errorf("truncated ReadFrame = %v want io.ErrUnexpectedEOF", err)
	}
}

// TestISOXMLOverLink exercises the framer through a real Link pair, including the
// nested-composite frame, to prove the delimiter framing survives the transport.
func TestISOXMLOverLink(t *testing.T) {
	client, server, cleanup := linkPair(t, link.WithFramer(link.ISOXML()))
	defer cleanup()

	msgs := []string{nested, flat, nested}
	go func() {
		for _, m := range msgs {
			if err := client.Send([]byte(m)); err != nil {
				t.Errorf("Send: %v", err)
				return
			}
		}
	}()
	for _, want := range msgs {
		got, err := server.Receive()
		if err != nil {
			t.Fatalf("Receive: %v", err)
		}
		if string(got) != want {
			t.Errorf("frame mismatch:\n got %q\nwant %q", got, want)
		}
	}
}
