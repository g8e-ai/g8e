// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// newProducerFileSvc creates a RuntimeFileService backed by a temp directory
// with the full .g8e runtime tree created. This is the Tier 1 (non-integration)
// equivalent of newTestFileSvc from test_setup_test.go.
func newProducerFileSvc(t *testing.T) fs.RuntimeFileService {
	t.Helper()
	baseDir := testutil.TempDir(t)
	svc, err := fs.NewRuntimeFileService(baseDir, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, svc.CreateRuntimeTree(context.Background()))
	return svc
}

// newProducerControllerTestEnv builds a real in-memory SQLite-backed
// ObserveProducerController (DocumentStoreService + SSEEventService +
// pubsub handler) so controller-level tests exercise the real
// persistence and SSE emission paths without mocks. Mirrors the
// Tier 1 in-memory SQLite pattern used by sse_event_service_test.go.
func newProducerControllerTestEnv(t *testing.T) *ObserveProducerController {
	t.Helper()
	logger := testutil.NewTestLogger()
	cfg := sqliteutil.DefaultDBConfig(":memory:")
	db, err := sqliteutil.OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(gatewaySchema)
	require.NoError(t, err)

	docStore := NewDocumentStoreService(db, logger)
	sseStore := NewSSEEventService(db, logger)
	pubsub := NewGatewayWebSocketHandler(logger)
	producer := NewObserveProducerService(docStore, sseStore, pubsub, newProducerFileSvc(t), logger)
	responder := response.NewWriter(logger)
	return newObserveProducerController(ObserveProducerControllerDeps{
		Cfg:          nil,
		Logger:       logger,
		ProducerSvc:  producer,
		Responder:    responder,
		MaxBodyBytes: 512 * 1024,
	})
}

// withAppAuthCtx stamps the mTLS-derived app identity and delegated user
// identity into the request context, mirroring handleAppAuth. The request
// contract contains no user_id field; the controller derives user_id from
// the peer certificate, never from the body.
func withAppAuthCtx(r *http.Request, appID, userID string) *http.Request {
	ctx := context.WithValue(r.Context(), constants.ContextKeyAppID, appID)
	ctx = context.WithValue(ctx, constants.ContextKeyUserID, userID)
	return r.WithContext(ctx)
}

// withCLIAuthCtx stamps the mTLS-derived CLI session identity and user
// identity into the request context, mirroring handleCLIAuth. The eval
// publication endpoint accepts CLI auth in addition to app auth so the
// g8e-evals publish command can publish through the governed CLI ingress.
func withCLIAuthCtx(r *http.Request, cliSessionID, userID string) *http.Request {
	ctx := context.WithValue(r.Context(), constants.ContextKeyCLISessionID, cliSessionID)
	ctx = context.WithValue(ctx, constants.ContextKeyUserID, userID)
	return r.WithContext(ctx)
}

// validAgentBody returns a JSON body for a valid agent producer request
// routed via a web session.
func validAgentBody(t *testing.T, agentID string, status models.AgentLifecycleStatus) string {
	t.Helper()
	body, err := json.Marshal(models.ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       agentID,
		DisplayName:   "Sage",
		Role:          "reasoner",
		Status:        status,
		ObservedAt:    time.Now().UTC(),
		WebSessionID:  "web-session-unit",
	})
	require.NoError(t, err)
	return string(body)
}

// validRunBody returns a JSON body for a valid run producer request routed
// via a web session.
func validRunBody(t *testing.T, runID string, status models.RunLifecycleStatus) string {
	t.Helper()
	body, err := json.Marshal(models.ObserveProducerRunStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		RunID:         runID,
		RunKind:       models.RunKindInvestigation,
		DisplayName:   "Investigation Alpha",
		Status:        status,
		TotalTasks:    3,
		ObservedAt:    time.Now().UTC(),
		WebSessionID:  "web-session-unit",
	})
	require.NoError(t, err)
	return string(body)
}

// decodeAcceptedResponse parses the typed ObserveProducerResponse from the
// recorder body.
func decodeAcceptedResponse(t *testing.T, w *httptest.ResponseRecorder) models.ObserveProducerResponse {
	t.Helper()
	var resp models.ObserveProducerResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body: %s", w.Body.String())
	return resp
}

