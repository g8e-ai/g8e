// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func unavailableTestBinding() EvaluationBinding {
	return EvaluationBinding{
		ScopeID:            "scope-1",
		RunID:              "run-1",
		WindowStartUnixMs:  1_700_000_000_000,
		WindowEndUnixMs:    1_700_000_001_000,
		EvaluatorID:        constants.KSIEvaluatorID,
		EvaluatorVersion:   constants.KSIEvaluatorVersion,
		MethodDefinitionID: constants.KSIMethodDefinitionVersion,
	}
}

func unavailableTestResultSet() *KSIResultSet {
	binding := unavailableTestBinding()
	return &KSIResultSet{
		Class:         ClassC,
		EvaluatedAtMs: binding.WindowEndUnixMs,
		Binding:       binding,
		Results: []KSIResult{
			{ID: "KSI-CMT-01", Status: KSIStatusSatisfied, Outcome: KSIOutcomeSatisfied, LastValidatedUnixMs: binding.WindowEndUnixMs, Binding: binding},
			{ID: "KSI-MLA-03", Status: KSIStatusNotSatisfied, Outcome: KSIOutcomeInvalidEvidence, LastValidatedUnixMs: binding.WindowEndUnixMs, Binding: binding},
			{ID: "KSI-SVC-05", Status: KSIStatusNotApplicable, Outcome: KSIOutcomeNotApplicable, LastValidatedUnixMs: binding.WindowEndUnixMs, Binding: binding},
		},
	}
}

func TestUnavailableIntervalValidateRejectsCrossBindingFields(t *testing.T) {
	binding := unavailableTestBinding()
	valid := UnavailableInterval{
		KSIID:       "KSI-MLA-03",
		ScopeID:     binding.ScopeID,
		RunID:       binding.RunID,
		StartUnixMs: binding.WindowStartUnixMs,
		EndUnixMs:   binding.WindowEndUnixMs,
		Outcome:     KSIOutcomeInvalidEvidence,
		Status:      KSIStatusNotSatisfied,
		Binding:     binding,
	}
	tests := []struct {
		name   string
		mutate func(*UnavailableInterval)
	}{
		{name: "scope differs from binding", mutate: func(interval *UnavailableInterval) { interval.ScopeID = "scope-2" }},
		{name: "run differs from binding", mutate: func(interval *UnavailableInterval) { interval.RunID = "run-2" }},
		{name: "start precedes binding window", mutate: func(interval *UnavailableInterval) { interval.StartUnixMs-- }},
		{name: "end exceeds binding window", mutate: func(interval *UnavailableInterval) { interval.EndUnixMs++ }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := valid
			tt.mutate(&candidate)
			err := candidate.Validate()
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrKSIBindingMismatch)
		})
	}
}

func TestUnavailableIntervalStoreAppendFromResultSetPersistsOnlyFailures(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	store := NewUnavailableIntervalStore(fileSvc, filepath.Join(constants.DataDirname, constants.ComplianceDirname))
	rs := unavailableTestResultSet()

	require.NoError(t, store.AppendFromResultSet(context.Background(), rs))
	intervals, err := store.ListIntervals(context.Background())
	require.NoError(t, err)
	require.Len(t, intervals, 1)
	assert.Equal(t, "KSI-MLA-03", intervals[0].KSIID)
	assert.Equal(t, rs.Binding.WindowStartUnixMs, intervals[0].StartUnixMs)
	assert.Equal(t, rs.Binding.WindowEndUnixMs, intervals[0].EndUnixMs)
	assert.Equal(t, rs.Binding, intervals[0].Binding)
}

func TestUnavailableIntervalStoreAppendFromResultSetRejectsMismatchedResultBinding(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	store := NewUnavailableIntervalStore(fileSvc, filepath.Join(constants.DataDirname, constants.ComplianceDirname))
	rs := unavailableTestResultSet()
	rs.Results[1].Binding.RunID = "other-run"

	err := store.AppendFromResultSet(context.Background(), rs)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKSIBindingMismatch)
}

