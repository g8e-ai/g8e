// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version.0.

package frontendverify

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/browserorigin"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// testGatewayServer stands up a TLS httptest.Server that simulates the
// Gateway's HTTPS health endpoint and CORS preflight handler. The
// server's certificate is signed by the provided root CA, and the CORS
// middleware reflects the provided allowed origin.
type testGatewayServer struct {
	t        *testing.T
	rootCert *x509.Certificate
	server   *httptest.Server
	allowed  string
	healthOK bool
	emitVary bool
}

func newTestGatewayServer(t *testing.T, allowedOrigin string, healthOK bool) *testGatewayServer {
	t.Helper()
	caKey, caCert := testutil.GenerateTestCAWithKey(t, "test-gateway-root")

	// Generate a leaf cert with both DNS and IP SANs so the TLS client
	// can validate it against 127.0.0.1 (httptest binds to 127.0.0.1).
	leafPEM, leafKey := generateLeafWithIPSANs(t, "localhost", caCert, caKey)
	keyDER, err := x509.MarshalECPrivateKey(leafKey)
	require.NoError(t, err)
	leafKeyPEM := testutil.EncodePEM("EC PRIVATE KEY", keyDER)
	leafTLSCert, err := tls.X509KeyPair([]byte(leafPEM), []byte(leafKeyPEM))
	require.NoError(t, err)

	gw := &testGatewayServer{
		t:        t,
		rootCert: caCert,
		allowed:  allowedOrigin,
		healthOK: healthOK,
		emitVary: true,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", gw.handleHealth)

	gw.server = httptest.NewUnstartedServer(mux)
	gw.server.TLS = &tls.Config{
		Certificates: []tls.Certificate{leafTLSCert},
	}
	gw.server.StartTLS()
	return gw
}

func (g *testGatewayServer) close() {
	g.server.Close()
}

func (g *testGatewayServer) healthURL() string {
	return g.server.URL + "/api/v1/health"
}

func (g *testGatewayServer) rootPool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(g.rootCert)
	return pool
}