// decodeErrorResponse parses the {error: string} envelope from the recorder.
func decodeErrorResponse(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "body: %s", w.Body.String())
	return env.Error
}

func TestObserveProducerController_HandleAgentState_PostSuccess(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(validAgentBody(t, "agent-ok", models.AgentLifecycleStatusRunning)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleAgentState(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeAcceptedResponse(t, w)
	assert.True(t, resp.Accepted)
}

func TestObserveProducerController_HandleRunState_PostSuccess(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(validRunBody(t, "run-ok", models.RunLifecycleStatusRunning)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeAcceptedResponse(t, w)
	assert.True(t, resp.Accepted)
}

func TestObserveProducerController_HandleAgentState_MethodRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveProducerAgentState, nil)
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleAgentState(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestObserveProducerController_HandleRunState_MethodRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPut, constants.APIPaths.ObserveProducerRunState, nil)
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestObserveProducerController_HandleAgentState_MissingAppIDRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(validAgentBody(t, "agent-noapp", models.AgentLifecycleStatusRunning)))
	// No ContextKeyAppID set: not an app workload.
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	controller.handleAgentState(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, decodeErrorResponse(t, w), constants.ErrForbidden.Error())
}

func TestObserveProducerController_HandleAgentState_MissingUserIDRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(validAgentBody(t, "agent-nouser", models.AgentLifecycleStatusRunning)))
	// App workload set but no delegated user identity.
	ctx := context.WithValue(req.Context(), constants.ContextKeyAppID, "app-workload")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	controller.handleAgentState(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestObserveProducerController_HandleRunState_MissingAppIDRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(validRunBody(t, "run-noapp", models.RunLifecycleStatusRunning)))
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestObserveProducerController_HandleRunState_MissingUserIDRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(validRunBody(t, "run-nouser", models.RunLifecycleStatusRunning)))
	ctx := context.WithValue(req.Context(), constants.ContextKeyAppID, "app-workload")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestObserveProducerController_HandleAgentState_MalformedJSONRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader("{not json"))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleAgentState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.True(t, errors.Is(constants.ErrInvalidJSONBody, constants.ErrInvalidJSONBody))
}

