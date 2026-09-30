// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// mustGenerateCA creates a self-signed CA certificate and key, returned as
// PEM. Mirrors internal/pkg/certutil's test key-generation style.
func mustGenerateCA(t *testing.T, commonName string) (*ecdsa.PrivateKey, *x509.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return key, cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// mustGenerateClientLeaf issues a client-auth leaf certificate signed by the
// given CA, returned as PEM cert and PEM key.
func mustGenerateClientLeaf(t *testing.T, caKey *ecdsa.PrivateKey, caCert *x509.Certificate, commonName string) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

// evalInferenceCredTestConfig builds a minimal config.Config rooted at base,
// with the trust bundle written to the canonical runtime-relative location
// resolveInferenceProbeAppCredentials reads. base is a plain isolated temp
// directory for tests that only exercise the on-disk tiers, or a
// fs.RuntimeFileService's own root (via Resolve("")) for tests that also
// exercise the self-enrollment fallback, so paths written through fileSvc
// and paths read directly off cfg line up.
func evalInferenceCredTestConfig(t *testing.T, base string, trustBundlePEM []byte) *config.Config {
	t.Helper()
	appCertDir := filepath.Join(base, "pki", "issued", "apps")
	require.NoError(t, os.MkdirAll(appCertDir, constants.PermDirPrivate))
	cfg := &config.Config{
		RuntimeDir: base,
		Paths:      &config.PathsConfig{},
	}
	cfg.Paths.Infra.AppCertDir = appCertDir
	trustDir := filepath.Join(base, "pki", "trust")
	require.NoError(t, os.MkdirAll(trustDir, constants.PermDirPrivate))
	require.NoError(t, os.WriteFile(filepath.Join(trustDir, constants.PkiFileGatewayBundle), trustBundlePEM, constants.PermFilePrivate))
	return cfg
}

// TestResolveInferenceProbeAppCredentials_StaleAppCertRejected reproduces the
// evaluation failure mode where every formation-based eval run comes back
// PROVIDER_FAILED: the on-disk apps/g8ee cert was copied out of the ensemble
// container before the gateway's PKI was last regenerated, so it no longer
// chains to the current trust bundle. The stale cert must never be handed to
// the HTTP client — it must be rejected with a clear, actionable error.
func TestResolveInferenceProbeAppCredentials_StaleAppCertRejected(t *testing.T) {
	t.Setenv(string(constants.EnvVar.AppCert), "")
	t.Setenv(string(constants.EnvVar.AppKey), "")

	_, _, currentCABundle := mustGenerateCA(t, "current g8e Root CA")
	cfg := evalInferenceCredTestConfig(t, testutil.TempDir(t), currentCABundle)

	// The issued app cert was signed by a DIFFERENT (prior) gateway CA
	// generation, simulating a stale copy left over from before a gateway
	// restart regenerated the root CA.
	staleCAKey, staleCACert, _ := mustGenerateCA(t, "prior g8e Root CA")
	staleCertPEM, staleKeyPEM := mustGenerateClientLeaf(t, staleCAKey, staleCACert, "g8ee")
	require.NoError(t, os.WriteFile(filepath.Join(cfg.Paths.Infra.AppCertDir, "g8ee.crt"), staleCertPEM, constants.PermFilePrivate))
	require.NoError(t, os.WriteFile(filepath.Join(cfg.Paths.Infra.AppCertDir, "g8ee.key"), staleKeyPEM, constants.PermFilePrivate))

	_, _, err := resolveInferenceProbeAppCredentials(nil, cfg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationAppCredentialStale)
}

// TestResolveInferenceProbeAppCredentials_CurrentAppCertAccepted confirms a
// cert issued by the gateway's current CA is still accepted, so the trust
// check does not reject valid, freshly-enrolled credentials.
func TestResolveInferenceProbeAppCredentials_CurrentAppCertAccepted(t *testing.T) {
	t.Setenv(string(constants.EnvVar.AppCert), "")
	t.Setenv(string(constants.EnvVar.AppKey), "")

	caKey, caCert, caBundle := mustGenerateCA(t, "current g8e Root CA")
	cfg := evalInferenceCredTestConfig(t, testutil.TempDir(t), caBundle)

	certPEM, keyPEM := mustGenerateClientLeaf(t, caKey, caCert, "g8ee")
	wantCert := filepath.Join(cfg.Paths.Infra.AppCertDir, "g8ee.crt")
	wantKey := filepath.Join(cfg.Paths.Infra.AppCertDir, "g8ee.key")
	require.NoError(t, os.WriteFile(wantCert, certPEM, constants.PermFilePrivate))
	require.NoError(t, os.WriteFile(wantKey, keyPEM, constants.PermFilePrivate))

	gotCert, gotKey, err := resolveInferenceProbeAppCredentials(nil, cfg)

	require.NoError(t, err)
	assert.Equal(t, wantCert, gotCert)
	assert.Equal(t, wantKey, gotKey)
}

// TestResolveInferenceProbeAppCredentials_NoCandidateReturnsMissingError
// confirms the absence of any candidate still reports the (now centralized)
// missing-credential sentinel rather than a raw ad hoc error string.
func TestResolveInferenceProbeAppCredentials_NoCandidateReturnsMissingError(t *testing.T) {
	t.Setenv(string(constants.EnvVar.AppCert), "")
	t.Setenv(string(constants.EnvVar.AppKey), "")

	_, _, caBundle := mustGenerateCA(t, "current g8e Root CA")
	cfg := evalInferenceCredTestConfig(t, testutil.TempDir(t), caBundle)

	_, _, err := resolveInferenceProbeAppCredentials(nil, cfg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationAppCredentialMissing)
}

// newEvalTestFileSvc builds an isolated RuntimeFileService for tests that
// exercise the self-enrollment fallback, which needs fileSvc for CLI
// credential and cert I/O alongside the direct os.* reads
// resolveInferenceProbeAppCredentials itself performs.
func newEvalTestFileSvc(t *testing.T) fs.RuntimeFileService {
	t.Helper()
	svc, err := fs.NewRuntimeFileService(testutil.TempDir(t), slog.Default())
	require.NoError(t, err)
	require.NoError(t, svc.CreateRuntimeTree(context.Background()))
	return svc
}

// mustGenerateClientLeafWithSPIFFE mirrors mustGenerateClientLeaf but stamps
// the app SPIFFE URI SAN a real delegated app credential carries, with a
// caller-controlled expiry. checkExistingAppCert only reuses a cert whose
// SPIFFE SAN and expiry both look right, so exercising the "stale but
// otherwise valid-looking" case needs it.
func mustGenerateClientLeafWithSPIFFE(t *testing.T, caKey *ecdsa.PrivateKey, caCert *x509.Certificate, agentName string, notAfter time.Time) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	uri, err := url.Parse("spiffe://g8e.local/app/" + agentName)
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: agentName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs:         []*url.URL{uri},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

// writeEvalCLICert writes a self-signed CLI cert/key pair, the mTLS client
// identity EnrollAgentApp presents when calling the gateway.
func writeEvalCLICert(t *testing.T, fileSvc fs.RuntimeFileService, cfg *config.Config) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber:          big.NewInt(5),
		Subject:               pkix.Name{CommonName: "test-cli"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	certRel, err := fileSvc.RelFromAbs(cfg.CLICertFile())
	require.NoError(t, err)
	keyRel, err := fileSvc.RelFromAbs(cfg.CLIKeyFile())
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, certPEM, constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, keyPEM, constants.PermFilePrivate))
}

