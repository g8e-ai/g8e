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
	"sort"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	explorerViewSchemaVersion = "1.3.0"
	campaignSourceRevision    = "g8e-eval-campaign"
	northStarSuiteID          = "north-star-25"
)

// CampaignViewRecord is one disclosure-safe explorer snapshot record published
// directly to the public mirror without a CampaignProjectionEnvelope wrapper.
type CampaignViewRecord struct {
	IdempotencyKey string
	Body           []byte
}

type runAggregateState struct {
	Scheduled      uint32
	Terminal       uint32
	Passed         uint32
	Failed         uint32
	ModelCount     uint32
	VariantRoles   map[string]*variantRoleAggregate
	EvaluatedCount uint32
}

type variantRoleAggregate struct {
	VariantID string
	Role      string
	Scheduled uint32
	Terminal  uint32
	Passed    uint32
	Failed    uint32
	Outcomes  map[string]uint32
}

// CampaignDatasetID returns the explorer dataset id for one live campaign run.
func CampaignDatasetID(runID string) string {
	return "ds-live-" + runID
}

// CatalogSnapshotIdempotencyKey returns the publication key for one catalog
// aggregate revision keyed by scheduled and terminal assignment counts.
func CatalogSnapshotIdempotencyKey(runID string, scheduled, terminal uint32) string {
	return fmt.Sprintf("%s:aggregate:catalog:s%d:t%d", runID, scheduled, terminal)
}

// ModelSummaryIdempotencyKey returns the publication key for one variant-role
// aggregate revision keyed by terminal assignment count for that bucket.
func ModelSummaryIdempotencyKey(runID, variantID, role string, terminal uint32) string {
	return fmt.Sprintf("%s:aggregate:model:%s:%s:t%d", runID, variantID, role, terminal)
}

// MethodologySnapshotIdempotencyKey returns the publication key for one
// methodology aggregate revision keyed by terminal assignment count.
func MethodologySnapshotIdempotencyKey(runID string, terminal uint32) string {
	return fmt.Sprintf("%s:aggregate:methodology:t%d", runID, terminal)
}

// CollectRunAggregateState derives explorer aggregate counters from canonical
// assignments and terminal results for one homogeneous model-role run.
func CollectRunAggregateState(assignments []*evalv1.EvaluationAssignment, results map[string]*evalv1.EvaluationAssignmentResult) (*runAggregateState, error) {
	if len(assignments) == 0 {
		return nil, fmt.Errorf("evaluation: collect run aggregate state: %w", constants.ErrMissingRequiredField)
	}
	state := &runAggregateState{
		Scheduled:    uint32(len(assignments)),
		VariantRoles: map[string]*variantRoleAggregate{},
	}
	scheduledVariants := make(map[string]struct{})
	evaluatedVariants := make(map[string]struct{})
	for _, assignment := range assignments {
		variantID, role, err := homogeneousVariantRole(assignment)
		if err != nil {
			return nil, err
		}
		scheduledVariants[variantID] = struct{}{}
		bucket := ensureVariantRoleAggregate(state, variantID, role)
		bucket.Scheduled++
		result := results[assignment.GetAssignmentId()]
		if result == nil {
			continue
		}
		state.Terminal++
		terminalStatus := deriveExplorerTerminalStatus(result)
		passed := terminalStatus == "completed"
		if passed {
			state.Passed++
			bucket.Passed++
		} else {
			state.Failed++
			bucket.Failed++
		}
		bucket.Terminal++
		bucket.Outcomes[terminalStatus]++
		evaluatedVariants[variantID] = struct{}{}
	}
	state.ModelCount = uint32(len(scheduledVariants))
	state.EvaluatedCount = uint32(len(evaluatedVariants))
	return state, nil
}

