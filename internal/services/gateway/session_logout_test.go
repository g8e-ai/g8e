// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func newLogoutService(infra *TestInfrastructure, pki cliCertificateRevoker) *SessionLogoutService {
	return NewSessionLogoutService(SessionLogoutServiceDeps{
		Logger:      infra.Logger,
		CLISessions: infra.CLISessionSvc,
		WebSessions: infra.WebSessionSvc,
		Reg:         infra.Reg,
		PKI:         pki,
	})
}

func newLogoutUser(t *testing.T, infra *TestInfrastructure) *models.User {
	t.Helper()
	user, err := infra.UserSvc.CreateUser()
	require.NoError(t, err)
	return user
}

// persistLogoutOperator stores an operator document with an active operator
// session, owned by userID.
func persistLogoutOperator(t *testing.T, infra *TestInfrastructure, userID, operatorID, operatorSessionID string) {
	t.Helper()
	now := time.Now().UTC()
	b, err := models.MarshalOperatorDocument(&operatorv1.OperatorDocument{
		Id:                operatorID,
		OperatorSessionId: operatorSessionID,
		Status:            string(constants.OperatorStatusActive),
		UserId:            userID,
		OperatorType:      string(constants.OperatorTypeRemote),
		CreatedAt:         timestamppb.New(now),
		UpdatedAt:         timestamppb.New(now),
	})
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(marshaler.CollectionName(constants.CollectionOperators), operatorID, b))
}

// bindLogoutOperatorToWeb persists an operator and binds it to a fresh web
// session of userID through the production bind path.
func bindLogoutOperatorToWeb(t *testing.T, infra *TestInfrastructure, userID, operatorID, operatorSessionID string) string {
	t.Helper()
	persistLogoutOperator(t, infra, userID, operatorID, operatorSessionID)
	web, err := infra.WebSessionSvc.CreateWebSession(userID)
	require.NoError(t, err)
	res, err := infra.Reg.BindOperators(models.BindOperatorsRequest{
		OperatorIDs:  []string{operatorID},
		UserID:       userID,
		WebSessionID: web.ID,
	})
	require.NoError(t, err)
	require.Equal(t, 1, res.BoundCount)
	return web.ID
}

type logoutCLISessionSpec struct {
	id        string
	serial    string
	bound     []string
	inactive  bool
	expiredAt time.Time
}

func persistLogoutCLISession(t *testing.T, infra *TestInfrastructure, userID string, spec logoutCLISessionSpec) {
	t.Helper()
	expires := spec.expiredAt
	if expires.IsZero() {
		expires = time.Now().UTC().Add(time.Hour)
	}
	primary := ""
	if len(spec.bound) > 0 {
		primary = spec.bound[0]
	}
	b, err := json.Marshal(models.CLISession{
		ID:                      spec.id,
		UserID:                  userID,
		OperatorSessionID:       primary,
		BoundOperatorSessionIDs: spec.bound,
		CertFingerprint:         "fp-" + spec.id,
		CertSerial:              spec.serial,
		CreatedAt:               time.Now().UTC().Add(-time.Minute),
		ExpiresAt:               expires,
		AbsoluteExpiresAt:       expires,
		IdleExpiresAt:           expires,
		SessionType:             string(constants.SessionTypeCLI),
		IsActive:                !spec.inactive,
	})
	require.NoError(t, err)
	require.NoError(t, infra.DocStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), spec.id, b))
}

func loadLogoutCLISession(t *testing.T, infra *TestInfrastructure, id string) *models.CLISession {
	t.Helper()
	session, err := infra.CLISessionSvc.loadCLISession(id)
	require.NoError(t, err)
	return session
}

func requireWebSessionGone(t *testing.T, infra *TestInfrastructure, id string) {
	t.Helper()
	doc, err := infra.DocStore.DocGet(marshaler.CollectionName(constants.CollectionWebSessions), id)
	require.NoError(t, err)
	assert.Nil(t, doc, "web session %s should be deleted", id)
}

