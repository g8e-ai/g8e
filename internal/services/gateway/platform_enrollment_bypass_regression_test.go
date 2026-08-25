// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

// Regression tests proving the certificate-issuance bypasses closed by the
// owner-approved platform enrollment protocol (Phase 4). Each test asserts
// fail-closed behavior: the bypass route is gone, the handler rejects
// unauthenticated callers, or the retained path refuses reserved platform
// identities. The tests live in the gateway package because they exercise
// real PKI, real SQLite, real handler code paths, and the real auth
// middleware chain — they are not unit tests.
//
// Bypasses proven closed:
//   1. Pre-bootstrap app issuance: POST /api/v1/pki/apps/enroll is removed
//      from both routers; the auth middleware fail-closes the unclassified
//      path to RouteAuthMTLS, returning 401 without a client certificate.
//   2. Post-bootstrap direct operator issuance: POST
//      /api/v1/auth/operator/enroll is removed from both routers; same
//      fail-closed 401.
//   3. Reserved-name issuance through the retained delegated path:
//      EnrollDelegatedApp rejects g8ed/g8ee/g8eo with
//      ErrPlatformEnrollmentReservedIdentity.
//   4. Privileged generic CSR signing: handlePKICSRSign rejects callers
//      with no client certificate (401) via the defense-in-depth mTLS check.
//   5. Plain HTTP router no longer registers the CSR signing bypass route;
//      the catch-all redirects unregistered paths to HTTPS (301).

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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// removedAppEnrollPath is the path of the removed unauthenticated app
// enrollment route. The constant was deleted from api_paths.go when the
// route was removed; the literal is retained here so the regression test
// can prove the route is gone.
const removedAppEnrollPath = "/api/v1/pki/apps/enroll"

// removedOperatorEnrollPath is the path of the removed unauthenticated
// operator enrollment route. The constant was deleted from api_paths.go
// when the route was removed; the literal is retained here so the
// regression test can prove the route is gone.
const removedOperatorEnrollPath = "/api/v1/auth/operator/enroll"

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

// TestPlatformEnrollmentBypassClosed_AppEnrollRouteRemoved proves that the
// unauthenticated app enrollment route is gone. The route is removed from
// both routers and no longer explicitly classified in the RouteAuthRegistry,
// so the auth middleware fail-closes it to RouteAuthMTLS. A POST with no
// client certificate returns 401 — no certificate is issued, regardless of
// bootstrap state. This closes the pre-bootstrap app issuance bypass
// (invariant 1) and the reserved-name issuance bypass (invariant 3).
func TestPlatformEnrollmentBypassClosed_AppEnrollRouteRemoved(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	router := h.buildPublicRouter()

	body := map[string]string{
		"csr_pem":  testutil.GenerateTestCSRP256(t, "g8ed"),
		"app_name": "g8ed",
		"app_type": "mcp-client",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, removedAppEnrollPath, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.TLS = nil
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code,
		"the removed app enrollment route must fail-closed to 401 (RouteAuthMTLS default) without a client certificate")
	assert.NotContains(t, rr.Body.String(), "certificate_pem",
		"no certificate must be issued from the removed route")
}

// TestPlatformEnrollmentBypassClosed_OperatorEnrollRouteRemoved proves
// that the unauthenticated operator enrollment route is gone. The route is
// removed from both routers and no longer classified, so the auth
// middleware fail-closes it to RouteAuthMTLS. A POST with no client
// certificate returns 401 — no operator or CLI certificate is issued,
// even after bootstrap. This closes the post-bootstrap direct operator
// issuance bypass (invariant 2).
func TestPlatformEnrollmentBypassClosed_OperatorEnrollRouteRemoved(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	router := h.buildPublicRouter()

	body := map[string]string{
		"csr_pem":            testutil.GenerateTestCSRP256(t, "operator"),
		"cli_csr_pem":        testutil.GenerateTestCSRP256(t, "operator-cli"),
		"system_fingerprint": "fp-bypass",
		"hostname":           "operator-host",
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, removedOperatorEnrollPath, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.TLS = nil
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code,
		"the removed operator enrollment route must fail-closed to 401 (RouteAuthMTLS default) without a client certificate")
	assert.NotContains(t, rr.Body.String(), "operator_cert",
		"no operator certificate must be issued from the removed route")
}

