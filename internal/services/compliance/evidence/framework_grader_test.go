// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evidence_test

import (
	"context"
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

const frameworkTestSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func frameworkTestAssertion(id string) *compliancev1.ControlAssertionDefinition {
	return &compliancev1.ControlAssertionDefinition{
		AssertionId:             id,
		AssertionVersion:        "1.0.0",
		Title:                   "Test assertion " + id,
		Statement:               "Test statement for " + id,
		Category:                "governance",
		ComponentScope:          []string{"gateway"},
		Responsibility:          "platform",
		ApplicableActionClasses: []string{"governed_mutation"},
		ApplicableArms:          []string{"governed"},
		RequiredEvidenceTypes:   []string{"action_receipt"},
		RequiredGraderRefs:      []*compliancev1.VersionedReference{{Id: "policy_outcome", Version: "1.0.0"}},
		RequiredVerifierRefs:    []*compliancev1.VersionedReference{{Id: "receipt_integrity", Version: "1.0.0"}},
		MinimumEvidenceLevel:    "L3",
		ValidationCycle:         "7d",
		MissingEvidencePolicy:   "unverifiable",
		PassingRule:             "all_required",
	}
}

func frameworkTestAssertionCatalog(ids ...string) *compliancev1.ControlAssertionCatalog {
	assertions := make([]*compliancev1.ControlAssertionDefinition, 0, len(ids))
	for _, id := range ids {
		assertions = append(assertions, frameworkTestAssertion(id))
	}
	return &compliancev1.ControlAssertionCatalog{
		CatalogId:      "framework-grader-test-assertions",
		CatalogVersion: "1.0.0",
		Sha256:         frameworkTestSHA256,
		Assertions:     assertions,
	}
}

func frameworkTestFramework(frameworkID, frameworkVersion string, controls ...*compliancev1.FrameworkControlDefinition) *compliancev1.FrameworkDefinition {
	return &compliancev1.FrameworkDefinition{
		FrameworkId:      frameworkID,
		FrameworkVersion: frameworkVersion,
		Title:            "Test Framework",
		Publisher:        "Test Publisher",
		Source:           "https://example.com/framework",
		CatalogSha256:    frameworkTestSHA256,
		EffectiveDate:    "2026-06-24",
		Controls:         controls,
	}
}

func frameworkTestControl(controlID, responsibility string) *compliancev1.FrameworkControlDefinition {
	return &compliancev1.FrameworkControlDefinition{
		ControlId:        controlID,
		Title:            "Test Control " + controlID,
		Description:      "Test control description for " + controlID,
		SourceReference:  "TEST",
		SupportStatus:    "mapped",
		SupportRationale: "Test crosswalk maps this control.",
		Responsibility:   responsibility,
	}
}

func frameworkTestCrosswalk(crosswalkID, frameworkID, frameworkVersion, controlID, mappingType, responsibility, evidenceLevel string, assertionRefs ...*compliancev1.VersionedReference) *compliancev1.ControlCrosswalk {
	return &compliancev1.ControlCrosswalk{
		CrosswalkId:           crosswalkID,
		CrosswalkVersion:      "1.0.0",
		FrameworkRef:          &compliancev1.VersionedReference{Id: frameworkID, Version: frameworkVersion},
		ControlId:             controlID,
		AssertionRefs:         assertionRefs,
		MappingType:           mappingType,
		Rationale:             "Test rationale",
		Responsibility:        responsibility,
		RequiredEvidenceLevel: evidenceLevel,
		ReviewedAt:            timestamppb.New(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)),
		ReviewerIdentity:      "g8e-project",
	}
}