// BuildRunAggregateViewRecords materializes catalog, model, and methodology
// explorer snapshot records for one live campaign run.
func BuildRunAggregateViewRecords(runID string, state *runAggregateState, observedAt time.Time) ([]CampaignViewRecord, error) {
	if runID == "" || state == nil || state.Scheduled == 0 {
		return nil, fmt.Errorf("evaluation: build run aggregate view records: %w", constants.ErrMissingRequiredField)
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	observed := observedAt.UTC().Format(time.RFC3339Nano)
	datasetID := CampaignDatasetID(runID)
	records := make([]CampaignViewRecord, 0, 2+len(state.VariantRoles))

	catalogBody, err := marshalCanonicalViewRecord(buildCatalogSnapshotRecord(datasetID, runID, observed, state))
	if err != nil {
		return nil, err
	}
	records = append(records, CampaignViewRecord{
		IdempotencyKey: CatalogSnapshotIdempotencyKey(runID, state.Scheduled, state.Terminal),
		Body:           catalogBody,
	})

	keys := make([]string, 0, len(state.VariantRoles))
	for key := range state.VariantRoles {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		bucket := state.VariantRoles[key]
		modelBody, err := marshalCanonicalViewRecord(buildModelSummaryRecord(datasetID, observed, bucket))
		if err != nil {
			return nil, err
		}
		records = append(records, CampaignViewRecord{
			IdempotencyKey: ModelSummaryIdempotencyKey(runID, bucket.VariantID, bucket.Role, bucket.Terminal),
			Body:           modelBody,
		})
	}

	methodologyBody, err := marshalCanonicalViewRecord(buildMethodologySnapshotRecord(datasetID, observed))
	if err != nil {
		return nil, err
	}
	records = append(records, CampaignViewRecord{
		IdempotencyKey: MethodologySnapshotIdempotencyKey(runID, state.Terminal),
		Body:           methodologyBody,
	})
	return records, nil
}

func buildCatalogSnapshotRecord(datasetID, runID, observedAt string, state *runAggregateState) map[string]any {
	return map[string]any{
		"schema_version":           explorerViewSchemaVersion,
		"kind":                     "catalog_snapshot",
		"dataset_id":               datasetID,
		"dataset_kind":             "live_run",
		"quality_state":            "live_in_progress",
		"observed_at":              observedAt,
		"source_revision_label":    campaignSourceRevision,
		"title":                    fmt.Sprintf("Live smoke run (%s)", runID),
		"description":              "Homogeneous full-pipeline model-role evaluation over the frozen north-star-25 catalog. Values are provisional while assignments are still executing.",
		"limitations":              catalogSnapshotLimitations(),
		"model_count":              state.ModelCount,
		"evaluated_count":          state.EvaluatedCount,
		"suite_count":              1,
		"run_count":                1,
		"assignment_count":         state.Scheduled,
		"provider_request_count":   state.Terminal,
		"provider_token_count":     0,
		"retry_count":              0,
		"verifier_passed_count":    0,
		"verifier_failed_count":    0,
		"generated_at":             observedAt,
	}
}

func buildModelSummaryRecord(datasetID, observedAt string, bucket *variantRoleAggregate) map[string]any {
	scheduled := bucket.Scheduled
	terminal := bucket.Terminal
	coverage := 0.0
	if scheduled > 0 {
		coverage = float64(terminal) / float64(scheduled)
	}
	record := map[string]any{
		"schema_version":        explorerViewSchemaVersion,
		"kind":                  "model_summary",
		"dataset_id":            datasetID,
		"quality_state":         qualityStateForModelSummary(terminal),
		"observed_at":           observedAt,
		"source_revision_label": campaignSourceRevision,
		"variant_id":            bucket.VariantID,
		"display_name":          displayNameForVariant(bucket.VariantID),
		"served_model_tag":      servedTagForVariant(bucket.VariantID),
		"role":                  bucket.Role,
		"backend_provider_class": "ollama",
		"inventory_only":        terminal == 0,
		"evaluation_coverage":   coverage,
	}
	if terminal > 0 {
		passEstimate := float64(bucket.Passed) / float64(terminal)
		record["pass_rate"] = map[string]any{
			"estimate":    passEstimate,
			"lower":       passEstimate,
			"upper":       passEstimate,
			"denominator": terminal,
		}
		record["terminal_outcomes"] = terminalOutcomesRecord(bucket.Outcomes)
	} else {
		record["unavailable_reasons"] = []string{"awaiting terminal assignments"}
	}
	return record
}

func buildMethodologySnapshotRecord(datasetID, observedAt string) map[string]any {
	return map[string]any{
		"schema_version":        explorerViewSchemaVersion,
		"kind":                  "methodology_snapshot",
		"dataset_id":            datasetID,
		"quality_state":         "live_in_progress",
		"observed_at":           observedAt,
		"source_revision_label": campaignSourceRevision,
		"metric_definitions":    methodologyMetricDefinitions(),
		"suite_definitions": []map[string]any{
			{
				"suite_id":     northStarSuiteID,
				"display_name": "North Star 25",
				"task_count":   25,
				"description":  "Frozen 25-scenario catalog covering instruction adherence, tool use, analysis, routing, verification, security, recovery, and final response.",
			},
		},
		"limitations": methodologyLimitations(),
	}
}

func catalogSnapshotLimitations() []string {
	return []string{
		"Live values are provisional and update as assignments complete.",
		"Model aggregates reflect designated role responsibility inside the production chat pipeline, not a provider-only benchmark.",
		"Resource telemetry remains unavailable until provider-boundary observation is published.",
	}
}

func methodologyMetricDefinitions() []map[string]any {
	return []map[string]any{
		{
			"key":                    "pass_rate",
			"name":                   "Pass rate",
			"unit":                   "proportion",
			"direction":              "higher_is_better",
			"denominator":            "terminal homogeneous model-role assignments for the variant and designated role",
			"missing_value_behavior": "excluded until a terminal assignment exists; never rendered as zero",
			"aggregation":            "mean over terminal assignments within the active live dataset",
			"uncertainty_method":     "point estimate while the smoke campaign is in progress",
			"explanation":            "The fraction of terminal assignments that passed for one frozen model variant acting in one designated role through the production chat pipeline.",
		},
		{
			"key":                    "evaluation_coverage",
			"name":                   "Evaluation coverage",
			"unit":                   "proportion",
			"direction":              "higher_is_better",
			"denominator":            "scheduled assignments for the variant and designated role",
			"missing_value_behavior": "rendered as zero only when no assignments are scheduled",
			"aggregation":            "terminal assignments divided by scheduled assignments",
			"uncertainty_method":     "none (descriptive)",
			"explanation":            "How much of the scheduled smoke matrix has reached a terminal public result for this variant and role.",
		},
	}
}

func methodologyLimitations() []string {
	return []string{
		"Live smoke values are provisional until the campaign completes and verification runs.",
		"Homogeneous model-role and heterogeneous system leaderboards remain separate datasets.",
		"GPU and system efficiency metrics remain unavailable until provider-boundary observation is published.",
	}
}

func terminalOutcomesRecord(outcomes map[string]uint32) map[string]uint32 {
	record := map[string]uint32{
		"completed":        0,
		"model_failed":     0,
		"grader_failed":    0,
		"invalid_evidence": 0,
		"stopped":          0,
	}
	for key, count := range outcomes {
		record[key] = count
	}
	return record
}

func qualityStateForModelSummary(terminal uint32) string {
	if terminal > 0 {
		return "live_in_progress"
	}
	return "not_evaluated"
}

func displayNameForVariant(variantID string) string {
	parts := strings.Split(variantID, "-")
	words := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		words = append(words, strings.ToUpper(part[:1])+part[1:])
	}
	return strings.Join(words, " ")
}

