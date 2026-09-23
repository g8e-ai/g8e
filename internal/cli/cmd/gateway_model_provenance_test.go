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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/model_provenance"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func testModelProvenanceWindow(attemptID string) (*evalv1.ModelProvenanceAttestationWindow, []byte) {
	digest := strings.Repeat("a", 64)
	window := &evalv1.ModelProvenanceAttestationWindow{
		SchemaVersion:              model_provenance.SchemaVersion,
		ProviderAttemptId:          attemptID,
		ProvenanceOperatorId:       "test-provenance-operator",
		ServedModelTag:             "probe-model:7b",
		ExpectedModelDigest:        digest,
		ObservedModelDigest:        digest,
		ManifestDigest:             strings.Repeat("b", 64),
		ManifestVerificationStatus: evalv1.ModelManifestVerificationStatus_MODEL_MANIFEST_VERIFICATION_STATUS_UNSIGNED,
		AttestedAtUnixMs:           1_700_000_000_000,
		DigestMatch:                true,
	}
	attestationDigest, err := model_provenance.ComputeAttestationDigest(window)
	if err != nil {
		panic(err)
	}
	window.AttestationDigest = attestationDigest

	windowBody, err := evalv1.MarshalCanonical(window)
	if err != nil {
		panic(err)
	}
	respBody, err := json.Marshal(models.ModelProvenanceResponse{Window: windowBody})
	if err != nil {
		panic(err)
	}
	return window, respBody
}

func TestRemoteModelProvenanceClient_Load_RejectsMissingInputs(t *testing.T) {
	t.Parallel()
	_, err := (&remoteModelProvenanceClient{}).Load(context.Background(), "attempt-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)

	client := &remoteModelProvenanceClient{client: &mockAPIClient{}}
	_, err = client.Load(context.Background(), "")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestRemoteModelProvenanceClient_Load_ReturnsGatewayWindow(t *testing.T) {
	t.Parallel()
	window, respBody := testModelProvenanceWindow("attempt-1")
	apiClient := &mockAPIClient{getResp: respBody}
	remote := &remoteModelProvenanceClient{client: apiClient}

	loaded, err := remote.Load(context.Background(), "attempt-1")
	require.NoError(t, err)
	assert.Equal(t, window.GetProviderAttemptId(), loaded.GetProviderAttemptId())
	assert.Equal(t, []string{constants.APIPaths.InferenceModelProvenanceAttestations + "attempt-1"}, apiClient.getCalls)

	cached, err := remote.Load(context.Background(), "attempt-1")
	require.NoError(t, err)
	assert.Equal(t, loaded.GetAttestationDigest(), cached.GetAttestationDigest())
	assert.Len(t, apiClient.getCalls, 1, "second load should use cache")
}

func TestRemoteModelProvenanceClient_Load_PropagatesGatewayErrors(t *testing.T) {
	t.Parallel()
	apiClient := &mockAPIClient{getErr: fmt.Errorf("network down")}
	remote := &remoteModelProvenanceClient{client: apiClient}

	_, err := remote.Load(context.Background(), "attempt-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gateway read")
}

func TestRemoteModelProvenanceClient_Load_RejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	apiClient := &mockAPIClient{getResp: []byte("not-json")}
	remote := &remoteModelProvenanceClient{client: apiClient}

	_, err := remote.Load(context.Background(), "attempt-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}

func TestNewModelProvenanceRemote_ReturnsNilWhenGatewayUnhealthy(t *testing.T) {
	withGatewayHealthCheck(t, false)
	fileSvc, cfg := newCmdTestEnv(t)

	remote, err := newModelProvenanceRemote(fileSvc, cfg)
	require.NoError(t, err)
	assert.Nil(t, remote)
}

func TestNewModelProvenanceRemote_ReturnsClientWhenGatewayHealthy(t *testing.T) {
	withGatewayHealthCheck(t, true)
	cfg, _, fileSvc := setupApproveSSETestEnv(t)

	remote, err := newModelProvenanceRemote(fileSvc, cfg)
	require.NoError(t, err)
	require.NotNil(t, remote)
}

func TestNewCampaignModelProvenanceReader_WiresRemoteWhenHealthy(t *testing.T) {
	withGatewayHealthCheck(t, true)
	cfg, _, fileSvc := setupApproveSSETestEnv(t)

	reader, err := newCampaignModelProvenanceReader(fileSvc, cfg)
	require.NoError(t, err)
	require.NotNil(t, reader)
}
