// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	campaignProjectionEnvelopeSchemaVersion = "1.0.0"
	publicMessageTypeAssignmentLifecycle    = "PublicAssignmentLifecycleRecord"
	publicMessageTypeAssignmentResult       = "PublicAssignmentResultProjection"
)

// CampaignProjectionEnvelope wraps one typed public eval projection with a
// deterministic idempotency key for append-only publication.
type CampaignProjectionEnvelope struct {
	SchemaVersion  string          `json:"schema_version"`
	MessageType    string          `json:"message_type"`
	IdempotencyKey string          `json:"idempotency_key"`
	Record         json.RawMessage `json:"record"`
}

// BuildAssignmentLifecycleProjection materializes one public lifecycle record
// from canonical assignment state.
func BuildAssignmentLifecycleProjection(assignment *evalv1.EvaluationAssignment, scenarioCategory evalv1.EvaluationScenarioCategory, observedAt time.Time) (*evalv1.PublicAssignmentLifecycleRecord, error) {
	if assignment == nil || assignment.GetAssignmentId() == "" || assignment.GetRunId() == "" {
		return nil, fmt.Errorf("evaluation: build assignment lifecycle projection: %w", constants.ErrMissingRequiredField)
	}
	if observedAt.IsZero() {
		observedAt = assignmentLifecycleObservedAt(assignment)
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	record := &evalv1.PublicAssignmentLifecycleRecord{
		AssignmentId:     assignment.GetAssignmentId(),
		RunId:            assignment.GetRunId(),
		ScenarioId:       assignment.GetScenarioId(),
		ScenarioCategory: scenarioCategory,
		Lane:             assignment.GetLane(),
		LifecycleStatus:  assignment.GetLifecycleStatus(),
		Repetition:       assignment.GetRepetition(),
		ObservedAt:       timestamppb.New(observedAt.UTC()),
	}
	if homogeneous, ok := assignment.GetTarget().(*evalv1.EvaluationAssignment_Homogeneous); ok && homogeneous.Homogeneous != nil {
		record.DesignatedRole = homogeneous.Homogeneous.GetDesignatedRole()
		if variant := homogeneous.Homogeneous.GetCandidateVariant(); variant != nil {
			record.VariantId = variant.GetVariantId()
		}
	}
	if heterogeneous, ok := assignment.GetTarget().(*evalv1.EvaluationAssignment_Heterogeneous); ok && heterogeneous.Heterogeneous != nil {
		if stack := heterogeneous.Heterogeneous.GetStack(); stack != nil {
			record.StackId = stack.GetStackId()
		}
	}
	return record, nil
}

// BuildAssignmentResultProjection materializes one public terminal assignment
// projection from canonical assignment and result records.
func BuildAssignmentResultProjection(assignment *evalv1.EvaluationAssignment, result *evalv1.EvaluationAssignmentResult, scenarioCategory evalv1.EvaluationScenarioCategory, summaryStatus evalv1.EvaluationVerdictStatus, verificationStatus string) (*evalv1.PublicAssignmentResultProjection, error) {
	if assignment == nil || result == nil {
		return nil, fmt.Errorf("evaluation: build assignment result projection: %w", constants.ErrMissingRequiredField)
	}
	if assignment.GetAssignmentId() != result.GetAssignmentId() || assignment.GetRunId() != result.GetRunId() {
		return nil, fmt.Errorf("evaluation: build assignment result projection: assignment/result binding mismatch")
	}
	if verificationStatus == "" {
		verificationStatus = "unverified"
	}
	projection := &evalv1.PublicAssignmentResultProjection{
		AssignmentId:       result.GetAssignmentId(),
		RunId:              result.GetRunId(),
		ScenarioId:         assignment.GetScenarioId(),
		ScenarioCategory:   scenarioCategory,
		Lane:               result.GetLane(),
		LifecycleStatus:    result.GetLifecycleStatus(),
		SummaryStatus:      summaryStatus,
		ResultDigest:       result.GetResultDigest(),
		VerificationStatus: verificationStatus,
		CompletedAt:        result.GetCompletedAt(),
		DecomposedScores:   append([]*evalv1.DecomposedScoreRecord(nil), result.GetDecomposedScores()...),
	}
	if homogeneous, ok := assignment.GetTarget().(*evalv1.EvaluationAssignment_Homogeneous); ok && homogeneous.Homogeneous != nil {
		projection.DesignatedRole = homogeneous.Homogeneous.GetDesignatedRole()
		if variant := homogeneous.Homogeneous.GetCandidateVariant(); variant != nil {
			projection.VariantId = variant.GetVariantId()
		}
	}
	projection.UnavailableMetricReasons = collectUnavailableMetricReasons(result)
	return projection, nil
}

// AssignmentLifecycleIdempotencyKey returns the deterministic publication key
// for one assignment lifecycle transition.
func AssignmentLifecycleIdempotencyKey(runID, assignmentID string, status evalv1.EvaluationAssignmentLifecycleStatus) string {
	return runID + ":" + assignmentID + ":lifecycle:" + status.String()
}

// AssignmentResultIdempotencyKey returns the deterministic publication key for
// one terminal assignment result projection.
func AssignmentResultIdempotencyKey(runID, assignmentID string) string {
	return runID + ":" + assignmentID + ":result"
}

// AssignmentVerifiedResultIdempotencyKey returns the publication key for one
// post-verify assignment result republication.
func AssignmentVerifiedResultIdempotencyKey(runID, assignmentID string) string {
	return runID + ":" + assignmentID + ":result:verified"
}

// MarshalCampaignProjectionEnvelope canonicalizes one typed projection record
// into a public-feed payload envelope.
func MarshalCampaignProjectionEnvelope(messageType, idempotencyKey string, record proto.Message) ([]byte, error) {
	return marshalCampaignProjectionEnvelope(messageType, idempotencyKey, record, nil)
}

// MarshalAssignmentResultProjectionEnvelope canonicalizes one terminal assignment
// projection and merges disclosure-safe benchmark observations when present.
func MarshalAssignmentResultProjectionEnvelope(idempotencyKey string, projection *evalv1.PublicAssignmentResultProjection, benchmark *PublicBenchmarkObservations) ([]byte, error) {
	return marshalCampaignProjectionEnvelope(publicMessageTypeAssignmentResult, idempotencyKey, projection, benchmark)
}

func marshalCampaignProjectionEnvelope(messageType, idempotencyKey string, record proto.Message, benchmark *PublicBenchmarkObservations) ([]byte, error) {
	if messageType == "" || idempotencyKey == "" || record == nil {
		return nil, fmt.Errorf("evaluation: marshal campaign projection envelope: %w", constants.ErrMissingRequiredField)
	}
	canonical, err := evalv1.MarshalCanonical(record)
	if err != nil {
		return nil, err
	}
	recordBody := json.RawMessage(canonical)
	if benchmark != nil {
		recordMap := map[string]json.RawMessage{}
		if err := json.Unmarshal(canonical, &recordMap); err != nil {
			return nil, fmt.Errorf("evaluation: marshal campaign projection envelope: %w", err)
		}
		benchmarkBody, err := json.Marshal(benchmark)
		if err != nil {
			return nil, fmt.Errorf("evaluation: marshal campaign projection envelope: %w", err)
		}
		recordMap["benchmark_observations"] = benchmarkBody
		merged, err := json.Marshal(recordMap)
		if err != nil {
			return nil, fmt.Errorf("evaluation: marshal campaign projection envelope: %w", err)
		}
		recordBody = merged
	}
	envelope := CampaignProjectionEnvelope{
		SchemaVersion:  campaignProjectionEnvelopeSchemaVersion,
		MessageType:    messageType,
		IdempotencyKey: idempotencyKey,
		Record:         recordBody,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal campaign projection envelope: %w", err)
	}
	var compact json.RawMessage
	if err := json.Unmarshal(body, &compact); err != nil {
		return nil, fmt.Errorf("evaluation: marshal campaign projection envelope: %w", err)
	}
	return compact, nil
}

// DerivePublicSummaryStatus maps one terminal assignment result to a public
// summary verdict using deterministic grades when present.
func DerivePublicSummaryStatus(result *evalv1.EvaluationAssignmentResult) evalv1.EvaluationVerdictStatus {
	if result == nil {
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSPECIFIED
	}
	switch result.GetLifecycleStatus() {
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED:
		for _, grade := range result.GetDeterministicGrades() {
			if grade.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL {
				return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
			}
		}
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL:
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_UNAVAILABLE:
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_POLICY_REJECTED:
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
	default:
		return evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL
	}
}

// ScenarioCategoryForAssignment resolves one scenario category from a frozen
// catalog using the assignment scenario ID.
func ScenarioCategoryForAssignment(catalog *evalv1.EvaluationScenarioCatalog, assignment *evalv1.EvaluationAssignment) (evalv1.EvaluationScenarioCategory, error) {
	if catalog == nil || assignment == nil || assignment.GetScenarioId() == "" {
		return evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_UNSPECIFIED, fmt.Errorf("evaluation: scenario category lookup: %w", constants.ErrMissingRequiredField)
	}
	for _, scenario := range catalog.GetScenarios() {
		if scenario.GetScenarioId() == assignment.GetScenarioId() {
			return scenario.GetCategory(), nil
		}
	}
	return evalv1.EvaluationScenarioCategory_EVALUATION_SCENARIO_CATEGORY_UNSPECIFIED, fmt.Errorf("evaluation: scenario category lookup: unknown scenario %s", assignment.GetScenarioId())
}

func assignmentLifecycleObservedAt(assignment *evalv1.EvaluationAssignment) time.Time {
	switch assignment.GetLifecycleStatus() {
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING:
		if ts := assignment.GetStartedAt(); ts != nil {
			return ts.AsTime().UTC()
		}
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED:
		if ts := assignment.GetQueuedAt(); ts != nil {
			return ts.AsTime().UTC()
		}
	default:
		if ts := assignment.GetCompletedAt(); ts != nil {
			return ts.AsTime().UTC()
		}
		if ts := assignment.GetStartedAt(); ts != nil {
			return ts.AsTime().UTC()
		}
		if ts := assignment.GetQueuedAt(); ts != nil {
			return ts.AsTime().UTC()
		}
	}
	return time.Time{}
}

func collectUnavailableMetricReasons(result *evalv1.EvaluationAssignmentResult) []string {
	reasons := make([]string, 0)
	for _, inference := range result.GetModelInferences() {
		if inference.GetUsageAvailability() == evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE {
			reasons = append(reasons, "model_usage_unavailable")
			break
		}
	}
	return reasons
}
