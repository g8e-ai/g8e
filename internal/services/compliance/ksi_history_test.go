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
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// setupHistoryTestFS creates a RuntimeFileService backed by a temp directory.
func setupHistoryTestFS(t *testing.T) fs.RuntimeFileService {
	t.Helper()
	baseDir := testutil.TempDir(t)
	require.NoError(t, paths.InitWithBase(baseDir))
	svc, err := fs.NewRuntimeFileService(baseDir, testutil.NewVerboseTestLogger(t))
	require.NoError(t, err)
	return svc
}

// TestKSIHistoryStore_SaveAndListRoundTrip verifies that saving a snapshot
// and listing it back produces the same data.
func TestKSIHistoryStore_SaveAndListRoundTrip(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	ctx := context.Background()
	historyDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.KSIHistoryDirname)
	store := NewKSIHistoryStore(fileSvc, historyDir)

	now := time.Now().UnixMilli()
	original := &KSIResultSet{
		Class:         ClassC,
		EvaluatedAtMs: now,
		Results: []KSIResult{
			{
				ID:                  "KSI-CMT-01",
				Status:              KSIStatusSatisfied,
				LastValidatedUnixMs: now,
				MethodCount:         2,
				Evidence: []Evidence{
					{Type: EvidenceTypeLedgerCommit, Reference: "commit-abc", Description: "Ledger commit exists"},
				},
			},
			{
				ID:                  "KSI-SVC-05",
				Status:              KSIStatusNotSatisfied,
				LastValidatedUnixMs: now,
				MethodCount:         2,
			},
		},
	}

	require.NoError(t, store.SaveSnapshot(ctx, original))

	snapshots, err := store.ListSnapshots(ctx)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)

	snap := snapshots[0]
	assert.Equal(t, original.Class, snap.Class)
	assert.Equal(t, original.EvaluatedAtMs, snap.EvaluatedAtMs)
	require.Len(t, snap.Results, 2)
	assert.Equal(t, original.Results[0].ID, snap.Results[0].ID)
	assert.Equal(t, original.Results[0].Status, snap.Results[0].Status)
	assert.Equal(t, original.Results[0].MethodCount, snap.Results[0].MethodCount)
	require.Len(t, snap.Results[0].Evidence, 1)
	assert.Equal(t, original.Results[0].Evidence[0].Type, snap.Results[0].Evidence[0].Type)
	assert.Equal(t, original.Results[0].Evidence[0].Reference, snap.Results[0].Evidence[0].Reference)
	assert.Equal(t, original.Results[1].ID, snap.Results[1].ID)
	assert.Equal(t, original.Results[1].Status, snap.Results[1].Status)
}

// TestKSIHistoryStore_MultipleSnapshotsSorted verifies that multiple snapshots
// are returned in chronological order (oldest first).
func TestKSIHistoryStore_MultipleSnapshotsSorted(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	ctx := context.Background()
	historyDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.KSIHistoryDirname)
	store := NewKSIHistoryStore(fileSvc, historyDir)

	baseMs := time.Now().UnixMilli()
	snapshots := []*KSIResultSet{
		{Class: ClassC, EvaluatedAtMs: baseMs + 2000, Results: []KSIResult{{ID: "KSI-CMT-01", Status: KSIStatusSatisfied, MethodCount: 2}}},
		{Class: ClassC, EvaluatedAtMs: baseMs, Results: []KSIResult{{ID: "KSI-CMT-01", Status: KSIStatusNotSatisfied, MethodCount: 2}}},
		{Class: ClassC, EvaluatedAtMs: baseMs + 1000, Results: []KSIResult{{ID: "KSI-CMT-01", Status: KSIStatusSatisfied, MethodCount: 2}}},
	}

	for _, rs := range snapshots {
		require.NoError(t, store.SaveSnapshot(ctx, rs))
	}

	got, err := store.ListSnapshots(ctx)
	require.NoError(t, err)
	require.Len(t, got, 3)

	assert.Equal(t, baseMs, got[0].EvaluatedAtMs)
	assert.Equal(t, baseMs+1000, got[1].EvaluatedAtMs)
	assert.Equal(t, baseMs+2000, got[2].EvaluatedAtMs)
}

