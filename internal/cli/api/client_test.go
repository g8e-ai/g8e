// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
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

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
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

func setupTestConfig(t *testing.T) (*config.Config, fs.RuntimeFileService, string) {
	t.Helper()

	tempDir := testutil.TempDir(t)

	absTempDir, err := filepath.Abs(tempDir)
	require.NoError(t, err)

	projectRoot := filepath.Join(absTempDir, "project")
	require.NoError(t, os.MkdirAll(projectRoot, constants.PermDirStandard))

	fileSvc, err := fs.NewRuntimeFileService(projectRoot, slog.Default())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

	caCertPEM := generateTestCA(t)

	cfg, err := config.Load(projectRoot)
	require.NoError(t, err)

	require.NoError(t, fileSvc.WriteFile(context.Background(), cfg.DefaultTrustBundleRelPath(), caCertPEM, constants.PermFilePublic))

	return cfg, fileSvc, tempDir
}

func setupTestCredentials(t *testing.T, fileSvc fs.RuntimeFileService, cfg *config.Config) {
	t.Helper()

	creds := &auth.Credentials{
		OperatorSessionID: "test-operator-session-id",
		UserID:            "test-user-id",
		OperatorID:        "test-operator-id",
		CLISessionID:      "test-cli-session-id",
	}

	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))

	certPEM, keyPEM, _ := generateTestCert(t)
	certRel, err := fileSvc.RelFromAbs(cfg.CLICertFile())
	require.NoError(t, err)
	keyRel, err := fileSvc.RelFromAbs(cfg.CLIKeyFile())
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), certRel, certPEM, constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyRel, keyPEM, constants.PermFilePrivate))
}

