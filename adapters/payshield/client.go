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

package payshield

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// errClosed is returned when a command is issued on a closed client.
var errClosed = errors.New("payshield: client is closed")

// client is the host-command transport: a single TCP connection to the device,
// serialised by a mutex (one command in flight at a time). Each message is a
// 2-byte big-endian length prefix over a payload of header‖body. This scaffold
// does not pool connections or auto-reconnect — production deployments should
// front it with the supervised-connection machinery (see the package doc).
type client struct {
	cfg  Config
	mu   sync.Mutex
	conn net.Conn
	r    *bufio.Reader
}

func dial(cfg Config) (*client, error) {
	d := net.Dialer{Timeout: cfg.DialTimeout}
	conn, err := d.Dial("tcp", cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("payshield: dial %s: %w", cfg.Addr, err)
	}
	return &client{cfg: cfg, conn: conn, r: bufio.NewReader(conn)}, nil
}

func (c *client) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// do sends one host command (header‖cmd‖data) and returns the response error
// code and the response data (after the echoed header, response code, and error
// code have been stripped).
func (c *client) do(cmd string, data []byte) (errCode string, respData []byte, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return "", nil, errClosed
	}
	if c.cfg.Timeout > 0 {
		_ = c.conn.SetDeadline(time.Now().Add(c.cfg.Timeout))
	}

	payload := make([]byte, 0, len(c.cfg.Header)+len(cmd)+len(data))
	payload = append(payload, c.cfg.Header...)
	payload = append(payload, cmd...)
	payload = append(payload, data...)
	if len(payload) > 0xFFFF {
		return "", nil, fmt.Errorf("payshield: command too large (%d bytes)", len(payload))
	}
	if err := writeFrame(c.conn, payload); err != nil {
		return "", nil, fmt.Errorf("payshield: write %s: %w", cmd, err)
	}

	resp, err := readFrame(c.r)
	if err != nil {
		return "", nil, fmt.Errorf("payshield: read %s response: %w", cmd, err)
	}
	h := len(c.cfg.Header)
	if len(resp) < h+4 {
		return "", nil, fmt.Errorf("payshield: short response (%d bytes)", len(resp))
	}
	resp = resp[h:]
	respCode := string(resp[:2])
	if want := incrementCmd(cmd); respCode != want {
		return "", nil, fmt.Errorf("payshield: response code %q, want %q", respCode, want)
	}
	return string(resp[2:4]), resp[4:], nil
}

// writeFrame writes a 2-byte big-endian length prefix followed by payload.
func writeFrame(w io.Writer, payload []byte) error {
	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// readFrame reads a 2-byte big-endian length prefix and that many bytes.
func readFrame(r io.Reader) ([]byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint16(hdr[:])
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
