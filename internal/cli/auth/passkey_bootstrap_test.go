//go:build integration

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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// NewPasskeyBootstrapServer
// ---------------------------------------------------------------------------

func TestNewPasskeyBootstrapServer(t *testing.T) {
	t.Parallel()

	gatewayURL := "https://test-gateway.example.com"
	bootstrapURL := "http://test-bootstrap.example.com"
	userID := "test-user-id"
	userName := "test-user"
	cliSessionID := "test-session-id"

	server := NewPasskeyBootstrapServer(gatewayURL, bootstrapURL, userID, userName, cliSessionID)

	require.NotNil(t, server)
	assert.Equal(t, gatewayURL, server.gatewayURL)
	assert.Equal(t, bootstrapURL, server.bootstrapURL)
	assert.Equal(t, userID, server.userID)
	assert.Equal(t, userName, server.userName)
	assert.Equal(t, cliSessionID, server.cliSessionID)
	assert.NotNil(t, server.done)
	assert.False(t, server.success)
	assert.Empty(t, server.errMessage)
}

// ---------------------------------------------------------------------------
// PasskeyBootstrapServer.Start
// ---------------------------------------------------------------------------

func TestPasskeyBootstrapServer_Start(t *testing.T) {
	t.Parallel()

	server := NewPasskeyBootstrapServer("https://test.com", "http://test.com", "user-id", "user", "session-id")

	url, err := server.Start()
	require.NoError(t, err)
	assert.NotEmpty(t, url)
	assert.Contains(t, url, "http://0.0.0.0:")
	assert.Contains(t, url, ":")

	server.Stop()
}

func TestPasskeyBootstrapServer_Start_ListensOnRandomPort(t *testing.T) {
	t.Parallel()

	server1 := NewPasskeyBootstrapServer("https://test.com", "http://test.com", "user-id", "user", "session-id")
	server2 := NewPasskeyBootstrapServer("https://test.com", "http://test.com", "user-id", "user", "session-id")

	url1, err := server1.Start()
	require.NoError(t, err)
	url2, err := server2.Start()
	require.NoError(t, err)

	// Two servers should get different ports
	assert.NotEqual(t, url1, url2)

	server1.Stop()
	server2.Stop()
}

// ---------------------------------------------------------------------------
// PasskeyBootstrapServer.Stop
// ---------------------------------------------------------------------------

func TestPasskeyBootstrapServer_Stop(t *testing.T) {
	t.Parallel()

	server := NewPasskeyBootstrapServer("https://test.com", "http://test.com", "user-id", "user", "session-id")

	url, err := server.Start()
	require.NoError(t, err)

	// Stop should not panic
	server.Stop()

	// Verify server is stopped by trying to make a request
	resp, err := http.Get(url)
	if resp != nil {
		resp.Body.Close()
	}
	require.Error(t, err)
	assert.Nil(t, resp)
}

// ---------------------------------------------------------------------------
// PasskeyBootstrapServer.Wait
// ---------------------------------------------------------------------------

func TestPasskeyBootstrapServer_Wait_Success(t *testing.T) {
	t.Parallel()

	server := NewPasskeyBootstrapServer("https://test.com", "http://test.com", "user-id", "user", "session-id")
	server.success = true
	close(server.done)

	success, timedOut := server.Wait(5 * time.Second)

	assert.True(t, success)
	assert.False(t, timedOut)
}

func TestPasskeyBootstrapServer_Wait_Timeout(t *testing.T) {
	t.Parallel()

	server := NewPasskeyBootstrapServer("https://test.com", "http://test.com", "user-id", "user", "session-id")

	success, timedOut := server.Wait(100 * time.Millisecond)

	assert.False(t, success)
	assert.True(t, timedOut)
}

func TestPasskeyBootstrapServer_Wait_Failure(t *testing.T) {
	t.Parallel()

	server := NewPasskeyBootstrapServer("https://test.com", "http://test.com", "user-id", "user", "session-id")
	server.success = false
	server.errMessage = "test error"
	close(server.done)

	success, timedOut := server.Wait(5 * time.Second)

	assert.False(t, success)
	assert.False(t, timedOut)
	assert.Equal(t, "test error", server.errMessage)
}

// ---------------------------------------------------------------------------
// PasskeyBootstrapServer.handleIndex
// ---------------------------------------------------------------------------

func TestPasskeyBootstrapServer_handleIndex(t *testing.T) {
	t.Parallel()

	server := NewPasskeyBootstrapServer("https://test.com", "http://test.com", "user-id", "test-user", "session-id")

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	server.handleIndex(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "Register Passkey for g8e CLI")
	assert.Contains(t, w.Body.String(), "user-id")
	assert.Contains(t, w.Body.String(), "test-user")
}

