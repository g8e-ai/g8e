// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package report

import (
	"bytes"
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
	tests := []struct {
		name     string
		analysis *compliancev1.ComplianceAnalysis
		format   Format
	}{
		{name: "nil analysis", format: FormatJSON},
		{name: "unsupported format", analysis: rendererTestAnalysis(), format: Format("yaml")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rendered, err := RenderComplianceAnalysis(test.analysis, test.format)

			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrValidationFailed)
			assert.Nil(t, rendered)
		})
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
