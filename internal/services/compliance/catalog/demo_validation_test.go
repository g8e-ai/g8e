// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package catalog_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func validFrameworkControlReference() *compliancev1.FrameworkControlReference {
	return &compliancev1.FrameworkControlReference{
		FrameworkRef: &compliancev1.VersionedReference{Id: "fedramp-20x", Version: "CR26-2026-06-24"},
		ControlId:    "KSI-MLA-07",
	}
}

func validDemoScenarioDefinition() *compliancev1.DemoScenarioDefinition {
	return &compliancev1.DemoScenarioDefinition{
		ScenarioId:                  "fedramp-deny",
		ScenarioVersion:             "1.0.0",
		DisplayNumber:               "2",
		Title:                       "Unauthorized Audit Trail Destruction Blocked",
		Purpose:                     "Verify fail-closed audit-trail protection.",
		RiskCategory:                "audit-integrity",
		ExpectedActionClasses:       []string{"FILE_DELETE"},
		ExpectedOutcome:             "blocked",
		ExpectedRejectionLayer:      "L1",
		InitialStateFixtureRef:      "cloudsvc-operations-log-present",
		TerminalStateAssertions:     []string{"operations log remains non-empty"},
		RequiredReceipts:            []string{"failed-stage"},
		RequiredDeterministicStages: []string{"L1"},
		AssertionRefs:               []*compliancev1.VersionedReference{{Id: "G8E-GOV-BLOCK-001", Version: "1.0.0"}},
		FrameworkControlRefs:        []*compliancev1.FrameworkControlReference{validFrameworkControlReference()},
		RequiredEvidenceTypes:       []string{"action_receipt", "state_observation"},
		RequiredEvidenceLevel:       "L3",
		TimeoutSeconds:              300,
		FailurePolicy:               "fail_closed",
		HarnessScenario:             "fedramp-deny",
	}
}

func validDemoManifest() *compliancev1.DemoManifest {
	return &compliancev1.DemoManifest{
		DemoId:                 "fedramp",
		DemoVersion:            "1.0.0",
		RunId:                  "run-1",
		ScopeId:                "scope-1",
		GeneratedAt:            timestamppb.New(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)),
		ScenarioDefinitionRefs: []*compliancev1.VersionedReference{{Id: "fedramp-deny", Version: "1.0.0"}},
		ProvenanceHashes:       []*compliancev1.NamedDigest{{Name: constants.DemosComposeFile, Sha256: validSHA256}},
		RequiredEnvironment:    []string{"docker", "g8e-binary"},
		FrameworkControlRefs:   []*compliancev1.FrameworkControlReference{validFrameworkControlReference()},
		SupportedLanes:         []string{"automated", "manual-notary"},
	}
}

func validDemoStepResult() *compliancev1.DemoStepResult {
	started := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	return &compliancev1.DemoStepResult{
		StepId:      "state-observation",
		Operation:   "observe cloud service operations log",
		StartedAt:   timestamppb.New(started),
		CompletedAt: timestamppb.New(started.Add(time.Second)),
		Status:      "passed",
		EvidenceRefs: []string{
			"artifact-state-1",
		},
		Required: true,
	}
}

func validDemoScenarioResult() *compliancev1.DemoScenarioResult {
	started := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	return &compliancev1.DemoScenarioResult{
		ResultId:             "run-1:fedramp-deny",
		ScenarioRef:          &compliancev1.VersionedReference{Id: "fedramp-deny", Version: "1.0.0"},
		DemoId:               "fedramp",
		ScopeId:              "scope-1",
		RunId:                "run-1",
		StartedAt:            timestamppb.New(started),
		CompletedAt:          timestamppb.New(started.Add(time.Second)),
		Status:               "passed",
		AttemptIds:           []string{"attempt-1"},
		ExecutionIds:         []string{"execution-1"},
		TransactionIds:       []string{"transaction-1"},
		ReceiptRefs:          []string{"artifact-receipt-1"},
		StateObservationRefs: []string{"artifact-state-1"},
		MetricRefs:           []string{"policy_outcome@1.0.0"},
		KsiRefs:              []string{"KSI-MLA-07"},
		AssertionRefs:        []*compliancev1.VersionedReference{{Id: "G8E-GOV-BLOCK-001", Version: "1.0.0"}},
		FrameworkControlRefs: []*compliancev1.FrameworkControlReference{validFrameworkControlReference()},
		StepResults:          []*compliancev1.DemoStepResult{validDemoStepResult()},
		VerificationStatus:   "verified",
		DisplayNumber:        "2",
		Title:                "Unauthorized Audit Trail Destruction Blocked",
		MetricsSummary:       "L1 denial and independent state verified",
	}
}

