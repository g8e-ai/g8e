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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// oscalTestCatalog returns a minimal valid KSICatalog for OSCAL tests.
func oscalTestCatalog() *KSICatalog {
	return &KSICatalog{
		Version: "test-1.0",
		Source:  "test-source",
		KSIs: []KSI{
			{
				ID:                "KSI-CMT-01",
				Title:             "Logging Changes",
				Category:          KSICategoryCMT,
				ControlRefs:       []string{"AU-2", "CM-3"},
				ApplicableClasses: []CertificationClass{ClassB, ClassC},
				ValidationCycle:   ValidationCycleMachine,
				AutomatedMethods: []AutomatedMethod{
					{Name: "audit_events_check", Description: "Verifies audit events exist"},
					{Name: "ledger_commits_check", Description: "Verifies ledger commits exist"},
				},
			},
			{
				ID:                "KSI-MLA-07",
				Title:             "Non-Repudiation",
				Category:          KSICategoryMLA,
				ControlRefs:       []string{"AU-10"},
				ApplicableClasses: []CertificationClass{ClassC},
				ValidationCycle:   ValidationCycleMachine,
				AutomatedMethods: []AutomatedMethod{
					{Name: "commitment_chain_check", Description: "Verifies commitment chain"},
				},
			},
		},
	}
}

