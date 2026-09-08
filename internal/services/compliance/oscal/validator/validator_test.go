// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version  2.0.

package validator

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/jsonschema"
)

// minimalValidOSCAL is a minimal valid OSCAL 1.1.2 assessment-results
// document that passes both structural and semantic validation.
//
//go:embed testdata/ifa_assessment-results-example-min.json
var officialNISTAssessmentResults []byte

//go:embed testdata/ifa_assessment-results-example-min.provenance.json
var officialNISTAssessmentResultsProvenance []byte

type officialFixtureProvenance struct {
	FixtureType      string `json:"fixture_type"`
	SchemaVersion    string `json:"schema_version"`
	SourceURL        string `json:"source_url"`
	SourceRepository string `json:"source_repository"`
	SourceRevision   string `json:"source_revision"`
	SourceBlob       string `json:"source_blob"`
	SourcePath       string `json:"source_path"`
	License          string `json:"license"`
	ByteLength       int    `json:"byte_length"`
	SHA256           string `json:"sha256"`
	RetrievedAt      string `json:"retrieved_at"`
	RetrievalMethod  string `json:"retrieval_method"`
	IntegrityNote    string `json:"integrity_note"`
}

const minimalValidOSCAL = `{
  "assessment-results": {
    "uuid": "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
    "metadata": {
      "title": "g8e Compliance Assessment Results",
      "last-modified": "2026-09-07T00:00:00Z",
      "version": "1.0.0",
      "oscal-version": "1.1.2"
    },
    "import-ap": {
      "href": "urn:g8e:assessment-plan:compliance-analysis:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    },
    "results": [
      {
        "uuid": "b1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
        "title": "Assessment Results",
        "description": "Compliance assessment results",
        "start": "2026-09-07T00:00:00Z",
        "end": "2026-09-07T01:00:00Z",
        "local-definitions": {
          "components": [
            {
              "uuid": "d1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
              "type": "software",
              "title": "Gateway",
              "description": "Assessment subject",
              "status": {"state": "operational"}
            }
          ]
        },
        "reviewed-controls": {
          "control-selections": [
            {
              "include-controls": [
                {"control-id": "KSI-MLA-07"}
              ]
            }
          ]
        },
        "observations": [
          {
            "uuid": "c1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
            "title": "Receipt Integrity Observation",
            "description": "Verified receipt signatures",
            "methods": ["TEST"],
            "subjects": [
              {
                "subject-uuid": "d1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
                "type": "component",
                "title": "Gateway"
              }
            ],
            "relevant-evidence": [
              {
                "href": "#e1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
                "description": "Signed receipt evidence"
              }
            ],
            "collected": "2026-09-07T00:30:00Z"
          }
        ],
        "findings": [
          {
            "uuid": "f1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
            "title": "KSI-MLA-07 Finding",
            "description": "Receipt integrity verified",
            "target": {
              "type": "statement-id",
              "target-id": "KSI-MLA-07",
              "status": {
                "state": "satisfied"
              }
            }
          }
        ]
      }
    ],
    "back-matter": {
      "resources": [
        {
          "uuid": "e1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
          "title": "Receipt Evidence",
          "description": "Content-addressed receipt evidence",
          "props": [
            {
              "name": "media-type",
              "value": "application/json"
            }
          ]
        }
      ]
    }
  }
}`

func TestValidator_NewValidator_CompilesEmbeddedSchema(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	assert.NotNil(t, v)
	assert.Equal(t, constants.OSCALValidatorID, v.ValidatorID())
	assert.Equal(t, constants.OSCALValidatorVersion, v.ValidatorVersion())
	assert.Equal(t, constants.OSCALSchemaVersion, v.SchemaVersion())
	assert.Equal(t, constants.OSCALSchemaSHA256, v.SchemaDigest())
}

func TestValidator_ValidMinimalDocument_PassesBothStages(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	result, err := v.Validate([]byte(minimalValidOSCAL))
	require.NoError(t, err)
	if !result.Valid {
		t.Fatalf("expected valid document, got structural failures: %v\nsemantic failures: %v",
			result.StructuralFailures, result.SemanticFailures)
	}
	assert.True(t, result.Valid)
	assert.Empty(t, result.StructuralFailures)
	assert.Empty(t, result.SemanticFailures)
}

