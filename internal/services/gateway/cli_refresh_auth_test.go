// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/g8e-ai/g8e/protocol"
)

// setupRefreshAuthTestInfra builds an AuthService backed by a real doc store
// with an active user and (optionally) a CLI session, returning the auth
// service, the user ID, and the CLI session ID. The middleware is constructed
// over a success-recording handler so tests can assert admission vs rejection.
func setupRefreshAuthTestInfra(t *testing.T, sessionID string, expired bool) (auth *AuthService, middleware http.Handler, userID, cliSessionID string) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)
	auth = infra.Auth

	user, err := infra.UserSvc.CreateUser()
	require.NoError(t, err)
	userID = user.ID

	if sessionID != "" {
		cliSessionID = sessionID
		expiresAt := time.Now().Add(1 * time.Hour)
		if expired {
			expiresAt = time.Now().Add(-1 * time.Hour)
		}
		doc := models.CLISession{
			ID:                cliSessionID,
			UserID:            userID,
			OperatorSessionID: "op-refresh-auth",
			SystemFingerprint: "sys-fp",
			CertFingerprint:   "cert-fp",
			CertSerial:        "serial",
			CreatedAt:         time.Now().Add(-2 * time.Hour),
			ExpiresAt:         expiresAt,
			AbsoluteExpiresAt: expiresAt,
			IdleExpiresAt:     expiresAt,
			SessionType:       string(constants.SessionTypeCLI),
			IsActive:          true,
			LoginMethod:       "mTLS",
		}
		b, err := json.Marshal(doc)
		require.NoError(t, err)
		require.NoError(t, infra.Stores.DocStore.DocSet(
			marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, b,
		))
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Record the stamped user ID so tests can assert the cert-derived
		// identity reached the handler.
		uid, _ := r.Context().Value(constants.ContextKeyUserID).(string)
		w.Header().Set("X-Stamped-User-ID", uid)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("admitted"))
	})
	middleware = auth.Middleware(handler)
	return auth, middleware, userID, cliSessionID
}

// cliRefreshMTLSRequest builds a request to the refresh endpoint (or a
// non-refresh endpoint when path is overridden) with the given CLI session
// ID header and a cert whose URI SAN is the CLI SPIFFE ID for the given
// user/session. This mirrors what handleMTLSAuth sees from a real mTLS
// handshake.
func cliRefreshMTLSRequest(t *testing.T, path, cliSessionID, userID string) *http.Request {
	t.Helper()
	wid := protocol.NewWorkloadIdentity()
	cliURI, err := wid.CLISPIFFEURL(userID, cliSessionID)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set(constants.HeaderCLISessionID, cliSessionID)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{
			{URIs: []*url.URL{cliURI}},
		},
	}
	return req
}

// TestHandleCLIRefreshAuth_CertURISANMatchesSessionID_Admitted verifies the
// primary recovery path through the unified auth middleware: the CLI session
// is expired, the request targets the refresh endpoint, the cert URI SAN
// session ID matches the header-provided session ID, and the user is active.
// The middleware admits the request to the refresh controller.
func TestHandleCLIRefreshAuth_CertURISANMatchesSessionID_Admitted(t *testing.T) {
	_, middleware, userID, cliSessionID := setupRefreshAuthTestInfra(t, "refresh-auth-expired", true)

	req := cliRefreshMTLSRequest(t, constants.APIPaths.AuthCLIRefresh, cliSessionID, userID)
	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "refresh endpoint should admit an expired session with a valid cert, body: %s", rr.Body.String())
	assert.Equal(t, "admitted", rr.Body.String())
	assert.Equal(t, userID, rr.Header().Get("X-Stamped-User-ID"), "user ID must be stamped from the cert URI SAN")
}

// TestHandleCLIRefreshAuth_CertURISANDoesNotMatchSessionID_Rejected verifies
// that a cert whose URI SAN session ID does not match the header-provided
// session ID is rejected with 403 ErrMTLSIdentityMismatch. This is the
// fail-closed guard against a cert trying to refresh a different session.
func TestHandleCLIRefreshAuth_CertURISANDoesNotMatchSessionID_Rejected(t *testing.T) {
	_, middleware, userID, cliSessionID := setupRefreshAuthTestInfra(t, "refresh-auth-mismatch", true)

	// Build a cert whose URI SAN session ID is different from the header.
	wid := protocol.NewWorkloadIdentity()
	wrongURI, err := wid.CLISPIFFEURL(userID, "different-session-id")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthCLIRefresh, nil)
	req.Header.Set(constants.HeaderCLISessionID, cliSessionID)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{
			{URIs: []*url.URL{wrongURI}},
		},
	}
	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrMTLSIdentityMismatch.Error())
}

