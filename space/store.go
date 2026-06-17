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
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Store is a durable store-and-forward queue: a keyed FIFO of []byte payloads
// backed by a crash-safe append-only log. Out (Push) appends an "out" record and
// In (Pull) appends a tombstone, so the queue state is reconstructed by replaying
// the log on Open. It is the persistence behind a switch that must hold messages
// for an endpoint that is currently down and forward them after a restart.
//
// Because disk I/O can fail, Store's API returns errors and so is a sibling of
// [Space] rather than an implementation of it. Payloads are held in memory for
// fast access; the log is the durable backing. Store is safe for concurrent use.
type Store struct {
	mu     sync.Mutex
	path   string
	file   *os.File
	sync   bool
	notify chan struct{}
	closed bool

	data map[string][]record
	seq  uint64
}

type record struct {
	seq uint64
	val []byte
}

const (
	opOut byte = 1
	opIn  byte = 2
)

// StoreOption configures a Store.
type StoreOption func(*Store)

// WithSync controls whether each log write is flushed to disk with fsync
// (default true). Disabling it trades durability for throughput and is intended
// for tests or non-critical queues.
func WithSync(on bool) StoreOption { return func(s *Store) { s.sync = on } }

// Open opens (creating if needed) the store backed by the log at path, replaying
// it to rebuild the queue state.
func Open(path string, opts ...StoreOption) (*Store, error) {
	s := &Store{
		path:   path,
		sync:   true,
		notify: make(chan struct{}),
		data:   map[string][]record{},
	}
	for _, o := range opts {
		o(s)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("space: open store: %w", err)
	}
	s.file = f
	if err := s.replay(); err != nil {
		f.Close()
		return nil, err
	}
	return s, nil
}

// replay reconstructs queue state from the log. A truncated trailing record
// (from a crash mid-write) is ignored, so the log is crash-safe.
func (s *Store) replay() error {
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("space: rewind log: %w", err)
	}
	r := bufio.NewReader(s.file)

	type live struct {
		seq uint64
		key string
		val []byte
	}
	var entries []live
	tomb := map[uint64]struct{}{}

	for {
		op, seq, key, val, err := readRecord(r)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break // clean end or truncated tail: stop replay
		}
		if err != nil {
			return fmt.Errorf("space: replay log: %w", err)
		}
		switch op {
		case opOut:
			entries = append(entries, live{seq: seq, key: key, val: val})
			if seq >= s.seq {
				s.seq = seq + 1
			}
		case opIn:
			tomb[seq] = struct{}{}
		}
	}
	for _, e := range entries {
		if _, dead := tomb[e.seq]; dead {
			continue
		}
		s.data[e.key] = append(s.data[e.key], record{seq: e.seq, val: e.val})
	}
	return nil
}

// Push enqueues a durable copy of value under key.
func (s *Store) Push(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	seq := s.seq
	if err := s.append(opOut, seq, key, value); err != nil {
		return err
	}
	s.seq++
	s.data[key] = append(s.data[key], record{seq: seq, val: append([]byte(nil), value...)})
	s.signal()
	return nil
}

// Pull removes and returns the head payload under key, blocking until one is
// available or ctx ends. The tombstone is durably written before the entry
// leaves memory, so a crash never resurrects a delivered entry.
func (s *Store) Pull(ctx context.Context, key string) ([]byte, error) {
	for {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return nil, ErrClosed
		}
		if len(s.data[key]) > 0 {
			v, err := s.takeHead(key)
			s.mu.Unlock()
			return v, err
		}
		n := s.notify
		s.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-n:
		}
	}
}

// Pullp is the non-blocking form of Pull: ok is false if the queue is empty.
func (s *Store) Pullp(key string) (value []byte, ok bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, false, ErrClosed
	}
	if len(s.data[key]) == 0 {
		return nil, false, nil
	}
	v, err := s.takeHead(key)
	if err != nil {
		return nil, false, err
	}
	return v, true, nil
}

// takeHead writes the head's tombstone, then removes it from memory. Caller holds
// the lock and has verified the queue is non-empty.
func (s *Store) takeHead(key string) ([]byte, error) {
	q := s.data[key]
	head := q[0]
	if err := s.append(opIn, head.seq, "", nil); err != nil {
		return nil, err // entry stays queued; nothing removed
	}
	q[0] = record{}
	rest := q[1:]
	if len(rest) == 0 {
		delete(s.data, key)
	} else {
		s.data[key] = rest
	}
	return head.val, nil
}

// Peek returns a copy of the head payload under key without removing it.
func (s *Store) Peek(key string) (value []byte, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := s.data[key]
	if len(q) == 0 {
		return nil, false
	}
	return append([]byte(nil), q[0].val...), true
}

// Len reports the number of entries queued under key.
func (s *Store) Len(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.data[key])
}

// signal wakes all waiters. Caller holds the lock.
func (s *Store) signal() {
	close(s.notify)
	s.notify = make(chan struct{})
}