func TestValidator_OfficialNISTAssessmentResultsFixture_PassesBothStages(t *testing.T) {
	var provenance officialFixtureProvenance
	require.NoError(t, json.Unmarshal(officialNISTAssessmentResultsProvenance, &provenance))
	assert.Equal(t, "oscal-assessment-results", provenance.FixtureType)
	assert.Equal(t, constants.OSCALSchemaVersion, provenance.SchemaVersion)
	assert.NotEmpty(t, provenance.SourceURL)
	assert.NotEmpty(t, provenance.SourceRepository)
	assert.NotEmpty(t, provenance.SourceRevision)
	assert.NotEmpty(t, provenance.SourceBlob)
	assert.NotEmpty(t, provenance.SourcePath)
	assert.NotEmpty(t, provenance.License)
	assert.NotEmpty(t, provenance.RetrievedAt)
	assert.NotEmpty(t, provenance.RetrievalMethod)
	assert.NotEmpty(t, provenance.IntegrityNote)
	assert.Equal(t, provenance.ByteLength, len(officialNISTAssessmentResults))
	digest := sha256.Sum256(officialNISTAssessmentResults)
	assert.Equal(t, provenance.SHA256, hex.EncodeToString(digest[:]))

	v, err := NewValidator()
	require.NoError(t, err)
	result, err := v.Validate(officialNISTAssessmentResults)
	require.NoError(t, err)
	assert.True(t, result.Valid, "structural failures: %v\nsemantic failures: %v", result.StructuralFailures, result.SemanticFailures)
	assert.Empty(t, result.StructuralFailures)
	assert.Empty(t, result.SemanticFailures)
}

func TestValidator_InvalidJSON_FailsStructural(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	result, err := v.Validate([]byte(`{invalid json`))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.StructuralFailures)
}

func TestValidator_DuplicateJSONKeyFailsWithTypedStructuralReason(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	result, err := v.Validate([]byte(`{"assessment-results":{"uuid":"a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d","uuid":"b1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"}}`))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	require.Len(t, result.StructuralFailures, 1)
	assert.Equal(t, jsonschema.ReasonCompileDuplicateKey, result.StructuralFailures[0].Reason)
	assert.Empty(t, result.SemanticFailures)
}

func TestValidator_MissingRequiredField_FailsStructural(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	// Missing uuid field
	doc := `{"assessment-results": {"metadata": {"title": "test", "last-modified": "2026-09-07T00:00:00Z", "version": "1.0.0", "oscal-version": "1.1.2"}, "import-ap": {"href": "#x"}, "results": []}}`
	result, err := v.Validate([]byte(doc))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.StructuralFailures)
}

func TestValidator_EmittedObjectMutationMatrixRejectsStructuralViolations(t *testing.T) {
	tests := []struct {
		name   string
		old    string
		new    string
		reason jsonschema.ReasonCode
	}{
		{name: "assessment-results required UUID", old: `"uuid": "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",`, reason: jsonschema.ReasonValueRequiredMissing},
		{name: "metadata required title", old: `"title": "g8e Compliance Assessment Results",`, reason: jsonschema.ReasonValueRequiredMissing},
		{name: "result required title", old: `"title": "Assessment Results",`, reason: jsonschema.ReasonValueRequiredMissing},
		{name: "component required description", old: `"description": "Assessment subject",`, reason: jsonschema.ReasonValueRequiredMissing},
		{name: "observation required methods", old: `"methods": ["TEST"],`, reason: jsonschema.ReasonValueRequiredMissing},
		{name: "finding required target", old: `"target": {`, new: `"removed-target": {`, reason: jsonschema.ReasonValueRequiredMissing},
		{name: "resource required UUID", old: `"uuid": "e1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",`, reason: jsonschema.ReasonValueRequiredMissing},
		{name: "result unknown property", old: `"title": "Assessment Results",`, new: `"title": "Assessment Results", "unknown": true,`, reason: jsonschema.ReasonValueAdditionalProp},
	}
	v, err := NewValidator()
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document := replaceInJSON(t, minimalValidOSCAL, tt.old, tt.new)
			result, err := v.Validate([]byte(document))
			require.NoError(t, err)
			assert.False(t, result.Valid)
			assert.True(t, hasStructuralFailure(result.StructuralFailures, tt.reason), result.StructuralFailures.Error())
			assert.Empty(t, result.SemanticFailures)
		})
	}
}

