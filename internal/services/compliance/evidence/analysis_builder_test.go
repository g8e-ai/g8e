// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version  2.0.

package evidence_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

const analysisTestSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

var analysisTestWindowStart = time.Date(2026, time.September, 6, 0, 0, 0, 0, time.UTC)
var analysisTestWindowEnd = time.Date(2026, time.September, 6, 23, 59, 59, 0, time.UTC)
var analysisTestEvaluatedAt = time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC)

// analysisTestAssertion builds a single-assertion catalog with the given
// category and responsibility so the builder can derive gap responsibility
// and finding severity from the assertion definition.
func analysisTestAssertion(id, category, responsibility string) *compliancev1.ControlAssertionDefinition {
	return &compliancev1.ControlAssertionDefinition{
		AssertionId:             id,
		AssertionVersion:        "1.0.0",
		Title:                   "Test assertion " + id,
		Statement:               "Test statement for " + id,
		Category:                category,
		ComponentScope:          []string{"gateway"},
		Responsibility:          responsibility,
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

func analysisTestAssertionCatalog(assertions ...*compliancev1.ControlAssertionDefinition) *compliancev1.ControlAssertionCatalog {
	return &compliancev1.ControlAssertionCatalog{
		CatalogId:      "analysis-builder-test-assertions",
		CatalogVersion: "1.0.0",
		Sha256:         analysisTestSHA256,
		Assertions:     assertions,
	}
}

func analysisTestFramework(frameworkID, frameworkVersion string, controls ...*compliancev1.FrameworkControlDefinition) *compliancev1.FrameworkDefinition {
	return &compliancev1.FrameworkDefinition{
		FrameworkId:      frameworkID,
		FrameworkVersion: frameworkVersion,
		Title:            "Test Framework",
		Publisher:        "Test Publisher",
		Source:           "https://example.com/framework",
		CatalogSha256:    analysisTestSHA256,
		EffectiveDate:    "2026-06-24",
		Controls:         controls,
	}
}

func analysisTestControl(controlID, responsibility string) *compliancev1.FrameworkControlDefinition {
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

func analysisTestCrosswalk(crosswalkID, frameworkID, frameworkVersion, controlID, mappingType, responsibility, evidenceLevel string, assertionRefs ...*compliancev1.VersionedReference) *compliancev1.ControlCrosswalk {
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

// analysisTestCatalogs builds a valid, mutually consistent catalog set with
// two assertions (one governance, one privacy) mapped to two framework
// controls with shared responsibility.
func analysisTestCatalogs(t *testing.T) (*compliancev1.ControlAssertionCatalog, *compliancev1.FrameworkCatalog, *compliancev1.ControlCrosswalkCatalog) {
	t.Helper()
	assertions := analysisTestAssertionCatalog(
		analysisTestAssertion("G8E-GOV-BLOCK-001", "governance", "platform"),
		analysisTestAssertion("G8E-PRIV-001", "privacy", "shared"),
	)
	framework := analysisTestFramework("fedramp-20x", "CR26-2026-06-24",
		analysisTestControl("KSI-MLA-07", "shared"),
		analysisTestControl("KSI-PRIV-01", "shared"),
	)
	frameworks := &compliancev1.FrameworkCatalog{
		CatalogId:      "analysis-builder-test-frameworks",
		CatalogVersion: "1.0.0",
		Sha256:         analysisTestSHA256,
		Frameworks:     []*compliancev1.FrameworkDefinition{framework},
	}
	crosswalks := &compliancev1.ControlCrosswalkCatalog{
		CatalogId:      "analysis-builder-test-crosswalks",
		CatalogVersion: "1.0.0",
		Sha256:         analysisTestSHA256,
		Mappings: []*compliancev1.ControlCrosswalk{
			analysisTestCrosswalk("fedramp-20x:KSI-MLA-07:G8E-GOV-BLOCK-001", "fedramp-20x", "CR26-2026-06-24", "KSI-MLA-07", "full", "shared", "L3",
				&compliancev1.VersionedReference{Id: "G8E-GOV-BLOCK-001", Version: "1.0.0"}),
			analysisTestCrosswalk("fedramp-20x:KSI-PRIV-01:G8E-PRIV-001", "fedramp-20x", "CR26-2026-06-24", "KSI-PRIV-01", "full", "shared", "L3",
				&compliancev1.VersionedReference{Id: "G8E-PRIV-001", Version: "1.0.0"}),
		},
	}
	require.NoError(t, catalog.ValidateCatalogSet(assertions, frameworks, crosswalks))
	return assertions, frameworks, crosswalks
}

// analysisTestAssertionAssessment builds a typed assertion assessment with
// the given status, freshness, and optional failure reason and limitations.
func analysisTestAssertionAssessment(assertionID, status, freshness, failureReason string, limitations ...string) *compliancev1.ControlAssertionAssessment {
	return &compliancev1.ControlAssertionAssessment{
		AssessmentId:    "assertion-assessment:sha256:" + assertionID + "00000000000000000000000000000000000000000000000000000000000",
		ScopeId:         "scope-1",
		AssertionRef:    &compliancev1.VersionedReference{Id: assertionID, Version: "1.0.0"},
		Status:          status,
		EvidenceLevel:   "L3",
		EvaluatedAt:     timestamppb.New(analysisTestEvaluatedAt),
		VerifierRef:     &compliancev1.VersionedReference{Id: constants.AssertionGraderID, Version: constants.AssertionGraderVersion},
		EvidenceRefs:    []string{"evidence-1"},
		FreshnessStatus: freshness,
		FailureReason:   failureReason,
		Limitations:     limitations,
	}
}

// analysisTestFrameworkAssessment builds a typed framework control assessment
// directly, bypassing the framework grader so the builder test exercises the
// aggregation path independently of the grading path.
func analysisTestFrameworkAssessment(frameworkID, frameworkVersion, controlID, status, responsibility string, limitations ...string) *compliancev1.FrameworkControlAssessment {
	return &compliancev1.FrameworkControlAssessment{
		AssessmentId:   "framework-assessment:sha256:" + controlID + "000000000000000000000000000000000000000000000000000000000",
		ScopeId:        "scope-1",
		FrameworkRef:   &compliancev1.VersionedReference{Id: frameworkID, Version: frameworkVersion},
		ControlId:      controlID,
		Status:         status,
		Responsibility: responsibility,
		EvidenceLevel:  "L3",
		MappingRefs:    []string{"xwalk-1"},
		Limitations:    limitations,
	}
}

// analysisTestGraph builds a small evidence graph with two nodes in scope-1
// where the metric node references the receipt node, so the builder can
// derive evidence links from graph references.
func analysisTestGraph(t *testing.T) *evidence.EvidenceGraph {
	t.Helper()
	graph := evidence.NewEvidenceGraph(0, nil)
	receiptBody := []byte(`{"receipt":"verified"}`)
	receiptDigest := sha256.Sum256(receiptBody)
	receiptID := evidence.ContentAddress(evidence.ArtifactTypeActionReceipt, receiptBody)
	require.NoError(t, graph.AddNode(evidence.EvidenceNode{
		ArtifactID:         receiptID,
		ArtifactType:       evidence.ArtifactTypeActionReceipt,
		SHA256:             hex.EncodeToString(receiptDigest[:]),
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          "g8e.operator.v1.ActionReceipt",
		ProducerIdentity:   "gateway",
		ProducedAt:         analysisTestEvaluatedAt.Add(-time.Hour),
		ScopeID:            "scope-1",
		RunID:              "run-1",
		VerificationStatus: evidence.VerificationStatusVerified,
		VerifierID:         "test-verifier",
		VerifierVersion:    "1.0.0",
		VerifiedAt:         analysisTestEvaluatedAt.Add(-time.Hour),
		BundlePath:         constants.EvalRunReceiptsFilename,
		CanonicalBytes:     receiptBody,
		References:         []string{},
	}))
	metricBody := []byte(`{"metric_id":"policy_outcome","metric_version":"1.0.0","value":1,"eligible":true,"verification_status":"verified"}`)
	metricDigest := sha256.Sum256(metricBody)
	metricID := evidence.ContentAddress(evidence.ArtifactTypeEvalMetric, metricBody)
	require.NoError(t, graph.AddNode(evidence.EvidenceNode{
		ArtifactID:         metricID,
		ArtifactType:       evidence.ArtifactTypeEvalMetric,
		SHA256:             hex.EncodeToString(metricDigest[:]),
		MediaType:          constants.MediaTypeJSON,
		SchemaRef:          "g8e_evals.schema.MetricObservation",
		ProducerIdentity:   "policy_outcome@1.0.0",
		ProducedAt:         analysisTestEvaluatedAt.Add(-time.Hour),
		ScopeID:            "scope-1",
		RunID:              "run-1",
		VerificationStatus: evidence.VerificationStatusVerified,
		VerifierID:         "test-verifier",
		VerifierVersion:    "1.0.0",
		VerifiedAt:         analysisTestEvaluatedAt.Add(-time.Hour),
		BundlePath:         constants.EvalRunMetricsFilename,
		CanonicalBytes:     metricBody,
		References:         []string{receiptID},
	}))
	return graph
}

// analysisBaseRequest builds a complete AnalysisRequest with all-satisfied
// assessments so the builder produces a complete evidence window.
func analysisBaseRequest(t *testing.T) evidence.AnalysisRequest {
	t.Helper()
	assertions, frameworks, crosswalks := analysisTestCatalogs(t)
	return evidence.AnalysisRequest{
		ScopeID:     "scope-1",
		WindowStart: analysisTestWindowStart,
		WindowEnd:   analysisTestWindowEnd,
		EvaluatedAt: analysisTestEvaluatedAt,
		Graph:       analysisTestGraph(t),
		Assertions:  assertions,
		Frameworks:  frameworks,
		Crosswalks:  crosswalks,
		AssertionAssessments: []*compliancev1.ControlAssertionAssessment{
			analysisTestAssertionAssessment("G8E-GOV-BLOCK-001", "satisfied", "fresh", ""),
			analysisTestAssertionAssessment("G8E-PRIV-001", "satisfied", "fresh", ""),
		},
		FrameworkAssessments: []*compliancev1.FrameworkControlAssessment{
			analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "KSI-MLA-07", "satisfied", "shared"),
			analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "KSI-PRIV-01", "satisfied", "shared"),
		},
	}
}

func TestBuildComplianceAnalysis_StampsGeneratorIdentityAndSchemaVersion(t *testing.T) {
	request := analysisBaseRequest(t)
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, constants.AnalysisBuilderID, analysis.GetGeneratorIdentity())
	assert.Equal(t, constants.AnalysisBuilderVersion, analysis.GetGeneratorVersion())
	assert.Equal(t, constants.AnalysisSchemaVersion, analysis.GetAnalysisSchemaVersion())
	assert.Equal(t, "scope-1", analysis.GetScopeRef())
	assert.Equal(t, analysisTestEvaluatedAt.UTC(), analysis.GetGeneratedAt().AsTime().UTC())
}

func TestBuildComplianceAnalysis_EvidenceWindowCompleteWhenAllSatisfied(t *testing.T) {
	request := analysisBaseRequest(t)
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	completeness := analysis.GetEvidenceWindowCompleteness()
	assert.Equal(t, "scope-1", completeness.GetScopeId())
	assert.Equal(t, int32(2), completeness.GetExpectedEvidenceCount())
	assert.Equal(t, int32(2), completeness.GetActualEvidenceCount())
	assert.Equal(t, "complete", completeness.GetCompletenessStatus())
	assert.Empty(t, completeness.GetMissingEvidenceRefs())
	assert.Empty(t, completeness.GetStaleEvidenceRefs())
}

func TestBuildComplianceAnalysis_EvidenceWindowPartialWhenSomeSatisfied(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[1] = analysisTestAssertionAssessment("G8E-PRIV-001", "not_satisfied", "fresh", "missing evidence")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	completeness := analysis.GetEvidenceWindowCompleteness()
	assert.Equal(t, int32(2), completeness.GetExpectedEvidenceCount())
	assert.Equal(t, int32(1), completeness.GetActualEvidenceCount())
	assert.Equal(t, "partial", completeness.GetCompletenessStatus())
}

func TestBuildComplianceAnalysis_EvidenceWindowEmptyWhenNoneSatisfied(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[0] = analysisTestAssertionAssessment("G8E-GOV-BLOCK-001", "not_satisfied", "fresh", "missing evidence")
	request.AssertionAssessments[1] = analysisTestAssertionAssessment("G8E-PRIV-001", "unverifiable", "fresh", "")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	completeness := analysis.GetEvidenceWindowCompleteness()
	assert.Equal(t, int32(0), completeness.GetActualEvidenceCount())
	assert.Equal(t, "empty", completeness.GetCompletenessStatus())
}

func TestBuildComplianceAnalysis_RecordsMissingAndStaleEvidenceRefs(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[0] = analysisTestAssertionAssessment("G8E-GOV-BLOCK-001", "satisfied", "incomplete", "")
	request.AssertionAssessments[1] = analysisTestAssertionAssessment("G8E-PRIV-001", "satisfied", "stale", "")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	completeness := analysis.GetEvidenceWindowCompleteness()
	assert.NotEmpty(t, completeness.GetMissingEvidenceRefs())
	assert.NotEmpty(t, completeness.GetStaleEvidenceRefs())
}

func TestBuildComplianceAnalysis_DerivesEvidenceLinksFromGraphReferences(t *testing.T) {
	request := analysisBaseRequest(t)
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	links := analysis.GetEvidenceLinks()
	require.Len(t, links, 1)
	assert.Equal(t, "references", links[0].GetLinkType())
	assert.NotEmpty(t, links[0].GetSourceRef())
	assert.NotEmpty(t, links[0].GetTargetRef())
	assert.NotEqual(t, links[0].GetSourceRef(), links[0].GetTargetRef())
}

func TestBuildComplianceAnalysis_EmbedsSortedTypedEvidenceResources(t *testing.T) {
	request := analysisBaseRequest(t)
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	resources := analysis.GetEvidenceResources()
	require.Len(t, resources, request.Graph.NodeCount())
	for i, resource := range resources {
		assert.Equal(t, request.ScopeID, resource.GetScopeId())
		assert.NotEmpty(t, resource.GetArtifactId())
		assert.NotEmpty(t, resource.GetArtifactType())
		assert.NotEmpty(t, resource.GetSha256())
		assert.NotEmpty(t, resource.GetBundlePath())
		assert.NotEmpty(t, resource.GetProducerIdentity())
		assert.Equal(t, string(evidence.VerificationStatusVerified), resource.GetVerificationStatus())
		if i > 0 {
			assert.Less(t, resources[i-1].GetArtifactId(), resource.GetArtifactId())
		}
	}
}

func TestBuildComplianceAnalysis_EmitsGapsForNotSatisfiedAssertions(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[0] = analysisTestAssertionAssessment("G8E-GOV-BLOCK-001", "not_satisfied", "fresh", "missing evidence")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	gap := findAnalysisGapByAssertion(t, analysis.GetGaps(), request.AssertionAssessments[0].GetAssessmentId())
	assert.Equal(t, "not_satisfied", gap.GetGapType())
	assert.Equal(t, "platform", gap.GetResponsibility())
	assert.Contains(t, gap.GetDescription(), "G8E-GOV-BLOCK-001")
}

func TestBuildComplianceAnalysis_EmitsGapsForNotSatisfiedFrameworkControls(t *testing.T) {
	request := analysisBaseRequest(t)
	request.FrameworkAssessments[0] = analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "KSI-MLA-07", "not_satisfied", "shared")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	gap := findAnalysisGapByControl(t, analysis.GetGaps(), "KSI-MLA-07")
	assert.Equal(t, "not_satisfied", gap.GetGapType())
	assert.Equal(t, "shared", gap.GetResponsibility())
	assert.Contains(t, gap.GetDescription(), "KSI-MLA-07")
}

