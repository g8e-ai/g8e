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
	"github.com/g8e-ai/g8e/internal/models"
)

func TestHandleApprovalAction(t *testing.T) {
	t.Run("Failure - unauthorized", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/txhash123", nil)
		rr := httptest.NewRecorder()

		c.handleApprovalAction(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayUnauthorized.Error())
	})

	t.Run("Failure - transaction not found", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/nonexistent-tx", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		c.handleApprovalAction(rr, req)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayTransactionNotFound.Error())
	})
}

func TestHandleApprovalChallenge(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		testMethodNotAllowed(t, func(w http.ResponseWriter, r *http.Request) {
			c.handleApprovalChallenge(w, r, "txhash123", user.ID)
		}, http.MethodPost, "/api/v1/approvals/txhash123/challenge")
	})

	t.Run("Failure - transaction not found", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/nonexistent/challenge", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		c.handleApprovalChallenge(rr, req, "nonexistent", user.ID)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayTransactionNotFound.Error())
	})

	t.Run("Failure - transaction belongs to another user", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user1, err := c.userSvc.CreateUser()
		require.NoError(t, err)
		user2, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		// Create a suspended transaction for user1 via suspended store
		txHash := "txhash123"
		suspendedTx := &models.SuspendedTransaction{
			TransactionHash: txHash,
			UserID:          user1.ID,
			ToolName:        "test-tool",
			ToolArguments:   []byte("{}"),
			ExpiresAt:       time.Now().Add(5 * time.Minute),
		}
		c.suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/"+txHash+"/challenge", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user2.ID))
		rr := httptest.NewRecorder()

		c.handleApprovalChallenge(rr, req, txHash, user2.ID)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayTransactionBelongsToAnother.Error())
	})
}

func TestHandleApprovalVerify(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		testMethodNotAllowed(t, func(w http.ResponseWriter, r *http.Request) {
			c.handleApprovalVerify(w, r, "txhash123", user.ID)
		}, http.MethodGet, "/api/v1/approvals/txhash123/verify")
	})

	t.Run("Failure - transaction not found", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/nonexistent/verify", strings.NewReader("{}"))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		c.handleApprovalVerify(rr, req, "nonexistent", user.ID)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayTransactionNotFound.Error())
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
		c.suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/"+txHash+"/verify", strings.NewReader("{invalid}"))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
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
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		c.handleCLIApproval(rr, req, "nonexistent", user.ID)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayTransactionNotFound.Error())
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
		c.suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx)

		body := map[string]string{
			"mtls_cert_fingerprint": "fp123",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/"+txHash, bytes.NewReader(b))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		c.handleCLIApproval(rr, req, txHash, user.ID)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayCLISignatureRequired.Error())
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
		c.suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx)

		body := map[string]string{
			"cli_signature": "sig123",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/"+txHash, bytes.NewReader(b))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		c.handleCLIApproval(rr, req, txHash, user.ID)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayMTLSCertFingerprintRequired.Error())
	})

	t.Run("Failure - missing approval_public_key", func(t *testing.T) {
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
		c.suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx)

		body := map[string]string{
			"cli_signature":         "sig123",
			"mtls_cert_fingerprint": "fp123",
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/"+txHash, bytes.NewReader(b))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		c.handleCLIApproval(rr, req, txHash, user.ID)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "approval_public_key required")
	})
}

func TestHandleApprovalPage(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testMethodNotAllowed(t, c.handleApprovalPage, http.MethodPost, "/api/v1/approve/txhash123")
	})

	t.Run("Failure - missing transaction hash", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/approve/", nil)
		rr := httptest.NewRecorder()

		c.handleApprovalPage(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayTransactionHashRequired.Error())
	})

	t.Run("Success - redirects to console with txHash", func(t *testing.T) {
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
		c.suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approve/"+txHash, nil)
		rr := httptest.NewRecorder()

		c.handleApprovalPage(rr, req)

		assert.Equal(t, http.StatusFound, rr.Code)
		assert.Equal(t, "/console#approve="+txHash, rr.Header().Get("Location"))
	})
}

func TestHandleListSuspendedTransactions(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		testMethodNotAllowed(t, c.handleListSuspendedTransactions, http.MethodPost, "/api/v1/approvals")
	})

	t.Run("Failure - unauthorized", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals", nil)
		rr := httptest.NewRecorder()

		c.handleListSuspendedTransactions(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayUnauthorized.Error())
	})

	t.Run("Success - empty list", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
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
			assert.Empty(t, transactions)
		}
	})

	t.Run("Success - with query user_id", func(t *testing.T) {
		t.Parallel()
		c, _ := setupTestAuthController(t)
		user, err := c.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals?user_id="+user.ID, nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
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
			assert.Empty(t, transactions)
		}
	})
}
