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
	"errors"
	"net"
	"sync"

	"github.com/teqpace-services/isopace/vault"
)

// Simulator is a payShield host-command TEST DOUBLE: a TCP server that speaks the
// adapter's framing and command set so the adapter can be exercised end to end
// without hardware. Its cryptography is the Isopace software vault
// ([vault.SoftVault]), so the values it returns are real ISO 9564 / ISO 9797-1
// results — but it is NOT a Thales payShield and NOT Thales's simulator: there is
// no LMK, no key-token security, and no PCI boundary. Use it for protocol and
// integration testing only. Keys are loaded in the clear via ImportKey, and the
// adapter's keyRef is the label under which a key was imported.
type Simulator struct {
	cfg Config
	ln  net.Listener
	v   *vault.SoftVault
	wg  sync.WaitGroup
}

// NewSimulator starts a simulator listening on cfg.Addr (default "127.0.0.1:0",
// an OS-chosen port — read it back with Addr). Call Close to stop it.
func NewSimulator(cfg Config) (*Simulator, error) {
	cfg = cfg.withDefaults()
	addr := cfg.Addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &Simulator{cfg: cfg, ln: ln, v: vault.NewSoftVault()}
	s.wg.Go(s.serve)
	return s, nil
}

// Addr returns the simulator's listen address (host:port).
func (s *Simulator) Addr() string { return s.ln.Addr().String() }

// ImportKey loads a clear key under ref. The adapter then names it as a keyRef.
func (s *Simulator) ImportKey(ref string, key []byte) { s.v.SetKey(ref, key) }

// Close stops the listener and waits for in-flight connections to drain.
func (s *Simulator) Close() error {
	err := s.ln.Close()
	s.wg.Wait()
	return err
}

func (s *Simulator) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return // listener closed
		}
		s.wg.Go(func() { s.handle(conn) })
	}
}

func (s *Simulator) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		payload, err := readFrame(r)
		if err != nil {
			return // connection closed or malformed
		}
		h := len(s.cfg.Header)
		if len(payload) < h+2 {
			return
		}
		body := payload[h:]
		cmd := string(body[:2])
		respCode, errCode, respData := s.dispatch(cmd, body[2:])

		out := make([]byte, 0, h+4+len(respData))
		out = append(out, s.cfg.Header...)
		out = append(out, respCode...)
		out = append(out, errCode...)
		out = append(out, respData...)
		if err := writeFrame(conn, out); err != nil {
			return
		}
	}
}

func (s *Simulator) dispatch(cmd string, data []byte) (respCode, errCode string, respData []byte) {
	respCode = incrementCmd(cmd)
	switch cmd {
	case s.cfg.TranslateCmd:
		errCode, respData = s.translate(data)
	case s.cfg.GenerateMACCmd:
		errCode, respData = s.genMAC(data)
	case s.cfg.VerifyMACCmd:
		errCode, respData = s.verifyMAC(data)
	default:
		errCode = codeBadCommand
	}
	return respCode, errCode, respData
}

func (s *Simulator) translate(data []byte) (string, []byte) {
	rd := newFieldReader(data)
	srcRef := rd.str()
	dstRef := rd.str()
	srcCode := rd.str()
	dstCode := rd.str()
	pan := rd.str()
	block := rd.hexBytes()
	if rd.err != nil {
		return codeFormatError, nil
	}
	srcFmt, ok1 := s.cfg.formatFromCode(srcCode)
	dstFmt, ok2 := s.cfg.formatFromCode(dstCode)
	if !ok1 || !ok2 {
		return codeFormatError, nil
	}
	out, err := s.v.TranslatePIN(srcRef, dstRef, block, pan, srcFmt, dstFmt)
	if err != nil {
		return hostCodeFor(err), nil
	}
	var f fieldWriter
	f.hexBytes(out)
	return codeOK, f.b
}

func (s *Simulator) genMAC(data []byte) (string, []byte) {
	keyRef, alg, pad, msg, ok := readMACBody(data)
	if !ok {
		return codeFormatError, nil
	}
	mac, err := s.v.GenerateMAC(keyRef, alg, pad, msg)
	if err != nil {
		return hostCodeFor(err), nil
	}
	var f fieldWriter
	f.hexBytes(mac)
	return codeOK, f.b
}

func (s *Simulator) verifyMAC(data []byte) (string, []byte) {
	rd := newFieldReader(data)
	keyRef := rd.str()
	ac := rd.str()
	pc := rd.str()
	msg := rd.hexBytes()
	mac := rd.hexBytes()
	if rd.err != nil {
		return codeFormatError, nil
	}
	alg, ok := algFromCode(ac)
	if !ok {
		return codeFormatError, nil
	}
	ok, err := s.v.VerifyMAC(keyRef, alg, padFromCode(pc), msg, mac)
	if err != nil {
		return hostCodeFor(err), nil
	}
	if ok {
		return codeOK, nil
	}
	return codeVerifyFail, nil
}

func readMACBody(data []byte) (keyRef string, alg vault.MACAlgorithm, pad vault.Padding, msg []byte, ok bool) {
	rd := newFieldReader(data)
	keyRef = rd.str()
	ac := rd.str()
	pc := rd.str()
	msg = rd.hexBytes()
	if rd.err != nil {
		return "", 0, 0, nil, false
	}
	alg, ok = algFromCode(ac)
	if !ok {
		return "", 0, 0, nil, false
	}
	return keyRef, alg, padFromCode(pc), msg, true
}

func hostCodeFor(err error) string {
	if errors.Is(err, vault.ErrUnknownKey) {
		return codeKeyNotFound
	}
	return codeFormatError
}
