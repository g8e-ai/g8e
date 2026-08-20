// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

// Phase 0 regression tests for the platform-activation-approval-sequence plan.
//
// These tests demonstrate the certificate-issuance bypasses that the
// owner-approved platform enrollment protocol must close. Each test asserts
// the CURRENT (insecure) behavior so the Phase 4 rip-and-replace can flip
// these expectations to fail-closed and prove the bypasses are gone. The
// tests live in the gateway package because they exercise real PKI, real
// SQLite, and real handler code paths — they are not unit tests.
//
// Bypasses covered:
//   1. Pre-activation app issuance: POST /api/v1/pki/apps/enroll issues a
//      long-lived app certificate (including the reserved dashboard name
//      "g8ed") before any human has activated the gateway.
//   2. Post-activation direct operator issuance: POST
//      /api/v1/auth/operator/enroll signs operator and CLI CSRs immediately
//      after activation, with no per-request owner approval.
//   3. Reserved-name issuance through generic app enrollment: the dashboard
//      ("g8ed") and ensemble ("g8ee") canonical names are accepted by the
//      generic app enrollment endpoint with no reservation check.
//   4. Privileged generic CSR signing: POST /api/v1/pki/csr/sign will sign
//      an operator leaf certificate from an unauthenticated caller because
//      the handler performs no authorization on the requested leaf type.
//
// When Phase 4 lands, these tests will be rewritten to assert the secure
// behavior (HTTP 403 / 401 / typed error) and renamed accordingly. They are
// intentionally kept together here so the bypass inventory is visible in
// one place during the transition.

package gateway

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// extractURISANs returns the URI SAN strings from a PEM-encoded certificate.
func extractURISANs(t *testing.T, certPEM string) []string {
	t.Helper()
	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block, "cert PEM must decode")
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err, "cert must parse")
	uris := make([]string, 0, len(cert.URIs))
	for _, u := range cert.URIs {
		uris = append(uris, u.String())
	}
	return uris
}

// TestPlatformEnrollmentBypass_PreActivationAppIssuance proves that the
// generic app enrollment endpoint issues a long-lived app certificate
// before the gateway has been activated by the first human. This is the
// dashboard/ensemble startup path: an unactivated gateway mints the g8ed
// identity on demand.
func TestPlatformEnrollmentBypass_PreActivationAppIssuance(t *testing.T) {
	c, _, _ := setupTestPKIController(t)

	// No user is created: the gateway is NOT activated. The plan's
	// invariant 1 is "a gateway with no users never issues a dashboard,
	// ensemble, or operator certificate." This test demonstrates the
	// current violation of that invariant.
	body := map[string]string{
		"csr_pem":  testutil.GenerateTestCSRP256(t, "g8ed"),
		"app_name": "g8ed",
		"app_type": "mcp-client",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.PKIAppsEnroll, bytes.NewReader(b))
	rr := httptest.NewRecorder()

	c.handlePKIAppsEnroll(rr, req)

	// CURRENT (insecure) behavior: HTTP 201 with a long-lived cert.
	// After Phase 4 this must become HTTP 403 with
	// ErrOperatorEnrollmentRequiresActivation (or a new
	// ErrPlatformEnrollmentRequiresActivation) and no cert.
	assert.Equal(t, http.StatusCreated, rr.Code, "Phase 0: app enrollment currently succeeds pre-activation (the bypass)")
	var resp AppEnrollResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Success, "Phase 0: app enrollment currently returns success pre-activation (the bypass)")
	assert.NotEmpty(t, resp.AppCert, "Phase 0: a certificate is currently issued pre-activation (the bypass)")
	uris := extractURISANs(t, resp.AppCert)
	assert.Contains(t, uris, "spiffe://g8e.local/app/g8ed", "Phase 0: the issued cert carries the reserved dashboard SPIFFE URI (the bypass)")
}

// TestPlatformEnrollmentBypass_ReservedNameIssuance proves that the generic
// app enrollment endpoint accepts the reserved platform component names
// "g8ed" (dashboard) and "g8ee" (ensemble) with no reservation check. After
// activation this remains a bypass because any caller can mint the
// dashboard or ensemble identity.
func TestPlatformEnrollmentBypass_ReservedNameIssuance(t *testing.T) {
	c, _, _ := setupTestPKIController(t)

	// The generic app enrollment path has no activation gate today, so
	// the reserved-name bypass is demonstrable without activation. The
	// point of this test is that even after Phase 4 adds an activation
	// gate, the reserved-name guard must also exist: activation alone
	// does not prevent an attacker from minting the dashboard or
	// ensemble identity via the generic path.

	reservedNames := []string{"g8ed", "g8ee"}
	for _, name := range reservedNames {
		t.Run(name, func(t *testing.T) {
			body := map[string]string{
				"csr_pem":  testutil.GenerateTestCSRP256(t, name),
				"app_name": name,
				"app_type": "mcp-client",
			}
			b, err := json.Marshal(body)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, constants.APIPaths.PKIAppsEnroll, bytes.NewReader(b))
			rr := httptest.NewRecorder()

			c.handlePKIAppsEnroll(rr, req)

			// CURRENT (insecure) behavior: HTTP 201 with the reserved
			// platform identity. After Phase 4 this must become HTTP
			// 403 with a typed reserved-name error and no cert.
			assert.Equal(t, http.StatusCreated, rr.Code, "Phase 0: reserved name %q is currently accepted (the bypass)", name)
			var resp AppEnrollResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			uris := extractURISANs(t, resp.AppCert)
			assert.Contains(t, uris, "spiffe://g8e.local/app/"+name, "Phase 0: the issued cert carries the reserved %q SPIFFE URI (the bypass)", name)
		})
	}
}

