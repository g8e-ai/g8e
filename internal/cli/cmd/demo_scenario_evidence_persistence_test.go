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
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancecatalog "github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// Phase 0 regression markers for the proof-backed compliance reporting plan
// (v2.1.3). These mirror the markers in the compliance package.
const (
	demoPhase0RegressionAfterFix = "PHASE0: AFTER FIX"
	demoPhase0RegressionIssue    = "PHASE0: ISSUE: v2.1.3 demo evidence not persisted as typed evidence"
)

func newFedRAMPDenyScenarioResultForTest(t *testing.T, startedAt time.Time) *compliancev1.DemoScenarioResult {
	t.Helper()
	definition, err := loadFedRAMPScenarioDefinition("fedramp-deny")
	require.NoError(t, err)
	return newFedRAMPDenyScenarioResult(startedAt, definition)
}

// TestPhase0Demo_ScenarioResultUsesProtocolOwnedRecord verifies that the
// private string-only scenario result is replaced by the protocol-owned typed
// result for the first evidence-grade FedRAMP scenario.
func TestPhase0Demo_ScenarioResultUsesProtocolOwnedRecord(t *testing.T) {
	startedAt := time.Date(2026, time.September, 1, 12, 30, 0, 0, time.UTC)

	result := newFedRAMPDenyScenarioResultForTest(t, startedAt)

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
	first := newFedRAMPDenyScenarioResultForTest(t, startedAt)
	second := newFedRAMPDenyScenarioResultForTest(t, startedAt)
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
	result := newFedRAMPDenyScenarioResultForTest(t, time.Date(2026, time.September, 1, 12, 30, 0, 0, time.UTC))

	assert.NotEmpty(t, result.GetAssertionRefs(), demoPhase0RegressionAfterFix)
	assert.NotEmpty(t, result.GetFrameworkControlRefs(), demoPhase0RegressionAfterFix)
	assert.NotEmpty(t, result.GetScenarioRef(), demoPhase0RegressionIssue)
	assert.NotEmpty(t, result.GetScopeId(), demoPhase0RegressionIssue)
}

func TestFedRAMPDenyScenarioResultUsesCanonicalDefinition(t *testing.T) {
	assertions, frameworks, _, err := compliancecatalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	scenarios, err := compliancecatalog.LoadDemoScenarioCatalog(assertions, frameworks)
	require.NoError(t, err)
	definition := compliancecatalog.FindDemoScenarioDefinition(scenarios, "fedramp-deny", "1.0.0")
	require.NotNil(t, definition)

	result := newFedRAMPDenyScenarioResult(time.Date(2026, time.September, 1, 12, 30, 0, 0, time.UTC), definition)

	assert.Equal(t, definition.GetDisplayNumber(), result.GetDisplayNumber())
	assert.Equal(t, definition.GetTitle(), result.GetTitle())
	require.Len(t, result.GetAssertionRefs(), len(definition.GetAssertionRefs()))
	for i := range definition.GetAssertionRefs() {
		assert.True(t, proto.Equal(definition.GetAssertionRefs()[i], result.GetAssertionRefs()[i]))
	}
	require.Len(t, result.GetFrameworkControlRefs(), len(definition.GetFrameworkControlRefs()))
	for i := range definition.GetFrameworkControlRefs() {
		assert.True(t, proto.Equal(definition.GetFrameworkControlRefs()[i], result.GetFrameworkControlRefs()[i]))
	}
}

func TestFedRAMPScenarioReferencesResolveCanonicalDefinitions(t *testing.T) {
	tests := []struct {
		scenario string
		id       string
		title    string
	}{
		{scenario: "1", id: "fedramp-provision", title: "Governed Cloud Resource Provisioning"},
		{scenario: "2", id: "fedramp-deny", title: "Unauthorized Audit Trail Destruction Blocked"},
		{scenario: "3", id: "fedramp-revert", title: "Governed Configuration Revert"},
		{scenario: "4", id: "fedramp-evidence-block", title: "Gateway Audit Vault Destruction Blocked"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			id, err := fedRAMPScenarioID(tt.scenario)
			require.NoError(t, err)
			assert.Equal(t, tt.id, id)
			definition, err := loadFedRAMPScenarioDefinition(id)
			require.NoError(t, err)
			assert.Equal(t, tt.scenario, definition.GetDisplayNumber())
			assert.Equal(t, tt.title, definition.GetTitle())
		})
	}
}

func TestFedRAMPDenyHarnessVerified_InterpretsHarnessExitStatus(t *testing.T) {
	tests := []struct {
		name       string
		harnessErr error
		verified   bool
	}{
		{name: "successful harness verification is verified", verified: true},
		{name: "failed harness verification is not verified", harnessErr: constants.ErrDemoScenarioFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.verified, fedRAMPDenyHarnessVerified(tt.harnessErr))
		})
	}
}

func TestNewFedRAMPScenarioResult_UsesCanonicalProvisionDefinition(t *testing.T) {
	definition, err := loadFedRAMPScenarioDefinition("fedramp-provision")
	require.NoError(t, err)
	startedAt := time.Date(2026, time.September, 1, 12, 30, 0, 0, time.UTC)

	result := newFedRAMPScenarioResult(startedAt, definition, "provision metrics")

	assert.Equal(t, "fedramp-provision", result.GetScenarioRef().GetId())
	assert.Equal(t, definition.GetScenarioVersion(), result.GetScenarioRef().GetVersion())
	assert.Equal(t, definition.GetDisplayNumber(), result.GetDisplayNumber())
	assert.Equal(t, definition.GetTitle(), result.GetTitle())
	assert.Equal(t, "provision metrics", result.GetMetricsSummary())
	require.Len(t, result.GetAssertionRefs(), len(definition.GetAssertionRefs()))
	for i := range definition.GetAssertionRefs() {
		assert.True(t, proto.Equal(definition.GetAssertionRefs()[i], result.GetAssertionRefs()[i]))
	}
	require.Len(t, result.GetFrameworkControlRefs(), len(definition.GetFrameworkControlRefs()))
	for i := range definition.GetFrameworkControlRefs() {
		assert.True(t, proto.Equal(definition.GetFrameworkControlRefs()[i], result.GetFrameworkControlRefs()[i]))
	}
}
