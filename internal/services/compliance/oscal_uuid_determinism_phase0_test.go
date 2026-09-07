// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// TestPhase0OSCAL_IdenticalInputsProduceByteIdenticalOutput verifies that identical canonical analysis produces reproducible unsigned OSCAL output.
func TestPhase0OSCAL_IdenticalInputsProduceByteIdenticalOutput(t *testing.T) {
	analysis := oscalTestAnalysis()
	exporter := NewOSCALExporter(oscalTestCatalog())

	doc1, err := exporter.GenerateAssessmentResults(analysis)
	require.NoError(t, err)
	doc2, err := exporter.GenerateAssessmentResults(analysis)
	require.NoError(t, err)

	raw1, err := json.Marshal(doc1)
	require.NoError(t, err)
	raw2, err := json.Marshal(doc2)
	require.NoError(t, err)

	assert.Equal(t, string(raw1), string(raw2), phase0RegressionAfterFix+": identical inputs produce byte-identical OSCAL output")
}

// TestPhase0OSCAL_ComponentIdentifiersAreStable verifies that catalog-bound component identifiers remain stable across generation calls.
func TestPhase0OSCAL_ComponentIdentifiersAreStable(t *testing.T) {
	exporter := NewOSCALExporter(oscalTestCatalog())

	compDef1, err := exporter.GenerateComponentDefinition()
	require.NoError(t, err)
	compDef2, err := exporter.GenerateComponentDefinition()
	require.NoError(t, err)

	assert.Equal(t, compDef1.UUID, compDef2.UUID, phase0RegressionAfterFix+": component-definition UUID is catalog-bound")
	require.NotEmpty(t, compDef1.Components)
	require.NotEmpty(t, compDef2.Components)
	assert.Equal(t, compDef1.Components[0].UUID, compDef2.Components[0].UUID, phase0RegressionAfterFix+": component UUID is catalog-bound")
}

// TestPhase0OSCAL_GenerateUUIDIsIdentityBound verifies deterministic namespace derivation and identity separation.
func TestPhase0OSCAL_GenerateUUIDIsIdentityBound(t *testing.T) {
	u1 := generateUUID("observation", "analysis-1", "assertion-1")
	u2 := generateUUID("observation", "analysis-1", "assertion-1")
	u3 := generateUUID("observation", "analysis-1", "assertion-2")

	assert.Equal(t, u1, u2, phase0RegressionAfterFix+": equal identities derive equal UUIDs")
	assert.NotEqual(t, u1, u3, phase0RegressionAfterFix+": distinct assertion identities derive distinct UUIDs")
}

func TestPhase0OSCAL_IdentifiersBindCanonicalRecordIdentities(t *testing.T) {
	newAnalysis := func() *compliancev1.ComplianceAnalysis {
		analysis := oscalTestAnalysis()
		analysis.AssertionAssessments = analysis.AssertionAssessments[:1]
		analysis.FrameworkAssessments = analysis.FrameworkAssessments[:1]
		analysis.EvidenceResources = analysis.EvidenceResources[:1]
		return analysis
	}
	tests := []struct {
		name     string
		mutate   func(*compliancev1.ComplianceAnalysis)
		selectID func(*OSCALAssessmentResults) string
	}{
		{name: "report identity", mutate: func(analysis *compliancev1.ComplianceAnalysis) { analysis.AnalysisId += "-changed" }, selectID: func(document *OSCALAssessmentResults) string { return document.UUID }},
		{name: "assertion assessment observation identity", mutate: func(analysis *compliancev1.ComplianceAnalysis) {
			analysis.AssertionAssessments[0].AssessmentId += "-changed"
			analysis.FrameworkAssessments[0].AssertionAssessmentRefs[0] = analysis.AssertionAssessments[0].AssessmentId
		}, selectID: func(document *OSCALAssessmentResults) string { return document.Results[0].Observations[0].UUID }},
		{name: "assertion assessment subject identity", mutate: func(analysis *compliancev1.ComplianceAnalysis) {
			analysis.AssertionAssessments[0].AssessmentId += "-changed"
			analysis.FrameworkAssessments[0].AssertionAssessmentRefs[0] = analysis.AssertionAssessments[0].AssessmentId
		}, selectID: func(document *OSCALAssessmentResults) string {
			return document.Results[0].Observations[0].Subjects[0].SubjectUUID
		}},
		{name: "evidence identity", mutate: func(analysis *compliancev1.ComplianceAnalysis) {
			const changedDigest = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
			changedArtifactID := "action-receipt:sha256:" + changedDigest
			analysis.EvidenceResources[0].ArtifactId = changedArtifactID
			analysis.EvidenceResources[0].Sha256 = changedDigest
			analysis.AssertionAssessments[0].EvidenceRefs[0] = changedArtifactID
		}, selectID: func(document *OSCALAssessmentResults) string { return document.BackMatter.Resources[0].UUID }},
		{name: "control identity", mutate: func(analysis *compliancev1.ComplianceAnalysis) {
			analysis.FrameworkAssessments[0].ControlId = "KSI-CMT-02"
		}, selectID: func(document *OSCALAssessmentResults) string { return document.Results[0].Findings[0].UUID }},
	}
	exporter := NewOSCALExporter(oscalTestCatalog())
	base, err := exporter.GenerateAssessmentResults(newAnalysis())
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changedAnalysis := newAnalysis()
			tt.mutate(changedAnalysis)
			changed, err := exporter.GenerateAssessmentResults(changedAnalysis)
			require.NoError(t, err)
			assert.NotEqual(t, tt.selectID(base), tt.selectID(changed), phase0RegressionAfterFix+": UUID binds the canonical record identity")
		})
	}
}
