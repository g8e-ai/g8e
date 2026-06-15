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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
)

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
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
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
		assert.False(t, resp["bootstrapped"].(bool))
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
		assert.True(t, resp["bootstrapped"].(bool))
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
		assert.NotEmpty(t, resp["operator_session_id"])
		assert.NotEmpty(t, resp["operator_id"])
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

func TestHandleCLIPasskeyRegisterChallenge(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testMethodNotAllowed(t, c.handleCLIPasskeyRegisterChallenge, http.MethodGet, "/api/v1/auth/passkeys/cli-register/challenge")
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testInvalidJSON(t, c.handleCLIPasskeyRegisterChallenge, http.MethodPost, "/api/v1/auth/passkeys/cli-register/challenge")
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testMissingUserID(t, c.handleCLIPasskeyRegisterChallenge, http.MethodPost, "/api/v1/auth/passkeys/cli-register/challenge")
	})

	t.Run("Failure - user not found", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{
			"user_id":   "nonexistent-user",
			"user_name": "test-user",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli-register/challenge", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleCLIPasskeyRegisterChallenge(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), "user not found")
	})

	t.Run("Failure - first-credential only with existing credentials", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		// Add a fake credential to simulate existing credentials
		user.PasskeyCredentials = []models.PasskeyCredential{{ID: []byte("existing-cred")}}
		updatedUser, err := json.Marshal(user)
		require.NoError(t, err)
		c.db.DocStore.DocSet("users", user.ID, updatedUser)

		body := map[string]string{
			"user_id":   user.ID,
			"user_name": "test-user",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli-register/challenge", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleCLIPasskeyRegisterChallenge(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "first-credential registration only")
	})

	t.Run("Success - valid request with mTLS enrollment", func(t *testing.T) {
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
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli-register/challenge", bytes.NewReader(b))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		c.handleCLIPasskeyRegisterChallenge(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["options"])
	})
}

func TestHandleCLIPasskeyRegisterVerify(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testMethodNotAllowed(t, c.handleCLIPasskeyRegisterVerify, http.MethodGet, "/api/v1/auth/passkeys/cli-register/verify")
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testInvalidJSON(t, c.handleCLIPasskeyRegisterVerify, http.MethodPost, "/api/v1/auth/passkeys/cli-register/verify")
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testMissingUserID(t, c.handleCLIPasskeyRegisterVerify, http.MethodPost, "/api/v1/auth/passkeys/cli-register/verify")
	})

	t.Run("Failure - user not found", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{
			"user_id": "nonexistent-user",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli-register/verify", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleCLIPasskeyRegisterVerify(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), "user not found")
	})

	t.Run("Failure - first-credential only with existing credentials", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		// Add a fake credential to simulate existing credentials
		user.PasskeyCredentials = []models.PasskeyCredential{{ID: []byte("existing-cred")}}
		updatedUser, err := json.Marshal(user)
		require.NoError(t, err)
		c.db.DocStore.DocSet("users", user.ID, updatedUser)

		body := map[string]string{
			"user_id": user.ID,
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli-register/verify", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleCLIPasskeyRegisterVerify(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "first-credential registration only")
	})

	t.Run("Success - already enrolled allows multiple credentials", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		// Add a fake credential to simulate existing credentials
		user.PasskeyCredentials = []models.PasskeyCredential{{ID: []byte("existing-cred")}}
		updatedUser, err := json.Marshal(user)
		require.NoError(t, err)
		c.db.DocStore.DocSet("users", user.ID, updatedUser)

		body := map[string]string{
			"user_id": user.ID,
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli-register/verify", bytes.NewReader(b))
		// Simulate mTLS auth
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		c.handleCLIPasskeyRegisterVerify(rr, req)

		// It will fail later at VerifyRegistration because we didn't provide a real attestation,
		// but it should NOT fail at the "first-credential only" check.
		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.False(t, resp["success"].(bool))
		assert.NotContains(t, resp["error"].(string), "first-credential registration only")
	})
}

