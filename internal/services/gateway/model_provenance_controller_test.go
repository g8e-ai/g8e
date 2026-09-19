// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/model_provenance"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestModelProvenanceControllerHandleModelProvenance_ReturnsWindow(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	store, err := model_provenance.NewWindowStore(fileSvc)
	require.NoError(t, err)
	digest := strings.Repeat("a", 64)
	window := &evalv1.ModelProvenanceAttestationWindow{
		SchemaVersion:              model_provenance.SchemaVersion,
		ProviderAttemptId:          "attempt-1",
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
	require.NoError(t, err)
	window.AttestationDigest = attestationDigest
	require.NoError(t, store.Save(ctx, window))

	logger := testutil.NewTestLogger()
	controller := newModelProvenanceController(ModelProvenanceControllerDeps{
		Logger:    logger,
		Responder: response.NewWriter(logger),
		Windows:   store,
	})
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.InferenceModelProvenanceAttestations+"attempt-1", nil)
	rr := httptest.NewRecorder()
	controller.handleModelProvenance(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resp models.ModelProvenanceResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	loaded := &evalv1.ModelProvenanceAttestationWindow{}
	require.NoError(t, evalv1.UnmarshalCanonical(resp.Window, loaded))
	assert.Equal(t, "attempt-1", loaded.GetProviderAttemptId())
}
