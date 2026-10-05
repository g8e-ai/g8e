// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// generateTestCertificateWithSPIFFE generates a test certificate with a SPIFFE URI SAN.
func generateTestCertificateWithSPIFFE(t *testing.T, agentName string, notAfter time.Time) (certPEM string, keyPEM string) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	spiffeID := "spiffe://g8e.local/app/" + agentName
	uri, err := url.Parse(spiffeID)
	require.NoError(t, err)

	now := time.Now().Truncate(time.Second)
	notAfter = notAfter.Truncate(time.Second)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: agentName,
		},
		NotBefore: now,
		NotAfter:  notAfter,
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
		},
		URIs:                  []*url.URL{uri},
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	certPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	}))

	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	require.NoError(t, err)

	keyPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	}))

	return certPEM, keyPEM
}

// writeTestCLICert generates a self-signed CLI cert and writes it to cfg.CLICertFile()/CLIKeyFile().
func writeTestCLICert(t *testing.T, fileSvc fs.RuntimeFileService, cfg *config.Config) {
	t.Helper()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "test-cli"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	certRel, err := fileSvc.RelFromAbs(cfg.CLICertFile())
	require.NoError(t, err)
	keyRel, err := fileSvc.RelFromAbs(cfg.CLIKeyFile())
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, certPEM, constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, keyPEM, constants.PermFilePrivate))
}

func TestValidateAppName(t *testing.T) {
	assert.ErrorIs(t, validateAppName(""), constants.ErrPlatformEnrollmentAppNameRequired)
	assert.ErrorIs(t, validateAppName("g8e"), constants.ErrPlatformEnrollmentInvalidAppName)
	assert.ErrorIs(t, validateAppName("g8eo"), constants.ErrPlatformEnrollmentInvalidAppName)
	assert.ErrorIs(t, validateAppName("g8ee"), constants.ErrPlatformEnrollmentInvalidAppName)
	assert.ErrorIs(t, validateAppName("operator"), constants.ErrPlatformEnrollmentInvalidAppName)
	assert.ErrorIs(t, validateAppName("INVALID-UPPER"), constants.ErrPlatformEnrollmentInvalidAppName)
	assert.ErrorIs(t, validateAppName("app/slash"), constants.ErrPlatformEnrollmentInvalidAppName)
	assert.NoError(t, validateAppName("g8e-eval"))
	assert.NoError(t, validateAppName("my-agent-1"))
	assert.NoError(t, validateAppName("custom.app"))
}

func TestLoadAppIdentity_Missing(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	cert, err := LoadAppIdentity(fileSvc, cfg, "my-agent")
	assert.Nil(t, cert)
	assert.ErrorIs(t, err, constants.ErrAppIdentityNotFound)
	assert.False(t, HasValidAppIdentity(fileSvc, cfg, "my-agent"))
}

func TestLoadAppIdentity_Expired(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	appName := "my-agent"
	certPEM, keyPEM := generateTestCertificateWithSPIFFE(t, appName, time.Now().Add(-1*time.Hour))

	certRel, err := fileSvc.RelFromAbs(cfg.AppCertFile(appName))
	require.NoError(t, err)
	keyRel, err := fileSvc.RelFromAbs(cfg.AppKeyFile(appName))
	require.NoError(t, err)

	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, []byte(certPEM), constants.PermFilePublic))
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, []byte(keyPEM), constants.PermFilePrivate))

	cert, err := LoadAppIdentity(fileSvc, cfg, appName)
	assert.Nil(t, cert)
	assert.ErrorIs(t, err, constants.ErrAppIdentityExpired)
	assert.False(t, HasValidAppIdentity(fileSvc, cfg, appName))
}

func TestLoadAppIdentity_Valid(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	appName := "my-agent"
	certPEM, keyPEM := generateTestCertificateWithSPIFFE(t, appName, time.Now().Add(24*time.Hour))

	certRel, err := fileSvc.RelFromAbs(cfg.AppCertFile(appName))
	require.NoError(t, err)
	keyRel, err := fileSvc.RelFromAbs(cfg.AppKeyFile(appName))
	require.NoError(t, err)

	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, []byte(certPEM), constants.PermFilePublic))
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, []byte(keyPEM), constants.PermFilePrivate))

	cert, err := LoadAppIdentity(fileSvc, cfg, appName)
	require.NoError(t, err)
	assert.NotNil(t, cert)
	assert.True(t, HasValidAppIdentity(fileSvc, cfg, appName))
}
