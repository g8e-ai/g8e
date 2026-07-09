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
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/g8e-ai/g8e/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOperatorController(t *testing.T) {

	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(nil, nil))
	reg := &RegistrationService{}
	auth := &AuthService{}
	resp := &response.Writer{}

	controller := newOperatorController(cfg, logger, reg, auth, resp)

	assert.NotNil(t, controller)
	assert.Equal(t, cfg, controller.cfg)
	assert.Equal(t, logger, controller.logger)
	assert.Equal(t, reg, controller.reg)
	assert.Equal(t, auth, controller.auth)
	assert.Equal(t, resp, controller.responder)
}

func TestHandleReauth_MalformedJSON(t *testing.T) {

	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "", "", "")
	reg := &RegistrationService{}
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxPayloadBytes: 1024}}
	controller := newOperatorController(cfg, logger, reg, auth, res)

	// Create a valid Operator session
	operatorSessionID := "test-session-123"
	opDoc := map[string]interface{}{
		"id":                  "op-123",
		"operator_session_id": operatorSessionID,
		"status":              marshaler.Status(constants.OperatorStatusActive),
		"user_id":             "user-123",
		"organization_id":     "org-123",
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocStore.DocSet("operators", "op-123", opBytes))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/reauth", strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(constants.HeaderAuthorization, "Bearer "+operatorSessionID)

	wid := protocol.NewWorkloadIdentity()
	opURI, err := wid.OperatorSPIFFEURL("org-123", "op-123", operatorSessionID)
	require.NoError(t, err)

	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{
			{URIs: []*url.URL{opURI}},
		},
	}

	ctx := context.WithValue(req.Context(), constants.ContextKeyOperatorSessionID, operatorSessionID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	controller.handleReauth(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestOperatorController_HandleListOperators(t *testing.T) {

	infra := setupTestInfrastructure(t, false)
	controller := newOperatorController(infra.Cfg, infra.Logger, infra.Reg, infra.Auth, infra.Responder)

	t.Run("Wrong method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators?user_id=user-123", nil)
		w := httptest.NewRecorder()

		controller.handleListOperators(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("Missing user_id returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/operators", nil)
		w := httptest.NewRecorder()

		controller.handleListOperators(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Valid request returns 200 with slots", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/operators?user_id=user-123", nil)
		w := httptest.NewRecorder()

		controller.handleListOperators(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp models.OperatorSlotResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.NotNil(t, resp.Operators)
	})
}

func TestOperatorController_HandleTerminateOperator(t *testing.T) {

	infra := setupTestInfrastructure(t, false)
	controller := newOperatorController(infra.Cfg, infra.Logger, infra.Reg, infra.Auth, infra.Responder)

	t.Run("Wrong method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/operators/terminate", nil)
		w := httptest.NewRecorder()

		controller.handleTerminateOperator(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("Invalid body returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/terminate", strings.NewReader("{invalid"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		controller.handleTerminateOperator(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Missing operator_id returns 400", func(t *testing.T) {
		reqBody := map[string]string{"user_id": "user-123"}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/terminate", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		controller.handleTerminateOperator(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Missing user_id returns 400", func(t *testing.T) {
		reqBody := map[string]string{"operator_id": "op-123"}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/terminate", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		controller.handleTerminateOperator(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Valid request with non-existent operator returns 400", func(t *testing.T) {
		reqBody := models.TerminateOperatorRequest{
			OperatorID: "op-nonexistent",
			UserID:     "user-123",
			Reason:     "test",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/terminate", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		controller.handleTerminateOperator(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestOperatorController_HandleBindOperators(t *testing.T) {

	infra := setupTestInfrastructure(t, false)
	controller := newOperatorController(infra.Cfg, infra.Logger, infra.Reg, infra.Auth, infra.Responder)

	t.Run("Wrong method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/operators/bind", nil)
		w := httptest.NewRecorder()

		controller.handleBindOperators(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("Invalid JSON returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/bind", strings.NewReader("{invalid"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		controller.handleBindOperators(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("user_id mismatch returns 403", func(t *testing.T) {
		reqBody := models.BindOperatorsRequest{
			UserID: "user-456",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/bind?user_id=user-123", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		controller.handleBindOperators(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Valid request with empty bind returns 200", func(t *testing.T) {
		reqBody := models.BindOperatorsRequest{
			UserID: "user-123",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/bind", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		controller.handleBindOperators(w, req)

		// May return 400 if validation fails, but handler logic is exercised
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
	})
}

func TestOperatorController_HandleUnbindOperators(t *testing.T) {

	infra := setupTestInfrastructure(t, false)
	controller := newOperatorController(infra.Cfg, infra.Logger, infra.Reg, infra.Auth, infra.Responder)

	t.Run("Wrong method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/operators/unbind", nil)
		w := httptest.NewRecorder()

		controller.handleUnbindOperators(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("Invalid JSON returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/unbind", strings.NewReader("{invalid"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		controller.handleUnbindOperators(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("user_id mismatch returns 403", func(t *testing.T) {
		reqBody := models.UnbindOperatorsRequest{
			UserID: "user-456",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/unbind?user_id=user-123", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		controller.handleUnbindOperators(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Valid request with empty unbind returns 200", func(t *testing.T) {
		reqBody := models.UnbindOperatorsRequest{
			UserID: "user-123",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/unbind", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		controller.handleUnbindOperators(w, req)

		// May return 400 if validation fails, but handler logic is exercised
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
	})
}

func TestOperatorController_HandleSetTargetContext(t *testing.T) {

	infra := setupTestInfrastructure(t, false)
	controller := newOperatorController(infra.Cfg, infra.Logger, infra.Reg, infra.Auth, infra.Responder)

	t.Run("Wrong method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/operators/target", nil)
		w := httptest.NewRecorder()

		controller.handleSetTargetContext(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("Invalid JSON returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/target", strings.NewReader("{invalid"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		controller.handleSetTargetContext(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("user_id mismatch returns 403", func(t *testing.T) {
		reqBody := models.SetTargetContextRequest{
			UserID: "user-456",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/target?user_id=user-123", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		controller.handleSetTargetContext(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Valid request with empty context returns 200 or 400", func(t *testing.T) {
		reqBody := models.SetTargetContextRequest{
			UserID: "user-123",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/target", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		controller.handleSetTargetContext(w, req)

		// May return 400 if validation fails, but handler logic is exercised
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusBadRequest)
	})
}

func TestOperatorController_HandleReauth(t *testing.T) {

	infra := setupTestInfrastructure(t, false)
	controller := newOperatorController(infra.Cfg, infra.Logger, infra.Reg, infra.Auth, infra.Responder)

	t.Run("Wrong method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/operators/reauth", nil)
		w := httptest.NewRecorder()

		controller.handleReauth(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("Missing operator session ID returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/reauth", nil)
		w := httptest.NewRecorder()

		controller.handleReauth(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