// oscalTestAnalysis returns canonical analysis with satisfied and not-satisfied control assessments.
func oscalTestAnalysis() *compliancev1.ComplianceAnalysis {
	const (
		receiptDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		metricDigest  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	receiptID := "action-receipt:sha256:" + receiptDigest
	metricID := "eval-metric:sha256:" + metricDigest
	generatedAt := time.Date(2026, time.September, 7, 12, 0, 0, 0, time.UTC)
	firstAssessmentID := "assertion-assessment:sha256:" + receiptDigest
	secondAssessmentID := "assertion-assessment:sha256:" + metricDigest
	return &compliancev1.ComplianceAnalysis{
		AnalysisId:            "compliance-analysis:sha256:" + receiptDigest,
		AnalysisSchemaVersion: constants.AnalysisSchemaVersion,
		ScopeRef:              "scope-1",
		GeneratedAt:           timestamppb.New(generatedAt),
		GeneratorIdentity:     constants.AnalysisBuilderID,
		GeneratorVersion:      constants.AnalysisBuilderVersion,
		AssertionAssessments: []*compliancev1.ControlAssertionAssessment{
			{
				AssessmentId: firstAssessmentID, ScopeId: "scope-1", AssertionRef: &compliancev1.VersionedReference{Id: "G8E-GOV-BLOCK-001", Version: "1.0.0"}, Status: "satisfied", EvidenceLevel: "L3", EvaluatedAt: timestamppb.New(generatedAt), VerifierRef: &compliancev1.VersionedReference{Id: constants.AssertionGraderID, Version: constants.AssertionGraderVersion}, EvidenceRefs: []string{receiptID}, FreshnessStatus: "fresh",
			},
			{
				AssessmentId: secondAssessmentID, ScopeId: "scope-1", AssertionRef: &compliancev1.VersionedReference{Id: "G8E-CM-STATE-001", Version: "1.0.0"}, Status: "not_satisfied", EvidenceLevel: "L2", EvaluatedAt: timestamppb.New(generatedAt), VerifierRef: &compliancev1.VersionedReference{Id: constants.AssertionGraderID, Version: constants.AssertionGraderVersion}, EvidenceRefs: []string{metricID}, MetricRefs: []string{metricID}, FreshnessStatus: "fresh", FailureReason: "required deterministic grader did not pass",
			},
		},
		FrameworkAssessments: []*compliancev1.FrameworkControlAssessment{
			{AssessmentId: "framework-assessment:sha256:" + receiptDigest, ScopeId: "scope-1", FrameworkRef: &compliancev1.VersionedReference{Id: "fedramp-20x", Version: "CR26-2026-06-24"}, ControlId: "KSI-CMT-01", Status: "satisfied", Responsibility: "platform", AssertionAssessmentRefs: []string{firstAssessmentID}, EvidenceLevel: "L3"},
			{AssessmentId: "framework-assessment:sha256:" + metricDigest, ScopeId: "scope-1", FrameworkRef: &compliancev1.VersionedReference{Id: "fedramp-20x", Version: "CR26-2026-06-24"}, ControlId: "KSI-MLA-07", Status: "not_satisfied", Responsibility: "shared", AssertionAssessmentRefs: []string{secondAssessmentID}, EvidenceLevel: "L2"},
		},
		EvidenceResources: []*compliancev1.ComplianceEvidenceReference{
			{ArtifactId: receiptID, ArtifactType: "action-receipt", Sha256: receiptDigest, MediaType: constants.MediaTypeJSON, SchemaRef: "g8e.operator.v1.ActionReceipt", ProducerIdentity: "gateway", ScopeId: "scope-1", VerificationStatus: "verified", VerifierId: constants.ReceiptEvidenceVerifierID, VerifierVersion: constants.ReceiptEvidenceVerifierVersion, BundlePath: constants.EvalRunReceiptsFilename},
			{ArtifactId: metricID, ArtifactType: "eval-metric", Sha256: metricDigest, MediaType: constants.MediaTypeJSON, SchemaRef: "g8e_evals.schema.MetricObservation", ProducerIdentity: "policy_outcome@1.0.0", ScopeId: "scope-1", VerificationStatus: "verified", VerifierId: constants.EvalRunVerifierID, VerifierVersion: constants.EvalRunVerifierVersion, BundlePath: constants.EvalRunMetricsFilename},
		},
		EvidenceGraphValid: true,
	}
}

// TestOSCALExporter_GenerateComponentDefinition asserts the component-definition
// struct fields, UUID format, metadata, component title/description, control
// implementations grouped by KSI category, and back-matter resource.
func TestOSCALExporter_GenerateComponentDefinition(t *testing.T) {
	cat := oscalTestCatalog()
	exporter := NewOSCALExporter(cat)

	compDef, err := exporter.GenerateComponentDefinition()
	require.NoError(t, err)
	require.NotNil(t, compDef)

	// UUID must be a deterministic RFC 4122 UUID v5.
	assert.NotEmpty(t, compDef.UUID)
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, compDef.UUID)

	// Metadata.
	assert.Equal(t, "g8e Platform Component Definition", compDef.Metadata.Title)
	assert.NotEmpty(t, compDef.Metadata.Published)
	assert.NotEmpty(t, compDef.Metadata.LastModified)
	assert.Equal(t, "test-1.0", compDef.Metadata.Version)
	assert.Equal(t, "1.1.2", compDef.Metadata.OscalVersion)

	// Component.
	require.Len(t, compDef.Components, 1)
	comp := compDef.Components[0]
	assert.NotEmpty(t, comp.UUID)
	assert.Equal(t, "software", comp.Type)
	assert.Contains(t, comp.Title, "g8e")
	assert.Contains(t, comp.Description, "zero-trust")

	// Control implementations: CMT and MLA are the two categories.
	require.Len(t, comp.ControlImplementations, 2)
	cmtImpl := comp.ControlImplementations[0]
	assert.Equal(t, "CMT", string(cat.KSIs[0].Category))
	assert.NotEmpty(t, cmtImpl.UUID)
	assert.Contains(t, cmtImpl.Description, "CMT")
	// KSI-CMT-01 has two control refs (AU-2, CM-3), so two implemented controls.
	require.Len(t, cmtImpl.ImplementedControls, 2)
	assert.Equal(t, "AU-2", cmtImpl.ImplementedControls[0].ControlID)
	assert.Equal(t, "CM-3", cmtImpl.ImplementedControls[1].ControlID)
	assert.Equal(t, "Logging Changes", cmtImpl.ImplementedControls[0].Description)
	assert.Equal(t, "Logging Changes", cmtImpl.ImplementedControls[1].Description)
	require.Len(t, cmtImpl.ImplementedControls[0].Statements, 2)
	require.Len(t, cmtImpl.ImplementedControls[1].Statements, 2)

	// Back-matter resource.
	require.Len(t, compDef.BackMatter.Resources, 1)
	res := compDef.BackMatter.Resources[0]
	assert.NotEmpty(t, res.UUID)
	assert.Equal(t, "FedRAMP 20x KSI Catalog", res.Title)
	require.Len(t, res.Props, 2)
	assert.Equal(t, "source", res.Props[0].Name)
	assert.Equal(t, "test-source", res.Props[0].Value)
	assert.Equal(t, "version", res.Props[1].Name)
	assert.Equal(t, "test-1.0", res.Props[1].Value)
}