func frameworkTestCatalogs(t *testing.T) (*compliancev1.ControlAssertionCatalog, *compliancev1.FrameworkCatalog, *compliancev1.ControlCrosswalkCatalog) {
	t.Helper()
	assertions := frameworkTestAssertionCatalog("G8E-GOV-BLOCK-001", "G8E-GOV-ALLOW-001")
	framework := frameworkTestFramework("fedramp-20x", "CR26-2026-06-24",
		frameworkTestControl("KSI-MLA-07", "shared"),
		frameworkTestControl("KSI-IAM-05", "shared"),
	)
	frameworks := &compliancev1.FrameworkCatalog{
		CatalogId:      "framework-grader-test-frameworks",
		CatalogVersion: "1.0.0",
		Sha256:         frameworkTestSHA256,
		Frameworks:     []*compliancev1.FrameworkDefinition{framework},
	}
	crosswalks := &compliancev1.ControlCrosswalkCatalog{
		CatalogId:      "framework-grader-test-crosswalks",
		CatalogVersion: "1.0.0",
		Sha256:         frameworkTestSHA256,
		Mappings: []*compliancev1.ControlCrosswalk{
			frameworkTestCrosswalk("fedramp-20x:KSI-MLA-07:G8E-GOV-BLOCK-001", "fedramp-20x", "CR26-2026-06-24", "KSI-MLA-07", "supporting", "shared", "L3",
				&compliancev1.VersionedReference{Id: "G8E-GOV-BLOCK-001", Version: "1.0.0"}),
			frameworkTestCrosswalk("fedramp-20x:KSI-IAM-05:G8E-GOV-ALLOW-001", "fedramp-20x", "CR26-2026-06-24", "KSI-IAM-05", "supporting", "shared", "L3",
				&compliancev1.VersionedReference{Id: "G8E-GOV-ALLOW-001", Version: "1.0.0"}),
		},
	}
	require.NoError(t, catalog.ValidateCatalogSet(assertions, frameworks, crosswalks))
	return assertions, frameworks, crosswalks
}

func frameworkTestAssessment(assertionID, status, evidenceLevel string) *compliancev1.ControlAssertionAssessment {
	failureReason := ""
	if status == "unverifiable" {
		failureReason = "required evidence is unavailable"
	}
	return &compliancev1.ControlAssertionAssessment{
		AssessmentId:    "assertion-assessment:sha256:" + assertionID + "00000000000000000000000000000000000000000000000000000000000",
		ScopeId:         "scope-1",
		AssertionRef:    &compliancev1.VersionedReference{Id: assertionID, Version: "1.0.0"},
		Status:          status,
		EvidenceLevel:   evidenceLevel,
		EvaluatedAt:     timestamppb.New(time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)),
		VerifierRef:     &compliancev1.VersionedReference{Id: constants.AssertionGraderID, Version: constants.AssertionGraderVersion},
		EvidenceRefs:    []string{"evidence-1"},
		FreshnessStatus: "fresh",
		FailureReason:   failureReason,
		Limitations:     []string{},
	}
}

func frameworkGraderBaseRequest(t *testing.T) evidence.FrameworkGradingRequest {
	t.Helper()
	assertions, frameworks, crosswalks := frameworkTestCatalogs(t)
	return evidence.FrameworkGradingRequest{
		ScopeID:     "scope-1",
		EvaluatedAt: time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC),
		Frameworks:  frameworks,
		Crosswalks:  crosswalks,
		Assertions:  assertions,
		AssertionAssessments: []*compliancev1.ControlAssertionAssessment{
			frameworkTestAssessment("G8E-GOV-BLOCK-001", "satisfied", "L3"),
			frameworkTestAssessment("G8E-GOV-ALLOW-001", "satisfied", "L3"),
		},
	}
}

func TestGradeFrameworkControls_SatisfiesControlWhenAllAssertionsSatisfied(t *testing.T) {
	request := frameworkGraderBaseRequest(t)
	assessments, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	assert.Len(t, assessments, 2)
	for _, assessment := range assessments {
		assert.Equal(t, "satisfied", assessment.GetStatus())
		assert.Equal(t, "L3", assessment.GetEvidenceLevel())
		assert.NotEmpty(t, assessment.GetMappingRefs())
		assert.NotEmpty(t, assessment.GetAssertionAssessmentRefs())
		assert.NoError(t, catalog.ValidateControlAssessment(assessment, "scope-1", request.Frameworks, request.Crosswalks))
	}
}