func TestBuildComplianceAnalysis_SkipsGapsForSatisfiedAndNotApplicable(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[0] = analysisTestAssertionAssessment("G8E-GOV-BLOCK-001", "not_applicable", "fresh", "")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	for _, gap := range analysis.GetGaps() {
		assert.NotEqual(t, request.AssertionAssessments[0].GetAssessmentId(), gap.GetAssertionRef())
	}
}

func TestBuildComplianceAnalysis_DeduplicatesAndSortsLimitations(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[0] = analysisTestAssertionAssessment("G8E-GOV-BLOCK-001", "satisfied", "fresh", "", "limitation-b", "limitation-a")
	request.AssertionAssessments[1] = analysisTestAssertionAssessment("G8E-PRIV-001", "satisfied", "fresh", "", "limitation-a", "limitation-c")
	request.FrameworkAssessments[0] = analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "KSI-MLA-07", "satisfied", "shared", "limitation-c")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	limitations := analysis.GetLimitations()
	assert.Equal(t, []string{"limitation-a", "limitation-b", "limitation-c"}, limitations)
}

func TestBuildComplianceAnalysis_EmitsFindingsForNotSatisfiedAssertionsWithGovernanceSeverity(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[0] = analysisTestAssertionAssessment("G8E-GOV-BLOCK-001", "not_satisfied", "fresh", "missing evidence")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	finding := findAnalysisFindingByAssertion(t, analysis.GetFindings(), request.AssertionAssessments[0].GetAssessmentId())
	assert.Equal(t, "high", finding.GetSeverity())
	assert.Contains(t, finding.GetDescription(), "G8E-GOV-BLOCK-001")
}

