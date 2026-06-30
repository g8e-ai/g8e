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

func TestPasskeyService_HandleApprovalAction(t *testing.T) {
	t.Run("Failure - unauthorized", func(t *testing.T) {
		t.Parallel()
		s, _, _ := setupTestPasskeyService(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/txhash123", nil)
		rr := httptest.NewRecorder()

		s.handleApprovalAction(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayUnauthorized.Error())
	})

	t.Run("Failure - unknown action returns 400", func(t *testing.T) {
		t.Parallel()
		s, userSvc, _ := setupTestPasskeyService(t)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/nonexistent-tx", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		s.handleApprovalAction(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "unknown action")
	})
}

func TestPasskeyService_HandleApprovalChallenge(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		s, userSvc, _ := setupTestPasskeyService(t)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		testMethodNotAllowed(t, func(w http.ResponseWriter, r *http.Request) {
			s.handleApprovalChallenge(w, r, "txhash123", user.ID)
		}, http.MethodPost, "/api/v1/approvals/txhash123/challenge")
	})

	t.Run("Failure - transaction not found", func(t *testing.T) {
		t.Parallel()
		s, userSvc, _ := setupTestPasskeyService(t)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/nonexistent/challenge", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		s.handleApprovalChallenge(rr, req, "nonexistent", user.ID)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayTransactionNotFound.Error())
	})

	t.Run("Failure - transaction belongs to another user", func(t *testing.T) {
		t.Parallel()
		s, userSvc, suspendedStore := setupTestPasskeyService(t)
		user1, err := userSvc.CreateUser()
		require.NoError(t, err)
		user2, err := userSvc.CreateUser()
		require.NoError(t, err)

		txHash := "txhash123"
		suspendedTx := &models.SuspendedTransaction{
			TransactionHash: txHash,
			UserID:          user1.ID,
			ToolName:        "test-tool",
			ToolArguments:   []byte("{}"),
			ExpiresAt:       time.Now().Add(5 * time.Minute),
		}
		suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/"+txHash+"/challenge", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user2.ID))
		rr := httptest.NewRecorder()

		s.handleApprovalChallenge(rr, req, txHash, user2.ID)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayTransactionBelongsToAnother.Error())
	})
}

