// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package report

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
)

type generationImporter struct {
	nodes []evidence.EvidenceNode
	err   error
}

func (i generationImporter) Import(context.Context) ([]evidence.EvidenceNode, error) {
	return i.nodes, i.err
}

func (generationImporter) SourceID() string {
	return "generation-test"
}

func validGenerationNode(scopeID string, producedAt, verifiedAt time.Time) evidence.EvidenceNode {
	body := []byte(`{"schema_version":"1.0.0"}`)
	artifactID := evidence.ContentAddress(evidence.ArtifactTypeDemoManifest, body)
	return evidence.EvidenceNode{
		ArtifactID:         artifactID,
		ArtifactType:       evidence.ArtifactTypeDemoManifest,
		SHA256:             artifactID[len(artifactID)-64:],
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          "g8e.compliance.v1.DemoRunManifest",
		ProducerIdentity:   "generation-test@1.0.0",
		ProducedAt:         producedAt,
		ScopeID:            scopeID,
		VerificationStatus: evidence.VerificationStatusVerified,
		VerifierID:         "generation-test-verifier",
		VerifierVersion:    "1.0.0",
		VerifiedAt:         verifiedAt,
		BundlePath:         constants.ComplianceBundleManifestFilename,
		CanonicalBytes:     body,
	}
}

func TestGenerateComplianceAnalysis_OrchestratesVerifiedEvidenceThroughCanonicalAnalysis(t *testing.T) {
	windowStart := time.Unix(1_700_000_000, 0).UTC()
	windowEnd := windowStart.Add(time.Hour)
	assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)

	result, err := GenerateComplianceAnalysis(context.Background(), GenerationRequest{
		ScopeID:     "scope-1",
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		EvaluatedAt: windowEnd,
		Importers:   []evidence.EvidenceImporter{generationImporter{nodes: []evidence.EvidenceNode{validGenerationNode("scope-1", windowStart, windowEnd)}}},
		Assertions:  assertions,
		Frameworks:  frameworks,
		Crosswalks:  crosswalks,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Analysis)
	assert.True(t, result.GraphReport.Valid)
	assert.Equal(t, "scope-1", result.Analysis.GetScopeRef())
	assert.Equal(t, constants.AnalysisBuilderID, result.Analysis.GetGeneratorIdentity())
	assert.Len(t, result.Analysis.GetAssertionAssessments(), len(assertions.GetAssertions()))
	assert.NotEmpty(t, result.Analysis.GetFrameworkAssessments())
	assert.True(t, result.Analysis.GetEvidenceGraphValid())
}

func TestGenerateComplianceAnalysis_RejectsMissingImporters(t *testing.T) {
	windowStart := time.Unix(1_700_000_000, 0).UTC()
	windowEnd := windowStart.Add(time.Hour)
	assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)

	result, err := GenerateComplianceAnalysis(context.Background(), GenerationRequest{
		ScopeID:     "scope-1",
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		EvaluatedAt: windowEnd,
		Assertions:  assertions,
		Frameworks:  frameworks,
		Crosswalks:  crosswalks,
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
	assert.Nil(t, result)
}

func TestGenerateComplianceAnalysis_RejectsEvidenceFromAnotherScope(t *testing.T) {
	windowStart := time.Unix(1_700_000_000, 0).UTC()
	windowEnd := windowStart.Add(time.Hour)
	assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)

	result, err := GenerateComplianceAnalysis(context.Background(), GenerationRequest{
		ScopeID:     "scope-2",
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		EvaluatedAt: windowEnd,
		Importers:   []evidence.EvidenceImporter{generationImporter{nodes: []evidence.EvidenceNode{validGenerationNode("scope-1", windowStart, windowEnd)}}},
		Assertions:  assertions,
		Frameworks:  frameworks,
		Crosswalks:  crosswalks,
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceScopeMismatch)
	require.NotNil(t, result)
	assert.Nil(t, result.Analysis)
}

func TestGenerateComplianceAnalysis_RejectsImporterFailureBeforeGrading(t *testing.T) {
	windowStart := time.Unix(1_700_000_000, 0).UTC()
	windowEnd := windowStart.Add(time.Hour)
	assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	importErr := errors.New("import failed")

	result, err := GenerateComplianceAnalysis(context.Background(), GenerationRequest{
		ScopeID:     "scope-1",
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		EvaluatedAt: windowEnd,
		Importers:   []evidence.EvidenceImporter{generationImporter{err: importErr}},
		Assertions:  assertions,
		Frameworks:  frameworks,
		Crosswalks:  crosswalks,
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
	require.NotNil(t, result)
	assert.Nil(t, result.Analysis)
	assert.False(t, result.GraphReport.Valid)
	require.Len(t, result.GraphReport.ImporterErrors, 1)
	assert.Contains(t, result.GraphReport.ImporterErrors[0].Error, importErr.Error())
}