func TestBuildComplianceAnalysis_EmitsFindingsForNotSatisfiedAssertionsWithNonGovernanceSeverity(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[1] = analysisTestAssertionAssessment("G8E-PRIV-001", "not_satisfied", "fresh", "missing evidence")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	finding := findAnalysisFindingByAssertion(t, analysis.GetFindings(), request.AssertionAssessments[1].GetAssessmentId())
	assert.Equal(t, "medium", finding.GetSeverity())
}

func TestBuildComplianceAnalysis_EmitsFindingsForNotSatisfiedFrameworkControls(t *testing.T) {
	request := analysisBaseRequest(t)
	request.FrameworkAssessments[0] = analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "KSI-MLA-07", "not_satisfied", "shared")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	finding := findAnalysisFindingByControl(t, analysis.GetFindings(), "KSI-MLA-07")
	assert.Equal(t, "medium", finding.GetSeverity())
	assert.Equal(t, "KSI-MLA-07", finding.GetRelatedControlId())
}

func TestBuildComplianceAnalysis_DerivesRemediationFromFindings(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[0] = analysisTestAssertionAssessment("G8E-GOV-BLOCK-001", "not_satisfied", "fresh", "missing evidence")
	request.FrameworkAssessments[1] = analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "KSI-PRIV-01", "not_satisfied", "shared")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	remediation := analysis.GetRemediation()
	require.Len(t, remediation, 2)
	priorities := make(map[string]string)
	owners := make(map[string]string)
	for _, r := range remediation {
		priorities[r.GetFindingRef()] = r.GetPriority()
		owners[r.GetFindingRef()] = r.GetOwner()
		assert.NotEmpty(t, r.GetAction())
	}
	highSeverityFinding := findAnalysisFindingByAssertion(t, analysis.GetFindings(), request.AssertionAssessments[0].GetAssessmentId())
	assert.Equal(t, "high", priorities[highSeverityFinding.GetFindingId()])
	assert.Equal(t, "platform", owners[highSeverityFinding.GetFindingId()])
	controlFinding := findAnalysisFindingByControl(t, analysis.GetFindings(), "KSI-PRIV-01")
	assert.Equal(t, "customer", owners[controlFinding.GetFindingId()])
}

