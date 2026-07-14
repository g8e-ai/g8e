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
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
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

// writeTestCredentials writes a credentials file with a synthetic CLISessionID so
// EnrollAgentApp can pass the LoadCredentials check without a real gateway session.
func writeTestCredentials(t *testing.T, fileSvc fs.RuntimeFileService, cfg *config.Config) {
	t.Helper()
	creds := &Credentials{
		UserID:       "test-user",
		CLISessionID: "test-session-id",
	}
	require.NoError(t, SaveCredentials(fileSvc, cfg, creds))
}

// writeTestCLICert generates a self-signed CLI cert and writes it to cfg.CLICertFile()/CLIKeyFile().
func writeTestCLICert(t *testing.T, cfg *config.Config) {
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

	require.NoError(t, os.MkdirAll(filepath.Dir(cfg.CLICertFile()), constants.PermDirPrivate))
	require.NoError(t, os.WriteFile(cfg.CLICertFile(), certPEM, constants.PermFilePrivate))
	require.NoError(t, os.WriteFile(cfg.CLIKeyFile(), keyPEM, constants.PermFilePrivate))
}

// startTLSEnrollServer starts a TLS test server with a localhost-valid cert and configures
// cfg to trust it via the CA bundle. Returns the server (cleanup registered via t.Cleanup).
func startTLSEnrollServer(t *testing.T, cfg *config.Config, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	// Generate a test CA.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          caSerial,
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	// Generate server cert signed by CA, valid for localhost / 127.0.0.1.
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serverSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	serverTemplate := &x509.Certificate{
		SerialNumber: serverSerial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	require.NoError(t, err)

	serverTLSCert := tls.Certificate{Certificate: [][]byte{serverDER}, PrivateKey: serverKey}

	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{serverTLSCert}}
	server.StartTLS()
	t.Cleanup(server.Close)

	// Write CA cert as trust bundle so EnrollAgentApp can verify the server.
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	caPath := filepath.Join(cfg.RuntimeDir, "test-ca.pem")
	require.NoError(t, os.MkdirAll(cfg.RuntimeDir, constants.PermDirPrivate))
	require.NoError(t, os.WriteFile(caPath, caPEM, constants.PermFilePrivate))
	cfg.Paths.Infra.CACertPath = caPath // absolute — TrustBundlePath() returns it directly
	cfg.Paths.Host = server.URL         // full URL — OperatorHTTPURL() returns it directly

	return server
}

// enrollResponse builds the standard enrollment success JSON with a fresh SPIFFE cert.
func enrollResponse(t *testing.T, agentName string) []byte {
	t.Helper()
	certPEM, _ := generateTestCertificateWithSPIFFE(t, agentName, time.Now().Add(365*24*time.Hour))
	resp := struct {
		Success bool   `json:"success"`
		AppCert string `json:"app_cert"`
		AppID   string `json:"app_id"`
	}{
		Success: true,
		AppCert: certPEM,
		AppID:   "spiffe://g8e.local/app/" + agentName,
	}
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	return b
}

// TestEnrollAgentApp_Idempotency_ValidCert tests that a valid cert (>7 days from expiry) is reused
// without contacting the gateway at all.
func TestEnrollAgentApp_Idempotency_ValidCert(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")
	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	agentName := "test-agent"
	certFile := cfg.AppCertFile(agentName)
	keyFile := cfg.AppKeyFile(agentName)

	// Create a valid cert with >7 days remaining
	certPEM, keyPEM := generateTestCertificateWithSPIFFE(t, agentName, time.Now().Add(30*24*time.Hour))
	require.NoError(t, os.MkdirAll(filepath.Dir(certFile), constants.PermDirPrivate))
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), constants.PermFilePrivate))
	require.NoError(t, os.WriteFile(keyFile, []byte(keyPEM), constants.PermFilePrivate))

	// No CLI cert, no CA bundle, no gateway — idempotency must short-circuit before any of that.
	appID, returnedCertFile, returnedKeyFile, err := EnrollAgentApp(fileSvc, cfg, agentName)

	require.NoError(t, err)
	assert.Equal(t, "spiffe://g8e.local/app/"+agentName, appID)
	assert.Equal(t, certFile, returnedCertFile)
	assert.Equal(t, keyFile, returnedKeyFile)
}

