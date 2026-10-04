// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
)

type doctrineCatalog struct {
	Doctrines []doctrineEntry `json:"doctrines"`
}

type doctrineEntry struct {
	OverlayIDs []string `json:"overlay_ids"`
}

func main() {
	overlayPath := filepath.Join(constants.DefaultOverlayDirPath, constants.COSAiSOverlaysFilename)
	doctrineGlob := filepath.Join(constants.DemosDirname, "*", constants.DemosDoctrineDir, "*.json")
	if err := validate(overlayPath, doctrineGlob, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// validate loads the overlay catalog at overlayPath, gathers detector
// overlay_ids from every doctrine file matching doctrineGlob, and reports
// finalized overlays that no detector covers. Progress is written to out.
func validate(overlayPath, doctrineGlob string, out io.Writer) error {
	overlayCatalog, err := compliance.LoadOverlayCatalog(overlayPath)
	if err != nil {
		return fmt.Errorf("cosais coverage: load overlay catalog: %w", err)
	}

	detectorOverlayIDs, err := collectDetectorOverlayIDs(doctrineGlob)
	if err != nil {
		return err
	}

	finalized := 0
	for _, overlay := range overlayCatalog.Overlays {
		if overlay.Status == compliance.OverlayStatusFinalized {
			finalized++
		}
	}
	if finalized == 0 {
		fmt.Fprintln(out, "PASS: No finalized COSAiS overlays yet.")
		fmt.Fprintln(out, "      Detector overlay_ids will be checked when NIST finalizes.")
		return nil
	}

	uncovered := compliance.CheckFinalizedOverlayCoverage(overlayCatalog, detectorOverlayIDs)
	if len(uncovered) > 0 {
		fmt.Fprintln(out, "FAIL: Finalized COSAiS overlays without detector coverage:")
		for _, id := range uncovered {
			fmt.Fprintf(out, "  - %s\n", id)
		}
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Action required: populate overlay_ids in doctrine JSON files")
		fmt.Fprintln(out, "to reference each finalized overlay, then re-run:")
		fmt.Fprintln(out, "  make cosais-validate")
		return fmt.Errorf("cosais coverage: %d finalized overlay(s) lack detector coverage", len(uncovered))
	}

	fmt.Fprintf(out, "PASS: All %d finalized COSAiS overlay(s) have detector coverage.\n", finalized)
	return nil
}

// collectDetectorOverlayIDs flattens overlay_ids across every doctrine file
// matching pattern. It errors when no file matches so a mislocated working
// directory cannot silently report zero coverage.
func collectDetectorOverlayIDs(pattern string) ([]string, error) {
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("cosais coverage: list doctrine files: %w", err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("cosais coverage: no doctrine files match %s", pattern)
	}

	overlayIDs := make([]string, 0)
	for _, path := range paths {
		encoded, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("cosais coverage: read %s: %w", path, err)
		}
		var catalog doctrineCatalog
		if err := json.Unmarshal(encoded, &catalog); err != nil {
			return nil, fmt.Errorf("cosais coverage: parse %s: %w", path, err)
		}
		for _, entry := range catalog.Doctrines {
			overlayIDs = append(overlayIDs, entry.OverlayIDs...)
		}
	}
	return overlayIDs, nil
}
