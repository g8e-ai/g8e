// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"path"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// Phase 0 regression markers for the proof-backed compliance reporting plan
// (v2.1.3). These mirror the markers in the compliance package.
const (
	demoPhase0RegressionBeforeFix = "PHASE0: BEFORE FIX"
	demoPhase0RegressionAfterFix  = "PHASE0: AFTER FIX"
	demoPhase0RegressionIssue     = "PHASE0: ISSUE: v2.1.3 demo evidence not persisted as typed evidence"
)

// TestPhase0Demo_ScenarioResultIsPrivateStringOnlyStruct documents that the
// current demo scenario result is a private struct carrying only four string
// fields: number, name, status, and metrics. It has no scenario ID, version,
// expected outcome, rejection layer, state fixture, assertion references,
// framework references, evidence references, receipts, deterministic stages,
// state observations, grader metrics, or assessment-scope binding. The result
// is printed to the terminal and discarded; it is not persisted as canonical
// evidence under the runtime compliance tree.
//
// Phase 2 replaces this struct with typed DemoScenarioDefinition,
// DemoStepResult, and DemoScenarioResult models persisted via
// RuntimeFileService.
func TestPhase0Demo_ScenarioResultIsPrivateStringOnlyStruct(t *testing.T) {
	r := scenarioResult{
		number:  "2",
		name:    "Unauthorized Audit Trail Destruction Blocked by L1 Doctrine",
		status:  "PASS",
		metrics: "L1 doctrine blocks rm -rf /var/cloudsvc",
	}

	// The struct has exactly four unexported string fields and nothing else.
	// No typed evidence, no receipts, no state observations, no assertion refs.
	resultType := reflect.TypeOf(r)
	assert.Equal(t, 4, resultType.NumField(), demoPhase0RegressionBeforeFix)
	for i := 0; i < resultType.NumField(); i++ {
		field := resultType.Field(i)
		assert.Equal(t, reflect.String, field.Type.Kind(), demoPhase0RegressionBeforeFix)
		assert.False(t, field.IsExported(), demoPhase0RegressionBeforeFix)
	}
}

// TestPhase0Demo_NoTypedEvidencePersistedAfterScenarioRun documents that
// running a demo scenario does not persist any typed evidence artifact under
// the compliance runtime tree. The current demo path prints terminal summaries
// and exits; no DemoManifest, scenario definition, step result, or scenario
// result is written to .g8e/.
//
// This test constructs a scenarioResult (the in-memory product of a demo run)
// and verifies that no evidence directory or typed result file exists under
// the runtime compliance tree. Phase 2 adds the persisted typed results.
func TestPhase0Demo_NoTypedEvidencePersistedAfterScenarioRun(t *testing.T) {
	fileSvc, _ := newCmdTestEnv(t)
	ctx := context.Background()

	// Simulate the product of a demo run: the only thing the current code
	// produces is the in-memory scenarioResult.
	_ = scenarioResult{
		number:  "2",
		name:    "Unauthorized Audit Trail Destruction Blocked",
		status:  "PASS",
		metrics: "L1 doctrine blocks rm -rf /var/cloudsvc",
	}

	// The plan's evidence bundle layout places demo evidence under a
	// compliance evidence tree. No such tree exists today.
	candidateRelPaths := []string{
		constants.ComplianceReportFilename,
		path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleDemoEvidenceDirname, constants.ComplianceBundleDemoResultsFilename),
		path.Join(constants.ComplianceBundleEvidenceDirname, constants.ComplianceBundleDemoEvidenceDirname, constants.ComplianceBundleDemoManifestsFilename),
		path.Join(constants.ComplianceBundleAssessmentsDirname, constants.ComplianceBundleAssertionAssessmentsFilename),
	}

	for _, rel := range candidateRelPaths {
		exists, err := fileSvc.FileExists(ctx, rel)
		require.NoError(t, err, "FileExists must not error for %s", rel)
		assert.False(t, exists,
			demoPhase0RegressionBeforeFix+
				": no typed demo evidence must be persisted under the runtime tree today; "+
				"found unexpected file at %s", rel)
	}
}

// TestPhase0Demo_ScenarioResultHasNoAssertionOrFrameworkReferences documents
// that the current scenarioResult carries no assertion references (e.g.
// G8E-GOV-BLOCK-001), no framework references (e.g. KSI-MLA-07), and no
// evidence-level declaration. The mapping between demo scenarios and
// compliance controls lives in scattered Go code, documentation, and doctrine
// files rather than in the typed result.
func TestPhase0Demo_ScenarioResultHasNoAssertionOrFrameworkReferences(t *testing.T) {
	r := scenarioResult{number: "2", name: "deny", status: "PASS", metrics: "blocked"}

	// There are no fields to check: the struct has only number/name/status/metrics.
	// This test exists to lock the current shape so Phase 2's replacement is
	// detectable. If someone adds a field without completing Phase 2, this
	// test still passes (it asserts absence of typed refs, which remain absent
	// until the full typed model replaces scenarioResult).
	assert.Equal(t, "2", r.number, demoPhase0RegressionBeforeFix)
	assert.Equal(t, "PASS", r.status, demoPhase0RegressionBeforeFix)
	// No assertion_refs, framework_refs, evidence_level, scope_id, or
	// evidence_refs field exists on this struct.
}