// TestEnrollAgentApp_Idempotency_ExpiringCert tests that an expiring cert (<7 days) triggers re-enrollment.
func TestEnrollAgentApp_Idempotency_ExpiringCert(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")
	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	agentName := "test-agent"
	certFile := cfg.AppCertFile(agentName)
	keyFile := cfg.AppKeyFile(agentName)

	// Create an expiring cert (<7 days remaining)
	certPEM, keyPEM := generateTestCertificateWithSPIFFE(t, agentName, time.Now().Add(3*24*time.Hour))
	require.NoError(t, os.MkdirAll(filepath.Dir(certFile), constants.PermDirPrivate))
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), constants.PermFilePrivate))
	require.NoError(t, os.WriteFile(keyFile, []byte(keyPEM), constants.PermFilePrivate))

	// Expiring cert → must re-enroll → needs TLS server + CLI cert + credentials
	startTLSEnrollServer(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, constants.APIPaths.PKIAppsDelegated, r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var req struct {
			CSR     string `json:"csr_pem"`
			AppName string `json:"app_name"`
			AppType string `json:"app_type"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, agentName, req.AppName)
		assert.Equal(t, "mcp-client", req.AppType)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write(enrollResponse(t, agentName))
	})
	writeTestCLICert(t, cfg)
	writeTestCredentials(t, fileSvc, cfg)

	appID, returnedCertFile, returnedKeyFile, err := EnrollAgentApp(fileSvc, cfg, agentName)

	require.NoError(t, err)
	assert.Equal(t, "spiffe://g8e.local/app/"+agentName, appID)
	assert.Equal(t, certFile, returnedCertFile)
	assert.Equal(t, keyFile, returnedKeyFile)
}

// TestEnrollAgentApp_NoCert tests enrollment when no cert exists.
func TestEnrollAgentApp_NoCert(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")
	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	agentName := "new-agent"
	certFile := cfg.AppCertFile(agentName)
	keyFile := cfg.AppKeyFile(agentName)

	startTLSEnrollServer(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, constants.APIPaths.PKIAppsDelegated, r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var req struct {
			CSR     string `json:"csr_pem"`
			AppName string `json:"app_name"`
			AppType string `json:"app_type"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, agentName, req.AppName)
		assert.Equal(t, "mcp-client", req.AppType)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write(enrollResponse(t, agentName))
	})
	require.NoError(t, os.MkdirAll(filepath.Dir(certFile), constants.PermDirPrivate))
	writeTestCLICert(t, cfg)
	writeTestCredentials(t, fileSvc, cfg)

	appID, returnedCertFile, returnedKeyFile, err := EnrollAgentApp(fileSvc, cfg, agentName)

	require.NoError(t, err)
	assert.Equal(t, "spiffe://g8e.local/app/"+agentName, appID)
	assert.Equal(t, certFile, returnedCertFile)
	assert.Equal(t, keyFile, returnedKeyFile)
	assert.FileExists(t, returnedCertFile)
	assert.FileExists(t, returnedKeyFile)
}

// TestEnrollAgentApp_NoURISAN tests re-enrollment when cert has no URI SAN.
func TestEnrollAgentApp_NoURISAN(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")
	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	agentName := "test-agent"
	certFile := cfg.AppCertFile(agentName)
	keyFile := cfg.AppKeyFile(agentName)

	// Create a cert without URI SAN
	csr, privKey, err := GenerateCSR(agentName)
	require.NoError(t, err)
	block, _ := pem.Decode([]byte(csr))
	require.NotNil(t, block)
	csrObj, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: agentName},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(30 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, csrObj.PublicKey, privKey)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	require.NoError(t, os.MkdirAll(filepath.Dir(certFile), constants.PermDirPrivate))
	require.NoError(t, os.WriteFile(certFile, certPEM, constants.PermFilePrivate))
	require.NoError(t, os.WriteFile(keyFile, keyPEM, constants.PermFilePrivate))

	startTLSEnrollServer(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, constants.APIPaths.PKIAppsDelegated, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write(enrollResponse(t, agentName))
	})
	writeTestCLICert(t, cfg)
	writeTestCredentials(t, fileSvc, cfg)

	appID, returnedCertFile, returnedKeyFile, err := EnrollAgentApp(fileSvc, cfg, agentName)

	require.NoError(t, err)
	assert.Equal(t, "spiffe://g8e.local/app/"+agentName, appID)
	assert.Equal(t, certFile, returnedCertFile)
	assert.Equal(t, keyFile, returnedKeyFile)
}

