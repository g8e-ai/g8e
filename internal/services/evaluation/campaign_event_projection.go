// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// PublicModelRoleInvocationSignal is the disclosure-safe input for projecting a
// model-role invocation into a public live event. Provider-boundary fields from
// observe payloads (served_model_tag, backend_name, exact quantization) are
// excluded by design.
type PublicModelRoleInvocationSignal struct {
	Release      CampaignRelease
	RunID        string
	AssignmentID string
	VariantID    string
	Role         models.ModelRole
	TaskID       string
	ObservedAt   string
	EventID      string
	Completed    int
	Total        int
	MetricDelta  PublicMetricDelta
}

// PublicMetricDelta is the explorer live-event metric_delta object.
// Keys are metric ids (pass_rate, input_tokens, latency_ms, tokens_per_second, retries).
// Values are the disclosure-safe evalv1.PublicLiveMetricValue shape.
type PublicMetricDelta map[string]*evalv1.PublicLiveMetricValue

// PublicMetricAvailabilitySignal is the disclosure-safe input for projecting
// per-assignment metric availability into a public metric_updated event. Role
// is the assignment's designated role: the metric grades that role.
type PublicMetricAvailabilitySignal struct {
	Release      CampaignRelease
	RunID        string
	AssignmentID string
	VariantID    string
	Role         models.ModelRole
	MetricID     string
	Numerator    int
	Denominator  int
	Rate         *float64
	ObservedAt   string
	EventID      string
	Completed    int
	Total        int
}

// HeadlineMetricID reports whether metricID is one of the explorer 1.5.0
// run-level headline metrics that metric_updated may refresh live.
func HeadlineMetricID(metricID string) bool {
	switch metricID {
	case "pass_rate", "latency_p50_ms", "output_throughput_p50_tokens_per_second":
		return true
	default:
		return false
	}
}

// ProjectModelRoleInvocationEvent maps a disclosure-safe invocation signal to
// a stage_updated live event body for record_type "event" publication.
func ProjectModelRoleInvocationEvent(signal PublicModelRoleInvocationSignal) (*evalv1.PublicLiveEvent, error) {
	if signal.RunID == "" || signal.AssignmentID == "" || signal.VariantID == "" || signal.EventID == "" || signal.ObservedAt == "" {
		return nil, fmt.Errorf("evaluation: project model role invocation event: %w", constants.ErrMissingRequiredField)
	}
	if signal.Role == "" {
		return nil, fmt.Errorf("evaluation: project model role invocation event: role: %w", constants.ErrMissingRequiredField)
	}
	if signal.Completed < 0 || signal.Total < 0 || signal.Completed > signal.Total {
		return nil, fmt.Errorf("evaluation: project model role invocation event: %w", constants.ErrEvaluationLiveEventProgressInvalid)
	}
	stageParts := []string{"model role invoked", string(signal.Role), signal.VariantID}
	if signal.TaskID != "" {
		stageParts = append(stageParts, signal.TaskID)
	}
	return &evalv1.PublicLiveEvent{
		SchemaVersion:       explorerViewSchemaVersion,
		Kind:                "stage_updated",
		DatasetId:           CampaignDatasetID(signal.RunID),
		QualityState:        "live_in_progress",
		ObservedAt:          signal.ObservedAt,
		SourceRevisionLabel: campaignSourceRevision,
		Release:             signal.Release.Release,
		ReleaseBasis:        signal.Release.publicBasis(),
		SourceRevision:      signal.Release.SourceRevision,
		EventId:             signal.EventID,
		RunId:               signal.RunID,
		AssignmentId:        signal.AssignmentID,
		VariantId:           signal.VariantID,
		Role:                string(signal.Role),
		LifecycleStatus:     "running",
		Completed:           proto.Uint32(uint32(signal.Completed)),
		Total:               proto.Uint32(uint32(signal.Total)),
		StageLabel:          strings.Join(stageParts, " · "),
		TaskId:              signal.TaskID,
		MetricDelta:         signal.MetricDelta,
	}, nil
}

// ProjectMetricAvailabilityEvent maps a disclosure-safe metric availability
// signal to a metric_updated live event body for record_type "event"
// publication.
func ProjectMetricAvailabilityEvent(signal PublicMetricAvailabilitySignal) (*evalv1.PublicLiveEvent, error) {
	if signal.RunID == "" || signal.AssignmentID == "" || signal.VariantID == "" || signal.MetricID == "" || signal.EventID == "" || signal.ObservedAt == "" {
		return nil, fmt.Errorf("evaluation: project metric availability event: %w", constants.ErrMissingRequiredField)
	}
	if signal.Role == "" {
		return nil, fmt.Errorf("evaluation: project metric availability event: role: %w", constants.ErrMissingRequiredField)
	}
	if signal.Completed < 0 || signal.Total < 0 || signal.Completed > signal.Total {
		return nil, fmt.Errorf("evaluation: project metric availability event: %w", constants.ErrEvaluationLiveEventProgressInvalid)
	}
	return &evalv1.PublicLiveEvent{
		SchemaVersion:       explorerViewSchemaVersion,
		Kind:                "metric_updated",
		DatasetId:           CampaignDatasetID(signal.RunID),
		QualityState:        "live_in_progress",
		ObservedAt:          signal.ObservedAt,
		SourceRevisionLabel: campaignSourceRevision,
		Release:             signal.Release.Release,
		ReleaseBasis:        signal.Release.publicBasis(),
		SourceRevision:      signal.Release.SourceRevision,
		EventId:             signal.EventID,
		RunId:               signal.RunID,
		AssignmentId:        signal.AssignmentID,
		VariantId:           signal.VariantID,
		Role:                string(signal.Role),
		LifecycleStatus:     "running",
		Completed:           proto.Uint32(uint32(signal.Completed)),
		Total:               proto.Uint32(uint32(signal.Total)),
		MetricDelta: PublicMetricDelta{
			signal.MetricID: metricAvailabilityValue(signal.Numerator, signal.Denominator, signal.Rate),
		},
	}, nil
}

// MarshalPublicLiveEvent serializes a projected live event for signed export.
// It is the one encoder of live event bodies: protobuf-owned JSON goes through
// the canonical owner, so a zero progress count or metric value is emitted
// (both fields track presence) instead of being dropped.
func MarshalPublicLiveEvent(event *evalv1.PublicLiveEvent) ([]byte, error) {
	if event == nil {
		return nil, fmt.Errorf("evaluation: marshal public live event: %w", constants.ErrMissingRequiredField)
	}
	body, err := evalv1.MarshalCanonical(event)
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal public live event: %w", err)
	}
	return body, nil
}

func liveMetricValue(value float64) *evalv1.PublicLiveMetricValue {
	return &evalv1.PublicLiveMetricValue{Value: &value}
}

func metricAvailabilityValue(numerator, denominator int, rate *float64) *evalv1.PublicLiveMetricValue {
	if denominator <= 0 {
		return &evalv1.PublicLiveMetricValue{UnavailableReason: publicUnavailableReasonString(evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_NO_SCORED_CALLS)}
	}
	if rate != nil {
		return liveMetricValue(*rate)
	}
	return liveMetricValue(float64(numerator) / float64(denominator))
}
