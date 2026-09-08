// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func frameworkProfileAnalysis(t *testing.T) (*compliancev1.ComplianceAnalysis, *compliancev1.FrameworkCatalog) {
	t.Helper()
	request := analysisBaseRequest(t)
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	return analysis, request.Frameworks
}

func TestBuildFrameworkProfiles_ProjectsEachCatalogFramework(t *testing.T) {
	analysis, frameworks := frameworkProfileAnalysis(t)
	secondFramework := analysisTestFramework("nist-800-53", "rev5", analysisTestControl("AC-2", "platform"))
	frameworks.Frameworks = append(frameworks.Frameworks, secondFramework)
	analysis.FrameworkAssessments = append(analysis.FrameworkAssessments,
		analysisTestFrameworkAssessment("nist-800-53", "rev5", "AC-2", "not_satisfied", "platform", "nist limitation"),
	)

	profiles, err := evidence.BuildFrameworkProfiles(context.Background(), evidence.FrameworkProfileRequest{Analysis: analysis, Frameworks: frameworks})
	require.NoError(t, err)
	require.Len(t, profiles, 2)
	assert.Equal(t, "fedramp-20x", profiles[0].GetFrameworkRef().GetId())
	assert.Equal(t, "nist-800-53", profiles[1].GetFrameworkRef().GetId())
	assert.Len(t, profiles[0].GetControlAssessments(), 2)
	require.Len(t, profiles[1].GetControlAssessments(), 1)
	assert.Equal(t, "AC-2", profiles[1].GetControlAssessments()[0].GetControlId())
}

func TestBuildFrameworkProfiles_PreservesUnderlyingControlAssessmentOutcomes(t *testing.T) {
	analysis, frameworks := frameworkProfileAnalysis(t)
	before := proto.Clone(analysis).(*compliancev1.ComplianceAnalysis)

	profiles, err := evidence.BuildFrameworkProfiles(context.Background(), evidence.FrameworkProfileRequest{Analysis: analysis, Frameworks: frameworks})
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Len(t, profiles[0].GetControlAssessments(), len(analysis.GetFrameworkAssessments()))
	for i, assessment := range profiles[0].GetControlAssessments() {
		assert.True(t, proto.Equal(analysis.GetFrameworkAssessments()[i], assessment))
		assert.NotSame(t, analysis.GetFrameworkAssessments()[i], assessment)
	}
	assert.True(t, proto.Equal(before, analysis))
}

func TestBuildFrameworkProfiles_StampsCanonicalMetadata(t *testing.T) {
	analysis, frameworks := frameworkProfileAnalysis(t)

	profiles, err := evidence.BuildFrameworkProfiles(context.Background(), evidence.FrameworkProfileRequest{Analysis: analysis, Frameworks: frameworks})
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	profile := profiles[0]
	assert.True(t, strings.HasPrefix(profile.GetProfileId(), "framework-profile:sha256:"))
	assert.Len(t, strings.TrimPrefix(profile.GetProfileId(), "framework-profile:sha256:"), 64)
	assert.Equal(t, constants.FrameworkProfileVersion, profile.GetProfileVersion())
	assert.Equal(t, analysis.GetAnalysisId(), profile.GetAnalysisRef())
	assert.True(t, proto.Equal(analysis.GetGeneratedAt(), profile.GetGeneratedAt()))
}

func TestBuildFrameworkProfiles_AggregatesOnlyFrameworkLocalLimitations(t *testing.T) {
	analysis, frameworks := frameworkProfileAnalysis(t)
	analysis.FrameworkAssessments[0].Limitations = []string{"shared limitation", "fedramp only"}
	analysis.FrameworkAssessments[1].Limitations = []string{"shared limitation"}
	secondFramework := analysisTestFramework("nist-800-53", "rev5", analysisTestControl("AC-2", "platform"))
	frameworks.Frameworks = append(frameworks.Frameworks, secondFramework)
	analysis.FrameworkAssessments = append(analysis.FrameworkAssessments,
		analysisTestFrameworkAssessment("nist-800-53", "rev5", "AC-2", "satisfied", "platform", "nist only"),
	)

	profiles, err := evidence.BuildFrameworkProfiles(context.Background(), evidence.FrameworkProfileRequest{Analysis: analysis, Frameworks: frameworks})
	require.NoError(t, err)
	require.Len(t, profiles, 2)
	assert.Equal(t, []string{"fedramp only", "shared limitation"}, profiles[0].GetLimitations())
	assert.Equal(t, []string{"nist only"}, profiles[1].GetLimitations())
}

