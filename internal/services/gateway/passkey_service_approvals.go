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

func (s *PasskeyService) handleApprovalAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, constants.APIPaths.ApprovalsPrefix)
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		s.responder.Error(w, http.StatusBadRequest, "invalid request path")
		return
	}

	txHash := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		s.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	switch action {
	case "challenge":
		s.handleApprovalChallenge(w, r, txHash, userID)
	case "verify":
		s.handleApprovalVerify(w, r, txHash, userID)
	default:
		s.responder.Error(w, http.StatusBadRequest, "unknown action")
	}
}

func (s *PasskeyService) handleApprovalChallenge(w http.ResponseWriter, r *http.Request, txHash, userID string) {
	if r.Method != http.MethodGet {
		s.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	suspendedTx, ok, err := s.mcpSvc.GetSuspendedTransaction(r.Context(), txHash)
	if err != nil || !ok {
		s.responder.Error(w, http.StatusNotFound, "transaction not found or expired")
		return
	}

	if suspendedTx.UserID != "" && suspendedTx.UserID != userID {
		s.responder.Error(w, http.StatusForbidden, "transaction belongs to another user")
		return
	}

	options, err := s.GenerateApprovalChallenge(userID, txHash)
	if err != nil {
		s.responder.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.responder.JSON(w, http.StatusOK, options)
}

func (s *PasskeyService) handleApprovalVerify(w http.ResponseWriter, r *http.Request, txHash, userID string) {
	if r.Method != http.MethodPost {
		s.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	suspendedTx, ok, err := s.mcpSvc.GetSuspendedTransaction(r.Context(), txHash)
	if err != nil || !ok {
		s.responder.Error(w, http.StatusNotFound, "transaction not found or expired")
		return
	}

	if suspendedTx.UserID != "" && suspendedTx.UserID != userID {
		s.responder.Error(w, http.StatusForbidden, "transaction belongs to another user")
		return
	}

	body, err := s.readBody(w, r)
	if err != nil {
		s.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req models.WebAuthnAssertionResponse
	if err := json.Unmarshal(body, &req); err != nil {
		s.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	proof := &commonv1.L3Proof{
		ClientDataJson:    req.ClientDataJSON,
		AuthenticatorData: req.AuthenticatorData,
		Signature:         req.Signature,
		CredentialId:      req.ID,
	}

	receipt, err := s.mcpSvc.ResumeWithL3Proof(r.Context(), txHash, userID, proof)
	if err != nil {
		if receipt != nil {
			s.responder.JSON(w, http.StatusForbidden, receipt)
			return
		}
		s.responder.Error(w, http.StatusForbidden, err.Error())
		return
	}

	s.responder.JSON(w, http.StatusOK, receipt)
}

// handleCLIApprovalStatus is an mTLS-authenticated endpoint that returns the current
// status of a suspended transaction. The CLI polls this endpoint after opening the
// browser approval page to detect when the user has completed the WebAuthn ceremony.
func (s *PasskeyService) handleCLIApprovalStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	txHash := strings.TrimPrefix(r.URL.Path, constants.APIPaths.ApprovalsCLIStatus)
	if txHash == "" {
		s.responder.Error(w, http.StatusBadRequest, "transaction hash required")
		return
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		s.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	suspendedTx, found, err := s.mcpSvc.GetSuspendedTransaction(r.Context(), txHash)
	if err != nil {
		s.logger.Error("Failed to get suspended transaction", "error", err, "txHash", txHash)
		s.responder.Error(w, http.StatusInternalServerError, "failed to get transaction")
		return
	}

	if !found {
		s.responder.JSON(w, http.StatusOK, models.ApprovalStatusResponse{
			Status: "expired_or_not_found",
		})
		return
	}

	if suspendedTx.UserID != "" && suspendedTx.UserID != userID {
		s.responder.Error(w, http.StatusForbidden, "transaction belongs to another user")
		return
	}

	if suspendedTx.Approved {
		s.responder.JSON(w, http.StatusOK, models.ApprovalStatusResponse{
			Status:   "approved",
			TxHash:   txHash,
			ToolName: suspendedTx.ToolName,
		})
		return
	}

	s.responder.JSON(w, http.StatusOK, models.ApprovalStatusResponse{
		Status:   "pending",
		TxHash:   txHash,
		ToolName: suspendedTx.ToolName,
	})
}

func (s *PasskeyService) handleApprovalPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	txHash := strings.TrimPrefix(r.URL.Path, constants.APIPaths.ApprovePagePrefix)
	if txHash == "" {
		http.Error(w, "transaction hash required", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/console/#approve="+url.QueryEscape(txHash), http.StatusFound)
}

func (s *PasskeyService) handleListSuspendedTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		s.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	transactions, err := s.suspendedStore.ListSuspendedTransactions(r.Context(), userID)
	if err != nil {
		s.logger.Error("Failed to list suspended transactions", "error", err, "userID", userID)
		s.responder.Error(w, http.StatusInternalServerError, "failed to list suspended transactions")
		return
	}

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

	s.responder.JSON(w, http.StatusOK, models.SuspendedTransactionsResponse{
		Transactions: txResponses,
	})
}
