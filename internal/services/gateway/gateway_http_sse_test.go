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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/protocol"
)

// makeTestAppCert creates a self-signed x509 certificate with app SPIFFE URI SANs.
// Used to simulate mTLS app workload identities for SSE push tests.
func makeTestAppCert(t *testing.T, spiffeURIs []string) *x509.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	var uris []*url.URL
	for _, s := range spiffeURIs {
		u, err := url.Parse(s)
		require.NoError(t, err)
		uris = append(uris, u)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-app-cert"},
		NotBefore:    time.Now().Add(-1 * time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		URIs:         uris,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)
	return cert
}

// makeTLSRequest creates an http.Request with r.TLS set to simulate mTLS auth.
func makeTLSRequest(method, path string, body string, cert *x509.Certificate) *http.Request {
	var bodyReader strings.Reader
	if body != "" {
		bodyReader = *strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, &bodyReader)
	if cert != nil {
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{cert},
		}
	}
	return req
}

// seedOperatorDoc inserts an operator document into the DocStore for test setup.
// The document is stored with operatorSessionID as the key, matching the SSE push
// code's DocGet(operators, operatorSessionID) lookup pattern.
func seedOperatorDoc(t *testing.T, h *HTTPHandler, opID, userID, operatorSessionID string) {
	t.Helper()
	op := models.OperatorDocumentGo{
		ID:                opID,
		UserID:            userID,
		Status:            constants.OperatorStatusActive,
		OperatorSessionID: operatorSessionID,
	}
	opBytes, err := json.Marshal(op)
	require.NoError(t, err)
	err = h.dataController.docStore.DocSet(marshaler.CollectionName(constants.CollectionOperators), operatorSessionID, opBytes)
	require.NoError(t, err)
}

// seedUserDoc inserts a user document with active status into the DocStore.
func seedUserDoc(t *testing.T, h *HTTPHandler, userID string) {
	t.Helper()
	userBytes, err := json.Marshal(map[string]interface{}{
		"status": string(constants.UserStatusActive),
	})
	require.NoError(t, err)
	err = h.dataController.docStore.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes)
	require.NoError(t, err)
}

// seedCLISessionDoc inserts a CLI session document into the DocStore for test setup.
func seedCLISessionDoc(t *testing.T, h *HTTPHandler, cliSessionID, userID, operatorSessionID string) {
	t.Helper()
	cliSess := models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: operatorSessionID,
	}
	cliBytes, err := json.Marshal(cliSess)
	require.NoError(t, err)
	err = h.dataController.docStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliBytes)
	require.NoError(t, err)
}

// bindWebSessionToOperators sets the KV binding from web session to operator session IDs.
func bindWebSessionToOperators(t *testing.T, h *HTTPHandler, webSessionID string, operatorSessionIDs []string) {
	t.Helper()
	raw, err := json.Marshal(operatorSessionIDs)
	require.NoError(t, err)
	err = h.dataController.kvStore.KVSet(sessionWebBindKey(webSessionID), string(raw), 0)
	require.NoError(t, err)
}

// bindOperatorToWebSession sets the KV binding from operator session to web session ID.
func bindOperatorToWebSession(t *testing.T, h *HTTPHandler, operatorSessionID, webSessionID string) {
	t.Helper()
	err := h.dataController.kvStore.KVSet(sessionOperatorBindKey(operatorSessionID), webSessionID, 0)
	require.NoError(t, err)
}

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
	body := fmt.Sprintf(`{"cli_session_id":"%s","event":{"type":"message","data":"hello"}}`, cliSessionID)
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
	body := `{"cli_session_id":"nonexistent-session","event":{"type":"test"}}`
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
	body := fmt.Sprintf(`{"cli_session_id":"%s","event":{"type":"test"}}`, cliSessionID)
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
	body := fmt.Sprintf(`{"cli_session_id":"%s","event":{"type":"test"}}`, cliSessionID)
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
	body := fmt.Sprintf(`{"web_session_id":"%s","event":{"type":"update"}}`, webSessionID)
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
	body := `{"web_session_id":"unbound-session","event":{"type":"test"}}`
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
	body := fmt.Sprintf(`{"web_session_id":"%s","event":{"type":"test"}}`, webSessionID)
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "unauthorized for target session")
}