func TestBuildComplianceAnalysis_GroupsSectionsByResponsibility(t *testing.T) {
	request := analysisBaseRequest(t)
	request.FrameworkAssessments[0] = analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "KSI-MLA-07", "satisfied", "platform")
	request.FrameworkAssessments[1] = analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "KSI-PRIV-01", "satisfied", "customer")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	sections := analysis.GetSections()
	responsibilities := make(map[string]*compliancev1.ControlSection)
	for _, section := range sections {
		responsibilities[section.GetResponsibility()] = section
	}
	assert.Contains(t, responsibilities, "platform")
	assert.Contains(t, responsibilities, "customer")
	assert.Equal(t, "Platform Controls", responsibilities["platform"].GetTitle())
	assert.Equal(t, "Customer Controls", responsibilities["customer"].GetTitle())
	assert.Len(t, responsibilities["platform"].GetControlAssessmentRefs(), 1)
	assert.Len(t, responsibilities["customer"].GetControlAssessmentRefs(), 1)
}

func TestBuildComplianceAnalysis_EmitsExplicitResponsibilityAndStatusSections(t *testing.T) {
	request := analysisBaseRequest(t)
	framework := request.Frameworks.GetFrameworks()[0]
	framework.Controls = append(framework.Controls,
		analysisTestControl("PLATFORM-01", "platform"),
		analysisTestControl("CUSTOMER-01", "customer"),
		analysisTestControl("INHERITED-01", "inherited"),
		analysisTestControl("ASSESSOR-01", "assessor"),
		analysisTestControl("PLANNED-01", "platform"),
		analysisTestControl("UNSUPPORTED-01", "shared"),
	)
	framework.Controls[len(framework.Controls)-2].SupportStatus = "planned"
	framework.Controls[len(framework.Controls)-2].SupportRationale = "A future catalog version maps this control."
	framework.Controls[len(framework.Controls)-1].SupportStatus = "unsupported"
	framework.Controls[len(framework.Controls)-1].SupportRationale = "No reviewed mapping exists."

	shared := request.FrameworkAssessments[0]
	platform := analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "PLATFORM-01", "not_satisfied", "platform")
	customer := analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "CUSTOMER-01", "unverifiable", "customer")
	inherited := analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "INHERITED-01", "satisfied", "inherited")
	inherited.AssertionAssessmentRefs = []string{request.AssertionAssessments[0].GetAssessmentId()}
	request.AssertionAssessments[0].FreshnessStatus = "stale"
	assessor := analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "ASSESSOR-01", "customer_attestation_required", "assessor")
	request.FrameworkAssessments = []*compliancev1.FrameworkControlAssessment{shared, platform, customer, inherited, assessor}

	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	sections := make(map[string]*compliancev1.ControlSection)
	for _, section := range analysis.GetSections() {
		key := section.GetStatusFilter()
		if key == "" {
			key = section.GetResponsibility()
		}
		sections[key] = section
	}

	require.Len(t, sections, 10)
	for _, key := range []string{"platform", "customer", "shared", "inherited", "assessor_required", "planned", "unsupported", "failed", "stale", "unverifiable"} {
		assert.Contains(t, sections, key)
	}
	assert.Equal(t, []string{platform.GetAssessmentId()}, sections["platform"].GetControlAssessmentRefs())
	assert.Equal(t, []string{customer.GetAssessmentId()}, sections["customer"].GetControlAssessmentRefs())
	assert.Equal(t, []string{shared.GetAssessmentId()}, sections["shared"].GetControlAssessmentRefs())
	assert.Equal(t, []string{inherited.GetAssessmentId()}, sections["inherited"].GetControlAssessmentRefs())
	assert.Equal(t, []string{assessor.GetAssessmentId()}, sections["assessor_required"].GetControlAssessmentRefs())
	assert.Equal(t, []string{platform.GetAssessmentId()}, sections["failed"].GetControlAssessmentRefs())
	assert.Equal(t, []string{inherited.GetAssessmentId()}, sections["stale"].GetControlAssessmentRefs())
	assert.Equal(t, []string{customer.GetAssessmentId()}, sections["unverifiable"].GetControlAssessmentRefs())
	assert.Empty(t, sections["planned"].GetControlAssessmentRefs())
	assert.Empty(t, sections["unsupported"].GetControlAssessmentRefs())
	require.Len(t, sections["planned"].GetControlRefs(), 1)
	assert.Equal(t, "PLANNED-01", sections["planned"].GetControlRefs()[0].GetControlId())
	require.Len(t, sections["unsupported"].GetControlRefs(), 1)
	assert.Equal(t, "UNSUPPORTED-01", sections["unsupported"].GetControlRefs()[0].GetControlId())
}

