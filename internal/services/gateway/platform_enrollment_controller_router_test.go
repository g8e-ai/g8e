// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

// Router-level integration tests for the platform enrollment controller.
// These tests prove the auth mode and transport availability of every
// platform enrollment route, satisfying the Phase 3 exit criterion:
//
//   - request, status, and complete are reachable on both the HTTPS router
//     (buildPublicRouter) and the plain HTTP discovery router
//     (buildHTTPRouter) without authentication (RouteAuthNone).
//   - pending and decision are reachable on the HTTPS router but NOT
//     registered on the plain HTTP router (denial of approval over plain
//     HTTP).
//   - pending and decision require authentication (denial by missing
//     identity).
//   - pending and decision deny non-owner identities (active second user).
//   - pending and decision deny inactive identities (disabled first user).
//   - pending and decision deny revoked/expired identities.
//   - pending and decision deny app identities (mTLS app URI SAN with no
//     app policy).
//
// The tests exercise the real auth middleware, real web session service, real
// user service, and real platform enrollment controller through the full
// router chain. They do not mock internal services.

package gateway

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// platformEnrollmentRouterEnv bundles the services needed by the
// router-level integration tests. The HTTPS and plain HTTP routers are
// pre-built from the full HTTPHandler so every test exercises the real
// middleware chain.
type platformEnrollmentRouterEnv struct {
	svc         *GatewayModeService
	httpsRouter http.Handler
	httpRouter  http.Handler
	enrollSvc   *PlatformEnrollmentService
	userSvc     *UserService
	webSession  *WebSessionService
	ownerID     string
}

// setupPlatformEnrollmentRouterEnv constructs a full GatewayModeService with
// the in-process OperatorPubSubService wired with PlatformEnrollmentDeps,
// creates the first user (the owner), and pre-builds both routers. The
// gateway is NOT started (no port binding); the routers are exercised
// directly via httptest.
func setupPlatformEnrollmentRouterEnv(t *testing.T) *platformEnrollmentRouterEnv {
	t.Helper()

	env := setupPlatformEnrollmentEnv(t, true)
	h := env.svc.GetHTTPHandler()
	require.NotNil(t, h, "HTTP handler must be wired after construction")

	return &platformEnrollmentRouterEnv{
		svc:         env.svc,
		httpsRouter: h.buildPublicRouter(),
		httpRouter:  h.buildHTTPRouter(),
		enrollSvc:   env.enrollSvc,
		userSvc:     env.userSvc,
		webSession:  env.svc.webSessionSvc,
		ownerID:     env.ownerID,
	}
}

// createWebSessionCookie creates a valid web session for the given user ID
// and returns the cookie to attach to a test request.
func createWebSessionCookie(t *testing.T, env *platformEnrollmentRouterEnv, userID string) *http.Cookie {
	t.Helper()
	session, err := env.webSession.CreateWebSession(userID)
	require.NoError(t, err)
	require.NotNil(t, session)
	return &http.Cookie{Name: constants.WebSessionCookieName, Value: session.ID}
}

// createExpiredWebSessionCookie creates a web session document with an
// already-expired timestamp, simulating a revoked/expired session.
func createExpiredWebSessionCookie(t *testing.T, env *platformEnrollmentRouterEnv, userID string) *http.Cookie {
	t.Helper()
	sessionID := "expired-pe-router-session"
	session := &models.WebSession{
		ID:              sessionID,
		UserID:          userID,
		ExpiresAtUnixMs: time.Now().Add(-1 * time.Hour).UnixMilli(),
		CreatedAtUnixMs: time.Now().Add(-2 * time.Hour).UnixMilli(),
	}
	sessionBytes, err := json.Marshal(session)
	require.NoError(t, err)
	require.NoError(t, env.svc.docStore.DocSet("web_sessions", sessionID, sessionBytes))
	return &http.Cookie{Name: constants.WebSessionCookieName, Value: sessionID}
}