func TestGradeFrameworkControls_NotSatisfiedWhenAnyAssertionNotSatisfied(t *testing.T) {
	request := frameworkGraderBaseRequest(t)
	request.AssertionAssessments[0] = frameworkTestAssessment("G8E-GOV-BLOCK-001", "not_satisfied", "L3")
	assessments, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, assessments, 2)
	blockAssessment := findFrameworkAssessmentByControl(t, assessments, "KSI-MLA-07")
	assert.Equal(t, "not_satisfied", blockAssessment.GetStatus())
}

func TestGradeFrameworkControls_StatusPrecedenceMatrix(t *testing.T) {
	tests := []struct {
		name     string
		statuses []string
		expected string
	}{
		{name: "all satisfied", statuses: []string{"satisfied", "satisfied"}, expected: "satisfied"},
		{name: "all not applicable", statuses: []string{"not_applicable", "not_applicable"}, expected: "not_applicable"},
		{name: "not satisfied precedes customer attestation", statuses: []string{"customer_attestation_required", "not_satisfied"}, expected: "not_satisfied"},
		{name: "not satisfied precedes unverifiable", statuses: []string{"unverifiable", "not_satisfied"}, expected: "not_satisfied"},
		{name: "customer attestation precedes unverifiable", statuses: []string{"unverifiable", "customer_attestation_required"}, expected: "customer_attestation_required"},
		{name: "unverifiable precedes satisfied", statuses: []string{"satisfied", "unverifiable"}, expected: "unverifiable"},
		{name: "satisfied and not applicable is satisfied", statuses: []string{"not_applicable", "satisfied"}, expected: "satisfied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := frameworkGraderBaseRequest(t)
			request.Crosswalks.Mappings[0].AssertionRefs = append(request.Crosswalks.Mappings[0].AssertionRefs, &compliancev1.VersionedReference{Id: "G8E-GOV-ALLOW-001", Version: "1.0.0"})
			request.AssertionAssessments[0] = frameworkTestAssessment("G8E-GOV-BLOCK-001", tt.statuses[0], "L3")
			request.AssertionAssessments[1] = frameworkTestAssessment("G8E-GOV-ALLOW-001", tt.statuses[1], "L3")
			assessments, err := evidence.GradeFrameworkControls(context.Background(), request)
			require.NoError(t, err)
			assessment := findFrameworkAssessmentByControl(t, assessments, "KSI-MLA-07")
			assert.Equal(t, tt.expected, assessment.GetStatus())
		})
	}
}

func TestGradeFrameworkControls_UnverifiableWhenAssertionAssessmentMissing(t *testing.T) {
	request := frameworkGraderBaseRequest(t)
	request.AssertionAssessments = request.AssertionAssessments[:1]
	assessments, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, assessments, 2)
	allowAssessment := findFrameworkAssessmentByControl(t, assessments, "KSI-IAM-05")
	assert.Equal(t, "unverifiable", allowAssessment.GetStatus())
	assert.NotEmpty(t, allowAssessment.GetLimitations())
}

func TestGradeFrameworkControls_CustomerAttestationRequiredWhenAssertionRequiresIt(t *testing.T) {
	assertions, frameworks, crosswalks := frameworkTestCatalogs(t)
	frameworks.Frameworks[0].Controls[0].Responsibility = "customer"
	crosswalks.Mappings[0].Responsibility = "customer"
	require.NoError(t, catalog.ValidateCatalogSet(assertions, frameworks, crosswalks))
	request := evidence.FrameworkGradingRequest{
		ScopeID:     "scope-1",
		EvaluatedAt: time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC),
		Frameworks:  frameworks,
		Crosswalks:  crosswalks,
		Assertions:  assertions,
		AssertionAssessments: []*compliancev1.ControlAssertionAssessment{
			frameworkTestAssessment("G8E-GOV-BLOCK-001", "customer_attestation_required", "L0"),
			frameworkTestAssessment("G8E-GOV-ALLOW-001", "satisfied", "L3"),
		},
	}
	assessments, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	blockAssessment := findFrameworkAssessmentByControl(t, assessments, "KSI-MLA-07")
	assert.Equal(t, "customer_attestation_required", blockAssessment.GetStatus())
}

