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
	"html"
	"net/http"
	"strings"
	"time"

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

	// If no action specified, treat as direct CLI approval (POST with mtls_cert_fingerprint)
	if action == "" {
		if r.Method != http.MethodPost {
			c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		c.handleCLIApproval(w, r, txHash, userID)
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

	var req AssertionResponse
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

func (c *AuthController) handleCLIApproval(w http.ResponseWriter, r *http.Request, txHash, userID string) {
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

	var req struct {
		CliSignature        string `json:"cli_signature"`
		MtlsCertFingerprint string `json:"mtls_cert_fingerprint"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.CliSignature == "" {
		c.responder.Error(w, http.StatusBadRequest, "cli_signature required")
		return
	}

	if req.MtlsCertFingerprint == "" {
		c.responder.Error(w, http.StatusBadRequest, "mtls_cert_fingerprint required")
		return
	}

	// Persist the approval with signature before resuming
	if err := c.suspendedStore.ApproveSuspendedTransaction(r.Context(), txHash, userID, req.CliSignature, req.MtlsCertFingerprint); err != nil {
		c.logger.Error("Failed to approve transaction", "error", err, "txHash", txHash, "userID", userID)
		c.responder.Error(w, http.StatusInternalServerError, "failed to approve transaction")
		return
	}

	// Create L3 proof with mtls_cert_fingerprint and cli_signature for CLI approval
	proof := &commonv1.L3Proof{
		MtlsCertFingerprint: req.MtlsCertFingerprint,
		CliSignature:        req.CliSignature,
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

func (c *AuthController) handleApprovalPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract transaction hash from URL path
	txHash := strings.TrimPrefix(r.URL.Path, constants.APIPaths.ApprovePagePrefix)
	if txHash == "" {
		http.Error(w, "transaction hash required", http.StatusBadRequest)
		return
	}

	// Retrieve suspended transaction from MCP gateway
	suspendedTx, ok, err := c.mcp.GetSuspendedTransaction(r.Context(), txHash)
	if err != nil || !ok {
		http.Error(w, "transaction not found or expired", http.StatusNotFound)
		return
	}

	// Format expiration time for display
	expiresAtStr := suspendedTx.ExpiresAt.Format(time.RFC3339)

	// Serve simple HTML approval page
	htmlStr := `<!DOCTYPE html>
<html>
<head>
    <title>Approve Transaction - g8e</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 40px auto; padding: 20px; }
        .container { border: 1px solid #ddd; border-radius: 8px; padding: 20px; }
        h1 { color: #333; }
        .transaction-info { background: #f5f5f5; padding: 15px; border-radius: 4px; margin: 20px 0; }
        .label { font-weight: bold; margin-bottom: 5px; }
        .value { margin-bottom: 15px; word-break: break-all; }
        .actions { margin-top: 20px; }
        button { padding: 10px 20px; margin-right: 10px; border: none; border-radius: 4px; cursor: pointer; font-size: 16px; }
        .approve { background: #4CAF50; color: white; }
        .reject { background: #f44336; color: white; }
        .approve:hover { background: #45a049; }
        .reject:hover { background: #da190b; }
        .status { margin-top: 20px; padding: 10px; border-radius: 4px; display: none; }
        .success { background: #d4edda; color: #155724; }
        .error { background: #f8d7da; color: #721c24; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Approve Transaction</h1>
        <p>A tool execution requires your authorization via WebAuthn.</p>
        
        <div class="transaction-info">
            <div class="label">Transaction Hash:</div>
            <div class="value">` + html.EscapeString(suspendedTx.TransactionHash) + `</div>
            
            <div class="label">Tool Name:</div>
            <div class="value">` + html.EscapeString(suspendedTx.ToolName) + `</div>
            
            <div class="label">Tool Arguments:</div>
            <div class="value"><pre>` + html.EscapeString(string(suspendedTx.ToolArguments)) + `</pre></div>
            
            <div class="label">Expires At:</div>
            <div class="value">` + html.EscapeString(expiresAtStr) + `</div>
        </div>
        
        <div class="actions">
            <button class="approve" onclick="approveTransaction()">Approve with WebAuthn</button>
            <button class="reject" onclick="rejectTransaction()">Reject</button>
        </div>
        
        <div id="status" class="status"></div>
    </div>

    <script>
        // Helper to convert base64url to Uint8Array
        function base64urlToUint8Array(base64url) {
            const padding = '='.repeat((4 - base64url.length % 4) % 4);
            const base64 = (base64url + padding).replace(/\-/g, '+').replace(/_/g, '/');
            const rawData = window.atob(base64);
            const outputArray = new Uint8Array(rawData.length);
            for (let i = 0; i < rawData.length; ++i) {
                outputArray[i] = rawData.charCodeAt(i);
            }
            return outputArray;
        }

        // Helper to convert Uint8Array to base64url
        function bufferToBase64url(buffer) {
            const bytes = new Uint8Array(buffer);
            let binary = '';
            for (let i = 0; i < bytes.byteLength; i++) {
                binary += String.fromCharCode(bytes[i]);
            }
            return window.btoa(binary)
                .replace(/\+/g, '-')
                .replace(/\//g, '_')
                .replace(/=/g, '');
        }

        async function approveTransaction() {
            const statusDiv = document.getElementById('status');
            const txHash = "` + html.EscapeString(suspendedTx.TransactionHash) + `";
            
            try {
                statusDiv.style.display = 'block';
                statusDiv.className = 'status';
                statusDiv.textContent = 'Requesting challenge...';

                // 1. Get Authentication Challenge
                const challengeResp = await fetch("/api/approve/" + txHash + "/challenge");
                if (!challengeResp.ok) {
                    if (challengeResp.status === 401) {
                        throw new Error("You must be logged in to approve transactions.");
                    }
                    const err = await challengeResp.json();
                    throw new Error(err.error || "Failed to get challenge");
                }
                const options = await challengeResp.json();

                // Prepare options for navigator.credentials.get
                options.publicKey.challenge = base64urlToUint8Array(options.publicKey.challenge);
                if (options.publicKey.allowCredentials) {
                    options.publicKey.allowCredentials.forEach(cred => {
                        cred.id = base64urlToUint8Array(cred.id);
                    });
                }

                statusDiv.textContent = 'Please follow your browser prompts to authorize with your passkey...';

                // 2. WebAuthn Assertion
                const assertion = await navigator.credentials.get({
                    publicKey: options.publicKey
                });

                statusDiv.textContent = 'Verifying authorization...';

                // 3. Verify Assertion
                const verifyResp = await fetch("/api/approve/" + txHash + "/verify", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({
                        id: assertion.id,
                        rawId: bufferToBase64url(assertion.rawId),
                        clientDataJSON: bufferToBase64url(assertion.response.clientDataJSON),
                        authenticatorData: bufferToBase64url(assertion.response.authenticatorData),
                        signature: bufferToBase64url(assertion.response.signature),
                        userHandle: assertion.response.userHandle ? bufferToBase64url(assertion.response.userHandle) : null
                    })
                });

                if (!verifyResp.ok) {
                    const err = await verifyResp.json();
                    throw new Error(err.error || "Verification failed");
                }

                const receipt = await verifyResp.json();
                statusDiv.className = 'status success';
                statusDiv.innerHTML = '<strong>Success!</strong><br/>Transaction approved and executed.<br/>Result: ' + (receipt.result_summary || "completed");
                
                // Optional: redirect back after success
                // setTimeout(() => { window.close(); }, 3000);
            } catch (err) {
                console.error(err);
                statusDiv.className = 'status error';
                statusDiv.textContent = 'Error: ' + err.message;
            }
        }

        function rejectTransaction() {
            const statusDiv = document.getElementById('status');
            statusDiv.style.display = 'block';
            statusDiv.className = 'status error';
            statusDiv.textContent = 'Transaction rejected. You can close this window.';
        }
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(htmlStr))
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

	// Query the user ID from the URL if provided (for admin access)
	queryUserID := r.URL.Query().Get("user_id")
	if queryUserID == "" {
		queryUserID = userID
	}

	// Get suspended transactions from the suspended transaction store
	transactions, err := c.suspendedStore.ListSuspendedTransactions(r.Context(), queryUserID)
	if err != nil {
		c.logger.Error("Failed to list suspended transactions", "error", err, "userID", queryUserID)
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