// TestKSIHistoryStore_GetHistoryForKSI verifies filtering by KSI ID across
// multiple snapshots.
func TestKSIHistoryStore_GetHistoryForKSI(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	ctx := context.Background()
	historyDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.KSIHistoryDirname)
	store := NewKSIHistoryStore(fileSvc, historyDir)

	baseMs := time.Now().UnixMilli()
	require.NoError(t, store.SaveSnapshot(ctx, &KSIResultSet{
		Class:         ClassC,
		EvaluatedAtMs: baseMs,
		Results: []KSIResult{
			{ID: "KSI-CMT-01", Status: KSIStatusNotSatisfied, MethodCount: 2},
			{ID: "KSI-MLA-03", Status: KSIStatusSatisfied, MethodCount: 2},
		},
	}))
	require.NoError(t, store.SaveSnapshot(ctx, &KSIResultSet{
		Class:         ClassC,
		EvaluatedAtMs: baseMs + 1000,
		Results: []KSIResult{
			{ID: "KSI-CMT-01", Status: KSIStatusSatisfied, MethodCount: 2},
			{ID: "KSI-MLA-03", Status: KSIStatusSatisfied, MethodCount: 2},
		},
	}))

	results, err := store.GetHistoryForKSI(ctx, "KSI-CMT-01")
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, KSIStatusNotSatisfied, results[0].Status)
	assert.Equal(t, KSIStatusSatisfied, results[1].Status)
}

// TestKSIHistoryStore_GetHistoryForKSI_NotFound returns ErrKSIHistoryEmpty
// when no snapshots contain the requested KSI ID.
func TestKSIHistoryStore_GetHistoryForKSI_NotFound(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	ctx := context.Background()
	historyDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.KSIHistoryDirname)
	store := NewKSIHistoryStore(fileSvc, historyDir)

	require.NoError(t, store.SaveSnapshot(ctx, &KSIResultSet{
		Class:         ClassC,
		EvaluatedAtMs: time.Now().UnixMilli(),
		Results:       []KSIResult{{ID: "KSI-CMT-01", Status: KSIStatusSatisfied, MethodCount: 2}},
	}))

	_, err := store.GetHistoryForKSI(ctx, "KSI-NONEXISTENT-99")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKSIHistoryEmpty)
}

// TestKSIHistoryStore_GetHistoryForKSI_NoSnapshots returns ErrKSIHistoryEmpty
// when the history directory has no snapshots at all.
func TestKSIHistoryStore_GetHistoryForKSI_NoSnapshots(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	ctx := context.Background()
	historyDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.KSIHistoryDirname)
	store := NewKSIHistoryStore(fileSvc, historyDir)

	_, err := store.GetHistoryForKSI(ctx, "KSI-CMT-01")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKSIHistoryEmpty)
}

// TestKSIHistoryStore_ListSnapshots_EmptyDirectory returns an empty slice
// when the history directory does not exist.
func TestKSIHistoryStore_ListSnapshots_EmptyDirectory(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	ctx := context.Background()
	historyDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.KSIHistoryDirname)
	store := NewKSIHistoryStore(fileSvc, historyDir)

	snapshots, err := store.ListSnapshots(ctx)
	require.NoError(t, err)
	assert.Empty(t, snapshots)
}

// TestKSIHistoryStore_SaveSnapshot_NilResultSet returns ErrKSIHistoryWriteFailed.
func TestKSIHistoryStore_SaveSnapshot_NilResultSet(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	ctx := context.Background()
	historyDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.KSIHistoryDirname)
	store := NewKSIHistoryStore(fileSvc, historyDir)

	err := store.SaveSnapshot(ctx, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKSIHistoryWriteFailed)
}

