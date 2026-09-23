// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// ProviderObservationRemote loads provider-boundary observation evidence from a
// remote gateway when local runtime files are unavailable.
type ProviderObservationRemote interface {
	Load(ctx context.Context, providerAttemptID string) (*evalv1.ProviderBoundaryObservationWindow, *operatorv1.InferenceProviderAttemptRecord, error)
}

const (
	providerObservationArtifactType = "provider-boundary-observation-window"
	providerObservationSchemaRef    = "g8e.eval.v1.ProviderBoundaryObservationWindow"
)

// ProviderObservationPolicy controls whether missing provider-boundary windows
// are verification failures or interim unavailable telemetry.
type ProviderObservationPolicy int

const (
	// ProviderObservationPolicyInterim records missing windows as unavailable
	// reasons without failing assignment verification.
	ProviderObservationPolicyInterim ProviderObservationPolicy = 0
	// ProviderObservationPolicyStrict requires complete provider-boundary
	// observation coverage for every scored inference.
	ProviderObservationPolicyStrict ProviderObservationPolicy = 1
)

// PublicMetricValue is one disclosure-safe scalar metric for explorer views.
type PublicMetricValue struct {
	Value             *float64 `json:"value,omitempty"`
	UnavailableReason string   `json:"unavailable_reason,omitempty"`
}

// PublicGradeSummary is one disclosure-safe deterministic grade for explorer views.
type PublicGradeSummary struct {
	CriterionID string `json:"criterion_id"`
	Status      string `json:"status"`
	Detail      string `json:"detail,omitempty"`
}

// PublicBenchmarkTiming carries assignment-level timing observations.
type PublicBenchmarkTiming struct {
	ModelLoadMS        *PublicMetricValue `json:"model_load_ms,omitempty"`
	TimeToFirstTokenMS *PublicMetricValue `json:"time_to_first_token_ms,omitempty"`
	GenerationMS       *PublicMetricValue `json:"generation_ms,omitempty"`
	WholeTaskMS        *PublicMetricValue `json:"whole_task_ms,omitempty"`
}

// PublicGPUObservation carries provider-boundary GPU and host RAM peaks.
type PublicGPUObservation struct {
	VRAMBeforeBytes    *PublicMetricValue `json:"vram_before_bytes,omitempty"`
	VRAMPeakBytes      *PublicMetricValue `json:"vram_peak_bytes,omitempty"`
	SystemRAMPeakBytes *PublicMetricValue `json:"system_ram_peak_bytes,omitempty"`
	UtilizationPercent *PublicMetricValue `json:"utilization_percent,omitempty"`
	TemperatureCelsius *PublicMetricValue `json:"temperature_celsius,omitempty"`
	PowerWatts         *PublicMetricValue `json:"power_watts,omitempty"`
	ClockMHz           *PublicMetricValue `json:"clock_mhz,omitempty"`
}

// PublicBenchmarkObservations is the explorer-safe benchmark projection for
// one terminal assignment.
type PublicBenchmarkObservations struct {
	GradeSummaries     []PublicGradeSummary          `json:"grade_summaries,omitempty"`
	ToolScorecard      map[string]*PublicMetricValue `json:"tool_scorecard,omitempty"`
	Timing             *PublicBenchmarkTiming        `json:"timing,omitempty"`
	GPU                *PublicGPUObservation         `json:"gpu,omitempty"`
	UnavailableReasons []string                      `json:"unavailable_reasons"`
}

var toolScorecardDimensions = []string{
	"tool_recognition",
	"tool_selection",
	"argument_schema",
	"argument_semantics",
	"permission_compliance",
	"result_interpretation",
	"follow_up_decision",
	"unnecessary_tool_calls",
	"looping",
	"recovery",
}

var gradeToToolScorecardDimension = map[string]string{
	"tool-selection":     "tool_selection",
	"policy-expectation": "permission_compliance",
	"role-invoked":       "tool_recognition",
	"governed-inference": "follow_up_decision",
}

