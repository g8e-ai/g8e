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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	testVRAMBytes    = uint64(24) << 30
	testHostRAMBytes = uint64(64) << 30
)

func testProviderEnvironment() *ProviderEnvironment {
	return &ProviderEnvironment{Source: ProviderEnvironmentSourceObserved, Memory: "64 GiB", Graphics: "24 GiB VRAM"}
}

func TestFormatCapacity(t *testing.T) {
	cases := map[string]struct {
		bytes uint64
		want  string
	}{
		"exact GiB":                    {24 << 30, "24 GiB"},
		"nvidia-smi MiB rounds up":     {24564 * 1024 * 1024, "24 GiB"},
		"reserved memory rounds up":    {67_323_612_365, "63 GiB"},
		"half a GiB rounds up":         {1 << 29, "1 GiB"},
		"below half a GiB is in MiB":   {100 << 20, "100 MiB"},
		"just under a half GiB in MiB": {(1 << 29) - 1, "512 MiB"},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, formatCapacity(test.bytes))
		})
	}
}

func TestObservedProviderEnvironmentFromCapacities(t *testing.T) {
	set := func(values ...uint64) map[uint64]struct{} {
		out := map[uint64]struct{}{}
		for _, value := range values {
			out[value] = struct{}{}
		}
		return out
	}

	assert.Equal(t, testProviderEnvironment(), observedProviderEnvironment(set(testVRAMBytes), set(testHostRAMBytes)))
	assert.Equal(t,
		&ProviderEnvironment{Source: ProviderEnvironmentSourceObserved, Memory: "64 GiB"},
		observedProviderEnvironment(set(), set(testHostRAMBytes)),
		"a capacity the observer never reported is omitted, not guessed",
	)
	assert.Equal(t,
		&ProviderEnvironment{Source: ProviderEnvironmentSourceObserved, Graphics: "24 GiB VRAM"},
		observedProviderEnvironment(set(testVRAMBytes), set()),
	)
	assert.Nil(t, observedProviderEnvironment(set(), set()), "no reported capacity means no environment")
	assert.Nil(t, observedProviderEnvironment(set(testVRAMBytes, testVRAMBytes*2), set(testHostRAMBytes)), "windows that disagree on VRAM make no claim")
	assert.Nil(t, observedProviderEnvironment(set(testVRAMBytes), set(testHostRAMBytes, testHostRAMBytes*2)), "windows that disagree on RAM make no claim")
}

func capacitySample(offsetSeconds int64, vram, hostRAM uint64, availability evalv1.ProviderHardwareMetricAvailability) *evalv1.ProviderBoundaryHardwareSample {
	return &evalv1.ProviderBoundaryHardwareSample{
		ObservedAtUnixNanos:   uint64(time.Unix(1_700_000_000+offsetSeconds, 0).UnixNano()),
		VramBytesAvailability: availability,
		VramTotalBytes:        vram,
		HostRamAvailability:   availability,
		HostRamTotalBytes:     hostRAM,
	}
}

func saveObservationWindow(t *testing.T, fileSvc fs.RuntimeFileService, attemptID string, samples ...*evalv1.ProviderBoundaryHardwareSample) {
	t.Helper()
	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:              provider_observer.SchemaVersion,
		ProviderAttemptId:          attemptID,
		ObserverId:                 "observer-test",
		ObserverClockSource:        provider_observer.DefaultObserverClockSource,
		WindowStartedAtUnixNanos:   uint64(time.Unix(1_700_000_000, 0).UnixNano()),
		WindowCompletedAtUnixNanos: uint64(time.Unix(1_700_000_010, 0).UnixNano()),
		AttemptStartedAtUnixMs:     time.Unix(1_700_000_000, 0).UnixMilli(),
		AttemptCompletedAtUnixMs:   time.Unix(1_700_000_010, 0).UnixMilli(),
		Samples:                    samples,
	}
	digest, err := provider_observer.ComputeObservationDigest(window)
	require.NoError(t, err)
	window.ObservationDigest = digest
	windowStore, err := provider_observer.NewWindowStore(fileSvc)
	require.NoError(t, err)
	require.NoError(t, windowStore.Save(context.Background(), window))
}

func resultWithAttempts(attemptIDs ...string) *evalv1.EvaluationAssignmentResult {
	result := &evalv1.EvaluationAssignmentResult{}
	for _, attemptID := range attemptIDs {
		result.ModelInferences = append(result.ModelInferences, &evalv1.ModelInferenceRecord{
			InferenceRecordId: "inference-" + attemptID,
			ProviderAttemptId: attemptID,
		})
	}
	return result
}

const reported = evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED

func TestObservedProviderEnvironmentReadsRunWindows(t *testing.T) {
	ctx := context.Background()
	newReader := func(t *testing.T) (*CampaignProviderObservationReader, fs.RuntimeFileService) {
		fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
		reader, err := NewCampaignProviderObservationReader(fileSvc)
		require.NoError(t, err)
		return reader, fileSvc
	}

	t.Run("derives the environment from windows across assignments", func(t *testing.T) {
		reader, fileSvc := newReader(t)
		saveObservationWindow(t, fileSvc, "attempt-1", capacitySample(1, testVRAMBytes, testHostRAMBytes, reported), capacitySample(2, testVRAMBytes, testHostRAMBytes, reported))
		saveObservationWindow(t, fileSvc, "attempt-2", capacitySample(1, testVRAMBytes, testHostRAMBytes, reported))

		environment, err := reader.ObservedProviderEnvironment(ctx, map[string]*evalv1.EvaluationAssignmentResult{
			"assignment-1": resultWithAttempts("attempt-1"),
			"assignment-2": resultWithAttempts("attempt-2"),
		})
		require.NoError(t, err)
		assert.Equal(t, testProviderEnvironment(), environment)
	})

	t.Run("skips an attempt whose window was never captured", func(t *testing.T) {
		reader, fileSvc := newReader(t)
		saveObservationWindow(t, fileSvc, "attempt-1", capacitySample(1, testVRAMBytes, testHostRAMBytes, reported))

		environment, err := reader.ObservedProviderEnvironment(ctx, map[string]*evalv1.EvaluationAssignmentResult{
			"assignment-1": resultWithAttempts("attempt-1", "attempt-missing"),
		})
		require.NoError(t, err)
		assert.Equal(t, testProviderEnvironment(), environment)
	})

	t.Run("makes no claim without any window", func(t *testing.T) {
		reader, _ := newReader(t)
		environment, err := reader.ObservedProviderEnvironment(ctx, map[string]*evalv1.EvaluationAssignmentResult{
			"assignment-1": resultWithAttempts("attempt-missing"),
			"assignment-2": {},
		})
		require.NoError(t, err)
		assert.Nil(t, environment)
	})

	t.Run("ignores capacities the observer marked unavailable", func(t *testing.T) {
		reader, fileSvc := newReader(t)
		unavailable := evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE
		saveObservationWindow(t, fileSvc, "attempt-1", capacitySample(1, testVRAMBytes, testHostRAMBytes, unavailable))

		environment, err := reader.ObservedProviderEnvironment(ctx, map[string]*evalv1.EvaluationAssignmentResult{
			"assignment-1": resultWithAttempts("attempt-1"),
		})
		require.NoError(t, err)
		assert.Nil(t, environment)
	})

	t.Run("makes no claim when windows disagree about the hardware", func(t *testing.T) {
		reader, fileSvc := newReader(t)
		saveObservationWindow(t, fileSvc, "attempt-1", capacitySample(1, testVRAMBytes, testHostRAMBytes, reported))
		saveObservationWindow(t, fileSvc, "attempt-2", capacitySample(1, testVRAMBytes/2, testHostRAMBytes, reported))

		environment, err := reader.ObservedProviderEnvironment(ctx, map[string]*evalv1.EvaluationAssignmentResult{
			"assignment-1": resultWithAttempts("attempt-1"),
			"assignment-2": resultWithAttempts("attempt-2"),
		})
		require.NoError(t, err)
		assert.Nil(t, environment)
	})

	t.Run("requires a reader", func(t *testing.T) {
		_, err := (*CampaignProviderObservationReader)(nil).ObservedProviderEnvironment(ctx, nil)
		assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
	})
}

func TestBuildRunAggregateViewRecordsCarriesProviderEnvironment(t *testing.T) {
	assignments := []*evalv1.EvaluationAssignment{
		homogeneousAssignment("assign-1", "qwen3-4b", evalv1.ModelCampaignRole_MODEL_CAMPAIGN_ROLE_PRIMARY),
	}
	run := &evalv1.EvaluationRun{
		RunId:           "run-1",
		StartedAt:       timestamppb.New(time.Unix(1_700_000_000, 0).UTC()),
		CampaignBinding: &evalv1.ModelCampaignBinding{CampaignId: "eval-qwen"},
	}
	catalogOf := func(environment *ProviderEnvironment) map[string]json.RawMessage {
		state, err := CollectRunAggregateState(assignments, map[string]*evalv1.EvaluationAssignmentResult{})
		require.NoError(t, err)
		state.ProviderEnvironment = environment
		records, err := BuildRunAggregateViewRecords(run, state, nil, time.Unix(1_700_000_050, 0).UTC())
		require.NoError(t, err)
		for _, record := range records {
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(record.Body, &fields))
			if string(fields["kind"]) == `"catalog_snapshot"` {
				return fields
			}
		}
		require.Fail(t, "no catalog_snapshot record was projected")
		return nil
	}

	with := catalogOf(testProviderEnvironment())
	assert.JSONEq(t, `{"source":"observed","memory":"64 GiB","graphics":"24 GiB VRAM"}`, string(with["provider_environment"]))
	assert.NotContains(t, catalogOf(nil), "provider_environment", "an unobserved environment must be omitted, not emitted empty")
}