func TestBuildFrameworkProfiles_EmitsEmptyProfileForUnassessedFramework(t *testing.T) {
	analysis, frameworks := frameworkProfileAnalysis(t)
	frameworks.Frameworks = append(frameworks.Frameworks, analysisTestFramework("nist-800-53", "rev5", analysisTestControl("AC-2", "platform")))

	profiles, err := evidence.BuildFrameworkProfiles(context.Background(), evidence.FrameworkProfileRequest{Analysis: analysis, Frameworks: frameworks})
	require.NoError(t, err)
	require.Len(t, profiles, 2)
	assert.Empty(t, profiles[1].GetControlAssessments())
	assert.Empty(t, profiles[1].GetLimitations())
}

func TestBuildFrameworkProfiles_IsDeterministicAcrossInputOrdering(t *testing.T) {
	analysis, frameworks := frameworkProfileAnalysis(t)
	secondFramework := analysisTestFramework("nist-800-53", "rev5", analysisTestControl("AC-2", "platform"))
	frameworks.Frameworks = append([]*compliancev1.FrameworkDefinition{secondFramework}, frameworks.Frameworks...)
	analysis.FrameworkAssessments = append(analysis.FrameworkAssessments,
		analysisTestFrameworkAssessment("nist-800-53", "rev5", "AC-2", "satisfied", "platform"),
	)

	first, err := evidence.BuildFrameworkProfiles(context.Background(), evidence.FrameworkProfileRequest{Analysis: analysis, Frameworks: frameworks})
	require.NoError(t, err)
	frameworks.Frameworks[0], frameworks.Frameworks[1] = frameworks.Frameworks[1], frameworks.Frameworks[0]
	for left, right := 0, len(analysis.FrameworkAssessments)-1; left < right; left, right = left+1, right-1 {
		analysis.FrameworkAssessments[left], analysis.FrameworkAssessments[right] = analysis.FrameworkAssessments[right], analysis.FrameworkAssessments[left]
	}
	second, err := evidence.BuildFrameworkProfiles(context.Background(), evidence.FrameworkProfileRequest{Analysis: analysis, Frameworks: frameworks})
	require.NoError(t, err)
	require.Len(t, first, len(second))
	for i := range first {
		firstBytes, marshalErr := compliancev1.MarshalCanonical(first[i])
		require.NoError(t, marshalErr)
		secondBytes, marshalErr := compliancev1.MarshalCanonical(second[i])
		require.NoError(t, marshalErr)
		assert.Equal(t, firstBytes, secondBytes)
	}
}

func TestBuildFrameworkProfiles_ProfileIDBindsAssessmentContent(t *testing.T) {
	analysis, frameworks := frameworkProfileAnalysis(t)
	first, err := evidence.BuildFrameworkProfiles(context.Background(), evidence.FrameworkProfileRequest{Analysis: analysis, Frameworks: frameworks})
	require.NoError(t, err)
	analysis.FrameworkAssessments[0].Status = "not_satisfied"
	second, err := evidence.BuildFrameworkProfiles(context.Background(), evidence.FrameworkProfileRequest{Analysis: analysis, Frameworks: frameworks})
	require.NoError(t, err)
	assert.NotEqual(t, first[0].GetProfileId(), second[0].GetProfileId())
}