func setupTLSClient(t *testing.T, fileSvc fs.RuntimeFileService, cfg *config.Config, server *httptest.Server) *Client {
	t.Helper()

	client, err := NewClientWithURL(fileSvc, cfg, server.URL)
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
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
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
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	client, err := NewClient(fileSvc, cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	assert.NotNil(t, client.httpClient)
	assert.Equal(t, cfg, client.cfg)
	assert.NotNil(t, client.creds)
	assert.Equal(t, 5*time.Second, client.httpClient.Timeout)

	transport, ok := client.httpClient.Transport.(*http.Transport)
	require.True(t, ok)
	assert.NotNil(t, transport.TLSClientConfig)
	assert.Equal(t, uint16(tls.VersionTLS13), transport.TLSClientConfig.MinVersion)
	assert.Len(t, transport.TLSClientConfig.Certificates, 1)
	assert.NotNil(t, transport.TLSClientConfig.RootCAs)
}

func TestNewClient_NoCredentials(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)

	credsDir := cfg.RuntimeDir
	os.RemoveAll(credsDir)

	client, err := NewClient(fileSvc, cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	require.ErrorIs(t, err, constants.ErrNotAuthenticated)
}

func TestNewClient_LoadCredentialsError(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)

	credsDir := cfg.RuntimeDir
	require.NoError(t, os.MkdirAll(credsDir, constants.PermDirPrivate))

	credsFile := cfg.CredentialsFile()
	require.NoError(t, os.WriteFile(credsFile, []byte("invalid json"), constants.PermFilePrivate))

	client, err := NewClient(fileSvc, cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	require.ErrorIs(t, err, constants.ErrFailedToLoadCredentials)
}

func TestNewClient_MissingCertFile(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	require.NoError(t, os.Remove(cfg.CLICertFile()))

	client, err := NewClient(fileSvc, cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	require.ErrorIs(t, err, constants.ErrFailedToLoadClientCertificate)
}

func TestNewClient_MissingKeyFile(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	require.NoError(t, os.Remove(cfg.CLIKeyFile()))

	client, err := NewClient(fileSvc, cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	require.ErrorIs(t, err, constants.ErrFailedToLoadClientCertificate)
}

func TestNewClient_MissingTrustBundle(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	caRel := cfg.DefaultTrustBundleRelPath()
	require.NoError(t, fileSvc.Remove(context.Background(), caRel))

	client, err := NewClient(fileSvc, cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	require.ErrorIs(t, err, constants.ErrFailedToReadTrustBundle)
}

func TestNewClient_InvalidTrustBundle(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	caRel := cfg.DefaultTrustBundleRelPath()
	require.NoError(t, fileSvc.WriteFile(context.Background(), caRel, []byte("not a valid PEM"), constants.PermFilePublic))

	client, err := NewClient(fileSvc, cfg)
	require.Error(t, err)
	assert.Nil(t, client)
	require.ErrorIs(t, err, constants.ErrFailedToParseTrustBundle)
}

func TestDoRequest_Success(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

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

	client := setupTLSClient(t, fileSvc, cfg, server)

	body := map[string]string{"key": "value"}
	resp, err := client.DoRequest("POST", "/api/test", body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"result":"success"}`, string(resp))
}

func TestDoRequest_GetWithoutBody(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Empty(t, r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":"test"}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, fileSvc, cfg, server)

	resp, err := client.DoRequest("GET", "/api/test", nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"data":"test"}`, string(resp))
}

func TestDoRequest_MarshalError(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	client, err := NewClient(fileSvc, cfg)
	require.NoError(t, err)

	body := make(chan int)
	_, err = client.DoRequest("POST", "https://localhost:9999/api/test", body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal request body")
}

func TestDoRequest_HTTPError(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	client, err := NewClient(fileSvc, cfg)
	require.NoError(t, err)

	_, err = client.DoRequest("GET", "/invalid-endpoint", nil)
	require.Error(t, err)
	assert.Error(t, err)
}

func TestDoRequest_APIError(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, fileSvc, cfg, server)

	_, err := client.DoRequest("GET", "/api/test", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
	assert.Contains(t, err.Error(), "not found")
}

func TestDoRequest_InvalidJSONResponse(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not valid json`))
	}))
	defer server.Close()

	client := setupTLSClient(t, fileSvc, cfg, server)

	_, err := client.DoRequest("GET", "/api/test", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON response")
}

func TestDoRequest_ReadResponseError(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

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

	client := setupTLSClient(t, fileSvc, cfg, server)

	_, err := client.DoRequest("GET", "/api/test", nil)
	require.Error(t, err)
	assert.Error(t, err)
}

func TestGet_Success(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"get":"success"}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, fileSvc, cfg, server)

	resp, err := client.Get("/api/test")
	require.NoError(t, err)
	assert.JSONEq(t, `{"get":"success"}`, string(resp))
}

func TestPost_Success(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"post":"success"}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, fileSvc, cfg, server)

	body := map[string]string{"data": "test"}
	resp, err := client.Post("/api/test", body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"post":"success"}`, string(resp))
}

func TestPut_Success(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "PUT", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"put":"success"}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, fileSvc, cfg, server)

	body := map[string]string{"data": "updated"}
	resp, err := client.Put("/api/test", body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"put":"success"}`, string(resp))
}

func TestDelete_Success(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "DELETE", r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"delete":"success"}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, fileSvc, cfg, server)

	resp, err := client.Delete("/api/test")
	require.NoError(t, err)
	assert.JSONEq(t, `{"delete":"success"}`, string(resp))
}

func TestDoRequest_HeadersSetCorrectly(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	var receivedHeaders http.Header
	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, fileSvc, cfg, server)

	_, err := client.DoRequest("POST", "/api/test", map[string]string{"test": "data"})
	require.NoError(t, err)

	assert.Equal(t, "test-operator-session-id", receivedHeaders.Get(constants.HeaderOperatorSessionID))
	assert.Equal(t, "test-cli-session-id", receivedHeaders.Get(constants.HeaderCLISessionID))
	assert.Equal(t, "test-user-id", receivedHeaders.Get(constants.HeaderUserID))
	assert.Equal(t, "test-operator-id", receivedHeaders.Get(constants.HeaderOperatorID))
	assert.Equal(t, "application/json", receivedHeaders.Get("Content-Type"))
}

func TestDoRequest_URLConstruction(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	var receivedURL string
	server := newLocalhostTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := setupTLSClient(t, fileSvc, cfg, server)

	_, err := client.DoRequest("GET", "/api/v1/resource", nil)
	require.NoError(t, err)

	assert.Equal(t, "/api/v1/resource", receivedURL)
}

func TestNewClient_TLSConfig(t *testing.T) {
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

	client, err := NewClient(fileSvc, cfg)
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
	cfg, fileSvc, _ := setupTestConfig(t)
	setupTestCredentials(t, fileSvc, cfg)

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

			client := setupTLSClient(t, fileSvc, cfg, server)

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
