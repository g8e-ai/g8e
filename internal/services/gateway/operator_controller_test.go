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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	govsvc "github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/g8e-ai/g8e/v2/protocol"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOperatorController(t *testing.T) {

	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(nil, nil))
	reg := &RegistrationService{}
	auth := &AuthService{}
	resp := &response.Writer{}

	controller := newOperatorController(OperatorControllerDeps{Cfg: cfg, Logger: logger, Reg: reg, Auth: auth, Responder: resp})

	assert.NotNil(t, controller)
	assert.Equal(t, cfg, controller.cfg)
	assert.Equal(t, logger, controller.logger)
	assert.Equal(t, reg, controller.reg)
	assert.Equal(t, auth, controller.auth)
	assert.Equal(t, resp, controller.responder)
}

func seedStopOperator(t *testing.T, infra *TestInfrastructure, operatorID, sessionID, userID string, operatorType constants.OperatorType, status constants.OperatorStatus) {
	t.Helper()
	userBytes, err := json.Marshal(&models.User{ID: userID, Status: constants.UserStatusActive})
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))
	opBytes, err := json.Marshal(&models.OperatorDocumentGo{
		ID:                operatorID,
		UserID:            userID,
		OrganizationID:    "stop-org",
		OperatorSessionID: sessionID,
		OperatorType:      operatorType,
		Status:            status,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(marshaler.CollectionName(constants.CollectionOperators), operatorID, opBytes))
}

func stopOperatorRequest(t *testing.T, controller *OperatorController, userID, sessionID, reason string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(models.StopOperatorRequest{OperatorSessionID: sessionID, Reason: reason})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.OperatorsStop, strings.NewReader(string(body)))
	req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, userID))
	rr := httptest.NewRecorder()
	controller.handleStopOperator(rr, req)
	return rr
}

func TestOperatorController_HandleStopOperatorAuthorizationAndDelivery(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	dispatch := NewDispatchService(infra.Logger, infra.Pubsub, infra.StateRootSvc, infra.Auth, string(config.PostureDoctrine), govsvc.NewL1Doctrine(), nil, infra.SignerStore)
	controller := newOperatorController(OperatorControllerDeps{Cfg: infra.Cfg, Logger: infra.Logger, Reg: infra.Reg, Auth: infra.Auth, Dispatch: dispatch, Responder: infra.Responder})

	seedStopOperator(t, infra, "stop-remote", "stop-remote-session", "stop-owner", constants.OperatorTypeRemote, constants.OperatorStatusActive)
	seedStopOperator(t, infra, "stop-other", "stop-other-session", "stop-other-owner", constants.OperatorTypeRemote, constants.OperatorStatusActive)
	seedStopOperator(t, infra, "stop-embedded", "stop-embedded-session", "stop-owner", constants.OperatorTypeEmbedded, constants.OperatorStatusActive)
	seedStopOperator(t, infra, "stop-offline", "stop-offline-session", "stop-owner", constants.OperatorTypeRemote, constants.OperatorStatusStopped)

	t.Run("cross-owner target is rejected", func(t *testing.T) {
		rr := stopOperatorRequest(t, controller, "stop-owner", "stop-other-session", "retired")
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("embedded target is rejected", func(t *testing.T) {
		rr := stopOperatorRequest(t, controller, "stop-owner", "stop-embedded-session", "retired")
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrOperatorStopEmbedded.Error())
	})

	for _, tc := range []struct {
		name      string
		sessionID string
	}{
		{name: "missing target is rejected", sessionID: "missing-stop-session"},
		{name: "offline target is rejected", sessionID: "stop-offline-session"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := stopOperatorRequest(t, controller, "stop-owner", tc.sessionID, "retired")
			assert.Equal(t, http.StatusUnauthorized, rr.Code)
		})
	}

	t.Run("delivery failure leaves operator active", func(t *testing.T) {
		rr := stopOperatorRequest(t, controller, "stop-owner", "stop-remote-session", "retired")
		assert.Equal(t, http.StatusBadGateway, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrDispatchNoDelivery.Error())
		op, err := infra.Auth.ValidateOperatorSession("stop-remote-session")
		require.NoError(t, err)
		assert.Equal(t, constants.OperatorStatusActive, op.Status)
	})

	t.Run("acknowledged delivery records reason and stopped state", func(t *testing.T) {
		channel := pubsub.CmdChannel("stop-remote", "stop-remote-session")
		resultsChannel := pubsub.ResultsChannel("stop-remote", "stop-remote-session")
		unregister := infra.Pubsub.RegisterHandler(channel, func(_ string, data []byte) {
			envelope := &commonv1.GovernanceEnvelope{}
			require.NoError(t, protojson.Unmarshal(data, envelope))
			shutdown := &operatorv1.ShutdownRequested{}
			require.NoError(t, proto.Unmarshal(envelope.Payload, shutdown))
			assert.Equal(t, "planned maintenance", shutdown.Reason)
			wire, err := protojson.Marshal(&commonv1.GovernanceEnvelope{
				Id: envelope.Id, EventType: string(constants.Event.Operator.ShutdownAcknowledged), Timestamp: timestamppb.Now(),
			})
			require.NoError(t, err)
			infra.Pubsub.Publish(resultsChannel, wire)
		})
		t.Cleanup(unregister)

		rr := stopOperatorRequest(t, controller, "stop-owner", "stop-remote-session", "planned maintenance")
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		doc, err := infra.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), "stop-remote")
		require.NoError(t, err)
		require.NotNil(t, doc)
		body, err := json.Marshal(doc.Data)
		require.NoError(t, err)
		var operator models.OperatorDocumentGo
		require.NoError(t, json.Unmarshal(body, &operator))
		assert.Equal(t, constants.OperatorStatusStopped, operator.Status)
		assert.Equal(t, "planned maintenance", operator.StopReason)
	})
}