// TestOSCALExporter_GenerateAssessmentResults asserts canonical assertion observations, framework findings, and evidence resources.
func TestOSCALExporter_GenerateAssessmentResults(t *testing.T) {
	analysis := oscalTestAnalysis()
	results, err := NewOSCALExporter(oscalTestCatalog()).GenerateAssessmentResults(analysis)
	require.NoError(t, err)
	require.NotNil(t, results)

	assert.NotEmpty(t, results.AssessmentResults.UUID)
	assert.Equal(t, "g8e Compliance Assessment Results", results.AssessmentResults.Metadata.Title)
	assert.Equal(t, constants.AnalysisSchemaVersion, results.AssessmentResults.Metadata.Version)
	assert.Equal(t, "1.1.2", results.AssessmentResults.Metadata.OscalVersion)
	assert.Equal(t, analysis.GetGeneratedAt().AsTime().UTC().Format(time.RFC3339), results.AssessmentResults.Metadata.Published)

	require.Len(t, results.AssessmentResults.Results, 1)
	result := results.AssessmentResults.Results[0]
	assert.Contains(t, result.Title, analysis.GetScopeRef())
	assert.NotEmpty(t, result.Start)
	require.Len(t, result.Observations, 2)
	assert.Contains(t, result.Observations[0].Title, "G8E-GOV-BLOCK-001")
	assert.Equal(t, "TEST", result.Observations[0].Methods[0])
	assert.Equal(t, constants.AssertionGraderID+"@"+constants.AssertionGraderVersion, oscalPropValue(result.Observations[0].Props, "g8e-verifier"))
	assert.Equal(t, analysis.GetAssertionAssessments()[0].GetAssessmentId(), result.Observations[0].Subjects[0].Title)
	require.Len(t, result.Findings, 2)
	assert.Equal(t, "satisfied", result.Findings[0].Target.Status.State)
	assert.Equal(t, "KSI-CMT-01", result.Findings[0].Target.TargetID)
	assert.Equal(t, "not-satisfied", result.Findings[1].Target.Status.State)
	assert.Equal(t, "KSI-MLA-07", result.Findings[1].Target.TargetID)
	require.Len(t, results.AssessmentResults.BackMatter.Resources, 2)
}

// TestOSCALExporter_NilCatalog asserts that component generation requires a catalog while analysis rendering does not.
func TestOSCALExporter_NilCatalog(t *testing.T) {
	exporter := NewOSCALExporter(nil)

	_, err := exporter.GenerateComponentDefinition()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)

	_, err = exporter.GenerateAssessmentResults(oscalTestAnalysis())
	require.NoError(t, err)
}