func TestGradeFrameworkControls_NotApplicableWhenAllMappingsNotApplicable(t *testing.T) {
	assertions := frameworkTestAssertionCatalog("G8E-GOV-BLOCK-001")
	framework := frameworkTestFramework("fedramp-20x", "CR26-2026-06-24",
		frameworkTestControl("KSI-MLA-07", "shared"),
	)
	frameworks := &compliancev1.FrameworkCatalog{CatalogId: "test", CatalogVersion: "1.0.0", Sha256: frameworkTestSHA256, Frameworks: []*compliancev1.FrameworkDefinition{framework}}
	crosswalks := &compliancev1.ControlCrosswalkCatalog{
		CatalogId:      "test",
		CatalogVersion: "1.0.0",
		Sha256:         frameworkTestSHA256,
		Mappings: []*compliancev1.ControlCrosswalk{
			frameworkTestCrosswalk("xwalk-1", "fedramp-20x", "CR26-2026-06-24", "KSI-MLA-07", "not_applicable", "shared", "L3",
				&compliancev1.VersionedReference{Id: "G8E-GOV-BLOCK-001", Version: "1.0.0"}),
		},
	}
	require.NoError(t, catalog.ValidateCatalogSet(assertions, frameworks, crosswalks))
	request := evidence.FrameworkGradingRequest{
		ScopeID:              "scope-1",
		EvaluatedAt:          time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC),
		Frameworks:           frameworks,
		Crosswalks:           crosswalks,
		Assertions:           assertions,
		AssertionAssessments: []*compliancev1.ControlAssertionAssessment{frameworkTestAssessment("G8E-GOV-BLOCK-001", "satisfied", "L3")},
	}
	assessments, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	assert.Empty(t, assessments)
}

func TestGradeFrameworkControls_UnverifiableWhenAllAssertionsUnverifiable(t *testing.T) {
	request := frameworkGraderBaseRequest(t)
	request.AssertionAssessments[0] = frameworkTestAssessment("G8E-GOV-BLOCK-001", "unverifiable", "L0")
	request.AssertionAssessments[1] = frameworkTestAssessment("G8E-GOV-ALLOW-001", "unverifiable", "L0")
	assessments, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, assessments, 2)
	for _, assessment := range assessments {
		assert.Equal(t, "unverifiable", assessment.GetStatus())
		assert.Equal(t, "L0", assessment.GetEvidenceLevel())
	}
}

func TestGradeFrameworkControls_EvidenceLevelIsMinimumAcrossAssertions(t *testing.T) {
	assertions, frameworks, crosswalks := frameworkTestCatalogs(t)
	crosswalks.Mappings[0].AssertionRefs = append(crosswalks.Mappings[0].AssertionRefs,
		&compliancev1.VersionedReference{Id: "G8E-GOV-ALLOW-001", Version: "1.0.0"})
	require.NoError(t, catalog.ValidateCatalogSet(assertions, frameworks, crosswalks))
	request := evidence.FrameworkGradingRequest{
		ScopeID:     "scope-1",
		EvaluatedAt: time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC),
		Frameworks:  frameworks,
		Crosswalks:  crosswalks,
		Assertions:  assertions,
		AssertionAssessments: []*compliancev1.ControlAssertionAssessment{
			frameworkTestAssessment("G8E-GOV-BLOCK-001", "satisfied", "L4"),
			frameworkTestAssessment("G8E-GOV-ALLOW-001", "satisfied", "L3"),
		},
	}
	assessments, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	blockAssessment := findFrameworkAssessmentByControl(t, assessments, "KSI-MLA-07")
	assert.Equal(t, "satisfied", blockAssessment.GetStatus())
	assert.Equal(t, "L3", blockAssessment.GetEvidenceLevel())
	assert.Len(t, blockAssessment.GetAssertionAssessmentRefs(), 2)
}

func TestGradeFrameworkControls_ResponsibilityMatchesControl(t *testing.T) {
	request := frameworkGraderBaseRequest(t)
	assessments, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	for _, assessment := range assessments {
		assert.Equal(t, "shared", assessment.GetResponsibility())
	}
}

