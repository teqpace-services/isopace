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
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/teqpace-services/isopace/vault"
)

// Host-command response error codes. "00" is success; the others are this
// scaffold's stand-ins for the device's two-digit error codes (the real codes
// are firmware-specific — see the Host Command Reference Manual).
const (
	codeOK          = "00"
	codeVerifyFail  = "01" // MAC verify: computed MAC did not match
	codeKeyNotFound = "10"
	codeFormatError = "20"
	codeBadCommand  = "30"
)

// HostError is returned when the device replies with a non-success error code.
type HostError struct {
	Op   string // the adapter operation (e.g. "TranslatePIN")
	Code string // the two-character host error code
}

func (e *HostError) Error() string {
	return fmt.Sprintf("payshield: %s: host error code %q", e.Op, e.Code)
}

// incrementCmd returns the response command code for a request code — the
// payShield convention is the request code with its last character incremented
// (e.g. "CC" → "CD", "M6" → "M7").
func incrementCmd(cmd string) string {
	if cmd == "" {
		return ""
	}
	b := []byte(cmd)
	b[len(b)-1]++
	return string(b)
}

// algCode / padCode map the vault MAC parameters to the one-character selectors
// carried in the MAC commands (ISO 9797-1 algorithm number; padding method).
func algCode(alg vault.MACAlgorithm) (string, error) {
	switch alg {
	case vault.MACAlg1:
		return "1", nil
	case vault.MACAlg3:
		return "3", nil
	default:
		return "", fmt.Errorf("payshield: unsupported MAC algorithm %d", alg)
	}
}

func algFromCode(s string) (vault.MACAlgorithm, bool) {
	switch s {
	case "1":
		return vault.MACAlg1, true
	case "3":
		return vault.MACAlg3, true
	default:
		return 0, false
	}
}

func padCode(pad vault.Padding) string {
	if pad == vault.Pad2 {
		return "2"
	}
	return "1"
}

func padFromCode(s string) vault.Padding {
	if s == "2" {
		return vault.Pad2
	}
	return vault.Pad1
}

// defaultFormatCodes maps each ISO 9564 PIN block format to a payShield
// PIN-block-format code. These are firmware-typical defaults; override them via
// Config.FormatCodes to match your device's Host Command Reference Manual.
var defaultFormatCodes = map[vault.PINBlockFormat]string{
	vault.ISO0: "01",
	vault.ISO1: "05",
	vault.ISO3: "47",
}

// fieldWriter builds a command/response body as a sequence of length-prefixed
// fields: each field is a 4-decimal-digit byte length followed by that many
// bytes. (This is a simplified, self-consistent stand-in for payShield's
// positional field layout — see the package doc.)
type fieldWriter struct{ b []byte }

func (w *fieldWriter) str(s string) {
	w.b = append(w.b, fmt.Sprintf("%04d", len(s))...)
	w.b = append(w.b, s...)
}

func (w *fieldWriter) hexBytes(p []byte) { w.str(hex.EncodeToString(p)) }

// fieldReader parses the length-prefixed fields written by fieldWriter. The
// first error is sticky; callers check err once after reading all fields.
type fieldReader struct {
	b   []byte
	i   int
	err error
}

func newFieldReader(b []byte) *fieldReader { return &fieldReader{b: b} }

func (r *fieldReader) str() string {
	if r.err != nil {
		return ""
	}
	if r.i+4 > len(r.b) {
		r.err = fmt.Errorf("payshield: truncated field length at offset %d", r.i)
		return ""
	}
	n, err := strconv.Atoi(string(r.b[r.i : r.i+4]))
	if err != nil || n < 0 {
		r.err = fmt.Errorf("payshield: bad field length %q", r.b[r.i:r.i+4])
		return ""
	}
	r.i += 4
	if r.i+n > len(r.b) {
		r.err = fmt.Errorf("payshield: truncated field value (want %d bytes at offset %d)", n, r.i)
		return ""
	}
	s := string(r.b[r.i : r.i+n])
	r.i += n
	return s
}

func (r *fieldReader) hexBytes() []byte {
	s := r.str()
	if r.err != nil {
		return nil
	}
	p, err := hex.DecodeString(s)
	if err != nil {
		r.err = fmt.Errorf("payshield: bad hex field: %w", err)
		return nil
	}
	return p
}