func TestHandleInternalSSEPush_UserIDSuccess(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opID := "op-push-user"
	opSessID := "opsess-push-user"
	userID := "user-push-target"

	seedOperatorDoc(t, h, opID, userID, opSessID)

	appSpiffe := protocol.NewWorkloadIdentity().AppSPIFFEID(opSessID)
	cert := makeTestAppCert(t, []string{appSpiffe})
	body := fmt.Sprintf(`{"user_id":"%s","event":{"type":"broadcast"}}`, userID)
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp models.SSEPushResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
}

func TestHandleInternalSSEPush_UserIDNoOperators(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	cert := makeTestAppCert(t, []string{"spiffe://g8e.local/app/op1"})
	body := `{"user_id":"user-without-ops","event":{"type":"test"}}`
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "unauthorized for target user")
}

func TestHandleInternalSSEPush_UserIDAppNotAuthorized(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opID := "op-user-mismatch"
	opSessID := "opsess-user-mismatch"
	userID := "user-mismatch"

	seedOperatorDoc(t, h, opID, userID, opSessID)

	// Use a different app identity
	cert := makeTestAppCert(t, []string{"spiffe://g8e.local/app/different-op"})
	body := fmt.Sprintf(`{"user_id":"%s","event":{"type":"test"}}`, userID)
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "unauthorized for target user")
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
	body := fmt.Sprintf(`{"cli_session_id":"%s","event":{"data":"no type here"}}`, cliSessionID)
	req := makeTLSRequest(http.MethodPost, "/api/v1/sse/push", body, cert)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEPush(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	// Verify the event was stored with type "unknown"
	route := SSERoute{CLISessionID: cliSessionID}
	rows, err := h.dataController.sseStore.SSEEventsListSince(route, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, string(constants.SystemHealthUnknown), rows[0].EventType)
}

// ---------------------------------------------------------------------------
// authorizeSSERoute
// ---------------------------------------------------------------------------

func TestAuthorizeSSERoute_MissingAuthIdentity(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	route := SSERoute{CLISessionID: "sess1"}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events?cli_session_id=sess1", nil)
	_, err := h.sseController.authorizeSSERoute(route, req)
	require.Error(t, err)
	sseErr, ok := err.(*sseAuthError)
	require.True(t, ok)
	assert.Equal(t, http.StatusUnauthorized, sseErr.status)
}

func TestAuthorizeSSERoute_MultipleRoutingTargets(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	route := SSERoute{CLISessionID: "sess1", WebSessionID: "web1"}
	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, "opsess1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)
	_, err := h.sseController.authorizeSSERoute(route, req)
	require.Error(t, err)
	sseErr, ok := err.(*sseAuthError)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, sseErr.status)
	assert.Contains(t, sseErr.message, "exactly one routing target required")
}

func TestAuthorizeSSERoute_NoRoutingTarget(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	route := SSERoute{}
	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, "opsess1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)
	_, err := h.sseController.authorizeSSERoute(route, req)
	require.Error(t, err)
	sseErr, ok := err.(*sseAuthError)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, sseErr.status)
}

func TestAuthorizeSSERoute_CLISession_OperatorMTLSAuth_Success(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-auth-cli-ok"
	userID := "user-auth-cli-ok"
	cliSessionID := "cli-auth-ok"

	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	route := SSERoute{CLISessionID: cliSessionID}
	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	channel, err := h.sseController.authorizeSSERoute(route, req)
	require.NoError(t, err)
	assert.Equal(t, "sse:cli:"+cliSessionID, channel)
}

func TestAuthorizeSSERoute_CLISession_OperatorMTLSAuth_WrongOperator(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-owner"
	cliSessionID := "cli-owned"
	seedCLISessionDoc(t, h, cliSessionID, "user1", opSessID)

	route := SSERoute{CLISessionID: cliSessionID}
	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, "different-opsess")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	_, err := h.sseController.authorizeSSERoute(route, req)
	require.Error(t, err)
	sseErr, ok := err.(*sseAuthError)
	require.True(t, ok)
	assert.Equal(t, http.StatusForbidden, sseErr.status)
	assert.Contains(t, sseErr.message, "operator session does not own this cli session")
}

