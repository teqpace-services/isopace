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

package link

import (
	"encoding/binary"
	"io"
)

// Framer turns a byte stream into discrete message frames. Implementations must
// be safe for one concurrent reader and one concurrent writer.
type Framer interface {
	// ReadFrame reads exactly one message body from r.
	ReadFrame(r io.Reader) ([]byte, error)
	// WriteFrame writes one message body to w.
	WriteFrame(w io.Writer, msg []byte) error
}

// defaultMaxFrame caps a frame body to guard against a hostile length prefix
// triggering a huge allocation. Width-2 prefixes are naturally bounded to
// 65535; width-4 prefixes are capped here.
const defaultMaxFrame = 1 << 20 // 1 MiB

// lengthPrefix frames a message with a big-endian length prefix of `width`
// bytes (the most common ISO-8583-over-TCP framing). The prefix counts the body
// bytes only.
type lengthPrefix struct {
	width int
	max   int
}

// LengthPrefix returns a Framer using a big-endian length prefix of width bytes
// (2 or 4). It panics for other widths.
func LengthPrefix(width int) Framer {
	switch width {
	case 2:
		return lengthPrefix{width: 2, max: 1<<16 - 1}
	case 4:
		return lengthPrefix{width: 4, max: defaultMaxFrame}
	default:
		panic("link: LengthPrefix width must be 2 or 4")
	}
}

func (lp lengthPrefix) ReadFrame(r io.Reader) ([]byte, error) {
	hdr := make([]byte, lp.width)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, err
	}
	var n int
	switch lp.width {
	case 2:
		n = int(binary.BigEndian.Uint16(hdr))
	case 4:
		n = int(binary.BigEndian.Uint32(hdr))
	}
	if n > lp.max {
		return nil, ErrTooLarge
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

func (lp lengthPrefix) WriteFrame(w io.Writer, msg []byte) error {
	if len(msg) > lp.max {
		return ErrTooLarge
	}
	hdr := make([]byte, lp.width)
	switch lp.width {
	case 2:
		binary.BigEndian.PutUint16(hdr, uint16(len(msg)))
	case 4:
		binary.BigEndian.PutUint32(hdr, uint32(len(msg)))
	}
	// Single write of prefix+body so concurrent writers never interleave.
	buf := make([]byte, 0, len(hdr)+len(msg))
	buf = append(buf, hdr...)
	buf = append(buf, msg...)
	_, err := w.Write(buf)
	return err
}
