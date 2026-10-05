// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// TestModelProvenanceControllerAttestPreflight_OutlivesServerWriteTimeout
// reproduces the campaign preflight failure: an attestation that finishes
// after the server WriteTimeout was cut mid-response and the TLS client saw
// "tls: bad record MAC". The handler clears the write deadline because the
// probe bounds its own wait.
func TestModelProvenanceControllerAttestPreflight_OutlivesServerWriteTimeout(t *testing.T) {
	logger := testutil.NewTestLogger()
	controller := newModelProvenanceController(ModelProvenanceControllerDeps{
		Logger:                logger,
		Responder:             response.NewWriter(logger),
		ProvenanceCoordinator: slowModelProvenancePreflight{delay: 300 * time.Millisecond},
	})
	srv := httptest.NewUnstartedServer(http.HandlerFunc(controller.handleModelProvenance))
	srv.Config.WriteTimeout = 100 * time.Millisecond
	srv.StartTLS()
	t.Cleanup(srv.Close)

	url := srv.URL + constants.APIPaths.InferenceModelProvenanceAttestations + "_attest?served_model_tag=probe-model%3A7b&expected_model_digest=" + strings.Repeat("a", 64)
	resp, err := srv.Client().Get(url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body models.ModelProvenanceAttestResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ready", body.Status)
}

// slowModelProvenancePreflight attests only after delay, like a Provenance
// Operator hashing a multi-gigabyte model.
type slowModelProvenancePreflight struct {
	stubModelProvenancePreflight
	delay time.Duration
}

func (s slowModelProvenancePreflight) PreflightStorageAttestation(ctx context.Context, tag, digest string) (*evalv1.ModelProvenanceAttestationWindow, error) {
	time.Sleep(s.delay)
	return s.stubModelProvenancePreflight.PreflightStorageAttestation(ctx, tag, digest)
}