// CampaignProviderObservationReader loads provider-boundary observation windows
// and governed provider-attempt records for campaign verification.
type CampaignProviderObservationReader struct {
	windows       provider_observer.WindowStore
	attempts      inference.AttemptStore
	localWindows  provider_observer.WindowStore
	localAttempts inference.AttemptStore
	remote        ProviderObservationRemote
}

// NewCampaignProviderObservationReader constructs one read-only provider
// observation accessor from runtime file services.
func NewCampaignProviderObservationReader(fileSvc fs.RuntimeFileService) (*CampaignProviderObservationReader, error) {
	return NewCampaignProviderObservationReaderWithRemote(fileSvc, nil)
}

// NewCampaignProviderObservationReaderWithRemote constructs one read-only
// provider observation accessor from local runtime files with optional gateway
// fallback when local evidence is missing.
func NewCampaignProviderObservationReaderWithRemote(fileSvc fs.RuntimeFileService, remote ProviderObservationRemote) (*CampaignProviderObservationReader, error) {
	if fileSvc == nil {
		return nil, fmt.Errorf("evaluation: provider observation reader: %w", constants.ErrMissingRequiredField)
	}
	localWindows, err := provider_observer.NewWindowStore(fileSvc)
	if err != nil {
		return nil, err
	}
	localAttempts, err := inference.NewAttemptStore(fileSvc)
	if err != nil {
		return nil, err
	}
	windows := localWindows
	attempts := localAttempts
	if remote != nil {
		windows = &fallbackProviderObservationWindows{local: localWindows, remote: remote}
		attempts = &fallbackProviderObservationAttempts{local: localAttempts, remote: remote}
	}
	return &CampaignProviderObservationReader{windows: windows, attempts: attempts, localWindows: localWindows, localAttempts: localAttempts, remote: remote}, nil
}

func (r *CampaignProviderObservationReader) CaptureAssignmentEvidence(ctx context.Context, result *evalv1.EvaluationAssignmentResult) error {
	if r == nil || result == nil || r.remote == nil {
		return nil
	}
	for _, inferenceRecord := range scoredModelInferences(result) {
		attemptID := inferenceRecord.GetProviderAttemptId()
		if attemptID == "" {
			continue
		}
		_, windowErr := r.localWindows.Load(ctx, attemptID)
		if windowErr != nil && !isProviderEvidenceNotFound(windowErr) {
			return fmt.Errorf("evaluation: capture provider observation window: %w", windowErr)
		}
		_, attemptErr := r.localAttempts.Get(ctx, attemptID)
		if attemptErr != nil && !isProviderEvidenceNotFound(attemptErr) {
			return fmt.Errorf("evaluation: capture provider attempt: %w", attemptErr)
		}
		if windowErr == nil && attemptErr == nil {
			continue
		}
		window, attempt, err := r.remote.Load(ctx, attemptID)
		if err != nil {
			if isProviderEvidenceNotFound(err) {
				continue
			}
			return fmt.Errorf("evaluation: capture provider evidence: %w", err)
		}
		if attemptErr != nil {
			if err := r.localAttempts.Import(ctx, attempt); err != nil {
				return fmt.Errorf("evaluation: persist provider attempt: %w", err)
			}
		}
		if windowErr != nil {
			if err := r.localWindows.Save(ctx, window); err != nil {
				return fmt.Errorf("evaluation: persist provider observation window: %w", err)
			}
		}
	}
	return nil
}

type fallbackProviderObservationWindows struct {
	local  provider_observer.WindowStore
	remote ProviderObservationRemote
}

func (s *fallbackProviderObservationWindows) Save(ctx context.Context, window *evalv1.ProviderBoundaryObservationWindow) error {
	return s.local.Save(ctx, window)
}

func (s *fallbackProviderObservationWindows) Load(ctx context.Context, providerAttemptID string) (*evalv1.ProviderBoundaryObservationWindow, error) {
	window, err := s.local.Load(ctx, providerAttemptID)
	if err == nil || !isProviderEvidenceNotFound(err) || s.remote == nil {
		return window, err
	}
	window, _, remoteErr := s.remote.Load(ctx, providerAttemptID)
	if remoteErr != nil {
		return nil, err
	}
	return window, nil
}