// append writes one record and flushes it if sync is enabled. Caller holds the
// lock.
func (s *Store) append(op byte, seq uint64, key string, val []byte) error {
	if _, err := s.file.Write(encodeRecord(op, seq, key, val)); err != nil {
		return fmt.Errorf("space: append log: %w", err)
	}
	if s.sync {
		if err := s.file.Sync(); err != nil {
			return fmt.Errorf("space: sync log: %w", err)
		}
	}
	return nil
}

// Compact rewrites the log with only the live entries, discarding tombstones and
// taken payloads, then atomically replaces the old log. Sequence numbers are
// preserved, so it is safe to call at any time.
func (s *Store) Compact() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}

	tmp := s.path + ".compact"
	tf, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("space: compact create: %w", err)
	}
	w := bufio.NewWriter(tf)
	for key, q := range s.data {
		for _, rec := range q {
			if _, err := w.Write(encodeRecord(opOut, rec.seq, key, rec.val)); err != nil {
				tf.Close()
				os.Remove(tmp)
				return fmt.Errorf("space: compact write: %w", err)
			}
		}
	}
	if err := w.Flush(); err != nil {
		tf.Close()
		os.Remove(tmp)
		return fmt.Errorf("space: compact flush: %w", err)
	}
	if err := tf.Sync(); err != nil {
		tf.Close()
		os.Remove(tmp)
		return fmt.Errorf("space: compact sync: %w", err)
	}
	tf.Close()

	if err := s.file.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("space: compact close old: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		// Best effort to reopen the original so the store stays usable.
		s.file, _ = os.OpenFile(s.path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
		return fmt.Errorf("space: compact rename: %w", err)
	}
	// fsync the parent directory so the rename is durable. Without this the
	// rename can be lost on a crash and the pre-compaction log would reappear,
	// silently undoing the compaction the caller was told succeeded.
	if err := syncDir(s.path); err != nil {
		s.file, _ = os.OpenFile(s.path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
		return fmt.Errorf("space: compact fsync dir: %w", err)
	}
	f, err := os.OpenFile(s.path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("space: compact reopen: %w", err)
	}
	s.file = f
	return nil
}

// syncDir fsyncs the directory containing path, making a create/rename within it
// durable.
func syncDir(path string) error {
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// Close flushes and closes the log and unblocks waiters with ErrClosed.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.signal()
	return s.file.Close()
}

// encodeRecord serialises one record as: uint32 bodyLen, then body. The body is
// op(1) seq(8) and, for opOut, keyLen(4) key valLen(4) val. The length prefix
// lets replay detect and ignore a truncated trailing record.
func encodeRecord(op byte, seq uint64, key string, val []byte) []byte {
	bodyLen := 1 + 8
	if op == opOut {
		bodyLen += 4 + len(key) + 4 + len(val)
	}
	buf := make([]byte, 4+bodyLen)
	binary.BigEndian.PutUint32(buf[0:4], uint32(bodyLen))
	buf[4] = op
	binary.BigEndian.PutUint64(buf[5:13], seq)
	if op == opOut {
		off := 13
		binary.BigEndian.PutUint32(buf[off:off+4], uint32(len(key)))
		off += 4
		off += copy(buf[off:], key)
		binary.BigEndian.PutUint32(buf[off:off+4], uint32(len(val)))
		off += 4
		copy(buf[off:], val)
	}
	return buf
}

// readRecord reads one record. It returns io.EOF at a clean boundary and
// io.ErrUnexpectedEOF for a truncated trailing record; both end replay.
func readRecord(r *bufio.Reader) (op byte, seq uint64, key string, val []byte, err error) {
	var lenBuf [4]byte
	if _, err = io.ReadFull(r, lenBuf[:]); err != nil {
		return // io.EOF if nothing left; io.ErrUnexpectedEOF if partial
	}
	bodyLen := binary.BigEndian.Uint32(lenBuf[:])
	if bodyLen < 9 {
		return 0, 0, "", nil, fmt.Errorf("corrupt record: body length %d", bodyLen)
	}
	body := make([]byte, bodyLen)
	if _, err = io.ReadFull(r, body); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return
	}
	op = body[0]
	seq = binary.BigEndian.Uint64(body[1:9])
	if op != opOut {
		return op, seq, "", nil, nil
	}
	off := 9
	if off+4 > len(body) {
		return 0, 0, "", nil, fmt.Errorf("corrupt record: key length")
	}
	kl := int(binary.BigEndian.Uint32(body[off : off+4]))
	off += 4
	if off+kl+4 > len(body) {
		return 0, 0, "", nil, fmt.Errorf("corrupt record: key/val bounds")
	}
	key = string(body[off : off+kl])
	off += kl
	vl := int(binary.BigEndian.Uint32(body[off : off+4]))
	off += 4
	if off+vl > len(body) {
		return 0, 0, "", nil, fmt.Errorf("corrupt record: val bounds")
	}
	val = append([]byte(nil), body[off:off+vl]...)
	return op, seq, key, val, nil
}