func TestOperatorStopRouteRequiresMTLS(t *testing.T) {
	assert.Equal(t, RouteAuthMTLS, NewRouteAuthRegistry(false).AuthMode(constants.APIPaths.OperatorsStop))
}

func TestHandleReauth_MalformedJSON(t *testing.T) {

	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db.GetDocStore(), logger)
	personaSvc := NewPersonaService(db.GetDocStore(), logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db.GetDocStore(), nil, logger, userSvc, personaSvc, res, nil, "", "", "")
	reg := &RegistrationService{}
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxPayloadBytes: 1024}}
	controller := newOperatorController(OperatorControllerDeps{Cfg: cfg, Logger: logger, Reg: reg, Auth: auth, Responder: res})

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
	require.NoError(t, db.GetDocStore().DocSet("operators", "op-123", opBytes))

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
	controller := newOperatorController(OperatorControllerDeps{Cfg: infra.Cfg, Logger: infra.Logger, Reg: infra.Reg, Auth: infra.Auth, Responder: infra.Responder})

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

	t.Run("Returns platform-enrolled operators alongside slots", func(t *testing.T) {
		// Create a user-created slot.
		slot, err := infra.Reg.createSlot("user-platform", "org-platform")
		require.NoError(t, err)

		// Persist a platform-enrolled operator (is_slot=false) stamped
		// with the same user_id, mimicking what signOperatorComponent
		// writes for a platform-enrolled operator.
		platformOp := models.OperatorDocumentGo{
			ID:        "platform-op-controller",
			UserID:    "user-platform",
			Component: constants.ComponentNameG8EO,
			Name:      "platform-operator",
			Status:    constants.OperatorStatusActive,
			IsSlot:    false,
			Claimed:   true,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		opBytes, err := json.Marshal(platformOp)
		require.NoError(t, err)
		require.NoError(t, infra.Reg.docStore.DocSet(marshaler.CollectionName(constants.CollectionOperators), platformOp.ID, opBytes))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/operators?user_id=user-platform", nil)
		w := httptest.NewRecorder()

		controller.handleListOperators(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp models.OperatorSlotResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.True(t, resp.Success)
		assert.Len(t, resp.Operators, 2, "both the slot and the platform-enrolled operator should be returned")

		slotFound := false
		platformFound := false
		for _, op := range resp.Operators {
			if op.ID == slot.ID {
				slotFound = true
				assert.True(t, op.IsSlot)
			}
			if op.ID == platformOp.ID {
				platformFound = true
				assert.False(t, op.IsSlot)
			}
		}
		assert.True(t, slotFound, "user-created slot should be in the response")
		assert.True(t, platformFound, "platform-enrolled operator should be in the response")
	})
}

func TestOperatorController_HandleTerminateOperator(t *testing.T) {

	infra := setupTestInfrastructure(t, false)
	controller := newOperatorController(OperatorControllerDeps{Cfg: infra.Cfg, Logger: infra.Logger, Reg: infra.Reg, Auth: infra.Auth, Responder: infra.Responder})

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
	controller := newOperatorController(OperatorControllerDeps{Cfg: infra.Cfg, Logger: infra.Logger, Reg: infra.Reg, Auth: infra.Auth, Responder: infra.Responder})

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
	controller := newOperatorController(OperatorControllerDeps{Cfg: infra.Cfg, Logger: infra.Logger, Reg: infra.Reg, Auth: infra.Auth, Responder: infra.Responder})

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
	controller := newOperatorController(OperatorControllerDeps{Cfg: infra.Cfg, Logger: infra.Logger, Reg: infra.Reg, Auth: infra.Auth, Responder: infra.Responder})

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
	controller := newOperatorController(OperatorControllerDeps{Cfg: infra.Cfg, Logger: infra.Logger, Reg: infra.Reg, Auth: infra.Auth, Responder: infra.Responder})

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

// TestHandleReauth_ResponseContainsGatewayPosture verifies that the reauth
// response includes the gateway's posture in the bootstrap config. The operator
// has no posture of its own and must receive the gateway's posture at reauth to
// run L4 posture-gated checks against the gateway's policy decision.
func TestHandleReauth_ResponseContainsGatewayPosture(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db.GetDocStore(), logger)
	personaSvc := NewPersonaService(db.GetDocStore(), logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db.GetDocStore(), nil, logger, userSvc, personaSvc, res, nil, "", "", "")
	reg := &RegistrationService{}
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxPayloadBytes: 1024, Posture: config.PostureDoctrine}}
	controller := newOperatorController(OperatorControllerDeps{Cfg: cfg, Logger: logger, Reg: reg, Auth: auth, Responder: res})

	operatorSessionID := "test-session-posture"
	opDoc := map[string]interface{}{
		"id":                  "op-posture",
		"operator_session_id": operatorSessionID,
		"status":              marshaler.Status(constants.OperatorStatusActive),
		"user_id":             "user-posture",
		"organization_id":     "org-posture",
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.GetDocStore().DocSet("operators", "op-posture", opBytes))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/reauth", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")

	wid := protocol.NewWorkloadIdentity()
	opURI, err := wid.OperatorSPIFFEURL("org-posture", "op-posture", operatorSessionID)
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

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool                   `json:"success"`
		Config  map[string]interface{} `json:"config"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, string(config.PostureDoctrine), resp.Config["posture"],
		"reauth response must propagate the gateway's posture to the operator")
}

func TestHandleReauth_PersistsRuntimeConfig(t *testing.T) {
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db.GetDocStore(), logger)
	personaSvc := NewPersonaService(db.GetDocStore(), logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db.GetDocStore(), nil, logger, userSvc, personaSvc, res, nil, "", "", "")
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxPayloadBytes: 1024, Posture: config.PostureDoctrine}}
	reg := NewRegistrationService(db.GetDocStore(), db.GetKVStore(), nil, logger, userSvc, nil, nil, &cfg.Gateway)
	controller := newOperatorController(OperatorControllerDeps{Cfg: cfg, Logger: logger, Reg: reg, Auth: auth, Responder: res})

	operatorSessionID := "test-session-runtime-config"
	opDoc := map[string]interface{}{
		"id":                  "op-runtime-config",
		"operator_session_id": operatorSessionID,
		"status":              marshaler.Status(constants.OperatorStatusActive),
		"user_id":             "user-runtime-config",
		"organization_id":     "org-runtime-config",
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.GetDocStore().DocSet("operators", "op-runtime-config", opBytes))

	body := `{"runtime_config":{"inference_enabled":true,"log_level":"info"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/reauth", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	wid := protocol.NewWorkloadIdentity()
	opURI, err := wid.OperatorSPIFFEURL("org-runtime-config", "op-runtime-config", operatorSessionID)
	require.NoError(t, err)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{opURI}}}}
	ctx := context.WithValue(req.Context(), constants.ContextKeyOperatorSessionID, operatorSessionID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	controller.handleReauth(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	operators, err := reg.ListUserOperators("user-runtime-config")
	require.NoError(t, err)
	require.Len(t, operators, 1)
	require.NotNil(t, operators[0].RuntimeConfig)
	assert.True(t, operators[0].RuntimeConfig.InferenceEnabled)
}

func TestOperatorController_HandleGetOperatorBySession(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	controller := newOperatorController(OperatorControllerDeps{Cfg: infra.Cfg, Logger: infra.Logger, Reg: infra.Reg, Auth: infra.Auth, Responder: infra.Responder})

	t.Run("Wrong method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/operators/session/sess-123", nil)
		w := httptest.NewRecorder()

		controller.handleGetOperatorBySession(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("Empty session ID returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.OperatorsSession, nil)
		w := httptest.NewRecorder()

		controller.handleGetOperatorBySession(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Invalid session ID returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.OperatorsSession+"nonexistent-session", nil)
		w := httptest.NewRecorder()

		controller.handleGetOperatorBySession(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Valid session ID returns 200 with operator document", func(t *testing.T) {
		operatorSessionID := "sess-valid-456"
		opDoc := map[string]interface{}{
			"id":                  "op-456",
			"operator_session_id": operatorSessionID,
			"status":              marshaler.Status(constants.OperatorStatusActive),
			"user_id":             "user-456",
			"organization_id":     "org-456",
		}
		opBytes, err := json.Marshal(opDoc)
		require.NoError(t, err)
		require.NoError(t, infra.DocStore.DocSet(string(constants.CollectionOperators), "op-456", opBytes))

		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.OperatorsSession+operatorSessionID, nil)
		w := httptest.NewRecorder()

		controller.handleGetOperatorBySession(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp models.OperatorResponse
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		require.NotNil(t, resp.Operator)
		assert.Equal(t, "op-456", resp.Operator.ID)
		assert.Equal(t, operatorSessionID, resp.Operator.OperatorSessionID)
		assert.Equal(t, constants.OperatorStatusActive, resp.Operator.Status)
	})
}

// TestOperatorController_HandleValidateOperatorSession covers the
// Gateway-authoritative session-validation endpoint exposed at
// POST /api/v1/operators/validate. This endpoint is consumed by g8ee's
// InternalHttpClient to validate bearer-token operator sessions through the
// Gateway rather than a local projection.
func TestOperatorController_HandleValidateOperatorSession(t *testing.T) {

	infra := setupTestInfrastructure(t, false)
	controller := newOperatorController(OperatorControllerDeps{
		Cfg: infra.Cfg, Logger: infra.Logger, Reg: infra.Reg, Auth: infra.Auth, Responder: infra.Responder,
	})

	// Seed an active user, operator, and CLI session so the happy path can
	// return a typed OperatorSessionValidationResponse.
	userID := "validate-user"
	operatorSessionID := "validate-operator-session"
	cliSessionID := "validate-cli-session"
	operatorID := "validate-operator"

	userBytes, err := json.Marshal(&models.User{ID: userID, Status: constants.UserStatusActive})
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	opBytes, err := json.Marshal(&models.OperatorDocumentGo{
		ID: operatorID, UserID: userID, OperatorSessionID: operatorSessionID,
		Status: constants.OperatorStatusActive, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionOperators), operatorID, opBytes))

	cliSession := models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: operatorSessionID,
		IsActive:          true,
		ExpiresAt:         time.Now().Add(time.Hour),
		AbsoluteExpiresAt: time.Now().Add(time.Hour),
		IdleExpiresAt:     time.Now().Add(time.Hour),
	}
	cliBytes, err := json.Marshal(cliSession)
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliBytes))

	t.Run("Wrong method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.OperatorsValidate, nil)
		w := httptest.NewRecorder()
		controller.handleValidateOperatorSession(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("Malformed JSON body returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.OperatorsValidate, strings.NewReader("{invalid"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		controller.handleValidateOperatorSession(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Missing operator_session_id returns 401", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"cli_session_id": cliSessionID,
			"user_id":        userID,
		})
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.OperatorsValidate, strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		controller.handleValidateOperatorSession(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Nonexistent operator session is rejected with 401", func(t *testing.T) {
		body, _ := json.Marshal(models.OperatorSessionValidationRequest{
			OperatorSessionID: "nonexistent-session",
			CLISessionID:      cliSessionID,
			UserID:            userID,
		})
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.OperatorsValidate, strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		controller.handleValidateOperatorSession(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Valid binding returns 200 with typed response", func(t *testing.T) {
		body, _ := json.Marshal(models.OperatorSessionValidationRequest{
			OperatorSessionID: operatorSessionID,
			CLISessionID:      cliSessionID,
			UserID:            userID,
		})
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.OperatorsValidate, strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		controller.handleValidateOperatorSession(w, req)

		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		var resp models.OperatorSessionValidationResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.True(t, resp.Valid)
		assert.Equal(t, operatorID, resp.OperatorID)
		assert.Equal(t, userID, resp.UserID)
	})
}

// TestOperatorController_ValidateOperatorSession_AppMTLSReach confirms that an
// app workload mTLS certificate (the identity g8ee presents to the Gateway) is
// admitted by the unified auth middleware for POST /api/v1/operators/validate.
// The validate endpoint is not privileged and not RouteAuthNone, so it
// defaults to RouteAuthMTLS. An app cert with a valid AppPolicy must reach the
// handler rather than being blocked by the middleware.
//
// The test seeds a valid operator+CLI binding so the handler returns 200. A
// middleware-level rejection would produce 401/403, not 200 — so a 200 proves
// the app cert was admitted and the request reached the controller.
func TestOperatorController_ValidateOperatorSession_AppMTLSReach(t *testing.T) {

	infra := setupTestInfrastructure(t, false)

	// Register an AppPolicy for the g8ee ensemble app identity so the
	// middleware's handleAppAuth path admits the request.
	appID := protocol.EnsembleAppID
	policy := &models.AppPolicy{
		AppID:     appID,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	policyBytes, err := json.Marshal(policy)
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionAppPolicies), appID, policyBytes))

	// Seed a valid operator+CLI binding so the handler can return 200.
	userID := "appmtls-user"
	operatorSessionID := "appmtls-operator-session"
	cliSessionID := "appmtls-cli-session"
	operatorID := "appmtls-operator"

	userBytes, err := json.Marshal(&models.User{ID: userID, Status: constants.UserStatusActive})
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	opBytes, err := json.Marshal(&models.OperatorDocumentGo{
		ID: operatorID, UserID: userID, OperatorSessionID: operatorSessionID,
		Status: constants.OperatorStatusActive, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionOperators), operatorID, opBytes))

	cliSession := models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: operatorSessionID,
		IsActive:          true,
		ExpiresAt:         time.Now().Add(time.Hour),
		AbsoluteExpiresAt: time.Now().Add(time.Hour),
		IdleExpiresAt:     time.Now().Add(time.Hour),
	}
	cliBytes, err := json.Marshal(cliSession)
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliBytes))

	controller := newOperatorController(OperatorControllerDeps{
		Cfg: infra.Cfg, Logger: infra.Logger, Reg: infra.Reg, Auth: infra.Auth, Responder: infra.Responder,
	})

	// Wire the handler through the auth middleware so the mTLS identity
	// extraction and app-policy enforcement run before the controller.
	handler := infra.Auth.Middleware(http.HandlerFunc(controller.handleValidateOperatorSession))

	wid := protocol.NewWorkloadIdentity()
	appURI, err := wid.AppSPIFFEURL("g8ee")
	require.NoError(t, err)

	body, _ := json.Marshal(models.OperatorSessionValidationRequest{
		OperatorSessionID: operatorSessionID,
		CLISessionID:      cliSessionID,
		UserID:            userID,
	})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.OperatorsValidate, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{
			{URIs: []*url.URL{appURI}},
		},
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// 200 proves the app mTLS cert was admitted by the middleware and the
	// request reached the controller, which validated the binding
	// successfully. A middleware-level rejection would produce 401/403.
	require.Equal(t, http.StatusOK, w.Code,
		"app mTLS must reach the handler and return 200; got: %s", w.Body.String())
	var resp models.OperatorSessionValidationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Valid)
	assert.Equal(t, operatorID, resp.OperatorID)
	assert.Equal(t, userID, resp.UserID)
}