// TestPlatformEnrollmentBypass_PostActivationDirectOperatorIssuance proves
// that the dedicated operator enrollment endpoint signs operator and CLI
// CSRs immediately after activation, with no per-request owner approval.
// This is the bypass that the new platform enrollment protocol replaces:
// the endpoint must be removed (Phase 4) and the operator must enroll
// through the approved request/complete flow.
func TestPlatformEnrollmentBypass_PostActivationDirectOperatorIssuance(t *testing.T) {
	c, _ := setupTestBootstrapController(t)

	// Activate the gateway by creating the first real user (the owner).
	_, err := c.userSvc.CreateUser()
	require.NoError(t, err)

	body := map[string]string{
		"csr_pem":            testutil.GenerateTestCSRP256(t, "operator"),
		"cli_csr_pem":        testutil.GenerateTestCSRP256(t, "operator-cli"),
		"system_fingerprint": "fp-bypass",
		"hostname":           "operator-host",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthOperatorEnroll, bytes.NewReader(b))
	rr := httptest.NewRecorder()

	c.handleOperatorEnrollment(rr, req)

	// CURRENT (insecure) behavior: HTTP 201 with operator and CLI certs
	// and no approval record. After Phase 4 this endpoint must be removed
	// and this test must assert HTTP 404 (handler gone) or that the
	// request is rejected without an approval reference.
	assert.Equal(t, http.StatusCreated, rr.Code, "Phase 0: operator enrollment currently issues certs with no approval (the bypass)")
	var resp models.OperatorEnrollmentResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.NotEmpty(t, resp.OperatorCert, "Phase 0: an operator cert is currently issued with no approval (the bypass)")
	assert.NotEmpty(t, resp.CLICert, "Phase 0: a CLI cert is currently issued with no approval (the bypass)")
	assert.NotEmpty(t, resp.OperatorID)
	assert.NotEmpty(t, resp.OperatorSessionID)
	assert.NotEmpty(t, resp.CLISessionID)

	// The persisted operator document has no approval provenance.
	opDoc, err := c.docStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), resp.OperatorID)
	require.NoError(t, err)
	require.NotNil(t, opDoc)
	dataBytes, err := json.Marshal(opDoc.Data)
	require.NoError(t, err)
	var op models.OperatorDocumentGo
	require.NoError(t, json.Unmarshal(dataBytes, &op))
	assert.Empty(t, op.UserID, "Phase 0: operator doc has no approving user (the bypass)")
}

// TestPlatformEnrollmentBypass_PrivilegedGenericCSRSign proves that the
// generic CSR signing endpoint signs an operator leaf certificate with no
// authorization on the requested leaf type. The handler is registered on
// both the plain HTTP router and the HTTPS router, so an unauthenticated
// caller on the discovery router can mint an operator identity.
func TestPlatformEnrollmentBypass_PrivilegedGenericCSRSign(t *testing.T) {
	c, _, _ := setupTestPKIController(t)

	body := map[string]string{
		"csr_pem":   testutil.GenerateTestCSRP256(t, "rogue-operator"),
		"leaf_type": constants.LeafTypeOperator,
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.PKICSRSign, bytes.NewReader(b))
	// No TLS state, no auth context: the caller is unauthenticated.
	req.TLS = nil
	rr := httptest.NewRecorder()

	c.handlePKICSRSign(rr, req)

	// CURRENT (insecure) behavior: HTTP 200 with an operator leaf cert.
	// After Phase 4 this must become HTTP 401/403 with a typed
	// unauthorized-leaf-type error and no cert.
	assert.Equal(t, http.StatusOK, rr.Code, "Phase 0: generic CSR sign currently mints an operator leaf with no auth (the bypass)")
	var resp models.PKICSRSignResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.CertificatePEM, "Phase 0: an operator cert is currently minted with no auth (the bypass)")
	uris := extractURISANs(t, resp.CertificatePEM)
	// The operator SPIFFE URI is generated by SignCSR for the operator
	// leaf type; any operator URI here proves the bypass.
	assert.NotEmpty(t, uris, "Phase 0: the minted operator cert carries an operator SPIFFE URI (the bypass)")
}

