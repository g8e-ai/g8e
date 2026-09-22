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
	explorerViewSchemaVersion = "1.5.0"
	campaignSourceRevision    = "g8e-eval-campaign"
	standardSuiteID           = "north-star-25"
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
	Headline       *runHeadlineMetrics
}

// runHeadlineMetrics carries the typed run-level metric aggregate for the
// explorer evaluation_summary headline cards.
type runHeadlineMetrics struct {
	PassRate            runMetricValue
	LatencyP50MS        runMetricValue
	OutputThroughputP50 runMetricValue
}

// runMetricValue mirrors the explorer RunMetricValue wire shape: one finite
// value or one typed unavailable reason plus contributor coverage counts.
type runMetricValue struct {
	Value             *float64
	Unit              string
	Observed          uint32
	Eligible          uint32
	Unavailable       uint32
	UnavailableReason evalv1.PublicUnavailableReason
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

// BoundVerificationSummaryIdempotencyKey returns the report-scoped key for a
// newly bound verification summary revision.
func BoundVerificationSummaryIdempotencyKey(runID, reportDigest string) string {
	return fmt.Sprintf("%s:verification:summary:%s", runID, reportDigest)
}

// VerifiedModelSummaryIdempotencyKey returns the deterministic publication key
// for one report-scoped variant-role revision. It is distinct from both
// aggregate revisions and the historical run-level verification summary so a
// later bound report can supersede an earlier report without suppressing it.
func VerifiedModelSummaryIdempotencyKey(runID, variantID, role, reportDigest string) string {
	return fmt.Sprintf("%s:verification:model:%s:%s:%s", runID, variantID, role, reportDigest)
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
	headline, err := collectRunHeadlineMetrics(assignments, results, state)
	if err != nil {
		return nil, err
	}
	state.Headline = headline
	return state, nil
}

const (
	runMetricUnitRatio           = "ratio"
	runMetricUnitMilliseconds    = "milliseconds"
	runMetricUnitTokensPerSecond = "tokens_per_second"
)

// collectRunHeadlineMetrics derives typed pass-rate, latency p50, and output
// throughput p50 aggregates from canonical persisted assignment results. Every
// persisted terminal result containing scored model inference activity is an
// eligible contributor; a contributor with missing or incomplete evidence
// increments the unavailable count rather than being silently dropped.
func collectRunHeadlineMetrics(assignments []*evalv1.EvaluationAssignment, results map[string]*evalv1.EvaluationAssignmentResult, state *runAggregateState) (*runHeadlineMetrics, error) {
	metrics := &runHeadlineMetrics{
		PassRate:            runMetricValue{Unit: runMetricUnitRatio, Eligible: state.Terminal, Observed: state.Terminal},
		LatencyP50MS:        runMetricValue{Unit: runMetricUnitMilliseconds},
		OutputThroughputP50: runMetricValue{Unit: runMetricUnitTokensPerSecond},
	}
	if state.Terminal > 0 {
		value := float64(state.Passed) / float64(state.Terminal)
		metrics.PassRate.Value = &value
	} else {
		metrics.PassRate.UnavailableReason = evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_UNAVAILABLE
	}
	latencies := make([]float64, 0, len(results))
	throughputs := make([]float64, 0, len(results))
	for _, assignment := range assignments {
		if assignment == nil || !assignmentLifecycleIsTerminal(assignment.GetLifecycleStatus()) {
			continue
		}
		result := results[assignment.GetAssignmentId()]
		if result == nil || !assignmentLifecycleIsTerminal(result.GetLifecycleStatus()) {
			continue
		}
		calls := result.GetModelInferences()
		if len(calls) == 0 {
			continue
		}
		metrics.LatencyP50MS.Eligible++
		metrics.OutputThroughputP50.Eligible++
		if result.ScoredInferenceSpanNanos == nil {
			metrics.LatencyP50MS.Unavailable++
		} else {
			span := result.GetScoredInferenceSpanNanos()
			if span/1_000_000 > maxPublicDurationMS {
				return nil, fmt.Errorf("evaluation: collect run headline metrics: scored inference span exceeds public bound: %w", constants.ErrEvidenceArtifactMalformed)
			}
			latencies = append(latencies, float64(span/1_000_000))
		}
		rate, complete, err := scoredAssignmentOutputThroughput(calls)
		if err != nil {
			return nil, err
		}
		if !complete {
			metrics.OutputThroughputP50.Unavailable++
		} else {
			throughputs = append(throughputs, rate)
		}
	}
	metrics.LatencyP50MS.Observed = uint32(len(latencies))
	metrics.OutputThroughputP50.Observed = uint32(len(throughputs))
	if len(latencies) > 0 {
		value := medianFloat64(latencies)
		metrics.LatencyP50MS.Value = &value
	} else {
		metrics.LatencyP50MS.UnavailableReason = contributorUnavailableReason(metrics.LatencyP50MS.Eligible, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_UNAVAILABLE)
	}
	if len(throughputs) > 0 {
		value := medianFloat64(throughputs)
		metrics.OutputThroughputP50.Value = &value
	} else {
		metrics.OutputThroughputP50.UnavailableReason = contributorUnavailableReason(metrics.OutputThroughputP50.Eligible, evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_INCOMPLETE_CONTRIBUTOR_EVIDENCE)
	}
	return metrics, nil
}

// contributorUnavailableReason selects the run-metric unavailable reason when
// no eligible contributor produced a value: no scored inference activity at
// all maps to no_scored_calls, otherwise the contributor-scoped reason applies.
func contributorUnavailableReason(eligible uint32, reason evalv1.PublicUnavailableReason) evalv1.PublicUnavailableReason {
	if eligible == 0 {
		return evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_NO_SCORED_CALLS
	}
	return reason
}

// scoredAssignmentOutputThroughput returns generated output tokens per second
// for one assignment's scored inference calls. Every contributing call must
// report usage and a positive generation duration; otherwise the assignment is
// an incomplete contributor.
func scoredAssignmentOutputThroughput(calls []*evalv1.ModelInferenceRecord) (float64, bool, error) {
	var tokens, duration uint64
	for _, call := range calls {
		if call == nil {
			return 0, false, fmt.Errorf("evaluation: collect run headline metrics: nil inference record: %w", constants.ErrEvidenceArtifactMalformed)
		}
		if call.GetUsageAvailability() != evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_REPORTED || call.GetGenerationDurationNanos() == 0 {
			return 0, false, nil
		}
		var err error
		if tokens, err = addResourceValue(tokens, uint64(call.GetCompletionTokens()), "output tokens"); err != nil {
			return 0, false, err
		}
		if duration, err = addResourceValue(duration, call.GetGenerationDurationNanos(), "generation duration"); err != nil {
			return 0, false, err
		}
	}
	return float64(tokens) / (float64(duration) / 1_000_000_000), true, nil
}

// medianFloat64 returns the deterministic median of finite values: the center
// value for an odd count or the arithmetic mean of the two center values for
// an even count.
func medianFloat64(values []float64) float64 {
	sort.Float64s(values)
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return (values[middle-1] + values[middle]) / 2
}

// runAggregateSettled reports whether every scheduled assignment has reached a
// terminal public outcome in the aggregate counters.
func runAggregateSettled(state *runAggregateState) bool {
	return state != nil && state.Scheduled > 0 && state.Terminal >= state.Scheduled
}

// BuildRunAggregateViewRecords materializes live evaluation_summary, catalog,
// model, and methodology explorer snapshot records for one campaign run. When
// report is a bound, applicable verification report for the run, the emitted
// summary carries the verified state; otherwise it reports not_run.
func BuildRunAggregateViewRecords(run *evalv1.EvaluationRun, state *runAggregateState, report *evalv1.EvaluationVerificationReport, observedAt time.Time) ([]CampaignViewRecord, error) {
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

	summaryRecord, err := buildEvaluationSummaryRecord(evaluationSummaryInput{
		Run: run, DatasetID: datasetID, ObservedAt: observed, State: state, Report: report,
	})
	if err != nil {
		return nil, err
	}
	summaryBody, err := marshalCanonicalViewRecord(summaryRecord)
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
// campaign run. When report is a bound, applicable verification report for the
// run, the emitted summary carries the verified state; otherwise not_run.
func BuildRunCompletionViewRecords(run *evalv1.EvaluationRun, assignments []*evalv1.EvaluationAssignment, results map[string]*evalv1.EvaluationAssignmentResult, state *runAggregateState, report *evalv1.EvaluationVerificationReport, observedAt time.Time) ([]CampaignViewRecord, error) {
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

	summaryRecord, err := buildEvaluationSummaryRecord(evaluationSummaryInput{
		Run: run, DatasetID: datasetID, ObservedAt: observed, State: state, Report: report,
	})
	if err != nil {
		return nil, err
	}
	summaryBody, err := marshalCanonicalViewRecord(summaryRecord)
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
// revision and eligible verified model-summary revisions for one campaign run.
func BuildRunVerificationViewRecords(run *evalv1.EvaluationRun, state *runAggregateState, report *evalv1.EvaluationVerificationReport, observedAt time.Time) ([]CampaignViewRecord, error) {
	if run == nil || state == nil || report == nil || report.GetRunId() != run.GetRunId() {
		return nil, fmt.Errorf("evaluation: build run verification view records: %w", constants.ErrMissingRequiredField)
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	observed := observedAt.UTC().Format(time.RFC3339Nano)
	datasetID := CampaignDatasetID(run.GetRunId())
	summaryRecord, err := buildEvaluationSummaryRecord(evaluationSummaryInput{
		Run: run, DatasetID: datasetID, ObservedAt: observed, State: state, Report: report,
	})
	if err != nil {
		return nil, err
	}
	summaryBody, err := marshalCanonicalViewRecord(summaryRecord)
	if err != nil {
		return nil, err
	}
	summaryKey := RunVerificationIdempotencyKey(run.GetRunId())
	if verifiedModelSummariesEligible(state, report, run) {
		summaryKey = BoundVerificationSummaryIdempotencyKey(run.GetRunId(), report.GetReportDigestRef().GetSha256())
	}
	records := []CampaignViewRecord{{
		IdempotencyKey: summaryKey,
		Body:           summaryBody,
	}}
	if !verifiedModelSummariesEligible(state, report, run) {
		return records, nil
	}
	return appendVerifiedModelSummaryRecords(records, run.GetRunId(), datasetID, observed, state, report, nil)
}

func appendVerifiedModelSummaryRecords(records []CampaignViewRecord, runID, datasetID, observed string, state *runAggregateState, report *evalv1.EvaluationVerificationReport, eligibleBuckets []VerifiedModelSummaryBucket) ([]CampaignViewRecord, error) {
	keys := make([]string, 0, len(state.VariantRoles))
	allowed := make(map[string]struct{}, len(eligibleBuckets))
	for _, bucket := range eligibleBuckets {
		role := strings.ToLower(strings.TrimPrefix(bucket.Role.String(), "MODEL_CAMPAIGN_ROLE_"))
		allowed[bucket.VariantID+":"+role] = struct{}{}
	}
	for key := range state.VariantRoles {
		if len(eligibleBuckets) > 0 {
			if _, ok := allowed[key]; !ok {
				continue
			}
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	reportDigest := report.GetReportDigestRef().GetSha256()
	for _, key := range keys {
		bucket := state.VariantRoles[key]
		if bucket == nil || bucket.Scheduled == 0 || bucket.Terminal < bucket.Scheduled {
			continue
		}
		modelRecord := buildCompletedModelSummaryRecord(datasetID, observed, bucket)
		if report.GetStatus() == evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS {
			modelRecord["quality_state"] = "exploratory_verified"
		}
		modelBody, err := marshalCanonicalViewRecord(modelRecord)
		if err != nil {
			return nil, err
		}
		records = append(records, CampaignViewRecord{
			IdempotencyKey: VerifiedModelSummaryIdempotencyKey(runID, bucket.VariantID, bucket.Role, reportDigest),
			Body:           modelBody,
		})
	}
	return records, nil
}

// BuildRunVerificationApplicability binds a report to the exact completed
// assignment population used by the aggregate projector.
func BuildRunVerificationApplicability(run *evalv1.EvaluationRun, spec *evalv1.EvaluationCampaignSpec, catalog *evalv1.EvaluationScenarioCatalog, assignments []*evalv1.EvaluationAssignment, results map[string]*evalv1.EvaluationAssignmentResult, report *evalv1.EvaluationVerificationReport) (*RunVerificationApplicability, error) {
	if run == nil || spec == nil || catalog == nil || report == nil || report.GetRunId() != run.GetRunId() {
		return nil, fmt.Errorf("evaluation: build run verification applicability: %w", constants.ErrMissingRequiredField)
	}
	applicability := &RunVerificationApplicability{RunID: run.GetRunId(), CampaignID: spec.GetCampaignId(), ExpectedAssignmentCount: uint32(len(assignments))}
	population := &evalv1.EvaluationVerifiedPopulation{SchemaVersion: "1.0.0", RunId: run.GetRunId(), CampaignId: spec.GetCampaignId(), CampaignDigest: spec.GetCampaignDigest(), CatalogRef: spec.GetCatalogRef(), CatalogDigest: spec.GetCatalogDigest(), ModelRegistryDigest: spec.GetModelRegistryDigest(), Lane: run.GetLane(), ExpectedAssignmentCount: uint32(len(assignments))}
	buckets := make(map[string]*VerifiedModelSummaryBucket)
	for _, assignment := range assignments {
		if assignment == nil || !isTerminalAssignmentLifecycle(assignment.GetLifecycleStatus()) {
			continue
		}
		result := results[assignment.GetAssignmentId()]
		if result == nil || result.GetResultDigest() == "" || assignment.GetDeterministicIdentity() == "" {
			continue
		}
		entry := &evalv1.EvaluationVerifiedPopulationEntry{AssignmentId: assignment.GetAssignmentId(), DeterministicIdentity: assignment.GetDeterministicIdentity(), ScenarioId: assignment.GetScenarioId(), Repetition: assignment.GetRepetition(), LifecycleStatus: assignment.GetLifecycleStatus(), ResultDigest: result.GetResultDigest()}
		if ref := assignment.GetScenarioRef(); ref != nil {
			entry.ScenarioVersion = ref.GetVersion()
		}
		if homogeneous, ok := assignment.GetTarget().(*evalv1.EvaluationAssignment_Homogeneous); ok && homogeneous.Homogeneous != nil && homogeneous.Homogeneous.GetCandidateVariant() != nil {
			entry.VariantId = homogeneous.Homogeneous.GetCandidateVariant().GetVariantId()
			entry.DesignatedRole = homogeneous.Homogeneous.GetDesignatedRole()
			key := entry.GetVariantId() + ":" + strings.ToLower(strings.TrimPrefix(entry.GetDesignatedRole().String(), "MODEL_CAMPAIGN_ROLE_"))
			bucket := buckets[key]
			if bucket == nil {
				bucket = &VerifiedModelSummaryBucket{VariantID: entry.GetVariantId(), Role: entry.GetDesignatedRole()}
				buckets[key] = bucket
			}
			bucket.AssignmentIDs = append(bucket.AssignmentIDs, entry.GetAssignmentId())
		}
		population.Entries = append(population.Entries, entry)
	}
	sort.Slice(population.Entries, func(i, j int) bool {
		return population.Entries[i].GetDeterministicIdentity() < population.Entries[j].GetDeterministicIdentity()
	})
	populationDigest, err := digestProto(population)
	if err != nil {
		return nil, err
	}
	applicability.VerifiedAssignmentCount = uint32(len(population.Entries))
	applicability.Population = population
	applicability.EligibleModelBuckets = make([]VerifiedModelSummaryBucket, 0, len(buckets))
	for _, bucket := range buckets {
		applicability.EligibleModelBuckets = append(applicability.EligibleModelBuckets, *bucket)
	}
	sort.Slice(applicability.EligibleModelBuckets, func(i, j int) bool {
		left := applicability.EligibleModelBuckets[i]
		right := applicability.EligibleModelBuckets[j]
		return left.VariantID+left.Role.String() < right.VariantID+right.Role.String()
	})
	applicability.Applicable = report.GetStatus() != evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSPECIFIED && report.GetVerifiedPopulationDigest() == populationDigest && report.GetExpectedAssignmentCount() == applicability.ExpectedAssignmentCount && report.GetVerifiedAssignmentCount() == applicability.VerifiedAssignmentCount && report.GetCampaignDigest() == population.GetCampaignDigest() && report.GetCatalogDigest() == population.GetCatalogDigest() && report.GetModelRegistryDigest() == population.GetModelRegistryDigest()
	if !applicability.Applicable {
		applicability.UnavailableReason = evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_UNAVAILABLE
	}
	return applicability, nil
}

// BuildVerifiedModelSummaryViewRecords projects report-scoped model revisions
// from the shared verified population inputs used by publication and export.
func BuildVerifiedModelSummaryViewRecords(input VerifiedModelSummaryProjectionInput) ([]CampaignViewRecord, error) {
	if input.Run == nil || input.Report == nil || input.Applicability == nil || !input.Applicability.Applicable {
		return nil, fmt.Errorf("evaluation: build verified model summary records: %w", constants.ErrEvidenceScopeMismatch)
	}
	if !verifiedModelSummariesEligibleFromApplicability(input.Run, input.Report, input.Applicability) {
		return nil, fmt.Errorf("evaluation: build verified model summary records: %w", constants.ErrStaleEvidence)
	}
	state, err := CollectRunAggregateState(input.Assignments, input.Results)
	if err != nil {
		return nil, err
	}
	return appendVerifiedModelSummaryRecords(nil, input.Run.GetRunId(), CampaignDatasetID(input.Run.GetRunId()), input.Report.GetVerifiedAt().AsTime().UTC().Format(time.RFC3339Nano), state, input.Report, input.Applicability.EligibleModelBuckets)
}

func verifiedModelSummariesEligibleFromApplicability(run *evalv1.EvaluationRun, report *evalv1.EvaluationVerificationReport, applicability *RunVerificationApplicability) bool {
	return applicability != nil && applicability.RunID == run.GetRunId() && applicability.ExpectedAssignmentCount == report.GetExpectedAssignmentCount() && applicability.VerifiedAssignmentCount == report.GetVerifiedAssignmentCount() && verifiedModelSummariesEligible(&runAggregateState{Scheduled: applicability.ExpectedAssignmentCount, Terminal: applicability.VerifiedAssignmentCount}, report, run)
}

func verifiedModelSummariesEligible(state *runAggregateState, report *evalv1.EvaluationVerificationReport, run *evalv1.EvaluationRun) bool {
	if report == nil || run == nil || report.GetRunId() != run.GetRunId() {
		return false
	}
	binding := run.GetCampaignBinding()
	return binding != nil &&
		report.GetSchemaVersion() == "2.0.0" &&
		report.GetVerifierContractVersion() == "2.0.0" &&
		report.GetVerifierReleaseVersion() != "" &&
		report.GetReportDigestRef() != nil &&
		publicSHA256Pattern.MatchString(report.GetReportDigestRef().GetSha256()) &&
		publicSHA256Pattern.MatchString(report.GetVerifiedPopulationDigest()) &&
		report.GetVerifiedAt() != nil &&
		report.GetCampaignDigest() == binding.GetCampaignDigest() &&
		report.GetCatalogDigest() == binding.GetCatalogDigest() &&
		report.GetModelRegistryDigest() == binding.GetModelRegistryDigest() &&
		state != nil &&
		state.Scheduled > 0 &&
		state.Terminal >= state.Scheduled &&
		report.GetExpectedAssignmentCount() == state.Scheduled &&
		report.GetVerifiedAssignmentCount() == state.Terminal
}

// runMetricValueRecord is the explorer RunMetricValue wire shape.
type runMetricValueRecord struct {
	Value             *float64 `json:"value,omitempty"`
	Unit              string   `json:"unit"`
	ObservedCount     uint32   `json:"observed_count"`
	EligibleCount     uint32   `json:"eligible_count"`
	UnavailableCount  uint32   `json:"unavailable_count"`
	UnavailableReason string   `json:"unavailable_reason,omitempty"`
}

func (m runMetricValue) record() runMetricValueRecord {
	record := runMetricValueRecord{
		Value:            m.Value,
		Unit:             m.Unit,
		ObservedCount:    m.Observed,
		EligibleCount:    m.Eligible,
		UnavailableCount: m.Unavailable,
	}
	if m.UnavailableReason != evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_UNSPECIFIED {
		record.UnavailableReason = publicUnavailableReasonString(m.UnavailableReason)
	}
	return record
}

// evaluationHeadlineMetricsRecord is the explorer EvaluationHeadlineMetrics
// wire shape for schema 1.5.0.
type evaluationHeadlineMetricsRecord struct {
	PassRate            runMetricValueRecord `json:"pass_rate"`
	LatencyP50MS        runMetricValueRecord `json:"latency_p50_ms"`
	OutputThroughputP50 runMetricValueRecord `json:"output_throughput_p50_tokens_per_second"`
}

// summaryVerificationMetadataRecord is the public-safe verification binding
// carried by an evaluation_summary that reports a bound verifier result.
type summaryVerificationMetadataRecord struct {
	Provenance              string `json:"provenance"`
	VerifierState           string `json:"verifier_state"`
	VerifierReleaseVersion  string `json:"verifier_release_version,omitempty"`
	VerifierContractVersion string `json:"verifier_contract_version,omitempty"`
	ReportDigest            string `json:"report_digest,omitempty"`
	PopulationDigest        string `json:"population_digest,omitempty"`
	VerifiedAt              string `json:"verified_at"`
}

// evaluationSummaryRecord is the typed explorer evaluation_summary wire shape
// for schema 1.5.0.
type evaluationSummaryRecord struct {
	SchemaVersion          string                           `json:"schema_version"`
	Kind                   string                           `json:"kind"`
	DatasetID              string                           `json:"dataset_id"`
	QualityState           string                           `json:"quality_state"`
	ObservedAt             string                           `json:"observed_at"`
	SourceRevisionLabel    string                           `json:"source_revision_label"`
	RunID                  string                           `json:"run_id"`
	CampaignID             string                           `json:"campaign_id"`
	SuiteID                string                           `json:"suite_id"`
	Arm                    string                           `json:"arm"`
	EvaluationUnit         string                           `json:"evaluation_unit"`
	ModelRoleMapping       map[string]string                `json:"model_role_mapping"`
	LifecycleState         string                           `json:"lifecycle_state"`
	AssignmentTotal        uint32                           `json:"assignment_total"`
	AssignmentCompleted    uint32                           `json:"assignment_completed"`
	AssignmentFailed       uint32                           `json:"assignment_failed"`
	TerminalOutcomes       map[string]uint32                `json:"terminal_outcomes"`
	StartedAt              string                           `json:"started_at,omitempty"`
	EndedAt                string                           `json:"ended_at,omitempty"`
	ElapsedSeconds         *float64                         `json:"elapsed_seconds,omitempty"`
	VerifierState          string                           `json:"verifier_state"`
	VerifierFailureSummary string                           `json:"verifier_failure_summary,omitempty"`
	VerificationMetadata   *summaryVerificationMetadataRecord `json:"verification_metadata,omitempty"`
	HeadlineMetrics        evaluationHeadlineMetricsRecord  `json:"headline_metrics"`
}

// evaluationSummaryInput carries every input the summary builder needs; the
// builder performs no evidence reads or hidden recomputation. Report is the
// bound, applicable verification report for the run, or nil.
type evaluationSummaryInput struct {
	Run        *evalv1.EvaluationRun
	DatasetID  string
	ObservedAt string
	State      *runAggregateState
	Report     *evalv1.EvaluationVerificationReport
}

// buildEvaluationSummaryRecord is the single construction point for live,
// completion, and verification evaluation_summary revisions. A bound
// applicable report produces passed or failed state with public verification
// metadata; without one the summary reports not_run.
func buildEvaluationSummaryRecord(input evaluationSummaryInput) (*evaluationSummaryRecord, error) {
	run, state := input.Run, input.State
	if run == nil || state == nil {
		return nil, fmt.Errorf("evaluation: build evaluation summary record: %w", constants.ErrMissingRequiredField)
	}
	settled := runAggregateSettled(state)
	record := &evaluationSummaryRecord{
		SchemaVersion:       explorerViewSchemaVersion,
		Kind:                "evaluation_summary",
		DatasetID:           input.DatasetID,
		QualityState:        qualityStateForCompletedAggregate(state),
		ObservedAt:          input.ObservedAt,
		SourceRevisionLabel: campaignSourceRevision,
		RunID:               run.GetRunId(),
		CampaignID:          run.GetCampaignBinding().GetCampaignId(),
		SuiteID:             standardSuiteID,
		Arm:                 armForRun(run),
		EvaluationUnit:      evaluationUnitForRun(run),
		ModelRoleMapping:    buildModelRoleMapping(state),
		LifecycleState:      "running",
		AssignmentTotal:     state.Scheduled,
		AssignmentCompleted: state.Passed,
		AssignmentFailed:    state.Failed,
		TerminalOutcomes:    aggregateTerminalOutcomes(state),
		VerifierState:       "not_run",
		HeadlineMetrics:     headlineMetricsForState(state),
	}
	if settled {
		record.LifecycleState = "completed"
		record.EndedAt = input.ObservedAt
	}
	if run.GetStartedAt() != nil {
		startedAtTime := run.GetStartedAt().AsTime().UTC()
		record.StartedAt = startedAtTime.Format(time.RFC3339Nano)
		observedTime, _ := time.Parse(time.RFC3339Nano, input.ObservedAt)
		if observedTime.IsZero() {
			if parsed, err := time.Parse(time.RFC3339, input.ObservedAt); err == nil {
				observedTime = parsed.UTC()
			}
		}
		if elapsed := elapsedSecondsBetween(startedAtTime, observedTime); elapsed != nil {
			record.ElapsedSeconds = elapsed
		}
	}
	if input.Report == nil {
		return record, nil
	}
	return applyBoundVerification(record, state, input.Report)
}

// applyBoundVerification projects a bound, applicable verification report onto
// the summary. Malformed bound reports fail closed rather than emitting a
// downgrade.
func applyBoundVerification(record *evaluationSummaryRecord, state *runAggregateState, report *evalv1.EvaluationVerificationReport) (*evaluationSummaryRecord, error) {
	if !boundVerificationMetadataComplete(report) {
		return nil, fmt.Errorf("evaluation: build evaluation summary record: bound verification report is incomplete: %w", constants.ErrEvidenceArtifactMalformed)
	}
	record.VerifierState = VerifierStateFromVerificationReport(report)
	record.VerificationMetadata = &summaryVerificationMetadataRecord{
		Provenance:              "bound",
		VerifierState:           record.VerifierState,
		VerifierReleaseVersion:  report.GetVerifierReleaseVersion(),
		VerifierContractVersion: report.GetVerifierContractVersion(),
		ReportDigest:            report.GetReportDigestRef().GetSha256(),
		PopulationDigest:        report.GetVerifiedPopulationDigest(),
		VerifiedAt:              report.GetVerifiedAt().AsTime().UTC().Format(time.RFC3339Nano),
	}
	if summary := formatCampaignVerifierFailureSummary(report); summary != "" {
		record.VerifierFailureSummary = summary
	}
	record.QualityState = qualityStateForVerifiedAggregate(state, report)
	record.ObservedAt = report.GetVerifiedAt().AsTime().UTC().Format(time.RFC3339Nano)
	return record, nil
}

// boundVerificationMetadataComplete reports whether a persisted report carries
// every public-safe field required for a bound verification summary revision.
func boundVerificationMetadataComplete(report *evalv1.EvaluationVerificationReport) bool {
	return report != nil &&
		report.GetStatus() != evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSPECIFIED &&
		report.GetSchemaVersion() == campaignVerificationSchemaVersion &&
		report.GetVerifierContractVersion() == constants.CampaignVerifierVersion &&
		report.GetVerifierReleaseVersion() != "" &&
		report.GetReportDigestRef() != nil &&
		publicSHA256Pattern.MatchString(report.GetReportDigestRef().GetSha256()) &&
		publicSHA256Pattern.MatchString(report.GetVerifiedPopulationDigest()) &&
		report.GetVerifiedAt() != nil
}

// headlineMetricsForState renders the collected run metrics; when the state
// was not produced by CollectRunAggregateState the metric cards report
// unavailable with zero eligible contributors rather than fabricating values.
func headlineMetricsForState(state *runAggregateState) evaluationHeadlineMetricsRecord {
	headline := state.Headline
	if headline == nil {
		headline = &runHeadlineMetrics{
			PassRate:            runMetricValue{Unit: runMetricUnitRatio, Eligible: state.Terminal, Observed: state.Terminal},
			LatencyP50MS:        runMetricValue{Unit: runMetricUnitMilliseconds, UnavailableReason: evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_NO_SCORED_CALLS},
			OutputThroughputP50: runMetricValue{Unit: runMetricUnitTokensPerSecond, UnavailableReason: evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_NO_SCORED_CALLS},
		}
		if state.Terminal > 0 {
			value := float64(state.Passed) / float64(state.Terminal)
			headline.PassRate.Value = &value
		} else {
			headline.PassRate.Observed = 0
			headline.PassRate.UnavailableReason = evalv1.PublicUnavailableReason_PUBLIC_UNAVAILABLE_REASON_SOURCE_UNAVAILABLE
		}
	}
	return evaluationHeadlineMetricsRecord{
		PassRate:            headline.PassRate.record(),
		LatencyP50MS:        headline.LatencyP50MS.record(),
		OutputThroughputP50: headline.OutputThroughputP50.record(),
	}
}

func elapsedSecondsBetween(startedAt, observedAt time.Time) *float64 {
	if startedAt.IsZero() || observedAt.IsZero() || observedAt.Before(startedAt) {
		return nil
	}
	seconds := math.Round(observedAt.Sub(startedAt).Seconds())
	return &seconds
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

func marshalCanonicalViewRecord(record any) ([]byte, error) {
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
