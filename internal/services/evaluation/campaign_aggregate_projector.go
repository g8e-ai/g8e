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
	"math"
	"sort"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	explorerViewSchemaVersion = "1.3.0"
	campaignSourceRevision    = "g8e-eval-campaign"
	standardSuiteID          = "north-star-25"
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

// EvaluationSummaryIdempotencyKey returns the publication key for one live
// evaluation_summary aggregate revision keyed by scheduled and terminal counts.
func EvaluationSummaryIdempotencyKey(runID string, scheduled, terminal uint32) string {
	return fmt.Sprintf("%s:aggregate:evaluation_summary:s%d:t%d", runID, scheduled, terminal)
}

// RunCompletionIdempotencyKey returns the publication key for the terminal
// evaluation_summary and completion aggregate revision for one run.
func RunCompletionIdempotencyKey(runID string) string {
	return runID + ":completion:final"
}

// RunVerificationIdempotencyKey returns the publication key for the post-verify
// evaluation_summary revision for one run.
func RunVerificationIdempotencyKey(runID string) string {
	return runID + ":verification:summary:v2"
}

// VerifierStateFromVerificationReport maps a persisted campaign verification
// report to the explorer verifier_state enum.
func VerifierStateFromVerificationReport(report *evalv1.EvaluationVerificationReport) string {
	if report != nil && report.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
		return "passed"
	}
	return "failed"
}

// RunAggregateComplete reports whether every scheduled assignment is settled:
// either a persisted result exists or the assignment lifecycle is terminal.
func RunAggregateComplete(assignments []*evalv1.EvaluationAssignment, results map[string]*evalv1.EvaluationAssignmentResult, state *runAggregateState) bool {
	if state == nil || state.Scheduled == 0 || len(assignments) != int(state.Scheduled) {
		return false
	}
	for _, assignment := range assignments {
		if !assignmentIsSettled(assignment, results[assignment.GetAssignmentId()] != nil) {
			return false
		}
	}
	return true
}

func assignmentIsSettled(assignment *evalv1.EvaluationAssignment, hasResult bool) bool {
	if hasResult {
		return true
	}
	return assignmentLifecycleIsTerminal(assignment.GetLifecycleStatus())
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
			if !assignmentLifecycleIsTerminal(assignment.GetLifecycleStatus()) {
				continue
			}
			terminalStatus := lifecycleTerminalOutcome(assignment.GetLifecycleStatus())
			state.Terminal++
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

// runAggregateSettled reports whether every scheduled assignment has reached a
// terminal public outcome in the aggregate counters.
func runAggregateSettled(state *runAggregateState) bool {
	return state != nil && state.Scheduled > 0 && state.Terminal >= state.Scheduled
}

// BuildRunAggregateViewRecords materializes live evaluation_summary, catalog,
// model, and methodology explorer snapshot records for one campaign run.
func BuildRunAggregateViewRecords(run *evalv1.EvaluationRun, state *runAggregateState, observedAt time.Time) ([]CampaignViewRecord, error) {
	if run == nil || run.GetRunId() == "" || state == nil || state.Scheduled == 0 {
		return nil, fmt.Errorf("evaluation: build run aggregate view records: %w", constants.ErrMissingRequiredField)
	}
	runID := run.GetRunId()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	observed := observedAt.UTC().Format(time.RFC3339Nano)
	datasetID := CampaignDatasetID(runID)
	settled := runAggregateSettled(state)
	records := make([]CampaignViewRecord, 0, 3+len(state.VariantRoles))

	summaryBody, err := marshalCanonicalViewRecord(buildLiveEvaluationSummaryRecord(run, datasetID, observed, state))
	if err != nil {
		return nil, err
	}
	records = append(records, CampaignViewRecord{
		IdempotencyKey: EvaluationSummaryIdempotencyKey(runID, state.Scheduled, state.Terminal),
		Body:           summaryBody,
	})

	var catalogRecord map[string]any
	if settled {
		catalogRecord = buildCompletedCatalogSnapshotRecord(datasetID, runID, observed, state)
	} else {
		catalogRecord = buildCatalogSnapshotRecord(datasetID, runID, observed, state)
	}
	catalogBody, err := marshalCanonicalViewRecord(catalogRecord)
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
		var modelRecord map[string]any
		if settled {
			modelRecord = buildCompletedModelSummaryRecord(datasetID, observed, bucket)
		} else {
			modelRecord = buildModelSummaryRecord(datasetID, observed, bucket)
		}
		modelBody, err := marshalCanonicalViewRecord(modelRecord)
		if err != nil {
			return nil, err
		}
		records = append(records, CampaignViewRecord{
			IdempotencyKey: ModelSummaryIdempotencyKey(runID, bucket.VariantID, bucket.Role, bucket.Terminal),
			Body:           modelBody,
		})
	}

	var methodologyRecord map[string]any
	if settled {
		methodologyRecord = buildCompletedMethodologySnapshotRecord(datasetID, observed)
	} else {
		methodologyRecord = buildMethodologySnapshotRecord(datasetID, observed)
	}
	methodologyBody, err := marshalCanonicalViewRecord(methodologyRecord)
	if err != nil {
		return nil, err
	}
	records = append(records, CampaignViewRecord{
		IdempotencyKey: MethodologySnapshotIdempotencyKey(runID, state.Terminal),
		Body:           methodologyBody,
	})
	return records, nil
}

