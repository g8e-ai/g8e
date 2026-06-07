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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
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
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false)
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
	err = pki.InitializePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	resp := response.NewWriter(logger)
	auth := NewAuthService(db, pki, logger, userSvc, personaSvc, resp, secretsDir, nil, "", "", "")
	cliSessionSvc := NewCLISessionService(db, logger)
	operatorSessionSvc := NewOperatorSessionService(db, logger)
	webSessionSvc := NewWebSessionService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, &cfg.Gateway)
	passkey, _ := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})
	mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
		Logger:          logger,
		Responder:       resp,
		SuspendedStore:  db,
		MaxPayloadBytes: cfg.Gateway.MaxPayloadBytes,
	})
	if err != nil {
		t.Fatalf("failed to create MCP gateway: %v", err)
	}

	authController := newAuthController(cfg, logger, db, auth, passkey, userSvc, reg, pki, webSessionSvc, mcpGateway, resp)
	return authController, cfg
}

func TestHandleBootstrap(t *testing.T) {
	t.Run("Success - Bootstrap with CSR over loopback", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		cliCsr := testutil.GenerateTestCSRP256(t, "test-cli")
		body := map[string]string{
			"name":               "Owner",
			"csr_pem":            csr,
			"cli_csr_pem":        cliCsr,
			"system_fingerprint": "test-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handleLocalBootstrap(rr, req)

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
		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		cliCsr := testutil.GenerateTestCSRP256(t, "test-cli")
		body := map[string]string{
			"name":               "Owner",
			"csr_pem":            csr,
			"cli_csr_pem":        cliCsr,
			"system_fingerprint": "test-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		req.RemoteAddr = "192.168.1.1:12345"
		rr := httptest.NewRecorder()

		c.handleLocalBootstrap(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.JSONEq(t, `{"error":"CSR auto-issue only available over loopback"}`, rr.Body.String())
	})

	t.Run("Success - Rotation for existing bootstrap user", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateBootstrapUser()
		require.NoError(t, err)
		require.NotNil(t, user)

		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		cliCsr := testutil.GenerateTestCSRP256(t, "test-cli")
		body := map[string]string{
			"name":               "Owner",
			"csr_pem":            csr,
			"cli_csr_pem":        cliCsr,
			"system_fingerprint": "rotated-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handleLocalBootstrap(rr, req)

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

		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		cliCsr := testutil.GenerateTestCSRP256(t, "test-cli")
		body := map[string]string{
			"name":               "Owner",
			"csr_pem":            csr,
			"cli_csr_pem":        cliCsr,
			"system_fingerprint": "fail-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handleLocalBootstrap(rr, req)

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
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleLocalBootstrap(rr, req)

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

func TestHandleAuthPasskeysRegisterChallenge(t *testing.T) {
	t.Run("Success - valid request", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		body := map[string]string{
			"user_id":   user.ID,
			"user_name": "test-user",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/register/challenge", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRegisterChallenge(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["options"])
	})

	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/passkeys/register/challenge", nil)
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRegisterChallenge(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/register/challenge", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRegisterChallenge(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{
			"user_name": "test-user",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/register/challenge", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRegisterChallenge(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "user_id required")
	})

	t.Run("Success - JIT route with first credential", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		body := map[string]string{
			"user_id":   user.ID,
			"user_name": "test-user",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/jit-register/challenge", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRegisterChallenge(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Failure - JIT route with existing credentials", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		// Add a fake credential to simulate existing credentials
		user.PasskeyCredentials = []models.PasskeyCredential{{ID: []byte("existing-cred")}}
		updatedUser, err := json.Marshal(user)
		require.NoError(t, err)
		c.db.DocSet("users", user.ID, updatedUser)

		body := map[string]string{
			"user_id":   user.ID,
			"user_name": "test-user",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/jit-register/challenge", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRegisterChallenge(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "first-credential registration only")
	})

	t.Run("Success - session context user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		body := map[string]string{
			"user_id":   "",
			"user_name": "test-user",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/register/challenge", bytes.NewReader(b))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRegisterChallenge(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Failure - session user_id mismatch", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		body := map[string]string{
			"user_id":   "other-user-id",
			"user_name": "test-user",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/register/challenge", bytes.NewReader(b))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRegisterChallenge(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "user_id mismatch")
	})
}

func TestHandleAuthPasskeysRegisterVerify(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/passkeys/register/verify", nil)
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRegisterVerify(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/register/verify", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRegisterVerify(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/register/verify", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRegisterVerify(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "user_id required")
	})

	t.Run("Failure - JIT route with existing credentials", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		// Add a fake credential to simulate existing credentials
		user.PasskeyCredentials = []models.PasskeyCredential{{ID: []byte("existing-cred")}}
		updatedUser, err := json.Marshal(user)
		require.NoError(t, err)
		c.db.DocSet("users", user.ID, updatedUser)

		body := map[string]string{
			"user_id": user.ID,
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/jit-register/verify", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRegisterVerify(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "first-credential registration only")
	})
}

func TestHandleAuthPasskeysAuthenticateChallenge(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/passkeys/authenticate/challenge", nil)
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysAuthenticateChallenge(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/authenticate/challenge", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysAuthenticateChallenge(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/authenticate/challenge", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysAuthenticateChallenge(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "user_id required")
	})

	t.Run("Failure - no passkeys registered", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		body := map[string]string{
			"user_id": user.ID,
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/authenticate/challenge", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysAuthenticateChallenge(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp["success"].(bool))
		assert.Contains(t, resp["error"].(string), "Found no credentials")
	})

	t.Run("Success - session context user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		body := map[string]string{
			"user_id": "",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/authenticate/challenge", bytes.NewReader(b))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysAuthenticateChallenge(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Failure - session user_id mismatch", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		body := map[string]string{
			"user_id": "other-user-id",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/authenticate/challenge", bytes.NewReader(b))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysAuthenticateChallenge(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "user_id mismatch")
	})
}

func TestHandleAuthPasskeysAuthenticateVerify(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/passkeys/authenticate/verify", nil)
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysAuthenticateVerify(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/authenticate/verify", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysAuthenticateVerify(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/authenticate/verify", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysAuthenticateVerify(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "user_id required")
	})

	t.Run("Success - session context user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		body := map[string]string{
			"user_id": "",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/authenticate/verify", bytes.NewReader(b))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysAuthenticateVerify(rr, req)

		// Will fail verification since no real assertion response, but should get past validation
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Failure - session user_id mismatch", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		body := map[string]string{
			"user_id": "other-user-id",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/authenticate/verify", bytes.NewReader(b))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysAuthenticateVerify(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "user_id mismatch")
	})
}

func TestHandleAuthPasskeys(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys", nil)
		rr := httptest.NewRecorder()

		c.handleAuthPasskeys(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/passkeys", nil)
		rr := httptest.NewRecorder()

		c.handleAuthPasskeys(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "user_id required")
	})

	t.Run("Success - list credentials", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/passkeys?user_id="+user.ID, nil)
		rr := httptest.NewRecorder()

		c.handleAuthPasskeys(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		// Credentials may be nil or empty slice when no credentials exist
		creds, ok := resp["credentials"]
		assert.True(t, ok)
		if creds != nil {
			assert.IsType(t, []interface{}{}, creds)
		}
	})
}

func TestHandleAuthPasskeysRevoke(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/passkeys/cred-id", nil)
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRevoke(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/passkeys/cred-id", nil)
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRevoke(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "user_id required")
	})

	t.Run("Failure - missing credential_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/passkeys/?user_id="+user.ID, nil)
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRevoke(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "credential_id required")
	})

	t.Run("Success - revoke credential", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/passkeys/test-cred-id?user_id="+user.ID, nil)
		rr := httptest.NewRecorder()

		c.handleAuthPasskeysRevoke(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.False(t, resp["found"].(bool)) // No credential exists
	})
}

