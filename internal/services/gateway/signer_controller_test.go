// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build integration

package gateway

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestSignerController(t *testing.T) (*SignerController, *Stores) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)

	signerController := newSignerController(SignerControllerDeps{
		Cfg:         infra.Cfg,
		Logger:      infra.Logger,
		DocStore:    infra.Stores.DocStore,
		SignerStore: infra.Stores.SignerStore,
		Responder:   infra.Responder,
	})

	return signerController, infra.Stores
}

func TestNewSignerController_AllDepsProvidedNoNilFields(t *testing.T) {
	infra := setupTestInfrastructure(t, false)

	controller := newSignerController(SignerControllerDeps{
		Cfg:         infra.Cfg,
		Logger:      infra.Logger,
		DocStore:    infra.Stores.DocStore,
		SignerStore: infra.Stores.SignerStore,
		Responder:   infra.Responder,
	})

	assert.NotNil(t, controller.cfg)
	assert.NotNil(t, controller.logger)
	assert.NotNil(t, controller.docStore)
	assert.NotNil(t, controller.signerStore)
	assert.NotNil(t, controller.responder)

	assert.Equal(t, infra.Cfg, controller.cfg)
	assert.Equal(t, infra.Logger, controller.logger)
	assert.Equal(t, infra.Stores.DocStore, controller.docStore)
	assert.Equal(t, infra.Stores.SignerStore, controller.signerStore)
	assert.Equal(t, infra.Responder, controller.responder)
}

func TestSignerControllerHandleGovernanceSigners(t *testing.T) {
	signerController, stores := setupTestSignerController(t)

	t.Run("GET - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/governance/signers", nil)
		rr := httptest.NewRecorder()
		signerController.handleGovernanceSigners(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"success":true`)
	})

	t.Run("POST - success", func(t *testing.T) {
		signer := models.TrustedSigner{
			ID:        "test-signer-1",
			PublicKey: strings.Repeat("a", 64),
			AddedAt:   time.Now().UTC(),
			Enabled:   true,
		}
		body := mustMarshalJSON(t, signer)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/governance/signers", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		signerController.handleGovernanceSigners(rr, req)
		assert.Equal(t, http.StatusCreated, rr.Code)
		t.Cleanup(func() { stores.DocStore.DocDelete("trusted_signers", "test-signer-1") })
	})

	t.Run("POST - missing id", func(t *testing.T) {
		signer := models.TrustedSigner{
			PublicKey: strings.Repeat("a", 64),
		}
		body := mustMarshalJSON(t, signer)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/governance/signers", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		signerController.handleGovernanceSigners(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "missing required field")
	})

	t.Run("POST - missing public_key", func(t *testing.T) {
		signer := models.TrustedSigner{
			ID: "test-signer-2",
		}
		body := mustMarshalJSON(t, signer)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/governance/signers", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		signerController.handleGovernanceSigners(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "missing required field")
	})

	t.Run("POST - invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/governance/signers", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()
		signerController.handleGovernanceSigners(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/governance/signers", nil)
		rr := httptest.NewRecorder()
		signerController.handleGovernanceSigners(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestSignerControllerHandleGovernanceSignerByID(t *testing.T) {
	signerController, stores := setupTestSignerController(t)

	t.Run("GET - not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/governance/signers/nonexistent", nil)
		rr := httptest.NewRecorder()
		signerController.handleGovernanceSignerByID(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("GET - success", func(t *testing.T) {
		signer := models.TrustedSigner{
			ID:        "test-signer-get",
			PublicKey: strings.Repeat("b", 64),
			AddedAt:   time.Now().UTC(),
			Enabled:   true,
		}
		signerBytes := mustMarshalJSON(t, signer)
		err := stores.DocStore.DocSet("trusted_signers", "test-signer-get", signerBytes)
		require.NoError(t, err)
		t.Cleanup(func() { stores.DocStore.DocDelete("trusted_signers", "test-signer-get") })

		req := httptest.NewRequest(http.MethodGet, "/api/v1/governance/signers/test-signer-get", nil)
		rr := httptest.NewRecorder()
		signerController.handleGovernanceSignerByID(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "test-signer-get")
	})

	t.Run("DELETE - success", func(t *testing.T) {
		signer := models.TrustedSigner{
			ID:        "test-signer-delete",
			PublicKey: strings.Repeat("c", 64),
			AddedAt:   time.Now().UTC(),
			Enabled:   true,
		}
		signerBytes := mustMarshalJSON(t, signer)
		err := stores.DocStore.DocSet("trusted_signers", "test-signer-delete", signerBytes)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/governance/signers/test-signer-delete", nil)
		rr := httptest.NewRecorder()
		signerController.handleGovernanceSignerByID(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("DELETE - not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/governance/signers/nonexistent", nil)
		rr := httptest.NewRecorder()
		signerController.handleGovernanceSignerByID(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Invalid signer id - empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/governance/signers/", nil)
		rr := httptest.NewRecorder()
		signerController.handleGovernanceSignerByID(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Invalid signer id - contains slash", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/governance/signers/invalid/id", nil)
		rr := httptest.NewRecorder()
		signerController.handleGovernanceSignerByID(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/governance/signers/test-id", nil)
		rr := httptest.NewRecorder()
		signerController.handleGovernanceSignerByID(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}
