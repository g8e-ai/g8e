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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

// TestKSICatalog_LoadFromDisk loads the shipped KSI catalog and validates its structure.
func TestKSICatalog_LoadFromDisk(t *testing.T) {
	catalogPath := filepath.Join("..", "..", "..", "docs", "reference", "ksi-catalog.json")

	catalog, err := LoadKSICatalog(catalogPath)
	require.NoError(t, err)

	assert.Equal(t, "CR26-2026-06-24", catalog.Version)
	assert.NotEmpty(t, catalog.Source)
	assert.NotEmpty(t, catalog.KSIs)

	// Verify all KSIs have required fields
	seen := make(map[string]bool)
	for _, ksi := range catalog.KSIs {
		assert.NotEmpty(t, ksi.ID, "KSI ID should not be empty")
		assert.NotEmpty(t, ksi.Title, "KSI %s title should not be empty", ksi.ID)
		assert.NotEmpty(t, ksi.Category, "KSI %s category should not be empty", ksi.ID)
		assert.NotEmpty(t, ksi.ControlRefs, "KSI %s should have control refs", ksi.ID)
		assert.NotEmpty(t, ksi.ApplicableClasses, "KSI %s should have applicable classes", ksi.ID)
		assert.NotEmpty(t, ksi.ValidationCycle, "KSI %s should have validation cycle", ksi.ID)

		assert.False(t, seen[ksi.ID], "duplicate KSI ID: %s", ksi.ID)
		seen[ksi.ID] = true
	}

	// Verify key KSIs are present
	expectedIDs := []string{
		"KSI-CMT-01", "KSI-CMT-03", "KSI-CMT-04",
		"KSI-MLA-03", "KSI-MLA-07", "KSI-MLA-08",
		"KSI-SVC-04", "KSI-SVC-05", "KSI-SVC-08", "KSI-SVC-09",
		"KSI-IAM-05", "KSI-IAM-07",
		"KSI-CNA-01", "KSI-CNA-05", "KSI-CNA-08",
	}
	for _, id := range expectedIDs {
		assert.True(t, seen[id], "expected KSI %s in catalog", id)
	}
}

