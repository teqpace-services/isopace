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

package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config is a hierarchical configuration tree loaded from JSON and overlaid with
// environment variables. Values are addressed by dot-separated paths
// ("link.timeout", "logging.level"). It is safe for concurrent reads, and
// [Config.Reload] swaps the whole tree atomically so a [ConfigWatcher] can apply
// edits to a live process.
//
// (YAML is a drop-in: parse it into the same map[string]any tree and the rest of
// this type is unchanged. JSON keeps the core dependency-free today.)
type Config struct {
	mu      sync.RWMutex
	tree    map[string]any
	path    string   // source file for Reload; empty if parsed from bytes
	prefix  string   // env override prefix
	environ []string // captured environment for overrides (os.Environ by default)
}

type loadCfg struct {
	prefix  string
	environ []string
}

// LoadOption configures [Load] / [Parse].
type LoadOption func(*loadCfg)

// WithEnvPrefix enables environment overrides: a variable PREFIX_A_B becomes the
// config path "a.b". The value is parsed as JSON when possible (so "5" is a
// number and "true" a bool), otherwise kept as a string. An empty prefix (the
// default) disables overrides.
func WithEnvPrefix(prefix string) LoadOption { return func(c *loadCfg) { c.prefix = prefix } }

// WithEnviron supplies the environment to read overrides from, in os.Environ
// "KEY=value" form. Intended for tests; defaults to os.Environ().
func WithEnviron(env []string) LoadOption {
	return func(c *loadCfg) { c.environ = append([]string(nil), env...) }
}

// Load reads and parses a JSON config file, then applies environment overrides.
func Load(path string, opts ...LoadOption) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("runtime: load config: %w", err)
	}
	c, err := Parse(data, opts...)
	if err != nil {
		return nil, err
	}
	c.path = path
	return c, nil
}

// Parse builds a Config from JSON bytes (no file backing, so Reload is a no-op).
func Parse(data []byte, opts ...LoadOption) (*Config, error) {
	lc := loadCfg{environ: os.Environ()}
	for _, o := range opts {
		o(&lc)
	}
	tree, err := buildTree(data, lc)
	if err != nil {
		return nil, err
	}
	return &Config{tree: tree, prefix: lc.prefix, environ: lc.environ}, nil
}

func buildTree(data []byte, lc loadCfg) (map[string]any, error) {
	tree := map[string]any{}
	if len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &tree); err != nil {
			return nil, fmt.Errorf("runtime: parse config: %w", err)
		}
	}
	if lc.prefix != "" {
		applyEnvOverrides(tree, lc.prefix, lc.environ)
	}
	return tree, nil
}

func applyEnvOverrides(tree map[string]any, prefix string, environ []string) {
	pfx := prefix
	if !strings.HasSuffix(pfx, "_") {
		pfx += "_"
	}
	for _, kv := range environ {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		key, val := kv[:eq], kv[eq+1:]
		if !strings.HasPrefix(key, pfx) {
			continue
		}
		path := strings.ToLower(strings.ReplaceAll(key[len(pfx):], "_", "."))
		setPath(tree, strings.Split(path, "."), coerce(val))
	}
}

// coerce parses an env value as JSON when it is well-formed, so numbers and
// booleans land as the right type; otherwise it stays a string.
func coerce(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		switch v.(type) {
		case float64, bool, nil:
			return v
		}
	}
	return s
}

func setPath(tree map[string]any, segs []string, val any) {
	m := tree
	for i, s := range segs {
		if i == len(segs)-1 {
			m[s] = val
			return
		}
		child, ok := m[s].(map[string]any)
		if !ok {
			child = map[string]any{}
			m[s] = child
		}
		m = child
	}
}

// Get returns the raw value at path and whether it was present.
func (c *Config) Get(path string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return lookup(c.tree, strings.Split(path, "."))
}

func lookup(tree map[string]any, segs []string) (any, bool) {
	var cur any = tree
	for _, s := range segs {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[s]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// String returns the string at path, or def if absent or not a string.
func (c *Config) String(path, def string) string {
	if v, ok := c.Get(path); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}

// Int returns the integer at path, accepting JSON numbers and numeric strings.
func (c *Config) Int(path string, def int) int {
	v, ok := c.Get(path)
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i
		}
	}
	return def
}

// Bool returns the boolean at path, accepting JSON bools and "true"/"false".
func (c *Config) Bool(path string, def bool) bool {
	v, ok := c.Get(path)
	if !ok {
		return def
	}
	switch b := v.(type) {
	case bool:
		return b
	case string:
		if pb, err := strconv.ParseBool(b); err == nil {
			return pb
		}
	}
	return def
}

// Duration returns the time.Duration at path. It accepts a duration string
// ("250ms", "5s") or a JSON number interpreted as seconds.
func (c *Config) Duration(path string, def time.Duration) time.Duration {
	v, ok := c.Get(path)
	if !ok {
		return def
	}
	switch d := v.(type) {
	case string:
		if pd, err := time.ParseDuration(d); err == nil {
			return pd
		}
	case float64:
		return time.Duration(d * float64(time.Second))
	}
	return def
}

// Unmarshal decodes the whole config tree into v (a pointer to a struct), using
// encoding/json tags.
func (c *Config) Unmarshal(v any) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return remarshal(c.tree, v)
}

// UnmarshalKey decodes the subtree at path into v.
func (c *Config) UnmarshalKey(path string, v any) error {
	sub, ok := c.Get(path)
	if !ok {
		return fmt.Errorf("runtime: config key %q not found", path)
	}
	return remarshal(sub, v)
}

func remarshal(node, v any) error {
	data, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("runtime: encode config node: %w", err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("runtime: decode config: %w", err)
	}
	return nil
}

// Reload re-reads the source file and re-applies environment overrides, swapping
// the tree atomically. It returns nil without changes for a Config that was
// parsed from bytes (no file backing).
func (c *Config) Reload() error {
	c.mu.RLock()
	path, prefix, environ := c.path, c.prefix, c.environ
	c.mu.RUnlock()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("runtime: reload config: %w", err)
	}
	tree, err := buildTree(data, loadCfg{prefix: prefix, environ: environ})
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.tree = tree
	c.mu.Unlock()
	return nil
}