func TestPasskeyService_HandleApprovalVerify(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		s, userSvc, _ := setupTestPasskeyService(t)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		testMethodNotAllowed(t, func(w http.ResponseWriter, r *http.Request) {
			s.handleApprovalVerify(w, r, "txhash123", user.ID)
		}, http.MethodGet, "/api/v1/approvals/txhash123/verify")
	})

	t.Run("Failure - transaction not found", func(t *testing.T) {
		t.Parallel()
		s, userSvc, _ := setupTestPasskeyService(t)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/nonexistent/verify", strings.NewReader("{}"))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		s.handleApprovalVerify(rr, req, "nonexistent", user.ID)

		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayTransactionNotFound.Error())
	})

	t.Run("Failure - invalid JSON", func(t *testing.T) {
		t.Parallel()
		s, userSvc, suspendedStore := setupTestPasskeyService(t)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		txHash := "txhash123"
		suspendedTx := &models.SuspendedTransaction{
			TransactionHash: txHash,
			UserID:          user.ID,
			ToolName:        "test-tool",
			ToolArguments:   []byte("{}"),
			ExpiresAt:       time.Now().Add(5 * time.Minute),
		}
		suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/"+txHash+"/verify", strings.NewReader("{invalid}"))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		s.handleApprovalVerify(rr, req, txHash, user.ID)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestPasskeyService_HandleCLIApprovalStatus(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		s, _, _ := setupTestPasskeyService(t)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/status/txhash123", nil)
		rr := httptest.NewRecorder()

		s.handleCLIApprovalStatus(rr, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Failure - missing transaction hash", func(t *testing.T) {
		t.Parallel()
		s, userSvc, _ := setupTestPasskeyService(t)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/status/", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		s.handleCLIApprovalStatus(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Failure - unauthorized", func(t *testing.T) {
		t.Parallel()
		s, _, _ := setupTestPasskeyService(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/status/txhash123", nil)
		rr := httptest.NewRecorder()

		s.handleCLIApprovalStatus(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Success - expired or not found", func(t *testing.T) {
		t.Parallel()
		s, userSvc, _ := setupTestPasskeyService(t)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/status/nonexistent", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		s.handleCLIApprovalStatus(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp models.ApprovalStatusResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.Equal(t, string(constants.SuspendedTxStatusExpiredOrNotFound), resp.Status)
	})

	t.Run("Success - pending", func(t *testing.T) {
		t.Parallel()
		s, userSvc, suspendedStore := setupTestPasskeyService(t)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		txHash := "txhash123"
		suspendedTx := &models.SuspendedTransaction{
			TransactionHash: txHash,
			UserID:          user.ID,
			ToolName:        "test-tool",
			ToolArguments:   []byte("{}"),
			ExpiresAt:       time.Now().Add(5 * time.Minute),
		}
		require.NoError(t, suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/status/"+txHash, nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		s.handleCLIApprovalStatus(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp models.ApprovalStatusResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.Equal(t, string(constants.SuspendedTxStatusPending), resp.Status)
		assert.Equal(t, txHash, resp.TxHash)
	})

	t.Run("Failure - transaction belongs to another user", func(t *testing.T) {
		t.Parallel()
		s, userSvc, suspendedStore := setupTestPasskeyService(t)
		user1, err := userSvc.CreateUser()
		require.NoError(t, err)
		user2, err := userSvc.CreateUser()
		require.NoError(t, err)

		txHash := "txhash123"
		suspendedTx := &models.SuspendedTransaction{
			TransactionHash: txHash,
			UserID:          user1.ID,
			ToolName:        "test-tool",
			ToolArguments:   []byte("{}"),
			ExpiresAt:       time.Now().Add(5 * time.Minute),
		}
		require.NoError(t, suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/status/"+txHash, nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user2.ID))
		rr := httptest.NewRecorder()

		s.handleCLIApprovalStatus(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
	})
}

func TestPasskeyService_HandleCLIListSuspended(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		s, _, _ := setupTestPasskeyService(t)
		testMethodNotAllowed(t, s.handleCLIListSuspended, http.MethodPost, "/api/v1/approvals/pending")
	})

	t.Run("Failure - unauthorized", func(t *testing.T) {
		t.Parallel()
		s, _, _ := setupTestPasskeyService(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/pending", nil)
		rr := httptest.NewRecorder()

		s.handleCLIListSuspended(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayUnauthorized.Error())
	})

	t.Run("Success - empty list", func(t *testing.T) {
		t.Parallel()
		s, userSvc, _ := setupTestPasskeyService(t)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/pending", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		s.handleCLIListSuspended(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp models.SuspendedTransactionsResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.Empty(t, resp.Transactions)
	})

	t.Run("Success - returns pending transaction", func(t *testing.T) {
		t.Parallel()
		s, userSvc, suspendedStore := setupTestPasskeyService(t)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		txHash := "txhash-pending"
		suspendedTx := &models.SuspendedTransaction{
			TransactionHash: txHash,
			UserID:          user.ID,
			ToolName:        "test-tool",
			ToolArguments:   []byte("{}"),
			ExpiresAt:       time.Now().Add(5 * time.Minute),
		}
		require.NoError(t, suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/pending", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		s.handleCLIListSuspended(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp models.SuspendedTransactionsResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		require.Len(t, resp.Transactions, 1)
		assert.Equal(t, txHash, resp.Transactions[0].TransactionHash)
	})

	t.Run("Success - filters out approved transactions", func(t *testing.T) {
		t.Parallel()
		s, userSvc, suspendedStore := setupTestPasskeyService(t)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		suspendedTx := &models.SuspendedTransaction{
			TransactionHash: "approved-tx",
			UserID:          user.ID,
			ToolName:        "test-tool",
			ToolArguments:   []byte("{}"),
			ExpiresAt:       time.Now().Add(5 * time.Minute),
			Approved:        true,
		}
		require.NoError(t, suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals/pending", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		s.handleCLIListSuspended(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp models.SuspendedTransactionsResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		assert.Empty(t, resp.Transactions)
	})
}

func TestPasskeyService_HandleApprovalPage(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		s, _, _ := setupTestPasskeyService(t)
		testMethodNotAllowed(t, s.handleApprovalPage, http.MethodPost, "/api/v1/approve/txhash123")
	})

	t.Run("Failure - missing transaction hash", func(t *testing.T) {
		t.Parallel()
		s, _, _ := setupTestPasskeyService(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/approve/", nil)
		rr := httptest.NewRecorder()

		s.handleApprovalPage(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayTransactionHashRequired.Error())
	})

	t.Run("Success - redirects to console with txHash", func(t *testing.T) {
		t.Parallel()
		s, userSvc, suspendedStore := setupTestPasskeyService(t)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		txHash := "txhash123"
		suspendedTx := &models.SuspendedTransaction{
			TransactionHash: txHash,
			UserID:          user.ID,
			ToolName:        "test-tool",
			ToolArguments:   []byte(`{"arg":"value"}`),
			ExpiresAt:       time.Now().Add(5 * time.Minute),
		}
		suspendedStore.StoreSuspendedTransaction(context.Background(), suspendedTx)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approve/"+txHash, nil)
		rr := httptest.NewRecorder()

		s.handleApprovalPage(rr, req)

		assert.Equal(t, http.StatusFound, rr.Code)
		assert.Equal(t, "/console/#approve="+txHash, rr.Header().Get("Location"))
	})
}

func TestPasskeyService_HandleListSuspendedTransactions(t *testing.T) {
	t.Run("Failure - method not allowed", func(t *testing.T) {
		t.Parallel()
		s, _, _ := setupTestPasskeyService(t)
		testMethodNotAllowed(t, s.handleListSuspendedTransactions, http.MethodPost, "/api/v1/approvals")
	})

	t.Run("Failure - unauthorized", func(t *testing.T) {
		t.Parallel()
		s, _, _ := setupTestPasskeyService(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals", nil)
		rr := httptest.NewRecorder()

		s.handleListSuspendedTransactions(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrGatewayUnauthorized.Error())
	})

	t.Run("Success - empty list", func(t *testing.T) {
		t.Parallel()
		s, userSvc, _ := setupTestPasskeyService(t)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		s.handleListSuspendedTransactions(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		transactions, ok := resp["transactions"].([]interface{})
		if !ok {
			assert.Nil(t, resp["transactions"])
		} else {
			assert.Empty(t, transactions)
		}
	})

	t.Run("Success - ignores query user_id (IDOR fix)", func(t *testing.T) {
		t.Parallel()
		s, userSvc, _ := setupTestPasskeyService(t)
		user, err := userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals?user_id=other-user", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, user.ID))
		rr := httptest.NewRecorder()

		s.handleListSuspendedTransactions(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		transactions, ok := resp["transactions"].([]interface{})
		if !ok {
			assert.Nil(t, resp["transactions"])
		} else {
			assert.Empty(t, transactions)
		}
	})
}