func servedTagForVariant(variantID string) string {
	parts := strings.Split(variantID, "-")
	if len(parts) >= 2 {
		family := strings.Join(parts[:len(parts)-1], "")
		size := parts[len(parts)-1]
		return family + ":" + size
	}
	return variantID
}

func homogeneousVariantRole(assignment *evalv1.EvaluationAssignment) (string, string, error) {
	homogeneous, ok := assignment.GetTarget().(*evalv1.EvaluationAssignment_Homogeneous)
	if !ok || homogeneous.Homogeneous == nil || homogeneous.Homogeneous.GetCandidateVariant() == nil {
		return "", "", fmt.Errorf("evaluation: homogeneous variant role lookup: %w", constants.ErrMissingRequiredField)
	}
	role, err := modelCampaignRoleLabel(homogeneous.Homogeneous.GetDesignatedRole())
	if err != nil {
		return "", "", err
	}
	return homogeneous.Homogeneous.GetCandidateVariant().GetVariantId(), role, nil
}

func ensureVariantRoleAggregate(state *runAggregateState, variantID, role string) *variantRoleAggregate {
	key := variantID + ":" + role
	bucket := state.VariantRoles[key]
	if bucket == nil {
		bucket = &variantRoleAggregate{
			VariantID: variantID,
			Role:      role,
			Outcomes:  map[string]uint32{},
		}
		state.VariantRoles[key] = bucket
	}
	return bucket
}

func deriveExplorerTerminalStatus(result *evalv1.EvaluationAssignmentResult) string {
	switch result.GetLifecycleStatus() {
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_STOPPED:
		return "stopped"
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL:
		return "grader_failed"
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_POLICY_REJECTED:
		return "invalid_evidence"
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED:
		if DerivePublicSummaryStatus(result) == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
			return "completed"
		}
		return "model_failed"
	default:
		return "model_failed"
	}
}

func marshalCanonicalViewRecord(record map[string]any) ([]byte, error) {
	body, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("evaluation: marshal campaign view record: %w", err)
	}
	var compact json.RawMessage
	if err := json.Unmarshal(body, &compact); err != nil {
		return nil, fmt.Errorf("evaluation: marshal campaign view record: %w", err)
	}
	return compact, nil
}
