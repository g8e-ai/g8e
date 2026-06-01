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
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	"github.com/google/uuid"
)

// AuthController handles authentication, passkey, and approval endpoints.
type AuthController struct {
	cfg        *config.Config
	logger     *slog.Logger
	db         *GatewayDBService
	auth       *AuthService
	passkey    *PasskeyService
	userSvc    *UserService
	reg        *RegistrationService
	pki        *PKIAuthority
	sessionSvc *SessionService
	mcp        *mcp.GatewayService
	responder  *responder.Responder
}

func newAuthController(cfg *config.Config, logger *slog.Logger, db *GatewayDBService, auth *AuthService, passkey *PasskeyService, userSvc *UserService, reg *RegistrationService, pki *PKIAuthority, sessionSvc *SessionService, mcp *mcp.GatewayService, responder *responder.Responder) *AuthController {
	return &AuthController{
		cfg:        cfg,
		logger:     logger,
		db:         db,
		auth:       auth,
		passkey:    passkey,
		userSvc:    userSvc,
		reg:        reg,
		pki:        pki,
		sessionSvc: sessionSvc,
		mcp:        mcp,
		responder:  responder,
	}
}

func (c *AuthController) readBody(r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, c.cfg.Gateway.MaxPayloadBytes)
	return io.ReadAll(r.Body)
}

// Passkey / L3 Brokerage Handlers
// =============================================================================

func (c *AuthController) handleAuthPasskeysRegisterChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var req struct {
		UserID   string `json:"user_id"`
		UserName string `json:"user_name"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// [PIVOT] Enforce session-to-user binding for public browser registration
	if ctxUserID, ok := r.Context().Value(userIDKey).(string); ok {
		if req.UserID != "" && req.UserID != ctxUserID {
			c.responder.Error(w, http.StatusForbidden, "user_id mismatch with session")
			return
		}
		req.UserID = ctxUserID
	}

	if req.UserID == "" {
		c.responder.Error(w, http.StatusBadRequest, "user_id required")
		return
	}

	// JIT passkey bootstrap: enforce first-credential-only when coming through JIT routes.
	// These routes are mounted behind JWTAuthMiddleware in the public router.
	// This prevents a stolen JWT from silently adding attacker-controlled authenticators.
	if strings.Contains(r.URL.Path, "/jit-") {
		user, err := c.userSvc.GetByID(req.UserID)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, "failed to fetch user")
			return
		}
		if user == nil {
			c.responder.Error(w, http.StatusNotFound, "user not found")
			return
		}
		if len(user.PasskeyCredentials) > 0 {
			c.responder.Error(w, http.StatusForbidden, "first-credential registration only; user already has credentials, require step-up via existing passkey or mTLS")
			return
		}
	}

	options, err := c.passkey.GenerateRegistrationChallenge(req.UserID, req.UserName)
	if err != nil {
		c.logger.Warn("Passkey register challenge failed", string(constants.ConnectionStateError), err, "userID", req.UserID)
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, map[string]interface{}{
		string(constants.AuthAuditResultSuccess): true,
		"options":                                options,
	})
}

func (c *AuthController) handleAuthPasskeysRegisterVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var req struct {
		UserID              string               `json:"user_id"`
		AttestationResponse *AttestationResponse `json:"attestation_response"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// [PIVOT] Enforce session-to-user binding for public browser registration
	if ctxUserID, ok := r.Context().Value(userIDKey).(string); ok {
		if req.UserID != "" && req.UserID != ctxUserID {
			c.responder.Error(w, http.StatusForbidden, "user_id mismatch with session")
			return
		}
		req.UserID = ctxUserID
	}

	if req.UserID == "" {
		c.responder.Error(w, http.StatusBadRequest, "user_id required")
		return
	}

	// JIT passkey bootstrap: enforce first-credential-only when coming through JIT routes.
	// These routes are mounted behind JWTAuthMiddleware in the public router.
	// This prevents a stolen JWT from silently adding attacker-controlled authenticators.
	if strings.Contains(r.URL.Path, "/jit-") {
		user, err := c.userSvc.GetByID(req.UserID)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, "failed to fetch user")
			return
		}
		if user == nil {
			c.responder.Error(w, http.StatusNotFound, "user not found")
			return
		}
		if len(user.PasskeyCredentials) > 0 {
			c.responder.Error(w, http.StatusForbidden, "first-credential registration only; user already has credentials, require step-up via existing passkey or mTLS")
			return
		}
	}

	cred, err := c.passkey.VerifyRegistration(req.UserID, r)
	if err != nil {
		c.logger.Warn("Passkey register verify failed", string(constants.ConnectionStateError), err, "userID", req.UserID)
		c.responder.JSON(w, http.StatusOK, models.PasskeyVerifyResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.responder.JSON(w, http.StatusOK, models.PasskeyVerifyResponse{
		Success:    true,
		Credential: cred,
	})
}

func (c *AuthController) handleAuthPasskeysAuthenticateChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	userID := req.UserID
	if ctxUserID, ok := r.Context().Value(userIDKey).(string); ok {
		if userID != "" && userID != ctxUserID {
			c.responder.Error(w, http.StatusForbidden, "user_id mismatch with session")
			return
		}
		userID = ctxUserID
	}

	if userID == "" {
		c.responder.Error(w, http.StatusBadRequest, "user_id required")
		return
	}

	options, err := c.passkey.GenerateAuthenticationChallenge(userID)
	if err != nil {
		c.logger.Warn("Passkey auth challenge failed", string(constants.ConnectionStateError), err, "userID", userID)
		c.responder.JSON(w, http.StatusOK, models.PasskeyChallengeResponse{
			Success:    false,
			Error:      err.Error(),
			NeedsSetup: err.Error() == "no passkeys registered",
		})
		return
	}

	c.responder.JSON(w, http.StatusOK, models.PasskeyChallengeResponse{
		Success: true,
		Options: options,
	})
}