// appMTLSCert returns a synthetic x509 certificate carrying an app SPIFFE
// URI SAN, simulating an mTLS-authenticated app identity with no app policy.
func appMTLSCert(t *testing.T) *x509.Certificate {
	t.Helper()
	appURI, err := url.Parse("spiffe://g8e.local/app/rogue-app")
	require.NoError(t, err)
	return &x509.Certificate{URIs: []*url.URL{appURI}}
}

// ============================================================================
// Transport availability tests
// ============================================================================

// TestPlatformEnrollmentRouter_PublicRoutesReachableOnHTTPS proves that
// request, status, and complete are registered on the HTTPS router
// (buildPublicRouter) and reachable without authentication. A 400 (bad
// request) proves the handler was invoked; a 401/403 would mean the auth
// middleware blocked it; a 404 would mean the route is not registered.
func TestPlatformEnrollmentRouter_PublicRoutesReachableOnHTTPS(t *testing.T) {
	env := setupPlatformEnrollmentRouterEnv(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "request",
			method: http.MethodPost,
			path:   constants.APIPaths.AuthPlatformEnrollmentRequest,
			body:   `{}`,
		},
		{
			name:   "status",
			method: http.MethodGet,
			path:   constants.APIPaths.AuthPlatformEnrollmentStatus,
			body:   "",
		},
		{
			name:   "complete",
			method: http.MethodPost,
			path:   constants.APIPaths.AuthPlatformEnrollmentComplete,
			body:   `{}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var bodyReader *bytes.Reader
			if tc.body != "" {
				bodyReader = bytes.NewReader([]byte(tc.body))
			} else {
				bodyReader = bytes.NewReader(nil)
			}
			req := httptest.NewRequest(tc.method, tc.path, bodyReader)
			req.Header.Set("Content-Type", "application/json")
			req.TLS = nil
			rr := httptest.NewRecorder()

			env.httpsRouter.ServeHTTP(rr, req)

			assert.NotEqual(t, http.StatusNotFound, rr.Code,
				"HTTPS router must register the %s route (got 404)", tc.name)
			assert.NotEqual(t, http.StatusUnauthorized, rr.Code,
				"HTTPS router must not block the %s route with auth (RouteAuthNone)", tc.name)
			assert.NotEqual(t, http.StatusForbidden, rr.Code,
				"HTTPS router must not block the %s route with auth (RouteAuthNone)", tc.name)
		})
	}
}

// TestPlatformEnrollmentRouter_PublicRoutesReachableOnPlainHTTP proves
// that request, status, and complete are registered on the plain HTTP
// discovery router (buildHTTPRouter) and reachable without authentication.
// An unenrolled workload has no client certificate, so the discovery
// surface must be available over plain HTTP.
func TestPlatformEnrollmentRouter_PublicRoutesReachableOnPlainHTTP(t *testing.T) {
	env := setupPlatformEnrollmentRouterEnv(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "request",
			method: http.MethodPost,
			path:   constants.APIPaths.AuthPlatformEnrollmentRequest,
			body:   `{}`,
		},
		{
			name:   "status",
			method: http.MethodGet,
			path:   constants.APIPaths.AuthPlatformEnrollmentStatus,
			body:   "",
		},
		{
			name:   "complete",
			method: http.MethodPost,
			path:   constants.APIPaths.AuthPlatformEnrollmentComplete,
			body:   `{}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var bodyReader *bytes.Reader
			if tc.body != "" {
				bodyReader = bytes.NewReader([]byte(tc.body))
			} else {
				bodyReader = bytes.NewReader(nil)
			}
			req := httptest.NewRequest(tc.method, tc.path, bodyReader)
			req.Header.Set("Content-Type", "application/json")
			req.TLS = nil
			rr := httptest.NewRecorder()

			env.httpRouter.ServeHTTP(rr, req)

			assert.NotEqual(t, http.StatusNotFound, rr.Code,
				"plain HTTP router must register the %s route (got 404)", tc.name)
			assert.NotEqual(t, http.StatusUnauthorized, rr.Code,
				"plain HTTP router must not block the %s route with auth", tc.name)
		})
	}
}