func TestValidator_EmittedObjectMutationMatrixRejectsUnknownProperties(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{name: "document root", old: `"assessment-results": {`, new: `"unknown": true, "assessment-results": {`},
		{name: "assessment results", old: `"uuid": "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",`, new: `"uuid": "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d", "unknown": true,`},
		{name: "metadata", old: `"title": "g8e Compliance Assessment Results",`, new: `"title": "g8e Compliance Assessment Results", "unknown": true,`},
		{name: "assessment plan import", old: `"href": "urn:g8e:assessment-plan:compliance-analysis:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`, new: `"href": "urn:g8e:assessment-plan:compliance-analysis:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "unknown": true`},
		{name: "result", old: `"title": "Assessment Results",`, new: `"title": "Assessment Results", "unknown": true,`},
		{name: "local definitions", old: `"local-definitions": {`, new: `"local-definitions": { "unknown": true,`},
		{name: "component", old: `"uuid": "d1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",`, new: `"uuid": "d1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d", "unknown": true,`},
		{name: "component status", old: `"status": {"state": "operational"}`, new: `"status": {"state": "operational", "unknown": true}`},
		{name: "reviewed controls", old: `"reviewed-controls": {`, new: `"reviewed-controls": { "unknown": true,`},
		{name: "control selection", old: `"include-controls": [`, new: `"unknown": true, "include-controls": [`},
		{name: "control identifier", old: `{"control-id": "KSI-MLA-07"}`, new: `{"control-id": "KSI-MLA-07", "unknown": true}`},
		{name: "observation", old: `"uuid": "c1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",`, new: `"uuid": "c1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d", "unknown": true,`},
		{name: "observation subject", old: `"subject-uuid": "d1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",`, new: `"subject-uuid": "d1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d", "unknown": true,`},
		{name: "relevant evidence", old: `"href": "#e1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",`, new: `"href": "#e1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d", "unknown": true,`},
		{name: "finding", old: `"uuid": "f1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",`, new: `"uuid": "f1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d", "unknown": true,`},
		{name: "finding target", old: `"type": "statement-id",`, new: `"type": "statement-id", "unknown": true,`},
		{name: "finding target status", old: `"state": "satisfied"`, new: `"state": "satisfied", "unknown": true`},
		{name: "back matter", old: `"back-matter": {`, new: `"back-matter": { "unknown": true,`},
		{name: "resource", old: `"uuid": "e1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",`, new: `"uuid": "e1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d", "unknown": true,`},
		{name: "property", old: `"name": "media-type",`, new: `"name": "media-type", "unknown": true,`},
	}
	v, err := NewValidator()
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document := replaceInJSON(t, minimalValidOSCAL, tt.old, tt.new)
			result, err := v.Validate([]byte(document))
			require.NoError(t, err)
			assert.False(t, result.Valid)
			assert.True(t, hasStructuralFailure(result.StructuralFailures, jsonschema.ReasonValueAdditionalProp), result.StructuralFailures.Error())
			assert.Empty(t, result.SemanticFailures)
		})
	}
}

func TestValidator_InvalidUUID_Fails(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	// Replace the valid UUID with an invalid one — this fails structural
	// validation (UUID pattern) and/or semantic validation
	doc := replaceInJSON(t, minimalValidOSCAL, `"a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"`, `"not-a-uuid"`)
	result, err := v.Validate([]byte(doc))
	require.NoError(t, err)
	assert.False(t, result.Valid)
}

func TestValidator_DuplicateUUID_FailsSemantic(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	// Make two elements share the same UUID — both are valid UUIDs so
	// structural validation passes, but semantic validation catches the
	// duplicate
	doc := replaceInJSON(t, minimalValidOSCAL, `"b1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"`, `"a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"`)
	result, err := v.Validate([]byte(doc))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	// May fail structural or semantic depending on which UUID is duplicated
	// The important thing is that it fails
	assert.Empty(t, result.StructuralFailures)
	assert.True(t, hasSemanticFailure(result.SemanticFailures, SemanticUUIDDuplicate))
}

func TestValidator_UnresolvedEvidenceRef_FailsSemantic(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	// Change the evidence href to a non-existent UUID (valid UUID format
	// but not in back-matter)
	doc := replaceInJSON(t, minimalValidOSCAL, `"#e1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"`, `"#99999999-eeee-4a7b-8c9d-0e1f2a3b4c5d"`)
	result, err := v.Validate([]byte(doc))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	// This should pass structural (href is a valid URI-reference) but fail
	// semantic (unresolved evidence reference)
	if len(result.StructuralFailures) == 0 {
		assert.True(t, hasSemanticFailure(result.SemanticFailures, SemanticObsEvidenceUnresolved))
	}
}

