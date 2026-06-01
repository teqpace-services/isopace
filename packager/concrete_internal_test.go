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

package packager

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "regenerate schemadef golden JSON files")

// schemaDefGolden maps each embedded schemadef file to the declarative config
// generated from the programmatic packager. The Go profile is the single source
// of truth; the JSON is emitted from the same description so the two forms cannot
// drift, and the golden test (or -update) keeps the committed files in sync.
func schemaDefGolden() map[string]schemaConfig {
	return map[string]schemaConfig{
		"zone.json":   zonePackager.config(),
		"fields.json": switchPackager("fields", "fields-127").config(),
		"switch.json": switchPackager("switch", "switch-127").config(),
	}
}

func TestSchemaDefsGolden(t *testing.T) {
	for name, cfg := range schemaDefGolden() {
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		data = append(data, '\n')
		path := filepath.Join("schemadef", name)

		if *update {
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
			continue
		}

		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s (regenerate with: go test ./packager -run TestSchemaDefsGolden -update): %v", name, err)
		}
		if !bytes.Equal(want, data) {
			t.Errorf("%s is stale; regenerate with: go test ./packager -run TestSchemaDefsGolden -update", name)
		}
	}
}