type fallbackProviderObservationAttempts struct {
	local  inference.AttemptStore
	remote ProviderObservationRemote
}

func (s *fallbackProviderObservationAttempts) Begin(ctx context.Context, record *operatorv1.InferenceProviderAttemptRecord) error {
	return s.local.Begin(ctx, record)
}

func (s *fallbackProviderObservationAttempts) Import(ctx context.Context, record *operatorv1.InferenceProviderAttemptRecord) error {
	return s.local.Import(ctx, record)
}

func (s *fallbackProviderObservationAttempts) Complete(ctx context.Context, providerAttemptID, resultDigest string) error {
	return s.local.Complete(ctx, providerAttemptID, resultDigest)
}

func (s *fallbackProviderObservationAttempts) Fail(ctx context.Context, providerAttemptID, failureSummary string) error {
	return s.local.Fail(ctx, providerAttemptID, failureSummary)
}

func (s *fallbackProviderObservationAttempts) Get(ctx context.Context, providerAttemptID string) (*operatorv1.InferenceProviderAttemptRecord, error) {
	attempt, err := s.local.Get(ctx, providerAttemptID)
	if err == nil || !isProviderEvidenceNotFound(err) || s.remote == nil {
		return attempt, err
	}
	_, attempt, remoteErr := s.remote.Load(ctx, providerAttemptID)
	if remoteErr != nil {
		return nil, err
	}
	return attempt, nil
}

func isProviderEvidenceNotFound(err error) bool {
	return isGatewayEvidenceNotFound(err)
}

// BindProviderBoundaryObservationRefs attaches observation evidence references
// to scored model inferences when durable windows exist.
func (r *CampaignProviderObservationReader) BindProviderBoundaryObservationRefs(ctx context.Context, result *evalv1.EvaluationAssignmentResult) error {
	if r == nil || result == nil {
		return nil
	}
	for _, inferenceRecord := range scoredModelInferences(result) {
		attemptID := inferenceRecord.GetProviderAttemptId()
		if attemptID == "" {
			continue
		}
		window, err := r.windows.Load(ctx, attemptID)
		if err != nil {
			if errors.Is(err, constants.ErrNotFound) {
				continue
			}
			return fmt.Errorf("evaluation: bind provider boundary observation refs: %w", err)
		}
		inferenceRecord.ProviderBoundaryObservationRef = providerBoundaryObservationRef(window)
	}
	return nil
}

// VerifyAssignmentProviderObservations independently checks provider-boundary
// observation coverage for every scored inference in one assignment result.
func (r *CampaignProviderObservationReader) VerifyAssignmentProviderObservations(
	ctx context.Context,
	result *evalv1.EvaluationAssignmentResult,
	policy ProviderObservationPolicy,
) ([]string, []string) {
	if r == nil || result == nil {
		return nil, nil
	}
	failures := make([]string, 0)
	unavailable := make([]string, 0)
	for _, inferenceRecord := range scoredModelInferences(result) {
		attemptID := inferenceRecord.GetProviderAttemptId()
		if attemptID == "" {
			continue
		}
		window, err := r.windows.Load(ctx, attemptID)
		if err != nil {
			if errors.Is(err, constants.ErrNotFound) {
				unavailable = appendUniqueStrings(unavailable, "source_not_captured")
				if policy == ProviderObservationPolicyStrict {
					failures = append(failures, fmt.Sprintf("inference %s missing provider-boundary observation window", inferenceRecord.GetInferenceRecordId()))
				}
				continue
			}
			failures = append(failures, fmt.Sprintf("inference %s observation window load failed: %v", inferenceRecord.GetInferenceRecordId(), err))
			continue
		}
		attempt, err := r.attempts.Get(ctx, attemptID)
		if err != nil {
			failures = append(failures, fmt.Sprintf("inference %s provider attempt load failed: %v", inferenceRecord.GetInferenceRecordId(), err))
			continue
		}
		report, err := provider_observer.VerifyObservationCoverage(window, attempt)
		if err != nil {
			failures = append(failures, fmt.Sprintf("inference %s observation coverage verification failed: %v", inferenceRecord.GetInferenceRecordId(), err))
			continue
		}
		if report == nil || report.Complete {
			continue
		}
		unavailable = appendUniqueStrings(unavailable, "incomplete_contributor_evidence")
		for _, reason := range report.FailureReasons {
			failures = append(failures, fmt.Sprintf("inference %s: %s", inferenceRecord.GetInferenceRecordId(), reason))
		}
	}
	return failures, unavailable
}