// TestHandleCLIRefreshAuth_UserDisabled_Rejected verifies that a disabled
// user is rejected even on the refresh endpoint. An expired session does
// not bypass user-disabled checks — the cert proves identity, but the user
// must still be active to receive a new session.
func TestHandleCLIRefreshAuth_UserDisabled_Rejected(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	auth := infra.Auth

	user, err := infra.UserSvc.CreateUser()
	require.NoError(t, err)

	cliSessionID := "refresh-auth-disabled"
	expiresAt := time.Now().Add(-1 * time.Hour)
	doc := models.CLISession{
		ID:                cliSessionID,
		UserID:            user.ID,
		OperatorSessionID: "op-disabled",
		CreatedAt:         time.Now().Add(-2 * time.Hour),
		ExpiresAt:         expiresAt,
		AbsoluteExpiresAt: expiresAt,
		IdleExpiresAt:     expiresAt,
		SessionType:       string(constants.SessionTypeCLI),
		IsActive:          true,
		LoginMethod:       "mTLS",
	}
	b, err := json.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, infra.Stores.DocStore.DocSet(
		marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, b,
	))

	// Disable the user after the session was created.
	require.NoError(t, infra.UserSvc.Disable(user.ID, "test", "actor", "op"))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := auth.Middleware(handler)

	req := cliRefreshMTLSRequest(t, constants.APIPaths.AuthCLIRefresh, cliSessionID, user.ID)
	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	assert.NotEqual(t, http.StatusOK, rr.Code, "disabled user must not be admitted to the refresh endpoint")
}

// TestHandleCLIAuth_ExpiredSessionOnNonRefreshEndpoint_FailClosed verifies
// that an expired CLI session on a non-refresh endpoint is rejected with
// 401 ErrCLISessionExpired. Only the refresh endpoint bypasses the expired-
// session check; all other endpoints fail closed.
func TestHandleCLIAuth_ExpiredSessionOnNonRefreshEndpoint_FailClosed(t *testing.T) {
	_, middleware, userID, cliSessionID := setupRefreshAuthTestInfra(t, "refresh-auth-nonrefresh-expired", true)

	// Target a non-refresh mTLS endpoint (e.g., the audit receipts list).
	req := cliRefreshMTLSRequest(t, constants.APIPaths.AuditReceipts, cliSessionID, userID)
	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrCLISessionExpired.Error())
}

// TestHandleCLIAuth_MissingSessionOnNonRefreshEndpoint_FailClosed verifies
// that a missing CLI session (no persisted document for the header-provided
// session ID) on a non-refresh endpoint is rejected with 401
// ErrCLISessionInvalid. Only the refresh endpoint bypasses the missing-
// session check; all other endpoints fail closed.
func TestHandleCLIAuth_MissingSessionOnNonRefreshEndpoint_FailClosed(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	auth := infra.Auth

	user, err := infra.UserSvc.CreateUser()
	require.NoError(t, err)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	middleware := auth.Middleware(handler)

	// No session persisted — use a random session ID that doesn't exist.
	req := cliRefreshMTLSRequest(t, constants.APIPaths.AuditReceipts, "nonexistent-session-id", user.ID)
	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrCLISessionInvalid.Error())
}

// TestHandleCLIRefreshAuth_MissingSessionOnRefreshEndpoint_Admitted verifies
// the gateway-volume-reset case through the middleware: the session ID from
// the cert URI SAN does not match any persisted session, but the request
// targets the refresh endpoint, the cert is valid, and the user is active.
// The middleware admits the request so the controller can issue a new
// session.
func TestHandleCLIRefreshAuth_MissingSessionOnRefreshEndpoint_Admitted(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	auth := infra.Auth

	user, err := infra.UserSvc.CreateUser()
	require.NoError(t, err)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := r.Context().Value(constants.ContextKeyUserID).(string)
		w.Header().Set("X-Stamped-User-ID", uid)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("admitted"))
	})
	middleware := auth.Middleware(handler)

	// No session persisted — simulate a volume reset.
	req := cliRefreshMTLSRequest(t, constants.APIPaths.AuthCLIRefresh, "missing-session-id", user.ID)
	rr := httptest.NewRecorder()
	middleware.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, "refresh endpoint should admit a missing session with a valid cert, body: %s", rr.Body.String())
	assert.Equal(t, "admitted", rr.Body.String())
	assert.Equal(t, user.ID, rr.Header().Get("X-Stamped-User-ID"))
}

// Suppress unused import warning for testutil when only a subset of helpers
// is used in a given build configuration.
var _ = testutil.NewTestLogger