func TestAuthorizeSSERoute_CLISession_CLIMTLSAuth_Success(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-cli-mtls"
	userID := "user-cli-mtls"
	cliSessionID := "cli-mtls-ok"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	route := SSERoute{CLISessionID: cliSessionID}
	// CLI mTLS: stamps userID but not operatorSessionID, webSessionID, or appID
	ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, userID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	channel, err := h.sseController.authorizeSSERoute(route, req)
	require.NoError(t, err)
	assert.Equal(t, "sse:cli:"+cliSessionID, channel)
}

func TestAuthorizeSSERoute_CLISession_CLIMTLSAuth_WrongUser(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-cli-wrong"
	userID := "user-owner"
	cliSessionID := "cli-wrong-user"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	route := SSERoute{CLISessionID: cliSessionID}
	ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, "different-user")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	_, err := h.sseController.authorizeSSERoute(route, req)
	require.Error(t, err)
	sseErr, ok := err.(*sseAuthError)
	require.True(t, ok)
	assert.Equal(t, http.StatusForbidden, sseErr.status)
	assert.Contains(t, sseErr.message, "user does not own this cli session")
}

func TestAuthorizeSSERoute_CLISession_CookieAuth_Success(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-cookie"
	userID := "user-cookie"
	cliSessionID := "cli-cookie-ok"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	route := SSERoute{CLISessionID: cliSessionID}
	ctx := context.WithValue(context.Background(), constants.ContextKeyWebSessionID, "web-sess")
	ctx = context.WithValue(ctx, constants.ContextKeyUserID, userID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	channel, err := h.sseController.authorizeSSERoute(route, req)
	require.NoError(t, err)
	assert.Equal(t, "sse:cli:"+cliSessionID, channel)
}

func TestAuthorizeSSERoute_CLISession_CookieAuth_WrongUser(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-cookie-wrong"
	userID := "user-cookie-owner"
	cliSessionID := "cli-cookie-wrong"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	route := SSERoute{CLISessionID: cliSessionID}
	ctx := context.WithValue(context.Background(), constants.ContextKeyWebSessionID, "web-sess")
	ctx = context.WithValue(ctx, constants.ContextKeyUserID, "different-user")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	_, err := h.sseController.authorizeSSERoute(route, req)
	require.Error(t, err)
	sseErr, ok := err.(*sseAuthError)
	require.True(t, ok)
	assert.Equal(t, http.StatusForbidden, sseErr.status)
}

func TestAuthorizeSSERoute_CLISession_NotFound(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	route := SSERoute{CLISessionID: "nonexistent"}
	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, "opsess1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	_, err := h.sseController.authorizeSSERoute(route, req)
	require.Error(t, err)
	sseErr, ok := err.(*sseAuthError)
	require.True(t, ok)
	assert.Equal(t, http.StatusForbidden, sseErr.status)
	assert.Contains(t, sseErr.message, "cli session not found")
}

func TestAuthorizeSSERoute_WebSession_OperatorMTLSAuth_Success(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-web-mtls"
	webSessionID := "web-mtls-ok"
	bindOperatorToWebSession(t, h, opSessID, webSessionID)

	route := SSERoute{WebSessionID: webSessionID}
	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	channel, err := h.sseController.authorizeSSERoute(route, req)
	require.NoError(t, err)
	assert.Equal(t, "sse:web:"+webSessionID, channel)
}

func TestAuthorizeSSERoute_WebSession_OperatorMTLSAuth_WrongBinding(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-web-bound"
	webSessionID := "web-bound"
	bindOperatorToWebSession(t, h, opSessID, webSessionID)

	route := SSERoute{WebSessionID: "different-web-session"}
	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	_, err := h.sseController.authorizeSSERoute(route, req)
	require.Error(t, err)
	sseErr, ok := err.(*sseAuthError)
	require.True(t, ok)
	assert.Equal(t, http.StatusForbidden, sseErr.status)
	assert.Contains(t, sseErr.message, "operator session does not own this web session")
}

func TestAuthorizeSSERoute_WebSession_OperatorMTLSAuth_NoBinding(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	route := SSERoute{WebSessionID: "web-unbound"}
	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, "opsess-unbound")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	_, err := h.sseController.authorizeSSERoute(route, req)
	require.Error(t, err)
	sseErr, ok := err.(*sseAuthError)
	require.True(t, ok)
	assert.Equal(t, http.StatusForbidden, sseErr.status)
}