// TestKSIHistoryStore_PruneOlderThan verifies that snapshots older than the
// retention period are removed and newer ones are kept.
func TestKSIHistoryStore_PruneOlderThan(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	ctx := context.Background()
	historyDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.KSIHistoryDirname)
	store := NewKSIHistoryStore(fileSvc, historyDir)

	now := time.Now()

	// Save a recent snapshot (within retention).
	recentMs := now.UnixMilli()
	require.NoError(t, store.SaveSnapshot(ctx, &KSIResultSet{
		Class:         ClassC,
		EvaluatedAtMs: recentMs,
		Results:       []KSIResult{{ID: "KSI-CMT-01", Status: KSIStatusSatisfied, MethodCount: 2}},
	}))

	// Manually write an old snapshot file with a backdated modification time.
	oldMs := now.Add(-100 * 24 * time.Hour).UnixMilli()
	oldFilename := snapshotFilename(oldMs)
	oldRelPath := filepath.Join(historyDir, oldFilename)
	oldData := []byte(`{"class":"C","evaluated_at_ms":` + strconv.FormatInt(oldMs, 10) + `,"results":[]}`)
	require.NoError(t, fileSvc.MkdirAll(ctx, historyDir, constants.PermDirStandard))
	require.NoError(t, fileSvc.WriteFile(ctx, oldRelPath, oldData, constants.PermFilePublic))

	// Backdate the file's modification time.
	absPath := fileSvc.Resolve(oldRelPath)
	require.NoError(t, os.Chtimes(absPath, now.Add(-100*24*time.Hour), now.Add(-100*24*time.Hour)))

	// Prune with 90-day retention.
	removed, err := store.PruneOlderThan(ctx, 90*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)

	// Verify only the recent snapshot remains.
	snapshots, err := store.ListSnapshots(ctx)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	assert.Equal(t, recentMs, snapshots[0].EvaluatedAtMs)
}

// TestKSIHistoryStore_PruneOlderThan_ZeroRetention prunes nothing.
func TestKSIHistoryStore_PruneOlderThan_ZeroRetention(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	ctx := context.Background()
	historyDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.KSIHistoryDirname)
	store := NewKSIHistoryStore(fileSvc, historyDir)

	require.NoError(t, store.SaveSnapshot(ctx, &KSIResultSet{
		Class:         ClassC,
		EvaluatedAtMs: time.Now().UnixMilli(),
		Results:       []KSIResult{{ID: "KSI-CMT-01", Status: KSIStatusSatisfied, MethodCount: 2}},
	}))

	removed, err := store.PruneOlderThan(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, removed)

	snapshots, err := store.ListSnapshots(ctx)
	require.NoError(t, err)
	assert.Len(t, snapshots, 1)
}

// TestKSIHistoryStore_ListSnapshots_ReadDirError verifies that non-ErrNotFound
// ReadDir errors are wrapped with ErrKSIHistoryReadFailed instead of being
// silently swallowed.
func TestKSIHistoryStore_ListSnapshots_ReadDirError(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	ctx := context.Background()
	historyDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.KSIHistoryDirname)
	store := NewKSIHistoryStore(fileSvc, historyDir)

	// Create a regular file at the history dir path so ReadDir fails with
	// ENOTDIR (wrapped as ErrDirectoryRead, not ErrNotFound).
	absPath := fileSvc.Resolve(historyDir)
	require.NoError(t, os.MkdirAll(filepath.Dir(absPath), constants.PermDirStandard))
	require.NoError(t, os.WriteFile(absPath, []byte("not a directory"), constants.PermFilePublic))

	_, err := store.ListSnapshots(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKSIHistoryReadFailed)
}

// TestKSIHistoryStore_PruneOlderThan_ReadDirError verifies that non-ErrNotFound
// ReadDir errors are wrapped with ErrKSIHistoryReadFailed instead of being
// silently swallowed.
func TestKSIHistoryStore_PruneOlderThan_ReadDirError(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	ctx := context.Background()
	historyDir := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.KSIHistoryDirname)
	store := NewKSIHistoryStore(fileSvc, historyDir)

	// Create a regular file at the history dir path so ReadDir fails with
	// ENOTDIR (wrapped as ErrDirectoryRead, not ErrNotFound).
	absPath := fileSvc.Resolve(historyDir)
	require.NoError(t, os.MkdirAll(filepath.Dir(absPath), constants.PermDirStandard))
	require.NoError(t, os.WriteFile(absPath, []byte("not a directory"), constants.PermFilePublic))

	_, err := store.PruneOlderThan(ctx, 24*time.Hour)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKSIHistoryReadFailed)
}

// TestSnapshotFilename verifies the filename format.
func TestSnapshotFilename(t *testing.T) {
	name := snapshotFilename(1700000000000)
	assert.Equal(t, "ksi-result-1700000000000.json", name)
}