func TestHandleApprovalAction(t *testing.T) {
	t.Run("Failure - unauthorized", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/txhash123", nil)
		rr := httptest.NewRecorder()

		c.handleApprovalAction(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "unauthorized")
	})

	t.Run("Failure - transaction not found", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/nonexistent-tx", nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleApprovalAction(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), "transaction not found")
	})
}

func TestHandleApprovalChallenge(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/txhash123/challenge", nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleApprovalChallenge(rr, req, "txhash123", user.ID)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - transaction not found", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/nonexistent/challenge", nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleApprovalChallenge(rr, req, "nonexistent", user.ID)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), "transaction not found")
	})

	t.Run("Failure - transaction belongs to another user", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user1, err := c.userSvc.CreateUser()
		require.NoError(t, err)
		user2, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		// Create a suspended transaction for user1 via DB
		txHash := "txhash123"
		suspendedTx := &models.SuspendedTransaction{
			TransactionHash: txHash,
			UserID:          user1.ID,
			ToolName:        "test-tool",
			ToolArguments:   []byte("{}"),
			ExpiresAt:       time.Now().Add(5 * time.Minute),
		}
		c.db.StoreSuspendedTransaction(suspendedTx)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/"+txHash+"/challenge", nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user2.ID))
		rr := httptest.NewRecorder()

		c.handleApprovalChallenge(rr, req, txHash, user2.ID)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "transaction belongs to another user")
	})
}