func TestBuildComplianceAnalysis_SortsSectionsBySectionID(t *testing.T) {
	request := analysisBaseRequest(t)
	request.FrameworkAssessments[0] = analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "KSI-MLA-07", "satisfied", "customer")
	request.FrameworkAssessments[1] = analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "KSI-PRIV-01", "satisfied", "platform")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	sections := analysis.GetSections()
	for i := 1; i < len(sections); i++ {
		assert.Less(t, sections[i-1].GetSectionId(), sections[i].GetSectionId())
	}
}

func TestBuildComplianceAnalysis_PropagatesGraphFailuresAndValidity(t *testing.T) {
	request := analysisBaseRequest(t)
	request.Graph = evidence.NewEvidenceGraph(0, nil)
	require.NoError(t, request.Graph.AddNode(evidence.EvidenceNode{
		ArtifactID:         evidence.ContentAddress(evidence.ArtifactTypeActionReceipt, []byte(`{}`)),
		ArtifactType:       evidence.ArtifactTypeActionReceipt,
		SHA256:             "0000000000000000000000000000000000000000000000000000000000000000",
		MediaType:          constants.MediaTypeJSON,
		ProducerIdentity:   "gateway",
		ProducedAt:         analysisTestEvaluatedAt,
		ScopeID:            "scope-1",
		RunID:              "run-1",
		VerificationStatus: evidence.VerificationStatusVerified,
		CanonicalBytes:     []byte(`{}`),
		References:         []string{"missing-ref"},
	}))
	request.Graph.ResolveReferences()
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	assert.False(t, analysis.GetEvidenceGraphValid())
	assert.NotEmpty(t, analysis.GetEvidenceGraphFailures())
}

func TestBuildComplianceAnalysis_ReportsValidGraphWhenNoFailures(t *testing.T) {
	request := analysisBaseRequest(t)
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	assert.True(t, analysis.GetEvidenceGraphValid())
	assert.Empty(t, analysis.GetEvidenceGraphFailures())
}

func TestBuildComplianceAnalysis_DeterministicAnalysisID(t *testing.T) {
	request := analysisBaseRequest(t)
	analysis1, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	analysis2, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	assert.NotEmpty(t, analysis1.GetAnalysisId())
	assert.True(t, strings.HasPrefix(analysis1.GetAnalysisId(), "compliance-analysis:sha256:"))
	assert.Equal(t, analysis1.GetAnalysisId(), analysis2.GetAnalysisId())
}

func TestBuildComplianceAnalysis_DifferentInputsProduceDifferentIDs(t *testing.T) {
	request := analysisBaseRequest(t)
	analysis1, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	request.EvaluatedAt = analysisTestEvaluatedAt.Add(time.Second)
	analysis2, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	assert.NotEqual(t, analysis1.GetAnalysisId(), analysis2.GetAnalysisId())
}

func TestBuildComplianceAnalysis_SortsAssertionAndFrameworkAssessments(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[0], request.AssertionAssessments[1] = request.AssertionAssessments[1], request.AssertionAssessments[0]
	request.FrameworkAssessments[0], request.FrameworkAssessments[1] = request.FrameworkAssessments[1], request.FrameworkAssessments[0]
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	assert.True(t, isSortedByAssessmentID(analysis.GetAssertionAssessments()))
	assert.True(t, isSortedByFrameworkAssessmentID(analysis.GetFrameworkAssessments()))
}

