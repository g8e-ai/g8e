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
)

func TestHandleUsers(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		c, _ := setupTestUserController(t)
		testMethodNotAllowed(t, c.handleUsers, http.MethodGet, "/api/v1/users")
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		c, _ := setupTestUserController(t)
		testInvalidJSON(t, c.handleUsers, http.MethodPost, "/api/v1/users")
	})

	t.Run("Success - creates user", func(t *testing.T) {
		c, _ := setupTestUserController(t)
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
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["user_id"])
	})
}

func TestHandleUserMe(t *testing.T) {
	t.Run("Failure - missing user_id in context", func(t *testing.T) {
		c, _ := setupTestUserController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		rr := httptest.NewRecorder()

		c.handleUserMe(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrNotAuthenticated.Error())
	})

	t.Run("Success - returns user data", func(t *testing.T) {
		c, _ := setupTestUserController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
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
		c, _ := setupTestUserController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "nonexistent-user"))
		rr := httptest.NewRecorder()

		c.handleUserMe(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), "user not found")
	})
}