func publicUnavailableMetricReasons(result *evalv1.EvaluationAssignmentResult) []string {
	for _, inference := range result.GetModelInferences() {
		if inference.GetUsageAvailability() == evalv1.EvaluationUsageAvailability_EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE {
			return []string{"source_unavailable"}
		}
	}
	return nil
}

// BuildPublicBenchmarkObservations derives disclosure-safe benchmark telemetry
// from canonical assignment result records and provider-boundary windows.
func (r *CampaignProviderObservationReader) BuildPublicBenchmarkObservations(ctx context.Context, result *evalv1.EvaluationAssignmentResult) (*PublicBenchmarkObservations, error) {
	return r.buildPublicBenchmarkObservations(ctx, result, nil)
}

// BuildPublicBenchmarkObservationsForScenario applies the frozen scenario
// requirements when producing the tool scorecard.
func (r *CampaignProviderObservationReader) BuildPublicBenchmarkObservationsForScenario(ctx context.Context, result *evalv1.EvaluationAssignmentResult, scenario *PublicScenarioContext) (*PublicBenchmarkObservations, error) {
	return r.buildPublicBenchmarkObservations(ctx, result, scenario)
}

func (r *CampaignProviderObservationReader) buildPublicBenchmarkObservations(ctx context.Context, result *evalv1.EvaluationAssignmentResult, scenario *PublicScenarioContext) (*PublicBenchmarkObservations, error) {
	if result == nil {
		return nil, fmt.Errorf("evaluation: build public benchmark observations: %w", constants.ErrMissingRequiredField)
	}
	observations := &PublicBenchmarkObservations{
		UnavailableReasons: publicUnavailableMetricReasons(result),
	}
	timing := deriveBenchmarkTiming(result)
	if timing.hasValues() {
		observations.Timing = timing.toPublic()
	}
	if gradeSummaries := buildPublicGradeSummaries(result); len(gradeSummaries) > 0 {
		observations.GradeSummaries = gradeSummaries
	}
	var scorecard map[string]*PublicMetricValue
	if scenario == nil {
		scorecard = buildToolScorecardObservations(result)
	} else {
		scorecard = buildToolScorecardForScenario(result, scenario)
	}
	if len(scorecard) > 0 {
		observations.ToolScorecard = scorecard
	}
	if r == nil {
		observations.UnavailableReasons = appendUniqueStrings(observations.UnavailableReasons, "source_not_captured")
		return observations, nil
	}
	gpu := &gpuAggregate{}
	for _, inferenceRecord := range scoredModelInferences(result) {
		attemptID := inferenceRecord.GetProviderAttemptId()
		if attemptID == "" {
			continue
		}
		window, err := r.windows.Load(ctx, attemptID)
		if err != nil {
			if isProviderEvidenceNotFound(err) {
				observations.UnavailableReasons = appendUniqueStrings(observations.UnavailableReasons, "source_not_captured")
				continue
			}
			return nil, fmt.Errorf("evaluation: build public benchmark observations: %w", err)
		}
		gpu.observeWindow(window)
	}
	if gpu.hasValues() {
		observations.GPU = gpu.toPublic()
	}
	if len(observations.UnavailableReasons) == 0 {
		observations.UnavailableReasons = nil
	}
	return observations, nil
}

