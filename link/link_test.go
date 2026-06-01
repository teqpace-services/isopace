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
	"errors"
	"net"
	"sync"
	"testing"

	"github.com/teqpace-services/isopace/link"
)

// linkPair returns a connected client/server Link pair over loopback TCP.
func linkPair(t *testing.T, opts ...link.Option) (client, server *link.Link, cleanup func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	type res struct {
		c   net.Conn
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := ln.Accept()
		ch <- res{c, err}
	}()
	cc, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	sr := <-ch
	if sr.err != nil {
		t.Fatal(sr.err)
	}
	client = link.New(cc, opts...)
	server = link.New(sr.c, opts...)
	return client, server, func() {
		client.Close()
		server.Close()
		ln.Close()
	}
}

func TestLinkFramingRoundTrip(t *testing.T) {
	client, server, cleanup := linkPair(t)
	defer cleanup()

	msgs := [][]byte{[]byte("0200hello"), {0x00, 0x01, 0x02}, []byte("third frame")}
	go func() {
		for _, m := range msgs {
			if err := client.Send(m); err != nil {
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
		if !bytes.Equal(got, want) {
			t.Errorf("frame = % X want % X", got, want)
		}
	}
}

func TestLinkOversizedFrame(t *testing.T) {
	client, _, cleanup := linkPair(t)
	defer cleanup()
	// 2-byte prefix tops out at 65535.
	if err := client.Send(make([]byte, 70000)); !errors.Is(err, link.ErrTooLarge) {
		t.Errorf("oversized Send err = %v want ErrTooLarge", err)
	}
}

func TestLinkFilterSymmetry(t *testing.T) {
	// XOR filter: identical on send and receive, so a pair cancels out.
	xor := link.FilterFunc(func(msg []byte, _ bool) ([]byte, error) {
		out := make([]byte, len(msg))
		for i, b := range msg {
			out[i] = b ^ 0x5A
		}
		return out, nil
	})
	client, server, cleanup := linkPair(t, link.WithFilters(xor))
	defer cleanup()

	want := []byte("PINBLOCK")
	go client.Send(want)
	got, err := server.Receive()
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("filtered round-trip = % X want % X", got, want)
	}
}

func TestLinkConcurrentSend(t *testing.T) {
	client, server, cleanup := linkPair(t)
	defer cleanup()

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := client.Send([]byte("0800netmgmt")); err != nil {
				t.Errorf("Send: %v", err)
			}
		}()
	}
	for i := 0; i < n; i++ {
		got, err := server.Receive()
		if err != nil {
			t.Fatalf("Receive: %v", err)
		}
		if !bytes.Equal(got, []byte("0800netmgmt")) {
			t.Errorf("frame corrupted under concurrency: % X", got)
		}
	}
	wg.Wait()
}

func TestLinkCloseIdempotent(t *testing.T) {
	client, _, cleanup := linkPair(t)
	defer cleanup()
	if err := client.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if err := client.Send([]byte("x")); !errors.Is(err, link.ErrClosed) {
		t.Errorf("Send after close = %v want ErrClosed", err)
	}
}