// writeEvalCLICredentials writes a CLI cert/key pair and a synthetic CLI
// session, the minimum EnrollAgentApp needs to treat the caller as an
// already-authenticated CLI session (see agent_enroll.go).
func writeEvalCLICredentials(t *testing.T, fileSvc fs.RuntimeFileService, cfg *config.Config) {
	t.Helper()
	writeEvalCLICert(t, fileSvc, cfg)
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, &auth.Credentials{UserID: "test-user", CLISessionID: "test-session-id"}))
}

// startEvalEnrollServer starts a TLS server signed by caKey/caCert — the same
// CA cfg's trust bundle carries — and points cfg's operator URL at it, so
// EnrollAgentApp's self-enrollment HTTP round trip resolves locally.
func startEvalEnrollServer(t *testing.T, cfg *config.Config, caKey *ecdsa.PrivateKey, caCert *x509.Certificate, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(4),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	require.NoError(t, err)

	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{serverDER}, PrivateKey: serverKey}}}
	server.StartTLS()
	t.Cleanup(server.Close)
	cfg.Paths.Host = server.URL
	return server
}

// evalEnrollResponseBody builds the AppEnrollResponse JSON body a gateway
// PKIAppsDelegated success reply carries, with the app cert signed by
// caKey/caCert.
func evalEnrollResponseBody(t *testing.T, caKey *ecdsa.PrivateKey, caCert *x509.Certificate, agentName string) []byte {
	t.Helper()
	certPEM, _ := mustGenerateClientLeafWithSPIFFE(t, caKey, caCert, agentName, time.Now().Add(365*24*time.Hour))
	resp := struct {
		Success bool   `json:"success"`
		AppCert string `json:"app_cert"`
		AppID   string `json:"app_id"`
	}{
		Success: true,
		AppCert: string(certPEM),
		AppID:   "spiffe://g8e.local/app/" + agentName,
	}
	body, err := json.Marshal(resp)
	require.NoError(t, err)
	return body
}

