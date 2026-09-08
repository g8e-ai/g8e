// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package compliance

import (
	"encoding/json"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/jsonschema"
	oscalvalidator "github.com/g8e-ai/g8e/v2/internal/services/compliance/oscal/validator"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

type OSCALDocumentValidator struct {
	validator *oscalvalidator.Validator
}

func NewOSCALDocumentValidator() (*OSCALDocumentValidator, error) {
	validator, err := oscalvalidator.NewValidator()
	if err != nil {
		return nil, fmt.Errorf("compliance: initialize OSCAL document validator: %w: %w", constants.ErrOSCALSchemaCompileFailed, err)
	}
	return &OSCALDocumentValidator{validator: validator}, nil
}

func (v *OSCALDocumentValidator) ValidateBytes(document []byte) (*compliancev1.OSCALValidationResult, error) {
	result, err := v.validator.Validate(document)
	if err != nil {
		return nil, fmt.Errorf("compliance: validate OSCAL assessment-results bytes: %w: %w", constants.ErrOSCALValidationFailed, err)
	}
	return newProtocolOSCALValidationResult(result), nil
}

func (v *OSCALDocumentValidator) ValidateAssessmentResults(document *OSCALAssessmentResults) (*compliancev1.OSCALValidationResult, error) {
	if document == nil {
		return nil, fmt.Errorf("compliance: validate typed OSCAL assessment-results: %w", constants.ErrOSCALValidationFailed)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("compliance: marshal typed OSCAL assessment-results: %w: %w", constants.ErrOSCALValidationFailed, err)
	}
	return v.ValidateBytes(encoded)
}

func newProtocolOSCALValidationResult(result *oscalvalidator.ValidationResult) *compliancev1.OSCALValidationResult {
	return &compliancev1.OSCALValidationResult{
		Validator: &compliancev1.OSCALValidatorIdentity{
			ValidatorId:      result.ValidatorID,
			ValidatorVersion: result.ValidatorVersion,
			SchemaVersion:    result.SchemaVersion,
			SchemaDigest:     result.SchemaDigest,
		},
		Valid:              result.Valid,
		StructuralFailures: newProtocolStructuralFailures(result.StructuralFailures),
		SemanticFailures:   newProtocolSemanticFailures(result.SemanticFailures),
	}
}

func newProtocolStructuralFailures(failures jsonschema.Failures) []*compliancev1.OSCALValidationFailure {
	results := make([]*compliancev1.OSCALValidationFailure, 0, len(failures))
	for _, failure := range failures {
		results = append(results, &compliancev1.OSCALValidationFailure{
			Code:        string(failure.Reason),
			Message:     failure.Message,
			InstancePtr: failure.InstancePtr,
			SchemaPtr:   failure.SchemaPtr,
			Keyword:     failure.Keyword,
		})
	}
	return results
}

func newProtocolSemanticFailures(failures oscalvalidator.SemanticFailures) []*compliancev1.OSCALValidationFailure {
	results := make([]*compliancev1.OSCALValidationFailure, 0, len(failures))
	for _, failure := range failures {
		results = append(results, &compliancev1.OSCALValidationFailure{
			Code:        string(failure.Reason),
			Message:     failure.Message,
			InstancePtr: failure.InstancePtr,
		})
	}
	return results
}
