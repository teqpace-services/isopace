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

// Package rbac is role-based access control for Isopace operations: users hold
// roles, roles grant permissions, and an authorization check tests whether a
// user may perform an action. It also stores user credentials (PBKDF2-HMAC-SHA256
// password hashes) so the admin API can authenticate operators.
//
// Permissions are colon-namespaced strings ("tx:read", "config:write") and a
// grant may end in ":*" to cover a namespace, or be "*" to cover everything, so
// a role like "operator" can be granted "tx:*" without enumerating every action.
//
// The model is stdlib-only and safe for concurrent use; an external identity
// provider (LDAP, OIDC) is layered on top by feeding authenticated identities
// into the same [Policy].
package rbac