func TestHandleApprovalVerify(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/txhash123/verify", nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleApprovalVerify(rr, req, "txhash123", user.ID)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - transaction not found", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/nonexistent/verify", strings.NewReader("{}"))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleApprovalVerify(rr, req, "nonexistent", user.ID)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), "transaction not found")
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		txHash := "txhash123"
		suspendedTx := &models.SuspendedTransaction{
			TransactionHash: txHash,
			UserID:          user.ID,
			ToolName:        "test-tool",
			ToolArguments:   []byte("{}"),
			ExpiresAt:       time.Now().Add(5 * time.Minute),
		}
		c.db.StoreSuspendedTransaction(suspendedTx)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/"+txHash+"/verify", strings.NewReader("{invalid}"))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleApprovalVerify(rr, req, txHash, user.ID)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestHandleCLIApproval(t *testing.T) {
	t.Run("Failure - transaction not found", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		body := map[string]string{
			"cli_signature":         "sig123",
			"mtls_cert_fingerprint": "fp123",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/nonexistent", bytes.NewReader(b))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleCLIApproval(rr, req, "nonexistent", user.ID)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), "transaction not found")
	})

	t.Run("Failure - missing cli_signature", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		txHash := "txhash123"
		suspendedTx := &models.SuspendedTransaction{
			TransactionHash: txHash,
			UserID:          user.ID,
			ToolName:        "test-tool",
			ToolArguments:   []byte("{}"),
			ExpiresAt:       time.Now().Add(5 * time.Minute),
		}
		c.db.StoreSuspendedTransaction(suspendedTx)

		body := map[string]string{
			"mtls_cert_fingerprint": "fp123",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/"+txHash, bytes.NewReader(b))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleCLIApproval(rr, req, txHash, user.ID)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "cli_signature required")
	})

	t.Run("Failure - missing mtls_cert_fingerprint", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		txHash := "txhash123"
		suspendedTx := &models.SuspendedTransaction{
			TransactionHash: txHash,
			UserID:          user.ID,
			ToolName:        "test-tool",
			ToolArguments:   []byte("{}"),
			ExpiresAt:       time.Now().Add(5 * time.Minute),
		}
		c.db.StoreSuspendedTransaction(suspendedTx)

		body := map[string]string{
			"cli_signature": "sig123",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/"+txHash, bytes.NewReader(b))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleCLIApproval(rr, req, txHash, user.ID)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "mtls_cert_fingerprint required")
	})
}

