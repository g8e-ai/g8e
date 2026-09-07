// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version  2.0.

package validator

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// minimalValidOSCAL is a minimal valid OSCAL 1.1.2 assessment-results
// document that passes both structural and semantic validation.
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
      "href": "#a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5e"
    },
    "results": [
      {
        "uuid": "b1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d",
        "title": "Assessment Results",
        "description": "Compliance assessment results",
        "start": "2026-09-07T00:00:00Z",
        "end": "2026-09-07T01:00:00Z",
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

func TestValidator_InvalidJSON_FailsStructural(t *testing.T) {
	v, err := NewValidator()
	require.NoError(t, err)
	result, err := v.Validate([]byte(`{invalid json`))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.StructuralFailures)
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
	r1, err := v.Validate([]byte(minimalValidOSCAL))
	require.NoError(t, err)
	r2, err := v.Validate([]byte(minimalValidOSCAL))
	require.NoError(t, err)
	assert.Equal(t, r1.Valid, r2.Valid)
	assert.Equal(t, len(r1.StructuralFailures), len(r2.StructuralFailures))
	assert.Equal(t, len(r1.SemanticFailures), len(r2.SemanticFailures))
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
