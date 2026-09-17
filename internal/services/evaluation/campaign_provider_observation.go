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
	Value float64 `json:"value"`
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
	Timing             *PublicBenchmarkTiming `json:"timing,omitempty"`
	GPU                *PublicGPUObservation  `json:"gpu,omitempty"`
	UnavailableReasons []string               `json:"unavailable_reasons"`
}

// CampaignProviderObservationReader loads provider-boundary observation windows
// and governed provider-attempt records for campaign verification.
type CampaignProviderObservationReader struct {
	windows  provider_observer.WindowStore
	attempts inference.AttemptStore
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
	return &CampaignProviderObservationReader{windows: windows, attempts: attempts}, nil
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
	if err == nil || !errors.Is(err, constants.ErrNotFound) || s.remote == nil {
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

func (s *fallbackProviderObservationAttempts) Complete(ctx context.Context, providerAttemptID, resultDigest string) error {
	return s.local.Complete(ctx, providerAttemptID, resultDigest)
}

func (s *fallbackProviderObservationAttempts) Fail(ctx context.Context, providerAttemptID, failureSummary string) error {
	return s.local.Fail(ctx, providerAttemptID, failureSummary)
}

func (s *fallbackProviderObservationAttempts) Get(ctx context.Context, providerAttemptID string) (*operatorv1.InferenceProviderAttemptRecord, error) {
	attempt, err := s.local.Get(ctx, providerAttemptID)
	if err == nil || !errors.Is(err, constants.ErrNotFound) || s.remote == nil {
		return attempt, err
	}
	_, attempt, remoteErr := s.remote.Load(ctx, providerAttemptID)
	if remoteErr != nil {
		return nil, err
	}
	return attempt, nil
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
				reason := fmt.Sprintf("provider_boundary_observation_missing:%s", attemptID)
				unavailable = append(unavailable, reason)
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
		unavailable = append(unavailable, fmt.Sprintf("provider_boundary_observation_incomplete:%s", attemptID))
		for _, reason := range report.FailureReasons {
			failures = append(failures, fmt.Sprintf("inference %s: %s", inferenceRecord.GetInferenceRecordId(), reason))
		}
	}
	return failures, unavailable
}

// BuildPublicBenchmarkObservations derives disclosure-safe benchmark telemetry
// from canonical assignment result records and provider-boundary windows.
func (r *CampaignProviderObservationReader) BuildPublicBenchmarkObservations(ctx context.Context, result *evalv1.EvaluationAssignmentResult) (*PublicBenchmarkObservations, error) {
	if result == nil {
		return nil, fmt.Errorf("evaluation: build public benchmark observations: %w", constants.ErrMissingRequiredField)
	}
	observations := &PublicBenchmarkObservations{
		UnavailableReasons: append([]string(nil), collectUnavailableMetricReasons(result)...),
	}
	if r == nil {
		observations.UnavailableReasons = appendUniqueStrings(observations.UnavailableReasons, "provider_boundary_observer_unconfigured")
		return observations, nil
	}
	timing := deriveBenchmarkTiming(result)
	gpu := &gpuAggregate{}
	for _, inferenceRecord := range scoredModelInferences(result) {
		attemptID := inferenceRecord.GetProviderAttemptId()
		if attemptID == "" {
			continue
		}
		window, err := r.windows.Load(ctx, attemptID)
		if err != nil {
			if errors.Is(err, constants.ErrNotFound) {
				observations.UnavailableReasons = appendUniqueStrings(observations.UnavailableReasons, "provider_boundary_observation_missing:"+attemptID)
				continue
			}
			return nil, fmt.Errorf("evaluation: build public benchmark observations: %w", err)
		}
		gpu.observeWindow(window)
	}
	if timing.hasValues() {
		observations.Timing = timing.toPublic()
	}
	if gpu.hasValues() {
		observations.GPU = gpu.toPublic()
	}
	if len(observations.UnavailableReasons) == 0 {
		observations.UnavailableReasons = nil
	}
	return observations, nil
}

func scoredModelInferences(result *evalv1.EvaluationAssignmentResult) []*evalv1.ModelInferenceRecord {
	if result == nil {
		return nil
	}
	records := make([]*evalv1.ModelInferenceRecord, 0, len(result.GetModelInferences()))
	for _, record := range result.GetModelInferences() {
		if record == nil || record.GetProviderAttemptId() == "" {
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
		timing.ModelLoadMS = &PublicMetricValue{Value: *aggregate.modelLoadMS}
	}
	if aggregate.timeToFirstTokenMS != nil {
		timing.TimeToFirstTokenMS = &PublicMetricValue{Value: *aggregate.timeToFirstTokenMS}
	}
	if aggregate.generationMS != nil {
		timing.GenerationMS = &PublicMetricValue{Value: *aggregate.generationMS}
	}
	if aggregate.wholeTaskMS != nil {
		timing.WholeTaskMS = &PublicMetricValue{Value: *aggregate.wholeTaskMS}
	}
	return timing
}

type gpuAggregate struct {
	vramBefore      *uint64
	vramPeak        *uint64
	hostRAMPeak     *uint64
	utilizationPeak *float64
	temperaturePeak *float64
	powerPeak       *float64
	clockPeak       *uint32
	seenVRAMBefore  bool
}

func (aggregate *gpuAggregate) observeWindow(window *evalv1.ProviderBoundaryObservationWindow) {
	if aggregate == nil || window == nil {
		return
	}
	for index, sample := range window.GetSamples() {
		if sample == nil {
			continue
		}
		if sample.GetVramBytesAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
			if !aggregate.seenVRAMBefore {
				aggregate.vramBefore = uint64Ptr(sample.GetVramUsedBytes())
				aggregate.seenVRAMBefore = true
			}
			aggregate.vramPeak = maxUint64Ptr(aggregate.vramPeak, sample.GetVramUsedBytes())
		}
		if sample.GetHostRamAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
			aggregate.hostRAMPeak = maxUint64Ptr(aggregate.hostRAMPeak, sample.GetHostRamUsedBytes())
		}
		if sample.GetGpuUtilizationAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
			aggregate.utilizationPeak = maxFloatPtr(aggregate.utilizationPeak, float64(sample.GetGpuUtilizationPercent()))
		}
		if sample.GetTemperatureAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
			aggregate.temperaturePeak = maxFloatPtr(aggregate.temperaturePeak, float64(sample.GetTemperatureCelsius()))
		}
		if sample.GetPowerAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
			aggregate.powerPeak = maxFloatPtr(aggregate.powerPeak, float64(sample.GetPowerWatts()))
		}
		if sample.GetClockAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
			aggregate.clockPeak = maxUint32Ptr(aggregate.clockPeak, sample.GetClockMhz())
		}
		if index == 0 && !aggregate.seenVRAMBefore && sample.GetVramBytesAvailability() != evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
			aggregate.seenVRAMBefore = true
		}
	}
}