func TestHandleCLIEnrollment(t *testing.T) {
	t.Run("Success - CLI enrollment over loopback after bootstrap", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		bootstrapUser, err := c.userSvc.CreateBootstrapUser()
		require.NoError(t, err)
		require.NotNil(t, bootstrapUser)

		cliCSR := testutil.GenerateTestCSRP256(t, "test-cli")
		body := map[string]string{
			"cli_csr_pem":        cliCSR,
			"system_fingerprint": "test-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli/enroll", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handleCLIEnrollment(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["cli_cert"])
		assert.NotEmpty(t, resp["cli_cert_chain"])
		assert.NotEmpty(t, resp["cli_session_id"])
		assert.NotEmpty(t, resp["user_id"])
		assert.NotEmpty(t, resp["hub_trust_bundle"])
		// Verify operator_session_id is NOT returned (CLI-only enrollment)
		_, hasOperatorSessionID := resp["operator_session_id"]
		assert.False(t, hasOperatorSessionID, "operator_session_id should not be returned for CLI-only enrollment")
	})

	t.Run("Failure - Non-loopback request rejected", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		bootstrapUser, err := c.userSvc.CreateBootstrapUser()
		require.NoError(t, err)
		require.NotNil(t, bootstrapUser)

		cliCSR := testutil.GenerateTestCSRP256(t, "test-cli")
		body := map[string]string{
			"cli_csr_pem":        cliCSR,
			"system_fingerprint": "test-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli/enroll", bytes.NewReader(b))
		req.RemoteAddr = "192.168.1.1:12345"
		rr := httptest.NewRecorder()

		c.handleCLIEnrollment(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "CLI enrollment only available over loopback")
	})

	t.Run("Failure - Rejected when not bootstrapped", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)

		cliCSR := testutil.GenerateTestCSRP256(t, "test-cli")
		body := map[string]string{
			"cli_csr_pem":        cliCSR,
			"system_fingerprint": "test-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli/enroll", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handleCLIEnrollment(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "CLI enrollment only available after bootstrap")
	})

	t.Run("Failure - Rejected when bootstrap user disabled", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		bootstrapUser, err := c.userSvc.CreateBootstrapUser()
		require.NoError(t, err)
		c.userSvc.Disable(bootstrapUser.ID, "retired", "actor", "op")

		cliCSR := testutil.GenerateTestCSRP256(t, "test-cli")
		body := map[string]string{
			"cli_csr_pem":        cliCSR,
			"system_fingerprint": "test-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli/enroll", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handleCLIEnrollment(rr, req)

		assert.Equal(t, http.StatusConflict, rr.Code)
		assert.Contains(t, rr.Body.String(), "bootstrap user is disabled")
	})

	t.Run("Failure - Missing cli_csr_pem", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		bootstrapUser, err := c.userSvc.CreateBootstrapUser()
		require.NoError(t, err)
		require.NotNil(t, bootstrapUser)

		body := map[string]string{
			"system_fingerprint": "test-fp",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli/enroll", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handleCLIEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "cli_csr_pem is required")
	})

	t.Run("Failure - Method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		bootstrapUser, err := c.userSvc.CreateBootstrapUser()
		require.NoError(t, err)
		require.NotNil(t, bootstrapUser)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/cli/enroll", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		c.handleCLIEnrollment(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestHandleApprovalPage(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/approve/txhash123", nil)
		rr := httptest.NewRecorder()

		c.handleApprovalPage(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - missing transaction hash", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/approve/", nil)
		rr := httptest.NewRecorder()

		c.handleApprovalPage(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "transaction hash required")
	})

	t.Run("Failure - transaction not found", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/approve/nonexistent", nil)
		rr := httptest.NewRecorder()

		c.handleApprovalPage(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), "transaction not found")
	})

	t.Run("Success - returns HTML page", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		txHash := "txhash123"
		suspendedTx := &models.SuspendedTransaction{
			TransactionHash: txHash,
			UserID:          user.ID,
			ToolName:        "test-tool",
			ToolArguments:   []byte(`{"arg":"value"}`),
			ExpiresAt:       time.Now().Add(5 * time.Minute),
		}
		c.db.StoreSuspendedTransaction(suspendedTx)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approve/"+txHash, nil)
		rr := httptest.NewRecorder()

		c.handleApprovalPage(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))
		assert.Contains(t, rr.Body.String(), "Approve Transaction")
		assert.Contains(t, rr.Body.String(), txHash)
		assert.Contains(t, rr.Body.String(), "test-tool")
	})
}