// TestOSCALExporter_NilAnalysis asserts that nil canonical analysis produces ErrValidationFailed.
func TestOSCALExporter_NilAnalysis(t *testing.T) {
	_, err := NewOSCALExporter(oscalTestCatalog()).GenerateAssessmentResults(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

// TestOSCALExporter_EmptyAnalysisAssessments asserts that analysis without assessments produces empty observations and findings.
func TestOSCALExporter_EmptyAnalysisAssessments(t *testing.T) {
	analysis := oscalTestAnalysis()
	analysis.AssertionAssessments = nil
	analysis.FrameworkAssessments = nil

	results, err := NewOSCALExporter(oscalTestCatalog()).GenerateAssessmentResults(analysis)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Nil(t, results)
}

// TestOSCALComponentDefinition_MarshalJSON verifies JSON serialization produces
// a valid OSCAL structure with expected top-level keys.
func TestOSCALComponentDefinition_MarshalJSON(t *testing.T) {
	exporter := NewOSCALExporter(oscalTestCatalog())

	compDef, err := exporter.GenerateComponentDefinition()
	require.NoError(t, err)

	data, err := json.Marshal(compDef)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Contains(t, raw, "uuid")
	assert.Contains(t, raw, "metadata")
	assert.Contains(t, raw, "components")
	assert.Contains(t, raw, "back-matter")

	// Verify metadata sub-keys.
	var meta map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw["metadata"], &meta))
	assert.Contains(t, meta, "title")
	assert.Contains(t, meta, "published")
	assert.Contains(t, meta, "last-modified")
	assert.Contains(t, meta, "version")
	assert.Contains(t, meta, "oscal-version")

	// Verify components is an array with at least one element.
	var components []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw["components"], &components))
	require.Len(t, components, 1)
	assert.Contains(t, components[0], "uuid")
	assert.Contains(t, components[0], "type")
	assert.Contains(t, components[0], "title")
	assert.Contains(t, components[0], "control-implementations")
}

// TestOSCALAssessmentResults_MarshalJSON verifies JSON serialization produces
// a valid OSCAL structure with expected top-level keys.
func TestOSCALAssessmentResults_MarshalJSON(t *testing.T) {
	exporter := NewOSCALExporter(oscalTestCatalog())

	results, err := exporter.GenerateAssessmentResults(oscalTestAnalysis())
	require.NoError(t, err)

	data, err := json.Marshal(results)
	require.NoError(t, err)

	var decoded OSCALAssessmentResults
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.NotEmpty(t, decoded.AssessmentResults.UUID)
	assert.NotEmpty(t, decoded.AssessmentResults.Metadata.Title)
	assert.NotEmpty(t, decoded.AssessmentResults.ImportAP.Href)
	require.Len(t, decoded.AssessmentResults.Results, 1)
	result := decoded.AssessmentResults.Results[0]
	assert.NotEmpty(t, result.UUID)
	assert.NotEmpty(t, result.Title)
	assert.NotEmpty(t, result.Start)
	assert.NotNil(t, result.ReviewedControls)
	assert.NotEmpty(t, result.Observations)
	assert.NotEmpty(t, result.Findings)
}

// TestOSCALExporter_GenerateAssessmentResults_MapsCanonicalStatuses verifies OSCAL-compatible framework finding statuses.
func TestOSCALExporter_GenerateAssessmentResults_MapsCanonicalStatuses(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected string
	}{
		{name: "satisfied", status: "satisfied", expected: "satisfied"},
		{name: "not satisfied", status: "not_satisfied", expected: "not-satisfied"},
		{name: "not applicable", status: "not_applicable", expected: "not-satisfied"},
		{name: "unverifiable", status: "unverifiable", expected: "not-satisfied"},
		{name: "customer attestation required", status: "customer_attestation_required", expected: "not-satisfied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := oscalTestAnalysis()
			analysis.FrameworkAssessments = analysis.FrameworkAssessments[:1]
			analysis.FrameworkAssessments[0].Status = tt.status
			results, err := NewOSCALExporter(nil).GenerateAssessmentResults(analysis)
			require.NoError(t, err)
			require.Len(t, results.AssessmentResults.Results[0].Findings, 1)
			assert.Equal(t, tt.expected, results.AssessmentResults.Results[0].Findings[0].Target.Status.State)
			assert.Contains(t, results.AssessmentResults.Results[0].Findings[0].Description, tt.status)
		})
	}
}