func (aggregate *gpuAggregate) hasValues() bool {
	return aggregate.vramBefore != nil || aggregate.vramPeak != nil || aggregate.hostRAMPeak != nil ||
		aggregate.utilizationPeak != nil || aggregate.temperaturePeak != nil || aggregate.powerPeak != nil || aggregate.clockPeak != nil
}

func (aggregate *gpuAggregate) toPublic() *PublicGPUObservation {
	gpu := &PublicGPUObservation{}
	if aggregate.vramBefore != nil {
		gpu.VRAMBeforeBytes = &PublicMetricValue{Value: float64(*aggregate.vramBefore)}
	}
	if aggregate.vramPeak != nil {
		gpu.VRAMPeakBytes = &PublicMetricValue{Value: float64(*aggregate.vramPeak)}
	}
	if aggregate.hostRAMPeak != nil {
		gpu.SystemRAMPeakBytes = &PublicMetricValue{Value: float64(*aggregate.hostRAMPeak)}
	}
	if aggregate.utilizationPeak != nil {
		gpu.UtilizationPercent = &PublicMetricValue{Value: *aggregate.utilizationPeak}
	}
	if aggregate.temperaturePeak != nil {
		gpu.TemperatureCelsius = &PublicMetricValue{Value: *aggregate.temperaturePeak}
	}
	if aggregate.powerPeak != nil {
		gpu.PowerWatts = &PublicMetricValue{Value: *aggregate.powerPeak}
	}
	if aggregate.clockPeak != nil {
		gpu.ClockMHz = &PublicMetricValue{Value: float64(*aggregate.clockPeak)}
	}
	return gpu
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
