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
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/models"
)

// PublicModelRoleInvocationSignal is the disclosure-safe input for projecting a
// model-role invocation into a public live event. Provider-boundary fields from
// observe payloads (served_model_tag, backend_name, exact quantization) are
// excluded by design.
type PublicModelRoleInvocationSignal struct {
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
// Keys are metric ids (pass_rate, input_tokens, latency_ms, tokens_per_second).
// Values are the disclosure-safe MetricValue shape.
type PublicMetricDelta map[string]PublicMetricValue

// PublicLiveEvent is one disclosure-safe explorer live event body.
type PublicLiveEvent struct {
	SchemaVersion       string            `json:"schema_version"`
	Kind                string            `json:"kind"`
	DatasetID           string            `json:"dataset_id"`
	QualityState        string            `json:"quality_state"`
	ObservedAt          string            `json:"observed_at"`
	SourceRevisionLabel string            `json:"source_revision_label,omitempty"`
	EventID             string            `json:"event_id"`
	RunID               string            `json:"run_id"`
	AssignmentID        string            `json:"assignment_id,omitempty"`
	VariantID           string            `json:"variant_id,omitempty"`
	Role                string            `json:"role,omitempty"`
	LifecycleStatus     string            `json:"lifecycle_status"`
	Completed           int               `json:"completed"`
	Total               int               `json:"total"`
	StageLabel          string            `json:"stage_label,omitempty"`
	TaskID              string            `json:"task_id,omitempty"`
	MetricDelta         PublicMetricDelta `json:"metric_delta,omitempty"`
}

// PublicMetricAvailabilitySignal is the disclosure-safe input for projecting
// per-assignment metric availability into a public metric_updated event.
type PublicMetricAvailabilitySignal struct {
	RunID        string
	AssignmentID string
	VariantID    string
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
func ProjectModelRoleInvocationEvent(signal PublicModelRoleInvocationSignal) (PublicLiveEvent, error) {
	if signal.RunID == "" || signal.AssignmentID == "" || signal.VariantID == "" || signal.EventID == "" || signal.ObservedAt == "" {
		return PublicLiveEvent{}, fmt.Errorf("evaluation: project model role invocation event: missing required field")
	}
	if signal.Role == "" {
		return PublicLiveEvent{}, fmt.Errorf("evaluation: project model role invocation event: missing role")
	}
	if signal.Completed < 0 || signal.Total < 0 || signal.Completed > signal.Total {
		return PublicLiveEvent{}, fmt.Errorf("evaluation: project model role invocation event: invalid progress")
	}
	stageParts := []string{"model role invoked", string(signal.Role), signal.VariantID}
	if signal.TaskID != "" {
		stageParts = append(stageParts, signal.TaskID)
	}
	return PublicLiveEvent{
		SchemaVersion:       explorerViewSchemaVersion,
		Kind:                "stage_updated",
		DatasetID:           CampaignDatasetID(signal.RunID),
		QualityState:        "live_in_progress",
		ObservedAt:          signal.ObservedAt,
		SourceRevisionLabel: campaignSourceRevision,
		EventID:             signal.EventID,
		RunID:               signal.RunID,
		AssignmentID:        signal.AssignmentID,
		VariantID:           signal.VariantID,
		Role:                string(signal.Role),
		LifecycleStatus:     "running",
		Completed:           signal.Completed,
		Total:               signal.Total,
		StageLabel:          strings.Join(stageParts, " · "),
		TaskID:              signal.TaskID,
		MetricDelta:         signal.MetricDelta,
	}, nil
}

// ProjectMetricAvailabilityEvent maps a disclosure-safe metric availability
// signal to a metric_updated live event body for record_type "event"
// publication.
func ProjectMetricAvailabilityEvent(signal PublicMetricAvailabilitySignal) (PublicLiveEvent, error) {
	if signal.RunID == "" || signal.AssignmentID == "" || signal.VariantID == "" || signal.MetricID == "" || signal.EventID == "" || signal.ObservedAt == "" {
		return PublicLiveEvent{}, fmt.Errorf("evaluation: project metric availability event: missing required field")
	}
	if signal.Completed < 0 || signal.Total < 0 || signal.Completed > signal.Total {
		return PublicLiveEvent{}, fmt.Errorf("evaluation: project metric availability event: invalid progress")
	}
	return PublicLiveEvent{
		SchemaVersion:       explorerViewSchemaVersion,
		Kind:                "metric_updated",
		DatasetID:           CampaignDatasetID(signal.RunID),
		QualityState:        "live_in_progress",
		ObservedAt:          signal.ObservedAt,
		SourceRevisionLabel: campaignSourceRevision,
		EventID:             signal.EventID,
		RunID:               signal.RunID,
		AssignmentID:        signal.AssignmentID,
		VariantID:           signal.VariantID,
		LifecycleStatus:     "running",
		Completed:           signal.Completed,
		Total:               signal.Total,
		MetricDelta: PublicMetricDelta{
			signal.MetricID: metricAvailabilityValue(signal.Numerator, signal.Denominator, signal.Rate),
		},
	}, nil
}

// MarshalPublicLiveEvent serializes a projected live event for signed export.
func MarshalPublicLiveEvent(event PublicLiveEvent) ([]byte, error) {
	return json.Marshal(event)
}

func metricAvailabilityValue(numerator, denominator int, rate *float64) PublicMetricValue {
	if denominator <= 0 {
		return PublicMetricValue{UnavailableReason: "no_scored_calls"}
	}
	if rate != nil {
		return *publicMetricValue(*rate)
	}
	return *publicMetricValue(float64(numerator) / float64(denominator))
}