func requireWebSessionPresent(t *testing.T, infra *TestInfrastructure, id string) {
	t.Helper()
	doc, err := infra.DocStore.DocGet(marshaler.CollectionName(constants.CollectionWebSessions), id)
	require.NoError(t, err)
	assert.NotNil(t, doc, "web session %s should remain", id)
}

func operatorBoundWebSession(t *testing.T, infra *TestInfrastructure, operatorID string) string {
	t.Helper()
	op, err := infra.Reg.GetOperator(operatorID)
	require.NoError(t, err)
	return op.BoundWebSessionId
}

func revocationReason(t *testing.T, infra *TestInfrastructure, serial string) string {
	t.Helper()
	doc, err := infra.DocStore.DocGet(marshaler.CollectionName(constants.CollectionRevokedCertificates), serial)
	require.NoError(t, err)
	require.NotNil(t, doc, "serial %s should be revoked", serial)
	var rev revocationDocument
	wire, err := json.Marshal(doc.ForWire())
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(wire, &rev))
	return rev.Reason
}

func TestSessionLogout_All_TerminatesWebAndCLISessionsUnbindsOperatorsAndRevokesCerts(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	svc := newLogoutService(infra, infra.PKI)
	user := newLogoutUser(t, infra)
	other := newLogoutUser(t, infra)

	webID := bindLogoutOperatorToWeb(t, infra, user.ID, "logout-op-1", "logout-opsess-1")
	otherWebID := bindLogoutOperatorToWeb(t, infra, other.ID, "logout-op-other", "logout-opsess-other")
	persistLogoutCLISession(t, infra, user.ID, logoutCLISessionSpec{id: "logout-cli-a", serial: "7001", bound: []string{"logout-opsess-1", "logout-opsess-2"}})
	persistLogoutCLISession(t, infra, user.ID, logoutCLISessionSpec{id: "logout-cli-current", serial: "7002", bound: []string{"logout-opsess-1"}})
	// Replaced earlier by refresh: inactive, but its certificate is the one the
	// active session still carries.
	persistLogoutCLISession(t, infra, user.ID, logoutCLISessionSpec{id: "logout-cli-old", serial: "7002", bound: []string{"logout-opsess-1"}, inactive: true})
	persistLogoutCLISession(t, infra, other.ID, logoutCLISessionSpec{id: "logout-cli-other", serial: "7099", bound: []string{"logout-opsess-other"}})

	result, err := svc.Logout(user.ID, "logout-cli-current", "", constants.LogoutScopeAll)
	require.NoError(t, err)

	assert.Equal(t, 1, result.WebSessionsTerminated)
	assert.Equal(t, 2, result.CLISessionsTerminated)
	assert.Equal(t, 2, result.CertificatesRevoked, "serials 7001 and 7002, each once")
	assert.Equal(t, []string{"logout-opsess-1", "logout-opsess-2"}, result.UnboundOperatorSessionIDs)

	requireWebSessionGone(t, infra, webID)
	assert.Empty(t, operatorBoundWebSession(t, infra, "logout-op-1"), "operator document must no longer name the web session")
	kvRaw, found := infra.KVStore.KVGet(sessionWebBindKey(webID))
	assert.False(t, found, "web bind KV entry should be gone, got %q", kvRaw)

	for _, id := range []string{"logout-cli-a", "logout-cli-current"} {
		session := loadLogoutCLISession(t, infra, id)
		assert.False(t, session.IsActive, "%s should be deactivated", id)
		assert.Empty(t, session.OperatorSessionID, "%s primary binding should be cleared", id)
		assert.Empty(t, session.BoundOperatorSessionIDs, "%s bound list should be cleared", id)
	}
	for _, serial := range []string{"7001", "7002"} {
		assert.Equal(t, constants.CLILogoutCertRevocationReason, revocationReason(t, infra, serial))
	}

	// The operator itself keeps its session: logout only unbinds.
	op, err := infra.Reg.GetOperator("logout-op-1")
	require.NoError(t, err)
	assert.Equal(t, "logout-opsess-1", op.OperatorSessionId)
	assert.Equal(t, string(constants.OperatorStatusActive), op.Status)

	// Another user's sessions, bindings and certificates are untouched.
	requireWebSessionPresent(t, infra, otherWebID)
	assert.Equal(t, otherWebID, operatorBoundWebSession(t, infra, "logout-op-other"))
	otherSession := loadLogoutCLISession(t, infra, "logout-cli-other")
	assert.True(t, otherSession.IsActive)
	assert.Equal(t, "logout-opsess-other", otherSession.OperatorSessionID)
	revoked, err := infra.PKI.IsRevoked("7099")
	require.NoError(t, err)
	assert.False(t, revoked)
}

