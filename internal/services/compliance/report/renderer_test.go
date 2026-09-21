// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func rendererTestAnalysis() *compliancev1.ComplianceAnalysis {
	generatedAt := timestamppb.New(time.Unix(1_700_000_000, 0).UTC())
	return &compliancev1.ComplianceAnalysis{
		AnalysisId:            "analysis:sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		AnalysisSchemaVersion: constants.AnalysisSchemaVersion,
		ScopeRef:              "scope-1",
		GeneratedAt:           generatedAt,
		GeneratorIdentity:     constants.AnalysisBuilderID,
		GeneratorVersion:      constants.AnalysisBuilderVersion,
		EvidenceWindowCompleteness: &compliancev1.EvidenceWindowCompleteness{
			ScopeId:               "scope-1",
			ExpectedEvidenceCount: 1,
			ActualEvidenceCount:   1,
			CompletenessStatus:    "complete",
			WindowStartRef:        "2023-11-14T22:13:20Z",
			WindowEndRef:          "2023-11-14T23:13:20Z",
		},
		AssertionAssessments: []*compliancev1.ControlAssertionAssessment{{
			AssessmentId:    "assertion-assessment-1",
			ScopeId:         "scope-1",
			AssertionRef:    &compliancev1.VersionedReference{Id: "assertion-1", Version: "1.0.0"},
			Status:          "satisfied",
			EvidenceLevel:   "cryptographic",
			EvaluatedAt:     generatedAt,
			VerifierRef:     &compliancev1.VersionedReference{Id: "assertion_assessment", Version: "1.0.0"},
			FreshnessStatus: "current",
		}},
		FrameworkAssessments: []*compliancev1.FrameworkControlAssessment{{
			AssessmentId:            "framework-assessment-1",
			ScopeId:                 "scope-1",
			FrameworkRef:            &compliancev1.VersionedReference{Id: "framework-1", Version: "1.0.0"},
			ControlId:               "control-1",
			Status:                  "satisfied",
			Responsibility:          "platform",
			AssertionAssessmentRefs: []string{"assertion-assessment-1"},
			EvidenceLevel:           "cryptographic",
		}},
		Limitations:        []string{"No <script>alert('unsafe')</script> limitation"},
		EvidenceGraphValid: true,
	}
}

func TestRenderComplianceAnalysis_RendersEveryViewFromCanonicalAnalysis(t *testing.T) {
	analysis := rendererTestAnalysis()
	tests := []struct {
		name      string
		format    Format
		mediaType string
		contains  []string
	}{
		{name: "canonical JSON", format: FormatJSON, mediaType: constants.MediaTypeJSON, contains: []string{analysis.GetAnalysisId(), analysis.GetScopeRef(), "assertion_assessments"}},
		{name: "OSCAL", format: FormatOSCAL, mediaType: constants.MediaTypeOSCALJSON, contains: []string{analysis.GetAnalysisId(), analysis.GetScopeRef(), "framework-1 control-1 Finding"}},
		{name: "Markdown", format: FormatMarkdown, mediaType: constants.MediaTypeMarkdown, contains: []string{analysis.GetAnalysisId(), analysis.GetScopeRef(), "| framework-1 | control-1 | satisfied |"}},
		{name: "CSV", format: FormatCSV, mediaType: constants.MediaTypeCSV, contains: []string{"record_type,identifier", analysis.GetAnalysisId(), "framework_control"}},
		{name: "HTML", format: FormatHTML, mediaType: constants.MediaTypeHTML, contains: []string{analysis.GetAnalysisId(), analysis.GetScopeRef(), "<td>control-1</td>"}},
		{name: "CLI", format: FormatCLI, mediaType: constants.MediaTypeText, contains: []string{analysis.GetAnalysisId(), analysis.GetScopeRef(), "satisfied=1"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first, err := RenderComplianceAnalysis(analysis, test.format)
			require.NoError(t, err)
			second, err := RenderComplianceAnalysis(analysis, test.format)
			require.NoError(t, err)

			assert.Equal(t, test.format, first.Format)
			assert.Equal(t, test.mediaType, first.MediaType)
			assert.Equal(t, first.Body, second.Body)
			for _, expected := range test.contains {
				assert.Contains(t, string(first.Body), expected)
			}
		})
	}
}

