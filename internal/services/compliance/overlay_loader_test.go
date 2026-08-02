// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package compliance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

// writeOverlayFile writes a JSON file with the given content to the directory.
func writeOverlayFile(t *testing.T, dir, name string, content any) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	data, err := json.MarshalIndent(content, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o644))
}

// testOverlayCatalog returns a minimal valid OverlayCatalog for testing.
func testOverlayCatalog() *OverlayCatalog {
	return &OverlayCatalog{
		Version: "test-v1",
		Source:  "test source",
		Overlays: []Overlay{
			{
				ID:          "COSAiS-TEST-01",
				Title:       "Test Overlay",
				Description: "Test description",
				UseCase:     "test_use_case",
				ControlRefs: []string{"AC-3", "AU-2"},
				Status:      OverlayStatusDraft,
			},
			{
				ID:          "COSAiS-TEST-02",
				Title:       "Second Test Overlay",
				UseCase:     "second_use_case",
				ControlRefs: []string{"SI-4"},
				Status:      OverlayStatusDraft,
			},
		},
	}
}

// TestLoadOverlayCatalog_Success loads a valid overlay catalog from disk.
func TestLoadOverlayCatalog_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overlays.json")
	writeOverlayFile(t, dir, "overlays.json", testOverlayCatalog())

	cat, err := LoadOverlayCatalog(path)
	require.NoError(t, err)
	require.NotNil(t, cat)
	assert.Equal(t, "test-v1", cat.Version)
	assert.Equal(t, "test source", cat.Source)
	require.Len(t, cat.Overlays, 2)
	assert.Equal(t, "COSAiS-TEST-01", cat.Overlays[0].ID)
	assert.Equal(t, "COSAiS-TEST-02", cat.Overlays[1].ID)
	assert.Equal(t, OverlayStatusDraft, cat.Overlays[0].Status)
}

// TestLoadOverlayCatalog_FileNotFound returns an error for a missing file.
func TestLoadOverlayCatalog_FileNotFound(t *testing.T) {
	_, err := LoadOverlayCatalog("/nonexistent/path/overlays.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read overlay catalog")
}

// TestLoadOverlayCatalog_InvalidJSON returns an error for malformed JSON.
func TestLoadOverlayCatalog_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(path, []byte("{not valid json"), 0o644))

	_, err := LoadOverlayCatalog(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse overlay catalog")
}

// TestOverlayCatalog_Validate_EmptyVersion returns an error for empty version.
func TestOverlayCatalog_Validate_EmptyVersion(t *testing.T) {
	cat := testOverlayCatalog()
	cat.Version = ""
	err := cat.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrOverlayCatalogInvalid)
	assert.Contains(t, err.Error(), "version is empty")
}

// TestOverlayCatalog_Validate_EmptySource returns an error for empty source.
func TestOverlayCatalog_Validate_EmptySource(t *testing.T) {
	cat := testOverlayCatalog()
	cat.Source = ""
	err := cat.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrOverlayCatalogInvalid)
	assert.Contains(t, err.Error(), "source is empty")
}

// TestOverlayCatalog_Validate_NoOverlays returns an error for empty overlays.
func TestOverlayCatalog_Validate_NoOverlays(t *testing.T) {
	cat := &OverlayCatalog{Version: "v1", Source: "src", Overlays: nil}
	err := cat.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrOverlayCatalogInvalid)
	assert.Contains(t, err.Error(), "no overlays")
}

// TestOverlayCatalog_Validate_DuplicateID returns an error for duplicate IDs.
func TestOverlayCatalog_Validate_DuplicateID(t *testing.T) {
	cat := testOverlayCatalog()
	cat.Overlays[1].ID = cat.Overlays[0].ID
	err := cat.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrOverlayCatalogInvalid)
	assert.Contains(t, err.Error(), "duplicate overlay ID")
}

// TestOverlayCatalog_Validate_EmptyID returns an error for empty overlay ID.
func TestOverlayCatalog_Validate_EmptyID(t *testing.T) {
	cat := testOverlayCatalog()
	cat.Overlays[0].ID = ""
	err := cat.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrOverlayCatalogInvalid)
	assert.Contains(t, err.Error(), "empty ID")
}

// TestOverlayCatalog_Validate_EmptyTitle returns an error for empty title.
func TestOverlayCatalog_Validate_EmptyTitle(t *testing.T) {
	cat := testOverlayCatalog()
	cat.Overlays[0].Title = ""
	err := cat.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrOverlayCatalogInvalid)
	assert.Contains(t, err.Error(), "empty title")
}