func TestSessionLogout_WebScope_LeavesCLISessionsAndCertsAlone(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	svc := newLogoutService(infra, infra.PKI)
	user := newLogoutUser(t, infra)

	webID := bindLogoutOperatorToWeb(t, infra, user.ID, "logout-web-op", "logout-web-opsess")
	persistLogoutCLISession(t, infra, user.ID, logoutCLISessionSpec{id: "logout-web-cli", serial: "7101", bound: []string{"logout-web-opsess"}})

	result, err := svc.Logout(user.ID, "logout-web-cli", "", constants.LogoutScopeWeb)
	require.NoError(t, err)

	assert.Equal(t, 1, result.WebSessionsTerminated)
	assert.Zero(t, result.CLISessionsTerminated)
	assert.Zero(t, result.CertificatesRevoked)
	assert.Equal(t, []string{"logout-web-opsess"}, result.UnboundOperatorSessionIDs)

	requireWebSessionGone(t, infra, webID)
	session := loadLogoutCLISession(t, infra, "logout-web-cli")
	assert.True(t, session.IsActive)
	assert.Equal(t, "logout-web-opsess", session.OperatorSessionID, "CLI binding survives a web-only logout")
	revoked, err := infra.PKI.IsRevoked("7101")
	require.NoError(t, err)
	assert.False(t, revoked)
}

func TestSessionLogout_CLIScope_LeavesWebSessionsAndWebBindingsAlone(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	svc := newLogoutService(infra, infra.PKI)
	user := newLogoutUser(t, infra)

	webID := bindLogoutOperatorToWeb(t, infra, user.ID, "logout-cli-op", "logout-cli-opsess")
	persistLogoutCLISession(t, infra, user.ID, logoutCLISessionSpec{id: "logout-cli-only", serial: "7201", bound: []string{"logout-cli-opsess"}})

	result, err := svc.Logout(user.ID, "logout-cli-only", "", constants.LogoutScopeCLI)
	require.NoError(t, err)

	assert.Zero(t, result.WebSessionsTerminated)
	assert.Equal(t, 1, result.CLISessionsTerminated)
	assert.Equal(t, 1, result.CertificatesRevoked)
	assert.Equal(t, []string{"logout-cli-opsess"}, result.UnboundOperatorSessionIDs)

	requireWebSessionPresent(t, infra, webID)
	assert.Equal(t, webID, operatorBoundWebSession(t, infra, "logout-cli-op"), "web binding survives a CLI-only logout")
	assert.False(t, loadLogoutCLISession(t, infra, "logout-cli-only").IsActive)
}