func TestRenderComplianceAnalysis_ProjectionIncludesCoverageDiagnosticsAndEvidenceLinks(t *testing.T) {
	analysis := rendererTestAnalysis()
	artifactID := "action-receipt:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	analysis.AssertionAssessments[0].EvidenceRefs = []string{artifactID}
	analysis.AssertionAssessments[0].Coverage = &compliancev1.AssessmentCoverage{
		SelectedSubjectCount:    2,
		AssessedSubjectCount:    1,
		FailedSubjectCount:      1,
		UnavailableSubjectCount: 1,
		UnavailableSubjects:     []*compliancev1.AssessmentSubjectSelection{{SourceAdmissionId: "source-1", RunId: "run-1", ScenarioId: "scenario-2"}},
	}
	analysis.AssertionAssessments[0].Diagnostics = []*compliancev1.AssessmentDiagnostic{{Code: "missing_observation", Severity: "warning", SourceAdmissionId: "source-1", Subject: &compliancev1.AssessmentSubjectSelection{RunId: "run-1", ScenarioId: "scenario-2"}, Message: "independent observation was not captured"}}
	analysis.Diagnostics = append(analysis.Diagnostics, analysis.AssertionAssessments[0].Diagnostics...)
	analysis.EvidenceResources = []*compliancev1.ComplianceEvidenceReference{{
		ArtifactId: artifactID, ArtifactType: "action-receipt", Sha256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", MediaType: constants.MediaTypeJSON, SchemaRef: "g8e.operator.v1.ActionReceipt", ProducerIdentity: "operator-1", ScopeId: "scope-1", RunId: "run-1", ScenarioId: "scenario-1", SourceAdmissionId: "source-1", VerificationStatus: "verified", VerifierId: "receipt-verifier", VerifierVersion: "1.0.0", BundlePath: "sources/operator-1/receipt.json",
	}}

	for _, format := range []Format{FormatCSV, FormatMarkdown, FormatHTML, FormatCLI, FormatOSCAL} {
		rendered, err := RenderComplianceAnalysis(analysis, format)
		require.NoError(t, err)
		body := string(rendered.Body)
		assert.Contains(t, body, artifactID)
		assert.Contains(t, body, "missing_observation")
		assert.Contains(t, body, "2 selected, 1 assessed, 1 failed, 1 unavailable")
	}
}

func TestRenderComplianceAnalysis_CanonicalJSONRoundTrips(t *testing.T) {
	analysis := rendererTestAnalysis()

	rendered, err := RenderComplianceAnalysis(analysis, FormatJSON)
	require.NoError(t, err)
	decoded := &compliancev1.ComplianceAnalysis{}
	require.NoError(t, compliancev1.UnmarshalCanonical(rendered.Body, decoded))

	assert.Equal(t, analysis, decoded)
}

func TestRenderComplianceAnalysis_TextRenderersEscapeAnalysisContent(t *testing.T) {
	for _, format := range []Format{FormatMarkdown, FormatHTML} {
		rendered, err := RenderComplianceAnalysis(rendererTestAnalysis(), format)
		require.NoError(t, err)

		assert.NotContains(t, string(rendered.Body), "<script>")
		assert.Contains(t, string(rendered.Body), "&lt;script&gt;")
	}
}

