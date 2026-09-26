// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectDetectorOverlayIDs_FromDemoDoctrineDirs(t *testing.T) {
	if _, err := os.Stat(filepath.Join("demos", "fedramp", "doctrine")); err != nil {
		t.Skip("demo doctrine directory not available from test working directory")
	}

	overlayIDs, err := collectDetectorOverlayIDs()
	if err != nil {
		t.Fatalf("collectDetectorOverlayIDs() error = %v", err)
	}
	if len(overlayIDs) == 0 {
		t.Fatal("expected overlay_ids from demo doctrine files")
	}
}

func TestValidate_RepoCatalogPassesOrReportsFinalizedGaps(t *testing.T) {
	overlayPath := filepath.Join("docs", "reference", "cosais-overlays.json")
	if _, err := os.Stat(overlayPath); err != nil {
		t.Skip("COSAiS overlay catalog not available from test working directory")
	}

	err := validate()
	if err != nil && !strings.Contains(err.Error(), "lack detector coverage") {
		t.Fatalf("validate() error = %v", err)
	}
}
