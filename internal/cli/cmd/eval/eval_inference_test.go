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
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// mustGenerateCA creates a self-signed CA certificate and key, returned as PEM.
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
func mustGenerateClientLeaf(t *testing.T, caKey *ecdsa.PrivateKey, caCert *x509.Certificate, appName string, notAfter time.Time) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	uri, err := url.Parse("spiffe://g8e.local/app/" + appName)
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: fmtAppCN(appName)},
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

func fmtAppCN(appName string) string {
	return "g8e-app-" + appName
}

func newEvalTestFileSvc(t *testing.T) fs.RuntimeFileService {
	t.Helper()
	svc, err := fs.NewRuntimeFileService(testutil.TempDir(t), slog.Default())
	require.NoError(t, err)
	require.NoError(t, svc.CreateRuntimeTree(context.Background()))
	return svc
}

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

func TestResolveInferenceProbeAppCredentials_NoCandidateReturnsMissingError(t *testing.T) {
	fileSvc := newEvalTestFileSvc(t)
	base := fileSvc.Resolve("")
	_, _, caBundle := mustGenerateCA(t, "current g8e Root CA")
	cfg := evalInferenceCredTestConfig(t, base, caBundle)

	_, _, err := resolveInferenceProbeAppCredentials(fileSvc, cfg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrAppIdentityNotFound)
	assert.Contains(t, err.Error(), "g8e auth enroll app g8e-eval")

	// Verify it never touches or expects g8ee paths.
	assert.NoFileExists(t, cfg.AppCertFile("g8ee"))
	assert.NoFileExists(t, filepath.Join(cfg.Paths.Infra.AppCertDir, "g8ee.crt"))
}

func TestResolveInferenceProbeAppCredentials_CurrentAppCertAccepted(t *testing.T) {
	fileSvc := newEvalTestFileSvc(t)
	base := fileSvc.Resolve("")
	caKey, caCert, caBundle := mustGenerateCA(t, "current g8e Root CA")
	cfg := evalInferenceCredTestConfig(t, base, caBundle)

	certPEM, keyPEM := mustGenerateClientLeaf(t, caKey, caCert, "g8e-eval", time.Now().Add(7*24*time.Hour))
	wantCert := cfg.AppCertFile("g8e-eval")
	wantKey := cfg.AppKeyFile("g8e-eval")

	certRel, err := fileSvc.RelFromAbs(wantCert)
	require.NoError(t, err)
	keyRel, err := fileSvc.RelFromAbs(wantKey)
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, certPEM, constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, keyPEM, constants.PermFilePrivate))

	gotCert, gotKey, err := resolveInferenceProbeAppCredentials(fileSvc, cfg)

	require.NoError(t, err)
	assert.Equal(t, wantCert, gotCert)
	assert.Equal(t, wantKey, gotKey)
}

func TestResolveInferenceProbeAppCredentials_StaleAppCertRejected(t *testing.T) {
	fileSvc := newEvalTestFileSvc(t)
	base := fileSvc.Resolve("")
	_, _, currentCABundle := mustGenerateCA(t, "current g8e Root CA")
	cfg := evalInferenceCredTestConfig(t, base, currentCABundle)

	// Signed by a prior gateway CA generation.
	staleCAKey, staleCACert, _ := mustGenerateCA(t, "prior g8e Root CA")
	staleCertPEM, staleKeyPEM := mustGenerateClientLeaf(t, staleCAKey, staleCACert, "g8e-eval", time.Now().Add(7*24*time.Hour))

	certRel, err := fileSvc.RelFromAbs(cfg.AppCertFile("g8e-eval"))
	require.NoError(t, err)
	keyRel, err := fileSvc.RelFromAbs(cfg.AppKeyFile("g8e-eval"))
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, staleCertPEM, constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, staleKeyPEM, constants.PermFilePrivate))

	_, _, err = resolveInferenceProbeAppCredentials(fileSvc, cfg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrAppIdentityUntrusted)
	assert.Contains(t, err.Error(), "g8e auth enroll app g8e-eval")
}

func TestResolveInferenceProbeAppCredentials_ExpiredAppCertRejected(t *testing.T) {
	fileSvc := newEvalTestFileSvc(t)
	base := fileSvc.Resolve("")
	caKey, caCert, caBundle := mustGenerateCA(t, "current g8e Root CA")
	cfg := evalInferenceCredTestConfig(t, base, caBundle)

	// Expired 1 hour ago.
	expiredCertPEM, expiredKeyPEM := mustGenerateClientLeaf(t, caKey, caCert, "g8e-eval", time.Now().Add(-time.Hour))

	certRel, err := fileSvc.RelFromAbs(cfg.AppCertFile("g8e-eval"))
	require.NoError(t, err)
	keyRel, err := fileSvc.RelFromAbs(cfg.AppKeyFile("g8e-eval"))
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, expiredCertPEM, constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, expiredKeyPEM, constants.PermFilePrivate))

	_, _, err = resolveInferenceProbeAppCredentials(fileSvc, cfg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrAppIdentityExpired)
	assert.Contains(t, err.Error(), "g8e auth enroll app g8e-eval")
}

func TestResolveInferenceProbeAppCredentials_NilParameters(t *testing.T) {
	_, _, err := resolveInferenceProbeAppCredentials(nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInternal)
}