func buildToolScorecardForScenario(result *evalv1.EvaluationAssignmentResult, scenario *PublicScenarioContext) map[string]*PublicMetricValue {
	scorecard := make(map[string]*PublicMetricValue, len(toolScorecardDimensions))
	required := make(map[string]bool, len(scenario.ToolScoreDimensions))
	for _, requirement := range scenario.ToolScoreDimensions {
		if requirement != nil {
			required[requirement.GetDimension().String()] = requirement.GetRequired()
		}
	}
	for _, dimension := range toolScorecardDimensions {
		if !required[toolScoreDimensionEnum(dimension).String()] {
			scorecard[dimension] = &PublicMetricValue{UnavailableReason: "scenario_not_applicable"}
			continue
		}
		scorecard[dimension] = &PublicMetricValue{UnavailableReason: "source_not_captured"}
	}
	for _, grade := range result.GetDeterministicGrades() {
		if grade == nil {
			continue
		}
		dimension, ok := gradeToToolScorecardDimension[grade.GetCriterionId()]
		if ok && required[toolScoreDimensionEnum(dimension).String()] {
			scorecard[dimension] = toolScorecardMetricFromGrade(grade)
		}
	}
	return scorecard
}

func toolScoreDimensionEnum(dimension string) evalv1.PublicToolScoreDimension {
	for _, candidate := range []evalv1.PublicToolScoreDimension{
		evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_TOOL_RECOGNITION,
		evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_TOOL_SELECTION,
		evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_ARGUMENT_SCHEMA,
		evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_ARGUMENT_SEMANTICS,
		evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_PERMISSION_COMPLIANCE,
		evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_RESULT_INTERPRETATION,
		evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_FOLLOW_UP_DECISION,
		evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_UNNECESSARY_TOOL_CALLS,
		evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_LOOPING,
		evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_RECOVERY,
	} {
		if candidate.String() == "PUBLIC_TOOL_SCORE_DIMENSION_"+strings.ToUpper(dimension) {
			return candidate
		}
	}
	return evalv1.PublicToolScoreDimension_PUBLIC_TOOL_SCORE_DIMENSION_UNSPECIFIED
}

func buildToolScorecardObservations(result *evalv1.EvaluationAssignmentResult) map[string]*PublicMetricValue {
	scorecard := make(map[string]*PublicMetricValue, len(toolScorecardDimensions))
	for _, dimension := range toolScorecardDimensions {
		scorecard[dimension] = &PublicMetricValue{
			UnavailableReason: "scenario_not_applicable",
		}
	}
	if result == nil {
		return scorecard
	}
	for _, grade := range result.GetDeterministicGrades() {
		if grade == nil {
			continue
		}
		dimension, ok := gradeToToolScorecardDimension[grade.GetCriterionId()]
		if !ok {
			continue
		}
		scorecard[dimension] = toolScorecardMetricFromGrade(grade)
	}
	return scorecard
}

func toolScorecardMetricFromGrade(grade *evalv1.DeterministicGrade) *PublicMetricValue {
	switch grade.GetStatus() {
	case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS,
		evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL:
		return publicMetricValue(grade.GetScore())
	default:
		return &PublicMetricValue{UnavailableReason: "source_unavailable"}
	}
}

func publicVerdictStatus(status evalv1.EvaluationVerdictStatus) string {
	switch status {
	case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_PASS:
		return "pass"
	case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL:
		return "fail"
	case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE:
		return "unavailable"
	case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNSUPPORTED:
		return "unsupported"
	case evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE:
		return "invalid_evidence"
	default:
		return "unspecified"
	}
}

func scoredModelInferences(result *evalv1.EvaluationAssignmentResult) []*evalv1.ModelInferenceRecord {
	if result == nil {
		return nil
	}
	records := make([]*evalv1.ModelInferenceRecord, 0, len(result.GetModelInferences()))
	for _, record := range result.GetModelInferences() {
		if record == nil {
			continue
		}
		records = append(records, record)
	}
	return records
}