func (c *AuthController) handleAuthPasskeysAuthenticateVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var req struct {
		UserID            string             `json:"user_id"`
		AssertionResponse *AssertionResponse `json:"assertion_response"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	userID := req.UserID
	if ctxUserID, ok := r.Context().Value(userIDKey).(string); ok {
		if userID != "" && userID != ctxUserID {
			c.responder.Error(w, http.StatusForbidden, "user_id mismatch with session")
			return
		}
		userID = ctxUserID
	}

	if userID == "" {
		c.responder.Error(w, http.StatusBadRequest, "user_id required")
		return
	}

	cred, err := c.passkey.VerifyAuthentication(userID, r)
	if err != nil {
		c.logger.Warn("Passkey auth verify failed", string(constants.ConnectionStateError), err, "userID", userID)
		c.responder.JSON(w, http.StatusOK, map[string]interface{}{
			string(constants.AuthAuditResultSuccess): false,
			string(constants.ConnectionStateError):   err.Error(),
		})
		return
	}

	webSession, err := c.passkey.CreateWebSession(userID)
	if err != nil {
		c.logger.Error("Failed to create web session after auth", string(constants.ConnectionStateError), err, "userID", userID)
		c.responder.JSON(w, http.StatusOK, map[string]interface{}{
			string(constants.AuthAuditResultSuccess): false,
			string(constants.ConnectionStateError):   "authentication succeeded but session creation failed",
		})
		return
	}

	c.responder.JSON(w, http.StatusOK, map[string]interface{}{
		string(constants.AuthAuditResultSuccess): true,
		"user_id":                                userID,
		"credential":                             cred,
		string(constants.SessionKeyPrefixWeb): map[string]interface{}{
			"id":                 webSession.ID,
			"expires_at_unix_ms": webSession.ExpiresAtUnixMs,
		},
	})
}

func (c *AuthController) handleAuthPasskeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		c.responder.Error(w, http.StatusBadRequest, "user_id required")
		return
	}

	creds, err := c.passkey.ListCredentials(userID)
	if err != nil {
		c.logger.Error("Failed to list credentials", string(constants.ConnectionStateError), err, "userID", userID)
		c.responder.Error(w, http.StatusInternalServerError, "failed to list credentials")
		return
	}

	c.responder.JSON(w, http.StatusOK, models.PasskeyCredentialsResponse{
		Success:     true,
		Credentials: creds,
	})
}

func (c *AuthController) handleAuthPasskeysRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		c.responder.Error(w, http.StatusBadRequest, "user_id required")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, constants.APIPaths.AuthPasskeysPrefix)
	if path == "" {
		c.responder.Error(w, http.StatusBadRequest, "credential_id required")
		return
	}

	found, remaining, err := c.passkey.RevokeCredential(userID, path)
	if err != nil {
		c.logger.Error("Failed to revoke credential", string(constants.ConnectionStateError), err, "userID", userID)
		c.responder.Error(w, http.StatusInternalServerError, "failed to revoke credential")
		return
	}

	c.responder.JSON(w, http.StatusOK, models.PasskeyRevokeResponse{
		Success:   true,
		Found:     found,
		Remaining: remaining,
	})
}

func (c *AuthController) handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Zero-PII: Email-based user creation removed
	// Users are created with only a generated ID and passkey credentials
	user, err := c.userSvc.CreateUser()
	if err != nil {
		c.logger.Warn("Failed to create user", string(constants.ConnectionStateError), err)
		c.responder.Error(w, http.StatusConflict, err.Error())
		return
	}

	c.responder.JSON(w, http.StatusCreated, map[string]interface{}{
		string(constants.AuthAuditResultSuccess): true,
		"user_id":                                user.ID,
	})
}

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

	userID, ok := r.Context().Value(userIDKey).(string)
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
	suspendedTx, ok := c.mcp.GetSuspendedTransaction(txHash)
	if !ok {
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
	suspendedTx, ok := c.mcp.GetSuspendedTransaction(txHash)
	if !ok {
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
	suspendedTx, ok := c.mcp.GetSuspendedTransaction(txHash)
	if !ok {
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
	if err := c.db.ApproveSuspendedTransaction(txHash, userID, req.CliSignature, req.MtlsCertFingerprint); err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Sprintf("failed to approve transaction: %v", err))
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
	suspendedTx, ok := c.mcp.GetSuspendedTransaction(txHash)
	if !ok {
		http.Error(w, "transaction not found or expired", http.StatusNotFound)
		return
	}

	// Format expiration time for display
	expiresAtStr := suspendedTx.ExpiresAt.Format(time.RFC3339)

	// Serve simple HTML approval page
	html := `<!DOCTYPE html>
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
	_, _ = w.Write([]byte(html))
}