func (g *testGatewayServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	g.applyCORS(w, r)
	if r.Method == http.MethodOptions {
		return
	}
	if !g.healthOK {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	resp := models.HealthResponse{
		Status: constants.GatewayModeStatusOK,
		Mode:   constants.GatewayModeGateway,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (g *testGatewayServer) applyCORS(w http.ResponseWriter, r *http.Request) {
	if g.emitVary {
		w.Header().Add(constants.HeaderVary, "Origin")
	}
	origin := r.Header.Get(constants.HeaderOrigin)
	if origin == "" || !strings.EqualFold(strings.TrimRight(origin, "/"), strings.TrimRight(g.allowed, "/")) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		return
	}
	w.Header().Set(constants.HeaderAccessControlAllowOrigin, origin)
	w.Header().Set(constants.HeaderAccessControlAllowCredentials, "true")
	if r.Method == http.MethodOptions {
		w.Header().Set(constants.HeaderAccessControlAllowMethods, "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set(constants.HeaderAccessControlAllowHeaders, "Content-Type, Authorization, X-G8E-Web-Session-ID, X-G8E-CLI-Session-ID, X-G8E-Operator-Session-ID, X-G8E-User-ID, X-G8E-Operator-ID, X-G8E-Request-ID, X-Requested-With")
		w.WriteHeader(http.StatusNoContent)
		return
	}
}

func mustParseOrigin(t *testing.T, raw string) browserorigin.Origin {
	t.Helper()
	origin, err := browserorigin.Parse(raw)
	require.NoError(t, err)
	return origin
}

// generateLeafWithIPSANs generates a leaf certificate signed by the
// given CA with DNS name "localhost" and IP SAN 127.0.0.1, so the TLS
// client can validate it against the httptest server's 127.0.0.1
// address.
func generateLeafWithIPSANs(t *testing.T, commonName string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (string, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, parent, &key.PublicKey, parentKey)
	require.NoError(t, err)
	return testutil.EncodePEM("CERTIFICATE", der), key
}

func TestVerifier_AllChecksPassWithCorrectConfig(t *testing.T) {
	origin := mustParseOrigin(t, "https://your-app.lovable.app")
	gw := newTestGatewayServer(t, origin.URL, true)
	defer gw.close()

	verifier := NewVerifier(VerifierDeps{})
	report, err := verifier.Verify(t.Context(), VerifyOptions{
		FrontendOrigin: origin,
		APIURL:         gw.healthURL(),
		RootPool:       gw.rootPool(),
		Timeout:        5 * time.Second,
	})
	require.NoError(t, err)
	assert.True(t, report.AllPassed)
	assert.Nil(t, report.FirstFailure)

	require.Len(t, report.Checks, 7)
	assert.Equal(t, CheckHTTPSHealth, report.Checks[0].Name)
	assert.Equal(t, CheckCertificateChain, report.Checks[1].Name)
	assert.Equal(t, CheckCORSOrigin, report.Checks[2].Name)
	assert.Equal(t, CheckCORSCredentials, report.Checks[3].Name)
	assert.Equal(t, CheckCORSMethods, report.Checks[4].Name)
	assert.Equal(t, CheckCORSHeaders, report.Checks[5].Name)
	assert.Equal(t, CheckCORSVary, report.Checks[6].Name)

	for _, c := range report.Checks {
		assert.Equal(t, CheckPass, c.Status, "check %s should pass", c.Name)
	}
}

func TestVerifier_HTTPSHealthFailsWithWrongRoot(t *testing.T) {
	origin := mustParseOrigin(t, "https://your-app.lovable.app")
	gw := newTestGatewayServer(t, origin.URL, true)
	defer gw.close()

	wrongCAKey, wrongCACert := testutil.GenerateTestCAWithKey(t, "wrong-root")
	wrongPool := x509.NewCertPool()
	wrongPool.AddCert(wrongCACert)
	_ = wrongCAKey

	verifier := NewVerifier(VerifierDeps{})
	report, err := verifier.Verify(t.Context(), VerifyOptions{
		FrontendOrigin: origin,
		APIURL:         gw.healthURL(),
		RootPool:       wrongPool,
		Timeout:        5 * time.Second,
	})
	require.NoError(t, err)
	assert.False(t, report.AllPassed)
	require.NotNil(t, report.FirstFailure)
	assert.Equal(t, CheckHTTPSHealth, report.FirstFailure.Name)
	assert.Equal(t, CheckFail, report.FirstFailure.Status)
	assert.Len(t, report.Checks, 1)
}

func TestVerifier_HTTPSHealthFailsWithNonOKStatus(t *testing.T) {
	origin := mustParseOrigin(t, "https://your-app.lovable.app")
	gw := newTestGatewayServer(t, origin.URL, false)
	defer gw.close()

	verifier := NewVerifier(VerifierDeps{})
	report, err := verifier.Verify(t.Context(), VerifyOptions{
		FrontendOrigin: origin,
		APIURL:         gw.healthURL(),
		RootPool:       gw.rootPool(),
		Timeout:        5 * time.Second,
	})
	require.NoError(t, err)
	assert.False(t, report.AllPassed)
	require.NotNil(t, report.FirstFailure)
	assert.Equal(t, CheckHTTPSHealth, report.FirstFailure.Name)
	assert.Contains(t, report.FirstFailure.Detail, "503")
}

func TestVerifier_CORSOriginMismatchFails(t *testing.T) {
	gw := newTestGatewayServer(t, "https://allowed.lovable.app", true)
	defer gw.close()

	origin := mustParseOrigin(t, "https://different.lovable.app")
	verifier := NewVerifier(VerifierDeps{})
	report, err := verifier.Verify(t.Context(), VerifyOptions{
		FrontendOrigin: origin,
		APIURL:         gw.healthURL(),
		RootPool:       gw.rootPool(),
		Timeout:        5 * time.Second,
	})
	require.NoError(t, err)
	assert.False(t, report.AllPassed)
	require.NotNil(t, report.FirstFailure)
	assert.Equal(t, CheckCORSOrigin, report.FirstFailure.Name)
}

func TestVerifier_CORSOriginReflectedExactly(t *testing.T) {
	origin := mustParseOrigin(t, "https://your-app.lovable.app")
	gw := newTestGatewayServer(t, origin.URL, true)
	defer gw.close()

	verifier := NewVerifier(VerifierDeps{})
	report, err := verifier.Verify(t.Context(), VerifyOptions{
		FrontendOrigin: origin,
		APIURL:         gw.healthURL(),
		RootPool:       gw.rootPool(),
		Timeout:        5 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, report.AllPassed)

	corsOriginCheck := report.Checks[2]
	assert.Equal(t, CheckCORSOrigin, corsOriginCheck.Name)
	assert.Equal(t, CheckPass, corsOriginCheck.Status)
	assert.Contains(t, corsOriginCheck.Detail, origin.URL)
}

func TestVerifier_CORSCredentialsPassWhenPresent(t *testing.T) {
	origin := mustParseOrigin(t, "https://your-app.lovable.app")
	gw := newTestGatewayServer(t, origin.URL, true)
	defer gw.close()

	verifier := NewVerifier(VerifierDeps{})
	report, err := verifier.Verify(t.Context(), VerifyOptions{
		FrontendOrigin: origin,
		APIURL:         gw.healthURL(),
		RootPool:       gw.rootPool(),
		Timeout:        5 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, report.AllPassed)

	credsCheck := report.Checks[3]
	assert.Equal(t, CheckCORSCredentials, credsCheck.Name)
	assert.Equal(t, CheckPass, credsCheck.Status)
}

func TestVerifier_CORSMethodsIncludeRequestedMethod(t *testing.T) {
	origin := mustParseOrigin(t, "https://your-app.lovable.app")
	gw := newTestGatewayServer(t, origin.URL, true)
	defer gw.close()

	verifier := NewVerifier(VerifierDeps{})
	report, err := verifier.Verify(t.Context(), VerifyOptions{
		FrontendOrigin: origin,
		APIURL:         gw.healthURL(),
		RootPool:       gw.rootPool(),
		Timeout:        5 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, report.AllPassed)

	methodsCheck := report.Checks[4]
	assert.Equal(t, CheckCORSMethods, methodsCheck.Name)
	assert.Equal(t, CheckPass, methodsCheck.Status)
	assert.Contains(t, methodsCheck.Detail, "POST")
}

func TestVerifier_CORSHeadersIncludeRequestedHeaders(t *testing.T) {
	origin := mustParseOrigin(t, "https://your-app.lovable.app")
	gw := newTestGatewayServer(t, origin.URL, true)
	defer gw.close()

	verifier := NewVerifier(VerifierDeps{})
	report, err := verifier.Verify(t.Context(), VerifyOptions{
		FrontendOrigin: origin,
		APIURL:         gw.healthURL(),
		RootPool:       gw.rootPool(),
		Timeout:        5 * time.Second,
	})
	require.NoError(t, err)
	require.True(t, report.AllPassed)

	headersCheck := report.Checks[5]
	assert.Equal(t, CheckCORSHeaders, headersCheck.Name)
	assert.Equal(t, CheckPass, headersCheck.Status)
}

func TestVerifier_CORSMissingVaryOriginFails(t *testing.T) {
	origin := mustParseOrigin(t, "https://your-app.lovable.app")
	gw := newTestGatewayServer(t, origin.URL, true)
	gw.emitVary = false
	defer gw.close()

	verifier := NewVerifier(VerifierDeps{})
	report, err := verifier.Verify(t.Context(), VerifyOptions{
		FrontendOrigin: origin,
		APIURL:         gw.healthURL(),
		RootPool:       gw.rootPool(),
		Timeout:        5 * time.Second,
	})
	require.NoError(t, err)
	assert.False(t, report.AllPassed)
	require.NotNil(t, report.FirstFailure)
	assert.Equal(t, CheckCORSVary, report.FirstFailure.Name)
}

func TestVerifier_ReportDeterministicOrder(t *testing.T) {
	origin := mustParseOrigin(t, "https://your-app.lovable.app")
	gw := newTestGatewayServer(t, origin.URL, true)
	defer gw.close()

	verifier := NewVerifier(VerifierDeps{})
	report, err := verifier.Verify(t.Context(), VerifyOptions{
		FrontendOrigin: origin,
		APIURL:         gw.healthURL(),
		RootPool:       gw.rootPool(),
		Timeout:        5 * time.Second,
	})
	require.NoError(t, err)

	expectedOrder := []CheckName{
		CheckHTTPSHealth,
		CheckCertificateChain,
		CheckCORSOrigin,
		CheckCORSCredentials,
		CheckCORSMethods,
		CheckCORSHeaders,
		CheckCORSVary,
	}
	require.Len(t, report.Checks, len(expectedOrder))
	for i, expected := range expectedOrder {
		assert.Equal(t, expected, report.Checks[i].Name, "check at index %d", i)
	}
}

func TestVerifier_FirstFailureIdentifiesResponsibleLayer(t *testing.T) {
	origin := mustParseOrigin(t, "https://your-app.lovable.app")
	gw := newTestGatewayServer(t, "https://other.lovable.app", true)
	defer gw.close()

	verifier := NewVerifier(VerifierDeps{})
	report, err := verifier.Verify(t.Context(), VerifyOptions{
		FrontendOrigin: origin,
		APIURL:         gw.healthURL(),
		RootPool:       gw.rootPool(),
		Timeout:        5 * time.Second,
	})
	require.NoError(t, err)
	assert.False(t, report.AllPassed)
	require.NotNil(t, report.FirstFailure)
	assert.Equal(t, CheckCORSOrigin, report.FirstFailure.Name)
}

func TestVerifier_DefaultTimeoutAppliedWhenZero(t *testing.T) {
	origin := mustParseOrigin(t, "https://your-app.lovable.app")
	gw := newTestGatewayServer(t, origin.URL, true)
	defer gw.close()

	verifier := NewVerifier(VerifierDeps{})
	report, err := verifier.Verify(t.Context(), VerifyOptions{
		FrontendOrigin: origin,
		APIURL:         gw.healthURL(),
		RootPool:       gw.rootPool(),
		Timeout:        0,
	})
	require.NoError(t, err)
	assert.True(t, report.AllPassed)
}

// Ensure the verifier never uses InsecureSkipVerify by verifying that a
// wrong root pool causes a TLS handshake failure (not a silent pass).
func TestVerifier_NeverUsesInsecureSkipVerify(t *testing.T) {
	origin := mustParseOrigin(t, "https://your-app.lovable.app")
	gw := newTestGatewayServer(t, origin.URL, true)
	defer gw.close()

	// Empty root pool — the TLS client should reject the server cert.
	emptyPool := x509.NewCertPool()

	verifier := NewVerifier(VerifierDeps{})
	report, err := verifier.Verify(t.Context(), VerifyOptions{
		FrontendOrigin: origin,
		APIURL:         gw.healthURL(),
		RootPool:       emptyPool,
		Timeout:        5 * time.Second,
	})
	require.NoError(t, err)
	assert.False(t, report.AllPassed)
	require.NotNil(t, report.FirstFailure)
	assert.Equal(t, CheckHTTPSHealth, report.FirstFailure.Name)
	assert.Contains(t, report.FirstFailure.Detail, "certificate")
}

// Ensure the verifier's HTTP client factory is injectable so tests can
// target a custom transport without touching the default.
func TestVerifier_HTTPClientFactoryInjectable(t *testing.T) {
	origin := mustParseOrigin(t, "https://your-app.lovable.app")
	gw := newTestGatewayServer(t, origin.URL, true)
	defer gw.close()

	factoryCalled := false
	verifier := NewVerifier(VerifierDeps{
		HTTPClientFactory: func(rootPool *x509.CertPool, timeout time.Duration) (*http.Client, error) {
			factoryCalled = true
			return defaultHTTPClientFactory(rootPool, timeout)
		},
	})
	_, err := verifier.Verify(t.Context(), VerifyOptions{
		FrontendOrigin: origin,
		APIURL:         gw.healthURL(),
		RootPool:       gw.rootPool(),
		Timeout:        5 * time.Second,
	})
	require.NoError(t, err)
	assert.True(t, factoryCalled, "injected HTTP client factory must be called")
}