// TestOverlayCatalog_Validate_EmptyUseCase returns an error for empty use_case.
func TestOverlayCatalog_Validate_EmptyUseCase(t *testing.T) {
	cat := testOverlayCatalog()
	cat.Overlays[0].UseCase = ""
	err := cat.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrOverlayCatalogInvalid)
	assert.Contains(t, err.Error(), "empty use_case")
}

// TestOverlayCatalog_Validate_EmptyStatus returns an error for empty status.
func TestOverlayCatalog_Validate_EmptyStatus(t *testing.T) {
	cat := testOverlayCatalog()
	cat.Overlays[0].Status = ""
	err := cat.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrOverlayCatalogInvalid)
	assert.Contains(t, err.Error(), "empty status")
}

// TestOverlayCatalog_FindOverlay returns the overlay for a known ID.
func TestOverlayCatalog_FindOverlay(t *testing.T) {
	cat := testOverlayCatalog()
	ov := cat.FindOverlay("COSAiS-TEST-01")
	require.NotNil(t, ov)
	assert.Equal(t, "Test Overlay", ov.Title)
}

// TestOverlayCatalog_FindOverlay_NotFound returns nil for unknown ID.
func TestOverlayCatalog_FindOverlay_NotFound(t *testing.T) {
	cat := testOverlayCatalog()
	assert.Nil(t, cat.FindOverlay("COSAiS-NONEXISTENT-99"))
}

// TestOverlayCatalog_HasOverlay returns true/false correctly.
func TestOverlayCatalog_HasOverlay(t *testing.T) {
	cat := testOverlayCatalog()
	assert.True(t, cat.HasOverlay("COSAiS-TEST-01"))
	assert.False(t, cat.HasOverlay("COSAiS-NONEXISTENT-99"))
}

// TestLoadOverlaysFromDir_EmptyDir returns an empty catalog for an empty directory.
func TestLoadOverlaysFromDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	cat, err := LoadOverlaysFromDir(dir)
	require.NoError(t, err)
	require.NotNil(t, cat)
	assert.Empty(t, cat.Overlays)
}

// TestLoadOverlaysFromDir_EmptyPath returns an empty catalog for empty path.
func TestLoadOverlaysFromDir_EmptyPath(t *testing.T) {
	cat, err := LoadOverlaysFromDir("")
	require.NoError(t, err)
	require.NotNil(t, cat)
	assert.Empty(t, cat.Overlays)
}

// TestLoadOverlaysFromDir_NonexistentDir returns an error for a missing directory.
func TestLoadOverlaysFromDir_NonexistentDir(t *testing.T) {
	_, err := LoadOverlaysFromDir("/nonexistent/overlay/dir")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read overlay dir")
}

// TestLoadOverlaysFromDir_SingleFile loads one overlay JSON file.
func TestLoadOverlaysFromDir_SingleFile(t *testing.T) {
	dir := t.TempDir()
	writeOverlayFile(t, dir, "overlays.json", testOverlayCatalog())

	cat, err := LoadOverlaysFromDir(dir)
	require.NoError(t, err)
	require.Len(t, cat.Overlays, 2)
	assert.Equal(t, "test-v1", cat.Version)
}

// TestLoadOverlaysFromDir_MultipleFiles merges overlay files in lexical order.
func TestLoadOverlaysFromDir_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	cat1 := &OverlayCatalog{
		Version: "v1",
		Source:  "source1",
		Overlays: []Overlay{
			{ID: "COSAiS-A-01", Title: "A1", UseCase: "uc_a1", Status: OverlayStatusDraft},
		},
	}
	cat2 := &OverlayCatalog{
		Version: "v1",
		Source:  "source2",
		Overlays: []Overlay{
			{ID: "COSAiS-B-01", Title: "B1", UseCase: "uc_b1", Status: OverlayStatusDraft},
		},
	}
	writeOverlayFile(t, dir, "a_overlays.json", cat1)
	writeOverlayFile(t, dir, "b_overlays.json", cat2)

	cat, err := LoadOverlaysFromDir(dir)
	require.NoError(t, err)
	require.Len(t, cat.Overlays, 2)
	assert.Equal(t, "COSAiS-A-01", cat.Overlays[0].ID)
	assert.Equal(t, "COSAiS-B-01", cat.Overlays[1].ID)
}