func providerBoundaryObservationRef(window *evalv1.ProviderBoundaryObservationWindow) *compliancev1.ComplianceEvidenceReference {
	if window == nil || window.GetProviderAttemptId() == "" {
		return nil
	}
	return &compliancev1.ComplianceEvidenceReference{
		ArtifactId:   window.GetProviderAttemptId(),
		ArtifactType: providerObservationArtifactType,
		SchemaRef:    providerObservationSchemaRef,
		Sha256:       window.GetObservationDigest(),
	}
}

type benchmarkTimingAggregate struct {
	modelLoadMS        *float64
	timeToFirstTokenMS *float64
	generationMS       *float64
	wholeTaskMS        *float64
}

func deriveBenchmarkTiming(result *evalv1.EvaluationAssignmentResult) benchmarkTimingAggregate {
	aggregate := benchmarkTimingAggregate{}
	for _, record := range scoredModelInferences(result) {
		if record.GetLoadDurationNanos() > 0 {
			aggregate.modelLoadMS = maxFloatPtr(aggregate.modelLoadMS, nanosToMillis(record.GetLoadDurationNanos()))
		}
		if record.GetRequestStartedAtUnixNanos() > 0 && record.GetFirstTokenAtUnixNanos() > record.GetRequestStartedAtUnixNanos() {
			aggregate.timeToFirstTokenMS = maxFloatPtr(
				aggregate.timeToFirstTokenMS,
				nanosToMillis(record.GetFirstTokenAtUnixNanos()-record.GetRequestStartedAtUnixNanos()),
			)
		}
		if record.GetGenerationDurationNanos() > 0 {
			aggregate.generationMS = maxFloatPtr(aggregate.generationMS, nanosToMillis(record.GetGenerationDurationNanos()))
		}
		if record.GetTotalDurationNanos() > 0 {
			aggregate.wholeTaskMS = maxFloatPtr(aggregate.wholeTaskMS, nanosToMillis(record.GetTotalDurationNanos()))
		}
	}
	return aggregate
}

func (aggregate benchmarkTimingAggregate) hasValues() bool {
	return aggregate.modelLoadMS != nil || aggregate.timeToFirstTokenMS != nil || aggregate.generationMS != nil || aggregate.wholeTaskMS != nil
}

func (aggregate benchmarkTimingAggregate) toPublic() *PublicBenchmarkTiming {
	timing := &PublicBenchmarkTiming{}
	if aggregate.modelLoadMS != nil {
		timing.ModelLoadMS = publicMetricValue(*aggregate.modelLoadMS)
	}
	if aggregate.timeToFirstTokenMS != nil {
		timing.TimeToFirstTokenMS = publicMetricValue(*aggregate.timeToFirstTokenMS)
	}
	if aggregate.generationMS != nil {
		timing.GenerationMS = publicMetricValue(*aggregate.generationMS)
	}
	if aggregate.wholeTaskMS != nil {
		timing.WholeTaskMS = publicMetricValue(*aggregate.wholeTaskMS)
	}
	return timing
}

type gpuMetricProjection struct {
	value             *float64
	unavailableReason string
}

type gpuAggregate struct {
	vramBefore       *uint64
	vramPeak         *uint64
	hostRAMPeak      *uint64
	utilizationPeak  *float64
	temperaturePeak  *float64
	powerPeak        *float64
	clockPeak        *uint32
	seenVRAMBefore   bool
	windowObserved   bool
	vramBeforeState  gpuMetricProjection
	vramPeakState    gpuMetricProjection
	hostRAMState     gpuMetricProjection
	utilizationState gpuMetricProjection
	temperatureState gpuMetricProjection
	powerState       gpuMetricProjection
	clockState       gpuMetricProjection
}