func TestGradeFrameworkControls_DeterministicAssessmentIDs(t *testing.T) {
	request := frameworkGraderBaseRequest(t)
	assessments1, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	assessments2, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	for i := range assessments1 {
		assert.Equal(t, assessments1[i].GetAssessmentId(), assessments2[i].GetAssessmentId())
		assert.NotEmpty(t, assessments1[i].GetAssessmentId())
	}
}

func TestGradeFrameworkControls_RejectsIncompleteRequests(t *testing.T) {
	valid := frameworkGraderBaseRequest(t)
	tests := []struct {
		name   string
		mutate func(*evidence.FrameworkGradingRequest)
	}{
		{name: "missing scope", mutate: func(r *evidence.FrameworkGradingRequest) { r.ScopeID = "" }},
		{name: "zero evaluated at", mutate: func(r *evidence.FrameworkGradingRequest) { r.EvaluatedAt = time.Time{} }},
		{name: "missing frameworks", mutate: func(r *evidence.FrameworkGradingRequest) { r.Frameworks = nil }},
		{name: "missing crosswalks", mutate: func(r *evidence.FrameworkGradingRequest) { r.Crosswalks = nil }},
		{name: "missing assertions", mutate: func(r *evidence.FrameworkGradingRequest) { r.Assertions = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := valid
			tt.mutate(&request)
			_, err := evidence.GradeFrameworkControls(context.Background(), request)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestGradeFrameworkControls_RejectsAssertionAssessmentFromAnotherScope(t *testing.T) {
	request := frameworkGraderBaseRequest(t)
	request.AssertionAssessments[0].ScopeId = "scope-2"
	_, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceScopeMismatch)
}

func TestGradeFrameworkControls_RejectsNilAssertionAssessment(t *testing.T) {
	request := frameworkGraderBaseRequest(t)
	request.AssertionAssessments = append(request.AssertionAssessments, nil)
	_, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestGradeFrameworkControls_RejectsDuplicateAssertionAssessments(t *testing.T) {
	request := frameworkGraderBaseRequest(t)
	request.AssertionAssessments = append(request.AssertionAssessments, request.AssertionAssessments[0])
	_, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceDuplicateID)
}

func TestGradeFrameworkControls_RejectsMalformedAssertionAssessment(t *testing.T) {
	request := frameworkGraderBaseRequest(t)
	request.AssertionAssessments[0].Status = "unknown"
	_, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestGradeFrameworkControls_StopsOnCancellation(t *testing.T) {
	request := frameworkGraderBaseRequest(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := evidence.GradeFrameworkControls(ctx, request)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestGradeFrameworkControls_SkipsUnsupportedControls(t *testing.T) {
	assertions, frameworks, crosswalks := frameworkTestCatalogs(t)
	frameworks.Frameworks[0].Controls = append(frameworks.Frameworks[0].Controls,
		&compliancev1.FrameworkControlDefinition{
			ControlId:        "AC-99",
			Title:            "Unsupported Control",
			Description:      "This control is not mapped.",
			SourceReference:  "TEST",
			SupportStatus:    "unsupported",
			SupportRationale: "No crosswalk mapping exists.",
			Responsibility:   "shared",
		})
	require.NoError(t, catalog.ValidateCatalogSet(assertions, frameworks, crosswalks))
	base := frameworkGraderBaseRequest(t)
	request := evidence.FrameworkGradingRequest{
		ScopeID:              "scope-1",
		EvaluatedAt:          time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC),
		Frameworks:           frameworks,
		Crosswalks:           crosswalks,
		Assertions:           assertions,
		AssertionAssessments: base.AssertionAssessments,
	}
	assessments, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	assert.Len(t, assessments, 2)
	for _, assessment := range assessments {
		assert.NotEqual(t, "AC-99", assessment.GetControlId())
	}
}

func TestGradeFrameworkControls_EmitsLimitationsForSupportingMappings(t *testing.T) {
	request := frameworkGraderBaseRequest(t)
	assessments, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	for _, assessment := range assessments {
		assert.NotEmpty(t, assessment.GetLimitations())
		found := false
		for _, lim := range assessment.GetLimitations() {
			if strings.Contains(lim, "supporting") {
				found = true
				break
			}
		}
		assert.True(t, found, "expected supporting mapping limitation")
	}
}

func TestGradeFrameworkControls_EmitsAssessmentForEveryMappedControl(t *testing.T) {
	assertions, frameworks, crosswalks, err := catalog.LoadCanonicalCatalogs()
	require.NoError(t, err)
	assertionAssessments := make([]*compliancev1.ControlAssertionAssessment, 0, len(assertions.GetAssertions()))
	for _, assertion := range assertions.GetAssertions() {
		assertionAssessments = append(assertionAssessments, &compliancev1.ControlAssertionAssessment{
			AssessmentId:    "assertion-assessment:sha256:" + assertion.GetAssertionId() + "0000000000000000000000000000000000000000000000000000",
			ScopeId:         "scope-1",
			AssertionRef:    &compliancev1.VersionedReference{Id: assertion.GetAssertionId(), Version: assertion.GetAssertionVersion()},
			Status:          "unverifiable",
			EvidenceLevel:   "L0",
			EvaluatedAt:     timestamppb.New(time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)),
			VerifierRef:     &compliancev1.VersionedReference{Id: constants.AssertionGraderID, Version: constants.AssertionGraderVersion},
			FreshnessStatus: "incomplete",
			FailureReason:   "required evidence is unavailable",
			Limitations:     []string{},
		})
	}
	request := evidence.FrameworkGradingRequest{
		ScopeID:              "scope-1",
		EvaluatedAt:          time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC),
		Frameworks:           frameworks,
		Crosswalks:           crosswalks,
		Assertions:           assertions,
		AssertionAssessments: assertionAssessments,
	}
	assessments, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	mappedControlCount := 0
	for _, framework := range frameworks.GetFrameworks() {
		for _, control := range framework.GetControls() {
			if control.GetSupportStatus() == "mapped" {
				mappedControlCount++
			}
		}
	}
	assert.Len(t, assessments, mappedControlCount)
	for _, assessment := range assessments {
		assert.NotEqual(t, "satisfied", assessment.GetStatus())
		assert.NoError(t, catalog.ValidateControlAssessment(assessment, "scope-1", frameworks, crosswalks))
	}
}

func TestGradeFrameworkControls_MultipleFrameworks(t *testing.T) {
	assertions := frameworkTestAssertionCatalog("G8E-GOV-BLOCK-001")
	framework1 := frameworkTestFramework("fedramp-20x", "CR26-2026-06-24", frameworkTestControl("KSI-MLA-07", "shared"))
	framework2 := frameworkTestFramework("nist-sp-800-53", "rev5", frameworkTestControl("AU-12", "shared"))
	frameworks := &compliancev1.FrameworkCatalog{CatalogId: "test", CatalogVersion: "1.0.0", Sha256: frameworkTestSHA256, Frameworks: []*compliancev1.FrameworkDefinition{framework1, framework2}}
	crosswalks := &compliancev1.ControlCrosswalkCatalog{
		CatalogId:      "test",
		CatalogVersion: "1.0.0",
		Sha256:         frameworkTestSHA256,
		Mappings: []*compliancev1.ControlCrosswalk{
			frameworkTestCrosswalk("xwalk-1", "fedramp-20x", "CR26-2026-06-24", "KSI-MLA-07", "supporting", "shared", "L3",
				&compliancev1.VersionedReference{Id: "G8E-GOV-BLOCK-001", Version: "1.0.0"}),
			frameworkTestCrosswalk("xwalk-2", "nist-sp-800-53", "rev5", "AU-12", "supporting", "shared", "L3",
				&compliancev1.VersionedReference{Id: "G8E-GOV-BLOCK-001", Version: "1.0.0"}),
		},
	}
	require.NoError(t, catalog.ValidateCatalogSet(assertions, frameworks, crosswalks))
	request := evidence.FrameworkGradingRequest{
		ScopeID:              "scope-1",
		EvaluatedAt:          time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC),
		Frameworks:           frameworks,
		Crosswalks:           crosswalks,
		Assertions:           assertions,
		AssertionAssessments: []*compliancev1.ControlAssertionAssessment{frameworkTestAssessment("G8E-GOV-BLOCK-001", "satisfied", "L3")},
	}
	assessments, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	assert.Len(t, assessments, 2)
	frameworkIDs := make(map[string]bool)
	for _, assessment := range assessments {
		frameworkIDs[assessment.GetFrameworkRef().GetId()] = true
		assert.NoError(t, catalog.ValidateControlAssessment(assessment, "scope-1", frameworks, crosswalks))
	}
	assert.True(t, frameworkIDs["fedramp-20x"])
	assert.True(t, frameworkIDs["nist-sp-800-53"])
}

func TestGradeFrameworkControls_MultipleMappingsPerControl(t *testing.T) {
	assertions := frameworkTestAssertionCatalog("G8E-GOV-BLOCK-001", "G8E-GOV-ALLOW-001")
	framework := frameworkTestFramework("fedramp-20x", "CR26-2026-06-24", frameworkTestControl("KSI-MLA-07", "shared"))
	frameworks := &compliancev1.FrameworkCatalog{CatalogId: "test", CatalogVersion: "1.0.0", Sha256: frameworkTestSHA256, Frameworks: []*compliancev1.FrameworkDefinition{framework}}
	crosswalks := &compliancev1.ControlCrosswalkCatalog{
		CatalogId:      "test",
		CatalogVersion: "1.0.0",
		Sha256:         frameworkTestSHA256,
		Mappings: []*compliancev1.ControlCrosswalk{
			frameworkTestCrosswalk("xwalk-1", "fedramp-20x", "CR26-2026-06-24", "KSI-MLA-07", "supporting", "shared", "L3",
				&compliancev1.VersionedReference{Id: "G8E-GOV-BLOCK-001", Version: "1.0.0"}),
			frameworkTestCrosswalk("xwalk-2", "fedramp-20x", "CR26-2026-06-24", "KSI-MLA-07", "supporting", "shared", "L3",
				&compliancev1.VersionedReference{Id: "G8E-GOV-ALLOW-001", Version: "1.0.0"}),
		},
	}
	require.NoError(t, catalog.ValidateCatalogSet(assertions, frameworks, crosswalks))
	request := evidence.FrameworkGradingRequest{
		ScopeID:     "scope-1",
		EvaluatedAt: time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC),
		Frameworks:  frameworks,
		Crosswalks:  crosswalks,
		Assertions:  assertions,
		AssertionAssessments: []*compliancev1.ControlAssertionAssessment{
			frameworkTestAssessment("G8E-GOV-BLOCK-001", "satisfied", "L4"),
			frameworkTestAssessment("G8E-GOV-ALLOW-001", "not_satisfied", "L3"),
		},
	}
	assessments, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, assessments, 1)
	assert.Equal(t, "not_satisfied", assessments[0].GetStatus())
	assert.Len(t, assessments[0].GetMappingRefs(), 2)
	assert.Len(t, assessments[0].GetAssertionAssessmentRefs(), 2)
}

func TestGradeFrameworkControls_PartialFailureDoesNotHideNotSatisfied(t *testing.T) {
	request := frameworkGraderBaseRequest(t)
	request.AssertionAssessments[0] = frameworkTestAssessment("G8E-GOV-BLOCK-001", "satisfied", "L3")
	request.AssertionAssessments[1] = frameworkTestAssessment("G8E-GOV-ALLOW-001", "not_satisfied", "L3")
	assessments, err := evidence.GradeFrameworkControls(context.Background(), request)
	require.NoError(t, err)
	for _, assessment := range assessments {
		if assessment.GetControlId() == "KSI-IAM-05" {
			assert.Equal(t, "not_satisfied", assessment.GetStatus())
		} else {
			assert.Equal(t, "satisfied", assessment.GetStatus())
		}
	}
}

func findFrameworkAssessmentByControl(t *testing.T, assessments []*compliancev1.FrameworkControlAssessment, controlID string) *compliancev1.FrameworkControlAssessment {
	t.Helper()
	for _, assessment := range assessments {
		if assessment.GetControlId() == controlID {
			return assessment
		}
	}
	t.Fatalf("assessment for control %s not found", controlID)
	return nil
}
