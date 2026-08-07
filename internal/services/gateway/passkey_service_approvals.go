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

// handleApprovalAction dispatches approval sub-actions (challenge, verify) based on
// the path segment after /api/v1/approvals/{txHash}/. It is not a standalone REST
// endpoint — see handleApprovalChallenge and handleApprovalVerify for the annotated
// sub-handlers.
func (h *PasskeyHandler) handleApprovalAction(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		h.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, constants.APIPaths.ApprovalsPrefix)
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		h.responder.Error(w, http.StatusBadRequest, "invalid request path")
		return
	}

	txHash := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch action {
	case "challenge":
		h.handleApprovalChallenge(w, r, txHash, userID)
	case "verify":
		h.handleApprovalVerify(w, r, txHash, userID)
	default:
		h.responder.Error(w, http.StatusBadRequest, "unknown action")
	}
}

// @Summary		Get approval challenge
// @Description	Generates a WebAuthn assertion challenge for approving a suspended transaction.
// @Tags			approvals
// @Produce		json
// @Param		txHash	path		string	true	"Transaction hash"
// @Success		200		{object}	json.RawMessage	"WebAuthn assertion options"
// @Failure		404		{string}	string			"Transaction not found or expired"
// @Failure		403		{string}	string			"Forbidden — belongs to another user"
// @Router			/api/v1/approvals/{txHash}/challenge [get]
func (h *PasskeyHandler) handleApprovalChallenge(w http.ResponseWriter, r *http.Request, txHash, userID string) {
	if r.Method != http.MethodGet {
		h.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	suspendedTx, ok, err := h.orchestrator.GetSuspendedTransaction(r.Context(), txHash)
	if err != nil {
		h.logger.Error("Failed to get suspended transaction", "error", err, "txHash", txHash)
		h.responder.Error(w, http.StatusInternalServerError, "failed to get transaction")
		return
	}
	if !ok {
		h.responder.Error(w, http.StatusNotFound, "transaction not found or expired")
		return
	}

	if suspendedTx.UserID != "" && suspendedTx.UserID != userID {
		h.responder.Error(w, http.StatusForbidden, "transaction belongs to another user")
		return
	}

	options, err := h.GenerateApprovalChallenge(userID, txHash)
	if err != nil {
		h.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.responder.JSON(w, http.StatusOK, options)
}

// @Summary		Verify approval
// @Description	Verifies a WebAuthn assertion response to approve a suspended transaction. On success,
// @Description	the transaction resumes execution with the L3 proof attached and an SSE event is emitted.
// @Tags			approvals
// @Accept		json
// @Produce		json
// @Param		txHash	path		string						true	"Transaction hash"
// @Param		body	body		models.WebAuthnAssertionResponse	true	"WebAuthn assertion response"
// @Success		200		{object}	json.RawMessage						"ActionReceipt"
// @Failure		403		{string}	string						"Forbidden — verification failed"
// @Failure		404		{string}	string						"Transaction not found or expired"
// @Router			/api/v1/approvals/{txHash}/verify [post]
func (h *PasskeyHandler) handleApprovalVerify(w http.ResponseWriter, r *http.Request, txHash, userID string) {
	if r.Method != http.MethodPost {
		h.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	suspendedTx, ok, err := h.orchestrator.GetSuspendedTransaction(r.Context(), txHash)
	if err != nil {
		h.logger.Error("Failed to get suspended transaction", "error", err, "txHash", txHash)
		h.responder.Error(w, http.StatusInternalServerError, "failed to get transaction")
		return
	}
	if !ok {
		h.responder.Error(w, http.StatusNotFound, "transaction not found or expired")
		return
	}

	if suspendedTx.UserID != "" && suspendedTx.UserID != userID {
		h.responder.Error(w, http.StatusForbidden, "transaction belongs to another user")
		return
	}

	body, err := h.readBody(w, r)
	if err != nil {
		h.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req models.WebAuthnAssertionResponse
	if err := json.Unmarshal(body, &req); err != nil {
		h.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	proof := &commonv1.L3Proof{
		ClientDataJson:    req.ClientDataJSON,
		AuthenticatorData: req.AuthenticatorData,
		Signature:         req.Signature,
		CredentialId:      req.ID,
	}

	receipt, err := h.orchestrator.ResumeWithL3Proof(r.Context(), txHash, userID, proof)
	if err != nil {
		if receipt != nil {
			h.responder.JSON(w, http.StatusForbidden, receipt)
			return
		}
		h.responder.Error(w, http.StatusForbidden, err.Error())
		return
	}

	sseUserID := userID
	if sseUserID == "" {
		sseUserID = suspendedTx.UserID
	}
	h.orchestrator.EmitApprovalCompletedSSE(sseUserID, suspendedTx.SubmitterCLISessionID, txHash)

	h.responder.JSON(w, http.StatusOK, receipt)
}

// handleCLIApprovalStatus is an mTLS-authenticated endpoint that returns the current
// status of a suspended transaction. Used by CLI clients for post-SSE verification
// of approval state.
func (h *PasskeyHandler) handleCLIApprovalStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	txHash := strings.TrimPrefix(r.URL.Path, constants.APIPaths.ApprovalsCLIStatus)
	if txHash == "" {
		h.responder.Error(w, http.StatusBadRequest, "transaction hash required")
		return
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		h.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	suspendedTx, found, err := h.orchestrator.GetSuspendedTransaction(r.Context(), txHash)
	if err != nil {
		h.logger.Error("Failed to get suspended transaction", "error", err, "txHash", txHash)
		h.responder.Error(w, http.StatusInternalServerError, "failed to get transaction")
		return
	}

	if !found {
		h.responder.JSON(w, http.StatusOK, models.ApprovalStatusResponse{
			Status: string(constants.SuspendedTxStatusExpiredOrNotFound),
		})
		return
	}

	if suspendedTx.UserID != "" && suspendedTx.UserID != userID {
		h.responder.Error(w, http.StatusForbidden, "transaction belongs to another user")
		return
	}

	if suspendedTx.Approved {
		h.responder.JSON(w, http.StatusOK, models.ApprovalStatusResponse{
			Status:   string(constants.SuspendedTxStatusApproved),
			TxHash:   txHash,
			ToolName: suspendedTx.ToolName,
		})
		return
	}

	h.responder.JSON(w, http.StatusOK, models.ApprovalStatusResponse{
		Status:   string(constants.SuspendedTxStatusPending),
		TxHash:   txHash,
		ToolName: suspendedTx.ToolName,
	})
}

func (h *PasskeyHandler) handleApprovalPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	txHash := strings.TrimPrefix(r.URL.Path, constants.APIPaths.ApprovePagePrefix)
	if txHash == "" {
		http.Error(w, "transaction hash required", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/console/#approve="+url.QueryEscape(txHash), http.StatusFound)
}

func (h *PasskeyHandler) handleCLIListSuspended(w http.ResponseWriter, r *http.Request) {
	h.listSuspendedTransactions(w, r, true)
}

// @Summary		List pending approvals
// @Description	Lists suspended transactions pending L3 passkey approval for the authenticated user.
// @Tags			approvals
// @Produce		json
// @Success		200	{object}	models.SuspendedTransactionsResponse
// @Failure		401	{string}	string	"Unauthorized"
// @Router			/api/v1/approvals [get]
func (h *PasskeyHandler) handleListSuspendedTransactions(w http.ResponseWriter, r *http.Request) {
	h.listSuspendedTransactions(w, r, false)
}

func (h *PasskeyHandler) listSuspendedTransactions(w http.ResponseWriter, r *http.Request, filterApproved bool) {
	if r.Method != http.MethodGet {
		h.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		h.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	transactions, err := h.orchestrator.ListSuspendedTransactions(r.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to list suspended transactions", "error", err, "userID", userID)
		h.responder.Error(w, http.StatusInternalServerError, "failed to list suspended transactions")
		return
	}

	var txResponses []models.SuspendedTxResponse
	for _, tx := range transactions {
		if filterApproved && tx.Approved {
			continue
		}
		txResponses = append(txResponses, models.SuspendedTxResponse{
			TransactionHash: tx.TransactionHash,
			CreatedAt:       tx.CreatedAt,
			ExpiresAt:       tx.ExpiresAt,
			ToolName:        tx.ToolName,
			UserID:          tx.UserID,
			OperatorID:      tx.OperatorID,
		})
	}

	h.responder.JSON(w, http.StatusOK, models.SuspendedTransactionsResponse{
		Transactions: txResponses,
	})
}
