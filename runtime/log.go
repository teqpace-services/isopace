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

package runtime

import (
	"io"
	"log/slog"
	"strings"
)

// LogFormat selects the slog handler used by [NewLogger].
type LogFormat int

const (
	// LogText uses slog's human-readable text handler.
	LogText LogFormat = iota
	// LogJSON uses slog's structured JSON handler (production default).
	LogJSON
)

// LogOptions configures [NewLogger].
type LogOptions struct {
	// Level is the minimum level to emit (default slog.LevelInfo).
	Level slog.Level
	// Format selects text or JSON output (default LogText).
	Format LogFormat
	// AddSource includes source file:line in records.
	AddSource bool
}

// NewLogger builds a *slog.Logger writing to w with the given options. It is the
// one place runtime constructs handlers, so deployments get consistent output.
func NewLogger(w io.Writer, opts LogOptions) *slog.Logger {
	h := &slog.HandlerOptions{Level: opts.Level, AddSource: opts.AddSource}
	var handler slog.Handler
	switch opts.Format {
	case LogJSON:
		handler = slog.NewJSONHandler(w, h)
	default:
		handler = slog.NewTextHandler(w, h)
	}
	return slog.New(handler)
}

// ParseLevel maps a case-insensitive level name ("debug", "info", "warn",
// "error") to an slog.Level, falling back to def for anything unrecognised.
func ParseLevel(s string, def slog.Level) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "err":
		return slog.LevelError
	default:
		return def
	}
}