func (c *AuthController) handleListSuspendedTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, ok := r.Context().Value(userIDKey).(string)
	if !ok || userID == "" {
		c.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Query the user ID from the URL if provided (for admin access)
	queryUserID := r.URL.Query().Get("user_id")
	if queryUserID == "" {
		queryUserID = userID
	}

	// Get suspended transactions from the gateway DB service
	transactions, err := c.db.ListSuspendedTransactions(queryUserID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Sprintf("failed to list suspended transactions: %v", err))
		return
	}

	// Convert to JSON-serializable format
	type SuspendedTxResponse struct {
		TransactionHash string    `json:"transaction_hash"`
		CreatedAt       time.Time `json:"created_at"`
		ExpiresAt       time.Time `json:"expires_at"`
		ToolName        string    `json:"tool_name"`
		UserID          string    `json:"user_id"`
		OperatorID      string    `json:"operator_id"`
	}

	var txResponses []SuspendedTxResponse
	for _, tx := range transactions {
		txResponses = append(txResponses, SuspendedTxResponse{
			TransactionHash: tx.TransactionHash,
			CreatedAt:       tx.CreatedAt,
			ExpiresAt:       tx.ExpiresAt,
			ToolName:        tx.ToolName,
			UserID:          tx.UserID,
			OperatorID:      tx.OperatorID,
		})
	}

	c.responder.JSON(w, http.StatusOK, map[string]interface{}{
		"transactions": txResponses,
	})
}

// =============================================================================
// Browser Auth Handlers (Public Router)
// =============================================================================

func (c *AuthController) handlePublicAuthLoginVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req struct {
		UserID            string             `json:"user_id"`
		AssertionResponse *AssertionResponse `json:"assertion_response"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	_, err = c.passkey.VerifyAuthentication(req.UserID, r)
	if err != nil {
		c.responder.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	webSession, err := c.passkey.CreateWebSession(req.UserID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to create web session")
		return
	}

	// Set HttpOnly Secure SameSite=Lax cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "g8e_session",
		Value:    webSession.ID,
		Path:     "/",
		Expires:  time.Unix(webSession.ExpiresAtUnixMs/1000, 0),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	c.responder.JSON(w, http.StatusOK, models.AuthLoginVerifyResponse{
		Success:      true,
		UserID:       req.UserID,
		WebSessionID: webSession.ID,
	})
}

func (c *AuthController) handlePublicAuthLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("g8e_session")
	if err == nil {
		// Best effort delete web session from DB
		_, _ = c.db.DocDelete(marshaler.CollectionName(constants.CollectionWebSessions), cookie.Value)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "g8e_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	c.responder.JSON(w, http.StatusOK, models.StatusResponse{Status: constants.GatewayModeStatusOK})
}

func (c *AuthController) handlePublicAuthBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := c.readBody(r)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req struct {
		Name              string `json:"name"`
		CSRPEM            string `json:"csr_pem"`
		CLICSRPEM         string `json:"cli_csr_pem,omitempty"`
		SystemFingerprint string `json:"system_fingerprint"`
		Hostname          string `json:"hostname"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Check if CSR signing is requested
	csrRequested := req.CSRPEM != ""

	// If CSR is requested for device enrollment (not local bootstrap), allow remote requests
	// Device enrollment requests include hostname and system_fingerprint fields
	isDeviceEnrollment := req.Hostname != "" && req.SystemFingerprint != ""

	// Only enforce loopback gate for local bootstrap, not device enrollment
	if csrRequested && !isDeviceEnrollment {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			c.logger.Warn("Bootstrap CSR request rejected: not from loopback", "remote_addr", r.RemoteAddr)
			c.responder.Error(w, http.StatusForbidden, "CSR auto-issue only available over loopback")
			return
		}
	}

	// Check for existing bootstrap user (plan §4.2, §9.1 rotation carve-out)
	bootstrapUser, err := c.userSvc.FindBootstrapUser()
	if err != nil {
		c.logger.Error("Failed to check for existing bootstrap user", string(constants.ConnectionStateError), err)
		c.responder.Error(w, http.StatusInternalServerError, "bootstrap check failed")
		return
	}

	var user *models.User
	if bootstrapUser != nil {
		// Bootstrap user exists - check if rotation is allowed
		if !bootstrapUser.IsActive() {
			c.logger.Warn("Bootstrap user is disabled, refusing rotation", "user_id", bootstrapUser.ID)
			c.responder.Error(w, http.StatusConflict, "bootstrap user is disabled, cannot rotate")
			return
		}
		if !csrRequested {
			c.logger.Warn("Bootstrap user exists but no CSR requested", "user_id", bootstrapUser.ID)
			c.responder.Error(w, http.StatusForbidden, "bootstrap already exists, CSR required for rotation")
			return
		}
		// Rotation allowed: active bootstrap user + CSR + loopback
		user = bootstrapUser
		c.logger.Info("[BOOTSTRAP] Rotating existing bootstrap user", "user_id", user.ID)
	} else {
		// No bootstrap user exists - create one.
		// Defense-in-depth: refuse if any user already exists, so bootstrap can
		// only run on a genuinely empty system.
		hasUsers, err := c.userSvc.HasAnyUsers()
		if err != nil {
			c.logger.Error("Failed to check for existing users during bootstrap", string(constants.ConnectionStateError), err)
			c.responder.Error(w, http.StatusInternalServerError, "bootstrap check failed")
			return
		}
		if hasUsers {
			c.logger.Warn("Bootstrap attempted on non-empty system", "remote_addr", r.RemoteAddr)
			c.responder.Error(w, http.StatusForbidden, "bootstrap only available for initial setup")
			return
		}

		// Create the bootstrap user
		// Zero-PII: Bootstrap user created with only generated ID
		user, err = c.userSvc.CreateBootstrapUser()
		if err != nil {
			c.logger.Error("Failed to create bootstrap user", string(constants.ConnectionStateError), err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to create user")
			return
		}
	}

	// Issue a web session cookie for passkey registration
	webSession, err := c.passkey.CreateWebSession(user.ID)
	if err != nil {
		c.logger.Error("Failed to create web session for bootstrap user", string(constants.ConnectionStateError), err, "user_id", user.ID)
		c.responder.Error(w, http.StatusInternalServerError, "user created but web session failed")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "g8e_session",
		Value:    webSession.ID,
		Path:     "/",
		Expires:  time.Unix(webSession.ExpiresAtUnixMs/1000, 0),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	response := map[string]interface{}{
		string(constants.AuthAuditResultSuccess): true,
		string(constants.HistoryActorUser):       user,
		string(constants.SessionKeyPrefixWeb):    webSession,
	}

	// If CSR is requested and loopback, sign and return cert (plan §4.2)
	if csrRequested {
		// Create operator slot for the bootstrap user
		operatorID := uuid.NewString()
		sessionID := uuid.NewString()
		cliSessionID := uuid.NewString()
		orgID := user.ID // Use user ID as org ID for bootstrap
		now := time.Now().UTC()

		operator := &models.OperatorDocumentGo{
			ID:                operatorID,
			UserID:            user.ID,
			OrganizationID:    orgID,
			Component:         constants.ComponentNameG8EO,
			Name:              "bootstrap-operator",
			Status:            constants.OperatorStatusActive,
			OperatorSessionID: sessionID,
			OperatorType:      constants.OperatorTypeSystem,
			SystemFingerprint: req.SystemFingerprint,
			Claimed:           true,
			ClaimedAt:         &now,
			CreatedAt:         now,
			UpdatedAt:         now,
		}

		// Sign the CSR
		certPEM, chainPEM, err := c.pki.SignCSR(req.CSRPEM, constants.LeafTypeOperator, orgID, operatorID, user.ID, sessionID, "")
		if err != nil {
			c.logger.Error("Failed to sign bootstrap CSR", string(constants.ConnectionStateError), err, "user_id", user.ID)
			c.responder.Error(w, http.StatusInternalServerError, "failed to sign CSR")
			return
		}

		operator.OperatorCert = certPEM

		// CLI certificate generation (mandatory)
		if req.CLICSRPEM == "" {
			c.logger.Error("Bootstrap request missing mandatory CLI CSR", "user_id", user.ID)
			c.responder.Error(w, http.StatusBadRequest, "cli_csr_pem is mandatory")
			return
		}

		cliCertPEM, cliCertChainPEM, err := c.pki.SignCSR(req.CLICSRPEM, constants.LeafTypeCLI, "", "", user.ID, cliSessionID, "")
		if err != nil {
			c.logger.Error("Failed to sign bootstrap CLI CSR", string(constants.ConnectionStateError), err, "user_id", user.ID)
			c.responder.Error(w, http.StatusInternalServerError, "failed to sign CLI CSR")
			return
		}

		// Calculate CLI certificate fingerprint and serial for L3 verification
		cliCertFingerprint := calculateFingerprintFromPEM(cliCertPEM)
		cliCertSerial := calculateSerialFromPEM(cliCertPEM)

		// Persist operator document
		opBytes, err := json.Marshal(operator)
		if err != nil {
			c.logger.Error("Failed to marshal operator document", string(constants.ConnectionStateError), err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to create operator")
			return
		}
		if err := c.db.DocSet(marshaler.CollectionName(constants.CollectionOperators), operatorID, opBytes); err != nil {
			c.logger.Error("Failed to persist operator document", string(constants.ConnectionStateError), err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to create operator")
			return
		}

		// Fetch trust bundle
		hubBundle, err := c.pki.GatewayTrustBundle()
		if err != nil {
			c.logger.Warn("Failed to fetch hub trust bundle", string(constants.ConnectionStateError), err)
			// Non-fatal - continue without bundle
		}

		err = c.sessionSvc.PersistSessions(
			cliSessionID,
			sessionID,
			user.ID,
			orgID,
			operatorID,
			req.SystemFingerprint,
			cliCertFingerprint,
			cliCertSerial,
			string(constants.HeartbeatTypeBootstrap),
		)
		if err != nil {
			c.logger.Error("Failed to persist sessions during bootstrap", string(constants.ConnectionStateError), err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to persist sessions")
			return
		}

		response["operator_cert"] = certPEM
		response["operator_cert_chain"] = chainPEM
		response["hub_trust_bundle"] = string(hubBundle)
		response["operator_session_id"] = sessionID
		response["operator_id"] = operatorID
		response["cli_session_id"] = cliSessionID
		response["cli_cert"] = cliCertPEM
		response["cli_cert_chain"] = cliCertChainPEM
		response["user_id"] = user.ID

		c.logger.Info("[BOOTSTRAP] System initialized with bootstrap user and CLI cert", "user_id", user.ID, "operator_id", operatorID, "cli_session_id_prefix", cliSessionID[:8])
	} else {
		c.logger.Info("[BOOTSTRAP] System initialized with bootstrap user (no CSR)", "user_id", user.ID)
	}

	c.responder.JSON(w, http.StatusCreated, response)
}

func (c *AuthController) handleBootstrapStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	hasUsers, err := c.userSvc.HasAnyUsers()
	if err != nil {
		c.logger.Error("Failed to check for existing users", string(constants.ConnectionStateError), err)
		c.responder.Error(w, http.StatusInternalServerError, "status check failed")
		return
	}

	c.responder.JSON(w, http.StatusOK, models.BootstrapStatusResponse{
		Bootstrapped: hasUsers,
	})
}

func (c *AuthController) handleUserMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDKey).(string)
	if !ok {
		c.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := c.userSvc.GetByID(userID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to get user")
		return
	}
	if user == nil {
		c.responder.Error(w, http.StatusNotFound, "user not found")
		return
	}

	c.responder.JSON(w, http.StatusOK, models.UserMeResponse{
		Success: true,
		User:    user,
	})
}

func (c *AuthController) handleWebSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(userIDKey).(string)
	if !ok {
		c.responder.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	cookie, _ := r.Cookie("g8e_session")
	webSessionID := ""
	if cookie != nil {
		webSessionID = cookie.Value
	}

	c.responder.JSON(w, http.StatusOK, models.WebSessionResponse{
		Success:      true,
		UserID:       userID,
		WebSessionID: webSessionID,
	})
}