func TestAuthorizeSSERoute_WebSession_CookieAuth_Success(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	webSessionID := "web-cookie-ok"
	userID := "user-web-cookie"

	route := SSERoute{WebSessionID: webSessionID}
	ctx := context.WithValue(context.Background(), constants.ContextKeyWebSessionID, webSessionID)
	ctx = context.WithValue(ctx, constants.ContextKeyUserID, userID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	channel, err := h.sseController.authorizeSSERoute(route, req)
	require.NoError(t, err)
	assert.Equal(t, "sse:web:"+webSessionID, channel)
}

func TestAuthorizeSSERoute_WebSession_CookieAuth_Mismatch(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	route := SSERoute{WebSessionID: "web-expected"}
	ctx := context.WithValue(context.Background(), constants.ContextKeyWebSessionID, "web-actual")
	ctx = context.WithValue(ctx, constants.ContextKeyUserID, "user1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	_, err := h.sseController.authorizeSSERoute(route, req)
	require.Error(t, err)
	sseErr, ok := err.(*sseAuthError)
	require.True(t, ok)
	assert.Equal(t, http.StatusForbidden, sseErr.status)
	assert.Contains(t, sseErr.message, "web session does not match authenticated session")
}

func TestAuthorizeSSERoute_UserID_OperatorMTLSAuth_Success(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opID := "op-user-mtls"
	opSessID := "opsess-user-mtls"
	userID := "user-mtls-ok"
	seedUserDoc(t, h, userID)
	seedOperatorDoc(t, h, opID, userID, opSessID)

	route := SSERoute{UserID: userID}
	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	channel, err := h.sseController.authorizeSSERoute(route, req)
	require.NoError(t, err)
	assert.Equal(t, "sse:user:"+userID, channel)
}

func TestAuthorizeSSERoute_UserID_OperatorMTLSAuth_DifferentUser(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opID := "op-user-diff"
	opSessID := "opsess-user-diff"
	userID := "user-owner"
	seedUserDoc(t, h, userID)
	seedOperatorDoc(t, h, opID, userID, opSessID)

	route := SSERoute{UserID: "different-user"}
	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	_, err := h.sseController.authorizeSSERoute(route, req)
	require.Error(t, err)
	sseErr, ok := err.(*sseAuthError)
	require.True(t, ok)
	assert.Equal(t, http.StatusForbidden, sseErr.status)
	assert.Contains(t, sseErr.message, "operator does not belong to this user")
}

func TestAuthorizeSSERoute_UserID_OperatorMTLSAuth_InvalidSession(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	route := SSERoute{UserID: "user1"}
	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, "nonexistent-opsess")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	_, err := h.sseController.authorizeSSERoute(route, req)
	require.Error(t, err)
	sseErr, ok := err.(*sseAuthError)
	require.True(t, ok)
	assert.Equal(t, http.StatusUnauthorized, sseErr.status)
	assert.Contains(t, sseErr.message, "invalid Operator session")
}

func TestAuthorizeSSERoute_UserID_CookieAuth_Success(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	userID := "user-cookie-ok"
	route := SSERoute{UserID: userID}
	ctx := context.WithValue(context.Background(), constants.ContextKeyWebSessionID, "web-sess")
	ctx = context.WithValue(ctx, constants.ContextKeyUserID, userID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	channel, err := h.sseController.authorizeSSERoute(route, req)
	require.NoError(t, err)
	assert.Equal(t, "sse:user:"+userID, channel)
}

func TestAuthorizeSSERoute_UserID_CookieAuth_Mismatch(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	route := SSERoute{UserID: "user-expected"}
	ctx := context.WithValue(context.Background(), constants.ContextKeyWebSessionID, "web-sess")
	ctx = context.WithValue(ctx, constants.ContextKeyUserID, "user-actual")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	_, err := h.sseController.authorizeSSERoute(route, req)
	require.Error(t, err)
	sseErr, ok := err.(*sseAuthError)
	require.True(t, ok)
	assert.Equal(t, http.StatusForbidden, sseErr.status)
	assert.Contains(t, sseErr.message, "user does not match authenticated user")
}

func TestAuthorizeSSERoute_AppCertExcludedFromMTLSAuth(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	userID := "user-app-test"
	route := SSERoute{UserID: userID}
	// App cert stamps both userID and appID — should NOT be treated as mTLS auth
	ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, constants.ContextKeyAppID, "spiffe://g8e.local/app/op1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events", nil).WithContext(ctx)

	_, err := h.sseController.authorizeSSERoute(route, req)
	require.Error(t, err)
	sseErr, ok := err.(*sseAuthError)
	require.True(t, ok)
	// Without webSessionID and with appID set, isMTLSAuth is false and isCookieAuth is false
	assert.Equal(t, http.StatusUnauthorized, sseErr.status)
}

// ---------------------------------------------------------------------------
// handleInternalSSEEvents
// ---------------------------------------------------------------------------

func TestHandleInternalSSEEvents_MethodNotAllowed(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sse/events", nil)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEEvents(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleInternalSSEEvents_AuthFailure(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events?cli_session_id=sess1", nil)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEEvents(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleInternalSSEEvents_BadRequestFromMultipleTargets(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, "opsess1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events?cli_session_id=sess1&web_session_id=web1", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEEvents(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleInternalSSEEvents_Success(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-events-ok"
	userID := "user-events-ok"
	cliSessionID := "cli-events-ok"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	// Push an event first
	route := SSERoute{CLISessionID: cliSessionID}
	err := h.dataController.sseStore.SSEEventsAppend(route, "test_event", `{"event":{"type":"test_event"}}`, "test-app")
	require.NoError(t, err)

	// Query events with operator mTLS auth
	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events?cli_session_id="+cliSessionID+"&since_id=0&limit=10", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEEvents(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp models.SSEEventsResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Count)
	assert.Len(t, resp.Events, 1)
	assert.Equal(t, "test_event", resp.Events[0].EventType)
}

func TestHandleInternalSSEEvents_EmptyResult(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-empty"
	userID := "user-empty"
	cliSessionID := "cli-empty"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/events?cli_session_id="+cliSessionID, nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEEvents(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	var resp models.SSEEventsResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Count)
	assert.Empty(t, resp.Events)
}

// ---------------------------------------------------------------------------
// handleInternalSSEStream
// ---------------------------------------------------------------------------

func TestHandleInternalSSEStream_MethodNotAllowed(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sse/stream", nil)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEStream(rr, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandleInternalSSEStream_AuthFailure(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream?cli_session_id=sess1", nil)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEStream(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandleInternalSSEStream_BadRequestFromMultipleTargets(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, "opsess1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream?cli_session_id=sess1&user_id=user1", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	h.sseController.handleInternalSSEStream(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandleInternalSSEStream_SSEHeadersSet(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-stream-headers"
	userID := "user-stream-headers"
	cliSessionID := "cli-stream-headers"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream?cli_session_id="+cliSessionID, nil).WithContext(ctx)
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()

	// Run in goroutine with cancellable context so we can capture headers and stop
	streamCtx, cancel := context.WithCancel(ctx)
	req = req.WithContext(streamCtx)

	done := make(chan struct{})
	go func() {
		h.sseController.handleInternalSSEStream(rr, req)
		close(done)
	}()

	// Give the handler a moment to write headers
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	assert.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "keep-alive", rr.Header().Get("Connection"))
	assert.Equal(t, "no", rr.Header().Get("X-Accel-Buffering"))
}

func TestHandleInternalSSEStream_SSEHeadersNoOrigin(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-stream-wildcard"
	userID := "user-stream-wildcard"
	cliSessionID := "cli-stream-wildcard"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream?cli_session_id="+cliSessionID, nil).WithContext(ctx)
	// No Origin header → SSE headers still set, no CORS headers
	rr := httptest.NewRecorder()

	streamCtx, cancel := context.WithCancel(ctx)
	req = req.WithContext(streamCtx)

	done := make(chan struct{})
	go func() {
		h.sseController.handleInternalSSEStream(rr, req)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	assert.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", rr.Header().Get("Cache-Control"))
	assert.Equal(t, "keep-alive", rr.Header().Get("Connection"))
	assert.Empty(t, rr.Header().Get("Access-Control-Allow-Origin"))
}

func TestHandleInternalSSEStream_LastEventIDOverridesSinceID(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-last-event"
	userID := "user-last-event"
	cliSessionID := "cli-last-event"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	// Push two events
	route := SSERoute{CLISessionID: cliSessionID}
	require.NoError(t, h.dataController.sseStore.SSEEventsAppend(route, "event1", `{"event":{"type":"event1"}}`, "app1"))
	require.NoError(t, h.dataController.sseStore.SSEEventsAppend(route, "event2", `{"event":{"type":"event2"}}`, "app1"))

	// Get all events to find the first event's ID
	rows, err := h.dataController.sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	firstID := rows[0].ID

	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream?cli_session_id="+cliSessionID+"&since_id=0", nil).WithContext(ctx)
	req.Header.Set("Last-Event-ID", fmt.Sprintf("%d", firstID))
	rr := httptest.NewRecorder()

	streamCtx, cancel := context.WithCancel(ctx)
	req = req.WithContext(streamCtx)

	done := make(chan struct{})
	go func() {
		h.sseController.handleInternalSSEStream(rr, req)
		close(done)
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done

	// Should have replayed only events after firstID (i.e., event2)
	body := rr.Body.String()
	assert.Contains(t, body, "event2")
	assert.NotContains(t, body, "event1")
}

func TestHandleInternalSSEStream_ReplaysEventsFromDB(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-replay"
	userID := "user-replay"
	cliSessionID := "cli-replay"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	route := SSERoute{CLISessionID: cliSessionID}
	// Push a dummy event first so the real event gets ID > 1
	require.NoError(t, h.dataController.sseStore.SSEEventsAppend(route, "dummy_event", `{"event":{"type":"dummy_event"}}`, "app1"))
	require.NoError(t, h.dataController.sseStore.SSEEventsAppend(route, "replay_event", `{"event":{"type":"replay_event"}}`, "app1"))

	// Get all events to find IDs
	rows, err := h.dataController.sseStore.SSEEventsListSince(route, 0, 100)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	dummyID := rows[0].ID
	replayID := rows[1].ID

	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream?cli_session_id="+cliSessionID, nil).WithContext(ctx)
	// Use dummy event ID as Last-Event-ID so replay starts after it
	req.Header.Set("Last-Event-ID", fmt.Sprintf("%d", dummyID))
	rr := httptest.NewRecorder()

	streamCtx, cancel := context.WithCancel(ctx)
	req = req.WithContext(streamCtx)

	done := make(chan struct{})
	go func() {
		h.sseController.handleInternalSSEStream(rr, req)
		close(done)
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done

	body := rr.Body.String()
	assert.Contains(t, body, "replay_event")
	assert.Contains(t, body, fmt.Sprintf("id: %d", replayID))
	assert.NotContains(t, body, "dummy_event")
}

func TestHandleInternalSSEStream_NoReplayWhenSinceIDZero(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-no-replay"
	userID := "user-no-replay"
	cliSessionID := "cli-no-replay"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	route := SSERoute{CLISessionID: cliSessionID}
	require.NoError(t, h.dataController.sseStore.SSEEventsAppend(route, "no_replay_event", `{"event":{"type":"no_replay_event"}}`, "app1"))

	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	// since_id=0 and no Last-Event-ID → no replay should occur
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream?cli_session_id="+cliSessionID+"&since_id=0", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	streamCtx, cancel := context.WithCancel(ctx)
	req = req.WithContext(streamCtx)

	done := make(chan struct{})
	go func() {
		h.sseController.handleInternalSSEStream(rr, req)
		close(done)
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done

	// Event should NOT be replayed since sinceID=0
	body := rr.Body.String()
	assert.NotContains(t, body, "no_replay_event")
}

func TestHandleInternalSSEStream_PubSubEventDelivery(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-pubsub"
	userID := "user-pubsub"
	cliSessionID := "cli-pubsub"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream?cli_session_id="+cliSessionID, nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	streamCtx, cancel := context.WithCancel(ctx)
	req = req.WithContext(streamCtx)

	done := make(chan struct{})
	go func() {
		h.sseController.handleInternalSSEStream(rr, req)
		close(done)
	}()

	// Wait for stream to start
	time.Sleep(100 * time.Millisecond)

	// Publish an event to the pubsub channel
	payload := `{"cli_session_id":"` + cliSessionID + `","event":{"type":"pubsub_event","data":"hello"}}`
	h.pubsub.Publish("sse:cli:"+cliSessionID, []byte(payload))

	// Wait for delivery
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	body := rr.Body.String()
	assert.Contains(t, body, "pubsub_event")
	assert.Contains(t, body, "data: "+payload)
}

func TestHandleInternalSSEStream_HeartbeatSent(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	// Override to a short interval so the test can observe a real heartbeat.
	h.sseController.heartbeat = 50 * time.Millisecond

	opSessID := "opsess-heartbeat"
	userID := "user-heartbeat"
	cliSessionID := "cli-heartbeat"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream?cli_session_id="+cliSessionID, nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	streamCtx, cancel := context.WithCancel(ctx)
	req = req.WithContext(streamCtx)

	done := make(chan struct{})
	go func() {
		h.sseController.handleInternalSSEStream(rr, req)
		close(done)
	}()

	// Wait long enough for at least one heartbeat tick (50ms interval).
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	body := rr.Body.String()
	assert.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"))
	assert.Contains(t, body, ": heartbeat\n\n", "expected at least one heartbeat comment in SSE stream")
}

func TestHandleInternalSSEStream_ClientLabelOperatorSession(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opSessID := "opsess-label"
	userID := "user-label"
	cliSessionID := "cli-label"
	seedCLISessionDoc(t, h, cliSessionID, userID, opSessID)

	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream?cli_session_id="+cliSessionID, nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	streamCtx, cancel := context.WithCancel(ctx)
	req = req.WithContext(streamCtx)

	done := make(chan struct{})
	go func() {
		h.sseController.handleInternalSSEStream(rr, req)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	// Stream should be established — just verify it didn't error
	assert.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"))
}

func TestHandleInternalSSEStream_ClientLabelWebSession(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	webSessionID := "web-label"
	userID := "user-web-label"

	ctx := context.WithValue(context.Background(), constants.ContextKeyWebSessionID, webSessionID)
	ctx = context.WithValue(ctx, constants.ContextKeyUserID, userID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream?web_session_id="+webSessionID, nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	streamCtx, cancel := context.WithCancel(ctx)
	req = req.WithContext(streamCtx)

	done := make(chan struct{})
	go func() {
		h.sseController.handleInternalSSEStream(rr, req)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	assert.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"))
}

func TestHandleInternalSSEStream_ClientLabelUserID(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	opID := "op-label-user"
	opSessID := "opsess-label-user"
	userID := "user-label-id"
	seedUserDoc(t, h, userID)
	seedOperatorDoc(t, h, opID, userID, opSessID)

	ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, opSessID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sse/stream?user_id="+userID, nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	streamCtx, cancel := context.WithCancel(ctx)
	req = req.WithContext(streamCtx)

	done := make(chan struct{})
	go func() {
		h.sseController.handleInternalSSEStream(rr, req)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	assert.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"))
}

// ---------------------------------------------------------------------------
// SSEPushPayload JSON round-trip
// ---------------------------------------------------------------------------

func TestSSEPushPayload_JSONRoundTrip(t *testing.T) {
	original := models.SSEPushPayload{
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