// TestKSICatalog_Validate detects structural problems in a catalog.
func TestKSICatalog_Validate(t *testing.T) {
	tests := []struct {
		name        string
		catalog     *KSICatalog
		wantErr     bool
		errContains string
	}{
		{
			name: "valid catalog",
			catalog: &KSICatalog{
				Version: "1.0",
				Source:  "test",
				KSIs: []KSI{
					{ID: "KSI-TEST-01", Title: "Test", Category: KSICategoryCMT, ControlRefs: []string{"CM-3"}, ApplicableClasses: []CertificationClass{ClassC}, ValidationCycle: ValidationCycleMachine},
				},
			},
			wantErr: false,
		},
		{
			name: "empty version",
			catalog: &KSICatalog{
				Source: "test",
				KSIs:   []KSI{{ID: "KSI-TEST-01", Title: "Test", Category: KSICategoryCMT}},
			},
			wantErr:     true,
			errContains: "version",
		},
		{
			name: "empty source",
			catalog: &KSICatalog{
				Version: "1.0",
				KSIs:    []KSI{{ID: "KSI-TEST-01", Title: "Test", Category: KSICategoryCMT}},
			},
			wantErr:     true,
			errContains: "source",
		},
		{
			name: "no KSIs",
			catalog: &KSICatalog{
				Version: "1.0",
				Source:  "test",
				KSIs:    []KSI{},
			},
			wantErr:     true,
			errContains: "no KSIs",
		},
		{
			name: "duplicate KSI ID",
			catalog: &KSICatalog{
				Version: "1.0",
				Source:  "test",
				KSIs: []KSI{
					{ID: "KSI-TEST-01", Title: "Test 1", Category: KSICategoryCMT},
					{ID: "KSI-TEST-01", Title: "Test 2", Category: KSICategoryCMT},
				},
			},
			wantErr:     true,
			errContains: "duplicate",
		},
		{
			name: "empty KSI ID",
			catalog: &KSICatalog{
				Version: "1.0",
				Source:  "test",
				KSIs:    []KSI{{ID: "", Title: "Test", Category: KSICategoryCMT}},
			},
			wantErr:     true,
			errContains: "empty ID",
		},
		{
			name: "empty KSI title",
			catalog: &KSICatalog{
				Version: "1.0",
				Source:  "test",
				KSIs:    []KSI{{ID: "KSI-TEST-01", Title: "", Category: KSICategoryCMT}},
			},
			wantErr:     true,
			errContains: "empty title",
		},
		{
			name: "empty KSI category",
			catalog: &KSICatalog{
				Version: "1.0",
				Source:  "test",
				KSIs:    []KSI{{ID: "KSI-TEST-01", Title: "Test", Category: ""}},
			},
			wantErr:     true,
			errContains: "empty category",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.catalog.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, constants.ErrKSICatalogInvalid)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestKSICatalog_FindKSI finds a KSI by ID.
func TestKSICatalog_FindKSI(t *testing.T) {
	catalog := &KSICatalog{
		Version: "1.0",
		Source:  "test",
		KSIs: []KSI{
			{ID: "KSI-CMT-01", Title: "Logging Changes", Category: KSICategoryCMT},
			{ID: "KSI-SVC-05", Title: "Validating Resource Integrity", Category: KSICategorySVC},
		},
	}

	ksi := catalog.FindKSI("KSI-CMT-01")
	require.NotNil(t, ksi)
	assert.Equal(t, "Logging Changes", ksi.Title)

	assert.Nil(t, catalog.FindKSI("KSI-NONEXISTENT-99"))
}

// TestKSICatalog_KSIsForClass filters KSIs by certification class.
func TestKSICatalog_KSIsForClass(t *testing.T) {
	catalog := &KSICatalog{
		Version: "1.0",
		Source:  "test",
		KSIs: []KSI{
			{ID: "KSI-CMT-01", Title: "A", Category: KSICategoryCMT, ApplicableClasses: []CertificationClass{ClassB, ClassC}},
			{ID: "KSI-CNA-08", Title: "B", Category: KSICategoryCNA, ApplicableClasses: []CertificationClass{ClassC}},
			{ID: "KSI-CED-01", Title: "C", Category: KSICategoryCED, ApplicableClasses: []CertificationClass{ClassB}},
		},
	}

	classC := catalog.KSIsForClass(ClassC)
	assert.Len(t, classC, 2)
	assert.Equal(t, "KSI-CMT-01", classC[0].ID)
	assert.Equal(t, "KSI-CNA-08", classC[1].ID)

	classB := catalog.KSIsForClass(ClassB)
	assert.Len(t, classB, 2)

	classD := catalog.KSIsForClass(ClassD)
	assert.Empty(t, classD)
}

// TestMinimumMethodsForClass returns correct minimum method counts.
func TestMinimumMethodsForClass(t *testing.T) {
	assert.Equal(t, 0, MinimumMethodsForClass(ClassA))
	assert.Equal(t, 1, MinimumMethodsForClass(ClassB))
	assert.Equal(t, 2, MinimumMethodsForClass(ClassC))
	assert.Equal(t, 4, MinimumMethodsForClass(ClassD))
}

// TestKSI_IsStale detects stale KSIs based on validation cycle.
func TestKSI_IsStale(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		ksi      KSI
		now      time.Time
		expected bool
	}{
		{
			name:     "never validated",
			ksi:      KSI{ValidationCycle: ValidationCycleMachine, LastValidatedUnixMs: 0},
			now:      now,
			expected: true,
		},
		{
			name:     "recently validated within 7d cycle",
			ksi:      KSI{ValidationCycle: ValidationCycleMachine, LastValidatedUnixMs: now.Add(-3 * 24 * time.Hour).UnixMilli()},
			now:      now,
			expected: false,
		},
		{
			name:     "validated beyond 7d cycle",
			ksi:      KSI{ValidationCycle: ValidationCycleMachine, LastValidatedUnixMs: now.Add(-8 * 24 * time.Hour).UnixMilli()},
			now:      now,
			expected: true,
		},
		{
			name:     "recently validated within 90d cycle",
			ksi:      KSI{ValidationCycle: ValidationCycleNonMachine, LastValidatedUnixMs: now.Add(-30 * 24 * time.Hour).UnixMilli()},
			now:      now,
			expected: false,
		},
		{
			name:     "validated beyond 90d cycle",
			ksi:      KSI{ValidationCycle: ValidationCycleNonMachine, LastValidatedUnixMs: now.Add(-91 * 24 * time.Hour).UnixMilli()},
			now:      now,
			expected: true,
		},
		{
			name:     "unknown validation cycle",
			ksi:      KSI{ValidationCycle: "unknown", LastValidatedUnixMs: now.Add(-1 * time.Hour).UnixMilli()},
			now:      now,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.ksi.IsStale(tt.now))
		})
	}
}