func TestUnavailableIntervalStoreFiltersAndTotalsChronologicalIntervals(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	store := NewUnavailableIntervalStore(fileSvc, filepath.Join(constants.DataDirname, constants.ComplianceDirname))
	binding := unavailableTestBinding()
	intervals := []UnavailableInterval{
		{KSIID: "KSI-MLA-03", ScopeID: binding.ScopeID, RunID: binding.RunID, StartUnixMs: binding.WindowStartUnixMs + 500, EndUnixMs: binding.WindowStartUnixMs + 900, Outcome: KSIOutcomeMethodFailure, Status: KSIStatusNotSatisfied, Binding: binding},
		{KSIID: "KSI-CMT-01", ScopeID: binding.ScopeID, RunID: binding.RunID, StartUnixMs: binding.WindowStartUnixMs + 100, EndUnixMs: binding.WindowStartUnixMs + 200, Outcome: KSIOutcomeInvalidEvidence, Status: KSIStatusNotSatisfied, Binding: binding},
		{KSIID: "KSI-MLA-03", ScopeID: binding.ScopeID, RunID: binding.RunID, StartUnixMs: binding.WindowStartUnixMs + 300, EndUnixMs: binding.WindowStartUnixMs + 400, Outcome: KSIOutcomeStaleEvidence, Status: KSIStatusNotSatisfied, Binding: binding},
	}
	for _, interval := range intervals {
		require.NoError(t, store.AppendInterval(context.Background(), interval))
	}

	filtered, err := store.GetIntervalsForKSI(context.Background(), "KSI-MLA-03")
	require.NoError(t, err)
	require.Len(t, filtered, 2)
	assert.Less(t, filtered[0].StartUnixMs, filtered[1].StartUnixMs)
	total, err := store.TotalUnavailableMs(context.Background(), "KSI-MLA-03")
	require.NoError(t, err)
	assert.Equal(t, int64(500), total)
}

func TestUnavailableIntervalStoreRejectsMalformedPersistedRecord(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	dir := filepath.Join(constants.DataDirname, constants.ComplianceDirname)
	store := NewUnavailableIntervalStore(fileSvc, dir)
	require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(dir, constants.KSIUnavailableIntervalsFilename), []byte("{"), constants.PermFilePublic))

	_, err := store.ListIntervals(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKSIUnavailableParseFailed)
}

func TestUnavailableIntervalStorePrunesExpiredIntervals(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	store := NewUnavailableIntervalStore(fileSvc, filepath.Join(constants.DataDirname, constants.ComplianceDirname))
	now := time.Now()
	binding := unavailableTestBinding()
	binding.WindowStartUnixMs = now.Add(-48 * time.Hour).UnixMilli()
	binding.WindowEndUnixMs = now.Add(-47 * time.Hour).UnixMilli()
	require.NoError(t, store.AppendInterval(context.Background(), UnavailableInterval{KSIID: "KSI-CMT-01", ScopeID: binding.ScopeID, RunID: binding.RunID, StartUnixMs: binding.WindowStartUnixMs, EndUnixMs: binding.WindowEndUnixMs, Outcome: KSIOutcomeInvalidEvidence, Status: KSIStatusNotSatisfied, Binding: binding}))
	binding.WindowStartUnixMs = now.Add(-time.Hour).UnixMilli()
	binding.WindowEndUnixMs = now.UnixMilli()
	require.NoError(t, store.AppendInterval(context.Background(), UnavailableInterval{KSIID: "KSI-MLA-03", ScopeID: binding.ScopeID, RunID: binding.RunID, StartUnixMs: binding.WindowStartUnixMs, EndUnixMs: binding.WindowEndUnixMs, Outcome: KSIOutcomeMethodFailure, Status: KSIStatusNotSatisfied, Binding: binding}))

	removed, err := store.PruneOlderThan(context.Background(), 24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, removed)
	remaining, err := store.ListIntervals(context.Background())
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "KSI-MLA-03", remaining[0].KSIID)
}

func TestUnavailableIntervalStoreListMissingFileIsEmpty(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	store := NewUnavailableIntervalStore(fileSvc, filepath.Join(constants.DataDirname, constants.ComplianceDirname))

	intervals, err := store.ListIntervals(context.Background())
	require.NoError(t, err)
	assert.Empty(t, intervals)
}

func TestUnavailableIntervalStoreAppendFromResultSetRejectsNil(t *testing.T) {
	fileSvc := setupHistoryTestFS(t)
	store := NewUnavailableIntervalStore(fileSvc, filepath.Join(constants.DataDirname, constants.ComplianceDirname))

	err := store.AppendFromResultSet(context.Background(), nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrKSIUnavailableWriteFailed))
}