func TestOSCALExporter_GenerateAssessmentResults_RejectsInvalidGraphAndEvidenceReferences(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*compliancev1.ComplianceAnalysis)
		target error
	}{
		{name: "invalid graph", mutate: func(analysis *compliancev1.ComplianceAnalysis) { analysis.EvidenceGraphValid = false }, target: constants.ErrInvalidEvidenceGraph},
		{name: "missing evidence resource", mutate: func(analysis *compliancev1.ComplianceAnalysis) {
			analysis.EvidenceResources = analysis.EvidenceResources[:1]
		}, target: constants.ErrUnresolvedReference},
		{name: "digest mismatch", mutate: func(analysis *compliancev1.ComplianceAnalysis) {
			analysis.EvidenceResources[0].Sha256 = analysis.EvidenceResources[1].Sha256
		}, target: constants.ErrUnresolvedReference},
		{name: "cross scope evidence", mutate: func(analysis *compliancev1.ComplianceAnalysis) { analysis.EvidenceResources[0].ScopeId = "scope-2" }, target: constants.ErrEvidenceScopeMismatch},
		{name: "missing bundle path", mutate: func(analysis *compliancev1.ComplianceAnalysis) { analysis.EvidenceResources[0].BundlePath = "" }, target: constants.ErrUnresolvedReference},
		{name: "missing assertion assessment", mutate: func(analysis *compliancev1.ComplianceAnalysis) {
			analysis.FrameworkAssessments[0].AssertionAssessmentRefs = []string{"missing"}
		}, target: constants.ErrUnresolvedReference},
		{name: "missing evidence link target", mutate: func(analysis *compliancev1.ComplianceAnalysis) {
			analysis.EvidenceLinks = []*compliancev1.EvidenceLink{{SourceRef: analysis.EvidenceResources[0].ArtifactId, TargetRef: "missing", LinkType: "references"}}
		}, target: constants.ErrUnresolvedReference},
		{name: "missing evidence link source", mutate: func(analysis *compliancev1.ComplianceAnalysis) {
			analysis.EvidenceLinks = []*compliancev1.EvidenceLink{{SourceRef: "missing", TargetRef: analysis.EvidenceResources[0].ArtifactId, LinkType: "references"}}
		}, target: constants.ErrUnresolvedReference},
		{name: "unsupported evidence link type", mutate: func(analysis *compliancev1.ComplianceAnalysis) {
			analysis.EvidenceLinks = []*compliancev1.EvidenceLink{{SourceRef: analysis.EvidenceResources[0].ArtifactId, TargetRef: analysis.EvidenceResources[1].ArtifactId, LinkType: "derived-from"}}
		}, target: constants.ErrUnresolvedReference},
		{name: "duplicate evidence link", mutate: func(analysis *compliancev1.ComplianceAnalysis) {
			link := &compliancev1.EvidenceLink{SourceRef: analysis.EvidenceResources[0].ArtifactId, TargetRef: analysis.EvidenceResources[1].ArtifactId, LinkType: "references"}
			analysis.EvidenceLinks = []*compliancev1.EvidenceLink{link, link}
		}, target: constants.ErrEvidenceDuplicateID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := oscalTestAnalysis()
			tt.mutate(analysis)
			_, err := NewOSCALExporter(nil).GenerateAssessmentResults(analysis)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.target)
		})
	}
}