func (aggregate *gpuAggregate) observeWindow(window *evalv1.ProviderBoundaryObservationWindow) {
	if aggregate == nil || window == nil {
		return
	}
	aggregate.windowObserved = true
	for index, sample := range window.GetSamples() {
		if sample == nil {
			continue
		}
		aggregate.observeVRAMBefore(sample, index)
		aggregate.observeVRAMPeak(sample)
		aggregate.observeHostRAM(sample)
		aggregate.observeUtilization(sample)
		aggregate.observeTemperature(sample)
		aggregate.observePower(sample)
		aggregate.observeClock(sample)
	}
}

func (aggregate *gpuAggregate) observeVRAMBefore(sample *evalv1.ProviderBoundaryHardwareSample, index int) {
	switch sample.GetVramBytesAvailability() {
	case evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED:
		if !aggregate.seenVRAMBefore {
			aggregate.vramBefore = uint64Ptr(sample.GetVramUsedBytes())
			aggregate.vramBeforeState.value = float64Ptr(float64(sample.GetVramUsedBytes()))
			aggregate.seenVRAMBefore = true
		}
	case evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE:
		aggregate.vramBeforeState.unavailableReason = gpuMetricUnavailableReason("vram_before_bytes")
	}
	if index == 0 && !aggregate.seenVRAMBefore && sample.GetVramBytesAvailability() != evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
		aggregate.seenVRAMBefore = true
	}
}

func (aggregate *gpuAggregate) observeVRAMPeak(sample *evalv1.ProviderBoundaryHardwareSample) {
	switch sample.GetVramBytesAvailability() {
	case evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED:
		aggregate.vramPeak = maxUint64Ptr(aggregate.vramPeak, sample.GetVramUsedBytes())
		aggregate.vramPeakState.value = maxFloatPtr(aggregate.vramPeakState.value, float64(sample.GetVramUsedBytes()))
	case evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE:
		aggregate.vramPeakState.unavailableReason = gpuMetricUnavailableReason("vram_peak_bytes")
	}
}

func (aggregate *gpuAggregate) observeHostRAM(sample *evalv1.ProviderBoundaryHardwareSample) {
	switch sample.GetHostRamAvailability() {
	case evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED:
		aggregate.hostRAMPeak = maxUint64Ptr(aggregate.hostRAMPeak, sample.GetHostRamUsedBytes())
		aggregate.hostRAMState.value = maxFloatPtr(aggregate.hostRAMState.value, float64(sample.GetHostRamUsedBytes()))
	case evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE:
		aggregate.hostRAMState.unavailableReason = gpuMetricUnavailableReason("system_ram_peak_bytes")
	}
}

func (aggregate *gpuAggregate) observeUtilization(sample *evalv1.ProviderBoundaryHardwareSample) {
	switch sample.GetGpuUtilizationAvailability() {
	case evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED:
		aggregate.utilizationPeak = maxFloatPtr(aggregate.utilizationPeak, float64(sample.GetGpuUtilizationPercent()))
		aggregate.utilizationState.value = maxFloatPtr(aggregate.utilizationState.value, float64(sample.GetGpuUtilizationPercent()))
	case evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE:
		aggregate.utilizationState.unavailableReason = gpuMetricUnavailableReason("utilization_percent")
	}
}

func (aggregate *gpuAggregate) observeTemperature(sample *evalv1.ProviderBoundaryHardwareSample) {
	switch sample.GetTemperatureAvailability() {
	case evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED:
		aggregate.temperaturePeak = maxFloatPtr(aggregate.temperaturePeak, float64(sample.GetTemperatureCelsius()))
		aggregate.temperatureState.value = maxFloatPtr(aggregate.temperatureState.value, float64(sample.GetTemperatureCelsius()))
	case evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE:
		aggregate.temperatureState.unavailableReason = gpuMetricUnavailableReason("temperature_celsius")
	}
}

func (aggregate *gpuAggregate) observePower(sample *evalv1.ProviderBoundaryHardwareSample) {
	switch sample.GetPowerAvailability() {
	case evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED:
		aggregate.powerPeak = maxFloatPtr(aggregate.powerPeak, float64(sample.GetPowerWatts()))
		aggregate.powerState.value = maxFloatPtr(aggregate.powerState.value, float64(sample.GetPowerWatts()))
	case evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE:
		aggregate.powerState.unavailableReason = gpuMetricUnavailableReason("power_watts")
	}
}

