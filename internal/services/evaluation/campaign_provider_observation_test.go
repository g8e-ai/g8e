// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestCampaignProviderObservationReader_BuildPublicBenchmarkObservations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	attempt := &operatorv1.InferenceProviderAttemptRecord{
		ProviderAttemptId: "attempt-1",
		Status:            operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_COMPLETED,
		StartedAtUnixMs:   time.Unix(1_700_000_000, 0).UnixMilli(),
		CompletedAtUnixMs: time.Unix(1_700_000_010, 0).UnixMilli(),
	}
	require.NoError(t, writeProviderAttemptRecord(ctx, fileSvc, attempt))
	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:              provider_observer.SchemaVersion,
		ProviderAttemptId:          "attempt-1",
		ObserverId:                 "observer-test",
		ObserverClockSource:        provider_observer.DefaultObserverClockSource,
		WindowStartedAtUnixNanos:   uint64(time.Unix(1_700_000_000, 0).UnixNano()),
		WindowCompletedAtUnixNanos: uint64(time.Unix(1_700_000_010, 0).UnixNano()),
		AttemptStartedAtUnixMs:     attempt.GetStartedAtUnixMs(),
		AttemptCompletedAtUnixMs:   attempt.GetCompletedAtUnixMs(),
		Samples: []*evalv1.ProviderBoundaryHardwareSample{
			{
				ObservedAtUnixNanos:        uint64(time.Unix(1_700_000_001, 0).UnixNano()),
				VramBytesAvailability:      evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
				VramUsedBytes:              1000,
				GpuUtilizationAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
				GpuUtilizationPercent:      42,
				HostRamAvailability:        evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
				HostRamUsedBytes:           2000,
			},
			{
				ObservedAtUnixNanos:   uint64(time.Unix(1_700_000_005, 0).UnixNano()),
				VramBytesAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
				VramUsedBytes:         3000,
				HostRamAvailability:   evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
				HostRamUsedBytes:      4000,
			},
		},
	}
	digest, err := provider_observer.ComputeObservationDigest(window)
	require.NoError(t, err)
	window.ObservationDigest = digest
	windowStore, err := provider_observer.NewWindowStore(fileSvc)
	require.NoError(t, err)
	require.NoError(t, windowStore.Save(ctx, window))

	reader, err := NewCampaignProviderObservationReader(fileSvc)
	require.NoError(t, err)
	result := &evalv1.EvaluationAssignmentResult{
		ModelInferences: []*evalv1.ModelInferenceRecord{{
			InferenceRecordId:       "inference-1",
			ProviderAttemptId:       "attempt-1",
			LoadDurationNanos:       2_000_000,
			GenerationDurationNanos: 40_000_000,
			TotalDurationNanos:      52_000_000,
		}},
	}
	benchmark, err := reader.BuildPublicBenchmarkObservations(ctx, result)
	require.NoError(t, err)
	require.NotNil(t, benchmark)
	require.NotNil(t, benchmark.Timing)
	assert.Equal(t, 2.0, *benchmark.Timing.ModelLoadMS.Value)
	assert.Equal(t, 40.0, *benchmark.Timing.GenerationMS.Value)
	require.NotNil(t, benchmark.GPU)
	assert.Equal(t, 1000.0, *benchmark.GPU.VRAMBeforeBytes.Value)
	assert.Equal(t, 3000.0, *benchmark.GPU.VRAMPeakBytes.Value)
	assert.Equal(t, 4000.0, *benchmark.GPU.SystemRAMPeakBytes.Value)
	assert.Equal(t, 42.0, *benchmark.GPU.UtilizationPercent.Value)
}

func TestBuildPublicGradeSummaries_FromDeterministicGrades(t *testing.T) {
	t.Parallel()
	summaries := buildPublicGradeSummaries(&evalv1.EvaluationAssignmentResult{
		DeterministicGrades: []*evalv1.DeterministicGrade{{
			CriterionId: "tool-selection",
			Status:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
			Detail:      "expected tool selection evidence is missing",
		}},
	})
	require.Len(t, summaries, 1)
	assert.Equal(t, "tool-selection", summaries[0].CriterionID)
	assert.Equal(t, "fail", summaries[0].Status)
	assert.Empty(t, summaries[0].Detail)
}

