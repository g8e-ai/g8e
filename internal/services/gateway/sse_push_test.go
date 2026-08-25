// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/protocol"
)

// ---------------------------------------------------------------------------
// sseAuthError
// ---------------------------------------------------------------------------

func TestSSEAuthError_Message(t *testing.T) {
	err := &sseAuthError{status: http.StatusForbidden, message: "test forbidden"}
	assert.Equal(t, "test forbidden", err.Error())
	assert.Equal(t, http.StatusForbidden, err.status)
}

// ---------------------------------------------------------------------------
// handleInternalSSEPush
// ---------------------------------------------------------------------------

func TestHandleInternalSSEPush_MethodNotAllowed(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	req := makeTLSRequest(http.MethodGet, "/api/v1/sse/push", "", nil)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleInternalSSEPush_MissingMTLSCertificate(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sse/push", strings.NewReader("{}"))
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "mTLS client certificate required")
}

func TestHandleInternalSSEPush_NotAppWorkloadIdentity(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	cert := makeTestAppCert(t, []string{"spiffe://g8e.local/operator/org1/op1/sess1"})
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", "{}", cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "unauthorized client identity")
}

func TestHandleInternalSSEPush_OperatorIdentityRejected(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	cert := makeTestAppCert(t, []string{"spiffe://g8e.local/app/g8eo"})
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", "{}", cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestHandleInternalSSEPush_GatewayIdentityRejected(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	cert := makeTestAppCert(t, []string{"spiffe://g8e.local/app/g8eg"})
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", "{}", cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestHandleInternalSSEPush_InvalidJSONBody(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	cert := makeTestAppCert(t, []string{"spiffe://g8e.local/app/op1"})
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", "{invalid json", cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid JSON body")
}

func TestHandleInternalSSEPush_MissingEventField(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	cert := makeTestAppCert(t, []string{"spiffe://g8e.local/app/op1"})
	body := `{"cli_session_id":"sess1"}`
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "event field is required")
}

func TestHandleInternalSSEPush_NoRoutingTarget(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	cert := makeTestAppCert(t, []string{"spiffe://g8e.local/app/op1"})
	body := `{"event":{"type":"test"}}`
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	// SSEEventsAppend will fail with route validation error since no routing target
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleInternalSSEPush_CLISessionSuccess(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opID := "op-push-cli"
	opSessID := "opsess-push-cli"
	userID := "user-push-cli"
	cliSessionID := "cli-push-cli"

	seedOperatorDoc(t, h, opID, userID, opSessID)
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	appSpiffe := protocol.NewWorkloadIdentity().AppSPIFFEID(opSessID)
	cert := makeTestAppCert(t, []string{appSpiffe})
	body := fmt.Sprintf(`{"user_id":"%s","cli_session_id":"%s","event":{"type":"message","data":"hello"}}`, userID, cliSessionID)
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp models.SSEPushResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, 1, resp.Delivered)
}

func TestHandleInternalSSEPush_CLISessionNotFound(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opID := "op-push-cli-notfound"
	opSessID := "opsess-push-cli-notfound"
	seedOperatorDoc(t, h, opID, "user-x", opSessID)

	appSpiffe := protocol.NewWorkloadIdentity().AppSPIFFEID(opSessID)
	cert := makeTestAppCert(t, []string{appSpiffe})
	body := `{"user_id":"user-x","cli_session_id":"nonexistent-session","event":{"type":"test"}}`
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "target session not found")
}

func TestHandleInternalSSEPush_CLISessionOperatorNotFound(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	cliSessionID := "cli-orphan"
	seedCLISessionDoc(t, h, cliSessionID, "user-orphan", "nonexistent-opsess")

	cert := makeTestAppCert(t, []string{"spiffe://g8e.local/app/op-orphan"})
	body := fmt.Sprintf(`{"user_id":"user-orphan","cli_session_id":"%s","event":{"type":"test"}}`, cliSessionID)
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "operator session not found")
}

func TestHandleInternalSSEPush_CLISessionAppNotAuthorized(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opID := "op-auth-cli"
	opSessID := "opsess-auth-cli"
	userID := "user-auth-cli"
	cliSessionID := "cli-auth-cli"

	seedOperatorDoc(t, h, opID, userID, opSessID)
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	// Use a different app identity that doesn't match the operator session
	cert := makeTestAppCert(t, []string{"spiffe://g8e.local/app/different-opsess"})
	body := fmt.Sprintf(`{"user_id":"%s","cli_session_id":"%s","event":{"type":"test"}}`, userID, cliSessionID)
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "unauthorized for target session")
}

