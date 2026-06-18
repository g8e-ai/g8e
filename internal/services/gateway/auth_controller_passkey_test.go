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

//go:build integration

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

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
		testMethodNotAllowed(t, c.handleAuthPasskeysRegisterChallenge, http.MethodGet, "/api/v1/auth/passkeys/register/challenge")
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testInvalidJSON(t, c.handleAuthPasskeysRegisterChallenge, http.MethodPost, "/api/v1/auth/passkeys/register/challenge")
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testMissingUserID(t, c.handleAuthPasskeysRegisterChallenge, http.MethodPost, "/api/v1/auth/passkeys/register/challenge")
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
		c.db.DocStore.DocSet("users", user.ID, updatedUser)

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
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
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
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
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
		testMethodNotAllowed(t, c.handleAuthPasskeysRegisterVerify, http.MethodGet, "/api/v1/auth/passkeys/register/verify")
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testInvalidJSON(t, c.handleAuthPasskeysRegisterVerify, http.MethodPost, "/api/v1/auth/passkeys/register/verify")
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testMissingUserID(t, c.handleAuthPasskeysRegisterVerify, http.MethodPost, "/api/v1/auth/passkeys/register/verify")
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
		c.db.DocStore.DocSet("users", user.ID, updatedUser)

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
		testMethodNotAllowed(t, c.handleAuthPasskeysAuthenticateChallenge, http.MethodGet, "/api/v1/auth/passkeys/authenticate/challenge")
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testInvalidJSON(t, c.handleAuthPasskeysAuthenticateChallenge, http.MethodPost, "/api/v1/auth/passkeys/authenticate/challenge")
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testMissingUserID(t, c.handleAuthPasskeysAuthenticateChallenge, http.MethodPost, "/api/v1/auth/passkeys/authenticate/challenge")
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
		assert.Contains(t, resp["error"].(string), "no passkeys registered")
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
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
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
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
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
		testMethodNotAllowed(t, c.handleAuthPasskeysAuthenticateVerify, http.MethodGet, "/api/v1/auth/passkeys/authenticate/verify")
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testInvalidJSON(t, c.handleAuthPasskeysAuthenticateVerify, http.MethodPost, "/api/v1/auth/passkeys/authenticate/verify")
	})

	t.Run("Failure - missing user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testMissingUserID(t, c.handleAuthPasskeysAuthenticateVerify, http.MethodPost, "/api/v1/auth/passkeys/authenticate/verify")
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
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
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
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
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