// TestOSCALExporter_GenerateComponentDefinition_DedupControlRefs verifies that
// when multiple KSIs in the same category reference the same control-id, the
// resulting OSCAL document has a single implemented-control entry with merged
// statements from all referencing KSIs. OSCAL 1.1.2 requires control-id to be
// unique within a control-implementation.
func TestOSCALExporter_GenerateComponentDefinition_DedupControlRefs(t *testing.T) {
	cat := &KSICatalog{
		Version: "test-1.0",
		Source:  "test",
		KSIs: []KSI{
			{
				ID:                "KSI-CMT-01",
				Title:             "Logging Changes",
				Category:          KSICategoryCMT,
				ControlRefs:       []string{"AU-2", "CM-3"},
				ApplicableClasses: []CertificationClass{ClassC},
				ValidationCycle:   ValidationCycleMachine,
				AutomatedMethods: []AutomatedMethod{
					{Name: "audit_events_check", Description: "Verifies audit events exist"},
				},
			},
			{
				ID:                "KSI-CMT-02",
				Title:             "Audit Review",
				Category:          KSICategoryCMT,
				ControlRefs:       []string{"AU-2"},
				ApplicableClasses: []CertificationClass{ClassC},
				ValidationCycle:   ValidationCycleMachine,
				AutomatedMethods: []AutomatedMethod{
					{Name: "ledger_commits_check", Description: "Verifies ledger commits exist"},
				},
			},
		},
	}
	exporter := NewOSCALExporter(cat)

	compDef, err := exporter.GenerateComponentDefinition()
	require.NoError(t, err)
	require.NotNil(t, compDef)

	require.Len(t, compDef.Components, 1)
	comp := compDef.Components[0]
	require.Len(t, comp.ControlImplementations, 1)
	cmtImpl := comp.ControlImplementations[0]

	// AU-2 should appear once, not twice, even though both KSIs reference it.
	// CM-3 appears once from KSI-CMT-01. Total: 2 unique control-ids.
	require.Len(t, cmtImpl.ImplementedControls, 2)

	// Find the AU-2 entry (could be at index 0 or 1 depending on iteration order).
	var au2 *OSCALImplementedControl
	for i := range cmtImpl.ImplementedControls {
		if cmtImpl.ImplementedControls[i].ControlID == "AU-2" {
			au2 = &cmtImpl.ImplementedControls[i]
			break
		}
	}
	require.NotNil(t, au2, "AU-2 implemented control must exist")

	// Merged description from both KSIs.
	assert.Contains(t, au2.Description, "Logging Changes")
	assert.Contains(t, au2.Description, "Audit Review")

	// Merged statements: 1 from KSI-CMT-01 + 1 from KSI-CMT-02 = 2 total.
	require.Len(t, au2.Statements, 2)
	statementIDs := []string{au2.Statements[0].StatementID, au2.Statements[1].StatementID}
	assert.Contains(t, statementIDs, "KSI-CMT-01:audit_events_check")
	assert.Contains(t, statementIDs, "KSI-CMT-02:ledger_commits_check")
}

// TestOSCALExporter_GenerateComponentDefinition_NoControlRefs asserts that a
// KSI with no control refs produces a validation error.
func TestOSCALExporter_GenerateComponentDefinition_NoControlRefs(t *testing.T) {
	cat := &KSICatalog{
		Version: "test-1.0",
		Source:  "test",
		KSIs: []KSI{
			{
				ID:                "KSI-BAD-01",
				Title:             "Bad KSI",
				Category:          KSICategoryCMT,
				ControlRefs:       []string{},
				ApplicableClasses: []CertificationClass{ClassC},
				ValidationCycle:   ValidationCycleMachine,
			},
		},
	}
	exporter := NewOSCALExporter(cat)

	_, err := exporter.GenerateComponentDefinition()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Contains(t, err.Error(), "no control refs")
}

// TestGenerateUUID_DeterministicV5 verifies stable identity-bound RFC 4122 UUID v5 generation.
func TestGenerateUUID_DeterministicV5(t *testing.T) {
	u1 := generateUUID("finding", "analysis-1", "control-1")
	u2 := generateUUID("finding", "analysis-1", "control-1")
	u3 := generateUUID("finding", "analysis-1", "control-2")

	assert.Equal(t, u1, u2)
	assert.Equal(t, "b4ef8d41-bebd-5517-aae7-9522a1963b1e", u1)
	assert.NotEqual(t, u1, u3)
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, u1)

	parsed, err := uuid.Parse(u1)
	require.NoError(t, err)
	assert.Equal(t, uuid.Version(5), parsed.Version())
}

