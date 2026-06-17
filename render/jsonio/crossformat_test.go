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

package jsonio_test

import (
	"bytes"
	"testing"

	"github.com/teqpace-services/isopace/iso8583"
	"github.com/teqpace-services/isopace/packager"
	"github.com/teqpace-services/isopace/render/jsonio"
	"github.com/teqpace-services/isopace/render/protobuf"
)

// TestCrossFormat verifies the SAME message projects losslessly through every
// lossless backend: ISO-8583 wire, JSON, and protobuf all reconstruct a
// wire-identical message (ARCHITECTURE.md §8 — one model, many renderings).
func TestCrossFormat(t *testing.T) {
	s := packager.ISO87A()
	c := iso8583.NewCodec(s)
	m := sample(t, s)

	want, err := c.Marshal(m, nil)
	if err != nil {
		t.Fatalf("ISO marshal: %v", err)
	}

	// via JSON
	jb, err := jsonio.Marshal(m)
	if err != nil {
		t.Fatalf("jsonio.Marshal: %v", err)
	}
	mj, err := jsonio.Unmarshal(jb, s)
	if err != nil {
		t.Fatalf("jsonio.Unmarshal: %v", err)
	}
	gotJSON, _ := c.Marshal(mj, nil)

	// via protobuf
	pb, err := protobuf.Marshal(m)
	if err != nil {
		t.Fatalf("protobuf.Marshal: %v", err)
	}
	mp, err := protobuf.Unmarshal(pb, s)
	if err != nil {
		t.Fatalf("protobuf.Unmarshal: %v", err)
	}
	gotPB, _ := c.Marshal(mp, nil)

	if !bytes.Equal(want, gotJSON) {
		t.Errorf("JSON backend diverged:\n want=%x\n  got=%x", want, gotJSON)
	}
	if !bytes.Equal(want, gotPB) {
		t.Errorf("protobuf backend diverged:\n want=%x\n  got=%x", want, gotPB)
	}
}
