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
		BundlePath:         constants.EvalRunReceiptsFilename,
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
		{format: FormatOSCAL, expectedSHA256: "e6452f033cf7c29b951b803f4114d8e53bff365bb3aefed862876f9cb8d76f5c"},
		{format: FormatMarkdown, expectedSHA256: "d9c9a0195677a4bef269f56670fcf77f90cdc114ad6b1992bd1cb254fe6265d5"},
		{format: FormatHTML, expectedSHA256: "2ad80e6c4c4ef18913ba925bd3f3598ce109dc42b25fe583fea337c9728f135f"},
		{format: FormatCLI, expectedSHA256: "ddb408d7bbde738afd7b09add4af91b1a137a8dfcd3061c1023bc8cd33669e84"},
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