func TestHandleInternalSSEPush_WebSessionSuccess(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opID := "op-push-web"
	opSessID := "opsess-push-web"
	webSessionID := "web-push-test"

	seedOperatorDoc(t, h, opID, "user-push-web", opSessID)
	bindWebSessionToOperators(t, h, webSessionID, []string{opSessID})

	appSpiffe := protocol.NewWorkloadIdentity().AppSPIFFEID(opSessID)
	cert := makeTestAppCert(t, []string{appSpiffe})
	body := fmt.Sprintf(`{"user_id":"user-push-web","web_session_id":"%s","event":{"type":"update"}}`, webSessionID)
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp models.SSEPushResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
}

func TestHandleInternalSSEPush_WebSessionNoBindings(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	cert := makeTestAppCert(t, []string{"spiffe://g8e.local/app/op1"})
	body := `{"user_id":"user-web-nobind","web_session_id":"unbound-session","event":{"type":"test"}}`
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "target session not found or not bound")
}

func TestHandleInternalSSEPush_WebSessionAppNotAuthorized(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opID := "op-web-auth"
	opSessID := "opsess-web-auth"
	webSessionID := "web-auth-test"

	seedOperatorDoc(t, h, opID, "user-web-auth", opSessID)
	bindWebSessionToOperators(t, h, webSessionID, []string{opSessID})

	// Different app identity
	cert := makeTestAppCert(t, []string{"spiffe://g8e.local/app/wrong-op"})
	body := fmt.Sprintf(`{"user_id":"user-web-auth","web_session_id":"%s","event":{"type":"test"}}`, webSessionID)
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "unauthorized for target session")
}

func TestHandleInternalSSEPush_EventWithoutTypeDefaultsToUnknown(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opID := "op-notype"
	opSessID := "opsess-notype"
	userID := "user-notype"
	cliSessionID := "cli-notype"

	seedOperatorDoc(t, h, opID, userID, opSessID)
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	appSpiffe := protocol.NewWorkloadIdentity().AppSPIFFEID(opSessID)
	cert := makeTestAppCert(t, []string{appSpiffe})
	// Event without "type" field — should default to "unknown"
	body := fmt.Sprintf(`{"user_id":"%s","cli_session_id":"%s","event":{"data":"no type here"}}`, userID, cliSessionID)
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	// Verify the event was stored with type "unknown"
	route := SSERoute{UserID: userID, CLISessionID: cliSessionID}
	rows, err := h.dataController.sseStore.SSEEventsListSince(route, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, string(constants.SystemHealthUnknown), rows[0].EventType)
}

// ---------------------------------------------------------------------------
// SSEPushPayload JSON round-trip
// ---------------------------------------------------------------------------

func TestSSEPushPayload_JSONRoundTrip(t *testing.T) {
	original := models.SSEPushPayload{
		UserID:       "user-123",
		WebSessionID: "web-123",
		Event:        json.RawMessage(`{"type":"test","data":"hello"}`),
	}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded models.SSEPushPayload
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, original.WebSessionID, decoded.WebSessionID)
	assert.Equal(t, original.CliSessionID, decoded.CliSessionID)
	assert.Equal(t, original.UserID, decoded.UserID)
	assert.JSONEq(t, string(original.Event), string(decoded.Event))
}

func TestSSEPushPayload_EmptyEvent(t *testing.T) {
	payload := models.SSEPushPayload{
		CliSessionID: "cli-123",
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var decoded models.SSEPushPayload
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "cli-123", decoded.CliSessionID)
	// json.RawMessage for a nil field unmarshals to the bytes "null"
	assert.Equal(t, "null", string(decoded.Event))
}

// ---------------------------------------------------------------------------
// R15 regression: no persistence on authorization failure
// ---------------------------------------------------------------------------

func TestHandleInternalSSEPush_AuthFailureDoesNotPersistEvent(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opID := "op-no-persist"
	opSessID := "opsess-no-persist"
	userID := "user-no-persist"
	cliSessionID := "cli-no-persist"

	seedOperatorDoc(t, h, opID, userID, opSessID)
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	// Use a mismatched app identity so authorization fails with 403.
	cert := makeTestAppCert(t, []string{"spiffe://g8e.local/app/wrong-opsess"})
	body := fmt.Sprintf(`{"user_id":"%s","cli_session_id":"%s","event":{"type":"should_not_persist"}}`, userID, cliSessionID)
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)

	// R15: the event must NOT be persisted in the DB.
	route := SSERoute{UserID: userID, CLISessionID: cliSessionID}
	count, err := h.dataController.sseStore.SSEEventsCount()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "no events should be persisted after auth failure")

	rows, err := h.dataController.sseStore.SSEEventsListSince(route, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, rows, "no rows should be retrievable for the route after auth failure")
}