func TestValidateDemoManifestRejectsIncompleteOrUnresolvedRequirements(t *testing.T) {
	frameworks := &compliancev1.FrameworkCatalog{CatalogId: "frameworks", CatalogVersion: "1.0.0", Sha256: validSHA256, Frameworks: []*compliancev1.FrameworkDefinition{validFramework()}}
	tests := []struct {
		name   string
		mutate func(*compliancev1.DemoManifest)
		want   error
	}{
		{name: "missing run identity", mutate: func(m *compliancev1.DemoManifest) { m.RunId = "" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "duplicate scenario reference", mutate: func(m *compliancev1.DemoManifest) {
			m.ScenarioDefinitionRefs = append(m.ScenarioDefinitionRefs, m.ScenarioDefinitionRefs[0])
		}, want: constants.ErrInvalidEvidenceGraph},
		{name: "unresolved scenario reference", mutate: func(m *compliancev1.DemoManifest) { m.ScenarioDefinitionRefs[0].Version = "2.0.0" }, want: constants.ErrUnresolvedReference},
		{name: "malformed provenance digest", mutate: func(m *compliancev1.DemoManifest) { m.ProvenanceHashes[0].Sha256 = "invalid" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "duplicate environment requirement", mutate: func(m *compliancev1.DemoManifest) {
			m.RequiredEnvironment = append(m.RequiredEnvironment, m.RequiredEnvironment[0])
		}, want: constants.ErrInvalidEvidenceGraph},
		{name: "duplicate execution lane", mutate: func(m *compliancev1.DemoManifest) { m.SupportedLanes = append(m.SupportedLanes, m.SupportedLanes[0]) }, want: constants.ErrInvalidEvidenceGraph},
		{name: "unknown framework control", mutate: func(m *compliancev1.DemoManifest) { m.FrameworkControlRefs[0].ControlId = "KSI-UNKNOWN" }, want: constants.ErrUnresolvedReference},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := validDemoManifest()
			tt.mutate(manifest)
			err := catalog.ValidateDemoManifest(manifest, []*compliancev1.DemoScenarioDefinition{nil, validDemoScenarioDefinition()}, frameworks)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
	assert.NoError(t, catalog.ValidateDemoManifest(validDemoManifest(), []*compliancev1.DemoScenarioDefinition{nil, validDemoScenarioDefinition()}, frameworks))
}

func TestValidateDemoScenarioDefinitionRejectsIncompleteOrUnresolvedRequirements(t *testing.T) {
	frameworks := &compliancev1.FrameworkCatalog{CatalogId: "frameworks", CatalogVersion: "1.0.0", Sha256: validSHA256, Frameworks: []*compliancev1.FrameworkDefinition{validFramework()}}
	tests := []struct {
		name   string
		mutate func(*compliancev1.DemoScenarioDefinition)
		want   error
	}{
		{name: "missing stable identity", mutate: func(d *compliancev1.DemoScenarioDefinition) { d.ScenarioId = "" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "missing action classes", mutate: func(d *compliancev1.DemoScenarioDefinition) { d.ExpectedActionClasses = nil }, want: constants.ErrInvalidEvidenceGraph},
		{name: "invalid expected outcome", mutate: func(d *compliancev1.DemoScenarioDefinition) { d.ExpectedOutcome = "unknown" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "blocked outcome without rejection layer", mutate: func(d *compliancev1.DemoScenarioDefinition) { d.ExpectedRejectionLayer = "" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "duplicate action class", mutate: func(d *compliancev1.DemoScenarioDefinition) {
			d.ExpectedActionClasses = append(d.ExpectedActionClasses, d.ExpectedActionClasses[0])
		}, want: constants.ErrInvalidEvidenceGraph},
		{name: "duplicate assertion reference", mutate: func(d *compliancev1.DemoScenarioDefinition) {
			d.AssertionRefs = append(d.AssertionRefs, d.AssertionRefs[0])
		}, want: constants.ErrInvalidEvidenceGraph},
		{name: "unknown assertion", mutate: func(d *compliancev1.DemoScenarioDefinition) { d.AssertionRefs[0].Version = "2.0.0" }, want: constants.ErrUnsupportedAssertion},
		{name: "incomplete framework control", mutate: func(d *compliancev1.DemoScenarioDefinition) { d.FrameworkControlRefs[0].ControlId = "" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "unknown framework", mutate: func(d *compliancev1.DemoScenarioDefinition) { d.FrameworkControlRefs[0].FrameworkRef.Version = "2.0.0" }, want: constants.ErrUnsupportedFramework},
		{name: "unknown framework control", mutate: func(d *compliancev1.DemoScenarioDefinition) { d.FrameworkControlRefs[0].ControlId = "KSI-UNKNOWN" }, want: constants.ErrUnresolvedReference},
		{name: "duplicate framework control", mutate: func(d *compliancev1.DemoScenarioDefinition) {
			d.FrameworkControlRefs = append(d.FrameworkControlRefs, d.FrameworkControlRefs[0])
		}, want: constants.ErrInvalidEvidenceGraph},
		{name: "invalid evidence level", mutate: func(d *compliancev1.DemoScenarioDefinition) { d.RequiredEvidenceLevel = "L9" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "zero timeout", mutate: func(d *compliancev1.DemoScenarioDefinition) { d.TimeoutSeconds = 0 }, want: constants.ErrInvalidEvidenceGraph},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := validDemoScenarioDefinition()
			tt.mutate(definition)
			err := catalog.ValidateDemoScenarioDefinition(definition, validAssertionCatalog(), frameworks)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
	assert.NoError(t, catalog.ValidateDemoScenarioDefinition(validDemoScenarioDefinition(), validAssertionCatalog(), frameworks))
}

func TestValidateDemoStepResultPreservesEveryTerminalStatus(t *testing.T) {
	statuses := []string{"passed", "failed", "skipped", "cancelled", "timed_out", "unverifiable"}
	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			step := validDemoStepResult()
			step.Status = status
			if status != "passed" {
				step.Failure = status
			}
			assert.NoError(t, catalog.ValidateDemoStepResult(step))
		})
	}
}

func TestValidateDemoStepResultRejectsContradictoryOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*compliancev1.DemoStepResult)
	}{
		{name: "completion before start", mutate: func(s *compliancev1.DemoStepResult) {
			s.CompletedAt = timestamppb.New(s.StartedAt.AsTime().Add(-time.Second))
		}},
		{name: "passed with failure", mutate: func(s *compliancev1.DemoStepResult) { s.Failure = "failed" }},
		{name: "failed without failure reason", mutate: func(s *compliancev1.DemoStepResult) { s.Status = "failed" }},
		{name: "duplicate evidence reference", mutate: func(s *compliancev1.DemoStepResult) { s.EvidenceRefs = append(s.EvidenceRefs, s.EvidenceRefs[0]) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := validDemoStepResult()
			tt.mutate(step)
			err := catalog.ValidateDemoStepResult(step)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestValidateDemoScenarioResultRejectsDuplicateAttemptIDs(t *testing.T) {
	result := validDemoScenarioResult()
	result.AttemptIds = append(result.AttemptIds, result.AttemptIds[0])

	err := catalog.ValidateDemoScenarioResult(result, validDemoScenarioDefinition(), "scope-1")

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestValidateDemoScenarioResultRejectsDuplicateInvestigationIDs(t *testing.T) {
	result := validDemoScenarioResult()
	result.InvestigationIds = []string{"investigation-1", "investigation-1"}

	err := catalog.ValidateDemoScenarioResult(result, validDemoScenarioDefinition(), "scope-1")

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestValidateDemoScenarioResultFailsClosedOnMissingRequiredEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*compliancev1.DemoScenarioResult)
		want   error
	}{
		{name: "missing result identity", mutate: func(r *compliancev1.DemoScenarioResult) { r.ResultId = "" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "cross-scope result", mutate: func(r *compliancev1.DemoScenarioResult) { r.ScopeId = "scope-2" }, want: constants.ErrEvidenceScopeMismatch},
		{name: "duplicate execution ID", mutate: func(r *compliancev1.DemoScenarioResult) { r.ExecutionIds = append(r.ExecutionIds, r.ExecutionIds[0]) }, want: constants.ErrInvalidEvidenceGraph},
		{name: "missing assertion reference", mutate: func(r *compliancev1.DemoScenarioResult) { r.AssertionRefs = nil }, want: constants.ErrUnresolvedReference},
		{name: "missing framework control reference", mutate: func(r *compliancev1.DemoScenarioResult) { r.FrameworkControlRefs = nil }, want: constants.ErrUnresolvedReference},
		{name: "missing receipt", mutate: func(r *compliancev1.DemoScenarioResult) { r.ReceiptRefs = nil }, want: constants.ErrInvalidEvidenceGraph},
		{name: "missing state observation", mutate: func(r *compliancev1.DemoScenarioResult) { r.StateObservationRefs = nil }, want: constants.ErrInvalidEvidenceGraph},
		{name: "unverified pass", mutate: func(r *compliancev1.DemoScenarioResult) { r.VerificationStatus = "unverifiable" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "failed result without reason", mutate: func(r *compliancev1.DemoScenarioResult) { r.Status = "failed" }, want: constants.ErrInvalidEvidenceGraph},
		{name: "failed required step in pass", mutate: func(r *compliancev1.DemoScenarioResult) {
			r.StepResults[0].Status = "failed"
			r.StepResults[0].Failure = "failed"
		}, want: constants.ErrInvalidEvidenceGraph},
		{name: "mismatched scenario version", mutate: func(r *compliancev1.DemoScenarioResult) { r.ScenarioRef.Version = "2.0.0" }, want: constants.ErrUnresolvedReference},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validDemoScenarioResult()
			tt.mutate(result)
			err := catalog.ValidateDemoScenarioResult(result, validDemoScenarioDefinition(), "scope-1")
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
	assert.NoError(t, catalog.ValidateDemoScenarioResult(validDemoScenarioResult(), validDemoScenarioDefinition(), "scope-1"))
}

func TestValidateDemoScenarioResultRejectsMissingRequiredMetricEvidence(t *testing.T) {
	definition := validDemoScenarioDefinition()
	definition.RequiredEvidenceTypes = append(definition.RequiredEvidenceTypes, "grader_metric")
	result := validDemoScenarioResult()
	result.MetricRefs = nil

	err := catalog.ValidateDemoScenarioResult(result, definition, "scope-1")

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}