func TestOSCALExporter_GenerateAssessmentResultsFromCanonicalAnalysis_ResolvesEvidenceResources(t *testing.T) {
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	artifactID := "action-receipt:sha256:" + digest
	generatedAt := time.Date(2026, time.September, 7, 12, 0, 0, 0, time.UTC)
	analysis := &compliancev1.ComplianceAnalysis{
		AnalysisId:            "compliance-analysis:sha256:" + digest,
		AnalysisSchemaVersion: constants.AnalysisSchemaVersion,
		ScopeRef:              "scope-1",
		GeneratedAt:           timestamppb.New(generatedAt),
		GeneratorIdentity:     constants.AnalysisBuilderID,
		GeneratorVersion:      constants.AnalysisBuilderVersion,
		AssertionAssessments: []*compliancev1.ControlAssertionAssessment{{
			AssessmentId:    "assertion-assessment:sha256:" + digest,
			ScopeId:         "scope-1",
			AssertionRef:    &compliancev1.VersionedReference{Id: "G8E-GOV-BLOCK-001", Version: "1.0.0"},
			Status:          "satisfied",
			EvidenceLevel:   "L3",
			EvaluatedAt:     timestamppb.New(generatedAt),
			VerifierRef:     &compliancev1.VersionedReference{Id: constants.AssertionGraderID, Version: constants.AssertionGraderVersion},
			EvidenceRefs:    []string{artifactID},
			FreshnessStatus: "fresh",
		}},
		FrameworkAssessments: []*compliancev1.FrameworkControlAssessment{{
			AssessmentId:            "framework-assessment:sha256:" + digest,
			ScopeId:                 "scope-1",
			FrameworkRef:            &compliancev1.VersionedReference{Id: "fedramp-20x", Version: "CR26-2026-06-24"},
			ControlId:               "KSI-MLA-07",
			Status:                  "satisfied",
			Responsibility:          "platform",
			AssertionAssessmentRefs: []string{"assertion-assessment:sha256:" + digest},
			EvidenceLevel:           "L3",
		}},
		EvidenceResources: []*compliancev1.ComplianceEvidenceReference{{
			ArtifactId:         artifactID,
			ArtifactType:       "action-receipt",
			Sha256:             digest,
			MediaType:          constants.MediaTypeJSON,
			SchemaRef:          "g8e.operator.v1.ActionReceipt",
			ProducerIdentity:   "gateway",
			ScopeId:            "scope-1",
			VerificationStatus: "verified",
			VerifierId:         constants.ReceiptEvidenceVerifierID,
			VerifierVersion:    constants.ReceiptEvidenceVerifierVersion,
			BundlePath:         constants.EvalRunReceiptsFilename,
		}},
		EvidenceGraphValid: true,
	}

	doc, err := NewOSCALExporter(oscalTestCatalog()).GenerateAssessmentResults(analysis)
	require.NoError(t, err)
	require.Len(t, doc.AssessmentResults.BackMatter.Resources, 1)
	resource := doc.AssessmentResults.BackMatter.Resources[0]
	assert.Equal(t, artifactID, oscalPropValue(resource.Props, "artifact-id"))
	assert.Equal(t, digest, oscalPropValue(resource.Props, "sha256"))
	require.Len(t, doc.AssessmentResults.Results, 1)
	require.Len(t, doc.AssessmentResults.Results[0].Observations, 1)
	require.Len(t, doc.AssessmentResults.Results[0].Observations[0].RelevantEvidence, 1)
	assert.Equal(t, "#"+resource.UUID, doc.AssessmentResults.Results[0].Observations[0].RelevantEvidence[0].Href)
}

func TestOSCALExporter_GenerateAssessmentResults_ResolvesEveryEvidenceLink(t *testing.T) {
	analysis := oscalTestAnalysis()
	analysis.EvidenceLinks = []*compliancev1.EvidenceLink{{SourceRef: analysis.EvidenceResources[0].ArtifactId, TargetRef: analysis.EvidenceResources[1].ArtifactId, LinkType: "references"}}
	document, err := NewOSCALExporter(oscalTestCatalog()).GenerateAssessmentResults(analysis)
	require.NoError(t, err)
	assert.Len(t, document.AssessmentResults.BackMatter.Resources, len(analysis.EvidenceResources))
}

