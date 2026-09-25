// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/publicdisclosure"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// ModelRoleInvocationIdempotencyKey returns the publication key for one
// disclosure-safe model-role invocation live event emitted when an assignment
// transitions to running.
func ModelRoleInvocationIdempotencyKey(runID, assignmentID, role string) string {
	return runID + ":" + assignmentID + ":invocation:" + role
}

// ScoredModelRoleInvocationIdempotencyKey returns the publication key for one
// disclosure-safe model-role invocation live event emitted when a scored model
// inference is first observed for the assignment.
func ScoredModelRoleInvocationIdempotencyKey(runID, assignmentID, role string) string {
	return runID + ":" + assignmentID + ":invocation-scored:" + role
}

// MetricAvailabilityIdempotencyKey returns the publication key for one
// per-assignment metric availability live event.
func MetricAvailabilityIdempotencyKey(runID, assignmentID, metricID string) string {
	return runID + ":" + assignmentID + ":metric:" + metricID
}

func (c *CampaignPublicationCoordinator) buildAssignmentLiveEventPublishRequests(
	ctx context.Context,
	assignment *evalv1.EvaluationAssignment,
	result *evalv1.EvaluationAssignmentResult,
) ([]campaignFeedPublishRequest, error) {
	if c == nil || assignment == nil || result == nil {
		return nil, fmt.Errorf("evaluation: build assignment live event publish requests: %w", constants.ErrMissingRequiredField)
	}
	completed, total, err := c.assignmentLiveEventProgress(ctx, assignment.GetRunId())
	if err != nil {
		return nil, err
	}
	observedAt := assignmentLiveEventObservedAt(assignment, result)
	requests, err := c.buildScoredModelRoleInvocationPublishRequests(assignment, result, observedAt, completed, total)
	if err != nil {
		return nil, err
	}
	if signal, ok := buildAssignmentPassMetricSignal(assignment, result, observedAt, completed, total); ok {
		event, err := ProjectMetricAvailabilityEvent(signal)
		if err != nil {
			return nil, err
		}
		body, err := MarshalPublicLiveEvent(event)
		if err != nil {
			return nil, err
		}
		if err := publicdisclosure.ValidatePublicFeedRecord(models.PublicFeedRecordTypeEvent, body); err != nil {
			return nil, fmt.Errorf("evaluation: build assignment live event publish requests: validate metric event: %w", err)
		}
		requests = append(requests, campaignFeedPublishRequest{
			IdempotencyKey: MetricAvailabilityIdempotencyKey(signal.RunID, signal.AssignmentID, signal.MetricID),
			RecordType:     models.PublicFeedRecordTypeEvent,
			Body:           body,
		})
	}
	return requests, nil
}

// PublishAssignmentInvocationLiveEvents emits the planned model-role invocation
// stage_updated event when an assignment transitions to running.
func (c *CampaignPublicationCoordinator) PublishAssignmentInvocationLiveEvents(ctx context.Context, assignment *evalv1.EvaluationAssignment) error {
	if c == nil || c.store == nil || c.files == nil || c.exporter == nil || assignment == nil {
		return fmt.Errorf("evaluation: publish assignment invocation live events: %w", constants.ErrMissingRequiredField)
	}
	completed, total, err := c.assignmentLiveEventProgress(ctx, assignment.GetRunId())
	if err != nil {
		return err
	}
	observedAt := assignmentLiveEventObservedAt(assignment, nil)
	requests, err := c.buildPlannedModelRoleInvocationPublishRequests(assignment, observedAt, completed, total)
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		return nil
	}
	_, err = c.exportFeedRecords(ctx, assignment.GetRunId(), requests)
	return err
}

// PublishAssignmentScoredInferenceLiveEvents emits stage_updated rows when one
// or more scored model inferences are first observed for an in-flight assignment.
func (c *CampaignPublicationCoordinator) PublishAssignmentScoredInferenceLiveEvents(ctx context.Context, assignment *evalv1.EvaluationAssignment, result *evalv1.EvaluationAssignmentResult) error {
	if c == nil || c.store == nil || c.files == nil || c.exporter == nil || assignment == nil || result == nil {
		return fmt.Errorf("evaluation: publish assignment scored inference live events: %w", constants.ErrMissingRequiredField)
	}
	completed, total, err := c.assignmentLiveEventProgress(ctx, assignment.GetRunId())
	if err != nil {
		return err
	}
	observedAt := assignmentLiveEventObservedAt(assignment, result)
	requests, err := c.buildScoredModelRoleInvocationPublishRequests(assignment, result, observedAt, completed, total)
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		return nil
	}
	_, err = c.exportFeedRecords(ctx, assignment.GetRunId(), requests)
	return err
}

