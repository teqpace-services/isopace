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

package link

import "io"

// ISOXML returns a Framer that delimits messages by the isomsg XML element
// rather than a length prefix — the framing an XML channel uses (jPOS's
// XMLChannel). A frame is exactly one balanced <isomsg>…</isomsg> element,
// including any nested <isomsg id="…"> children; WriteFrame appends a trailing
// newline so a peer reading line-wise sees a record boundary. Pair it with the
// render/xmlio packager:
//
//	l := link.New(conn, link.WithFramer(link.ISOXML()))
//	l.Send(mustMarshalXML(msg)) // xmlio.Marshal
//	frame, _ := l.Receive()     // xmlio.Unmarshal(frame, schema)
//
// XML is delimiter-framed, not length-framed, so ReadFrame consumes the stream
// one byte at a time and stops exactly on the closing tag — it never reads past
// a frame boundary. That keeps the Framer stateless (safe to share across Links)
// at the cost of throughput, an acceptable trade for the human-readable XML
// transport, which is not a hot path.
func ISOXML() Framer { return isoXML{} }

type isoXML struct{}

// WriteFrame writes the XML body followed by a newline record separator.
func (isoXML) WriteFrame(w io.Writer, msg []byte) error {
	if len(msg) > defaultMaxFrame {
		return ErrTooLarge
	}
	buf := make([]byte, 0, len(msg)+1)
	buf = append(buf, msg...)
	buf = append(buf, '\n')
	_, err := w.Write(buf)
	return err
}

// ReadFrame reads one complete isomsg element. It skips inter-frame whitespace,
// then accumulates bytes until the <isomsg> open count and </isomsg> close count
// balance, so nested sub-messages are kept within a single frame.
func (isoXML) ReadFrame(r io.Reader) ([]byte, error) {
	var (
		buf   []byte
		tag   []byte
		inTag bool
		depth int
		begun bool // at least one <isomsg> open has been seen
	)
	one := make([]byte, 1)
	for {
		if _, err := io.ReadFull(r, one); err != nil {
			if err == io.EOF && len(buf) == 0 {
				return nil, io.EOF
			}
			if err == io.EOF {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, err
		}
		b := one[0]

		// Drop whitespace that precedes the opening tag so a stray newline
		// between frames never bleeds into the next one.
		if !begun && !inTag && len(buf) == 0 && isSpace(b) {
			continue
		}

		buf = append(buf, b)
		if len(buf) > defaultMaxFrame {
			return nil, ErrTooLarge
		}

		switch {
		case b == '<':
			inTag = true
			tag = tag[:0]
		case b == '>' && inTag:
			inTag = false
			isOpen, isClose, selfClose := classifyTag(tag)
			if isOpen {
				begun = true
				if !selfClose {
					depth++
				}
			}
			if isClose {
				depth--
			}
			if begun && depth == 0 {
				return buf, nil
			}
		case inTag:
			tag = append(tag, b)
		}
	}
}

// classifyTag inspects the bytes between '<' and '>' and reports whether the tag
// opens or closes an <isomsg> element, and whether it is self-closing. Only
// isomsg tags affect framing; <field> and any other element are ignored. It
// assumes well-formed, attribute-escaped XML, where '<' and '>' appear only as
// tag delimiters (the output of xmlio.Marshal).
func classifyTag(tag []byte) (isOpen, isClose, selfClose bool) {
	end := len(tag)
	for end > 0 && isSpace(tag[end-1]) {
		end--
	}
	i := 0
	for i < end && isSpace(tag[i]) {
		i++
	}
	if i < end && tag[i] == '/' { // closing tag: </name>
		return false, readName(tag, i+1, end) == "isomsg", false
	}
	selfClose = end > 0 && tag[end-1] == '/'
	if readName(tag, i, end) == "isomsg" {
		return true, false, selfClose
	}
	return false, false, selfClose
}

// readName returns the element name: the run from i up to the first space, '/',
// or the tag end.
func readName(tag []byte, i, end int) string {
	start := i
	for i < end && !isSpace(tag[i]) && tag[i] != '/' {
		i++
	}
	return string(tag[start:i])
}

func isSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }
