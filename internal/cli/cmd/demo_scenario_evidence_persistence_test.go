// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// Phase 0 regression markers for the proof-backed compliance reporting plan
// (v2.1.3). These mirror the markers in the compliance package.
const (
	demoPhase0RegressionAfterFix = "PHASE0: AFTER FIX"
	demoPhase0RegressionIssue    = "PHASE0: ISSUE: v2.1.3 demo evidence not persisted as typed evidence"
)

// TestPhase0Demo_ScenarioResultUsesProtocolOwnedRecord verifies that the
// private string-only scenario result is replaced by the protocol-owned typed
// result for the first evidence-grade FedRAMP scenario.
func TestPhase0Demo_ScenarioResultUsesProtocolOwnedRecord(t *testing.T) {
	startedAt := time.Date(2026, time.September, 1, 12, 30, 0, 0, time.UTC)

	result := newFedRAMPDenyScenarioResult(startedAt)

	assert.IsType(t, &compliancev1.DemoScenarioResult{}, result, demoPhase0RegressionAfterFix)
	assert.Equal(t, "fedramp-deny", result.GetScenarioRef().GetId(), demoPhase0RegressionIssue)
	assert.Equal(t, "1.0.0", result.GetScenarioRef().GetVersion(), demoPhase0RegressionIssue)
	assert.Equal(t, constants.DemosOrgFedRAMP, result.GetDemoId(), demoPhase0RegressionIssue)
	assert.Equal(t, "fedramp-demo-scope", result.GetScopeId(), demoPhase0RegressionIssue)
	assert.NotEmpty(t, result.GetRunId(), demoPhase0RegressionIssue)
	assert.Equal(t, "2", result.GetDisplayNumber(), demoPhase0RegressionAfterFix)
}

// TestPhase0Demo_EvidenceGradeScenarioResultPersistsCanonicalEvidence verifies
// that typed scenario results accumulate as canonical protojson under the
// runtime compliance evidence tree.
func TestPhase0Demo_EvidenceGradeScenarioResultPersistsCanonicalEvidence(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	ctx := context.Background()
	startedAt := time.Date(2026, time.September, 1, 12, 30, 0, 0, time.UTC)
	first := newFedRAMPDenyScenarioResult(startedAt)
	second := newFedRAMPDenyScenarioResult(startedAt)
	second.ResultId += ":second"

	require.NoError(t, persistDemoScenarioResult(ctx, fileSvc, first), demoPhase0RegressionIssue)
	require.NoError(t, persistDemoScenarioResult(ctx, fileSvc, second), demoPhase0RegressionIssue)

	relPath := filepath.Join(constants.DataDirname, constants.ComplianceDirname, constants.DemoEvidenceDirname, first.GetRunId(), constants.DemoRunResultsFilename)
	exists, err := fileSvc.FileExists(ctx, relPath)
	require.NoError(t, err)
	assert.True(t, exists, demoPhase0RegressionAfterFix)

	encodedFirst, err := compliancev1.MarshalCanonical(first)
	require.NoError(t, err)
	encodedSecond, err := compliancev1.MarshalCanonical(second)
	require.NoError(t, err)
	persisted, err := fileSvc.ReadFile(ctx, relPath)
	require.NoError(t, err)
	assert.Equal(t, string(encodedFirst)+"\n"+string(encodedSecond), string(persisted), demoPhase0RegressionAfterFix)
}

// TestPhase0Demo_ScenarioResultCarriesAssertionAndFrameworkReferences verifies
// that the evidence-grade result binds its assertion and framework-control
// references instead of relying on terminal text or scattered mappings.
func TestPhase0Demo_ScenarioResultCarriesAssertionAndFrameworkReferences(t *testing.T) {
	result := newFedRAMPDenyScenarioResult(time.Date(2026, time.September, 1, 12, 30, 0, 0, time.UTC))

	assert.Equal(t, fedrampAssertionRefs, result.GetAssertionRefs(), demoPhase0RegressionAfterFix)
	assert.Equal(t, fedrampFrameworkControlRefs, result.GetFrameworkControlRefs(), demoPhase0RegressionAfterFix)
	assert.NotEmpty(t, result.GetScenarioRef(), demoPhase0RegressionIssue)
	assert.NotEmpty(t, result.GetScopeId(), demoPhase0RegressionIssue)
}
