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

package space

import (
	"context"
	"errors"
)

// ErrClosed is returned by blocking operations on a closed space.
var ErrClosed = errors.New("space: closed")

// Local is the canonical Space; a distributed backend implements the same shape.
var _ Space = (*Local)(nil)

// Space is a keyed tuple space. A key holds a FIFO bag of entries, so a key is a
// queue: Out enqueues, In dequeues (take), and Rd peeks the head (read) without
// removing it. The blocking forms (In, Rd) wait until an entry is available,
// bounded by the supplied context; the probe forms (Inp, Rdp) never block.
//
// Implementations must be safe for concurrent use.
type Space interface {
	// Out writes value under key. After the space is closed it is a no-op.
	Out(key string, value any)

	// In blocks until an entry is available under key, then removes and returns
	// it. It returns the context error if ctx is cancelled or its deadline
	// passes, or ErrClosed if the space is closed while waiting.
	In(ctx context.Context, key string) (any, error)

	// Rd is like In but returns the head entry without removing it, so the same
	// entry can be read by several callers and later taken with In.
	Rd(ctx context.Context, key string) (any, error)

	// Inp is the non-blocking form of In: ok is false if no entry is present.
	Inp(key string) (value any, ok bool)

	// Rdp is the non-blocking form of Rd.
	Rdp(key string) (value any, ok bool)

	// Len reports how many entries are queued under key.
	Len(key string) int

	// Close releases the space and unblocks waiters with ErrClosed.
	Close() error
}