func TestHandleCLIBrowserPasskeyRegisterChallenge(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testMethodNotAllowed(t, c.handleCLIBrowserPasskeyRegisterChallenge, http.MethodGet, "/api/v1/auth/passkeys/cli-browser-register/challenge")
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testInvalidJSON(t, c.handleCLIBrowserPasskeyRegisterChallenge, http.MethodPost, "/api/v1/auth/passkeys/cli-browser-register/challenge")
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testMissingUserID(t, c.handleCLIBrowserPasskeyRegisterChallenge, http.MethodPost, "/api/v1/auth/passkeys/cli-browser-register/challenge")
	})

	t.Run("Failure - user not found", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{
			"user_id":        "nonexistent-user",
			"user_name":      "test-user",
			"cli_session_id": "session-123",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli-browser-register/challenge", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleCLIBrowserPasskeyRegisterChallenge(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), "user not found")
	})

	t.Run("Failure - first-credential only with existing credentials", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		// Add a fake credential to simulate existing credentials
		user.PasskeyCredentials = []models.PasskeyCredential{{ID: []byte("existing-cred")}}
		updatedUser, err := json.Marshal(user)
		require.NoError(t, err)
		c.db.DocStore.DocSet("users", user.ID, updatedUser)

		body := map[string]string{
			"user_id":        user.ID,
			"user_name":      "test-user",
			"cli_session_id": "session-123",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli-browser-register/challenge", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleCLIBrowserPasskeyRegisterChallenge(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "first-credential registration only")
	})

	t.Run("Success - valid request", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		body := map[string]string{
			"user_id":        user.ID,
			"user_name":      "test-user",
			"cli_session_id": "session-123",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli-browser-register/challenge", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleCLIBrowserPasskeyRegisterChallenge(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["options"])
	})
}

func TestHandleCLIBrowserPasskeyRegisterVerify(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testMethodNotAllowed(t, c.handleCLIBrowserPasskeyRegisterVerify, http.MethodGet, "/api/v1/auth/passkeys/cli-browser-register/verify")
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testInvalidJSON(t, c.handleCLIBrowserPasskeyRegisterVerify, http.MethodPost, "/api/v1/auth/passkeys/cli-browser-register/verify")
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testMissingUserID(t, c.handleCLIBrowserPasskeyRegisterVerify, http.MethodPost, "/api/v1/auth/passkeys/cli-browser-register/verify")
	})

	t.Run("Failure - user not found", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{
			"user_id":        "nonexistent-user",
			"cli_session_id": "session-123",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli-browser-register/verify", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleCLIBrowserPasskeyRegisterVerify(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), "user not found")
	})

	t.Run("Failure - first-credential only with existing credentials", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		// Add a fake credential to simulate existing credentials
		user.PasskeyCredentials = []models.PasskeyCredential{{ID: []byte("existing-cred")}}
		updatedUser, err := json.Marshal(user)
		require.NoError(t, err)
		c.db.DocStore.DocSet("users", user.ID, updatedUser)

		body := map[string]string{
			"user_id":        user.ID,
			"cli_session_id": "session-123",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli-browser-register/verify", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleCLIBrowserPasskeyRegisterVerify(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "first-credential registration only")
	})
}

func TestHandleCLIPasskeyAuthenticateChallenge(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/passkeys/cli/authenticate/challenge", nil)
		rr := httptest.NewRecorder()

		c.handleCLIPasskeyAuthenticateChallenge(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli/authenticate/challenge", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()

		c.handleCLIPasskeyAuthenticateChallenge(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "invalid JSON body")
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli/authenticate/challenge", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleCLIPasskeyAuthenticateChallenge(rr, req)

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
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli/authenticate/challenge", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleCLIPasskeyAuthenticateChallenge(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.False(t, resp["success"].(bool))
		assert.Contains(t, resp["error"].(string), "no passkeys registered")
	})

	t.Run("Success - valid request", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		// Add a fake credential to simulate existing credentials
		user.PasskeyCredentials = []models.PasskeyCredential{{ID: []byte("existing-cred")}}
		updatedUser, err := json.Marshal(user)
		require.NoError(t, err)
		c.db.DocStore.DocSet("users", user.ID, updatedUser)

		body := map[string]string{
			"user_id": user.ID,
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli/authenticate/challenge", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleCLIPasskeyAuthenticateChallenge(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["options"])
	})
}

