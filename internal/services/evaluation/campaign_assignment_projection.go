// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// BuildPublicAssignmentProjection composes the approved public builders for a
// terminal assignment. Context and observation dependencies are resolved by
// callers; this function only composes their typed results.
func BuildPublicAssignmentProjection(ctx context.Context, input PublicAssignmentBuildInput) (*PublicAssignmentRecord, error) {
	if ctx == nil || input.Assignment == nil || input.Result == nil || input.ScenarioContext == nil {
		return nil, fmt.Errorf("evaluation: build public assignment projection: %w", constants.ErrMissingRequiredField)
	}
	if input.Assignment.GetAssignmentId() != input.Result.GetAssignmentId() || input.Assignment.GetRunId() != input.Result.GetRunId() || input.Assignment.GetScenarioId() != input.ScenarioContext.ScenarioID {
		return nil, fmt.Errorf("evaluation: build public assignment projection: %w", constants.ErrEvidenceScopeMismatch)
	}
	verificationStatus := input.VerificationStatus
	if verificationStatus == "" {
		verificationStatus = "unverified"
	}
	projection, err := BuildAssignmentResultProjection(input.Assignment, input.Result, input.ScenarioContext.Category, DerivePublicSummaryStatus(input.Result), verificationStatus)
	if err != nil {
		return nil, err
	}
	projection.ScenarioSummary, err = BuildPublicScenarioSummary(input.ScenarioContext)
	if err != nil {
		return nil, err
	}
	grades := input.GradeSummaries
	if grades == nil {
		grades, err = BuildPublicGradeSummarySet(input.Result, input.ScenarioContext)
		if err != nil {
			return nil, err
		}
	}
	projection.SemanticGradeSummaries = append([]*evalv1.PublicSemanticGradeSummary(nil), grades.Semantic...)
	activity := input.Activity
	bindings := input.EvidenceBindings
	if activity == nil {
		activity, bindings, err = BuildPublicAssignmentEvidence(PublicAssignmentEvidenceInput{Result: input.Result, PublicProofs: bindings})
		if err != nil {
			return nil, err
		}
	}
	projection.ActivitySummary = activity.Summary
	projection.EvidenceBindings = append([]*evalv1.PublicEvidenceBinding(nil), bindings...)
	projection.VerificationMetadata = input.VerificationMetadata

	extensions := input.Extensions
	if extensions.BenchmarkObservations == nil {
		extensions.BenchmarkObservations, err = input.ObservationReader.BuildPublicBenchmarkObservationsForScenario(ctx, input.Result, input.ScenarioContext)
		if err != nil {
			return nil, err
		}
	}
	if extensions.ResourceSummary == nil {
		extensions.ResourceSummary, err = BuildPublicResourceSummary(input.Result)
		if err != nil {
			return nil, err
		}
	}
	return &PublicAssignmentRecord{Projection: projection, Extensions: extensions}, nil
}

// MarshalPublicAssignmentRecord returns the canonical public assignment record
// body, combining canonical protobuf JSON with its named public extensions.
func MarshalPublicAssignmentRecord(record *PublicAssignmentRecord) ([]byte, error) {
	if record == nil || record.Projection == nil {
		return nil, fmt.Errorf("evaluation: marshal public assignment record: %w", constants.ErrMissingRequiredField)
	}
	canonical, err := evalv1.MarshalCanonical(record.Projection)
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal public assignment record: %w", err)
	}
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(canonical, &fields); err != nil {
		return nil, fmt.Errorf("evaluation: marshal public assignment record: %w", err)
	}
	extensions, err := json.Marshal(record.Extensions)
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal public assignment record extensions: %w", err)
	}
	var extensionFields map[string]json.RawMessage
	if err := json.Unmarshal(extensions, &extensionFields); err != nil {
		return nil, fmt.Errorf("evaluation: marshal public assignment record extensions: %w", err)
	}
	for key, value := range extensionFields {
		if _, exists := fields[key]; exists {
			return nil, fmt.Errorf("evaluation: marshal public assignment record: extension key %q collides with protobuf field: %w", key, constants.ErrEvidenceSchemaMismatch)
		}
		fields[key] = value
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal public assignment record: %w", err)
	}
	validationEnvelope, err := json.Marshal(CampaignProjectionEnvelope{
		SchemaVersion:  campaignProjectionEnvelopeEnrichedVersion,
		MessageType:    publicMessageTypeAssignmentResult,
		IdempotencyKey: "canonical-record-validation",
		Record:         body,
	})
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal public assignment record validation envelope: %w", err)
	}
	if err := ValidatePublicAssignmentRecord(campaignProjectionEnvelopeEnrichedVersion, validationEnvelope); err != nil {
		return nil, fmt.Errorf("evaluation: validate public assignment record: %w", err)
	}
	return body, nil
}

func marshalPublicAssignmentEnvelope(idempotencyKey string, record *PublicAssignmentRecord) ([]byte, error) {
	if idempotencyKey == "" {
		return nil, fmt.Errorf("evaluation: marshal public assignment envelope: %w", constants.ErrMissingRequiredField)
	}
	recordBody, err := MarshalPublicAssignmentRecord(record)
	if err != nil {
		return nil, err
	}
	envelope := CampaignProjectionEnvelope{SchemaVersion: campaignProjectionEnvelopeEnrichedVersion, MessageType: publicMessageTypeAssignmentResult, IdempotencyKey: idempotencyKey, Record: recordBody}
	body, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal public assignment envelope: %w", err)
	}
	if err := ValidatePublicAssignmentRecord(campaignProjectionEnvelopeEnrichedVersion, body); err != nil {
		return nil, fmt.Errorf("evaluation: validate public assignment envelope: %w", err)
	}
	return body, nil
}
