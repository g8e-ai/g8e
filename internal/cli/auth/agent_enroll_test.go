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

package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestCertificateWithSPIFFE generates a test certificate with a SPIFFE URI SAN
func generateTestCertificateWithSPIFFE(t *testing.T, agentName string, notAfter time.Time) (certPEM string, keyPEM string) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	spiFFEID := "spiffe://g8e.local/app/" + agentName
	uri, err := url.Parse(spiFFEID)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: agentName,
		},
		NotBefore: time.Now(),
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

// TestEnrollAgentApp_Idempotency_ValidCert tests that a valid cert (>7 days from expiry) is reused
func TestEnrollAgentApp_Idempotency_ValidCert(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:      tmpDir,
		RuntimeDir:       filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:           filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:       filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir:   tmpDir,
		Paths:            &config.PathsConfig{},
		TestPortOverride: 59999, // Use non-existent port to ensure gateway is not reachable
	}

	agentName := "test-agent"
	certFile := cfg.AppCertFile(agentName)
	keyFile := cfg.AppKeyFile(agentName)

	// Create a valid cert with >7 days remaining
	certPEM, keyPEM := generateTestCertificateWithSPIFFE(t, agentName, time.Now().Add(30*24*time.Hour))
	require.NoError(t, os.MkdirAll(filepath.Dir(certFile), 0700))
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), 0600))
	require.NoError(t, os.WriteFile(keyFile, []byte(keyPEM), 0600))

	// Call EnrollAgentApp - should reuse the existing cert without contacting gateway
	appID, returnedCertFile, returnedKeyFile, err := EnrollAgentApp(cfg, agentName)

	require.NoError(t, err)
	assert.Equal(t, "spiffe://g8e.local/app/"+agentName, appID)
	assert.Equal(t, certFile, returnedCertFile)
	assert.Equal(t, keyFile, returnedKeyFile)
}

// TestEnrollAgentApp_Idempotency_ExpiringCert tests that an expiring cert (<7 days) triggers re-enrollment
func TestEnrollAgentApp_Idempotency_ExpiringCert(t *testing.T) {
	t.Parallel()

	// Mock server to handle enrollment request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, constants.APIPaths.PKIAppsEnroll, r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var req struct {
			CSR     string `json:"csr_pem"`
			AppName string `json:"app_name"`
			AppType string `json:"app_type"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "test-agent", req.AppName)
		assert.Equal(t, "mcp-client", req.AppType)

		// Return successful enrollment response
		certPEM, _ := generateTestCertificateWithSPIFFE(t, "test-agent", time.Now().Add(365*24*time.Hour))
		resp := struct {
			Success     bool   `json:"success"`
			AppCert     string `json:"app_cert"`
			CertChain   string `json:"cert_chain"`
			TrustBundle string `json:"trust_bundle"`
			AppID       string `json:"app_id"`
		}{
			Success:     true,
			AppCert:     certPEM,
			CertChain:   "",
			TrustBundle: "",
			AppID:       "spiffe://g8e.local/app/test-agent",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:      tmpDir,
		RuntimeDir:       filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:           filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:       filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir:   tmpDir,
		Paths:            &config.PathsConfig{},
		TestPortOverride: extractPortFromURL(server.URL),
	}

	agentName := "test-agent"
	certFile := cfg.AppCertFile(agentName)
	keyFile := cfg.AppKeyFile(agentName)

	// Create an expiring cert (<7 days remaining)
	certPEM, keyPEM := generateTestCertificateWithSPIFFE(t, agentName, time.Now().Add(3*24*time.Hour))
	require.NoError(t, os.MkdirAll(filepath.Dir(certFile), 0700))
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), 0600))
	require.NoError(t, os.WriteFile(keyFile, []byte(keyPEM), 0600))

	// Call EnrollAgentApp - should re-enroll due to expiring cert
	appID, returnedCertFile, returnedKeyFile, err := EnrollAgentApp(cfg, agentName)

	require.NoError(t, err)
	assert.Equal(t, "spiffe://g8e.local/app/"+agentName, appID)
	assert.Equal(t, certFile, returnedCertFile)
	assert.Equal(t, keyFile, returnedKeyFile)
}

// TestEnrollAgentApp_NoCert tests enrollment when no cert exists
func TestEnrollAgentApp_NoCert(t *testing.T) {
	t.Parallel()

	// Mock server to handle enrollment request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, constants.APIPaths.PKIAppsEnroll, r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var req struct {
			CSR     string `json:"csr_pem"`
			AppName string `json:"app_name"`
			AppType string `json:"app_type"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "new-agent", req.AppName)
		assert.Equal(t, "mcp-client", req.AppType)

		// Return successful enrollment response
		certPEM, _ := generateTestCertificateWithSPIFFE(t, "new-agent", time.Now().Add(365*24*time.Hour))
		resp := struct {
			Success     bool   `json:"success"`
			AppCert     string `json:"app_cert"`
			CertChain   string `json:"cert_chain"`
			TrustBundle string `json:"trust_bundle"`
			AppID       string `json:"app_id"`
		}{
			Success:     true,
			AppCert:     certPEM,
			CertChain:   "",
			TrustBundle: "",
			AppID:       "spiffe://g8e.local/app/new-agent",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:      tmpDir,
		RuntimeDir:       filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:           filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:       filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir:   tmpDir,
		Paths:            &config.PathsConfig{},
		TestPortOverride: extractPortFromURL(server.URL),
	}

	agentName := "new-agent"

	// Call EnrollAgentApp - should enroll since no cert exists
	appID, certFile, keyFile, err := EnrollAgentApp(cfg, agentName)

	require.NoError(t, err)
	assert.Equal(t, "spiffe://g8e.local/app/"+agentName, appID)
	assert.FileExists(t, certFile)
	assert.FileExists(t, keyFile)
}