func TestObserveProducerController_HandleRunState_MalformedJSONRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader("plain text"))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleAgentState_UnknownFieldRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body := validAgentBody(t, "agent-unknown", models.AgentLifecycleStatusRunning)
	// Inject an unknown field; strict decode must reject it.
	body = strings.Replace(body, `}`, `,"user_id":"attacker"}`, 1)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(body))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleAgentState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleRunState_UnknownFieldRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body := validRunBody(t, "run-unknown", models.RunLifecycleStatusRunning)
	body = strings.Replace(body, `}`, `,"secret":"leak"}`, 1)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(body))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleAgentState_TrailingJSONRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body := validAgentBody(t, "agent-trailing", models.AgentLifecycleStatusRunning) + `{"extra":"obj"}`

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(body))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleAgentState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleRunState_TrailingJSONRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body := validRunBody(t, "run-trailing", models.RunLifecycleStatusRunning) + `42`

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(body))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleAgentState_OversizedBodyRejected(t *testing.T) {
	logger := testutil.NewTestLogger()
	cfg := sqliteutil.DefaultDBConfig(":memory:")
	db, err := sqliteutil.OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(gatewaySchema)
	require.NoError(t, err)

	docStore := NewDocumentStoreService(db, logger)
	sseStore := NewSSEEventService(db, logger)
	pubsub := NewGatewayWebSocketHandler(logger)
	producer := NewObserveProducerService(docStore, sseStore, pubsub, newProducerFileSvc(t), logger)
	responder := response.NewWriter(logger)
	// Tiny max body to force oversize rejection.
	controller := newObserveProducerController(ObserveProducerControllerDeps{
		Cfg:          nil,
		Logger:       logger,
		ProducerSvc:  producer,
		Responder:    responder,
		MaxBodyBytes: 16,
	})

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(validAgentBody(t, "agent-big", models.AgentLifecycleStatusRunning)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleAgentState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleAgentState_WebRouteConstruction(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body := validAgentBody(t, "agent-web-route", models.AgentLifecycleStatusRunning)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(body))
	req = withAppAuthCtx(req, "app-workload", "user-web")
	w := httptest.NewRecorder()

	controller.handleAgentState(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestObserveProducerController_HandleAgentState_CLIRouteConstruction(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body, err := json.Marshal(models.ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-cli-route",
		DisplayName:   "Sage",
		Role:          "reasoner",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    time.Now().UTC(),
		CLISessionID:  "cli-session-unit",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(string(body)))
	req = withAppAuthCtx(req, "app-workload", "user-cli")
	w := httptest.NewRecorder()

	controller.handleAgentState(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestObserveProducerController_HandleRunState_MissingRouteRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	// No web_session_id and no cli_session_id: targetless route.
	body, err := json.Marshal(models.ObserveProducerRunStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		RunID:         "run-no-route",
		RunKind:       models.RunKindInvestigation,
		DisplayName:   "No Route",
		Status:        models.RunLifecycleStatusRunning,
		TotalTasks:    1,
		ObservedAt:    time.Now().UTC(),
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(string(body)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleAgentState_MutuallyExclusiveRouteRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body, err := json.Marshal(models.ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-both",
		DisplayName:   "Sage",
		Role:          "reasoner",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    time.Now().UTC(),
		WebSessionID:  "web-1",
		CLISessionID:  "cli-1",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(string(body)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleAgentState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleAgentState_InvalidTransitionRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	// Seed a completed agent.
	seedReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(validAgentBody(t, "agent-trans", models.AgentLifecycleStatusCompleted)))
	seedReq = withAppAuthCtx(seedReq, "app-workload", "user-1")
	seedW := httptest.NewRecorder()
	controller.handleAgentState(seedW, seedReq)
	require.Equal(t, http.StatusOK, seedW.Code)

	// Attempt terminal regression: completed -> running.
	regressBody := validAgentBody(t, "agent-trans", models.AgentLifecycleStatusRunning)
	regressReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(regressBody))
	regressReq = withAppAuthCtx(regressReq, "app-workload", "user-1")
	regressW := httptest.NewRecorder()
	controller.handleAgentState(regressW, regressReq)

	assert.Equal(t, http.StatusBadRequest, regressW.Code)
}

func TestObserveProducerController_HandleRunState_InvalidTransitionRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	// Seed a completed run.
	seedReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(validRunBody(t, "run-trans", models.RunLifecycleStatusCompleted)))
	seedReq = withAppAuthCtx(seedReq, "app-workload", "user-1")
	seedW := httptest.NewRecorder()
	controller.handleRunState(seedW, seedReq)
	require.Equal(t, http.StatusOK, seedW.Code)

	// Attempt terminal regression: completed -> running.
	regressBody := validRunBody(t, "run-trans", models.RunLifecycleStatusRunning)
	regressReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(regressBody))
	regressReq = withAppAuthCtx(regressReq, "app-workload", "user-1")
	regressW := httptest.NewRecorder()
	controller.handleRunState(regressW, regressReq)

	assert.Equal(t, http.StatusBadRequest, regressW.Code)
}

func TestObserveProducerController_HandleAgentState_StaleUpdateRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	later := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	earlier := time.Date(2026, 9, 9, 11, 0, 0, 0, time.UTC)

	seedBody, err := json.Marshal(models.ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-stale",
		DisplayName:   "Sage",
		Role:          "reasoner",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    later,
		WebSessionID:  "web-1",
	})
	require.NoError(t, err)
	seedReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(string(seedBody)))
	seedReq = withAppAuthCtx(seedReq, "app-workload", "user-1")
	seedW := httptest.NewRecorder()
	controller.handleAgentState(seedW, seedReq)
	require.Equal(t, http.StatusOK, seedW.Code)

	staleBody, err := json.Marshal(models.ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-stale",
		DisplayName:   "Sage",
		Role:          "reasoner",
		Status:        models.AgentLifecycleStatusWaiting,
		ObservedAt:    earlier,
		WebSessionID:  "web-1",
	})
	require.NoError(t, err)
	staleReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(string(staleBody)))
	staleReq = withAppAuthCtx(staleReq, "app-workload", "user-1")
	staleW := httptest.NewRecorder()
	controller.handleAgentState(staleW, staleReq)

	assert.Equal(t, http.StatusBadRequest, staleW.Code)
}

