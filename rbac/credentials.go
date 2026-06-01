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

package rbac

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io"
)

// DefaultIterations is the PBKDF2 work factor used by [HashPassword].
const DefaultIterations = 600_000

const (
	saltLen = 16
	keyLen  = 32
)

// Credential is a stored password verifier: a PBKDF2-HMAC-SHA256 hash with its
// salt and iteration count. It never holds the password itself.
type Credential struct {
	Salt       []byte
	Iterations int
	Hash       []byte
}

// HashPassword derives a [Credential] for password using a fresh random salt and
// the default work factor.
func HashPassword(password string) (Credential, error) {
	return hashPasswordRand(password, DefaultIterations, rand.Reader)
}

// hashPassword derives a credential at the given work factor (falling back to
// the default for a non-positive value), with a fresh random salt.
func hashPassword(password string, iterations int) (Credential, error) {
	if iterations <= 0 {
		iterations = DefaultIterations
	}
	return hashPasswordRand(password, iterations, rand.Reader)
}

func hashPasswordRand(password string, iterations int, r io.Reader) (Credential, error) {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(r, salt); err != nil {
		return Credential{}, fmt.Errorf("rbac: read salt: %w", err)
	}
	hash, err := pbkdf2.Key(sha256.New, password, salt, iterations, keyLen)
	if err != nil {
		return Credential{}, fmt.Errorf("rbac: derive key: %w", err)
	}
	return Credential{Salt: salt, Iterations: iterations, Hash: hash}, nil
}

// Verify reports whether password matches the credential, in constant time with
// respect to the hash comparison.
func (c Credential) Verify(password string) bool {
	if len(c.Hash) == 0 || c.Iterations <= 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, c.Salt, c.Iterations, len(c.Hash))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, c.Hash) == 1
}