// TestPlatformEnrollmentBypassClosed_ReservedNameRejectedByDelegatedPath
// proves that the retained authenticated delegated app enrollment path
// rejects the reserved platform component names (g8ed, g8ee, g8eo). Those
// identities are issued only through the owner-approved platform enrollment
// protocol. This closes the reserved-name issuance bypass (invariant 3) on
// the one path that remains.
func TestPlatformEnrollmentBypassClosed_ReservedNameRejectedByDelegatedPath(t *testing.T) {
	c, _, _ := setupTestPKIController(t)

	reservedNames := []string{"g8ed", "g8ee", "g8eo"}
	for _, name := range reservedNames {
		t.Run(name, func(t *testing.T) {
			resp, err := c.appEnrollment.EnrollDelegatedApp(AppEnrollRequest{
				CSR:     testutil.GenerateTestCSRP256(t, name),
				AppName: name,
				AppType: "mcp-client",
			}, "user-approver-1")
			require.NoError(t, err)
			assert.False(t, resp.Success, "reserved name %q must be rejected", name)
			assert.Contains(t, resp.Error, constants.ErrPlatformEnrollmentReservedIdentity.Error(),
				"reserved name %q must be rejected with ErrPlatformEnrollmentReservedIdentity", name)
			assert.Empty(t, resp.AppCert, "no certificate must be issued for reserved name %q", name)
		})
	}
}

// TestPlatformEnrollmentBypassClosed_PrivilegedGenericCSRSignRequiresMTLS
// proves that the generic CSR signing endpoint rejects callers with no
// client certificate. The handler enforces mTLS directly (defense-in-depth)
// and the route auth registry classifies the endpoint as RouteAuthMTLS. A
// request with no TLS state returns 401 — no privileged platform leaf type
// (operator, CLI, app, gateway-peer) is minted. This closes the privileged
// generic CSR signing bypass (invariant 16).
func TestPlatformEnrollmentBypassClosed_PrivilegedGenericCSRSignRequiresMTLS(t *testing.T) {
	c, _, _ := setupTestPKIController(t)

	body := map[string]string{
		"csr_pem":   testutil.GenerateTestCSRP256(t, "rogue-operator"),
		"leaf_type": constants.LeafTypeOperator,
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.PKICSRSign, bytes.NewReader(b))
	req.TLS = nil
	rr := httptest.NewRecorder()

	c.handlePKICSRSign(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code,
		"generic CSR sign must reject unauthenticated callers with 401 (mTLS required)")
	assert.Contains(t, rr.Body.String(), constants.ErrMissingCertificate.Error(),
		"the 401 must carry the typed ErrMissingCertificate error")
}

// TestPlatformEnrollmentBypass_DelegatedAppIsShortLived documents the
// current SignDelegatedCSR behavior so the Phase 2 SignPlatformAppCSR
// implementation can be contrasted against it. Delegated credentials are
// 1-hour, client-auth-only, dual-SAN. The new platform app signer must use
// normal platform workload validity and server+client auth, not reuse this
// 1-hour path. This is a reference characterization, not a bypass.
func TestPlatformEnrollmentBypass_DelegatedAppIsShortLived(t *testing.T) {
	c, _, _ := setupTestPKIController(t)

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

	assert.Less(t, cert.NotAfter.Sub(cert.NotBefore).Hours(), float64(2),
		"SignDelegatedCSR is short-lived (~1h); the new SignPlatformAppCSR must NOT reuse this validity")
	assert.ElementsMatch(t, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, cert.ExtKeyUsage,
		"SignDelegatedCSR is client-auth-only; the new SignPlatformAppCSR must add server auth for the dashboard/ensemble")
	uris := extractURISANs(t, resp.AppCert)
	require.Len(t, uris, 2)
	assert.Equal(t, "spiffe://g8e.local/app/delegated-app", uris[0], "app SAN is first; Phase 2 must preserve app-first ordering")
	assert.Equal(t, "spiffe://g8e.local/user/user-approver-1", uris[1], "user SAN is second; Phase 2 must bind the approving user")
}

// TestPlatformEnrollmentBypassClosed_PlainHTTPRouterDoesNotRegisterBypassRoutes
// proves that the plain HTTP discovery router no longer registers the CSR
// signing bypass route. The catch-all redirects unregistered paths to HTTPS
// (301), so the handler is never reached on plain HTTP. The operator-enroll
// and app-enroll routes are likewise gone (they redirect too). This closes
// the transport-layer bypass where the plain HTTP router registered
// handlers without the auth middleware.
func TestPlatformEnrollmentBypassClosed_PlainHTTPRouterDoesNotRegisterBypassRoutes(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)
	router := h.buildHTTPRouter()

	bypassRoutes := []struct {
		name string
		path string
		body string
	}{
		{
			name: "operator enrollment",
			path: removedOperatorEnrollPath,
			body: `{"csr_pem":"","system_fingerprint":"","hostname":""}`,
		},
		{
			name: "app enrollment",
			path: removedAppEnrollPath,
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
			req.TLS = nil
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			// The catch-all redirects unregistered paths to HTTPS (301).
			// A 400 would mean the handler was reached without auth (the
			// old bypass); a 301 proves the route is not registered on
			// plain HTTP.
			assert.Equal(t, http.StatusMovedPermanently, rr.Code,
				"plain HTTP router must not register the %s bypass route; the catch-all redirects to HTTPS", route.name)
		})
	}
}