// TestLoadOverlaysFromDir_DuplicateAcrossFiles returns an error for duplicate IDs.
func TestLoadOverlaysFromDir_DuplicateAcrossFiles(t *testing.T) {
	dir := t.TempDir()

	cat1 := &OverlayCatalog{
		Version: "v1",
		Source:  "source1",
		Overlays: []Overlay{
			{ID: "COSAiS-DUP-01", Title: "Dup", UseCase: "uc", Status: OverlayStatusDraft},
		},
	}
	cat2 := &OverlayCatalog{
		Version: "v1",
		Source:  "source2",
		Overlays: []Overlay{
			{ID: "COSAiS-DUP-01", Title: "Dup2", UseCase: "uc2", Status: OverlayStatusDraft},
		},
	}
	writeOverlayFile(t, dir, "a.json", cat1)
	writeOverlayFile(t, dir, "b.json", cat2)

	_, err := LoadOverlaysFromDir(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate overlay ID")
}

// TestLoadOverlaysFromDir_IgnoresNonJSON ignores non-JSON files.
func TestLoadOverlaysFromDir_IgnoresNonJSON(t *testing.T) {
	dir := t.TempDir()
	writeOverlayFile(t, dir, "overlays.json", testOverlayCatalog())
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Overlays"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))

	cat, err := LoadOverlaysFromDir(dir)
	require.NoError(t, err)
	require.Len(t, cat.Overlays, 2)
}

// TestLoadOverlaysFromDir_InvalidFile returns an error for malformed JSON.
func TestLoadOverlaysFromDir_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{broken"), 0o644))

	_, err := LoadOverlaysFromDir(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse overlay file")
}

// TestValidateOverlayRefs_AllPresent returns no dangling refs when all overlay
// IDs referenced by KSIs exist in the catalog.
func TestValidateOverlayRefs_AllPresent(t *testing.T) {
	ksiCat := &KSICatalog{
		Version: "v1",
		Source:  "src",
		KSIs: []KSI{
			{ID: "KSI-CMT-01", Title: "T", Category: KSICategoryCMT, OverlayRefs: []string{"COSAiS-TEST-01"}},
		},
	}
	overlayCat := testOverlayCatalog()

	dangling := ValidateOverlayRefs(ksiCat, overlayCat)
	assert.Empty(t, dangling)
}

// TestValidateOverlayRefs_DanglingRef returns dangling refs when a KSI
// references an overlay ID not in the catalog.
func TestValidateOverlayRefs_DanglingRef(t *testing.T) {
	ksiCat := &KSICatalog{
		Version: "v1",
		Source:  "src",
		KSIs: []KSI{
			{ID: "KSI-CMT-01", Title: "T", Category: KSICategoryCMT, OverlayRefs: []string{"COSAiS-MISSING-99"}},
		},
	}
	overlayCat := testOverlayCatalog()

	dangling := ValidateOverlayRefs(ksiCat, overlayCat)
	require.Len(t, dangling, 1)
	assert.Contains(t, dangling[0], "KSI-CMT-01")
	assert.Contains(t, dangling[0], "COSAiS-MISSING-99")
}

// TestValidateOverlayRefs_NoOverlayRefs returns empty when KSIs have no overlay refs.
func TestValidateOverlayRefs_NoOverlayRefs(t *testing.T) {
	ksiCat := &KSICatalog{
		Version: "v1",
		Source:  "src",
		KSIs: []KSI{
			{ID: "KSI-CMT-01", Title: "T", Category: KSICategoryCMT},
		},
	}
	overlayCat := testOverlayCatalog()

	dangling := ValidateOverlayRefs(ksiCat, overlayCat)
	assert.Empty(t, dangling)
}

// TestOverlayJSON_RoundTrip verifies that an OverlayCatalog serializes and
// deserializes correctly via JSON.
func TestOverlayJSON_RoundTrip(t *testing.T) {
	original := testOverlayCatalog()
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded OverlayCatalog
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original.Version, decoded.Version)
	assert.Equal(t, original.Source, decoded.Source)
	require.Len(t, decoded.Overlays, len(original.Overlays))
	for i := range original.Overlays {
		assert.Equal(t, original.Overlays[i].ID, decoded.Overlays[i].ID)
		assert.Equal(t, original.Overlays[i].Title, decoded.Overlays[i].Title)
		assert.Equal(t, original.Overlays[i].UseCase, decoded.Overlays[i].UseCase)
		assert.Equal(t, original.Overlays[i].Status, decoded.Overlays[i].Status)
		assert.Equal(t, original.Overlays[i].ControlRefs, decoded.Overlays[i].ControlRefs)
	}
}
