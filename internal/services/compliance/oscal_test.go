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

	"github.com/g8e-ai/g8e/internal/constants"
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

// oscalTestResultSet returns a KSIResultSet with satisfied and not-satisfied results.
func oscalTestResultSet() *KSIResultSet {
	return &KSIResultSet{
		Class:         ClassC,
		EvaluatedAtMs: time.Now().UnixMilli(),
		Results: []KSIResult{
			{
				ID:                  "KSI-CMT-01",
				Status:              KSIStatusSatisfied,
				MethodCount:         2,
				LastValidatedUnixMs: time.Now().UnixMilli(),
				Evidence: []Evidence{
					{Type: EvidenceTypeExecutionID, Reference: "events:42", Description: "Audit events exist"},
					{Type: EvidenceTypeLedgerCommit, Reference: "commit-abc", Description: "Ledger commits exist"},
				},
			},
			{
				ID:                  "KSI-MLA-07",
				Status:              KSIStatusNotSatisfied,
				MethodCount:         1,
				LastValidatedUnixMs: time.Now().UnixMilli(),
				Evidence: []Evidence{
					{Type: EvidenceTypeLedgerCommit, Reference: "commit-def", Description: "Chain broken"},
				},
			},
		},
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

	// UUID must be non-empty and look like a UUID.
	assert.NotEmpty(t, compDef.UUID)
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, compDef.UUID)

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

// TestOSCALExporter_GenerateAssessmentResults asserts observations per KSI
// result, findings with satisfied/not-satisfied status, and evidence anchors
// from KSIResultSet.Results[].Evidence.
func TestOSCALExporter_GenerateAssessmentResults(t *testing.T) {
	cat := oscalTestCatalog()
	exporter := NewOSCALExporter(cat)
	rs := oscalTestResultSet()

	results, err := exporter.GenerateAssessmentResults(rs)
	require.NoError(t, err)
	require.NotNil(t, results)

	// Top-level fields.
	assert.NotEmpty(t, results.UUID)
	assert.Equal(t, "g8e KSI Assessment Results", results.Metadata.Title)
	assert.Equal(t, "test-1.0", results.Metadata.Version)
	assert.Equal(t, "1.1.2", results.Metadata.OscalVersion)

	require.Len(t, results.Results, 1)
	result := results.Results[0]
	assert.Contains(t, result.Title, "Class C")
	assert.NotEmpty(t, result.Start)

	// Observations: one per KSI result.
	require.Len(t, result.Observations, 2)
	obs0 := result.Observations[0]
	assert.NotEmpty(t, obs0.UUID)
	assert.Contains(t, obs0.Title, "KSI-CMT-01")
	assert.Equal(t, "Logging Changes", obs0.Description)
	require.Len(t, obs0.Methods, 1)
	assert.Equal(t, "TEST-AUTOMATED", obs0.Methods[0].MethodID)
	require.Len(t, obs0.Subjects, 1)
	assert.Equal(t, "assessment-target", obs0.Subjects[0].Type)
	assert.Equal(t, "KSI-CMT-01", obs0.Subjects[0].Title)

	// Evidence anchors from KSIResultSet.
	require.Len(t, obs0.RelevantEvidence, 2)
	assert.Contains(t, obs0.RelevantEvidence[0].Href, "execution_id:events:42")
	assert.Contains(t, obs0.RelevantEvidence[1].Href, "ledger_commit:commit-abc")

	// Findings: one per KSI result with correct status mapping.
	require.Len(t, result.Findings, 2)
	finding0 := result.Findings[0]
	assert.NotEmpty(t, finding0.UUID)
	assert.Contains(t, finding0.Title, "KSI-CMT-01")
	assert.Equal(t, "satisfied", finding0.Target.Status)
	assert.Equal(t, "KSI-CMT-01", finding0.Target.TargetID)

	finding1 := result.Findings[1]
	assert.Equal(t, "not-satisfied", finding1.Target.Status)
	assert.Contains(t, finding1.Description, "KSI-MLA-07")
}

// TestOSCALExporter_NilCatalog asserts that a nil catalog produces
// ErrValidationFailed.
func TestOSCALExporter_NilCatalog(t *testing.T) {
	exporter := NewOSCALExporter(nil)

	_, err := exporter.GenerateComponentDefinition()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)

	_, err = exporter.GenerateAssessmentResults(oscalTestResultSet())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

// TestOSCALExporter_NilResultSet asserts that a nil result set produces
// ErrValidationFailed.
func TestOSCALExporter_NilResultSet(t *testing.T) {
	exporter := NewOSCALExporter(oscalTestCatalog())

	_, err := exporter.GenerateAssessmentResults(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
}

// TestOSCALExporter_EmptyResultSet asserts that an empty result set produces
// valid output with empty observations/findings.
func TestOSCALExporter_EmptyResultSet(t *testing.T) {
	exporter := NewOSCALExporter(oscalTestCatalog())

	rs := &KSIResultSet{
		Class:         ClassC,
		EvaluatedAtMs: time.Now().UnixMilli(),
		Results:       []KSIResult{},
	}

	results, err := exporter.GenerateAssessmentResults(rs)
	require.NoError(t, err)
	require.NotNil(t, results)

	require.Len(t, results.Results, 1)
	assert.Empty(t, results.Results[0].Observations)
	assert.Empty(t, results.Results[0].Findings)
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

	results, err := exporter.GenerateAssessmentResults(oscalTestResultSet())
	require.NoError(t, err)

	data, err := json.Marshal(results)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Contains(t, raw, "uuid")
	assert.Contains(t, raw, "metadata")
	assert.Contains(t, raw, "results")

	// Verify results array.
	var resArr []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw["results"], &resArr))
	require.Len(t, resArr, 1)
	assert.Contains(t, resArr[0], "uuid")
	assert.Contains(t, resArr[0], "title")
	assert.Contains(t, resArr[0], "start")
	assert.Contains(t, resArr[0], "observations")
	assert.Contains(t, resArr[0], "findings")
}