func TestValidator_InvalidTimestamp_Fails(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	doc := replaceInJSON(t, minimalValidOSCAL, `"2026-09-07T00:00:00Z"`, `"not-a-timestamp"`)
	result, err := v.Validate([]byte(doc))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	// May fail structural (date-time format) or semantic (timestamp)
}

func TestValidator_InvalidOSCALVersion_FailsSemantic(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	doc := replaceInJSON(t, minimalValidOSCAL, `"oscal-version": "1.1.2"`, `"oscal-version": "1.0.0"`)
	result, err := v.Validate([]byte(doc))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	// "1.0.0" is a valid string, so structural passes; semantic catches it
	if len(result.StructuralFailures) == 0 {
		assert.True(t, hasSemanticFailure(result.SemanticFailures, SemanticOSCALVersionInvalid))
	}
}

func TestValidator_InvalidFindingStatus_FailsSemantic(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	doc := replaceInJSON(t, minimalValidOSCAL, `"state": "satisfied"`, `"state": "invalid-status"`)
	result, err := v.Validate([]byte(doc))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	// "invalid-status" is a valid string, so structural passes; semantic catches it
	if len(result.StructuralFailures) == 0 {
		assert.True(t, hasSemanticFailure(result.SemanticFailures, SemanticFindingStatusInvalid))
	}
}

func TestValidator_InvalidMediaType_FailsSemantic(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	doc := replaceInJSON(t, minimalValidOSCAL, `"value": "application/json"`, `"value": "invalid/media-type"`)
	result, err := v.Validate([]byte(doc))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	// "invalid/media-type" is a valid string, so structural passes; semantic catches it
	if len(result.StructuralFailures) == 0 {
		assert.True(t, hasSemanticFailure(result.SemanticFailures, SemanticMediaTypeInvalid))
	}
}

func TestValidator_EmptyObservationSubjects_FailsSemantic(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	doc := replaceInJSON(t, minimalValidOSCAL, `"subjects": [
              {
                "subject-uuid": "d1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
                "type": "component",
                "title": "Gateway"
              }
            ]`, `"subjects": []`)
	result, err := v.Validate([]byte(doc))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	// Empty subjects array may pass structural (minItems not set) but fail semantic
	if len(result.StructuralFailures) == 0 {
		assert.True(t, hasSemanticFailure(result.SemanticFailures, SemanticObsSubjectMissing))
	}
}