// BuildRunCompletionViewRecords materializes the terminal evaluation_summary
// plus completion catalog, model, and methodology snapshots for one finished
// campaign run.
func BuildRunCompletionViewRecords(run *evalv1.EvaluationRun, assignments []*evalv1.EvaluationAssignment, results map[string]*evalv1.EvaluationAssignmentResult, state *runAggregateState, observedAt time.Time) ([]CampaignViewRecord, error) {
	if run == nil || state == nil || !RunAggregateComplete(assignments, results, state) {
		return nil, fmt.Errorf("evaluation: build run completion view records: %w", constants.ErrMissingRequiredField)
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	observed := observedAt.UTC().Format(time.RFC3339Nano)
	runID := run.GetRunId()
	datasetID := CampaignDatasetID(runID)
	records := make([]CampaignViewRecord, 0, 2+len(state.VariantRoles))

	summaryBody, err := marshalCanonicalViewRecord(buildEvaluationSummaryRecord(run, datasetID, observed, state, "completed", true))
	if err != nil {
		return nil, err
	}
	records = append(records, CampaignViewRecord{
		IdempotencyKey: RunCompletionIdempotencyKey(runID) + ":evaluation_summary:v2",
		Body:           summaryBody,
	})

	catalogBody, err := marshalCanonicalViewRecord(buildCompletedCatalogSnapshotRecord(datasetID, runID, observed, state))
	if err != nil {
		return nil, err
	}
	records = append(records, CampaignViewRecord{
		IdempotencyKey: RunCompletionIdempotencyKey(runID) + ":catalog",
		Body:           catalogBody,
	})

	keys := make([]string, 0, len(state.VariantRoles))
	for key := range state.VariantRoles {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		bucket := state.VariantRoles[key]
		modelBody, err := marshalCanonicalViewRecord(buildCompletedModelSummaryRecord(datasetID, observed, bucket))
		if err != nil {
			return nil, err
		}
		records = append(records, CampaignViewRecord{
			IdempotencyKey: RunCompletionIdempotencyKey(runID) + ":model:" + bucket.VariantID + ":" + bucket.Role,
			Body:           modelBody,
		})
	}

	methodologyBody, err := marshalCanonicalViewRecord(buildCompletedMethodologySnapshotRecord(datasetID, observed))
	if err != nil {
		return nil, err
	}
	records = append(records, CampaignViewRecord{
		IdempotencyKey: RunCompletionIdempotencyKey(runID) + ":methodology",
		Body:           methodologyBody,
	})
	return records, nil
}

func buildCatalogSnapshotRecord(datasetID, runID, observedAt string, state *runAggregateState) map[string]any {
	return map[string]any{
		"schema_version":         explorerViewSchemaVersion,
		"kind":                   "catalog_snapshot",
		"dataset_id":             datasetID,
		"dataset_kind":           "live_run",
		"quality_state":          "live_in_progress",
		"observed_at":            observedAt,
		"source_revision_label":  campaignSourceRevision,
		"title":                  fmt.Sprintf("Live smoke run (%s)", runID),
		"description":            "Homogeneous full-pipeline model-role evaluation over the frozen standard scenario catalog. Values are provisional while assignments are still executing.",
		"limitations":            catalogSnapshotLimitations(),
		"model_count":            state.ModelCount,
		"evaluated_count":        state.EvaluatedCount,
		"suite_count":            1,
		"run_count":              1,
		"assignment_count":       state.Scheduled,
		"provider_request_count": state.Terminal,
		"provider_token_count":   0,
		"retry_count":            0,
		"verifier_passed_count":  0,
		"verifier_failed_count":  0,
		"generated_at":           observedAt,
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
		"schema_version":         explorerViewSchemaVersion,
		"kind":                   "model_summary",
		"dataset_id":             datasetID,
		"quality_state":          qualityStateForModelSummary(terminal),
		"observed_at":            observedAt,
		"source_revision_label":  campaignSourceRevision,
		"variant_id":             bucket.VariantID,
		"display_name":           displayNameForVariant(bucket.VariantID),
		"served_model_tag":       servedTagForVariant(bucket.VariantID),
		"role":                   bucket.Role,
		"backend_provider_class": "ollama",
		"inventory_only":         terminal == 0,
		"evaluation_coverage":    coverage,
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

func buildCompletedCatalogSnapshotRecord(datasetID, runID, observedAt string, state *runAggregateState) map[string]any {
	record := buildCatalogSnapshotRecord(datasetID, runID, observedAt, state)
	record["quality_state"] = qualityStateForCompletedAggregate(state)
	record["description"] = "Homogeneous full-pipeline model-role evaluation over the frozen standard scenario catalog. The campaign matrix is complete; values remain provisional until verification runs."
	record["limitations"] = []string{
		"Campaign execution is complete; values remain provisional until verification runs.",
		"Model aggregates reflect designated role responsibility inside the production chat pipeline, not a provider-only benchmark.",
		"Resource telemetry remains unavailable until provider-boundary observation is published.",
	}
	return record
}

func buildCompletedModelSummaryRecord(datasetID, observedAt string, bucket *variantRoleAggregate) map[string]any {
	record := buildModelSummaryRecord(datasetID, observedAt, bucket)
	if bucket.Scheduled > 0 && bucket.Terminal >= bucket.Scheduled {
		record["quality_state"] = "exploratory_partial"
	}
	return record
}

func buildCompletedMethodologySnapshotRecord(datasetID, observedAt string) map[string]any {
	record := buildMethodologySnapshotRecord(datasetID, observedAt)
	record["quality_state"] = "exploratory_partial"
	record["limitations"] = []string{
		"Campaign execution is complete; values remain provisional until verification runs.",
		"Homogeneous model-role and heterogeneous system leaderboards remain separate datasets.",
		"GPU and system efficiency metrics remain unavailable until provider-boundary observation is published.",
	}
	return record
}

// BuildRunVerificationViewRecords materializes the post-verify evaluation_summary
// revision for one campaign run.
func BuildRunVerificationViewRecords(run *evalv1.EvaluationRun, state *runAggregateState, report *evalv1.EvaluationVerificationReport, observedAt time.Time) ([]CampaignViewRecord, error) {
	if run == nil || state == nil || report == nil || report.GetRunId() != run.GetRunId() {
		return nil, fmt.Errorf("evaluation: build run verification view records: %w", constants.ErrMissingRequiredField)
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	observed := observedAt.UTC().Format(time.RFC3339Nano)
	datasetID := CampaignDatasetID(run.GetRunId())
	summaryBody, err := marshalCanonicalViewRecord(buildVerifiedEvaluationSummaryRecord(run, datasetID, observed, state, report))
	if err != nil {
		return nil, err
	}
	return []CampaignViewRecord{{
		IdempotencyKey: RunVerificationIdempotencyKey(run.GetRunId()),
		Body:           summaryBody,
	}}, nil
}

func buildLiveEvaluationSummaryRecord(run *evalv1.EvaluationRun, datasetID, observedAt string, state *runAggregateState) map[string]any {
	lifecycle := "running"
	includeEndedAt := false
	if runAggregateSettled(state) {
		lifecycle = "completed"
		includeEndedAt = true
	}
	return buildEvaluationSummaryRecord(run, datasetID, observedAt, state, lifecycle, includeEndedAt)
}

func buildEvaluationSummaryRecord(
	run *evalv1.EvaluationRun,
	datasetID, observedAt string,
	state *runAggregateState,
	lifecycleState string,
	includeEndedAt bool,
) map[string]any {
	var startedAtTime time.Time
	startedAt := ""
	if run.GetStartedAt() != nil {
		startedAtTime = run.GetStartedAt().AsTime().UTC()
		startedAt = startedAtTime.Format(time.RFC3339Nano)
	}
	observedTime, _ := time.Parse(time.RFC3339Nano, observedAt)
	if observedTime.IsZero() {
		if parsed, err := time.Parse(time.RFC3339, observedAt); err == nil {
			observedTime = parsed.UTC()
		}
	}
	passRate := 0.0
	if state.Terminal > 0 {
		passRate = float64(state.Passed) / float64(state.Terminal)
	}
	record := map[string]any{
		"schema_version":        explorerViewSchemaVersion,
		"kind":                  "evaluation_summary",
		"dataset_id":            datasetID,
		"quality_state":         qualityStateForCompletedAggregate(state),
		"observed_at":           observedAt,
		"source_revision_label": campaignSourceRevision,
		"run_id":                run.GetRunId(),
		"campaign_id":           run.GetCampaignBinding().GetCampaignId(),
		"suite_id":              standardSuiteID,
		"arm":                   armForRun(run),
		"evaluation_unit":       evaluationUnitForRun(run),
		"model_role_mapping":    buildModelRoleMapping(state),
		"lifecycle_state":       lifecycleState,
		"assignment_total":      state.Scheduled,
		"assignment_completed":  state.Passed,
		"assignment_failed":     state.Failed,
		"terminal_outcomes":     aggregateTerminalOutcomes(state),
		"verifier_state":        "not_applicable",
		"headline_metrics":      map[string]any{},
	}
	if startedAt != "" {
		record["started_at"] = startedAt
		if includeEndedAt {
			record["ended_at"] = observedAt
		}
		if !startedAtTime.IsZero() && !observedTime.IsZero() {
			if elapsed := elapsedSecondsBetween(startedAtTime, observedTime); elapsed != nil {
				record["elapsed_seconds"] = *elapsed
			}
		}
	}
	if state.Terminal > 0 {
		record["headline_metrics"] = map[string]any{
			"pass_rate": map[string]any{"value": passRate},
		}
	}
	return record
}

func elapsedSecondsBetween(startedAt, observedAt time.Time) *float64 {
	if startedAt.IsZero() || observedAt.IsZero() || observedAt.Before(startedAt) {
		return nil
	}
	seconds := math.Round(observedAt.Sub(startedAt).Seconds())
	return &seconds
}

func buildVerifiedEvaluationSummaryRecord(run *evalv1.EvaluationRun, datasetID, observedAt string, state *runAggregateState, report *evalv1.EvaluationVerificationReport) map[string]any {
	record := buildEvaluationSummaryRecord(run, datasetID, observedAt, state, "completed", true)
	record["quality_state"] = qualityStateForVerifiedAggregate(state, report)
	record["verifier_state"] = VerifierStateFromVerificationReport(report)
	if summary := formatCampaignVerifierFailureSummary(report); summary != "" {
		record["verifier_failure_summary"] = summary
	}
	if report.GetVerifiedAt() != nil {
		record["observed_at"] = report.GetVerifiedAt().AsTime().UTC().Format(time.RFC3339Nano)
	}
	return record
}

// buildModelRoleMapping declares one variant per role when the run matrix
// uses a single variant in that role; otherwise it stays empty because
// homogeneous smoke matrices evaluate many variants per role independently.
func buildModelRoleMapping(state *runAggregateState) map[string]string {
	mapping := map[string]string{}
	if state == nil {
		return mapping
	}
	roleVariants := map[string]map[string]struct{}{}
	for _, bucket := range state.VariantRoles {
		if bucket == nil || bucket.Role == "" || bucket.VariantID == "" {
			continue
		}
		if roleVariants[bucket.Role] == nil {
			roleVariants[bucket.Role] = map[string]struct{}{}
		}
		roleVariants[bucket.Role][bucket.VariantID] = struct{}{}
	}
	for role, variants := range roleVariants {
		if len(variants) != 1 {
			continue
		}
		for variantID := range variants {
			mapping[role] = variantID
		}
	}
	return mapping
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
				"suite_id":     standardSuiteID,
				"display_name": "Standard 25",
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

func assignmentLifecycleIsTerminal(status evalv1.EvaluationAssignmentLifecycleStatus) bool {
	switch status {
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_UNSPECIFIED,
		evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED,
		evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING:
		return false
	default:
		return true
	}
}

func lifecycleTerminalOutcome(status evalv1.EvaluationAssignmentLifecycleStatus) string {
	switch status {
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED:
		return "completed"
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL,
		evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_GRADER_FAILED:
		return "grader_failed"
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_POLICY_REJECTED:
		return "invalid_evidence"
	case evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_STOPPED:
		return "stopped"
	default:
		return "model_failed"
	}
}

func qualityStateForCompletedAggregate(state *runAggregateState) string {
	if state != nil && state.Scheduled > 0 && state.Terminal >= state.Scheduled {
		return "exploratory_partial"
	}
	if state.Terminal > 0 {
		return "live_in_progress"
	}
	return "not_evaluated"
}

func formatCampaignVerifierFailureSummary(report *evalv1.EvaluationVerificationReport) string {
	if report == nil {
		return ""
	}
	reasons := report.GetFailureReasons()
	if len(reasons) == 0 {
		return ""
	}
	if len(reasons) == 1 {
		return reasons[0]
	}
	const maxDetail = 3
	const maxLen = 500
	summary := fmt.Sprintf("%d verification failure(s)", len(reasons))
	detailCount := len(reasons)
	if detailCount > maxDetail {
		detailCount = maxDetail
	}
	detail := strings.Join(reasons[:detailCount], "; ")
	if len(reasons) > maxDetail {
		detail += fmt.Sprintf("; ... and %d more", len(reasons)-maxDetail)
	}
	combined := summary + ": " + detail
	if len(combined) > maxLen {
		return combined[:maxLen-3] + "..."
	}
	return combined
}

func qualityStateForVerifiedAggregate(state *runAggregateState, report *evalv1.EvaluationVerificationReport) string {
	if report != nil && report.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
		return "exploratory_verified"
	}
	return qualityStateForCompletedAggregate(state)
}

func aggregateTerminalOutcomes(state *runAggregateState) map[string]uint32 {
	record := terminalOutcomesRecord(nil)
	for _, bucket := range state.VariantRoles {
		for key, count := range bucket.Outcomes {
			record[key] += count
		}
	}
	return record
}

func armForRun(run *evalv1.EvaluationRun) string {
	if run.GetLane() == evalv1.EvaluationLane_EVALUATION_LANE_SYSTEM {
		return "heterogeneous-system"
	}
	return "homogeneous-model-role"
}

func evaluationUnitForRun(run *evalv1.EvaluationRun) string {
	if run.GetLane() == evalv1.EvaluationLane_EVALUATION_LANE_SYSTEM {
		return "system"
	}
	return "model"
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
