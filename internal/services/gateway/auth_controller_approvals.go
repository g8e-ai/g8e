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
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

func (c *AuthController) handleApprovalAction(w http.ResponseWriter, r *http.Request) {
	// Path format: /api/v1/approvals/{txHash} or /api/v1/approvals/{txHash}/{action}
	path := strings.TrimPrefix(r.URL.Path, constants.APIPaths.ApprovalsPrefix)
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		c.responder.Error(w, http.StatusBadRequest, "invalid request path")
		return
	}

	txHash := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	switch action {
	case "challenge":
		c.handleApprovalChallenge(w, r, txHash, userID)
	case "verify":
		c.handleApprovalVerify(w, r, txHash, userID)
	default:
		c.responder.Error(w, http.StatusBadRequest, "unknown action")
	}
}

func (c *AuthController) handleApprovalChallenge(w http.ResponseWriter, r *http.Request, txHash, userID string) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Retrieve suspended transaction to ensure it exists and belongs to the user
	// (or the user is authorized to approve it)
	suspendedTx, ok, err := c.mcp.GetSuspendedTransaction(r.Context(), txHash)
	if err != nil || !ok {
		c.responder.Error(w, http.StatusNotFound, "transaction not found or expired")
		return
	}

	// For now, we allow any logged-in user to see the challenge, but we might
	// want to restrict this to the UserID stashed in the suspended transaction
	// if it was initiated by a specific user.
	if suspendedTx.UserID != "" && suspendedTx.UserID != userID {
		c.responder.Error(w, http.StatusForbidden, "transaction belongs to another user")
		return
	}

	options, err := c.passkey.GenerateApprovalChallenge(userID, txHash)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, options)
}

func (c *AuthController) handleApprovalVerify(w http.ResponseWriter, r *http.Request, txHash, userID string) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Retrieve suspended transaction to ensure it exists and belongs to the user
	suspendedTx, ok, err := c.mcp.GetSuspendedTransaction(r.Context(), txHash)
	if err != nil || !ok {
		c.responder.Error(w, http.StatusNotFound, "transaction not found or expired")
		return
	}

	// Verify the logged-in user is authorized to approve this transaction
	if suspendedTx.UserID != "" && suspendedTx.UserID != userID {
		c.responder.Error(w, http.StatusForbidden, "transaction belongs to another user")
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req models.WebAuthnAssertionResponse
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Map AssertionResponse to commonv1.L3Proof
	proof := &commonv1.L3Proof{
		ClientDataJson:    req.ClientDataJSON,
		AuthenticatorData: req.AuthenticatorData,
		Signature:         req.Signature,
		CredentialId:      req.ID,
	}

	// Resume the transaction with the proof
	receipt, err := c.mcp.ResumeWithL3Proof(r.Context(), txHash, userID, proof)
	if err != nil {
		// If it's still a verification failure, return the receipt if available
		if receipt != nil {
			c.responder.JSON(w, http.StatusForbidden, receipt)
			return
		}
		c.responder.Error(w, http.StatusForbidden, err.Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, receipt)
}

// handleCLIApprovalStatus is an mTLS-authenticated endpoint that returns the current
// status of a suspended transaction. The CLI polls this endpoint after opening the
// browser approval page to detect when the user has completed the WebAuthn ceremony.
func (c *AuthController) handleCLIApprovalStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Path format: /api/v1/approvals/status/{txHash}
	txHash := strings.TrimPrefix(r.URL.Path, constants.APIPaths.ApprovalsCLIStatus)
	if txHash == "" {
		c.responder.Error(w, http.StatusBadRequest, "transaction hash required")
		return
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	suspendedTx, found, err := c.mcp.GetSuspendedTransaction(r.Context(), txHash)
	if err != nil {
		c.logger.Error("Failed to get suspended transaction", "error", err, "txHash", txHash)
		c.responder.Error(w, http.StatusInternalServerError, "failed to get transaction")
		return
	}

	if !found {
		c.responder.JSON(w, http.StatusOK, models.ApprovalStatusResponse{
			Status: "expired_or_not_found",
		})
		return
	}

	if suspendedTx.UserID != "" && suspendedTx.UserID != userID {
		c.responder.Error(w, http.StatusForbidden, "transaction belongs to another user")
		return
	}

	if suspendedTx.Approved {
		c.responder.JSON(w, http.StatusOK, models.ApprovalStatusResponse{
			Status:   "approved",
			TxHash:   txHash,
			ToolName: suspendedTx.ToolName,
		})
		return
	}

	c.responder.JSON(w, http.StatusOK, models.ApprovalStatusResponse{
		Status:   "pending",
		TxHash:   txHash,
		ToolName: suspendedTx.ToolName,
	})
}

func (c *AuthController) handleApprovalPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	txHash := strings.TrimPrefix(r.URL.Path, constants.APIPaths.ApprovePagePrefix)
	if txHash == "" {
		http.Error(w, "transaction hash required", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/console/#approve="+url.QueryEscape(txHash), http.StatusFound)
}

func (c *AuthController) handleListSuspendedTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Get suspended transactions from the suspended transaction store
	transactions, err := c.suspendedStore.ListSuspendedTransactions(r.Context(), userID)
	if err != nil {
		c.logger.Error("Failed to list suspended transactions", "error", err, "userID", userID)
		c.responder.Error(w, http.StatusInternalServerError, "failed to list suspended transactions")
		return
	}

	// Convert to JSON-serializable format
	var txResponses []models.SuspendedTxResponse
	for _, tx := range transactions {
		txResponses = append(txResponses, models.SuspendedTxResponse{
			TransactionHash: tx.TransactionHash,
			CreatedAt:       tx.CreatedAt,
			ExpiresAt:       tx.ExpiresAt,
			ToolName:        tx.ToolName,
			UserID:          tx.UserID,
			OperatorID:      tx.OperatorID,
		})
	}

	c.responder.JSON(w, http.StatusOK, models.SuspendedTransactionsResponse{
		Transactions: txResponses,
	})
}