func TestValidator_SemanticBindingsRejectInconsistentDocuments(t *testing.T) {
	contentAddressProps := `"props": [
            {"name": "artifact-id", "value": "action-receipt:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
            {"name": "artifact-type", "value": "action-receipt"},
            {"name": "sha256", "value": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
            {"name": "media-type", "value": "application/json"},
            {"name": "schema-ref", "value": "g8e.operator.v1.ActionReceipt"},
            {"name": "producer-identity", "value": "gateway"},
            {"name": "verification-status", "value": "verified"},
            {"name": "verifier-id", "value": "receipt-verifier"},
            {"name": "verifier-version", "value": "1.0.0"},
            {"name": "scope-id", "value": "scope-1"},
            {"name": "bundle-path", "value": "receipts.jsonl"}
          ]`
	withContentAddress := replaceInJSON(t, minimalValidOSCAL, `"props": [
            {
              "name": "media-type",
              "value": "application/json"
            }
          ]`, contentAddressProps)
	withContentAddress = replaceInJSON(t, withContentAddress, `"end": "2026-09-07T01:00:00Z",`, `"end": "2026-09-07T01:00:00Z",
        "props": [{"name": "g8e-scope-id", "value": "scope-1"}],`)
	tests := []struct {
		name     string
		document string
		reason   SemanticReasonCode
	}{
		{
			name:     "unresolved local assessment plan",
			document: replaceInJSON(t, minimalValidOSCAL, `"href": "urn:g8e:assessment-plan:compliance-analysis:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`, `"href": "#99999999-eeee-4a7b-8c9d-0e1f2a3b4c5d"`),
			reason:   SemanticImportAPMissing,
		},
		{
			name:     "malformed local evidence reference",
			document: replaceInJSON(t, minimalValidOSCAL, `"href": "#e1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"`, `"href": "#not-a-resource"`),
			reason:   SemanticObsEvidenceUnresolved,
		},
		{
			name:     "duplicate reviewed control",
			document: replaceInJSON(t, minimalValidOSCAL, `{"control-id": "KSI-MLA-07"}`, `{"control-id": "KSI-MLA-07"}, {"control-id": "KSI-MLA-07"}`),
			reason:   SemanticReviewedControls,
		},
		{
			name:     "finding target outside reviewed controls",
			document: replaceInJSON(t, minimalValidOSCAL, `"target-id": "KSI-MLA-07"`, `"target-id": "KSI-CMT-99"`),
			reason:   SemanticFindingTargetMissing,
		},
		{
			name:     "unresolved observation subject",
			document: replaceInJSON(t, minimalValidOSCAL, `"subject-uuid": "d1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"`, `"subject-uuid": "99999999-eeee-4a7b-8c9d-0e1f2a3b4c5d"`),
			reason:   SemanticObsSubjectMissing,
		},
		{
			name:     "result interval is inverted",
			document: replaceInJSON(t, minimalValidOSCAL, `"end": "2026-09-07T01:00:00Z"`, `"end": "2026-09-06T23:00:00Z"`),
			reason:   SemanticTimestampInvalid,
		},
		{
			name:     "observation is outside result interval",
			document: replaceInJSON(t, minimalValidOSCAL, `"collected": "2026-09-07T00:30:00Z"`, `"collected": "2026-09-07T02:00:00Z"`),
			reason:   SemanticTimestampInvalid,
		},
		{
			name:     "artifact type does not match content address",
			document: replaceInJSON(t, withContentAddress, `"name": "artifact-type", "value": "action-receipt"`, `"name": "artifact-type", "value": "eval-metric"`),
			reason:   SemanticContentAddressInvalid,
		},
		{
			name:     "digest does not match content address",
			document: replaceInJSON(t, withContentAddress, `"name": "sha256", "value": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`, `"name": "sha256", "value": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`),
			reason:   SemanticContentAddressInvalid,
		},
		{
			name:     "duplicate evidence property",
			document: replaceInJSON(t, withContentAddress, `{"name": "artifact-type", "value": "action-receipt"}`, `{"name": "artifact-type", "value": "action-receipt"}, {"name": "artifact-type", "value": "action-receipt"}`),
			reason:   SemanticContentAddressInvalid,
		},
		{
			name:     "verified evidence missing verifier identity",
			document: replaceInJSON(t, withContentAddress, `{"name": "verifier-id", "value": "receipt-verifier"},`, ``),
			reason:   SemanticContentAddressInvalid,
		},
		{
			name:     "incomplete encryption metadata",
			document: replaceInJSON(t, withContentAddress, `{"name": "bundle-path", "value": "receipts.jsonl"}`, `{"name": "bundle-path", "value": "receipts.jsonl"}, {"name": "encryption-algorithm", "value": "AES-256-GCM"}`),
			reason:   SemanticContentAddressInvalid,
		},
		{
			name: "content-addressed evidence missing result scope",
			document: replaceInJSON(t, withContentAddress, `        "props": [{"name": "g8e-scope-id", "value": "scope-1"}],
`, ``),
			reason: SemanticEvidenceScopeMismatch,
		},
		{
			name:     "cross-scope evidence link",
			document: replaceInJSON(t, withContentAddress, `{"name": "scope-id", "value": "scope-1"}`, `{"name": "scope-id", "value": "scope-2"}`),
			reason:   SemanticEvidenceScopeMismatch,
		},
		{
			name: "back-matter resource missing title and description",
			document: replaceInJSON(t, minimalValidOSCAL, `          "title": "Receipt Evidence",
          "description": "Content-addressed receipt evidence",
`, ``),
			reason: SemanticBackMatterResourceMissing,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := NewValidator()
			require.NoError(t, err)
			result, err := v.Validate([]byte(tt.document))
			require.NoError(t, err)
			assert.Empty(t, result.StructuralFailures)
			assert.False(t, result.Valid)
			assert.True(t, hasSemanticFailure(result.SemanticFailures, tt.reason), result.SemanticFailures.Error())
		})
	}
}

func TestValidator_FindingTargetPassesWhenReviewedControlsIncludesAll(t *testing.T) {
	document := replaceInJSON(t, minimalValidOSCAL, `"include-controls": [
                {"control-id": "KSI-MLA-07"}
              ]`, `"include-all": {}`)
	v, err := NewValidator()
	require.NoError(t, err)
	result, err := v.Validate([]byte(document))
	require.NoError(t, err)
	assert.True(t, result.Valid, result.SemanticFailures.Error())
}

