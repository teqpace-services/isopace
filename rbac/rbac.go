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

package rbac

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Permission is a colon-namespaced action identifier, e.g. "tx:read". A grant of
// "ns:*" covers every action in namespace ns; "*" covers everything.
type Permission string

// Errors returned by the policy.
var (
	ErrNoSuchRole = errors.New("rbac: no such role")
	ErrNoSuchUser = errors.New("rbac: no such user")
)

// Policy is the RBAC model: roles map to permission sets and users map to roles
// (and an optional credential). It is safe for concurrent use.
type Policy struct {
	mu         sync.RWMutex
	roles      map[string]map[Permission]struct{}
	users      map[string]*userRec
	iterations int
	dummy      *Credential // for constant-time Authenticate of unknown users
}

type userRec struct {
	roles map[string]struct{}
	cred  *Credential
}

// Option configures a Policy.
type Option func(*Policy)

// WithIterations sets the PBKDF2 work factor used by [Policy.SetPassword]
// (default [DefaultIterations]). Lower it only for tests.
func WithIterations(n int) Option {
	return func(p *Policy) {
		if n > 0 {
			p.iterations = n
		}
	}
}

// NewPolicy returns an empty policy.
func NewPolicy(opts ...Option) *Policy {
	p := &Policy{
		roles:      map[string]map[Permission]struct{}{},
		users:      map[string]*userRec{},
		iterations: DefaultIterations,
	}
	for _, o := range opts {
		o(p)
	}
	// A dummy credential at the policy's work factor lets Authenticate do
	// equivalent PBKDF2 work for unknown users, so response time does not reveal
	// whether a user exists.
	if d, err := hashPassword("", p.iterations); err == nil {
		p.dummy = &d
	}
	return p
}

// AddRole defines (or replaces) a role with the given permissions.
func (p *Policy) AddRole(name string, perms ...Permission) {
	set := make(map[Permission]struct{}, len(perms))
	for _, perm := range perms {
		set[perm] = struct{}{}
	}
	p.mu.Lock()
	p.roles[name] = set
	p.mu.Unlock()
}

// AddUser defines (or replaces) a user with the given roles. Unknown roles are
// allowed (a role may be defined later); they simply grant nothing until defined.
func (p *Policy) AddUser(name string, roles ...string) {
	rec := &userRec{roles: make(map[string]struct{}, len(roles))}
	for _, r := range roles {
		rec.roles[r] = struct{}{}
	}
	p.mu.Lock()
	if existing, ok := p.users[name]; ok {
		rec.cred = existing.cred // preserve any set password
	}
	p.users[name] = rec
	p.mu.Unlock()
}

// Grant adds a role to a user.
func (p *Policy) Grant(user, role string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	rec, ok := p.users[user]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNoSuchUser, user)
	}
	rec.roles[role] = struct{}{}
	return nil
}

// Revoke removes a role from a user.
func (p *Policy) Revoke(user, role string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	rec, ok := p.users[user]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNoSuchUser, user)
	}
	delete(rec.roles, role)
	return nil
}

// SetPassword stores a credential for the user, using the policy's work factor.
func (p *Policy) SetPassword(user, password string) error {
	cred, err := hashPassword(password, p.iterations)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	rec, ok := p.users[user]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNoSuchUser, user)
	}
	rec.cred = &cred
	return nil
}

// Authenticate reports whether the user exists, has a password set, and the
// password matches. It performs equivalent PBKDF2 work whether or not the user
// exists, so timing does not reveal which usernames are registered.
func (p *Policy) Authenticate(user, password string) bool {
	p.mu.RLock()
	rec, ok := p.users[user]
	var cred *Credential
	if ok {
		cred = rec.cred
	}
	dummy := p.dummy
	p.mu.RUnlock()
	if cred == nil {
		if dummy != nil {
			_ = dummy.Verify(password) // constant-time defence; result discarded
		}
		return false
	}
	return cred.Verify(password)
}

// Can reports whether the user holds a permission that satisfies perm, through
// any of its roles.
func (p *Policy) Can(user string, perm Permission) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	rec, ok := p.users[user]
	if !ok {
		return false
	}
	for role := range rec.roles {
		for granted := range p.roles[role] {
			if permMatch(granted, perm) {
				return true
			}
		}
	}
	return false
}

// Roles returns the user's roles, sorted, or an error if the user is unknown.
func (p *Policy) Roles(user string) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	rec, ok := p.users[user]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNoSuchUser, user)
	}
	out := make([]string, 0, len(rec.roles))
	for r := range rec.roles {
		out = append(out, r)
	}
	sort.Strings(out)
	return out, nil
}

// permMatch reports whether a granted permission satisfies a requested one. A
// grant of "*" matches anything; "ns:*" matches any "ns:..." action; otherwise
// the grant must equal the request.
func permMatch(granted, requested Permission) bool {
	if granted == "*" || granted == requested {
		return true
	}
	if strings.HasSuffix(string(granted), ":*") {
		prefix := strings.TrimSuffix(string(granted), "*") // keep the trailing colon
		return strings.HasPrefix(string(requested), prefix)
	}
	return false
}
