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

// Package vault is the Isopace payment-security layer: the cryptographic
// primitives a switch needs — PIN block formats (ISO 9564), message
// authentication codes (ISO 9797-1), DUKPT key derivation (ANSI X9.24-1), TR-31
// key blocks (ASC X9.143), and EMV application cryptograms — behind a [Vault]
// façade that abstracts where the keys live.
//
// # Backends and HSMs
//
// [SoftVault] is the in-process backend: keys are held in memory and operations
// run with the Go standard library's crypto. A real Hardware Security Module is
// a drop-in [Vault] adapter (e.g. PKCS#11 via cgo) in a separate, optionally
// imported module, so the core stays stdlib-only.
//
// SECURITY: SoftVault is for development, testing, and conformance — not for
// production PIN or key handling. Software key storage provides none of the
// physical and logical protections (tamper resistance, key separation, secure
// key loading) that PCI PIN and P2PE require; production deployments must use a
// certified HSM behind the Vault interface. The primitives here are implemented
// to the public standards and validated against published test vectors so the
// same code path can be checked for conformance.
//
// The 3DES-based formats are the widely deployed ones and are what these
// primitives implement; AES successors (ISO 9564 format 4, TR-31 version D) are
// noted where not yet covered.
package vault