func TestSessionLogout_RepeatedLogoutIsIdempotentAndKeepsEarlierRevocation(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	svc := newLogoutService(infra, infra.PKI)
	user := newLogoutUser(t, infra)

	persistLogoutCLISession(t, infra, user.ID, logoutCLISessionSpec{id: "logout-idem-a", serial: "7301", bound: []string{"logout-idem-op"}})
	persistLogoutCLISession(t, infra, user.ID, logoutCLISessionSpec{id: "logout-idem-b", serial: "7302", inactive: true})
	require.NoError(t, infra.PKI.RevokeCertificate("7302", "cli_rotation"))

	first, err := svc.Logout(user.ID, "logout-idem-a", "", constants.LogoutScopeAll)
	require.NoError(t, err)
	assert.Equal(t, 1, first.CLISessionsTerminated)
	assert.Equal(t, 1, first.CertificatesRevoked, "7302 was already revoked and must be skipped")

	second, err := svc.Logout(user.ID, "logout-idem-a", "", constants.LogoutScopeAll)
	require.NoError(t, err)
	assert.Zero(t, second.CLISessionsTerminated)
	assert.Zero(t, second.CertificatesRevoked)
	assert.Empty(t, second.UnboundOperatorSessionIDs)

	assert.Equal(t, constants.CLILogoutCertRevocationReason, revocationReason(t, infra, "7301"))
	assert.Equal(t, "cli_rotation", revocationReason(t, infra, "7302"), "an earlier revocation reason must not be overwritten")
}

func TestSessionLogout_ExpiredActiveSession_IsTerminatedUnreportedAndItsCertRevoked(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	svc := newLogoutService(infra, infra.PKI)
	user := newLogoutUser(t, infra)

	// is_active but past expiry: exactly the state `auth refresh` would revive
	// from a still-valid certificate.
	persistLogoutCLISession(t, infra, user.ID, logoutCLISessionSpec{
		id: "logout-stale", serial: "7401", bound: []string{"logout-stale-opsess"}, expiredAt: time.Now().UTC().Add(-time.Hour),
	})

	result, err := svc.Logout(user.ID, "", "", constants.LogoutScopeCLI)
	require.NoError(t, err)

	assert.Zero(t, result.CLISessionsTerminated, "an already-expired session is not reported as a logout")
	assert.Empty(t, result.UnboundOperatorSessionIDs)
	assert.Equal(t, 1, result.CertificatesRevoked)
	assert.False(t, loadLogoutCLISession(t, infra, "logout-stale").IsActive)
}

func TestSessionLogout_RevokesPresentedCertificateEvenWithoutSessionDocument(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	svc := newLogoutService(infra, infra.PKI)
	user := newLogoutUser(t, infra)

	result, err := svc.Logout(user.ID, "missing-session", "7501", constants.LogoutScopeCLI)
	require.NoError(t, err)

	assert.Equal(t, 1, result.CertificatesRevoked)
	assert.Equal(t, constants.CLILogoutCertRevocationReason, revocationReason(t, infra, "7501"))
}

func TestSessionLogout_RejectsInvalidScopeAndMissingUser(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	svc := newLogoutService(infra, infra.PKI)
	user := newLogoutUser(t, infra)

	_, err := svc.Logout(user.ID, "", "", constants.LogoutScope("everything"))
	require.ErrorIs(t, err, constants.ErrLogoutScopeInvalid)

	_, err = svc.Logout("", "", "", constants.LogoutScopeAll)
	require.ErrorIs(t, err, constants.ErrRegistrationUserIDRequired)
}

type failingRevoker struct {
	isRevokedErr error
	revokeErr    error
	revoked      []string
}

func (f *failingRevoker) IsRevoked(string) (bool, error) { return false, f.isRevokedErr }

func (f *failingRevoker) RevokeCertificate(serial, _ string) error {
	if f.revokeErr != nil {
		return f.revokeErr
	}
	f.revoked = append(f.revoked, serial)
	return nil
}

func TestSessionLogout_RevocationFailureIsReportedAfterTryingEverySerial(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	user := newLogoutUser(t, infra)
	persistLogoutCLISession(t, infra, user.ID, logoutCLISessionSpec{id: "logout-fail-a", serial: "7601", bound: []string{"logout-fail-op"}})
	persistLogoutCLISession(t, infra, user.ID, logoutCLISessionSpec{id: "logout-fail-b", serial: "7602"})

	boom := errors.New("crl store unavailable")
	svc := newLogoutService(infra, &failingRevoker{revokeErr: boom})

	result, err := svc.Logout(user.ID, "logout-fail-a", "", constants.LogoutScopeCLI)

	require.ErrorIs(t, err, boom)
	require.NotNil(t, result, "the partial result is returned with the error")
	assert.Equal(t, 2, result.CLISessionsTerminated, "sessions are already terminated when revocation fails")
	assert.Zero(t, result.CertificatesRevoked)
	assert.Contains(t, err.Error(), "7601")
	assert.Contains(t, err.Error(), "7602", "every serial is attempted, not just the first")
}

