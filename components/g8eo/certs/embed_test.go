// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestCAPEM creates a self-signed CA certificate and returns its PEM bytes.
// Uses ECDSA P-256 for speed. The certificate is valid for 1 hour.
func generateTestCAPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}

// saveAndRestoreCA saves the current CA value and restores it after the test.
func saveAndRestoreCA(t *testing.T) {
	t.Helper()
	original := GetRawCA()
	t.Cleanup(func() { SetCA(original) })
}

func TestGetRawCA_ReturnsStoredValue(t *testing.T) {
	saveAndRestoreCA(t)
	SetCA(nil)
	assert.Nil(t, GetRawCA())
}

func TestSetCA_StoresBytes(t *testing.T) {
	saveAndRestoreCA(t)
	pem := []byte("fake-pem-data")
	SetCA(pem)
	assert.Equal(t, pem, GetRawCA())
}

func TestSetCA_OverwritesPreviousValue(t *testing.T) {
	saveAndRestoreCA(t)
	SetCA([]byte("first"))
	SetCA([]byte("second"))
	assert.Equal(t, []byte("second"), GetRawCA())
}

func TestSetCA_NilClearsValue(t *testing.T) {
	saveAndRestoreCA(t)
	SetCA([]byte("some-ca"))
	SetCA(nil)
	assert.Nil(t, GetRawCA())
}

func TestGetServerCARootCAs_WhenCANotSet(t *testing.T) {
	saveAndRestoreCA(t)
	SetCA(nil)
	pool, err := GetServerCARootCAs()
	assert.Nil(t, pool)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server CA not set")
}

func TestGetServerCARootCAs_InvalidPEM(t *testing.T) {
	saveAndRestoreCA(t)
	SetCA([]byte("not-a-valid-pem-block"))
	pool, err := GetServerCARootCAs()
	assert.Nil(t, pool)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestGetServerCARootCAs_ValidPEM(t *testing.T) {
	saveAndRestoreCA(t)
	caBytes := generateTestCAPEM(t)
	SetCA(caBytes)

	pool, err := GetServerCARootCAs()
	require.NoError(t, err)
	assert.NotNil(t, pool)
}

func TestGetTLSConfig_WhenCANotSet(t *testing.T) {
	saveAndRestoreCA(t)
	SetCA(nil)
	cfg, err := GetTLSConfig()
	assert.Nil(t, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server CA not set")
}

func TestGetTLSConfig_InvalidPEM(t *testing.T) {
	saveAndRestoreCA(t)
	SetCA([]byte("not-pem"))
	cfg, err := GetTLSConfig()
	assert.Nil(t, cfg)
	require.Error(t, err)
}

func TestGetTLSConfig_MinVersionAndCurves(t *testing.T) {
	saveAndRestoreCA(t)
	caBytes := generateTestCAPEM(t)
	SetCA(caBytes)

	cfg, err := GetTLSConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	assert.NotNil(t, cfg.RootCAs)
	assert.Contains(t, cfg.CurvePreferences, tls.X25519)
	assert.Contains(t, cfg.CurvePreferences, tls.CurveP384)
	assert.Contains(t, cfg.CurvePreferences, tls.CurveP256)
}