func (aggregate *gpuAggregate) observeClock(sample *evalv1.ProviderBoundaryHardwareSample) {
	switch sample.GetClockAvailability() {
	case evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED:
		aggregate.clockPeak = maxUint32Ptr(aggregate.clockPeak, sample.GetClockMhz())
		if aggregate.clockState.value == nil || float64(sample.GetClockMhz()) > *aggregate.clockState.value {
			aggregate.clockState.value = float64Ptr(float64(sample.GetClockMhz()))
		}
	case evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE:
		aggregate.clockState.unavailableReason = gpuMetricUnavailableReason("clock_mhz")
	}
}

func gpuMetricUnavailableReason(metric string) string {
	return "source_unavailable"
}

func (aggregate *gpuAggregate) hasValues() bool {
	return aggregate.vramBefore != nil || aggregate.vramPeak != nil || aggregate.hostRAMPeak != nil ||
		aggregate.utilizationPeak != nil || aggregate.temperaturePeak != nil || aggregate.powerPeak != nil || aggregate.clockPeak != nil ||
		aggregate.windowObserved && (aggregate.vramBeforeState.unavailableReason != "" || aggregate.vramPeakState.unavailableReason != "" ||
			aggregate.hostRAMState.unavailableReason != "" || aggregate.utilizationState.unavailableReason != "" ||
			aggregate.temperatureState.unavailableReason != "" || aggregate.powerState.unavailableReason != "" ||
			aggregate.clockState.unavailableReason != "")
}

func (aggregate *gpuAggregate) toPublic() *PublicGPUObservation {
	gpu := &PublicGPUObservation{}
	gpu.VRAMBeforeBytes = aggregate.projectMetric(aggregate.vramBeforeState)
	gpu.VRAMPeakBytes = aggregate.projectMetric(aggregate.vramPeakState)
	gpu.SystemRAMPeakBytes = aggregate.projectMetric(aggregate.hostRAMState)
	gpu.UtilizationPercent = aggregate.projectMetric(aggregate.utilizationState)
	gpu.TemperatureCelsius = aggregate.projectMetric(aggregate.temperatureState)
	gpu.PowerWatts = aggregate.projectMetric(aggregate.powerState)
	gpu.ClockMHz = aggregate.projectMetric(aggregate.clockState)
	return gpu
}

func (aggregate *gpuAggregate) projectMetric(state gpuMetricProjection) *PublicMetricValue {
	if state.value != nil {
		return publicMetricValue(*state.value)
	}
	if aggregate.windowObserved && state.unavailableReason != "" {
		return &PublicMetricValue{UnavailableReason: state.unavailableReason}
	}
	return nil
}

func publicMetricValue(value float64) *PublicMetricValue {
	copied := value
	return &PublicMetricValue{Value: &copied}
}

func float64Ptr(value float64) *float64 {
	return &value
}

func nanosToMillis(value uint64) float64 {
	return float64(value) / 1_000_000
}

func maxFloatPtr(current *float64, candidate float64) *float64 {
	if math.IsNaN(candidate) || math.IsInf(candidate, 0) {
		return current
	}
	if current == nil || candidate > *current {
		value := candidate
		return &value
	}
	return current
}

func maxUint64Ptr(current *uint64, candidate uint64) *uint64 {
	if current == nil || candidate > *current {
		value := candidate
		return &value
	}
	return current
}

func maxUint32Ptr(current *uint32, candidate uint32) *uint32 {
	if current == nil || candidate > *current {
		value := candidate
		return &value
	}
	return current
}

func uint64Ptr(value uint64) *uint64 {
	return &value
}

func appendUniqueStrings(values []string, candidate string) []string {
	for _, existing := range values {
		if existing == candidate {
			return values
		}
	}
	return append(values, candidate)
}