func TestBuildComplianceAnalysis_RejectsIncompleteRequests(t *testing.T) {
	valid := analysisBaseRequest(t)
	tests := []struct {
		name   string
		mutate func(*evidence.AnalysisRequest)
	}{
		{name: "missing scope", mutate: func(r *evidence.AnalysisRequest) { r.ScopeID = "" }},
		{name: "nil graph", mutate: func(r *evidence.AnalysisRequest) { r.Graph = nil }},
		{name: "nil assertions", mutate: func(r *evidence.AnalysisRequest) { r.Assertions = nil }},
		{name: "nil frameworks", mutate: func(r *evidence.AnalysisRequest) { r.Frameworks = nil }},
		{name: "nil crosswalks", mutate: func(r *evidence.AnalysisRequest) { r.Crosswalks = nil }},
		{name: "zero window start", mutate: func(r *evidence.AnalysisRequest) { r.WindowStart = time.Time{} }},
		{name: "zero window end", mutate: func(r *evidence.AnalysisRequest) { r.WindowEnd = time.Time{} }},
		{name: "zero evaluated at", mutate: func(r *evidence.AnalysisRequest) { r.EvaluatedAt = time.Time{} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := valid
			tt.mutate(&request)
			_, err := evidence.BuildComplianceAnalysis(context.Background(), request)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestBuildComplianceAnalysis_RejectsInvertedEvidenceWindow(t *testing.T) {
	request := analysisBaseRequest(t)
	request.WindowStart = analysisTestWindowEnd
	request.WindowEnd = analysisTestWindowStart
	_, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestBuildComplianceAnalysis_RejectsEvaluatedAtOutsideWindow(t *testing.T) {
	request := analysisBaseRequest(t)
	request.EvaluatedAt = analysisTestWindowEnd.Add(time.Hour)
	_, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestBuildComplianceAnalysis_RejectsInvalidAssertionCatalog(t *testing.T) {
	request := analysisBaseRequest(t)
	request.Assertions = &compliancev1.ControlAssertionCatalog{
		CatalogId:      "bad",
		CatalogVersion: "1.0.0",
		Sha256:         analysisTestSHA256,
		Assertions:     []*compliancev1.ControlAssertionDefinition{{AssertionId: "", AssertionVersion: "1.0.0"}},
	}
	_, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.Error(t, err)
}

func TestBuildComplianceAnalysis_RejectsInvalidFrameworkCatalog(t *testing.T) {
	request := analysisBaseRequest(t)
	request.Frameworks = &compliancev1.FrameworkCatalog{
		CatalogId:      "bad",
		CatalogVersion: "1.0.0",
		Sha256:         analysisTestSHA256,
		Frameworks:     []*compliancev1.FrameworkDefinition{{FrameworkId: "", FrameworkVersion: "1.0.0"}},
	}
	_, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.Error(t, err)
}

func TestBuildComplianceAnalysis_RejectsAssertionAssessmentFromAnotherScope(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[0].ScopeId = "scope-2"
	_, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceScopeMismatch)
}

func TestBuildComplianceAnalysis_RejectsFrameworkAssessmentFromAnotherScope(t *testing.T) {
	request := analysisBaseRequest(t)
	request.FrameworkAssessments[0].ScopeId = "scope-2"
	_, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvidenceScopeMismatch)
}

func TestBuildComplianceAnalysis_RejectsNilAssertionAssessment(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments = append(request.AssertionAssessments, nil)
	_, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestBuildComplianceAnalysis_RejectsNilFrameworkAssessment(t *testing.T) {
	request := analysisBaseRequest(t)
	request.FrameworkAssessments = append(request.FrameworkAssessments, nil)
	_, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestBuildComplianceAnalysis_RejectsAssertionAssessmentWithNilAssertionRef(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[0].AssertionRef = nil
	_, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestBuildComplianceAnalysis_RejectsFrameworkAssessmentWithNilFrameworkRef(t *testing.T) {
	request := analysisBaseRequest(t)
	request.FrameworkAssessments[0].FrameworkRef = nil
	_, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
}

func TestBuildComplianceAnalysis_StopsOnCancellation(t *testing.T) {
	request := analysisBaseRequest(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := evidence.BuildComplianceAnalysis(ctx, request)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestBuildComplianceAnalysis_AggregatesRealGraderOutput(t *testing.T) {
	assertions, frameworks, crosswalks := analysisTestCatalogs(t)
	graph := analysisTestGraph(t)
	assertionAssessments, err := evidence.GradeControlAssertions(context.Background(), evidence.AssertionGradingRequest{
		ScopeID:     "scope-1",
		WindowStart: analysisTestWindowStart,
		WindowEnd:   analysisTestWindowEnd,
		EvaluatedAt: analysisTestEvaluatedAt,
		Assertions:  assertions,
		Graph:       graph,
	})
	require.NoError(t, err)
	frameworkAssessments, err := evidence.GradeFrameworkControls(context.Background(), evidence.FrameworkGradingRequest{
		ScopeID:              "scope-1",
		EvaluatedAt:          analysisTestEvaluatedAt,
		Frameworks:           frameworks,
		Crosswalks:           crosswalks,
		Assertions:           assertions,
		AssertionAssessments: assertionAssessments,
	})
	require.NoError(t, err)
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), evidence.AnalysisRequest{
		ScopeID:              "scope-1",
		WindowStart:          analysisTestWindowStart,
		WindowEnd:            analysisTestWindowEnd,
		EvaluatedAt:          analysisTestEvaluatedAt,
		Graph:                graph,
		Assertions:           assertions,
		Frameworks:           frameworks,
		Crosswalks:           crosswalks,
		AssertionAssessments: assertionAssessments,
		FrameworkAssessments: frameworkAssessments,
	})
	require.NoError(t, err)
	assert.Equal(t, "scope-1", analysis.GetScopeRef())
	assert.Len(t, analysis.GetAssertionAssessments(), len(assertionAssessments))
	assert.Len(t, analysis.GetFrameworkAssessments(), len(frameworkAssessments))
	for _, assessment := range analysis.GetAssertionAssessments() {
		assert.NoError(t, catalog.ValidateAssertionAssessment(assessment, "scope-1", assertions))
	}
	for _, assessment := range analysis.GetFrameworkAssessments() {
		assert.NoError(t, catalog.ValidateControlAssessment(assessment, "scope-1", frameworks, crosswalks))
	}
}

func TestBuildComplianceAnalysis_GapIDsAreDeterministicAndContentAddressed(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[0] = analysisTestAssertionAssessment("G8E-GOV-BLOCK-001", "not_satisfied", "fresh", "missing evidence")
	analysis1, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	analysis2, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	require.NotEmpty(t, analysis1.GetGaps())
	for i := range analysis1.GetGaps() {
		assert.True(t, strings.HasPrefix(analysis1.GetGaps()[i].GetGapId(), "compliance-gap:sha256:"))
		assert.Equal(t, analysis1.GetGaps()[i].GetGapId(), analysis2.GetGaps()[i].GetGapId())
	}
}

func TestBuildComplianceAnalysis_FindingAndRemediationIDsAreContentAddressed(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[0] = analysisTestAssertionAssessment("G8E-GOV-BLOCK-001", "not_satisfied", "fresh", "missing evidence")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	require.NotEmpty(t, analysis.GetFindings())
	for _, finding := range analysis.GetFindings() {
		assert.True(t, strings.HasPrefix(finding.GetFindingId(), "compliance-finding:sha256:"))
	}
	require.NotEmpty(t, analysis.GetRemediation())
	for _, r := range analysis.GetRemediation() {
		assert.True(t, strings.HasPrefix(r.GetRemediationId(), "compliance-remediation:sha256:"))
	}
}

func TestBuildComplianceAnalysis_SortsGapsFindingsAndRemediation(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[0] = analysisTestAssertionAssessment("G8E-GOV-BLOCK-001", "not_satisfied", "fresh", "missing evidence")
	request.AssertionAssessments[1] = analysisTestAssertionAssessment("G8E-PRIV-001", "not_satisfied", "fresh", "missing evidence")
	request.FrameworkAssessments[0] = analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "KSI-MLA-07", "not_satisfied", "shared")
	request.FrameworkAssessments[1] = analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "KSI-PRIV-01", "not_satisfied", "shared")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	assert.True(t, isSortedByGapID(analysis.GetGaps()))
	assert.True(t, isSortedByFindingID(analysis.GetFindings()))
}

func TestBuildComplianceAnalysis_SortsEvidenceLinksBySourceThenTarget(t *testing.T) {
	request := analysisBaseRequest(t)
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	links := analysis.GetEvidenceLinks()
	for i := 1; i < len(links); i++ {
		if links[i-1].GetSourceRef() == links[i].GetSourceRef() {
			assert.LessOrEqual(t, links[i-1].GetTargetRef(), links[i].GetTargetRef())
		} else {
			assert.Less(t, links[i-1].GetSourceRef(), links[i].GetSourceRef())
		}
	}
}

func TestBuildComplianceAnalysis_SectionIDsAreContentAddressed(t *testing.T) {
	request := analysisBaseRequest(t)
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	for _, section := range analysis.GetSections() {
		assert.True(t, strings.HasPrefix(section.GetSectionId(), "control-section:sha256:"))
	}
}

func TestBuildComplianceAnalysis_DoesNotMutateInputs(t *testing.T) {
	request := analysisBaseRequest(t)
	assertionOrderBefore := assessmentIDs(request.AssertionAssessments)
	frameworkOrderBefore := frameworkAssessmentIDs(request.FrameworkAssessments)
	_, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, assertionOrderBefore, assessmentIDs(request.AssertionAssessments))
	assert.Equal(t, frameworkOrderBefore, frameworkAssessmentIDs(request.FrameworkAssessments))
}

func TestBuildComplianceAnalysis_EmitsNoFindingsWhenAllSatisfied(t *testing.T) {
	request := analysisBaseRequest(t)
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	assert.Empty(t, analysis.GetFindings())
	assert.Empty(t, analysis.GetRemediation())
}

func TestBuildComplianceAnalysis_EmitsNoGapsWhenAllSatisfied(t *testing.T) {
	request := analysisBaseRequest(t)
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	assert.Empty(t, analysis.GetGaps())
}

func TestBuildComplianceAnalysis_EmitsGapsForUnverifiableAssertions(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[0] = analysisTestAssertionAssessment("G8E-GOV-BLOCK-001", "unverifiable", "incomplete", "")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	gap := findAnalysisGapByAssertion(t, analysis.GetGaps(), request.AssertionAssessments[0].GetAssessmentId())
	assert.Equal(t, "unverifiable", gap.GetGapType())
}

func TestBuildComplianceAnalysis_EmitsGapsForCustomerAttestationRequired(t *testing.T) {
	request := analysisBaseRequest(t)
	request.FrameworkAssessments[0] = analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "KSI-MLA-07", "customer_attestation_required", "shared")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	gap := findAnalysisGapByControl(t, analysis.GetGaps(), "KSI-MLA-07")
	assert.Equal(t, "customer_attestation_required", gap.GetGapType())
}

func TestBuildComplianceAnalysis_EmitsRemediationOwnerCustomerForControlFindings(t *testing.T) {
	request := analysisBaseRequest(t)
	request.FrameworkAssessments[0] = analysisTestFrameworkAssessment("fedramp-20x", "CR26-2026-06-24", "KSI-MLA-07", "not_satisfied", "shared")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, analysis.GetRemediation(), 1)
	assert.Equal(t, "customer", analysis.GetRemediation()[0].GetOwner())
	assert.Contains(t, analysis.GetRemediation()[0].GetAction(), "KSI-MLA-07")
}

func TestBuildComplianceAnalysis_EmitsRemediationOwnerPlatformForAssertionFindings(t *testing.T) {
	request := analysisBaseRequest(t)
	request.AssertionAssessments[0] = analysisTestAssertionAssessment("G8E-GOV-BLOCK-001", "not_satisfied", "fresh", "missing evidence")
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, analysis.GetRemediation(), 1)
	assert.Equal(t, "platform", analysis.GetRemediation()[0].GetOwner())
	assert.Contains(t, analysis.GetRemediation()[0].GetAction(), "assertion")
}

func TestBuildComplianceAnalysis_EvidenceLinksOnlyForScopeNodes(t *testing.T) {
	request := analysisBaseRequest(t)
	otherBody := []byte(`{"other":"scope"}`)
	otherDigest := sha256.Sum256(otherBody)
	otherID := evidence.ContentAddress(evidence.ArtifactTypeActionReceipt, otherBody)
	require.NoError(t, request.Graph.AddNode(evidence.EvidenceNode{
		ArtifactID:         otherID,
		ArtifactType:       evidence.ArtifactTypeActionReceipt,
		SHA256:             hex.EncodeToString(otherDigest[:]),
		MediaType:          constants.MediaTypeJSON,
		ProducerIdentity:   "gateway",
		ProducedAt:         analysisTestEvaluatedAt,
		ScopeID:            "scope-2",
		RunID:              "run-2",
		VerificationStatus: evidence.VerificationStatusVerified,
		CanonicalBytes:     otherBody,
		References:         []string{},
	}))
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	for _, link := range analysis.GetEvidenceLinks() {
		assert.NotEqual(t, otherID, link.GetSourceRef())
	}
}

func TestBuildComplianceAnalysis_EmptyAssessmentsPreserveExplicitCatalogSections(t *testing.T) {
	assertions, frameworks, crosswalks := analysisTestCatalogs(t)
	request := evidence.AnalysisRequest{
		ScopeID:              "scope-1",
		WindowStart:          analysisTestWindowStart,
		WindowEnd:            analysisTestWindowEnd,
		EvaluatedAt:          analysisTestEvaluatedAt,
		Graph:                analysisTestGraph(t),
		Assertions:           assertions,
		Frameworks:           frameworks,
		Crosswalks:           crosswalks,
		AssertionAssessments: []*compliancev1.ControlAssertionAssessment{},
		FrameworkAssessments: []*compliancev1.FrameworkControlAssessment{},
	}
	analysis, err := evidence.BuildComplianceAnalysis(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, int32(0), analysis.GetEvidenceWindowCompleteness().GetExpectedEvidenceCount())
	assert.Equal(t, "empty", analysis.GetEvidenceWindowCompleteness().GetCompletenessStatus())
	assert.Empty(t, analysis.GetGaps())
	assert.Empty(t, analysis.GetFindings())
	require.Len(t, analysis.GetSections(), 10)
	for _, section := range analysis.GetSections() {
		assert.Empty(t, section.GetControlAssessmentRefs())
	}
}

func findAnalysisGapByAssertion(t *testing.T, gaps []*compliancev1.ComplianceGap, assertionRef string) *compliancev1.ComplianceGap {
	t.Helper()
	for _, gap := range gaps {
		if gap.GetAssertionRef() == assertionRef {
			return gap
		}
	}
	t.Fatalf("gap for assertion %s not found", assertionRef)
	return nil
}

func findAnalysisGapByControl(t *testing.T, gaps []*compliancev1.ComplianceGap, controlID string) *compliancev1.ComplianceGap {
	t.Helper()
	for _, gap := range gaps {
		if gap.GetControlId() == controlID {
			return gap
		}
	}
	t.Fatalf("gap for control %s not found", controlID)
	return nil
}

func findAnalysisFindingByAssertion(t *testing.T, findings []*compliancev1.ComplianceFinding, assertionRef string) *compliancev1.ComplianceFinding {
	t.Helper()
	for _, finding := range findings {
		if finding.GetRelatedAssertionRef() == assertionRef {
			return finding
		}
	}
	t.Fatalf("finding for assertion %s not found", assertionRef)
	return nil
}

func findAnalysisFindingByControl(t *testing.T, findings []*compliancev1.ComplianceFinding, controlID string) *compliancev1.ComplianceFinding {
	t.Helper()
	for _, finding := range findings {
		if finding.GetRelatedControlId() == controlID {
			return finding
		}
	}
	t.Fatalf("finding for control %s not found", controlID)
	return nil
}

func isSortedByAssessmentID(assessments []*compliancev1.ControlAssertionAssessment) bool {
	for i := 1; i < len(assessments); i++ {
		if assessments[i-1].GetAssessmentId() > assessments[i].GetAssessmentId() {
			return false
		}
	}
	return true
}

func isSortedByFrameworkAssessmentID(assessments []*compliancev1.FrameworkControlAssessment) bool {
	for i := 1; i < len(assessments); i++ {
		if assessments[i-1].GetAssessmentId() > assessments[i].GetAssessmentId() {
			return false
		}
	}
	return true
}

func isSortedByGapID(gaps []*compliancev1.ComplianceGap) bool {
	for i := 1; i < len(gaps); i++ {
		if gaps[i-1].GetGapId() > gaps[i].GetGapId() {
			return false
		}
	}
	return true
}

func isSortedByFindingID(findings []*compliancev1.ComplianceFinding) bool {
	for i := 1; i < len(findings); i++ {
		if findings[i-1].GetFindingId() > findings[i].GetFindingId() {
			return false
		}
	}
	return true
}

func assessmentIDs(assessments []*compliancev1.ControlAssertionAssessment) []string {
	ids := make([]string, 0, len(assessments))
	for _, a := range assessments {
		ids = append(ids, a.GetAssessmentId())
	}
	return ids
}

func frameworkAssessmentIDs(assessments []*compliancev1.FrameworkControlAssessment) []string {
	ids := make([]string, 0, len(assessments))
	for _, a := range assessments {
		ids = append(ids, a.GetAssessmentId())
	}
	return ids
}