func TestObserveProducerController_HandleRunState_CrossUserForbiddenNonDisclosing(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	// Seed a run for user-a.
	seedReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(validRunBody(t, "run-cross", models.RunLifecycleStatusQueued)))
	seedReq = withAppAuthCtx(seedReq, "app-workload", "user-a")
	seedW := httptest.NewRecorder()
	controller.handleRunState(seedW, seedReq)
	require.Equal(t, http.StatusOK, seedW.Code)

	// user-b attempts to update user-a's run: ownership mismatch maps to 403
	// with the same non-disclosing forbidden response (no record existence leak).
	attackBody := validRunBody(t, "run-cross", models.RunLifecycleStatusRunning)
	attackReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(attackBody))
	attackReq = withAppAuthCtx(attackReq, "app-workload", "user-b")
	attackW := httptest.NewRecorder()
	controller.handleRunState(attackW, attackReq)

	assert.Equal(t, http.StatusForbidden, attackW.Code)
	msg := decodeErrorResponse(t, attackW)
	assert.Contains(t, msg, constants.ErrForbidden.Error())
	// The forbidden response must not disclose which sentinel (agent vs run)
	// triggered the mismatch.
	assert.NotContains(t, msg, constants.ErrObserveRunNotFound.Error())
	assert.NotContains(t, msg, constants.ErrObserveAgentNotFound.Error())
}

func TestObserveProducerController_HandleAgentState_CrossUserForbiddenNonDisclosing(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	// Seed an agent for user-a.
	seedReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(validAgentBody(t, "agent-cross", models.AgentLifecycleStatusIdle)))
	seedReq = withAppAuthCtx(seedReq, "app-workload", "user-a")
	seedW := httptest.NewRecorder()
	controller.handleAgentState(seedW, seedReq)
	require.Equal(t, http.StatusOK, seedW.Code)

	// user-b attempts to update user-a's agent.
	attackBody := validAgentBody(t, "agent-cross", models.AgentLifecycleStatusRunning)
	attackReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(attackBody))
	attackReq = withAppAuthCtx(attackReq, "app-workload", "user-b")
	attackW := httptest.NewRecorder()
	controller.handleAgentState(attackW, attackReq)

	assert.Equal(t, http.StatusForbidden, attackW.Code)
	msg := decodeErrorResponse(t, attackW)
	assert.Contains(t, msg, constants.ErrForbidden.Error())
	assert.NotContains(t, msg, constants.ErrObserveAgentNotFound.Error())
	assert.NotContains(t, msg, constants.ErrObserveRunNotFound.Error())
}