func TestBuildFrameworkProfiles_RejectsIncompleteInputs(t *testing.T) {
	analysis, frameworks := frameworkProfileAnalysis(t)
	tests := []struct {
		name    string
		request evidence.FrameworkProfileRequest
	}{
		{name: "nil analysis", request: evidence.FrameworkProfileRequest{Frameworks: frameworks}},
		{name: "nil frameworks", request: evidence.FrameworkProfileRequest{Analysis: analysis}},
		{name: "missing analysis identity", request: evidence.FrameworkProfileRequest{Analysis: proto.Clone(analysis).(*compliancev1.ComplianceAnalysis), Frameworks: frameworks}},
	}
	tests[2].request.Analysis.AnalysisId = ""
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := evidence.BuildFrameworkProfiles(context.Background(), test.request)
			assert.ErrorIs(t, err, constants.ErrFrameworkProfileInvalid)
		})
	}
}

func TestBuildFrameworkProfiles_RejectsInvalidOrUnknownAssessmentReferences(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*compliancev1.ComplianceAnalysis)
	}{
		{name: "cross-scope assessment", mutate: func(analysis *compliancev1.ComplianceAnalysis) { analysis.FrameworkAssessments[0].ScopeId = "scope-2" }},
		{name: "unknown framework", mutate: func(analysis *compliancev1.ComplianceAnalysis) {
			analysis.FrameworkAssessments[0].FrameworkRef.Id = "unknown"
		}},
		{name: "unknown control", mutate: func(analysis *compliancev1.ComplianceAnalysis) {
			analysis.FrameworkAssessments[0].ControlId = "unknown"
		}},
		{name: "nil assessment", mutate: func(analysis *compliancev1.ComplianceAnalysis) { analysis.FrameworkAssessments[0] = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			analysis, frameworks := frameworkProfileAnalysis(t)
			test.mutate(analysis)
			_, err := evidence.BuildFrameworkProfiles(context.Background(), evidence.FrameworkProfileRequest{Analysis: analysis, Frameworks: frameworks})
			assert.ErrorIs(t, err, constants.ErrFrameworkProfileInvalid)
		})
	}
}

func TestBuildFrameworkProfiles_RejectsDuplicateAssessments(t *testing.T) {
	analysis, frameworks := frameworkProfileAnalysis(t)
	analysis.FrameworkAssessments = append(analysis.FrameworkAssessments, proto.Clone(analysis.FrameworkAssessments[0]).(*compliancev1.FrameworkControlAssessment))

	_, err := evidence.BuildFrameworkProfiles(context.Background(), evidence.FrameworkProfileRequest{Analysis: analysis, Frameworks: frameworks})
	assert.ErrorIs(t, err, constants.ErrFrameworkProfileInvalid)
}

func TestBuildFrameworkProfiles_RejectsInvalidFrameworkCatalog(t *testing.T) {
	analysis, frameworks := frameworkProfileAnalysis(t)
	frameworks.Frameworks[0].Controls[0].SupportStatus = "invalid"

	_, err := evidence.BuildFrameworkProfiles(context.Background(), evidence.FrameworkProfileRequest{Analysis: analysis, Frameworks: frameworks})
	assert.ErrorIs(t, err, constants.ErrFrameworkProfileInvalid)
}

func TestBuildFrameworkProfiles_RejectsInvalidEvidenceGraphAnalysis(t *testing.T) {
	analysis, frameworks := frameworkProfileAnalysis(t)
	analysis.EvidenceGraphValid = false

	_, err := evidence.BuildFrameworkProfiles(context.Background(), evidence.FrameworkProfileRequest{Analysis: analysis, Frameworks: frameworks})
	assert.ErrorIs(t, err, constants.ErrFrameworkProfileInvalid)
}

func TestBuildFrameworkProfiles_PropagatesCancellation(t *testing.T) {
	analysis, frameworks := frameworkProfileAnalysis(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := evidence.BuildFrameworkProfiles(ctx, evidence.FrameworkProfileRequest{Analysis: analysis, Frameworks: frameworks})
	assert.True(t, errors.Is(err, context.Canceled))
}
