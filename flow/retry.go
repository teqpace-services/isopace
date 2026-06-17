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

package flow

import (
	"context"
	"time"
)

// RetryPolicy governs per-stage prepare retries. Retryable decides whether an
// error warrants another attempt (default: any non-nil error). Backoff returns
// the delay before attempt n (1-based; default: a fixed 50ms). MaxAttempts caps
// total prepare attempts including the first (default 3).
type RetryPolicy struct {
	MaxAttempts int
	Backoff     func(attempt int) time.Duration
	Retryable   func(err error) bool
}

// DefaultRetryPolicy returns a sensible policy: up to 3 attempts, fixed 50ms
// backoff, all errors retryable.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		Backoff:     func(int) time.Duration { return 50 * time.Millisecond },
		Retryable:   func(error) bool { return true },
	}
}

func (p RetryPolicy) normalised() RetryPolicy {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 3
	}
	if p.Backoff == nil {
		p.Backoff = func(int) time.Duration { return 50 * time.Millisecond }
	}
	if p.Retryable == nil {
		p.Retryable = func(error) bool { return true }
	}
	return p
}

// retryStage wraps a stage to retry its Prepare on retryable errors. Commit and
// Abort are forwarded to the inner stage unchanged (and are not retried).
type retryStage struct {
	inner  Stage
	policy RetryPolicy
}

// Retry wraps inner so its Prepare is retried per policy. Commit/Abort (if inner
// implements them) are forwarded and run once. Retrying Prepare assumes its work
// is safe to repeat — keep reservations with side effects in Commit.
func Retry(inner Stage, policy RetryPolicy) Stage {
	return retryStage{inner: inner, policy: policy.normalised()}
}

func (r retryStage) Name() string { return r.inner.Name() }

func (r retryStage) Prepare(ctx context.Context, ex *Exchange) (Result, error) {
	var res Result
	var err error
	for attempt := 1; attempt <= r.policy.MaxAttempts; attempt++ {
		res, err = r.inner.Prepare(ctx, ex)
		// An explicit Abort on the exchange is a decision, not a transient
		// failure — do not retry over it.
		if err == nil || ex.aborted || !r.policy.Retryable(err) {
			return res, err
		}
		if attempt == r.policy.MaxAttempts {
			break
		}
		timer := time.NewTimer(r.policy.Backoff(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return res, ctx.Err()
		case <-timer.C:
		}
	}
	return res, err
}

func (r retryStage) Commit(ctx context.Context, ex *Exchange) error {
	if c, ok := r.inner.(Committer); ok {
		return c.Commit(ctx, ex)
	}
	return nil
}

func (r retryStage) Abort(ctx context.Context, ex *Exchange) error {
	if a, ok := r.inner.(Aborter); ok {
		return a.Abort(ctx, ex)
	}
	return nil
}