func TestObserveProducerController_HandleAgentState_RequestBodyCannotOverrideUserID(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	// The request model has no user_id field. Attempting to inject one is
	// rejected by strict decode as an unknown field.
	body := validAgentBody(t, "agent-override", models.AgentLifecycleStatusRunning)
	body = strings.Replace(body, `}`, `,"user_id":"attacker"}`, 1)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(body))
	req = withAppAuthCtx(req, "app-workload", "user-real")
	w := httptest.NewRecorder()

	controller.handleAgentState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleRunState_UnknownIdentityFieldRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body := validRunBody(t, "run-identity", models.RunLifecycleStatusRunning)
	body = strings.Replace(body, `}`, `,"user_id":"attacker","owner":"attacker"}`, 1)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(body))
	req = withAppAuthCtx(req, "app-workload", "user-real")
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleAgentState_UnsupportedSchemaVersionRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body, err := json.Marshal(models.ObserveProducerAgentStateRequest{
		SchemaVersion: "9.9.9",
		AgentID:       "agent-schema",
		DisplayName:   "Sage",
		Role:          "reasoner",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    time.Now().UTC(),
		WebSessionID:  "web-1",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(string(body)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleAgentState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleRunState_MissingDisplayNameRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body, err := json.Marshal(models.ObserveProducerRunStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		RunID:         "run-nodisplay",
		RunKind:       models.RunKindInvestigation,
		DisplayName:   "",
		Status:        models.RunLifecycleStatusRunning,
		TotalTasks:    1,
		ObservedAt:    time.Now().UTC(),
		WebSessionID:  "web-1",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(string(body)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleRunState_NegativeTaskCountRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body, err := json.Marshal(models.ObserveProducerRunStateRequest{
		SchemaVersion:  constants.ObserveEventPayloadSchemaVersion,
		RunID:          "run-neg",
		RunKind:        models.RunKindInvestigation,
		DisplayName:    "Neg",
		Status:         models.RunLifecycleStatusRunning,
		CompletedTasks: -1,
		TotalTasks:     3,
		ObservedAt:     time.Now().UTC(),
		WebSessionID:   "web-1",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(string(body)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleRunState_CompletedExceedsTotalRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body, err := json.Marshal(models.ObserveProducerRunStateRequest{
		SchemaVersion:  constants.ObserveEventPayloadSchemaVersion,
		RunID:          "run-exceed",
		RunKind:        models.RunKindInvestigation,
		DisplayName:    "Exceed",
		Status:         models.RunLifecycleStatusRunning,
		CompletedTasks: 5,
		TotalTasks:     3,
		ObservedAt:     time.Now().UTC(),
		WebSessionID:   "web-1",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(string(body)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleRunState_ReversedTimestampsRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	started := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	ended := time.Date(2026, 9, 9, 11, 0, 0, 0, time.UTC)

	body, err := json.Marshal(models.ObserveProducerRunStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		RunID:         "run-reversed",
		RunKind:       models.RunKindInvestigation,
		DisplayName:   "Reversed",
		Status:        models.RunLifecycleStatusCompleted,
		TotalTasks:    1,
		StartedAt:     &started,
		EndedAt:       &ended,
		ObservedAt:    time.Now().UTC(),
		WebSessionID:  "web-1",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(string(body)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleRunState_ValidZeroCountersAccepted(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body, err := json.Marshal(models.ObserveProducerRunStateRequest{
		SchemaVersion:  constants.ObserveEventPayloadSchemaVersion,
		RunID:          "run-zero",
		RunKind:        models.RunKindInvestigation,
		DisplayName:    "Zero",
		Status:         models.RunLifecycleStatusRunning,
		CompletedTasks: 0,
		TotalTasks:     0,
		ObservedAt:     time.Now().UTC(),
		WebSessionID:   "web-1",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(string(body)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, decodeAcceptedResponse(t, w).Accepted)
}

func TestObserveProducerController_HandleAgentState_UnknownStatusRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body, err := json.Marshal(map[string]any{
		"schema_version": constants.ObserveEventPayloadSchemaVersion,
		"agent_id":       "agent-bogus",
		"display_name":   "Sage",
		"role":           "reasoner",
		"status":         "bogus",
		"observed_at":    time.Now().UTC().Format(time.RFC3339Nano),
		"web_session_id": "web-1",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(string(body)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleAgentState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleRunState_UnknownRunKindRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body, err := json.Marshal(map[string]any{
		"schema_version": constants.ObserveEventPayloadSchemaVersion,
		"run_id":         "run-bogus-kind",
		"run_kind":       "bogus",
		"display_name":   "Bogus",
		"status":         "running",
		"total_tasks":    1,
		"observed_at":    time.Now().UTC().Format(time.RFC3339Nano),
		"web_session_id": "web-1",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(string(body)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleAgentState_ResponseHasNoOwnershipField(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerAgentState, strings.NewReader(validAgentBody(t, "agent-resp", models.AgentLifecycleStatusRunning)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleAgentState(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.NotContains(t, body, "user_id")
	assert.NotContains(t, body, "agent_id")
	assert.Contains(t, body, "accepted")
}

func TestObserveProducerController_HandleRunState_ResponseHasNoOwnershipField(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerRunState, strings.NewReader(validRunBody(t, "run-resp", models.RunLifecycleStatusRunning)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleRunState(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.NotContains(t, body, "user_id")
	assert.NotContains(t, body, "run_id")
	assert.Contains(t, body, "accepted")
}

// validEvalPublicationBody returns a JSON body for a valid eval publication
// request routed via a web session. The download content is a small JSON
// object whose SHA-256 and size match the declared values.
func validEvalPublicationBody(t *testing.T, runID string) string {
	t.Helper()
	content := []byte(`{"analysis":"ok"}`)
	contentHash := sha256.Sum256(content)
	contentHashHex := hex.EncodeToString(contentHash[:])
	contentB64 := base64.StdEncoding.EncodeToString(content)
	body, err := json.Marshal(models.ObserveProducerEvalPublicationRequest{
		SchemaVersion:    constants.ObservePublicationSchemaVersion,
		BundleID:         "bundle-" + runID,
		RunID:            runID,
		ReleaseVersion:   "2.1.8",
		SuiteID:          "suite-" + runID,
		SuiteVersion:     "1.0.0",
		CampaignID:       "campaign-" + runID,
		ArmIDs:           []string{"arm-" + runID},
		ModelCohortIDs:   []string{"cohort-" + runID},
		AssignmentCount:  1,
		ReceiptCount:     1,
		AssignedTasks:    1,
		TerminalAttempts: 1,
		Metrics: []models.EvalMetricSummary{
			{
				SchemaVersion:      constants.ObserveEventPayloadSchemaVersion,
				MetricID:           "metric-" + runID,
				MetricVersion:      "1.0.0",
				ModelCohortID:      "cohort-" + runID,
				ArmID:              "arm-" + runID,
				Unit:               "count",
				Eligible:           1,
				Denominator:        1,
				VerificationStatus: models.EvalVerificationVerified,
			},
		},
		VerificationReport: models.VerificationReportWire{
			SchemaVersion:  constants.VerificationReportSchemaVersion,
			BundleID:       "bundle-" + runID,
			RunID:          runID,
			ReleaseVersion: "2.1.8",
			VerifiedAt:     time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
			OK:             true,
			Layers: []models.LayerResultWire{
				{Layer: 1, Passed: true, FailureCount: 0},
			},
		},
		BundleManifest: models.BundleManifestWire{
			SchemaVersion: constants.BundleManifestSchemaVersion,
			BundleID:      "bundle-" + runID,
			RunID:         runID,
			Artifacts: []models.BundleArtifactEntryWire{
				{
					Path:         "analysis/analysis.json",
					MediaType:    "application/json",
					PrivacyClass: "public",
					SHA256:       contentHashHex,
					ByteLength:   int64(len(content)),
					ArtifactType: "analysis",
				},
			},
		},
		Downloads: []models.ObserveProducerDownloadArtifactInput{
			{
				ArtifactID:            "artifact-" + runID,
				Filename:              "analysis.json",
				MediaType:             "application/json",
				ByteSize:              int64(len(content)),
				SHA256:                contentHashHex,
				PrivacyClassification: models.DownloadPrivacyPublicSafe,
				SourceRunID:           runID,
				Content:               contentB64,
			},
		},
		WebSessionID: "web-session-unit",
	})
	require.NoError(t, err)
	return string(body)
}

func TestObserveProducerController_HandleEvalPublication_PostSuccess(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerEvalPublication, strings.NewReader(validEvalPublicationBody(t, "eval-ok")))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleEvalPublication(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	resp := decodeAcceptedResponse(t, w)
	assert.True(t, resp.Accepted)
}

func TestObserveProducerController_HandleEvalPublication_MethodRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.ObserveProducerEvalPublication, nil)
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleEvalPublication(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestObserveProducerController_HandleEvalPublication_MissingIdentityRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerEvalPublication, strings.NewReader(validEvalPublicationBody(t, "eval-noapp")))
	ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	controller.handleEvalPublication(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, decodeErrorResponse(t, w), constants.ErrForbidden.Error())
}

func TestObserveProducerController_HandleEvalPublication_CLIAuthAccepted(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerEvalPublication, strings.NewReader(validEvalPublicationBody(t, "eval-cli")))
	req = withCLIAuthCtx(req, "cli-session-1", "user-cli")
	w := httptest.NewRecorder()

	controller.handleEvalPublication(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	resp := decodeAcceptedResponse(t, w)
	assert.True(t, resp.Accepted)
}

func TestObserveProducerController_HandleEvalPublication_CLISessionMissingUserIDRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerEvalPublication, strings.NewReader(validEvalPublicationBody(t, "eval-cli-nouser")))
	ctx := context.WithValue(req.Context(), constants.ContextKeyCLISessionID, "cli-session-1")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	controller.handleEvalPublication(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestObserveProducerController_HandleEvalPublication_MissingUserIDRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerEvalPublication, strings.NewReader(validEvalPublicationBody(t, "eval-nouser")))
	ctx := context.WithValue(req.Context(), constants.ContextKeyAppID, "app-workload")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	controller.handleEvalPublication(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestObserveProducerController_HandleEvalPublication_MalformedJSONRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerEvalPublication, strings.NewReader("{not json"))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleEvalPublication(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleEvalPublication_UnknownFieldsRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body := validEvalPublicationBody(t, "eval-unknown") + `,"extra":true}`
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerEvalPublication, strings.NewReader(body))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleEvalPublication(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleEvalPublication_UnverifiedReportRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	body := validEvalPublicationBody(t, "eval-unverified")
	// Patch the OK field to false.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(body), &raw))
	raw["verification_report"] = []byte(`{"schema_version":"1.0.0","bundle_id":"bundle-eval-unverified","run_id":"eval-unverified","release_version":"2.1.8","verified_at":"2026-09-09T12:00:00Z","ok":false,"layers":[],"failures":[]}`)
	patched, err := json.Marshal(raw)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerEvalPublication, strings.NewReader(string(patched)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleEvalPublication(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, decodeErrorResponse(t, w), constants.ErrObservePublicationNotVerified.Error())
}

func TestObserveProducerController_HandleEvalPublication_MissingRouteRejected(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	// Build a body with no routing target.
	content := []byte(`{"analysis":"ok"}`)
	contentHash := sha256.Sum256(content)
	contentHashHex := hex.EncodeToString(contentHash[:])
	contentB64 := base64.StdEncoding.EncodeToString(content)
	body, err := json.Marshal(models.ObserveProducerEvalPublicationRequest{
		SchemaVersion:   constants.ObservePublicationSchemaVersion,
		BundleID:        "bundle-no-route",
		RunID:           "eval-no-route",
		ReleaseVersion:  "2.1.8",
		SuiteID:         "suite-no-route",
		SuiteVersion:    "1.0.0",
		CampaignID:      "campaign-no-route",
		ArmIDs:          []string{"arm-no-route"},
		ModelCohortIDs:  []string{"cohort-no-route"},
		AssignmentCount: 1,
		VerificationReport: models.VerificationReportWire{
			SchemaVersion: constants.VerificationReportSchemaVersion,
			BundleID:      "bundle-no-route",
			RunID:         "eval-no-route",
			OK:            true,
		},
		BundleManifest: models.BundleManifestWire{
			SchemaVersion: constants.BundleManifestSchemaVersion,
			BundleID:      "bundle-no-route",
			RunID:         "eval-no-route",
			Artifacts: []models.BundleArtifactEntryWire{
				{Path: "a.json", MediaType: "application/json", PrivacyClass: "public", SHA256: contentHashHex, ByteLength: int64(len(content))},
			},
		},
		Downloads: []models.ObserveProducerDownloadArtifactInput{
			{ArtifactID: "dl-no-route", Filename: "a.json", MediaType: "application/json", ByteSize: int64(len(content)), SHA256: contentHashHex, PrivacyClassification: models.DownloadPrivacyPublicSafe, SourceRunID: "eval-no-route", Content: contentB64},
		},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerEvalPublication, strings.NewReader(string(body)))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleEvalPublication(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestObserveProducerController_HandleEvalPublication_ResponseHasNoOwnershipField(t *testing.T) {
	controller := newProducerControllerTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.ObserveProducerEvalPublication, strings.NewReader(validEvalPublicationBody(t, "eval-resp")))
	req = withAppAuthCtx(req, "app-workload", "user-1")
	w := httptest.NewRecorder()

	controller.handleEvalPublication(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	body := w.Body.String()
	assert.NotContains(t, body, "user_id")
	assert.Contains(t, body, "accepted")
}