// TestPlatformEnrollmentRouter_OwnerRoutesNotOnPlainHTTP proves that
// pending and decision are NOT registered on the plain HTTP discovery
// router. They require owner authentication (web session or mTLS), which is
// only available over HTTPS. A 404 (or redirect) proves the route is not
// registered on plain HTTP — this is the "denial of approval over plain
// HTTP" requirement.
func TestPlatformEnrollmentRouter_OwnerRoutesNotOnPlainHTTP(t *testing.T) {
	env := setupPlatformEnrollmentRouterEnv(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "pending",
			method: http.MethodGet,
			path:   constants.APIPaths.AuthPlatformEnrollmentPending,
			body:   "",
		},
		{
			name:   "decision",
			method: http.MethodPost,
			path:   constants.APIPaths.AuthPlatformEnrollmentDecision,
			body:   `{"request_id":"nonexistent","decision":"approve"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var bodyReader *bytes.Reader
			if tc.body != "" {
				bodyReader = bytes.NewReader([]byte(tc.body))
			} else {
				bodyReader = bytes.NewReader(nil)
			}
			req := httptest.NewRequest(tc.method, tc.path, bodyReader)
			req.Header.Set("Content-Type", "application/json")
			req.TLS = nil
			rr := httptest.NewRecorder()

			env.httpRouter.ServeHTTP(rr, req)

			// The plain HTTP router either returns 404 (route not registered)
			// or redirects to HTTPS (catch-all). Either way, the handler is
			// NOT reached, so no approval action is possible over plain HTTP.
			assert.True(t, rr.Code == http.StatusNotFound || rr.Code == http.StatusMovedPermanently || rr.Code == http.StatusTemporaryRedirect,
				"plain HTTP router must NOT reach the %s handler (got %d; expected 404 or redirect)", tc.name, rr.Code)
		})
	}
}

// TestPlatformEnrollmentRouter_OwnerRoutesReachableOnHTTPSWithOwner proves
// that pending and decision are registered on the HTTPS router and reachable
// when the active first user (the owner) presents a valid web session
// cookie. Pending returns 200 with an empty list; decision on a nonexistent
// request returns 404 (not 401/403), proving the handler was reached and
// the owner was authorized.
func TestPlatformEnrollmentRouter_OwnerRoutesReachableOnHTTPSWithOwner(t *testing.T) {
	env := setupPlatformEnrollmentRouterEnv(t)

	t.Run("pending returns 200 with owner cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthPlatformEnrollmentPending, nil)
		req.AddCookie(createWebSessionCookie(t, env, env.ownerID))
		rr := httptest.NewRecorder()

		env.httpsRouter.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code, "owner cookie must reach the pending handler")
		var resp models.PlatformEnrollmentPendingResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.Empty(t, resp.Requests, "no pending requests exist yet")
	})

	t.Run("decision reaches handler with owner cookie (404 for nonexistent)", func(t *testing.T) {
		body, err := json.Marshal(models.PlatformEnrollmentDecisionRequest{
			RequestID: "nonexistent-request-id",
			Decision:  models.PlatformEnrollmentDecisionApprove,
		})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPlatformEnrollmentDecision, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(createWebSessionCookie(t, env, env.ownerID))
		rr := httptest.NewRecorder()

		env.httpsRouter.ServeHTTP(rr, req)

		// 404 proves the handler was reached (owner authorized) and the
		// request was not found. A 401/403 would mean auth failed.
		assert.Equal(t, http.StatusNotFound, rr.Code, "owner cookie must reach the decision handler; nonexistent request returns 404")
	})
}

// ============================================================================
// Auth mode denial tests (HTTPS router)
// ============================================================================

// TestPlatformEnrollmentRouter_OwnerRoutesDenyMissingIdentity proves that
// pending and decision deny requests with no authentication context (no
// web session cookie, no mTLS certificate). The auth middleware returns 401.
func TestPlatformEnrollmentRouter_OwnerRoutesDenyMissingIdentity(t *testing.T) {
	env := setupPlatformEnrollmentRouterEnv(t)

	t.Run("pending denies missing identity", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthPlatformEnrollmentPending, nil)
		req.TLS = nil
		rr := httptest.NewRecorder()

		env.httpsRouter.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code, "pending must deny missing identity")
	})

	t.Run("decision denies missing identity", func(t *testing.T) {
		body, err := json.Marshal(models.PlatformEnrollmentDecisionRequest{
			RequestID: "any",
			Decision:  models.PlatformEnrollmentDecisionApprove,
		})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPlatformEnrollmentDecision, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.TLS = nil
		rr := httptest.NewRecorder()

		env.httpsRouter.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code, "decision must deny missing identity")
	})
}

// TestPlatformEnrollmentRouter_OwnerRoutesDenyNonOwner proves that pending
// and decision deny an authenticated but non-owner identity (an active
// second user). The auth middleware stamps the second user's ID in context;
// the controller's requireActiveFirstUser rejects with 403.
func TestPlatformEnrollmentRouter_OwnerRoutesDenyNonOwner(t *testing.T) {
	env := setupPlatformEnrollmentRouterEnv(t)

	secondUser, err := env.userSvc.CreateUser()
	require.NoError(t, err)
	require.NotEqual(t, env.ownerID, secondUser.ID, "second user must be a different user")

	t.Run("pending denies non-owner", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthPlatformEnrollmentPending, nil)
		req.AddCookie(createWebSessionCookie(t, env, secondUser.ID))
		rr := httptest.NewRecorder()

		env.httpsRouter.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code, "pending must deny non-owner")
	})

	t.Run("decision denies non-owner", func(t *testing.T) {
		body, err := json.Marshal(models.PlatformEnrollmentDecisionRequest{
			RequestID: "any",
			Decision:  models.PlatformEnrollmentDecisionApprove,
		})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPlatformEnrollmentDecision, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(createWebSessionCookie(t, env, secondUser.ID))
		rr := httptest.NewRecorder()

		env.httpsRouter.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code, "decision must deny non-owner")
	})
}

// TestPlatformEnrollmentRouter_OwnerRoutesDenyInactiveOwner proves that
// pending and decision deny an inactive (disabled) first user. The auth
// middleware's web session validation rejects disabled users with 401
// ("identity disabled"), providing defense-in-depth before the controller's
// own IsActive check.
func TestPlatformEnrollmentRouter_OwnerRoutesDenyInactiveOwner(t *testing.T) {
	env := setupPlatformEnrollmentRouterEnv(t)

	// Create a web session for the owner BEFORE disabling, so the session
	// exists but the user is inactive when the request is made.
	cookie := createWebSessionCookie(t, env, env.ownerID)

	require.NoError(t, env.userSvc.Disable(env.ownerID, "test-inactive", "test-actor", ""))

	t.Run("pending denies inactive owner", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthPlatformEnrollmentPending, nil)
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()

		env.httpsRouter.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code, "pending must deny inactive owner")
		assert.Contains(t, rr.Body.String(), "identity disabled",
			"middleware must reject the disabled user's session")
	})

	t.Run("decision denies inactive owner", func(t *testing.T) {
		body, err := json.Marshal(models.PlatformEnrollmentDecisionRequest{
			RequestID: "any",
			Decision:  models.PlatformEnrollmentDecisionApprove,
		})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPlatformEnrollmentDecision, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()

		env.httpsRouter.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code, "decision must deny inactive owner")
		assert.Contains(t, rr.Body.String(), "identity disabled",
			"middleware must reject the disabled user's session")
	})
}

// TestPlatformEnrollmentRouter_OwnerRoutesDenyRevokedSession proves that
// pending and decision deny an expired/revoked web session. The auth
// middleware's web session validation rejects expired sessions with 401
// ("web session expired").
func TestPlatformEnrollmentRouter_OwnerRoutesDenyRevokedSession(t *testing.T) {
	env := setupPlatformEnrollmentRouterEnv(t)

	cookie := createExpiredWebSessionCookie(t, env, env.ownerID)

	t.Run("pending denies expired session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthPlatformEnrollmentPending, nil)
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()

		env.httpsRouter.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code, "pending must deny expired session")
		assert.Contains(t, rr.Body.String(), "web session expired",
			"middleware must reject the expired session")
	})

	t.Run("decision denies expired session", func(t *testing.T) {
		body, err := json.Marshal(models.PlatformEnrollmentDecisionRequest{
			RequestID: "any",
			Decision:  models.PlatformEnrollmentDecisionApprove,
		})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPlatformEnrollmentDecision, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		rr := httptest.NewRecorder()

		env.httpsRouter.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code, "decision must deny expired session")
		assert.Contains(t, rr.Body.String(), "web session expired",
			"middleware must reject the expired session")
	})
}

// TestPlatformEnrollmentRouter_OwnerRoutesDenyAppIdentity proves that
// pending and decision deny an mTLS-authenticated app identity. The auth
// middleware's handleAppAuth path looks up the app policy; with no policy
// (deny-by-default), the request is rejected. An app identity must never
// reach the owner-only decision endpoint.
func TestPlatformEnrollmentRouter_OwnerRoutesDenyAppIdentity(t *testing.T) {
	env := setupPlatformEnrollmentRouterEnv(t)

	cert := appMTLSCert(t)

	t.Run("pending denies app identity", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthPlatformEnrollmentPending, nil)
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
		rr := httptest.NewRecorder()

		env.httpsRouter.ServeHTTP(rr, req)

		assert.True(t, rr.Code == http.StatusUnauthorized || rr.Code == http.StatusForbidden,
			"pending must deny app identity (got %d; expected 401 or 403)", rr.Code)
	})

	t.Run("decision denies app identity", func(t *testing.T) {
		body, err := json.Marshal(models.PlatformEnrollmentDecisionRequest{
			RequestID: "any",
			Decision:  models.PlatformEnrollmentDecisionApprove,
		})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPlatformEnrollmentDecision, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
		rr := httptest.NewRecorder()

		env.httpsRouter.ServeHTTP(rr, req)

		assert.True(t, rr.Code == http.StatusUnauthorized || rr.Code == http.StatusForbidden,
			"decision must deny app identity (got %d; expected 401 or 403)", rr.Code)
	})
}

// ============================================================================
// Full flow test through the router
// ============================================================================

// TestPlatformEnrollmentRouter_FullApproveFlowThroughHTTPS proves the
// complete enrollment flow through the HTTPS router: create a request,
// list pending (owner sees it), approve it (owner decides), and verify the
// request is no longer pending. This exercises the real auth middleware,
// real controller, real enrollment service, and real governance pipeline
// end-to-end through the router chain.
func TestPlatformEnrollmentRouter_FullApproveFlowThroughHTTPS(t *testing.T) {
	env := setupPlatformEnrollmentRouterEnv(t)

	// Generate a dashboard CSR and create a request via the HTTPS router
	// (RouteAuthNone — no auth needed).
	csr, _ := generateAppCSRAndKey(t)
	createBody, err := json.Marshal(models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-router-1",
		Hostname:      "dashboard-router.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPlatformEnrollmentRequest, bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.TLS = nil
	rr := httptest.NewRecorder()
	env.httpsRouter.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code, "request creation must succeed via HTTPS router")

	var createResp models.PlatformEnrollmentCreateResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &createResp))
	require.NotEmpty(t, createResp.RequestID)
	require.NotEmpty(t, createResp.Token)

	// List pending as the owner — the request must appear.
	pendingReq := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthPlatformEnrollmentPending, nil)
	pendingReq.AddCookie(createWebSessionCookie(t, env, env.ownerID))
	pendingRR := httptest.NewRecorder()
	env.httpsRouter.ServeHTTP(pendingRR, pendingReq)
	require.Equal(t, http.StatusOK, pendingRR.Code, "pending list must succeed with owner cookie")

	var pendingResp models.PlatformEnrollmentPendingResponse
	require.NoError(t, json.Unmarshal(pendingRR.Body.Bytes(), &pendingResp))
	require.Len(t, pendingResp.Requests, 1, "one pending request must be visible to the owner")
	assert.Equal(t, createResp.RequestID, pendingResp.Requests[0].RequestID)
	assert.Equal(t, models.PlatformComponentDashboard, pendingResp.Requests[0].ComponentKind)
	// The pending list must never expose the requester token.
	assert.NotContains(t, pendingRR.Body.String(), createResp.Token,
		"pending list must never expose the requester token")

	// Approve the request as the owner via the decision endpoint.
	decBody, err := json.Marshal(models.PlatformEnrollmentDecisionRequest{
		RequestID: createResp.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	require.NoError(t, err)
	decReq := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPlatformEnrollmentDecision, bytes.NewReader(decBody))
	decReq.Header.Set("Content-Type", "application/json")
	decReq.AddCookie(createWebSessionCookie(t, env, env.ownerID))
	decRR := httptest.NewRecorder()
	env.httpsRouter.ServeHTTP(decRR, decReq)
	require.Equal(t, http.StatusOK, decRR.Code, "decision must succeed with owner cookie")

	var decResp models.PlatformEnrollmentDecisionResponse
	require.NoError(t, json.Unmarshal(decRR.Body.Bytes(), &decResp))
	assert.Equal(t, createResp.RequestID, decResp.RequestID)
	assert.Equal(t, models.PlatformEnrollmentStateApproved, decResp.State)

	// Approved requests no longer await an owner decision and are excluded from the pending list.
	pendingReq2 := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthPlatformEnrollmentPending, nil)
	pendingReq2.AddCookie(createWebSessionCookie(t, env, env.ownerID))
	pendingRR2 := httptest.NewRecorder()
	env.httpsRouter.ServeHTTP(pendingRR2, pendingReq2)
	require.Equal(t, http.StatusOK, pendingRR2.Code)

	var pendingResp2 models.PlatformEnrollmentPendingResponse
	require.NoError(t, json.Unmarshal(pendingRR2.Body.Bytes(), &pendingResp2))
	assert.Empty(t, pendingResp2.Requests)
}

// TestPlatformEnrollmentRouter_StatusNoStoreHeader proves that the status
// endpoint sets Cache-Control: no-store, preventing intermediate caches from
// storing token-scoped responses.
func TestPlatformEnrollmentRouter_StatusNoStoreHeader(t *testing.T) {
	env := setupPlatformEnrollmentRouterEnv(t)

	// Create a request so we have a valid token to query status.
	csr, _ := generateAppCSRAndKey(t)
	createBody, _ := json.Marshal(models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-nostore-1",
		Hostname:      "dashboard-nostore.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	})
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPlatformEnrollmentRequest, bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	env.httpsRouter.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)
	var createResp models.PlatformEnrollmentCreateResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &createResp))

	// Query status and verify the no-store header.
	statusReq := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthPlatformEnrollmentStatus+"?token="+url.QueryEscape(createResp.Token), nil)
	statusRR := httptest.NewRecorder()
	env.httpsRouter.ServeHTTP(statusRR, statusReq)
	require.Equal(t, http.StatusOK, statusRR.Code)
	assert.Equal(t, "no-store", statusRR.Header().Get("Cache-Control"),
		"status endpoint must set Cache-Control: no-store")
}

// TestPlatformEnrollmentRouter_PendingNoStoreHeader proves that the pending
// endpoint sets Cache-Control: no-store, preventing intermediate caches from
// storing owner-visible metadata.
func TestPlatformEnrollmentRouter_PendingNoStoreHeader(t *testing.T) {
	env := setupPlatformEnrollmentRouterEnv(t)

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthPlatformEnrollmentPending, nil)
	req.AddCookie(createWebSessionCookie(t, env, env.ownerID))
	rr := httptest.NewRecorder()
	env.httpsRouter.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "no-store", rr.Header().Get("Cache-Control"),
		"pending endpoint must set Cache-Control: no-store")
}

// TestPlatformEnrollmentRouter_PreBootstrapRequestRejected proves that the
// request endpoint rejects enrollment requests before the gateway is
// bootstrapped (no users exist). This is invariant 1: a gateway with no users
// never issues a platform certificate. The test creates a fresh gateway
// without an owner and asserts the request is rejected with 403.
func TestPlatformEnrollmentRouter_PreBootstrapRequestRejected(t *testing.T) {
	// Build a gateway with NO owner (not bootstrapped).
	env := setupPlatformEnrollmentEnv(t, false)
	h := env.svc.GetHTTPHandler()
	require.NotNil(t, h)
	httpsRouter := h.buildPublicRouter()

	csr, _ := generateAppCSRAndKey(t)
	body, err := json.Marshal(models.PlatformEnrollmentCreateRequest{
		ComponentKind: models.PlatformComponentDashboard,
		InstanceID:    "dashboard-pre-bootstrap-1",
		Hostname:      "dashboard-pre-bootstrap.local",
		App:           &models.PlatformAppCSRPayload{CSRPEM: csr},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPlatformEnrollmentRequest, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	httpsRouter.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code,
		"pre-bootstrap request must be rejected with 403 (invariant 1)")
	assert.Contains(t, rr.Body.String(), constants.ErrPlatformEnrollmentRequiresBootstrap.Error(),
		"the typed bootstrap-required error must be returned")
}

// TestPlatformEnrollmentRouter_CSRValidationRejectsInvalidBody proves that
// the request endpoint validates the request body and rejects invalid JSON
// with 400, proving the handler was reached (not blocked by auth) and
// performs input validation.
func TestPlatformEnrollmentRouter_CSRValidationRejectsInvalidBody(t *testing.T) {
	env := setupPlatformEnrollmentRouterEnv(t)

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPlatformEnrollmentRequest, bytes.NewReader([]byte("not json")))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		env.httpsRouter.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code, "invalid JSON must return 400")
	})

	t.Run("invalid component kind returns 400", func(t *testing.T) {
		body, err := json.Marshal(map[string]interface{}{
			"component_kind": "unknown",
			"instance_id":    "x",
			"hostname":       "x.local",
			"app":            map[string]string{"csr_pem": "invalid"},
		})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthPlatformEnrollmentRequest, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		env.httpsRouter.ServeHTTP(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code, "invalid component kind must return 400")
	})
}

// TestPlatformEnrollmentRouter_MethodEnforcement proves that each route
// rejects the wrong HTTP method with 405 Method Not Allowed.
func TestPlatformEnrollmentRouter_MethodEnforcement(t *testing.T) {
	env := setupPlatformEnrollmentRouterEnv(t)

	cases := []struct {
		name        string
		path        string
		wrongMethod string
	}{
		{"request rejects GET", constants.APIPaths.AuthPlatformEnrollmentRequest, http.MethodGet},
		{"status rejects POST", constants.APIPaths.AuthPlatformEnrollmentStatus, http.MethodPost},
		{"complete rejects GET", constants.APIPaths.AuthPlatformEnrollmentComplete, http.MethodGet},
		{"pending rejects POST", constants.APIPaths.AuthPlatformEnrollmentPending, http.MethodPost},
		{"decision rejects GET", constants.APIPaths.AuthPlatformEnrollmentDecision, http.MethodGet},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.wrongMethod, tc.path, nil)
			// Add owner cookie for pending/decision so we get past auth
			// and reach the method check.
			req.AddCookie(createWebSessionCookie(t, env, env.ownerID))
			rr := httptest.NewRecorder()
			env.httpsRouter.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusMethodNotAllowed, rr.Code,
				"wrong method must return 405")
		})
	}
}