// TestEnrollAgentApp_InvalidCert tests re-enrollment when cert is invalid (unparseable).
func TestEnrollAgentApp_InvalidCert(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")
	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	agentName := "test-agent"
	certFile := cfg.AppCertFile(agentName)
	keyFile := cfg.AppKeyFile(agentName)

	// Write invalid cert data
	require.NoError(t, os.MkdirAll(filepath.Dir(certFile), constants.PermDirPrivate))
	require.NoError(t, os.WriteFile(certFile, []byte("invalid-cert-data"), constants.PermFilePrivate))
	require.NoError(t, os.WriteFile(keyFile, []byte("invalid-key-data"), constants.PermFilePrivate))

	startTLSEnrollServer(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, constants.APIPaths.PKIAppsDelegated, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write(enrollResponse(t, agentName))
	})
	writeTestCLICert(t, cfg)
	writeTestCredentials(t, fileSvc, cfg)

	appID, returnedCertFile, returnedKeyFile, err := EnrollAgentApp(fileSvc, cfg, agentName)

	require.NoError(t, err)
	assert.Equal(t, "spiffe://g8e.local/app/"+agentName, appID)
	assert.Equal(t, certFile, returnedCertFile)
	assert.Equal(t, keyFile, returnedKeyFile)
}

// TestEnrollAgentApp_EnrollmentError tests error handling when enrollment fails.
func TestEnrollAgentApp_EnrollmentError(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")
	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	agentName := "test-agent"

	startTLSEnrollServer(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
	})
	writeTestCLICert(t, cfg)
	writeTestCredentials(t, fileSvc, cfg)

	_, _, _, err := EnrollAgentApp(fileSvc, cfg, agentName)

	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrHTTPStatusError))
}

// TestEnrollAgentApp_GatewayUnreachable tests error handling when the gateway is unreachable.
func TestEnrollAgentApp_GatewayUnreachable(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")
	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	agentName := "test-agent"

	// CLI cert, CA bundle, and credentials must exist so we reach the POST before failing.
	writeTestCLICert(t, cfg)
	writeTestCredentials(t, fileSvc, cfg)
	dummyCert, _ := generateTestCertificateWithSPIFFE(t, "dummy", time.Now().Add(24*time.Hour))
	caPath := filepath.Join(runtimeDir, "test-ca.pem")
	require.NoError(t, os.WriteFile(caPath, []byte(dummyCert), constants.PermFilePrivate))
	cfg.Paths.Infra.CACertPath = caPath

	_, _, _, err := EnrollAgentApp(fileSvc, cfg, agentName)

	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrHTTPRequestExecuteFailed))
}

// TestEnrollAgentApp_NoCLICredentials tests error handling when CLI credentials are missing.
func TestEnrollAgentApp_NoCLICredentials(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")
	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	agentName := "test-agent"

	// Write CLI cert and CA bundle but no credentials
	writeTestCLICert(t, cfg)
	dummyCert, _ := generateTestCertificateWithSPIFFE(t, "dummy", time.Now().Add(24*time.Hour))
	caPath := filepath.Join(runtimeDir, "test-ca.pem")
	require.NoError(t, os.WriteFile(caPath, []byte(dummyCert), constants.PermFilePrivate))
	cfg.Paths.Infra.CACertPath = caPath

	_, _, _, err := EnrollAgentApp(fileSvc, cfg, agentName)

	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrNotAuthenticated))
}