func (c *CampaignPublicationCoordinator) buildPlannedModelRoleInvocationPublishRequests(
	assignment *evalv1.EvaluationAssignment,
	observedAt string,
	completed int,
	total int,
) ([]campaignFeedPublishRequest, error) {
	requests := make([]campaignFeedPublishRequest, 0, 1)
	for _, signal := range buildPlannedModelRoleInvocationSignals(assignment, observedAt, completed, total) {
		event, err := ProjectModelRoleInvocationEvent(signal)
		if err != nil {
			return nil, err
		}
		body, err := MarshalPublicLiveEvent(event)
		if err != nil {
			return nil, err
		}
		if err := publicdisclosure.ValidatePublicFeedRecord(models.PublicFeedRecordTypeEvent, body); err != nil {
			return nil, fmt.Errorf("evaluation: build planned model role invocation publish requests: validate invocation event: %w", err)
		}
		requests = append(requests, campaignFeedPublishRequest{
			IdempotencyKey: ModelRoleInvocationIdempotencyKey(signal.RunID, signal.AssignmentID, string(signal.Role)),
			RecordType:     models.PublicFeedRecordTypeEvent,
			Body:           body,
		})
	}
	return requests, nil
}

func (c *CampaignPublicationCoordinator) buildScoredModelRoleInvocationPublishRequests(
	assignment *evalv1.EvaluationAssignment,
	result *evalv1.EvaluationAssignmentResult,
	observedAt string,
	completed int,
	total int,
) ([]campaignFeedPublishRequest, error) {
	requests := make([]campaignFeedPublishRequest, 0, len(result.GetModelInferences()))
	for _, signal := range buildScoredModelRoleInvocationSignals(assignment, result, observedAt, completed, total) {
		event, err := ProjectModelRoleInvocationEvent(signal)
		if err != nil {
			return nil, err
		}
		body, err := MarshalPublicLiveEvent(event)
		if err != nil {
			return nil, err
		}
		if err := publicdisclosure.ValidatePublicFeedRecord(models.PublicFeedRecordTypeEvent, body); err != nil {
			return nil, fmt.Errorf("evaluation: build scored model role invocation publish requests: validate invocation event: %w", err)
		}
		requests = append(requests, campaignFeedPublishRequest{
			IdempotencyKey: ScoredModelRoleInvocationIdempotencyKey(signal.RunID, signal.AssignmentID, string(signal.Role)),
			RecordType:     models.PublicFeedRecordTypeEvent,
			Body:           body,
		})
	}
	return requests, nil
}

func (c *CampaignPublicationCoordinator) assignmentLiveEventProgress(ctx context.Context, runID string) (completed int, total int, err error) {
	assignments, err := c.store.ListAssignments(ctx, runID)
	if err != nil {
		return 0, 0, err
	}
	results := make(map[string]*evalv1.EvaluationAssignmentResult, len(assignments))
	for _, assignment := range assignments {
		exists, err := c.store.AssignmentResultExists(ctx, runID, assignment.GetAssignmentId())
		if err != nil {
			return 0, 0, err
		}
		if !exists {
			continue
		}
		result, err := c.store.LoadAssignmentResult(ctx, runID, assignment.GetAssignmentId())
		if err != nil {
			return 0, 0, err
		}
		results[assignment.GetAssignmentId()] = result
	}
	state, err := CollectRunAggregateState(assignments, results)
	if err != nil {
		return 0, 0, err
	}
	return int(state.Terminal), int(state.Scheduled), nil
}