// TestEnrollAgentApp_NoURISAN tests re-enrollment when cert has no URI SAN
func TestEnrollAgentApp_NoURISAN(t *testing.T) {
	t.Parallel()

	// Mock server to handle enrollment request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, constants.APIPaths.PKIAppsEnroll, r.URL.Path)

		// Return successful enrollment response
		certPEM, _ := generateTestCertificateWithSPIFFE(t, "test-agent", time.Now().Add(365*24*time.Hour))
		resp := struct {
			Success     bool   `json:"success"`
			AppCert     string `json:"app_cert"`
			CertChain   string `json:"cert_chain"`
			TrustBundle string `json:"trust_bundle"`
			AppID       string `json:"app_id"`
		}{
			Success:     true,
			AppCert:     certPEM,
			CertChain:   "",
			TrustBundle: "",
			AppID:       "spiffe://g8e.local/app/test-agent",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:      tmpDir,
		RuntimeDir:       filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:           filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:       filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir:   tmpDir,
		Paths:            &config.PathsConfig{},
		TestPortOverride: extractPortFromURL(server.URL),
	}

	agentName := "test-agent"
	certFile := cfg.AppCertFile(agentName)
	keyFile := cfg.AppKeyFile(agentName)

	// Create a cert without URI SAN (using a different generator)
	csr, privKey, err := GenerateCSR(agentName)
	require.NoError(t, err)

	// Parse the CSR to get the public key
	block, _ := pem.Decode([]byte(csr))
	require.NotNil(t, block)
	csrObj, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)

	// Create a cert without URI SAN
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: agentName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(30 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, csrObj.PublicKey, privKey)
	require.NoError(t, err)

	certPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	}))

	keyBytes, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)

	keyPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	}))

	require.NoError(t, os.MkdirAll(filepath.Dir(certFile), 0700))
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), 0600))
	require.NoError(t, os.WriteFile(keyFile, []byte(keyPEM), 0600))

	// Call EnrollAgentApp - should re-enroll due to missing URI SAN
	appID, returnedCertFile, returnedKeyFile, err := EnrollAgentApp(cfg, agentName)

	require.NoError(t, err)
	assert.Equal(t, "spiffe://g8e.local/app/"+agentName, appID)
	assert.Equal(t, certFile, returnedCertFile)
	assert.Equal(t, keyFile, returnedKeyFile)
}