// TestOSCALExporter_GenerateAssessmentResults_NotApplicableStatus verifies
// that KSIStatusNotApplicable maps to "not-applicable" in OSCAL findings.
func TestOSCALExporter_GenerateAssessmentResults_NotApplicableStatus(t *testing.T) {
	cat := oscalTestCatalog()
	exporter := NewOSCALExporter(cat)

	rs := &KSIResultSet{
		Class:         ClassC,
		EvaluatedAtMs: time.Now().UnixMilli(),
		Results: []KSIResult{
			{
				ID:                  "KSI-CMT-01",
				Status:              KSIStatusNotApplicable,
				MethodCount:         0,
				LastValidatedUnixMs: time.Now().UnixMilli(),
			},
		},
	}

	results, err := exporter.GenerateAssessmentResults(rs)
	require.NoError(t, err)
	require.Len(t, results.Results, 1)
	require.Len(t, results.Results[0].Findings, 1)
	assert.Equal(t, "not-applicable", results.Results[0].Findings[0].Target.Status)
}

// TestOSCALExporter_GenerateAssessmentResults_UnknownKSI asserts that a result
// referencing an unknown KSI ID produces an error.
func TestOSCALExporter_GenerateAssessmentResults_UnknownKSI(t *testing.T) {
	exporter := NewOSCALExporter(oscalTestCatalog())

	rs := &KSIResultSet{
		Class:         ClassC,
		EvaluatedAtMs: time.Now().UnixMilli(),
		Results: []KSIResult{
			{ID: "KSI-FAKE-99", Status: KSIStatusSatisfied, MethodCount: 1, LastValidatedUnixMs: time.Now().UnixMilli()},
		},
	}

	_, err := exporter.GenerateAssessmentResults(rs)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrValidationFailed)
	assert.Contains(t, err.Error(), "unknown KSI")
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

// TestGenerateUUID_RandomV4 verifies that generateUUID produces unique
// RFC 4122 UUID v4 strings with the correct format.
func TestGenerateUUID_RandomV4(t *testing.T) {
	u1 := generateUUID()
	u2 := generateUUID()

	assert.NotEqual(t, u1, u2, "two calls should produce different UUIDs")
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, u1)
	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, u2)

	_, err := uuid.Parse(u1)
	assert.NoError(t, err)
}
