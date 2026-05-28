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

package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	clierrors "github.com/g8e-ai/g8e/internal/cli/errors"
	"github.com/g8e-ai/g8e/internal/constants"
)

func generateTestCert(t *testing.T) (certPEM, keyPEM []byte, privKey *ecdsa.PrivateKey) {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "test-client"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &privKey.PublicKey, privKey)
	require.NoError(t, err)

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	keyBytes, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	return certPEM, keyPEM, privKey
}

func generateTestCA(t *testing.T) (caCertPEM []byte) {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "g8e Test CA"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &privKey.PublicKey, privKey)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func setupTestConfig(t *testing.T) (*config.Config, string) {
	t.Helper()

	tempDir := t.TempDir()

	projectRoot := filepath.Join(tempDir, "project")
	require.NoError(t, os.MkdirAll(projectRoot, 0755))

	runtimeDir := filepath.Join(projectRoot, ".g8e")
	require.NoError(t, os.MkdirAll(runtimeDir, 0755))

	pkiDir := filepath.Join(projectRoot, ".g8e", "pki")
	require.NoError(t, os.MkdirAll(pkiDir, 0755))

	secretsDir := filepath.Join(projectRoot, ".g8e", "secrets")
	require.NoError(t, os.MkdirAll(secretsDir, 0755))

	credentialsDir := filepath.Join(tempDir, "credentials")
	require.NoError(t, os.MkdirAll(credentialsDir, 0700))

	protocolDir := filepath.Join(projectRoot, "protocol")
	constantsDir := filepath.Join(protocolDir, "constants")
	require.NoError(t, os.MkdirAll(constantsDir, 0755))

	pathsJSON := `{
		"host": "localhost",
		"infra": {
			"app_cert_dir": "/tmp/app/certs",
			"ca_cert_path": "pki/ca.crt",
			"db_path": "/tmp/db",
			"docs_dir": "/tmp/docs",
			"pki_dir": "pki",
			"protocol_constants_dir": "protocol/constants",
			"protocol_dir": "protocol",
			"protocol_models_dir": "protocol/models",
			"secrets_dir": "secrets",
			"ssh_config_path": "/tmp/ssh/config"
		},
		"ports": {
			"insecure_mcp_gateway": 18789,
			"operator_bootstrap_https": 8441,
			"operator_https": 8440,
			"operator_public_https": 8442
		}
	}`
	pathsPath := filepath.Join(constantsDir, "paths.json")
	require.NoError(t, os.WriteFile(pathsPath, []byte(pathsJSON), 0644))

	caCertPEM := generateTestCA(t)
	trustBundlePath := filepath.Join(projectRoot, ".g8e", "pki", "trust", "hub-bundle.pem")
	require.NoError(t, os.MkdirAll(filepath.Dir(trustBundlePath), 0755))
	require.NoError(t, os.WriteFile(trustBundlePath, caCertPEM, 0644))

	cfg, err := config.Load(projectRoot)
	require.NoError(t, err)

	// Override credentials directory to use temp directory for test isolation
	cfg.CredentialsDir = credentialsDir

	return cfg, tempDir
}

func setupTestCredentials(t *testing.T, cfg *config.Config) {
	t.Helper()

	creds := &auth.Credentials{
		OperatorSessionID: "test-operator-session-id",
		UserID:            "test-user-id",
		OperatorID:        "test-operator-id",
		CLISessionID:      "test-cli-session-id",
	}

	require.NoError(t, auth.SaveCredentials(cfg, creds))

	certPEM, keyPEM, _ := generateTestCert(t)
	require.NoError(t, os.WriteFile(cfg.CLICertFile(), certPEM, 0600))
	require.NoError(t, os.WriteFile(cfg.CLIKeyFile(), keyPEM, 0600))
}

func setupTLSClient(t *testing.T, cfg *config.Config, server *httptest.Server) *Client {
	t.Helper()

	_, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	var port int
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)
	cfg.Paths.Ports.OperatorHTTPS = port

	client, err := NewClient(cfg)
	require.NoError(t, err)

	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(server.Certificate())

	transport := client.httpClient.Transport.(*http.Transport)
	transport.TLSClientConfig.RootCAs = caCertPool

	return client
}

func newLocalhostTLSServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &privKey.PublicKey, privKey)
	require.NoError(t, err)

	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  privKey,
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	server := httptest.NewUnstartedServer(handler)
	server.TLS = tlsConfig
	server.StartTLS()

	return server
}