// TestEnrollAgentApp_MissingCLICert tests error handling when CLI cert is missing.
func TestEnrollAgentApp_MissingCLICert(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")
	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	agentName := "test-agent"

	// Write credentials and CA bundle but no CLI cert
	writeTestCredentials(t, fileSvc, cfg)
	dummyCert, _ := generateTestCertificateWithSPIFFE(t, "dummy", time.Now().Add(24*time.Hour))
	caPath := filepath.Join(runtimeDir, "test-ca.pem")
	require.NoError(t, os.WriteFile(caPath, []byte(dummyCert), constants.PermFilePrivate))
	cfg.Paths.Infra.CACertPath = caPath

	_, _, _, err := EnrollAgentApp(fileSvc, cfg, agentName)

	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrFailedToLoadClientCertificate))
}

// TestEnrollAgentApp_MissingCABundle tests error handling when CA bundle is missing.
func TestEnrollAgentApp_MissingCABundle(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")
	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	agentName := "test-agent"

	// Write CLI cert and credentials but no CA bundle
	writeTestCLICert(t, cfg)
	writeTestCredentials(t, fileSvc, cfg)

	_, _, _, err := EnrollAgentApp(fileSvc, cfg, agentName)

	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrFailedToReadTrustBundle))
}

// TestEnrollAgentApp_WrongSPIFFEID tests re-enrollment when cert has wrong SPIFFE ID.
func TestEnrollAgentApp_WrongSPIFFEID(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	runtimeDir := fileSvc.Resolve("")
	cfg := &config.Config{
		ProjectRoot: runtimeDir,
		RuntimeDir:  runtimeDir,
		Paths:       &config.PathsConfig{},
	}

	agentName := "test-agent"
	certFile := cfg.AppCertFile(agentName)
	keyFile := cfg.AppKeyFile(agentName)

	// Create a cert with a different SPIFFE ID
	certPEM, keyPEM := generateTestCertificateWithSPIFFE(t, "different-agent", time.Now().Add(30*24*time.Hour))
	require.NoError(t, os.MkdirAll(filepath.Dir(certFile), constants.PermDirPrivate))
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), constants.PermFilePrivate))
	require.NoError(t, os.WriteFile(keyFile, []byte(keyPEM), constants.PermFilePrivate))

	startTLSEnrollServer(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, constants.APIPaths.PKIAppsDelegated, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write(enrollResponse(t, agentName))
	})
	writeTestCLICert(t, cfg)
	writeTestCredentials(t, fileSvc, cfg)

	appID, returnedCertFile, returnedKeyFile, err := EnrollAgentApp(fileSvc, cfg, agentName)

	require.NoError(t, err)
	assert.Equal(t, "spiffe://g8e.local/app/"+agentName, appID)
	assert.Equal(t, certFile, returnedCertFile)
	assert.Equal(t, keyFile, returnedKeyFile)
}

// TestCheckExistingAppCert_NoFile tests checkExistingAppCert when cert file doesn't exist.
func TestCheckExistingAppCert_NoFile(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	certFile := fileSvc.Resolve("nonexistent-cert.pem")

	appID, ok := checkExistingAppCert(fileSvc, certFile, "test-agent")

	assert.Empty(t, appID)
	assert.False(t, ok)
}

// TestCheckExistingAppCert_InvalidPEM tests checkExistingAppCert with invalid PEM data.
func TestCheckExistingAppCert_InvalidPEM(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	certFile := fileSvc.Resolve("invalid-cert.pem")
	require.NoError(t, os.WriteFile(certFile, []byte("not-valid-pem"), constants.PermFilePrivate))

	appID, ok := checkExistingAppCert(fileSvc, certFile, "test-agent")

	assert.Empty(t, appID)
	assert.False(t, ok)
}