func TestHandleListSuspendedTransactions(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals", nil)
		rr := httptest.NewRecorder()

		c.handleListSuspendedTransactions(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - unauthorized", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals", nil)
		rr := httptest.NewRecorder()

		c.handleListSuspendedTransactions(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "unauthorized")
	})

	t.Run("Success - empty list", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals", nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleListSuspendedTransactions(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		// When empty, transactions may be null or empty array
		transactions, ok := resp["transactions"].([]interface{})
		if !ok {
			// If null, that's acceptable for empty list
			assert.Nil(t, resp["transactions"])
		} else {
			assert.Len(t, transactions, 0)
		}
	})

	t.Run("Success - with query user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals?user_id="+user.ID, nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleListSuspendedTransactions(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		// When empty, transactions may be null or empty array
		transactions, ok := resp["transactions"].([]interface{})
		if !ok {
			// If null, that's acceptable for empty list
			assert.Nil(t, resp["transactions"])
		} else {
			assert.Len(t, transactions, 0)
		}
	})
}

func TestHandleUserMe(t *testing.T) {
	t.Run("Failure - missing user_id in context", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		rr := httptest.NewRecorder()

		c.handleUserMe(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "unauthorized")
	})

	t.Run("Success - returns user data", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleUserMe(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotNil(t, resp["user"])
	})

	t.Run("Failure - user not found", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, "nonexistent-user"))
		rr := httptest.NewRecorder()

		c.handleUserMe(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), "user not found")
	})
}

func TestHandleWebSession(t *testing.T) {
	t.Run("Failure - missing user_id in context", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/websession", nil)
		rr := httptest.NewRecorder()

		c.handleWebSession(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "unauthorized")
	})

	t.Run("Success - returns session data with cookie", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/websession", nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		req.AddCookie(&http.Cookie{Name: "g8e_session", Value: "test-session-id"})
		rr := httptest.NewRecorder()

		c.handleWebSession(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.Equal(t, user.ID, resp["user_id"])
		assert.Equal(t, "test-session-id", resp["web_session_id"])
	})

	t.Run("Success - returns session data without cookie", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/websession", nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, user.ID))
		rr := httptest.NewRecorder()

		c.handleWebSession(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.Equal(t, user.ID, resp["user_id"])
		assert.Equal(t, "", resp["web_session_id"])
	})
}

func TestHandleUsers(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
		rr := httptest.NewRecorder()

		c.handleUsers(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()

		c.handleUsers(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "invalid JSON")
	})

	t.Run("Success - creates user", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{
			"name": "Test User",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleUsers(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["user_id"])
	})
}

func TestHandlePublicAuthLoginVerify(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login/verify", nil)
		rr := httptest.NewRecorder()

		c.handlePublicAuthLoginVerify(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/verify", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()

		c.handlePublicAuthLoginVerify(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "invalid JSON")
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login/verify", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handlePublicAuthLoginVerify(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "user not found")
	})
}

func TestHandlePublicAuthLogout(t *testing.T) {
	t.Run("Success - clears cookie", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		req.AddCookie(&http.Cookie{Name: "g8e_session", Value: "test-session"})
		rr := httptest.NewRecorder()

		c.handlePublicAuthLogout(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		cookies := rr.Result().Cookies()
		assert.Len(t, cookies, 1)
		assert.Equal(t, "g8e_session", cookies[0].Name)
		assert.Equal(t, -1, cookies[0].MaxAge)
	})

	t.Run("Success - no cookie present", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
		rr := httptest.NewRecorder()

		c.handlePublicAuthLogout(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})
}
