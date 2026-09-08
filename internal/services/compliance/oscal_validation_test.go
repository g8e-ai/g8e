// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	oscalvalidator "github.com/g8e-ai/g8e/v2/internal/services/compliance/oscal/validator"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func TestOSCALDocumentValidator_ValidateBytesReturnsProtocolOwnedStructuralFailures(t *testing.T) {
	validator, err := NewOSCALDocumentValidator()
	require.NoError(t, err)

	result, err := validator.ValidateBytes([]byte(`{"invalid":true}`))
	require.NoError(t, err)
	require.NotNil(t, result.GetValidator())
	assert.False(t, result.GetValid())
	assert.Equal(t, constants.OSCALValidatorID, result.GetValidator().GetValidatorId())
	assert.Equal(t, constants.OSCALValidatorVersion, result.GetValidator().GetValidatorVersion())
	assert.Equal(t, constants.OSCALSchemaVersion, result.GetValidator().GetSchemaVersion())
	assert.Equal(t, constants.OSCALSchemaSHA256, result.GetValidator().GetSchemaDigest())
	assert.NotEmpty(t, result.GetStructuralFailures())
	assert.Empty(t, result.GetSemanticFailures())
	assert.NotEmpty(t, result.GetStructuralFailures()[0].GetCode())
	assert.NotEmpty(t, result.GetStructuralFailures()[0].GetMessage())
}

func TestOSCALDocumentValidator_ValidateAssessmentResultsAcceptsGeneratedTypedDocument(t *testing.T) {
	document, err := NewOSCALExporter(oscalTestCatalog()).GenerateAssessmentResults(oscalTestAnalysis())
	require.NoError(t, err)
	validator, err := NewOSCALDocumentValidator()
	require.NoError(t, err)

	result, err := validator.ValidateAssessmentResults(document)
	require.NoError(t, err)
	assert.True(t, result.GetValid())
	assert.Empty(t, result.GetStructuralFailures())
	assert.Empty(t, result.GetSemanticFailures())
}

func TestOSCALDocumentValidator_ValidateAssessmentResultsRejectsNilDocument(t *testing.T) {
	validator, err := NewOSCALDocumentValidator()
	require.NoError(t, err)

	result, err := validator.ValidateAssessmentResults(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrOSCALValidationFailed)
	assert.Nil(t, result)
}

func TestOSCALDocumentValidator_ValidateAssessmentResultsReturnsProtocolOwnedSemanticFailures(t *testing.T) {
	document, err := NewOSCALExporter(oscalTestCatalog()).GenerateAssessmentResults(oscalTestAnalysis())
	require.NoError(t, err)
	for i := range document.AssessmentResults.BackMatter.Resources[0].Props {
		if document.AssessmentResults.BackMatter.Resources[0].Props[i].Name == "scope-id" {
			document.AssessmentResults.BackMatter.Resources[0].Props[i].Value = "scope-2"
		}
	}
	validator, err := NewOSCALDocumentValidator()
	require.NoError(t, err)

	result, err := validator.ValidateAssessmentResults(document)
	require.NoError(t, err)
	assert.False(t, result.GetValid())
	assert.Empty(t, result.GetStructuralFailures())
	require.NotEmpty(t, result.GetSemanticFailures())
	assert.Equal(t, string(oscalvalidator.SemanticEvidenceScopeMismatch), result.GetSemanticFailures()[0].GetCode())
	assert.NotEmpty(t, result.GetSemanticFailures()[0].GetMessage())
	assert.NotEmpty(t, result.GetSemanticFailures()[0].GetInstancePtr())
}

func TestOSCALDocumentValidator_ValidationResultIsCanonicalProtocolJSON(t *testing.T) {
	validator, err := NewOSCALDocumentValidator()
	require.NoError(t, err)
	result, err := validator.ValidateBytes([]byte(`{"invalid":true}`))
	require.NoError(t, err)

	encoded, err := compliancev1.MarshalCanonical(result)
	require.NoError(t, err)
	decoded := &compliancev1.OSCALValidationResult{}
	require.NoError(t, compliancev1.UnmarshalCanonical(encoded, decoded))
	assert.True(t, proto.Equal(result, decoded))
}

func TestOSCALDocumentValidator_ValidationResultGoldenVector(t *testing.T) {
	validator, err := NewOSCALDocumentValidator()
	require.NoError(t, err)
	result, err := validator.ValidateBytes([]byte(`{"invalid":true}`))
	require.NoError(t, err)
	encoded, err := compliancev1.MarshalCanonical(result)
	require.NoError(t, err)
	digest := sha256.Sum256(encoded)
	assert.Equal(t, "03ecd403d6048fc97cea0e0d62074bdccba7a4f6080c3e58b3fa7998e156c27f", hex.EncodeToString(digest[:]))
}

func TestOSCALDocumentValidator_ValidateBytesReturnsDeterministicFailures(t *testing.T) {
	validator, err := NewOSCALDocumentValidator()
	require.NoError(t, err)
	document := []byte(`{"assessment-results":{"uuid":"invalid"}}`)

	first, err := validator.ValidateBytes(document)
	require.NoError(t, err)
	second, err := validator.ValidateBytes(document)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}
