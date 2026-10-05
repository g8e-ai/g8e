// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance"
)

// repoRoot is the repository root relative to this package's directory.
const repoRoot = "../../.."

type overlayFixture struct {
	ID     string
	Status compliance.OverlayStatus
}

// writeOverlayCatalog writes a valid overlay catalog containing overlays and
// returns its path.
func writeOverlayCatalog(t *testing.T, dir string, overlays ...overlayFixture) string {
	t.Helper()
	catalog := compliance.OverlayCatalog{Version: "1.0", Source: "test"}
	for _, overlay := range overlays {
		catalog.Overlays = append(catalog.Overlays, compliance.Overlay{
			ID: overlay.ID, Title: overlay.ID, UseCase: "use-case", Status: overlay.Status,
		})
	}
	encoded, err := json.Marshal(catalog)
	require.NoError(t, err)
	path := filepath.Join(dir, constants.COSAiSOverlaysFilename)
	require.NoError(t, os.WriteFile(path, encoded, 0o644))
	return path
}

// writeDoctrineFile writes a doctrine file whose detectors carry the given
// overlay_ids, one detector per entry, and returns a glob matching it.
func writeDoctrineFile(t *testing.T, dir, name string, detectorOverlayIDs ...[]string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	doctrines := make([]map[string]any, 0, len(detectorOverlayIDs))
	for _, ids := range detectorOverlayIDs {
		doctrines = append(doctrines, map[string]any{"overlay_ids": ids})
	}
	encoded, err := json.Marshal(map[string]any{"doctrines": doctrines})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), encoded, 0o644))
	return filepath.Join(dir, "*.json")
}

func TestCollectDetectorOverlayIDs_FlattensAcrossFilesAndDetectors(t *testing.T) {
	root := t.TempDir()
	writeDoctrineFile(t, filepath.Join(root, "demos", "alpha", "doctrine"), "a.json", []string{"OV-1", "OV-2"}, nil)
	writeDoctrineFile(t, filepath.Join(root, "demos", "beta", "doctrine"), "b.json", []string{"OV-3"})

	got, err := collectDetectorOverlayIDs(filepath.Join(root, "demos", "*", "doctrine", "*.json"))

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"OV-1", "OV-2", "OV-3"}, got)
}

func TestCollectDetectorOverlayIDs_ReturnsEmptyNonNilSliceWhenDetectorsHaveNoOverlays(t *testing.T) {
	glob := writeDoctrineFile(t, t.TempDir(), "d.json", nil, []string{})

	got, err := collectDetectorOverlayIDs(glob)

	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestCollectDetectorOverlayIDs_ErrorsWhenNoDoctrineFileMatches(t *testing.T) {
	pattern := filepath.Join(t.TempDir(), "demos", "*", "doctrine", "*.json")

	_, err := collectDetectorOverlayIDs(pattern)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no doctrine files match")
	assert.Contains(t, err.Error(), pattern)
}

func TestCollectDetectorOverlayIDs_RejectsMalformedGlobPattern(t *testing.T) {
	_, err := collectDetectorOverlayIDs("[")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "list doctrine files")
	assert.ErrorIs(t, err, filepath.ErrBadPattern)
}