func TestBuildToolScorecardObservations_FailingToolSelection(t *testing.T) {
	t.Parallel()
	scorecard := buildToolScorecardObservations(&evalv1.EvaluationAssignmentResult{
		DeterministicGrades: []*evalv1.DeterministicGrade{{
			CriterionId: "tool-selection",
			Status:      evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
			Score:       0,
		}},
	})
	require.NotNil(t, scorecard["tool_selection"])
	assert.Equal(t, 0.0, *scorecard["tool_selection"].Value)
	assert.Equal(t, "scenario_not_applicable", scorecard["tool_recognition"].UnavailableReason)
}

func TestBuildPublicBenchmarkObservations_AllUnavailableGPUMetrics(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	attempt := &operatorv1.InferenceProviderAttemptRecord{
		ProviderAttemptId: "attempt-unavail",
		Status:            operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_COMPLETED,
		StartedAtUnixMs:   time.Unix(1_700_000_000, 0).UnixMilli(),
		CompletedAtUnixMs: time.Unix(1_700_000_010, 0).UnixMilli(),
	}
	require.NoError(t, writeProviderAttemptRecord(ctx, fileSvc, attempt))
	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:              provider_observer.SchemaVersion,
		ProviderAttemptId:          "attempt-unavail",
		ObserverId:                 "observer-test",
		ObserverClockSource:        provider_observer.DefaultObserverClockSource,
		WindowStartedAtUnixNanos:   uint64(time.Unix(1_700_000_000, 0).UnixNano()),
		WindowCompletedAtUnixNanos: uint64(time.Unix(1_700_000_010, 0).UnixNano()),
		AttemptStartedAtUnixMs:     attempt.GetStartedAtUnixMs(),
		AttemptCompletedAtUnixMs:   attempt.GetCompletedAtUnixMs(),
		Samples: []*evalv1.ProviderBoundaryHardwareSample{{
			ObservedAtUnixNanos:        uint64(time.Unix(1_700_000_001, 0).UnixNano()),
			VramBytesAvailability:      evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE,
			GpuUtilizationAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE,
			TemperatureAvailability:    evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE,
			PowerAvailability:          evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE,
			ClockAvailability:          evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE,
			HostRamAvailability:        evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE,
		}},
	}
	digest, err := provider_observer.ComputeObservationDigest(window)
	require.NoError(t, err)
	window.ObservationDigest = digest
	windowStore, err := provider_observer.NewWindowStore(fileSvc)
	require.NoError(t, err)
	require.NoError(t, windowStore.Save(ctx, window))

	reader, err := NewCampaignProviderObservationReader(fileSvc)
	require.NoError(t, err)
	benchmark, err := reader.BuildPublicBenchmarkObservations(ctx, &evalv1.EvaluationAssignmentResult{
		ModelInferences: []*evalv1.ModelInferenceRecord{{
			InferenceRecordId: "inference-1",
			ProviderAttemptId: "attempt-unavail",
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, benchmark.GPU)
	assert.Equal(t, "provider gpu collector unavailable: vram_before_bytes", benchmark.GPU.VRAMBeforeBytes.UnavailableReason)
	assert.Equal(t, "provider gpu collector unavailable: vram_peak_bytes", benchmark.GPU.VRAMPeakBytes.UnavailableReason)
	assert.Equal(t, "provider gpu collector unavailable: system_ram_peak_bytes", benchmark.GPU.SystemRAMPeakBytes.UnavailableReason)
	assert.Equal(t, "provider gpu collector unavailable: utilization_percent", benchmark.GPU.UtilizationPercent.UnavailableReason)
	assert.Equal(t, "provider gpu collector unavailable: temperature_celsius", benchmark.GPU.TemperatureCelsius.UnavailableReason)
	assert.Equal(t, "provider gpu collector unavailable: power_watts", benchmark.GPU.PowerWatts.UnavailableReason)
	assert.Equal(t, "provider gpu collector unavailable: clock_mhz", benchmark.GPU.ClockMHz.UnavailableReason)
}

func TestMarshalAssignmentResultProjectionEnvelope_IncludesToolScorecard(t *testing.T) {
	t.Parallel()
	projection := &evalv1.PublicAssignmentResultProjection{
		AssignmentId: "assign-1",
		RunId:        "run-1",
		ScenarioId:   "scenario-1",
	}
	body, err := MarshalAssignmentResultProjectionEnvelope("run-1:assign-1:result", &PublicAssignmentRecord{Projection: projection, Extensions: PublicAssignmentRecordExtensions{BenchmarkObservations: &PublicBenchmarkObservations{
		GradeSummaries: []PublicGradeSummary{{
			CriterionID: "tool-selection",
			Status:      "fail",
		}},
		ToolScorecard: map[string]*PublicMetricValue{
			"tool_selection": publicMetricValue(0),
		},
	}}})
	require.NoError(t, err)
	envelope := CampaignProjectionEnvelope{}
	require.NoError(t, json.Unmarshal(body, &envelope))
	record := map[string]any{}
	require.NoError(t, json.Unmarshal(envelope.Record, &record))
	benchmark, ok := record["benchmark_observations"].(map[string]any)
	require.True(t, ok)
	summaries, ok := benchmark["grade_summaries"].([]any)
	require.True(t, ok)
	require.Len(t, summaries, 1)
	scorecard, ok := benchmark["tool_scorecard"].(map[string]any)
	require.True(t, ok)
	selection, ok := scorecard["tool_selection"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 0.0, selection["value"])
}

func TestCampaignProviderObservationReader_VerifyAssignmentProviderObservations_InterimMissingWindow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	reader, err := NewCampaignProviderObservationReader(fileSvc)
	require.NoError(t, err)
	result := &evalv1.EvaluationAssignmentResult{
		ModelInferences: []*evalv1.ModelInferenceRecord{{
			InferenceRecordId: "inference-1",
			ProviderAttemptId: "attempt-missing",
		}},
	}
	failures, unavailable := reader.VerifyAssignmentProviderObservations(ctx, result, ProviderObservationPolicyInterim)
	assert.Empty(t, failures)
	assert.Equal(t, []string{"source_not_captured"}, unavailable)
}

type stubProviderObservationRemote struct {
	window  *evalv1.ProviderBoundaryObservationWindow
	attempt *operatorv1.InferenceProviderAttemptRecord
}

func (s *stubProviderObservationRemote) Load(_ context.Context, providerAttemptID string) (*evalv1.ProviderBoundaryObservationWindow, *operatorv1.InferenceProviderAttemptRecord, error) {
	if s == nil || s.window == nil || s.attempt == nil || s.window.GetProviderAttemptId() != providerAttemptID {
		return nil, nil, constants.ErrNotFound
	}
	return s.window, s.attempt, nil
}

func TestCampaignProviderObservationReaderWithRemote_LoadsGatewayEvidenceOnLocalMiss(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	attempt := &operatorv1.InferenceProviderAttemptRecord{
		ProviderAttemptId: "attempt-remote",
		Status:            operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_COMPLETED,
		StartedAtUnixMs:   time.Unix(1_700_000_000, 0).UnixMilli(),
		CompletedAtUnixMs: time.Unix(1_700_000_010, 0).UnixMilli(),
	}
	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:              provider_observer.SchemaVersion,
		ProviderAttemptId:          "attempt-remote",
		ObserverId:                 "observer-test",
		ObserverClockSource:        provider_observer.DefaultObserverClockSource,
		WindowStartedAtUnixNanos:   uint64(time.Unix(1_700_000_000, 0).UnixNano()),
		WindowCompletedAtUnixNanos: uint64(time.Unix(1_700_000_010, 0).UnixNano()),
		AttemptStartedAtUnixMs:     attempt.GetStartedAtUnixMs(),
		AttemptCompletedAtUnixMs:   attempt.GetCompletedAtUnixMs(),
		Samples: []*evalv1.ProviderBoundaryHardwareSample{{
			ObservedAtUnixNanos:        uint64(time.Unix(1_700_000_001, 0).UnixNano()),
			GpuUtilizationAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			GpuUtilizationPercent:      10,
		}},
	}
	digest, err := provider_observer.ComputeObservationDigest(window)
	require.NoError(t, err)
	window.ObservationDigest = digest

	reader, err := NewCampaignProviderObservationReaderWithRemote(fileSvc, &stubProviderObservationRemote{
		window:  window,
		attempt: attempt,
	})
	require.NoError(t, err)
	result := &evalv1.EvaluationAssignmentResult{
		ModelInferences: []*evalv1.ModelInferenceRecord{{
			InferenceRecordId: "inference-1",
			ProviderAttemptId: "attempt-remote",
		}},
	}
	failures, unavailable := reader.VerifyAssignmentProviderObservations(ctx, result, ProviderObservationPolicyStrict)
	assert.Empty(t, failures)
	assert.Empty(t, unavailable)
}

