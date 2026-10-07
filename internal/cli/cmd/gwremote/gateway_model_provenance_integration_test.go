// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gwremote

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestLoadModelProvenanceAttestation_ReturnsGatewayWindow(t *testing.T) {
	WithGatewayHealthCheck(t, true)
	cfg, _, fileSvc := cmdtest.SetupApproveSSETestEnv(t)
	window, _ := testModelProvenanceWindow("preflight-provenance")
	windowBody, err := evalv1.MarshalCanonical(window)
	require.NoError(t, err)
	respBody, err := json.Marshal(models.ModelProvenanceAttestResponse{
		Status:              "ready",
		ServedModelTag:      window.GetServedModelTag(),
		ExpectedModelDigest: window.GetExpectedModelDigest(),
		Window:              windowBody,
	})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, constants.APIPaths.InferenceModelProvenanceAttestations+"_attest") {
			w.Header().Set("Content-Type", "application/json")
			_, writeErr := w.Write(respBody)
			require.NoError(t, writeErr)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	cmdtest.WithEndpointOverride(t, server.URL)

	loaded, err := LoadModelProvenanceAttestation(fileSvc, cfg, window.GetServedModelTag(), window.GetExpectedModelDigest())
	require.NoError(t, err)
	assert.Equal(t, window.GetAttestationDigest(), loaded.GetAttestationDigest())
}