// TestPlatformEnrollmentBypass_DelegatedAppIsShortLived documents the
// current SignDelegatedCSR behavior so the Phase 2 SignPlatformAppCSR
// implementation can be contrasted against it. Delegated credentials are
// 1-hour, client-auth-only, dual-SAN. The new platform app signer must use
// normal platform workload validity and server+client auth, not reuse this
// 1-hour path.
func TestPlatformEnrollmentBypass_DelegatedAppIsShortLived(t *testing.T) {
	c, _, _ := setupTestPKIController(t)

	// The delegated path requires mTLS; this test documents the signer
	// behavior, not the route auth, so call the service directly.
	resp, err := c.appEnrollment.EnrollDelegatedApp(AppEnrollRequest{
		CSR:     testutil.GenerateTestCSRP256(t, "delegated-app"),
		AppName: "delegated-app",
		AppType: "mcp-client",
	}, "user-approver-1")
	require.NoError(t, err)
	assert.True(t, resp.Success)

	block, _ := pem.Decode([]byte(resp.AppCert))
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	// Document the 1-hour validity so the Phase 2 signer does not reuse it.
	assert.Less(t, cert.NotAfter.Sub(cert.NotBefore).Hours(), float64(2),
		"Phase 0: SignDelegatedCSR is short-lived (~1h); the new SignPlatformAppCSR must NOT reuse this validity")
	// Document the client-auth-only EKU so the Phase 2 signer adds server auth.
	assert.ElementsMatch(t, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, cert.ExtKeyUsage,
		"Phase 0: SignDelegatedCSR is client-auth-only; the new SignPlatformAppCSR must add server auth for the dashboard/ensemble")
	// Document the dual-SAN ordering (app first, user second) so Phase 2
	// preserves the app-first invariant.
	uris := extractURISANs(t, resp.AppCert)
	require.Len(t, uris, 2)
	assert.Equal(t, "spiffe://g8e.local/app/delegated-app", uris[0], "Phase 0: app SAN is first; Phase 2 must preserve app-first ordering")
	assert.Equal(t, "spiffe://g8e.local/user/user-approver-1", uris[1], "Phase 0: user SAN is second; Phase 2 must bind the approving user")
}

// TestPlatformEnrollmentBypass_PlainHTTPRouterRegistersBypassHandlers
// proves that the plain HTTP discovery router (buildHTTPRouter) registers
// the operator enrollment, app enrollment, and generic CSR signing
// handlers without applying the auth middleware. This is the transport
// layer of the bypass: even though the RouteAuthRegistry classifies these
// routes as RouteAuthMTLS (fail-closed default) or RouteAuthNone, the plain
// HTTP router does not consult the registry at all — it registers the
// handlers directly behind only pathTraversalGuard and rateLimitMiddleware.
//
// After Phase 4, the plain HTTP router must not register the removed
// bypass routes. The new platform enrollment request/status/complete
// routes will be registered on plain HTTP (token-scoped, like CLI
// recovery), but pending/decision will be HTTPS-only.
func TestPlatformEnrollmentBypass_PlainHTTPRouterRegistersBypassHandlers(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	router := h.buildHTTPRouter()

	// The plain HTTP router must not apply auth middleware. The bypass
	// routes are reachable without mTLS. We prove this by sending a POST
	// to each bypass route with no TLS state and asserting the response
	// is NOT a 401/403 from the auth middleware (i.e., the handler was
	// reached). We use deliberately invalid bodies so the handler
	// returns a 400 (bad request) rather than a 201 (successful
	// enrollment), proving the handler was invoked without auth.
	bypassRoutes := []struct {
		name string
		path string
		body string
	}{
		{
			name: "operator enrollment",
			path: constants.APIPaths.AuthOperatorEnroll,
			body: `{"csr_pem":"","system_fingerprint":"","hostname":""}`,
		},
		{
			name: "app enrollment",
			path: constants.APIPaths.PKIAppsEnroll,
			body: `{"csr_pem":"","app_name":"","app_type":""}`,
		},
		{
			name: "generic CSR sign",
			path: constants.APIPaths.PKICSRSign,
			body: `{"csr_pem":"","leaf_type":""}`,
		},
	}

	for _, route := range bypassRoutes {
		t.Run(route.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, route.path, bytes.NewReader([]byte(route.body)))
			req.Header.Set("Content-Type", "application/json")
			// No TLS state: the caller is unauthenticated.
			req.TLS = nil
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			// The handler was reached (not blocked by auth middleware)
			// if the response is anything other than 401. A 400 means
			// the handler validated the body and rejected it; a 401
			// would mean the auth middleware blocked it. Phase 0
			// expects the handler to be reached (the bypass).
			assert.NotEqual(t, http.StatusUnauthorized, rr.Code,
				"Phase 0: plain HTTP router reaches the %s handler without auth (the bypass); Phase 4 must remove or secure this registration", route.name)
		})
	}
}
