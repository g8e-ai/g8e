// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestProviderObservationControllerHandleProviderObservation_RejectsNonGet(t *testing.T) {
	logger := testutil.NewTestLogger()
	controller := newProviderObservationController(ProviderObservationControllerDeps{
		Logger:    logger,
		Responder: response.NewWriter(logger),
	})

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.InferenceProviderObservations+"attempt-1", nil)
	rr := httptest.NewRecorder()
	controller.handleProviderObservation(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestProviderObservationControllerHandleProviderObservation_NotFound(t *testing.T) {
	logger := testutil.NewTestLogger()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	windows, err := provider_observer.NewWindowStore(fileSvc)
	require.NoError(t, err)
	attempts, err := inference.NewAttemptStore(fileSvc)
	require.NoError(t, err)

	controller := newProviderObservationController(ProviderObservationControllerDeps{
		Logger:    logger,
		Responder: response.NewWriter(logger),
		Windows:   windows,
		Attempts:  attempts,
	})

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.InferenceProviderObservations+"missing-attempt", nil)
	rr := httptest.NewRecorder()
	controller.handleProviderObservation(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestProviderObservationControllerHandleProviderObservation_SynthesizesAttemptFromWindow(t *testing.T) {
	ctx := context.Background()
	logger := testutil.NewTestLogger()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	windows, err := provider_observer.NewWindowStore(fileSvc)
	require.NoError(t, err)
	attempts, err := inference.NewAttemptStore(fileSvc)
	require.NoError(t, err)

	started := time.Unix(1_700_000_000, 0).UnixMilli()
	completed := time.Unix(1_700_000_010, 0).UnixMilli()
	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:              provider_observer.SchemaVersion,
		ProviderAttemptId:          "attempt-window-only",
		ObserverId:                 "observer-test",
		ObserverClockSource:        provider_observer.DefaultObserverClockSource,
		WindowStartedAtUnixNanos:   uint64(time.Unix(1_700_000_000, 0).UnixNano()),
		WindowCompletedAtUnixNanos: uint64(time.Unix(1_700_000_010, 0).UnixNano()),
		AttemptStartedAtUnixMs:     started,
		AttemptCompletedAtUnixMs:   completed,
		Samples: []*evalv1.ProviderBoundaryHardwareSample{{
			ObservedAtUnixNanos: uint64(time.Unix(1_700_000_001, 0).UnixNano()),
			HostRamAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			HostRamUsedBytes:    1,
			HostRamTotalBytes:   2,
		}},
	}
	digest, err := provider_observer.ComputeObservationDigest(window)
	require.NoError(t, err)
	window.ObservationDigest = digest
	require.NoError(t, windows.Save(ctx, window))

	controller := newProviderObservationController(ProviderObservationControllerDeps{
		Logger:    logger,
		Responder: response.NewWriter(logger),
		Windows:   windows,
		Attempts:  attempts,
	})

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.InferenceProviderObservations+"attempt-window-only", nil)
	rr := httptest.NewRecorder()
	controller.handleProviderObservation(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp models.ProviderObservationResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	loadedAttempt := &operatorv1.InferenceProviderAttemptRecord{}
	require.NoError(t, protojson.Unmarshal(resp.ProviderAttempt, loadedAttempt))
	assert.Equal(t, "attempt-window-only", loadedAttempt.GetProviderAttemptId())
	assert.Equal(t, started, loadedAttempt.GetStartedAtUnixMs())
	assert.Equal(t, completed, loadedAttempt.GetCompletedAtUnixMs())
}

func TestProviderObservationControllerHandleProviderObservation_ReturnsBundle(t *testing.T) {
	ctx := context.Background()
	logger := testutil.NewTestLogger()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	windows, err := provider_observer.NewWindowStore(fileSvc)
	require.NoError(t, err)
	attempts, err := inference.NewAttemptStore(fileSvc)
	require.NoError(t, err)

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
		Samples: []*evalv1.ProviderBoundaryHardwareSample{{
			ObservedAtUnixNanos:        uint64(time.Unix(1_700_000_001, 0).UnixNano()),
			VramBytesAvailability:      evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			VramUsedBytes:              1000,
			GpuUtilizationAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			GpuUtilizationPercent:      42,
		}},
	}
	digest, err := provider_observer.ComputeObservationDigest(window)
	require.NoError(t, err)
	window.ObservationDigest = digest
	require.NoError(t, windows.Save(ctx, window))

	controller := newProviderObservationController(ProviderObservationControllerDeps{
		Logger:    logger,
		Responder: response.NewWriter(logger),
		Windows:   windows,
		Attempts:  attempts,
	})

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.InferenceProviderObservations+"attempt-1", nil)
	rr := httptest.NewRecorder()
	controller.handleProviderObservation(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp models.ProviderObservationResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Window)
	require.NotEmpty(t, resp.ProviderAttempt)

	loadedWindow := &evalv1.ProviderBoundaryObservationWindow{}
	require.NoError(t, evalv1.UnmarshalCanonical(resp.Window, loadedWindow))
	assert.Equal(t, "attempt-1", loadedWindow.GetProviderAttemptId())

	loadedAttempt := &operatorv1.InferenceProviderAttemptRecord{}
	require.NoError(t, protojson.Unmarshal(resp.ProviderAttempt, loadedAttempt))
	assert.Equal(t, "attempt-1", loadedAttempt.GetProviderAttemptId())
}

type stubProviderObservationPreflight struct {
	err error
}

func (s stubProviderObservationPreflight) PreflightCommandDelivery(context.Context) error {
	return s.err
}

func TestProviderObservationControllerHandleProviderObservationPreflight_Ready(t *testing.T) {
	logger := testutil.NewTestLogger()
	controller := newProviderObservationController(ProviderObservationControllerDeps{
		Logger:                 logger,
		Responder:              response.NewWriter(logger),
		ObservationCoordinator: stubProviderObservationPreflight{},
	})

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.InferenceProviderObservations+"_preflight", nil)
	rr := httptest.NewRecorder()
	controller.handleProviderObservation(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "ready", resp["status"])
}

func TestProviderObservationControllerHandleProviderObservationPreflight_Unavailable(t *testing.T) {
	logger := testutil.NewTestLogger()
	controller := newProviderObservationController(ProviderObservationControllerDeps{
		Logger:    logger,
		Responder: response.NewWriter(logger),
		ObservationCoordinator: stubProviderObservationPreflight{
			err: constants.ErrEvaluationObservationUnavailable,
		},
	})

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.InferenceProviderObservations+"_preflight", nil)
	rr := httptest.NewRecorder()
	controller.handleProviderObservation(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
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
