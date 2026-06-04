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

// Package payshield is a reference adapter that exposes a Thales payShield
// payment HSM as the Isopace vault capabilities a switch needs: [vault.PINTranslator]
// (PCI-secure PIN translate, the operation a stock PKCS#11 HSM cannot do) and
// [vault.Macer] (ISO 9797-1 message MAC). It lives in a separate module so the
// stdlib-only core gains no HSM-transport dependency.
//
// # Why payShield (and the capability split)
//
// A payment HSM performs PIN translate atomically inside the device, so the
// clear PIN never reaches host memory (PCI PIN Security). That is exactly the
// capability a general-purpose PKCS#11 HSM lacks — see adapters/pkcs11, which
// implements only [vault.Macer] for that reason. This adapter therefore advertises
// [vault.PINTranslator] in addition to [vault.Macer]. Keys are referenced by the
// device's own key token (a key encrypted under the HSM's Local Master Key); the
// clear key never leaves the device.
//
// # Protocol
//
// The adapter speaks the payShield host-command protocol: a TCP message framed
// by a 2-byte big-endian length prefix over header‖command. The request command
// code (e.g. "CC" translate, "M6"/"M8" MAC generate/verify) and the response
// code (the request code with its last character incremented) and a two-digit
// error code follow the payShield convention. The command codes and PIN-block
// format codes are configurable to match a specific firmware's Host Command
// Reference Manual.
//
// # Scaffold status (read before production)
//
// This is a SCAFFOLD validated against the in-repo [Simulator] (a protocol test
// double whose cryptography is the Isopace software vault). It exercises the
// command flow — framing, command building, response and error-code parsing, the
// capability surface — end to end in CI without hardware. Two things are NOT yet
// real-device-faithful and are the remaining integration work against a genuine
// payShield (or Thales's own simulator):
//
//   - The on-wire FIELD layout is a simplified, self-consistent stand-in
//     (length-prefixed fields) for payShield's positional field structure. The
//     command set, key references, format codes, and error handling are modelled
//     on payShield; the byte-level field packing must be swapped for the exact
//     layout from the Host Command Reference for the targeted firmware.
//   - It has not been validated against real payShield key tokens, LMK schemes,
//     or PCI-certified hardware. Independent security review is required.
package payshield

import (
	"time"

	"github.com/teqpace-services/isopace/vault"
)

// Config configures a payShield-backed Vault.
type Config struct {
	Addr        string        // host:port of the payShield (or a Simulator)
	Header      string        // message header the device echoes; default "" (none)
	DialTimeout time.Duration // connect timeout; default 5s
	Timeout     time.Duration // per-command deadline; default 5s

	// Host command codes. Override to match the targeted firmware's Host Command
	// Reference Manual. Defaults: TranslateCmd "CC", GenerateMACCmd "M6",
	// VerifyMACCmd "M8".
	TranslateCmd   string
	GenerateMACCmd string
	VerifyMACCmd   string

	// FormatCodes overrides the ISO 9564 → payShield PIN-block-format code map
	// (see defaultFormatCodes). Entries override defaults; unset formats fall back.
	FormatCodes map[vault.PINBlockFormat]string
}

func (c Config) withDefaults() Config {
	if c.DialTimeout == 0 {
		c.DialTimeout = 5 * time.Second
	}
	if c.Timeout == 0 {
		c.Timeout = 5 * time.Second
	}
	if c.TranslateCmd == "" {
		c.TranslateCmd = "CC"
	}
	if c.GenerateMACCmd == "" {
		c.GenerateMACCmd = "M6"
	}
	if c.VerifyMACCmd == "" {
		c.VerifyMACCmd = "M8"
	}
	return c
}

func (c Config) formatCode(f vault.PINBlockFormat) (string, bool) {
	if c.FormatCodes != nil {
		if s, ok := c.FormatCodes[f]; ok {
			return s, true
		}
	}
	s, ok := defaultFormatCodes[f]
	return s, ok
}

func (c Config) formatFromCode(code string) (vault.PINBlockFormat, bool) {
	for _, f := range []vault.PINBlockFormat{vault.ISO0, vault.ISO1, vault.ISO3} {
		if s, ok := c.formatCode(f); ok && s == code {
			return f, true
		}
	}
	return 0, false
}