func TestPasskeyBootstrapServer_handleIndex_NonRootPath(t *testing.T) {
	t.Parallel()

	server := NewPasskeyBootstrapServer("https://test.com", "http://test.com", "user-id", "user", "session-id")

	req := httptest.NewRequest("GET", "/other", nil)
	w := httptest.NewRecorder()

	server.handleIndex(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ---------------------------------------------------------------------------
// PasskeyBootstrapServer.handleRegister
// ---------------------------------------------------------------------------

func TestPasskeyBootstrapServer_handleRegister_Success(t *testing.T) {
	t.Parallel()

	server := NewPasskeyBootstrapServer("https://test.com", "http://test.com", "user-id", "user", "session-id")

	req := httptest.NewRequest("POST", "/register?status=success", nil)
	w := httptest.NewRecorder()

	server.handleRegister(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK", w.Body.String())
	assert.True(t, server.success)
	assert.False(t, server.errMessage != "")
}

func TestPasskeyBootstrapServer_handleRegister_Error(t *testing.T) {
	t.Parallel()

	server := NewPasskeyBootstrapServer("https://test.com", "http://test.com", "user-id", "user", "session-id")

	req := httptest.NewRequest("POST", "/register?status=error&error=test%20error", nil)
	w := httptest.NewRecorder()

	server.handleRegister(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.False(t, server.success)
	assert.Equal(t, "test error", server.errMessage)
}

func TestPasskeyBootstrapServer_handleRegister_InvalidMethod(t *testing.T) {
	t.Parallel()

	server := NewPasskeyBootstrapServer("https://test.com", "http://test.com", "user-id", "user", "session-id")

	req := httptest.NewRequest("GET", "/register", nil)
	w := httptest.NewRecorder()

	server.handleRegister(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestPasskeyBootstrapServer_handleRegister_Options(t *testing.T) {
	t.Parallel()

	server := NewPasskeyBootstrapServer("https://test.com", "http://test.com", "user-id", "user", "session-id")

	req := httptest.NewRequest("OPTIONS", "/register", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()

	server.handleRegister(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "POST, OPTIONS", w.Header().Get("Access-Control-Allow-Methods"))
}

func TestPasskeyBootstrapServer_handleRegister_WithOrigin(t *testing.T) {
	t.Parallel()

	server := NewPasskeyBootstrapServer("https://test.com", "http://test.com", "user-id", "user", "session-id")

	req := httptest.NewRequest("POST", "/register?status=success", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()

	server.handleRegister(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}

// ---------------------------------------------------------------------------
// VerifyPasskeyRegistration
// ---------------------------------------------------------------------------

func TestVerifyPasskeyRegistration_NetworkError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	// VerifyPasskeyRegistration now uses mTLS: supply CLI cert and a CA bundle
	// so the test reaches the network dial (and fails there, as expected).
	writeTestCLICert(t, cfg)
	dummyCert, _ := generateTestCertificateWithSPIFFE(t, "dummy", time.Now().Add(24*time.Hour))
	caPath := filepath.Join(tmpDir, "test-ca.pem")
	require.NoError(t, os.WriteFile(caPath, []byte(dummyCert), 0600))
	cfg.Paths.Infra.CACertPath = caPath

	hasPasskey, err := VerifyPasskeyRegistration(cfg, "test-user")

	require.Error(t, err)
	assert.False(t, hasPasskey)
}

// ---------------------------------------------------------------------------
// RegisterPasskeyDirectly
// ---------------------------------------------------------------------------

func TestRegisterPasskeyDirectly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		ProjectRoot:    tmpDir,
		RuntimeDir:     paths.Infra.RuntimeDir,
		PKIDir:         paths.Infra.PkiDir,
		SecretsDir:     paths.Infra.SecretsDir,
		CredentialsDir: tmpDir,
		Paths:          &config.PathsConfig{},
	}

	err := RegisterPasskeyDirectly(cfg, "test-user")

	// The function should fail with either network error or browser interaction error
	// depending on whether it reaches the server or not
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// PasskeyAttestationResponse
// ---------------------------------------------------------------------------

func TestPasskeyAttestationResponse(t *testing.T) {
	t.Parallel()

	resp := PasskeyAttestationResponse{
		ID:                "test-id",
		RawID:             "AQID", // base64 of [1, 2, 3]
		ClientDataJSON:    "test-client-data",
		AttestationObject: "test-attestation",
		Transports:        []string{"internal", "hybrid"},
	}

	assert.Equal(t, "test-id", resp.ID)
	assert.Equal(t, "AQID", resp.RawID)
	assert.Equal(t, "test-client-data", resp.ClientDataJSON)
	assert.Equal(t, "test-attestation", resp.AttestationObject)
	assert.Equal(t, []string{"internal", "hybrid"}, resp.Transports)
}