func TestHandleCLIPasskeyAuthenticateVerify(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/passkeys/cli/authenticate/verify", nil)
		rr := httptest.NewRecorder()

		c.handleCLIPasskeyAuthenticateVerify(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli/authenticate/verify", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()

		c.handleCLIPasskeyAuthenticateVerify(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "invalid JSON body")
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli/authenticate/verify", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleCLIPasskeyAuthenticateVerify(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "user_id required")
	})

	t.Run("Failure - verify error", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		body := map[string]interface{}{
			"user_id": user.ID,
			"assertion_response": map[string]interface{}{
				"id": "invalid",
			},
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cli/authenticate/verify", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleCLIPasskeyAuthenticateVerify(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.False(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["error"])
	})
}

func TestHandleDeviceEnrollment(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/device/enroll", nil)
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "invalid JSON")
	})

	t.Run("Failure - missing csr_pem", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{
			"system_fingerprint": "fp-123",
			"hostname":           "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "csr_pem is required")
	})

	t.Run("Failure - missing system_fingerprint", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{
			"csr_pem":  testutil.GenerateTestCSRP256(t, "test-device"),
			"hostname": "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "system_fingerprint is required")
	})

	t.Run("Failure - missing hostname", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-device"),
			"system_fingerprint": "fp-123",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "hostname is required")
	})

	t.Run("Failure - bootstrap user disabled", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		bootstrapUser, err := c.userSvc.CreateBootstrapUser()
		require.NoError(t, err)
		c.userSvc.Disable(bootstrapUser.ID, "test", "actor", "op")

		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-device"),
			"system_fingerprint": "fp-123",
			"hostname":           "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusConflict, rr.Code)
		assert.Contains(t, rr.Body.String(), "bootstrap user is disabled")
	})

	t.Run("Failure - device enrollment on non-empty system", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		// Create a regular user to make system non-empty
		c.userSvc.CreateUser()

		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-device"),
			"system_fingerprint": "fp-123",
			"hostname":           "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "device enrollment only available for initial setup")
	})

	t.Run("Failure - missing cli_csr_pem", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)

		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-device"),
			"system_fingerprint": "fp-123",
			"hostname":           "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "cli_csr_pem is mandatory")
	})

	t.Run("Success - initial bootstrap", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)

		// Mock actuator key reader
		c.actuatorKeyReader = &mockActuatorKeyReader{
			keyID:     "act-123",
			publicKey: "pub-123",
		}

		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-device"),
			"cli_csr_pem":        testutil.GenerateTestCSRP256(t, "test-cli"),
			"system_fingerprint": "fp-123",
			"hostname":           "test-host",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp models.DeviceEnrollmentResponse
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.NotEmpty(t, resp.OperatorCert)
		assert.NotEmpty(t, resp.CLICert)
		assert.Equal(t, "act-123", resp.ActuatorKeyID)
		assert.Equal(t, "pub-123", resp.ActuatorPubKey)

		// Verify user was created
		user, err := c.userSvc.GetByID(resp.UserID)
		require.NoError(t, err)
		assert.NotNil(t, user)
		assert.True(t, user.IsBootstrap)
	})

	t.Run("Success - existing bootstrap user", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		bootstrapUser, err := c.userSvc.CreateBootstrapUser()
		require.NoError(t, err)

		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-device"),
			"cli_csr_pem":        testutil.GenerateTestCSRP256(t, "test-cli"),
			"system_fingerprint": "fp-124",
			"hostname":           "test-host-2",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp models.DeviceEnrollmentResponse
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, bootstrapUser.ID, resp.UserID)
	})

	t.Run("Success - actuator key reader error (graceful degradation)", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)

		// Mock actuator key reader that returns an error
		c.actuatorKeyReader = &mockActuatorKeyReader{
			err: errors.New("file not found"),
		}

		body := map[string]string{
			"csr_pem":            testutil.GenerateTestCSRP256(t, "test-device"),
			"cli_csr_pem":        testutil.GenerateTestCSRP256(t, "test-cli"),
			"system_fingerprint": "fp-125",
			"hostname":           "test-host-3",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/enroll", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		c.handleDeviceEnrollment(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp models.DeviceEnrollmentResponse
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Empty(t, resp.ActuatorKeyID)
		assert.Empty(t, resp.ActuatorPubKey)
	})
}