func TestCampaignProviderObservationReader_VerifyAssignmentProviderObservations_StrictMissingWindow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	reader, err := NewCampaignProviderObservationReader(fileSvc)
	require.NoError(t, err)
	result := &evalv1.EvaluationAssignmentResult{
		ModelInferences: []*evalv1.ModelInferenceRecord{{
			InferenceRecordId: "inference-1",
			ProviderAttemptId: "attempt-missing",
		}},
	}
	failures, unavailable := reader.VerifyAssignmentProviderObservations(ctx, result, ProviderObservationPolicyStrict)
	assert.NotEmpty(t, failures)
	assert.Equal(t, []string{"source_not_captured"}, unavailable)
}

func TestMarshalAssignmentResultProjectionEnvelope_IncludesBenchmarkObservations(t *testing.T) {
	t.Parallel()
	projection := &evalv1.PublicAssignmentResultProjection{
		AssignmentId: "assign-1",
		RunId:        "run-1",
		ScenarioId:   "scenario-1",
	}
	body, err := MarshalAssignmentResultProjectionEnvelope("run-1:assign-1:result", &PublicAssignmentRecord{Projection: projection, Extensions: PublicAssignmentRecordExtensions{BenchmarkObservations: &PublicBenchmarkObservations{
		GPU: &PublicGPUObservation{
			VRAMPeakBytes: publicMetricValue(4096),
		},
		UnavailableReasons: []string{"source_not_captured"},
	}}})
	require.NoError(t, err)
	envelope := CampaignProjectionEnvelope{}
	require.NoError(t, json.Unmarshal(body, &envelope))
	record := map[string]any{}
	require.NoError(t, json.Unmarshal(envelope.Record, &record))
	benchmark, ok := record["benchmark_observations"].(map[string]any)
	require.True(t, ok)
	gpu, ok := benchmark["gpu"].(map[string]any)
	require.True(t, ok)
	peak, ok := gpu["vram_peak_bytes"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 4096.0, peak["value"])
}