// TestCheckExistingAppCert_InvalidCertificate tests checkExistingAppCert with unparseable certificate.
func TestCheckExistingAppCert_InvalidCertificate(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	certFile := fileSvc.Resolve("invalid-cert.pem")
	// Write a PEM block that's not a valid certificate
	invalidPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("not-a-valid-certificate"),
	})
	require.NoError(t, os.WriteFile(certFile, invalidPEM, constants.PermFilePrivate))

	appID, ok := checkExistingAppCert(fileSvc, certFile, "test-agent")

	assert.Empty(t, appID)
	assert.False(t, ok)
}

// TestCheckExistingAppCert_ExpiringSoon tests checkExistingAppCert with cert expiring soon.
func TestCheckExistingAppCert_ExpiringSoon(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	certFile := fileSvc.Resolve("expiring-cert.pem")
	agentName := "test-agent"

	// Create cert expiring in 3 days (< 7 day threshold)
	certPEM, _ := generateTestCertificateWithSPIFFE(t, agentName, time.Now().Add(3*24*time.Hour))
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), constants.PermFilePrivate))

	appID, ok := checkExistingAppCert(fileSvc, certFile, agentName)

	assert.Empty(t, appID)
	assert.False(t, ok)
}

// TestCheckExistingAppCert_ValidWithCorrectSPIFFE tests checkExistingAppCert with valid cert and correct SPIFFE ID.
func TestCheckExistingAppCert_ValidWithCorrectSPIFFE(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	certFile := fileSvc.Resolve("valid-cert.pem")
	agentName := "test-agent"

	// Create valid cert with >7 days remaining and correct SPIFFE ID
	certPEM, _ := generateTestCertificateWithSPIFFE(t, agentName, time.Now().Add(30*24*time.Hour))
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), constants.PermFilePrivate))

	appID, ok := checkExistingAppCert(fileSvc, certFile, agentName)

	expectedID := "spiffe://g8e.local/app/" + agentName
	assert.Equal(t, expectedID, appID)
	assert.True(t, ok)
}

// TestCheckExistingAppCert_ValidWithWrongSPIFFE tests checkExistingAppCert with valid cert but wrong SPIFFE ID.
func TestCheckExistingAppCert_ValidWithWrongSPIFFE(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	certFile := fileSvc.Resolve("valid-cert.pem")
	agentName := "test-agent"

	// Create valid cert with >7 days remaining but different SPIFFE ID
	certPEM, _ := generateTestCertificateWithSPIFFE(t, "different-agent", time.Now().Add(30*24*time.Hour))
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), constants.PermFilePrivate))

	appID, ok := checkExistingAppCert(fileSvc, certFile, agentName)

	assert.Empty(t, appID)
	assert.False(t, ok)
}

// TestCheckExistingAppCert_ExactlyAtThreshold tests checkExistingAppCert with cert at exactly 7 days.
func TestCheckExistingAppCert_ExactlyAtThreshold(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	certFile := fileSvc.Resolve("threshold-cert.pem")
	agentName := "test-agent"

	// Create cert expiring exactly at 7 days (should be rejected)
	certPEM, _ := generateTestCertificateWithSPIFFE(t, agentName, time.Now().Add(7*24*time.Hour))
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), constants.PermFilePrivate))

	appID, ok := checkExistingAppCert(fileSvc, certFile, agentName)

	assert.Empty(t, appID)
	assert.False(t, ok)
}

// TestCheckExistingAppCert_JustAboveThreshold tests checkExistingAppCert with cert just above 7 days.
func TestCheckExistingAppCert_JustAboveThreshold(t *testing.T) {
	t.Parallel()

	fileSvc := newAuthTestFileSvc(t)
	certFile := fileSvc.Resolve("valid-cert.pem")
	agentName := "test-agent"

	// Create cert expiring in 7 days + 1 second (should be accepted)
	certPEM, _ := generateTestCertificateWithSPIFFE(t, agentName, time.Now().Add(7*24*time.Hour+time.Second))
	require.NoError(t, os.WriteFile(certFile, []byte(certPEM), constants.PermFilePrivate))

	appID, ok := checkExistingAppCert(fileSvc, certFile, agentName)

	expectedID := "spiffe://g8e.local/app/" + agentName
	assert.Equal(t, expectedID, appID)
	assert.True(t, ok)
}
