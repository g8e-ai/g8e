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

package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/g8e-ai/g8e/internal/services/keystore"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func setupTestAuthController(t *testing.T) (*AuthController, *config.Config) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := t.TempDir()
	pkiDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	os.RemoveAll(secretsDir)
	os.MkdirAll(secretsDir, 0755)

	backend, err := keystore.NewTestBackend()
	require.NoError(t, err)
	ks, err := keystore.NewWithBackend(t.TempDir(), logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.EnsurePermissions())
	sm := &SecretManager{
		db:         db.db,
		secretsDir: t.TempDir(),
		logger:     logger,
		keystore:   ks,
	}

	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	err = pki.EnsurePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	resp := responder.New(logger)
	auth := NewAuthService(db, pki, logger, userSvc, personaSvc, resp, secretsDir, nil, "", "", "")
	sessionSvc := NewSessionService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc, sessionSvc, &cfg.Gateway)
	passkey, _ := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})
	mcpGateway := mcp.NewGatewayService(mcp.Dependencies{
		Logger:          logger,
		Responder:       resp,
		SuspendedStore:  db,
		MaxPayloadBytes: cfg.Gateway.MaxPayloadBytes,
	})

	authController := newAuthController(cfg, logger, db, auth, passkey, userSvc, reg, pki, sessionSvc, mcpGateway, resp)
	return authController, cfg
}

func TestHandleBootstrap(t *testing.T) {
	t.Run("Success - Bootstrap with CSR over loopback", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		csr := testutil.GenerateTestCSR(t, "test-operator")
		cliCsr := testutil.GenerateTestCSR(t, "test-cli")
		body := map[string]string{
			"name":               "Owner",
			"csr_pem":            csr,
			"cli_csr_pem":        cliCsr,
			"system_fingerprint": "test-fp",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handlePublicAuthBootstrap(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["operator_cert"])
		assert.NotEmpty(t, resp["operator_cert_chain"])
		assert.NotEmpty(t, resp["hub_trust_bundle"])
		assert.NotEmpty(t, resp["operator_session_id"])
		assert.NotEmpty(t, resp["cli_session_id"])
		assert.NotEqual(t, resp["operator_session_id"], resp["cli_session_id"],
			"cli_session_id MUST be a distinct identifier from operator_session_id")
	})

	t.Run("Failure - Non-loopback CSR request rejected", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		csr := testutil.GenerateTestCSR(t, "test-operator")
		cliCsr := testutil.GenerateTestCSR(t, "test-cli")
		body := map[string]string{
			"name":               "Owner",
			"csr_pem":            csr,
			"cli_csr_pem":        cliCsr,
			"system_fingerprint": "test-fp",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		req.RemoteAddr = "192.168.1.1:12345"
		rr := httptest.NewRecorder()

		c.handlePublicAuthBootstrap(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.JSONEq(t, `{"error":"CSR auto-issue only available over loopback"}`, rr.Body.String())
	})

	t.Run("Success - Rotation for existing bootstrap user", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateBootstrapUser()
		require.NoError(t, err)
		require.NotNil(t, user)

		csr := testutil.GenerateTestCSR(t, "test-operator")
		cliCsr := testutil.GenerateTestCSR(t, "test-cli")
		body := map[string]string{
			"name":               "Owner",
			"csr_pem":            csr,
			"cli_csr_pem":        cliCsr,
			"system_fingerprint": "rotated-fp",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handlePublicAuthBootstrap(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["operator_cert"])
	})

	t.Run("Failure - Rotation fails for disabled bootstrap user", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, _ := c.userSvc.CreateBootstrapUser()
		c.userSvc.Disable(user.ID, "retired", "actor", "op")

		csr := testutil.GenerateTestCSR(t, "test-operator")
		cliCsr := testutil.GenerateTestCSR(t, "test-cli")
		body := map[string]string{
			"name":               "Owner",
			"csr_pem":            csr,
			"cli_csr_pem":        cliCsr,
			"system_fingerprint": "fail-fp",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handlePublicAuthBootstrap(rr, req)

		assert.Equal(t, http.StatusConflict, rr.Code)
		assert.JSONEq(t, `{"error":"bootstrap user is disabled, cannot rotate"}`, rr.Body.String())
	})

	t.Run("Failure - Rejects bootstrap if ANY other users exist", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		c.userSvc.CreateUser()

		body := map[string]string{
			"name": "Superadmin",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handlePublicAuthBootstrap(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.JSONEq(t, `{"error":"bootstrap only available for initial setup"}`, rr.Body.String())
	})
}

func TestHandleBootstrapStatus(t *testing.T) {
	t.Run("Initially not bootstrapped", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap/status", nil)
		rr := httptest.NewRecorder()
		c.handleBootstrapStatus(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, false, resp["bootstrapped"])
	})

	t.Run("Bootstrapped after creating a user", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		_, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap/status", nil)
		rr := httptest.NewRecorder()
		c.handleBootstrapStatus(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, true, resp["bootstrapped"])
	})
}