// attemptStubExecutor completes assignments like the shared stub but gives each
// scored inference a provider attempt ID, so observation windows can be bound.
type attemptStubExecutor struct{ stubCampaignExecutor }

func (e *attemptStubExecutor) ExecuteAssignment(ctx context.Context, req AssignmentExecutionRequest) (*evalv1.EvaluationAssignmentResult, error) {
	result, err := e.stubCampaignExecutor.ExecuteAssignment(ctx, req)
	if err != nil {
		return nil, err
	}
	for _, record := range result.GetModelInferences() {
		record.ProviderAttemptId = "attempt-" + result.GetAssignmentId()
	}
	result.ResultDigest = ""
	if result.ResultDigest, err = ComputeAssignmentResultDigest(result); err != nil {
		return nil, err
	}
	return result, nil
}

func TestPublishedCatalogCarriesTheObservedEnvironmentOnlyOnceTheRunIsComplete(t *testing.T) {
	ctx := context.Background()
	files := newCampaignMemoryFileService()
	store := NewStore(files)
	exporter := &recordingCampaignFeedExporter{}
	coordinator := NewCampaignPublicationCoordinator(store, files, NewMemoryCampaignPublicationStateStore(), exporter, nil)
	controller := NewCampaignController(store, &attemptStubExecutor{}, func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, func(prefix string) string { return prefix + "-1" }).WithPublication(coordinator)
	req := testCampaignInitRequest(t)
	truncated := &evalv1.EvaluationScenarioCatalog{
		SchemaVersion: req.Catalog.GetSchemaVersion(),
		CatalogRef:    req.Catalog.GetCatalogRef(),
		Scenarios:     req.Catalog.GetScenarios()[:3],
	}
	truncatedDigest, err := ComputeScenarioCatalogDigest(truncated)
	require.NoError(t, err)
	truncated.CatalogDigest = truncatedDigest
	req.Catalog = truncated
	run, err := controller.InitializeCampaign(ctx, req)
	require.NoError(t, err)
	_, err = controller.ScheduleHomogeneousRun(ctx, run.GetRunId())
	require.NoError(t, err)
	assignments, err := store.ListAssignments(ctx, run.GetRunId())
	require.NoError(t, err)
	for _, assignment := range assignments {
		saveObservationWindow(t, files, "attempt-"+assignment.GetAssignmentId(), capacitySample(1, testVRAMBytes, testHostRAMBytes, reported))
	}
	binding := CampaignExecutionBinding{
		InferenceOperatorSessionID: "inf-session",
		DataOperatorID:             "data-op",
		DataOperatorSessionID:      "data-session",
		ModelRegistryDigest:        req.Inventory.RegistryDigest,
		ModelRegistry:              InferenceVariantsFromEvalRegistry(req.Inventory.Variants),
	}
	catalogs := func() []map[string]json.RawMessage {
		var out []map[string]json.RawMessage
		for _, record := range exporter.records {
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(record.RecordBytes), &fields))
			if string(fields["kind"]) == `"catalog_snapshot"` {
				out = append(out, fields)
			}
		}
		return out
	}

	_, _, err = controller.ExecuteNextAssignment(ctx, run.GetRunId(), binding, req.ScenarioArtifacts)
	require.NoError(t, err)
	live := catalogs()
	require.NotEmpty(t, live, "a live aggregate publishes a catalog snapshot")
	for _, catalog := range live {
		assert.NotContains(t, catalog, "provider_environment", "a run still in progress claims no environment")
	}

	for {
		summary, err := controller.RunSummary(ctx, run.GetRunId())
		require.NoError(t, err)
		if summary.QueuedCount == 0 && summary.RunningCount == 0 {
			break
		}
		_, _, err = controller.ExecuteNextAssignment(ctx, run.GetRunId(), binding, req.ScenarioArtifacts)
		require.NoError(t, err)
	}
	_, err = coordinator.PublishRunCompletion(ctx, run.GetRunId(), time.Unix(1_700_000_100, 0).UTC())
	require.NoError(t, err)

	all := catalogs()
	last := all[len(all)-1]
	assert.JSONEq(t, `{"source":"observed","memory":"64 GiB","graphics":"24 GiB VRAM"}`, string(last["provider_environment"]), "the finished run's catalog carries the observed environment")
}
