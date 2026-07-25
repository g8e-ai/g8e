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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

func TestHandleEnrollmentTokenGenerate(t *testing.T) {
	t.Run("Success - returns 201 with token when context has user_id and cli_session_id", func(t *testing.T) {
		c, _ := setupTestEnrollmentTokenController(t)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/enrollment-token/generate", nil)
		ctx := context.WithValue(req.Context(), constants.ContextKeyUserID, "user-gen-1")
		ctx = context.WithValue(ctx, constants.ContextKeyCLISessionID, "cli-gen-1")
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		c.handleEnrollmentTokenGenerate(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp["token"])
	})

	t.Run("Failure - 401 when context has no user_id or cli_session_id", func(t *testing.T) {
		c, _ := setupTestEnrollmentTokenController(t)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/enrollment-token/generate", nil)
		rr := httptest.NewRecorder()

		c.handleEnrollmentTokenGenerate(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Failure - method not allowed", func(t *testing.T) {
		c, _ := setupTestEnrollmentTokenController(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/enrollment-token/generate", nil)
		rr := httptest.NewRecorder()

		c.handleEnrollmentTokenGenerate(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestHandleEnrollmentTokenValidate(t *testing.T) {
	t.Run("Success - returns 200 with user_id and cli_session_id for valid token", func(t *testing.T) {
		c, _ := setupTestEnrollmentTokenController(t)

		token, err := c.enrollmentTokenSvc.GenerateToken("user-val-1", "cli-val-1")
		require.NoError(t, err)

		body, _ := json.Marshal(map[string]string{"token": token.Token})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/enrollment-token/validate", bytes.NewReader(body))
		rr := httptest.NewRecorder()

		c.handleEnrollmentTokenValidate(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]string
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.Equal(t, "user-val-1", resp["user_id"])
		assert.Equal(t, "cli-val-1", resp["cli_session_id"])
	})

	t.Run("Failure - 410 Gone for expired token", func(t *testing.T) {
		c, _ := setupTestEnrollmentTokenController(t)

		token, err := c.enrollmentTokenSvc.GenerateToken("user-exp-1", "cli-exp-1")
		require.NoError(t, err)

		expiredToken := &models.EnrollmentToken{
			Token:        token.Token,
			UserID:       "user-exp-1",
			CLISessionID: "cli-exp-1",
			CreatedAt:    token.CreatedAt,
			ExpiresAt:    token.ExpiresAt.Add(-1 * time.Hour),
			Consumed:     false,
		}
		expiredData, _ := json.Marshal(expiredToken)
		c.enrollmentTokenSvc.db.DocSet(
			marshaler.CollectionName(constants.CollectionEnrollmentTokens),
			token.Token, expiredData,
		)

		body, _ := json.Marshal(map[string]string{"token": token.Token})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/enrollment-token/validate", bytes.NewReader(body))
		rr := httptest.NewRecorder()

		c.handleEnrollmentTokenValidate(rr, req)

		assert.Equal(t, http.StatusGone, rr.Code)
	})

	t.Run("Failure - 409 Conflict for consumed token", func(t *testing.T) {
		c, _ := setupTestEnrollmentTokenController(t)

		token, err := c.enrollmentTokenSvc.GenerateToken("user-con-1", "cli-con-1")
		require.NoError(t, err)

		_, err = c.enrollmentTokenSvc.ValidateAndConsumeToken(token.Token)
		require.NoError(t, err)

		body, _ := json.Marshal(map[string]string{"token": token.Token})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/enrollment-token/validate", bytes.NewReader(body))
		rr := httptest.NewRecorder()

		c.handleEnrollmentTokenValidate(rr, req)

		assert.Equal(t, http.StatusConflict, rr.Code)
	})

	t.Run("Failure - 401 for unknown token", func(t *testing.T) {
		c, _ := setupTestEnrollmentTokenController(t)

		body, _ := json.Marshal(map[string]string{"token": "nonexistenttoken1234567890abcdef"})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/enrollment-token/validate", bytes.NewReader(body))
		rr := httptest.NewRecorder()

		c.handleEnrollmentTokenValidate(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Failure - 400 for empty token field", func(t *testing.T) {
		c, _ := setupTestEnrollmentTokenController(t)

		body, _ := json.Marshal(map[string]string{"token": ""})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/enrollment-token/validate", bytes.NewReader(body))
		rr := httptest.NewRecorder()

		c.handleEnrollmentTokenValidate(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Failure - 400 for invalid JSON", func(t *testing.T) {
		c, _ := setupTestEnrollmentTokenController(t)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/enrollment-token/validate", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()

		c.handleEnrollmentTokenValidate(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Failure - method not allowed", func(t *testing.T) {
		c, _ := setupTestEnrollmentTokenController(t)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/enrollment-token/validate", nil)
		rr := httptest.NewRecorder()

		c.handleEnrollmentTokenValidate(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - oversized body rejected by readRequestBody", func(t *testing.T) {
		c, _ := setupTestEnrollmentTokenController(t)
		c.cfg.Gateway.MaxPayloadBytes = 100

		largeBody := strings.Repeat("a", 200)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/enrollment-token/validate", strings.NewReader(largeBody))
		rr := httptest.NewRecorder()

		c.handleEnrollmentTokenValidate(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestEnrollmentTokenRouteRegistration(t *testing.T) {
	t.Run("Generate endpoint is registered on public router behind mTLS", func(t *testing.T) {
		h, _, _ := setupTestHTTPHandler(t)
		router := h.buildPublicRouter()

		body, _ := json.Marshal(map[string]string{"token": "test"})
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthEnrollmentTokenGenerate, bytes.NewReader(body))
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		assert.NotEqual(t, http.StatusNotFound, rr.Code, "generate endpoint must be registered on public router")
	})

	t.Run("Validate endpoint is registered on public router", func(t *testing.T) {
		h, _, _ := setupTestHTTPHandler(t)
		router := h.buildPublicRouter()

		body, _ := json.Marshal(map[string]string{"token": "nonexistenttoken1234567890abcdef"})
		req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthEnrollmentTokenValidate, bytes.NewReader(body))
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		assert.NotEqual(t, http.StatusNotFound, rr.Code, "validate endpoint must be registered on public router")
	})
}
