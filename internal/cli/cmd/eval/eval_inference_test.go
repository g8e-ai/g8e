// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
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

// evalInferenceCredTestConfig builds a minimal config.Config rooted at an
// isolated temp directory, with the trust bundle written to the canonical
// runtime-relative location resolveInferenceProbeAppCredentials reads.
func evalInferenceCredTestConfig(t *testing.T, trustBundlePEM []byte) *config.Config {
	t.Helper()
	base := testutil.TempDir(t)
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
	cfg := evalInferenceCredTestConfig(t, currentCABundle)

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
	cfg := evalInferenceCredTestConfig(t, caBundle)

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
	cfg := evalInferenceCredTestConfig(t, caBundle)

	_, _, err := resolveInferenceProbeAppCredentials(nil, cfg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationAppCredentialMissing)
}