func TestNewClient_Success(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	client, err := NewClient(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	assert.NotNil(t, client.httpClient)
	assert.Equal(t, cfg, client.cfg)
	assert.NotNil(t, client.creds)
	assert.Equal(t, 30*time.Second, client.httpClient.Timeout)

	transport, ok := client.httpClient.Transport.(*http.Transport)
	require.True(t, ok)
	assert.NotNil(t, transport.TLSClientConfig)
	assert.Equal(t, uint16(tls.VersionTLS13), transport.TLSClientConfig.MinVersion)
	assert.Len(t, transport.TLSClientConfig.Certificates, 1)
	assert.NotNil(t, transport.TLSClientConfig.RootCAs)
}

func TestNewClient_NoCredentials(t *testing.T) {
	cfg, _ := setupTestConfig(t)

	credsDir := cfg.CredentialsDir
	os.RemoveAll(credsDir)

	client, err := NewClient(cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	assert.True(t, errors.Is(err, clierrors.ErrNotAuthenticated))
}

func TestNewClient_LoadCredentialsError(t *testing.T) {
	cfg, _ := setupTestConfig(t)

	credsDir := cfg.CredentialsDir
	require.NoError(t, os.MkdirAll(credsDir, 0700))

	credsFile := cfg.CredentialsFile()
	require.NoError(t, os.WriteFile(credsFile, []byte("invalid json"), 0600))

	client, err := NewClient(cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	assert.True(t, errors.Is(err, clierrors.ErrFailedToLoadCredentials))
}

func TestNewClient_MissingCertFile(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	require.NoError(t, os.Remove(cfg.CLICertFile()))

	client, err := NewClient(cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	assert.True(t, errors.Is(err, clierrors.ErrFailedToLoadClientCertificate))
}

func TestNewClient_MissingKeyFile(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	require.NoError(t, os.Remove(cfg.CLIKeyFile()))

	client, err := NewClient(cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	assert.True(t, errors.Is(err, clierrors.ErrFailedToLoadClientCertificate))
}

func TestNewClient_MissingTrustBundle(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	caCertPath := cfg.TrustBundlePath()
	require.NoError(t, os.Remove(caCertPath))

	client, err := NewClient(cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	assert.True(t, errors.Is(err, clierrors.ErrFailedToReadTrustBundle))
}

func TestNewClient_InvalidTrustBundle(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	caCertPath := cfg.TrustBundlePath()
	require.NoError(t, os.WriteFile(caCertPath, []byte("not a valid PEM"), 0644))

	client, err := NewClient(cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	assert.True(t, errors.Is(err, clierrors.ErrFailedToParseTrustBundle))
}

func TestDoRequest_Success(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/test", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "test-operator-session-id", r.Header.Get(constants.HeaderOperatorSessionID))
		assert.Equal(t, "test-cli-session-id", r.Header.Get(constants.HeaderCLISessionID))
		assert.Equal(t, "test-user-id", r.Header.Get(constants.HeaderUserID))
		assert.Equal(t, "test-operator-id", r.Header.Get(constants.HeaderOperatorID))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"success"}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, cfg, server)

	body := map[string]string{"key": "value"}
	resp, err := client.DoRequest("POST", "/api/test", body)
	require.NoError(t, err)
	assert.Equal(t, `{"result":"success"}`, string(resp))
}

func TestDoRequest_GetWithoutBody(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":"test"}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, cfg, server)

	resp, err := client.DoRequest("GET", "/api/test", nil)
	require.NoError(t, err)
	assert.Equal(t, `{"data":"test"}`, string(resp))
}

func TestDoRequest_MarshalError(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	client, err := NewClient(cfg)
	require.NoError(t, err)

	body := make(chan int)
	_, err = client.DoRequest("POST", "https://localhost:9999/api/test", body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal request body")
}

func TestDoRequest_HTTPError(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	client, err := NewClient(cfg)
	require.NoError(t, err)

	_, err = client.DoRequest("GET", "/invalid-endpoint", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute request")
}

func TestDoRequest_APIError(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, cfg, server)

	_, err := client.DoRequest("GET", "/api/test", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
	assert.Contains(t, err.Error(), "not found")
}

func TestDoRequest_InvalidJSONResponse(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not valid json`))
	}))
	defer server.Close()

	client := setupTLSClient(t, cfg, server)

	_, err := client.DoRequest("GET", "/api/test", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON response")
}

func TestDoRequest_ReadResponseError(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		hj, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hj.Hijack()
			conn.Close()
		}
	}))
	defer server.Close()

	client := setupTLSClient(t, cfg, server)

	_, err := client.DoRequest("GET", "/api/test", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read response")
}

func TestGet_Success(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"get":"success"}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, cfg, server)

	resp, err := client.Get("/api/test")
	require.NoError(t, err)
	assert.Equal(t, `{"get":"success"}`, string(resp))
}

func TestPost_Success(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"post":"success"}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, cfg, server)

	body := map[string]string{"data": "test"}
	resp, err := client.Post("/api/test", body)
	require.NoError(t, err)
	assert.Equal(t, `{"post":"success"}`, string(resp))
}

func TestPut_Success(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "PUT", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"put":"success"}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, cfg, server)

	body := map[string]string{"data": "updated"}
	resp, err := client.Put("/api/test", body)
	require.NoError(t, err)
	assert.Equal(t, `{"put":"success"}`, string(resp))
}

func TestDelete_Success(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "DELETE", r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"delete":"success"}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, cfg, server)

	resp, err := client.Delete("/api/test")
	require.NoError(t, err)
	assert.Equal(t, `{"delete":"success"}`, string(resp))
}

func TestDoRequest_HeadersSetCorrectly(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	var receivedHeaders http.Header
	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, cfg, server)

	_, err := client.DoRequest("POST", "/api/test", map[string]string{"test": "data"})
	require.NoError(t, err)

	assert.Equal(t, "test-operator-session-id", receivedHeaders.Get(constants.HeaderOperatorSessionID))
	assert.Equal(t, "test-cli-session-id", receivedHeaders.Get(constants.HeaderCLISessionID))
	assert.Equal(t, "test-user-id", receivedHeaders.Get(constants.HeaderUserID))
	assert.Equal(t, "test-operator-id", receivedHeaders.Get(constants.HeaderOperatorID))
	assert.Equal(t, "application/json", receivedHeaders.Get("Content-Type"))
}

func TestDoRequest_URLConstruction(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	var receivedURL string
	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, cfg, server)

	_, err := client.DoRequest("GET", "/api/v1/resource", nil)
	require.NoError(t, err)

	assert.Equal(t, "/api/v1/resource", receivedURL)
}

func TestNewClient_TLSConfig(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	client, err := NewClient(cfg)
	require.NoError(t, err)

	transport, ok := client.httpClient.Transport.(*http.Transport)
	require.True(t, ok)

	tlsConfig := transport.TLSClientConfig
	require.NotNil(t, tlsConfig)

	assert.Equal(t, uint16(tls.VersionTLS13), tlsConfig.MinVersion)
	assert.Len(t, tlsConfig.Certificates, 1)
	assert.NotNil(t, tlsConfig.RootCAs)
	assert.False(t, tlsConfig.InsecureSkipVerify)
}

func TestDoRequest_ResponseBodyValidation(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	setupTestCredentials(t, cfg)

	testCases := []struct {
		name       string
		response   string
		statusCode int
		expectErr  bool
		errMsg     string
	}{
		{
			name:       "valid JSON",
			response:   `{"valid": true}`,
			statusCode: http.StatusOK,
			expectErr:  false,
		},
		{
			name:       "empty JSON object",
			response:   `{}`,
			statusCode: http.StatusOK,
			expectErr:  false,
		},
		{
			name:       "JSON array",
			response:   `[1, 2, 3]`,
			statusCode: http.StatusOK,
			expectErr:  false,
		},
		{
			name:       "invalid JSON",
			response:   `{invalid}`,
			statusCode: http.StatusOK,
			expectErr:  true,
			errMsg:     "invalid JSON response",
		},
		{
			name:       "plain text",
			response:   `plain text`,
			statusCode: http.StatusOK,
			expectErr:  true,
			errMsg:     "invalid JSON response",
		},
		{
			name:       "error response with JSON",
			response:   `{"error": "something went wrong"}`,
			statusCode: http.StatusInternalServerError,
			expectErr:  true,
			errMsg:     "500",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				w.Write([]byte(tc.response))
			}))
			defer server.Close()

			client := setupTLSClient(t, cfg, server)

			resp, err := client.DoRequest("GET", "/api/test", nil)
			if tc.expectErr {
				require.Error(t, err)
				if tc.errMsg != "" {
					assert.Contains(t, err.Error(), tc.errMsg)
				}
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.response, string(resp))
			}
		})
	}
}