// TestKSI_JSONRoundTrip verifies KSI struct serialization round-trips through JSON.
func TestKSI_JSONRoundTrip(t *testing.T) {
	original := KSI{
		ID:                  "KSI-SVC-05",
		Title:               "Validating Resource Integrity",
		Category:            KSICategorySVC,
		Description:         "Cryptographic methods validate integrity of machine-based information resources.",
		ControlRefs:         []string{"CM-2", "CM-8", "SC-13", "SC-23", "SI-7", "SR-10"},
		OverlayRefs:         []string{"COSAiS-LLM-001"},
		ApplicableClasses:   []CertificationClass{ClassB, ClassC},
		ValidationCycle:     ValidationCycleMachine,
		Status:              KSIStatusSatisfied,
		AutomatedMethods:    []AutomatedMethod{{Name: "merkle_root_check", Description: "Verifies ledger Merkle root"}},
		Evidence:            []Evidence{{Type: EvidenceTypeMerkleRoot, Reference: "abc123", Description: "Current Merkle root"}},
		LastValidatedUnixMs: time.Now().UnixMilli(),
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded KSI
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original.ID, decoded.ID)
	assert.Equal(t, original.Title, decoded.Title)
	assert.Equal(t, original.Category, decoded.Category)
	assert.Equal(t, original.Description, decoded.Description)
	assert.Equal(t, original.ControlRefs, decoded.ControlRefs)
	assert.Equal(t, original.OverlayRefs, decoded.OverlayRefs)
	assert.Equal(t, original.ApplicableClasses, decoded.ApplicableClasses)
	assert.Equal(t, original.ValidationCycle, decoded.ValidationCycle)
	assert.Equal(t, original.Status, decoded.Status)
	assert.Equal(t, original.AutomatedMethods, decoded.AutomatedMethods)
	assert.Equal(t, original.Evidence, decoded.Evidence)
	assert.Equal(t, original.LastValidatedUnixMs, decoded.LastValidatedUnixMs)
}

// TestKSIResultSet_JSONRoundTrip verifies KSIResultSet serialization round-trips through JSON.
func TestKSIResultSet_JSONRoundTrip(t *testing.T) {
	original := KSIResultSet{
		Class:         ClassC,
		EvaluatedAtMs: time.Now().UnixMilli(),
		Results: []KSIResult{
			{
				ID:                  "KSI-CMT-01",
				Status:              KSIStatusSatisfied,
				Evidence:            []Evidence{{Type: EvidenceTypeLedgerCommit, Reference: "commit-abc"}},
				LastValidatedUnixMs: time.Now().UnixMilli(),
				MethodCount:         2,
			},
			{
				ID:     "KSI-SVC-08",
				Status: KSIStatusNotSatisfied,
			},
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded KSIResultSet
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original.Class, decoded.Class)
	assert.Equal(t, original.EvaluatedAtMs, decoded.EvaluatedAtMs)
	assert.Len(t, decoded.Results, 2)
	assert.Equal(t, original.Results[0].ID, decoded.Results[0].ID)
	assert.Equal(t, original.Results[0].Status, decoded.Results[0].Status)
	assert.Equal(t, original.Results[0].MethodCount, decoded.Results[0].MethodCount)
	assert.Equal(t, original.Results[1].Status, decoded.Results[1].Status)
}

// TestLoadKSICatalog_FileNotFound returns a wrapped error for missing files.
func TestLoadKSICatalog_FileNotFound(t *testing.T) {
	_, err := LoadKSICatalog("/nonexistent/path/to/ksi-catalog.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read KSI catalog")
}

// TestLoadKSICatalog_InvalidJSON returns a parse error for malformed JSON.
func TestLoadKSICatalog_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bad.json")
	require.NoError(t, os.WriteFile(path, []byte("{not valid json"), 0644))

	_, err := LoadKSICatalog(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse KSI catalog")
}

// TestLoadKSICatalog_ValidationFailure returns a validation error for structurally invalid catalogs.
func TestLoadKSICatalog_ValidationFailure(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "invalid.json")
	// Valid JSON but missing required fields (no version)
	catalog := KSICatalog{Source: "test", KSIs: []KSI{{ID: "KSI-X-01", Title: "X", Category: KSICategoryCMT}}}
	data, err := json.Marshal(catalog)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))

	_, err = LoadKSICatalog(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validate KSI catalog")
}
