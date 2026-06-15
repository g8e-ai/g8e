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
	"errors"
	"net/http"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

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
	if ctxUserID, ok := r.Context().Value(constants.ContextKeyUserID).(string); ok {
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
		c.logger.Warn("Passkey register challenge failed", "error", err, "userID", req.UserID)
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, models.PasskeyRegisterChallengeResponse{
		Success: true,
		Options: options,
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
	if ctxUserID, ok := r.Context().Value(constants.ContextKeyUserID).(string); ok {
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

	responseJSON, err := json.Marshal(req.AttestationResponse)
	if err != nil {
		c.logger.Warn("Failed to marshal attestation response", "error", err, "userID", req.UserID)
		c.responder.JSON(w, http.StatusOK, models.PasskeyVerifyResponse{
			Success: false,
			Error:   "failed to marshal attestation response",
		})
		return
	}

	cred, err := c.passkey.VerifyRegistration(req.UserID, responseJSON)
	if err != nil {
		c.logger.Warn("Passkey register verify failed", "error", err, "userID", req.UserID)
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
	if ctxUserID, ok := r.Context().Value(constants.ContextKeyUserID).(string); ok {
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
		c.logger.Warn("Passkey auth challenge failed", "error", err, "userID", userID)
		c.responder.JSON(w, http.StatusOK, models.PasskeyChallengeResponse{
			Success:    false,
			Error:      err.Error(),
			NeedsSetup: errors.Is(err, constants.ErrNoPasskeysRegistered),
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
	if ctxUserID, ok := r.Context().Value(constants.ContextKeyUserID).(string); ok {
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

	responseJSON, err := json.Marshal(req.AssertionResponse)
	if err != nil {
		c.logger.Warn("Failed to marshal assertion response", "error", err, "userID", userID)
		c.responder.JSON(w, http.StatusOK, models.PasskeyAuthVerifyResponse{
			Success: false,
			Error:   "failed to marshal assertion response",
		})
		return
	}

	cred, err := c.passkey.VerifyAuthentication(userID, responseJSON)
	if err != nil {
		c.logger.Warn("Passkey auth verify failed", "error", err, "userID", userID)
		c.responder.JSON(w, http.StatusOK, models.PasskeyAuthVerifyResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	webSession, err := c.webSessionSvc.CreateWebSession(userID)
	if err != nil {
		c.logger.Error("Failed to create web session after auth", "error", err, "userID", userID)
		c.responder.JSON(w, http.StatusOK, models.PasskeyAuthVerifyResponse{
			Success: false,
			Error:   "authentication succeeded but session creation failed",
		})
		return
	}

	c.responder.JSON(w, http.StatusOK, models.PasskeyAuthVerifyResponse{
		Success:    true,
		UserID:     userID,
		Credential: cred,
		WebSession: &models.WebSessionInfo{
			ID:              webSession.ID,
			ExpiresAtUnixMs: webSession.ExpiresAtUnixMs,
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
		c.logger.Error("Failed to list credentials", "error", err, "userID", userID)
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
		c.logger.Error("Failed to revoke credential", "error", err, "userID", userID)
		c.responder.Error(w, http.StatusInternalServerError, "failed to revoke credential")
		return
	}

	c.responder.JSON(w, http.StatusOK, models.PasskeyRevokeResponse{
		Success:   true,
		Found:     found,
		Remaining: remaining,
	})
}