func assignmentLiveEventObservedAt(assignment *evalv1.EvaluationAssignment, result *evalv1.EvaluationAssignmentResult) string {
	if result.GetCompletedAt() != nil {
		return result.GetCompletedAt().AsTime().UTC().Format(time.RFC3339Nano)
	}
	if assignment.GetCompletedAt() != nil {
		return assignment.GetCompletedAt().AsTime().UTC().Format(time.RFC3339Nano)
	}
	if assignment.GetStartedAt() != nil {
		return assignment.GetStartedAt().AsTime().UTC().Format(time.RFC3339Nano)
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func buildPlannedModelRoleInvocationSignals(
	assignment *evalv1.EvaluationAssignment,
	observedAt string,
	completed int,
	total int,
) []PublicModelRoleInvocationSignal {
	pairs, err := assignmentVariantRolePairs(assignment)
	if err != nil {
		return nil
	}
	signals := make([]PublicModelRoleInvocationSignal, 0, len(pairs))
	for _, pair := range pairs {
		signals = append(signals, PublicModelRoleInvocationSignal{
			RunID:        assignment.GetRunId(),
			AssignmentID: assignment.GetAssignmentId(),
			VariantID:    pair.variantID,
			Role:         models.ModelRole(pair.role),
			TaskID:       assignment.GetScenarioId(),
			ObservedAt:   observedAt,
			EventID:      ModelRoleInvocationIdempotencyKey(assignment.GetRunId(), assignment.GetAssignmentId(), pair.role) + ":event",
			Completed:    completed,
			Total:        total,
		})
	}
	return signals
}

func buildScoredModelRoleInvocationSignals(
	assignment *evalv1.EvaluationAssignment,
	result *evalv1.EvaluationAssignmentResult,
	observedAt string,
	completed int,
	total int,
) []PublicModelRoleInvocationSignal {
	seen := make(map[string]struct{})
	signals := make([]PublicModelRoleInvocationSignal, 0, len(result.GetModelInferences()))
	for _, record := range reportedModelInferences(result) {
		if record == nil {
			continue
		}
		roleLabel, err := modelCampaignRoleLabel(record.GetModelRole())
		if err != nil {
			continue
		}
		role := models.ModelRole(roleLabel)
		if _, ok := seen[roleLabel]; ok {
			continue
		}
		seen[roleLabel] = struct{}{}
		variantID := ""
		if record.GetModelVariant() != nil {
			variantID = record.GetModelVariant().GetVariantId()
		}
		if variantID == "" {
			variantID, err = variantIDForAssignmentModelRole(assignment, record.GetModelRole())
			if err != nil {
				continue
			}
		}
		signals = append(signals, PublicModelRoleInvocationSignal{
			RunID:        assignment.GetRunId(),
			AssignmentID: assignment.GetAssignmentId(),
			VariantID:    variantID,
			Role:         role,
			TaskID:       assignment.GetScenarioId(),
			ObservedAt:   observedAt,
			EventID:      ScoredModelRoleInvocationIdempotencyKey(assignment.GetRunId(), assignment.GetAssignmentId(), roleLabel) + ":event",
			Completed:    completed,
			Total:        total,
			MetricDelta:  modelInferenceMetricDelta(record),
		})
	}
	return signals
}

func modelInferenceMetricDelta(record *evalv1.ModelInferenceRecord) map[string]any {
	if record == nil || record.GetUsageAvailability() != evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED {
		return nil
	}
	delta := map[string]any{
		"input_tokens":  map[string]any{"value": record.GetPromptTokens()},
		"output_tokens": map[string]any{"value": record.GetCompletionTokens()},
	}
	if generationNanos := record.GetGenerationDurationNanos(); generationNanos > 0 {
		delta["latency_ms"] = map[string]any{"value": float64(generationNanos) / float64(time.Millisecond)}
		if completionTokens := record.GetCompletionTokens(); completionTokens > 0 {
			delta["tokens_per_second"] = map[string]any{
				"value": float64(completionTokens) / (float64(generationNanos) / float64(time.Second)),
			}
		}
	}
	return delta
}

func reportedModelInferences(result *evalv1.EvaluationAssignmentResult) []*evalv1.ModelInferenceRecord {
	records := make([]*evalv1.ModelInferenceRecord, 0, len(result.GetModelInferences()))
	for _, record := range result.GetModelInferences() {
		if record == nil {
			continue
		}
		if record.GetUsageAvailability() != evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED {
			continue
		}
		records = append(records, record)
	}
	return records
}

func buildAssignmentPassMetricSignal(
	assignment *evalv1.EvaluationAssignment,
	result *evalv1.EvaluationAssignmentResult,
	observedAt string,
	completed int,
	total int,
) (PublicMetricAvailabilitySignal, bool) {
	status := DerivePublicSummaryStatus(result)
	if status == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSPECIFIED {
		return PublicMetricAvailabilitySignal{}, false
	}
	variantID, err := primaryVariantIDForAssignment(assignment)
	if err != nil {
		return PublicMetricAvailabilitySignal{}, false
	}
	rate := 0.0
	numerator := 0
	if status == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
		rate = 1
		numerator = 1
	}
	return PublicMetricAvailabilitySignal{
		RunID:        assignment.GetRunId(),
		AssignmentID: assignment.GetAssignmentId(),
		VariantID:    variantID,
		MetricID:     "pass_rate",
		Numerator:    numerator,
		Denominator:  1,
		Rate:         &rate,
		ObservedAt:   observedAt,
		EventID:      MetricAvailabilityIdempotencyKey(assignment.GetRunId(), assignment.GetAssignmentId(), "pass_rate") + ":event",
		Completed:    completed,
		Total:        total,
	}, true
}
