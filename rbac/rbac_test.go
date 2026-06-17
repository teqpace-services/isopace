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

package rbac_test

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/teqpace-services/isopace/rbac"
)

func TestPolicyAuthorization(t *testing.T) {
	p := rbac.NewPolicy(rbac.WithIterations(1)) // no credentials exercised here
	p.AddRole("operator", "tx:read", "tx:reverse")
	p.AddRole("admin", "admin:*")
	p.AddRole("super", "*")
	p.AddUser("alice", "operator")
	p.AddUser("bob", "admin")
	p.AddUser("root", "super")
	p.AddUser("nobody")

	cases := []struct {
		user string
		perm rbac.Permission
		want bool
	}{
		{"alice", "tx:read", true},
		{"alice", "tx:reverse", true},
		{"alice", "tx:void", false}, // not granted
		{"alice", "admin:restart", false},
		{"bob", "admin:restart", true}, // admin:* wildcard
		{"bob", "admin:read", true},
		{"bob", "tx:read", false},
		{"root", "anything:at:all", true}, // * matches everything
		{"nobody", "tx:read", false},      // no roles
		{"ghost", "tx:read", false},       // unknown user
	}
	for _, c := range cases {
		if got := p.Can(c.user, c.perm); got != c.want {
			t.Errorf("Can(%q, %q) = %v want %v", c.user, c.perm, got, c.want)
		}
	}
}

func TestPolicyGrantRevokeRoles(t *testing.T) {
	p := rbac.NewPolicy(rbac.WithIterations(1)) // no credentials exercised here
	p.AddRole("r", "x:y")
	p.AddUser("u")
	if p.Can("u", "x:y") {
		t.Fatal("user has permission before grant")
	}
	if err := p.Grant("u", "r"); err != nil {
		t.Fatal(err)
	}
	if !p.Can("u", "x:y") {
		t.Error("grant did not take effect")
	}
	roles, err := p.Roles("u")
	if err != nil || !slices.Equal(roles, []string{"r"}) {
		t.Errorf("Roles = %v,%v", roles, err)
	}
	if err := p.Revoke("u", "r"); err != nil {
		t.Fatal(err)
	}
	if p.Can("u", "x:y") {
		t.Error("revoke did not take effect")
	}
	if err := p.Grant("ghost", "r"); !errors.Is(err, rbac.ErrNoSuchUser) {
		t.Errorf("Grant unknown user = %v want ErrNoSuchUser", err)
	}
}

func TestCredentialsAuthenticate(t *testing.T) {
	p := rbac.NewPolicy(rbac.WithIterations(4096)) // low work factor keeps the test fast
	p.AddUser("alice", "admin")
	if p.Authenticate("alice", "pw") {
		t.Error("authenticated before password set")
	}
	if err := p.SetPassword("alice", "s3cr3t"); err != nil {
		t.Fatal(err)
	}
	if !p.Authenticate("alice", "s3cr3t") {
		t.Error("correct password rejected")
	}
	if p.Authenticate("alice", "wrong") {
		t.Error("wrong password accepted")
	}
	if p.Authenticate("ghost", "x") {
		t.Error("unknown user authenticated")
	}
	// A user with no password set must not authenticate (and must take the
	// constant-time dummy path, not an early map-miss return).
	p.AddUser("nopw", "admin")
	if p.Authenticate("nopw", "anything") {
		t.Error("password-less user authenticated")
	}
	if err := p.SetPassword("ghost", "x"); !errors.Is(err, rbac.ErrNoSuchUser) {
		t.Errorf("SetPassword unknown user = %v want ErrNoSuchUser", err)
	}
}

func TestCredentialHashUniqueSaltAndVerify(t *testing.T) {
	c1, err := rbac.HashPassword("same")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := rbac.HashPassword("same")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(c1.Salt, c2.Salt) {
		t.Error("salts are not unique")
	}
	if bytes.Equal(c1.Hash, c2.Hash) {
		t.Error("same password produced identical hashes (salt not applied)")
	}
	if !c1.Verify("same") || c1.Verify("different") {
		t.Error("Verify result incorrect")
	}
	// Re-grant a role keeps the password (AddUser preserves credential).
	var zero rbac.Credential
	if zero.Verify("anything") {
		t.Error("zero credential verified")
	}
}

func TestAddUserPreservesPassword(t *testing.T) {
	p := rbac.NewPolicy(rbac.WithIterations(4096))
	p.AddUser("alice")
	if err := p.SetPassword("alice", "pw"); err != nil {
		t.Fatal(err)
	}
	p.AddUser("alice", "admin") // re-declare with roles
	if !p.Authenticate("alice", "pw") {
		t.Error("re-declaring the user dropped the password")
	}
}