func TestCampaignRunVerifier_WithReaders(t *testing.T) {
	files := newCampaignMemoryFileService()
	reader, err := NewCampaignProviderObservationReader(files)
	require.NoError(t, err)
	provenanceReader, err := NewCampaignModelProvenanceReader(files)
	require.NoError(t, err)

	verifier := NewCampaignRunVerifier(func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }).
		WithProviderObservationReader(reader, ProviderObservationPolicyInterim).
		WithModelProvenanceReader(provenanceReader, ModelProvenancePolicyInterim)
	require.NotNil(t, verifier)
	assert.Equal(t, ProviderObservationPolicyInterim, verifier.providerObservationPolicy)
	assert.Equal(t, ModelProvenancePolicyInterim, verifier.modelProvenancePolicy)
}

func TestBuildPublicBenchmarkObservations_UnconfiguredObserverKeepsResultTelemetry(t *testing.T) {
	t.Parallel()
	result := &evalv1.EvaluationAssignmentResult{ModelInferences: []*evalv1.ModelInferenceRecord{{LoadDurationNanos: 1_000_000, GenerationDurationNanos: 2_000_000, TotalDurationNanos: 3_000_000}}}
	benchmark, err := (*CampaignProviderObservationReader)(nil).BuildPublicBenchmarkObservations(context.Background(), result)
	require.NoError(t, err)
	require.NotNil(t, benchmark.Timing)
	assert.Equal(t, 1.0, *benchmark.Timing.ModelLoadMS.Value)
	assert.Contains(t, benchmark.UnavailableReasons, "source_not_captured")
}

func writeProviderAttemptRecord(ctx context.Context, fileSvc fs.RuntimeFileService, record *operatorv1.InferenceProviderAttemptRecord) error {
	dir := filepath.Join(constants.DataDirname, constants.InferenceDirname, constants.InferenceAttemptsDirname)
	if err := fileSvc.MkdirAll(ctx, dir, constants.PermDirStandard); err != nil {
		return err
	}
	body, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(record)
	if err != nil {
		return err
	}
	return fileSvc.WriteFile(ctx, filepath.Join(dir, record.GetProviderAttemptId()+constants.FileExtJSON), body, constants.PermFilePrivate)
}