func TestSessionLogout_RevocationCheckFailureIsReported(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	user := newLogoutUser(t, infra)
	persistLogoutCLISession(t, infra, user.ID, logoutCLISessionSpec{id: "logout-check-fail", serial: "7701"})

	boom := errors.New("revocation lookup failed")
	revoker := &failingRevoker{isRevokedErr: boom}
	svc := newLogoutService(infra, revoker)

	_, err := svc.Logout(user.ID, "logout-check-fail", "", constants.LogoutScopeCLI)

	require.ErrorIs(t, err, boom)
	assert.Empty(t, revoker.revoked, "a serial whose status could not be checked is not revoked blindly")
}

func TestSessionLogout_CLIScopeWithoutPKIFailsClosed(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	user := newLogoutUser(t, infra)
	persistLogoutCLISession(t, infra, user.ID, logoutCLISessionSpec{id: "logout-nopki", serial: "7801"})
	svc := newLogoutService(infra, nil)

	_, err := svc.Logout(user.ID, "logout-nopki", "", constants.LogoutScopeCLI)

	require.ErrorIs(t, err, constants.ErrPKIDatabaseNotAvailable)
}

// --- handler -------------------------------------------------------------

func setupLogoutController(t *testing.T) (*CLIRefreshController, *TestInfrastructure, *models.User) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)
	c := newCLIRefreshController(CLIRefreshControllerDeps{
		Cfg:                infra.Cfg,
		Logger:             infra.Logger,
		CLISessionSvc:      infra.CLISessionSvc,
		OperatorSessionSvc: infra.OperatorSessionSvc,
		Reg:                infra.Reg,
		Auth:               infra.Auth,
		UserSvc:            infra.UserSvc,
		LogoutSvc:          newLogoutService(infra, infra.PKI),
		Responder:          infra.Responder,
	})
	return c, infra, newLogoutUser(t, infra)
}

func logoutRequest(t *testing.T, method, body, userID, cliSessionID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, constants.APIPaths.AuthCLILogout, bytes.NewReader([]byte(body)))
	ctx := req.Context()
	if userID != "" {
		ctx = context.WithValue(ctx, constants.ContextKeyUserID, userID)
	}
	ctx = context.WithValue(ctx, constants.ContextKeyCLISessionID, cliSessionID)
	return req.WithContext(ctx)
}

func TestCLIRefreshController_Logout_EmptyBodyDefaultsToAllScope(t *testing.T) {
	c, infra, user := setupLogoutController(t)
	webID := bindLogoutOperatorToWeb(t, infra, user.ID, "logout-h-op", "logout-h-opsess")
	persistLogoutCLISession(t, infra, user.ID, logoutCLISessionSpec{id: "logout-h-cli", serial: "7901", bound: []string{"logout-h-opsess"}})

	rr := httptest.NewRecorder()
	c.handleLogout(rr, logoutRequest(t, http.MethodPost, "", user.ID, "logout-h-cli"))

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp models.CLILogoutResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, user.ID, resp.UserID)
	assert.Equal(t, constants.LogoutScopeAll, resp.Scope)
	assert.Equal(t, 1, resp.WebSessionsTerminated)
	assert.Equal(t, 1, resp.CLISessionsTerminated)
	assert.Equal(t, 1, resp.CLICertificatesRevoked)
	assert.Equal(t, []string{"logout-h-opsess"}, resp.UnboundOperatorSessionIDs)
	requireWebSessionGone(t, infra, webID)
}