// Vault is a payShield-backed vault that satisfies PINTranslator and Macer.
type Vault struct {
	cfg Config
	cl  *client
}

var (
	_ vault.PINTranslator = (*Vault)(nil)
	_ vault.Macer         = (*Vault)(nil)
)

// Open dials the payShield (or Simulator) at cfg.Addr and returns a Vault. Call
// Close to release the connection.
func Open(cfg Config) (*Vault, error) {
	cfg = cfg.withDefaults()
	cl, err := dial(cfg)
	if err != nil {
		return nil, err
	}
	return &Vault{cfg: cfg, cl: cl}, nil
}

// Close releases the underlying connection.
func (v *Vault) Close() error { return v.cl.close() }

// TranslatePIN implements vault.PINTranslator via the device's PIN-translate
// command: the device re-enciphers the encrypted PIN block from srcRef/srcFormat
// to dstRef/dstFormat internally, so the clear PIN never reaches the host.
func (v *Vault) TranslatePIN(srcRef, dstRef string, encBlock []byte, pan string, srcFormat, dstFormat vault.PINBlockFormat) ([]byte, error) {
	sf, ok := v.cfg.formatCode(srcFormat)
	if !ok {
		return nil, &HostError{Op: "TranslatePIN", Code: codeFormatError}
	}
	df, ok := v.cfg.formatCode(dstFormat)
	if !ok {
		return nil, &HostError{Op: "TranslatePIN", Code: codeFormatError}
	}
	var f fieldWriter
	f.str(srcRef)
	f.str(dstRef)
	f.str(sf)
	f.str(df)
	f.str(pan)
	f.hexBytes(encBlock)

	ec, data, err := v.cl.do(v.cfg.TranslateCmd, f.b)
	if err != nil {
		return nil, err
	}
	if ec != codeOK {
		return nil, &HostError{Op: "TranslatePIN", Code: ec}
	}
	rd := newFieldReader(data)
	out := rd.hexBytes()
	if rd.err != nil {
		return nil, rd.err
	}
	return out, nil
}

// GenerateMAC implements vault.Macer via the device's MAC-generate command.
func (v *Vault) GenerateMAC(keyRef string, alg vault.MACAlgorithm, pad vault.Padding, data []byte) ([]byte, error) {
	body, err := macBody(keyRef, alg, pad, data)
	if err != nil {
		return nil, err
	}
	ec, resp, err := v.cl.do(v.cfg.GenerateMACCmd, body)
	if err != nil {
		return nil, err
	}
	if ec != codeOK {
		return nil, &HostError{Op: "GenerateMAC", Code: ec}
	}
	rd := newFieldReader(resp)
	mac := rd.hexBytes()
	if rd.err != nil {
		return nil, rd.err
	}
	return mac, nil
}

// VerifyMAC implements vault.Macer via the device's MAC-verify command: a
// success code means the MAC matched; the verify-fail code means it did not.
func (v *Vault) VerifyMAC(keyRef string, alg vault.MACAlgorithm, pad vault.Padding, data, mac []byte) (bool, error) {
	body, err := macBody(keyRef, alg, pad, data)
	if err != nil {
		return false, err
	}
	var f fieldWriter
	f.b = body
	f.hexBytes(mac)

	ec, _, err := v.cl.do(v.cfg.VerifyMACCmd, f.b)
	if err != nil {
		return false, err
	}
	switch ec {
	case codeOK:
		return true, nil
	case codeVerifyFail:
		return false, nil
	default:
		return false, &HostError{Op: "VerifyMAC", Code: ec}
	}
}

// macBody builds the common MAC command fields: keyRef, algorithm, padding, and
// the hex-encoded message.
func macBody(keyRef string, alg vault.MACAlgorithm, pad vault.Padding, data []byte) ([]byte, error) {
	ac, err := algCode(alg)
	if err != nil {
		return nil, err
	}
	var f fieldWriter
	f.str(keyRef)
	f.str(ac)
	f.str(padCode(pad))
	f.hexBytes(data)
	return f.b, nil
}