// TestEnrollAgentApp_InvalidCert tests re-enrollment when cert is invalid
func TestEnrollAgentApp_InvalidCert(t *testing.T) {
	t.Parallel()

	// Mock server to handle enrollment request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, constants.APIPaths.PKIAppsEnroll, r.URL.Path)

		// Return successful enrollment response
		certPEM, _ := generateTestCertificateWithSPIFFE(t, "test-agent", time.Now().Add(365*24*time.Hour))
		resp := struct {
			Success     bool   `json:"success"`
			AppCert     string `json:"app_cert"`
			CertChain   string `json:"cert_chain"`
			TrustBundle string `json:"trust_bundle"`
			AppID       string `json:"app_id"`
		}{
			Success:     true,
			AppCert:     certPEM,
			CertChain:   "",
			TrustBundle: "",
			AppID:       "spiffe://g8e.local/app/test-agent",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:      tmpDir,
		RuntimeDir:       filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:           filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:       filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir:   tmpDir,
		Paths:            &config.PathsConfig{},
		TestPortOverride: extractPortFromURL(server.URL),
	}

	agentName := "test-agent"
	certFile := cfg.AppCertFile(agentName)
	keyFile := cfg.AppKeyFile(agentName)

	// Create invalid cert data
	require.NoError(t, os.MkdirAll(filepath.Dir(certFile), 0700))
	require.NoError(t, os.WriteFile(certFile, []byte("invalid-cert-data"), 0600))
	require.NoError(t, os.WriteFile(keyFile, []byte("invalid-key-data"), 0600))

	// Call EnrollAgentApp - should re-enroll due to invalid cert
	appID, returnedCertFile, returnedKeyFile, err := EnrollAgentApp(cfg, agentName)

	require.NoError(t, err)
	assert.Equal(t, "spiffe://g8e.local/app/"+agentName, appID)
	assert.Equal(t, certFile, returnedCertFile)
	assert.Equal(t, keyFile, returnedKeyFile)
}

// TestEnrollAgentApp_EnrollmentError tests error handling when enrollment fails
func TestEnrollAgentApp_EnrollmentError(t *testing.T) {
	t.Parallel()

	// Mock server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:      tmpDir,
		RuntimeDir:       filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:           filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:       filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir:   tmpDir,
		Paths:            &config.PathsConfig{},
		TestPortOverride: extractPortFromURL(server.URL),
	}

	agentName := "test-agent"

	// Call EnrollAgentApp - should fail
	_, _, _, err := EnrollAgentApp(cfg, agentName)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "enrollment failed")
}

// TestEnrollAgentApp_GatewayUnreachable tests error handling when gateway is unreachable
func TestEnrollAgentApp_GatewayUnreachable(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:      tmpDir,
		RuntimeDir:       filepath.Join(tmpDir, constants.Paths.Infra.RuntimeDir),
		PKIDir:           filepath.Join(tmpDir, constants.Paths.Infra.PkiDir),
		SecretsDir:       filepath.Join(tmpDir, constants.Paths.Infra.SecretsDir),
		CredentialsDir:   tmpDir,
		Paths:            &config.PathsConfig{},
		TestPortOverride: 59999, // Use non-existent port to ensure gateway is not reachable
	}

	agentName := "test-agent"

	// Call EnrollAgentApp - should fail due to unreachable gateway
	_, _, _, err := EnrollAgentApp(cfg, agentName)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to POST enrollment request")
}