func TestCLIRefreshController_Logout_HonorsRequestedScope(t *testing.T) {
	c, infra, user := setupLogoutController(t)
	webID := bindLogoutOperatorToWeb(t, infra, user.ID, "logout-hs-op", "logout-hs-opsess")
	persistLogoutCLISession(t, infra, user.ID, logoutCLISessionSpec{id: "logout-hs-cli", serial: "7911", bound: []string{"logout-hs-opsess"}})

	rr := httptest.NewRecorder()
	c.handleLogout(rr, logoutRequest(t, http.MethodPost, `{"scope":"web"}`, user.ID, "logout-hs-cli"))

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp models.CLILogoutResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, constants.LogoutScopeWeb, resp.Scope)
	assert.Equal(t, 1, resp.WebSessionsTerminated)
	assert.Zero(t, resp.CLISessionsTerminated)
	requireWebSessionGone(t, infra, webID)
	assert.True(t, loadLogoutCLISession(t, infra, "logout-hs-cli").IsActive)
}

func TestCLIRefreshController_Logout_RejectsBadRequests(t *testing.T) {
	c, _, user := setupLogoutController(t)

	tests := []struct {
		name   string
		method string
		body   string
		user   string
		status int
	}{
		{name: "wrong method", method: http.MethodGet, body: "", user: user.ID, status: http.StatusMethodNotAllowed},
		{name: "no authenticated user", method: http.MethodPost, body: "", user: "", status: http.StatusUnauthorized},
		{name: "malformed JSON", method: http.MethodPost, body: "{", user: user.ID, status: http.StatusBadRequest},
		{name: "unknown scope", method: http.MethodPost, body: `{"scope":"everything"}`, user: user.ID, status: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			c.handleLogout(rr, logoutRequest(t, tt.method, tt.body, tt.user, "any-session"))
			assert.Equal(t, tt.status, rr.Code, rr.Body.String())
		})
	}
}

func TestCLIRefreshController_Logout_ServiceFailureIsServerError(t *testing.T) {
	c, infra, user := setupLogoutController(t)
	persistLogoutCLISession(t, infra, user.ID, logoutCLISessionSpec{id: "logout-500", serial: "7921"})
	c.logoutSvc = newLogoutService(infra, &failingRevoker{revokeErr: errors.New("boom")})

	rr := httptest.NewRecorder()
	c.handleLogout(rr, logoutRequest(t, http.MethodPost, `{"scope":"cli"}`, user.ID, "logout-500"))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.NotContains(t, rr.Body.String(), "boom", "internal error text must not leak to the client")
}

// --- routing and admission ----------------------------------------------

func TestRouteAuthRegistry_CLILogoutIsMTLS(t *testing.T) {
	for _, jwks := range []bool{false, true} {
		assert.Equal(t, RouteAuthMTLS, NewRouteAuthRegistry(jwks).AuthMode(constants.APIPaths.AuthCLILogout))
	}
}

func TestHandleCLIAuth_LogoutIsAdmittedOnCertificateAlone(t *testing.T) {
	t.Run("expired session", func(t *testing.T) {
		_, middleware, userID, cliSessionID := setupRefreshAuthTestInfra(t, "logout-auth-expired", true, "cert-fp")
		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, cliRefreshMTLSRequest(t, constants.APIPaths.AuthCLILogout, cliSessionID, userID))
		assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, userID, rr.Header().Get("X-Stamped-User-ID"))
	})

	t.Run("missing session", func(t *testing.T) {
		infra := setupTestInfrastructure(t, false)
		user := newLogoutUser(t, infra)
		middleware := infra.Auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, cliRefreshMTLSRequest(t, constants.APIPaths.AuthCLILogout, "gone-session", user.ID))
		assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	})

	t.Run("certificate for a different session is rejected", func(t *testing.T) {
		_, middleware, userID, cliSessionID := setupRefreshAuthTestInfra(t, "logout-auth-mismatch", true, "cert-fp")
		req := cliRefreshMTLSRequest(t, constants.APIPaths.AuthCLILogout, cliSessionID, userID)
		req.Header.Set(constants.HeaderCLISessionID, "some-other-session")
		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})
}
