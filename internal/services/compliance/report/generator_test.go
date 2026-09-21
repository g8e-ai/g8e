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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
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

func validGenerationScope(windowStart, windowEnd time.Time) *compliancev1.AssessmentScope {
	return &compliancev1.AssessmentScope{
		ScopeId:               "scope-1",
		OrganizationId:        "org-1",
		DeploymentId:          "deployment-1",
		ProductVersion:        "2.1.12",
		BuildIdentity:         "build-1",
		SourceRevision:        "revision-1",
		ComponentInventory:    []*compliancev1.ComponentInventoryEntry{{ComponentId: "operator-1", ComponentType: "operator", Version: "2.1.12", Digest: strings.Repeat("a", 64)}},
		NetworkTopologyHash:   strings.Repeat("b", 64),
		ConfigurationHashes:   []*compliancev1.NamedDigest{{Name: "operator-1", Sha256: strings.Repeat("c", 64)}},
		DoctrineBundleHashes:  []*compliancev1.NamedDigest{{Name: "doctrine", Sha256: strings.Repeat("d", 64)}},
		TrustAnchorIds:        []string{"root-1"},
		CryptographicMode:     "standard",
		AssessmentWindowStart: timestamppb.New(windowStart),
		AssessmentWindowEnd:   timestamppb.New(windowEnd),
		ActivePosture:         constants.PostureDoctrine,
		SourceAdmissions: []*compliancev1.AssessmentSourceAdmission{{
			AdmissionId:              "source-1",
			SourceKind:               "native-evaluation",
			SourceVersion:            "1.0.0",
			SourceScopeId:            "source-scope-1",
			OwnerRuntimeBoundary:     "operator-1",
			AcquisitionBoundary:      "operator-local-export",
			RunId:                    "run-1",
			VerifierRef:              &compliancev1.VersionedReference{Id: "generation-test-verifier", Version: "1.0.0"},
			DisclosureClassification: constants.ComplianceBundleProfileRestricted,
		}},
		Applicability: &compliancev1.AssessmentApplicabilitySelection{
			Components:    []string{"operator"},
			ActionClasses: []string{"governed_mutation"},
			Arms:          []string{"governed"},
		},
		SelectedPopulation: &compliancev1.AssessmentPopulationSelection{},
		AssessmentAsOf:     timestamppb.New(windowEnd),
	}
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
		RunID:              "run-1",
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
		Scope:      validGenerationScope(windowStart, windowEnd),
		Sources:    []GenerationSource{{AdmissionID: "source-1", Importer: generationImporter{nodes: []evidence.EvidenceNode{validGenerationNode("scope-1", windowStart, windowEnd)}}}},
		Assertions: assertions,
		Frameworks: frameworks,
		Crosswalks: crosswalks,
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
	assert.Len(t, result.Analysis.GetAssessmentScopeSha256(), 64)
	require.Len(t, result.Analysis.GetEvidenceResources(), 1)
	assert.Equal(t, "source-1", result.Analysis.GetEvidenceResources()[0].GetSourceAdmissionId())
	require.Len(t, result.Profiles, len(frameworks.GetFrameworks()))
	for _, profile := range result.Profiles {
		assert.Equal(t, result.Analysis.GetAnalysisId(), profile.GetAnalysisRef())
		assert.Equal(t, constants.FrameworkProfileVersion, profile.GetProfileVersion())
	}
}

func TestGenerateComplianceAnalysis_UsesProtectedPopulationForCoverageAndDiagnostics(t *testing.T) {
	windowStart := time.Unix(1_700_000_000, 0).UTC()
	windowEnd := windowStart.Add(time.Hour)
	assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	scope := validGenerationScope(windowStart, windowEnd)
	scope.SelectedPopulation.Subjects = []*compliancev1.AssessmentSubjectSelection{{SourceAdmissionId: "source-1", RunId: "run-1", ScenarioId: "selected-scenario"}}

	result, err := GenerateComplianceAnalysis(context.Background(), GenerationRequest{
		Scope:      scope,
		Sources:    []GenerationSource{{AdmissionID: "source-1", Importer: generationImporter{nodes: []evidence.EvidenceNode{validGenerationNode("scope-1", windowStart, windowEnd)}}}},
		Assertions: assertions,
		Frameworks: frameworks,
		Crosswalks: crosswalks,
	})

	require.NoError(t, err)
	diagnosticCount := 0
	for _, assessment := range result.Analysis.GetAssertionAssessments() {
		if assessment.GetCoverage() == nil {
			continue
		}
		assert.Equal(t, int32(1), assessment.GetCoverage().GetSelectedSubjectCount())
		assert.Equal(t, int32(1), assessment.GetCoverage().GetUnavailableSubjectCount())
		require.Len(t, assessment.GetCoverage().GetUnavailableSubjects(), 1)
		assert.Equal(t, "source-1", assessment.GetCoverage().GetUnavailableSubjects()[0].GetSourceAdmissionId())
		assert.Equal(t, "selected-scenario", assessment.GetCoverage().GetUnavailableSubjects()[0].GetScenarioId())
		diagnosticCount += len(assessment.GetDiagnostics())
	}
	require.Positive(t, diagnosticCount)
	assert.Len(t, result.Analysis.GetDiagnostics(), diagnosticCount)
	for _, diagnostic := range result.Analysis.GetDiagnostics() {
		assert.Equal(t, "source-1", diagnostic.GetSourceAdmissionId())
		assert.Equal(t, "selected-scenario", diagnostic.GetSubject().GetScenarioId())
	}
}

func TestGenerateComplianceAnalysis_RejectsMissingImporters(t *testing.T) {
	windowStart := time.Unix(1_700_000_000, 0).UTC()
	windowEnd := windowStart.Add(time.Hour)
	assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)

	result, err := GenerateComplianceAnalysis(context.Background(), GenerationRequest{
		Scope:      validGenerationScope(windowStart, windowEnd),
		Assertions: assertions,
		Frameworks: frameworks,
		Crosswalks: crosswalks,
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
	scope := validGenerationScope(windowStart, windowEnd)
	scope.ScopeId = "scope-2"

	result, err := GenerateComplianceAnalysis(context.Background(), GenerationRequest{
		Scope:      scope,
		Sources:    []GenerationSource{{AdmissionID: "source-1", Importer: generationImporter{nodes: []evidence.EvidenceNode{validGenerationNode("scope-1", windowStart, windowEnd)}}}},
		Assertions: assertions,
		Frameworks: frameworks,
		Crosswalks: crosswalks,
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
		Scope:      validGenerationScope(windowStart, windowEnd),
		Sources:    []GenerationSource{{AdmissionID: "source-1", Importer: generationImporter{err: importErr}}},
		Assertions: assertions,
		Frameworks: frameworks,
		Crosswalks: crosswalks,
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrReportVerificationFailed)
	require.NotNil(t, result)
	assert.Nil(t, result.Analysis)
	assert.False(t, result.GraphReport.Valid)
	require.Len(t, result.GraphReport.ImporterErrors, 1)
	assert.Contains(t, result.GraphReport.ImporterErrors[0].Error, importErr.Error())
}