func TestValidator_ContentAddressedEvidenceWithMatchingScopePasses(t *testing.T) {
	contentAddressProps := `"props": [
            {"name": "artifact-id", "value": "action-receipt:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
            {"name": "artifact-type", "value": "action-receipt"},
            {"name": "sha256", "value": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
            {"name": "media-type", "value": "application/json"},
            {"name": "schema-ref", "value": "g8e.operator.v1.ActionReceipt"},
            {"name": "producer-identity", "value": "gateway"},
            {"name": "verification-status", "value": "verified"},
            {"name": "verifier-id", "value": "receipt-verifier"},
            {"name": "verifier-version", "value": "1.0.0"},
            {"name": "scope-id", "value": "scope-1"},
            {"name": "bundle-path", "value": "receipts.jsonl"}
          ]`
	document := replaceInJSON(t, minimalValidOSCAL, `"props": [
            {
              "name": "media-type",
              "value": "application/json"
            }
          ]`, contentAddressProps)
	document = replaceInJSON(t, document, `"end": "2026-09-07T01:00:00Z",`, `"end": "2026-09-07T01:00:00Z",
        "props": [{"name": "g8e-scope-id", "value": "scope-1"}],`)
	v, err := NewValidator()
	require.NoError(t, err)
	result, err := v.Validate([]byte(document))
	require.NoError(t, err)
	assert.True(t, result.Valid, result.SemanticFailures.Error())
}

func TestValidator_ValidatorIdentity(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	result, err := v.Validate([]byte(minimalValidOSCAL))
	require.NoError(t, err)
	assert.Equal(t, constants.OSCALValidatorID, result.ValidatorID)
	assert.Equal(t, constants.OSCALValidatorVersion, result.ValidatorVersion)
	assert.Equal(t, constants.OSCALSchemaVersion, result.SchemaVersion)
	assert.Equal(t, constants.OSCALSchemaSHA256, result.SchemaDigest)
}

func TestValidator_StructuralFailurePreventsSemanticValidation(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	// A structurally invalid document should not get semantic failures
	result, err := v.Validate([]byte(`{"invalid": true}`))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.StructuralFailures)
	// Semantic validation is skipped when structural validation fails
	assert.Empty(t, result.SemanticFailures)
}

func TestValidator_DeterministicResults(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	document := replaceInJSON(t, minimalValidOSCAL, `"b1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"`, `"a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"`)
	document = replaceInJSON(t, document, `"2026-09-07T00:30:00Z"`, `"2026-09-07T02:00:00Z"`)
	expected, err := v.Validate([]byte(document))
	require.NoError(t, err)
	for range 25 {
		actual, err := v.Validate([]byte(document))
		require.NoError(t, err)
		assert.Equal(t, expected, actual)
	}
}

func TestIsValidUUID(t *testing.T) {
	assert.True(t, isValidUUID("a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"))
	assert.True(t, isValidUUID("A1B2C3D4-E5F6-4A7B-8C9D-0E1F2A3B4C5D"))
	assert.False(t, isValidUUID("not-a-uuid"))
	assert.False(t, isValidUUID("a1b2c3d4-e5f6-4a7b-8c9d"))
	assert.False(t, isValidUUID("a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5x"))
	assert.False(t, isValidUUID(""))
}

// --- Helpers ---

func hasStructuralFailure(failures jsonschema.Failures, reason jsonschema.ReasonCode) bool {
	for _, failure := range failures {
		if failure.Reason == reason {
			return true
		}
	}
	return false
}

func hasSemanticFailure(failures SemanticFailures, reason SemanticReasonCode) bool {
	for _, f := range failures {
		if f.Reason == reason {
			return true
		}
	}
	return false
}

func replaceInJSON(t *testing.T, src, old, new string) string {
	t.Helper()
	result := replaceFirst(src, old, new)
	if result == src {
		t.Fatalf("replaceInJSON: old string %q not found in source", old)
	}
	return result
}

func replaceFirst(s, old, new string) string {
	idx := indexOf(s, old)
	if idx == -1 {
		return s
	}
	return s[:idx] + new + s[idx+len(old):]
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// Ensure json import is used
var _ = json.Marshal
var _ = fmt.Sprintf