func TestCollectDetectorOverlayIDs_ReportsUnparseableDoctrineFileByPath(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "broken.json")
	require.NoError(t, os.WriteFile(bad, []byte("{not json"), 0o644))

	_, err := collectDetectorOverlayIDs(filepath.Join(dir, "*.json"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse "+bad)
}

func TestCollectDetectorOverlayIDs_ReportsUnreadableDoctrineFileByPath(t *testing.T) {
	dir := t.TempDir()
	// A directory whose name matches *.json is listed by Glob but cannot be read as a file.
	unreadable := filepath.Join(dir, "dir.json")
	require.NoError(t, os.Mkdir(unreadable, 0o755))

	_, err := collectDetectorOverlayIDs(filepath.Join(dir, "*.json"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read "+unreadable)
}

func TestValidate_PassesWithNoFinalizedOverlaysAndExplainsWhy(t *testing.T) {
	dir := t.TempDir()
	overlays := writeOverlayCatalog(t, dir,
		overlayFixture{ID: "OV-DRAFT", Status: compliance.OverlayStatusDraft},
		overlayFixture{ID: "OV-OLD", Status: compliance.OverlayStatusDeprecated})
	doctrines := writeDoctrineFile(t, filepath.Join(dir, "doctrine"), "d.json", nil)
	var out bytes.Buffer

	require.NoError(t, validate(overlays, doctrines, &out))

	assert.Contains(t, out.String(), "PASS: No finalized COSAiS overlays yet.")
}

func TestValidate_PassesWhenEveryFinalizedOverlayHasDetectorCoverage(t *testing.T) {
	dir := t.TempDir()
	overlays := writeOverlayCatalog(t, dir,
		overlayFixture{ID: "OV-A", Status: compliance.OverlayStatusFinalized},
		overlayFixture{ID: "OV-B", Status: compliance.OverlayStatusFinalized},
		overlayFixture{ID: "OV-DRAFT", Status: compliance.OverlayStatusDraft})
	doctrines := writeDoctrineFile(t, filepath.Join(dir, "doctrine"), "d.json", []string{"OV-A"}, []string{"OV-B"})
	var out bytes.Buffer

	require.NoError(t, validate(overlays, doctrines, &out))

	assert.Contains(t, out.String(), "PASS: All 2 finalized COSAiS overlay(s) have detector coverage.")
}

func TestValidate_FailsListingEachUncoveredFinalizedOverlay(t *testing.T) {
	dir := t.TempDir()
	overlays := writeOverlayCatalog(t, dir,
		overlayFixture{ID: "OV-COVERED", Status: compliance.OverlayStatusFinalized},
		overlayFixture{ID: "OV-GAP-1", Status: compliance.OverlayStatusFinalized},
		overlayFixture{ID: "OV-GAP-2", Status: compliance.OverlayStatusFinalized},
		overlayFixture{ID: "OV-DRAFT-GAP", Status: compliance.OverlayStatusDraft})
	doctrines := writeDoctrineFile(t, filepath.Join(dir, "doctrine"), "d.json", []string{"OV-COVERED"})
	var out bytes.Buffer

	err := validate(overlays, doctrines, &out)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 finalized overlay(s) lack detector coverage")
	report := out.String()
	assert.Contains(t, report, "FAIL: Finalized COSAiS overlays without detector coverage:")
	assert.Contains(t, report, "  - OV-GAP-1\n")
	assert.Contains(t, report, "  - OV-GAP-2\n")
	assert.NotContains(t, report, "OV-COVERED")
	assert.NotContains(t, report, "OV-DRAFT-GAP", "draft overlays do not require coverage")
	assert.Contains(t, report, "make cosais-validate")
}

func TestValidate_TreatsCoverageFromAnyDoctrineFileAsSufficient(t *testing.T) {
	dir := t.TempDir()
	overlays := writeOverlayCatalog(t, dir, overlayFixture{ID: "OV-A", Status: compliance.OverlayStatusFinalized})
	writeDoctrineFile(t, filepath.Join(dir, "doctrine"), "first.json", nil)
	writeDoctrineFile(t, filepath.Join(dir, "doctrine"), "second.json", []string{"OV-A"})

	require.NoError(t, validate(overlays, filepath.Join(dir, "doctrine", "*.json"), &bytes.Buffer{}))
}

func TestValidate_WrapsOverlayCatalogLoadFailure(t *testing.T) {
	dir := t.TempDir()
	doctrines := writeDoctrineFile(t, filepath.Join(dir, "doctrine"), "d.json", nil)

	err := validate(filepath.Join(dir, "missing-overlays.json"), doctrines, &bytes.Buffer{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cosais coverage: load overlay catalog")
	assert.ErrorIs(t, err, constants.ErrOverlayReadFailed)
}

func TestValidate_RejectsInvalidOverlayCatalogBeforeReadingDoctrines(t *testing.T) {
	dir := t.TempDir()
	invalid := filepath.Join(dir, constants.COSAiSOverlaysFilename)
	require.NoError(t, os.WriteFile(invalid, []byte(`{"version":"1.0","source":"test","overlays":[]}`), 0o644))

	err := validate(invalid, filepath.Join(dir, "no-such-dir", "*.json"), &bytes.Buffer{})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrOverlayCatalogInvalid)
	assert.NotContains(t, err.Error(), "no doctrine files match")
}

func TestValidate_PropagatesDoctrineCollectionFailure(t *testing.T) {
	dir := t.TempDir()
	overlays := writeOverlayCatalog(t, dir, overlayFixture{ID: "OV-A", Status: compliance.OverlayStatusFinalized})

	err := validate(overlays, filepath.Join(dir, "no-such-dir", "*.json"), &bytes.Buffer{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no doctrine files match")
}

func TestValidate_CommittedCatalogAndDemoDoctrinesAgree(t *testing.T) {
	overlays := filepath.Join(repoRoot, constants.DefaultOverlayDirPath, constants.COSAiSOverlaysFilename)
	doctrines := filepath.Join(repoRoot, constants.DemosDirname, "*", constants.DemosDoctrineDir, "*.json")
	var out bytes.Buffer

	require.NoError(t, validate(overlays, doctrines, &out), "report:\n%s", out.String())

	assert.Contains(t, out.String(), "PASS")
}
