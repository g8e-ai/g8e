// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func withGatewayHealthCheck(t *testing.T, healthy bool) {
	original := gatewayHealthCheck
	gatewayHealthCheck = func() bool { return healthy }
	t.Cleanup(func() { gatewayHealthCheck = original })
}

func testProviderObservationBundle(attemptID string) (*evalv1.ProviderBoundaryObservationWindow, *operatorv1.InferenceProviderAttemptRecord, []byte) {
	attempt := &operatorv1.InferenceProviderAttemptRecord{
		ProviderAttemptId: attemptID,
		Status:            operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_COMPLETED,
		StartedAtUnixMs:   time.Unix(1_700_000_000, 0).UnixMilli(),
		CompletedAtUnixMs: time.Unix(1_700_000_010, 0).UnixMilli(),
	}
	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:              provider_observer.SchemaVersion,
		ProviderAttemptId:          attemptID,
		ObserverId:                 "observer-test",
		ObserverClockSource:        provider_observer.DefaultObserverClockSource,
		WindowStartedAtUnixNanos:   uint64(time.Unix(1_700_000_000, 0).UnixNano()),
		WindowCompletedAtUnixNanos: uint64(time.Unix(1_700_000_010, 0).UnixNano()),
		AttemptStartedAtUnixMs:     attempt.GetStartedAtUnixMs(),
		AttemptCompletedAtUnixMs:   attempt.GetCompletedAtUnixMs(),
	}
	digest, err := provider_observer.ComputeObservationDigest(window)
	if err != nil {
		panic(err)
	}
	window.ObservationDigest = digest

	windowBody, err := evalv1.MarshalCanonical(window)
	if err != nil {
		panic(err)
	}
	attemptBody, err := protojson.Marshal(attempt)
	if err != nil {
		panic(err)
	}
	respBody, err := json.Marshal(models.ProviderObservationResponse{
		Window:          windowBody,
		ProviderAttempt: attemptBody,
	})
	if err != nil {
		panic(err)
	}
	return window, attempt, respBody
}

func TestRemoteProviderObservationClient_Load_RejectsMissingInputs(t *testing.T) {
	t.Parallel()
	_, _, err := (&remoteProviderObservationClient{}).Load(context.Background(), "attempt-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)

	client := &remoteProviderObservationClient{client: &mockAPIClient{}}
	_, _, err = client.Load(context.Background(), "")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestRemoteProviderObservationClient_Load_ReturnsGatewayBundle(t *testing.T) {
	t.Parallel()
	window, attempt, respBody := testProviderObservationBundle("attempt-1")
	apiClient := &mockAPIClient{getResp: respBody}
	remote := &remoteProviderObservationClient{client: apiClient}

	loadedWindow, loadedAttempt, err := remote.Load(context.Background(), "attempt-1")
	require.NoError(t, err)
	assert.Equal(t, window.GetProviderAttemptId(), loadedWindow.GetProviderAttemptId())
	assert.Equal(t, attempt.GetProviderAttemptId(), loadedAttempt.GetProviderAttemptId())
	assert.Equal(t, []string{constants.APIPaths.InferenceProviderObservations + "attempt-1"}, apiClient.getCalls)

	cachedWindow, cachedAttempt, err := remote.Load(context.Background(), "attempt-1")
	require.NoError(t, err)
	assert.Equal(t, loadedWindow.GetObservationDigest(), cachedWindow.GetObservationDigest())
	assert.Equal(t, loadedAttempt.GetProviderAttemptId(), cachedAttempt.GetProviderAttemptId())
	assert.Len(t, apiClient.getCalls, 1, "second load should use cache")
}

func TestRemoteProviderObservationClient_Load_PropagatesGatewayErrors(t *testing.T) {
	t.Parallel()
	apiClient := &mockAPIClient{getErr: fmt.Errorf("network down")}
	remote := &remoteProviderObservationClient{client: apiClient}

	_, _, err := remote.Load(context.Background(), "attempt-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gateway read")
}

func TestRemoteProviderObservationClient_Load_RejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	apiClient := &mockAPIClient{getResp: []byte("not-json")}
	remote := &remoteProviderObservationClient{client: apiClient}

	_, _, err := remote.Load(context.Background(), "attempt-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}

func TestNewProviderObservationRemote_ReturnsNilWhenGatewayUnhealthy(t *testing.T) {
	withGatewayHealthCheck(t, false)
	fileSvc, cfg := newCmdTestEnv(t)

	remote, err := newProviderObservationRemote(fileSvc, cfg)
	require.NoError(t, err)
	assert.Nil(t, remote)
}

func TestNewProviderObservationRemote_ReturnsClientWhenGatewayHealthy(t *testing.T) {
	withGatewayHealthCheck(t, true)
	cfg, _, fileSvc := setupApproveSSETestEnv(t)

	remote, err := newProviderObservationRemote(fileSvc, cfg)
	require.NoError(t, err)
	require.NotNil(t, remote)
}

func TestPreflightProviderObservationDelivery_RejectsUnhealthyGateway(t *testing.T) {
	withGatewayHealthCheck(t, false)
	fileSvc, cfg := newCmdTestEnv(t)

	err := preflightProviderObservationDelivery(fileSvc, cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationObservationUnavailable)
}

func TestPreflightProviderObservationDelivery_RequiresReadyStatus(t *testing.T) {
	withGatewayHealthCheck(t, true)
	cfg, _, fileSvc := setupApproveSSETestEnv(t)
	cfg.Paths = &config.PathsConfig{Host: "http://127.0.0.1:1"}

	err := preflightProviderObservationDelivery(fileSvc, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider observation preflight")
}

func TestNewCampaignProviderObservationReader_WiresRemoteWhenHealthy(t *testing.T) {
	withGatewayHealthCheck(t, true)
	cfg, _, fileSvc := setupApproveSSETestEnv(t)

	reader, err := newCampaignProviderObservationReader(fileSvc, cfg)
	require.NoError(t, err)
	require.NotNil(t, reader)
}
