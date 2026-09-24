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
func ProjectModelRoleInvocationEvent(signal PublicModelRoleInvocationSignal) (map[string]any, error) {
	if signal.RunID == "" || signal.AssignmentID == "" || signal.VariantID == "" || signal.EventID == "" || signal.ObservedAt == "" {
		return nil, fmt.Errorf("evaluation: project model role invocation event: missing required field")
	}
	if signal.Role == "" {
		return nil, fmt.Errorf("evaluation: project model role invocation event: missing role")
	}
	if signal.Completed < 0 || signal.Total < 0 || signal.Completed > signal.Total {
		return nil, fmt.Errorf("evaluation: project model role invocation event: invalid progress")
	}
	stageParts := []string{"model role invoked", string(signal.Role), signal.VariantID}
	if signal.TaskID != "" {
		stageParts = append(stageParts, signal.TaskID)
	}
	event := map[string]any{
		"schema_version":        explorerViewSchemaVersion,
		"kind":                  "stage_updated",
		"dataset_id":            CampaignDatasetID(signal.RunID),
		"quality_state":         "live_in_progress",
		"observed_at":           signal.ObservedAt,
		"source_revision_label": campaignSourceRevision,
		"event_id":              signal.EventID,
		"run_id":                signal.RunID,
		"assignment_id":         signal.AssignmentID,
		"variant_id":            signal.VariantID,
		"role":                  string(signal.Role),
		"lifecycle_status":      "running",
		"completed":             signal.Completed,
		"total":                 signal.Total,
		"stage_label":           strings.Join(stageParts, " · "),
	}
	if signal.TaskID != "" {
		event["task_id"] = signal.TaskID
	}
	return event, nil
}

// ProjectMetricAvailabilityEvent maps a disclosure-safe metric availability
// signal to a metric_updated live event body for record_type "event"
// publication.
func ProjectMetricAvailabilityEvent(signal PublicMetricAvailabilitySignal) (map[string]any, error) {
	if signal.RunID == "" || signal.AssignmentID == "" || signal.VariantID == "" || signal.MetricID == "" || signal.EventID == "" || signal.ObservedAt == "" {
		return nil, fmt.Errorf("evaluation: project metric availability event: missing required field")
	}
	if signal.Completed < 0 || signal.Total < 0 || signal.Completed > signal.Total {
		return nil, fmt.Errorf("evaluation: project metric availability event: invalid progress")
	}
	metricValue := metricAvailabilityValue(signal.Numerator, signal.Denominator, signal.Rate)
	event := map[string]any{
		"schema_version":        explorerViewSchemaVersion,
		"kind":                  "metric_updated",
		"dataset_id":            CampaignDatasetID(signal.RunID),
		"quality_state":         "live_in_progress",
		"observed_at":           signal.ObservedAt,
		"source_revision_label": campaignSourceRevision,
		"event_id":              signal.EventID,
		"run_id":                signal.RunID,
		"assignment_id":         signal.AssignmentID,
		"variant_id":            signal.VariantID,
		"lifecycle_status":      "running",
		"completed":             signal.Completed,
		"total":                 signal.Total,
		"metric_delta": map[string]any{
			signal.MetricID: metricValue,
		},
	}
	return event, nil
}

// MarshalPublicLiveEvent serializes a projected live event for signed export.
func MarshalPublicLiveEvent(event map[string]any) ([]byte, error) {
	return json.Marshal(event)
}

func metricAvailabilityValue(numerator, denominator int, rate *float64) map[string]any {
	if denominator <= 0 {
		return map[string]any{"unavailable_reason": "no_scored_calls"}
	}
	if rate != nil {
		return map[string]any{"value": *rate}
	}
	return map[string]any{"value": float64(numerator) / float64(denominator)}
}