func TestOSCALExporter_GenerateAssessmentResults_ConformsToV112RequiredShape(t *testing.T) {
	document, err := NewOSCALExporter(oscalTestCatalog()).GenerateAssessmentResults(oscalTestAnalysis())
	require.NoError(t, err)

	assessmentResults := document.AssessmentResults
	assert.NotEmpty(t, assessmentResults.UUID)
	assert.NotEmpty(t, assessmentResults.ImportAP.Href)
	require.Len(t, assessmentResults.Results, 1)
	result := assessmentResults.Results[0]
	require.NotNil(t, result.ReviewedControls)
	require.NotEmpty(t, result.ReviewedControls.ControlSelections)
	require.NotEmpty(t, result.Observations)
	assert.NotEmpty(t, result.Observations[0].Collected)
	require.NotEmpty(t, result.Findings)
	for _, finding := range result.Findings {
		assert.Equal(t, "objective-id", finding.Target.Type)
		assert.Contains(t, []string{"satisfied", "not-satisfied"}, finding.Target.Status.State)
	}

	body, err := json.Marshal(document)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"assessment-results"`)
	assert.Contains(t, string(body), `"reviewed-controls"`)
	assert.Contains(t, string(body), `"status":{"state":`)
}

func TestValidateOSCALAssessmentResults_RejectsSchemaViolations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*OSCALAssessmentResults)
		target error
	}{
		{name: "missing document", target: constants.ErrValidationFailed},
		{name: "invalid root UUID", mutate: func(document *OSCALAssessmentResults) { document.AssessmentResults.UUID = "invalid" }, target: constants.ErrValidationFailed},
		{name: "missing assessment plan import", mutate: func(document *OSCALAssessmentResults) { document.AssessmentResults.ImportAP.Href = "" }, target: constants.ErrValidationFailed},
		{name: "missing reviewed controls", mutate: func(document *OSCALAssessmentResults) { document.AssessmentResults.Results[0].ReviewedControls = nil }, target: constants.ErrValidationFailed},
		{name: "empty reviewed control selections", mutate: func(document *OSCALAssessmentResults) {
			document.AssessmentResults.Results[0].ReviewedControls.ControlSelections = nil
		}, target: constants.ErrValidationFailed},
		{name: "missing observation collection time", mutate: func(document *OSCALAssessmentResults) {
			document.AssessmentResults.Results[0].Observations[0].Collected = ""
		}, target: constants.ErrValidationFailed},
		{name: "invalid observation method", mutate: func(document *OSCALAssessmentResults) {
			document.AssessmentResults.Results[0].Observations[0].Methods = []string{"AUTOMATED"}
		}, target: constants.ErrValidationFailed},
		{name: "invalid finding status", mutate: func(document *OSCALAssessmentResults) {
			document.AssessmentResults.Results[0].Findings[0].Target.Status.State = "unknown"
		}, target: constants.ErrValidationFailed},
		{name: "unresolved relevant evidence", mutate: func(document *OSCALAssessmentResults) {
			document.AssessmentResults.Results[0].Observations[0].RelevantEvidence[0].Href = "#00000000-0000-5000-8000-000000000000"
		}, target: constants.ErrUnresolvedReference},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var document *OSCALAssessmentResults
			if tt.mutate != nil {
				var err error
				document, err = NewOSCALExporter(oscalTestCatalog()).GenerateAssessmentResults(oscalTestAnalysis())
				require.NoError(t, err)
				tt.mutate(document)
			}
			err := validateOSCALAssessmentResults(document)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.target)
		})
	}
}

func oscalPropValue(props []OSCALProp, name string) string {
	for _, prop := range props {
		if prop.Name == name {
			return prop.Value
		}
	}
	return ""
}