// TestResolveInferenceProbeAppCredentials_SelfEnrollsWhenMissing reproduces
// the exact failure from an `eval runs start` formation run: no G8E_APP_CERT/
// G8E_APP_KEY, nothing copied out of the ensemble container, and no
// previously self-enrolled g8ee cert on disk. Instead of surfacing the
// missing-credential error, resolveInferenceProbeAppCredentials must
// self-enroll g8ee using the CLI's already-authenticated session, the same
// path `mcp agent run` uses, so the operator never has to hand-copy a
// certificate out of a container.
func TestResolveInferenceProbeAppCredentials_SelfEnrollsWhenMissing(t *testing.T) {
	t.Setenv(string(constants.EnvVar.AppCert), "")
	t.Setenv(string(constants.EnvVar.AppKey), "")

	fileSvc := newEvalTestFileSvc(t)
	base := fileSvc.Resolve("")
	caKey, caCert, caBundle := mustGenerateCA(t, "current g8e Root CA")
	cfg := evalInferenceCredTestConfig(t, base, caBundle)
	writeEvalCLICredentials(t, fileSvc, cfg)

	var enrollCalls int
	startEvalEnrollServer(t, cfg, caKey, caCert, func(w http.ResponseWriter, r *http.Request) {
		enrollCalls++
		assert.Equal(t, constants.APIPaths.PKIAppsDelegated, r.URL.Path)
		w.Header().Set(constants.HeaderContentType, constants.HeaderValueApplicationJSON)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(evalEnrollResponseBody(t, caKey, caCert, "g8ee"))
	})

	certFile, keyFile, err := resolveInferenceProbeAppCredentials(fileSvc, cfg)

	require.NoError(t, err)
	assert.Equal(t, 1, enrollCalls)
	assert.Equal(t, cfg.AppCertFile("g8ee"), certFile)
	assert.Equal(t, cfg.AppKeyFile("g8ee"), keyFile)
	assert.FileExists(t, certFile)
	assert.FileExists(t, keyFile)
}

// TestResolveInferenceProbeAppCredentials_StaleManagedCertSelfEnrollsReplacement
// covers the case checkExistingAppCert alone would get wrong: a g8ee cert at
// the self-enrolled location with a valid SPIFFE SAN and long remaining
// validity, but issued by a gateway PKI generation the current trust bundle
// no longer recognizes. Self-enrollment must clear that stale pair before
// calling EnrollAgentApp, or EnrollAgentApp's own freshness check (expiry +
// SPIFFE SAN only, not trust-chain) would just hand back the same untrusted
// cert.
func TestResolveInferenceProbeAppCredentials_StaleManagedCertSelfEnrollsReplacement(t *testing.T) {
	t.Setenv(string(constants.EnvVar.AppCert), "")
	t.Setenv(string(constants.EnvVar.AppKey), "")

	fileSvc := newEvalTestFileSvc(t)
	base := fileSvc.Resolve("")
	caKey, caCert, caBundle := mustGenerateCA(t, "current g8e Root CA")
	cfg := evalInferenceCredTestConfig(t, base, caBundle)

	staleCAKey, staleCACert, _ := mustGenerateCA(t, "prior g8e Root CA")
	staleCertPEM, staleKeyPEM := mustGenerateClientLeafWithSPIFFE(t, staleCAKey, staleCACert, "g8ee", time.Now().Add(30*24*time.Hour))
	managedCertRel, err := fileSvc.RelFromAbs(cfg.AppCertFile("g8ee"))
	require.NoError(t, err)
	managedKeyRel, err := fileSvc.RelFromAbs(cfg.AppKeyFile("g8ee"))
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), managedCertRel, staleCertPEM, constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), managedKeyRel, staleKeyPEM, constants.PermFilePrivate))

	writeEvalCLICredentials(t, fileSvc, cfg)

	var enrollCalls int
	startEvalEnrollServer(t, cfg, caKey, caCert, func(w http.ResponseWriter, r *http.Request) {
		enrollCalls++
		w.Header().Set(constants.HeaderContentType, constants.HeaderValueApplicationJSON)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(evalEnrollResponseBody(t, caKey, caCert, "g8ee"))
	})

	certFile, keyFile, err := resolveInferenceProbeAppCredentials(fileSvc, cfg)

	require.NoError(t, err)
	assert.Equal(t, 1, enrollCalls)
	assert.Equal(t, cfg.AppCertFile("g8ee"), certFile)
	assert.Equal(t, cfg.AppKeyFile("g8ee"), keyFile)
	replacedPEM, err := os.ReadFile(certFile)
	require.NoError(t, err)
	assert.NotEqual(t, staleCertPEM, replacedPEM)
}

// TestResolveInferenceProbeAppCredentials_SelfEnrollFailureWrapsMissingError
// confirms that when self-enrollment cannot proceed (a CLI mTLS identity
// exists but has no authenticated session behind it — the credentials file
// is absent), the original missing-credential sentinel still surfaces,
// wrapped around the underlying authentication failure so the operator sees
// why self-enrollment didn't happen instead of a generic message.
func TestResolveInferenceProbeAppCredentials_SelfEnrollFailureWrapsMissingError(t *testing.T) {
	t.Setenv(string(constants.EnvVar.AppCert), "")
	t.Setenv(string(constants.EnvVar.AppKey), "")

	fileSvc := newEvalTestFileSvc(t)
	base := fileSvc.Resolve("")
	_, _, caBundle := mustGenerateCA(t, "current g8e Root CA")
	cfg := evalInferenceCredTestConfig(t, base, caBundle)
	writeEvalCLICert(t, fileSvc, cfg) // no session saved: LoadCredentials returns nil, nil

	_, _, err := resolveInferenceProbeAppCredentials(fileSvc, cfg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationAppCredentialMissing)
	assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
}