func TestRenderComplianceAnalysis_RejectsInvalidInput(t *testing.T) {
	invalidOSCALAnalysis := rendererTestAnalysis()
	invalidOSCALAnalysis.EvidenceResources = []*compliancev1.ComplianceEvidenceReference{{
		ArtifactId:         "action-receipt:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ArtifactType:       "action-receipt",
		Sha256:             "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		MediaType:          "invalid/media-type",
		SchemaRef:          "g8e.operator.v1.ActionReceipt",
		ProducerIdentity:   "gateway",
		ScopeId:            invalidOSCALAnalysis.GetScopeRef(),
		VerificationStatus: "verified",
		BundlePath:         constants.EvaluationReportFilename,
	}}
	tests := []struct {
		name     string
		analysis *compliancev1.ComplianceAnalysis
		format   Format
		target   error
	}{
		{name: "nil analysis", format: FormatJSON, target: constants.ErrValidationFailed},
		{name: "unsupported format", analysis: rendererTestAnalysis(), format: Format("yaml"), target: constants.ErrValidationFailed},
		{name: "OSCAL validation failure", analysis: invalidOSCALAnalysis, format: FormatOSCAL, target: constants.ErrOSCALValidationFailed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rendered, err := RenderComplianceAnalysis(test.analysis, test.format)

			require.Error(t, err)
			assert.ErrorIs(t, err, test.target)
			assert.Nil(t, rendered)
		})
	}
}

func TestRenderComplianceAnalysis_GoldenVectors(t *testing.T) {
	tests := []struct {
		format         Format
		expectedSHA256 string
	}{
		{format: FormatJSON, expectedSHA256: "9f71eaab3df307e70d12108b0400807b01fb1df813d2cff7ea7573fa3104ac93"},
		{format: FormatOSCAL, expectedSHA256: "cb9ac5cef6519b7940385f9d416e7c85e4f963c82ae16480733d85749897c8aa"},
		{format: FormatMarkdown, expectedSHA256: "88d59561c7f44f1f1642267a95320d4a0bf0c18290f3bde0fb9ae0890d300a5b"},
		{format: FormatHTML, expectedSHA256: "f84706e409b2e7ffcdf2314fa424e322740dcc2d5a0cbdf02b45ad269554e1d2"},
		{format: FormatCSV, expectedSHA256: "f2fb840b8ed213583b398518b66397dd70dc2cf0adb7b9d7e5d1d02c5c7fd5f5"},
		{format: FormatCLI, expectedSHA256: "828c8e51abe2bafc4d2b92f72bf41950de9beaf373ae59a53b37426fea469a26"},
	}
	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			rendered, err := RenderComplianceAnalysis(rendererTestAnalysis(), tt.format)
			require.NoError(t, err)
			digest := sha256.Sum256(rendered.Body)
			assert.Equal(t, tt.expectedSHA256, hex.EncodeToString(digest[:]))
		})
	}
}

func TestRenderComplianceAnalysis_CanonicalJSONReproducesEveryRenderer(t *testing.T) {
	analysis := rendererTestAnalysis()
	canonical, err := RenderComplianceAnalysis(analysis, FormatJSON)
	require.NoError(t, err)
	decoded := &compliancev1.ComplianceAnalysis{}
	require.NoError(t, compliancev1.UnmarshalCanonical(canonical.Body, decoded))
	for _, format := range SupportedFormats() {
		original, err := RenderComplianceAnalysis(analysis, format)
		require.NoError(t, err)
		reproduced, err := RenderComplianceAnalysis(decoded, format)
		require.NoError(t, err)
		assert.Equal(t, original.Body, reproduced.Body)
	}
}

func TestRenderComplianceAnalysis_FormatsProduceDistinctBodies(t *testing.T) {
	analysis := rendererTestAnalysis()
	seen := make([][]byte, 0, len(SupportedFormats()))
	for _, format := range SupportedFormats() {
		rendered, err := RenderComplianceAnalysis(analysis, format)
		require.NoError(t, err)
		for _, body := range seen {
			assert.False(t, bytes.Equal(body, rendered.Body))
		}
		seen = append(seen, rendered.Body)
	}
}
