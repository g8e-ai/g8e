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
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/google/uuid"
)

// CLI Passkey Bootstrap Handlers
// =============================================================================

// handleCLIPasskeyRegisterChallenge handles passkey registration challenges for CLI bootstrap.
// This is a public endpoint (no auth) for the initial bootstrap where no credentials exist yet.
// It enforces first-credential-only to prevent credential stuffing attacks.
func (c *AuthController) handleCLIPasskeyRegisterChallenge(w http.ResponseWriter, r *http.Request) {
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

	if req.UserID == "" {
		c.responder.Error(w, http.StatusBadRequest, "user_id required")
		return
	}

	// Enforce first-credential-only for CLI bootstrap, unless authenticated via mTLS
	user, err := c.userSvc.GetByID(req.UserID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to fetch user")
		return
	}
	if user == nil {
		c.responder.Error(w, http.StatusNotFound, "user not found")
		return
	}

	// Check if already authenticated via mTLS
	authenticatedUserID, _ := r.Context().Value(constants.ContextKeyUserID).(string)
	isEnrolled := authenticatedUserID == req.UserID

	if len(user.PasskeyCredentials) > 0 && !isEnrolled {
		c.responder.Error(w, http.StatusForbidden, "first-credential registration only; user already has credentials")
		return
	}

	options, err := c.passkey.GenerateRegistrationChallenge(req.UserID, req.UserName)
	if err != nil {
		c.logger.Warn("CLI passkey register challenge failed", "error", err, "userID", req.UserID)
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, models.PasskeyRegisterChallengeResponse{
		Success: true,
		Options: options,
	})
}

// handleCLIPasskeyRegisterVerify handles passkey registration verification for CLI bootstrap.
// This is a public endpoint (no auth) for the initial bootstrap where no credentials exist yet.
// It enforces first-credential-only to prevent credential stuffing attacks.
func (c *AuthController) handleCLIPasskeyRegisterVerify(w http.ResponseWriter, r *http.Request) {
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

	if req.UserID == "" {
		c.responder.Error(w, http.StatusBadRequest, "user_id required")
		return
	}

	// Enforce first-credential-only for CLI bootstrap, unless authenticated via mTLS
	user, err := c.userSvc.GetByID(req.UserID)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to fetch user")
		return
	}
	if user == nil {
		c.responder.Error(w, http.StatusNotFound, "user not found")
		return
	}

	// Check if already authenticated via mTLS
	authenticatedUserID, _ := r.Context().Value(constants.ContextKeyUserID).(string)
	isEnrolled := authenticatedUserID == req.UserID

	if len(user.PasskeyCredentials) > 0 && !isEnrolled {
		c.responder.Error(w, http.StatusForbidden, "first-credential registration only; user already has credentials")
		return
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
		c.logger.Warn("CLI passkey register verify failed", "error", err, "userID", req.UserID)
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

// handleCLIBrowserPasskeyRegisterChallenge handles passkey registration challenges for browser-based CLI bootstrap.
// This is a public endpoint (no auth) for the initial bootstrap where no credentials exist yet.
// It creates a web session and binds it to the CLI session ID after successful registration.
func (c *AuthController) handleCLIBrowserPasskeyRegisterChallenge(w http.ResponseWriter, r *http.Request) {
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
		UserID       string `json:"user_id"`
		UserName     string `json:"user_name"`
		CLISessionID string `json:"cli_session_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.UserID == "" {
		c.responder.Error(w, http.StatusBadRequest, "user_id required")
		return
	}

	// Enforce first-credential-only for CLI bootstrap
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
		c.responder.Error(w, http.StatusForbidden, "first-credential registration only; user already has credentials")
		return
	}

	options, err := c.passkey.GenerateRegistrationChallenge(req.UserID, req.UserName)
	if err != nil {
		c.logger.Warn("CLI browser passkey register challenge failed", "error", err, "userID", req.UserID)
		c.responder.JSON(w, http.StatusOK, models.PasskeyRegisterChallengeResponse{
			Success: false,
		})
		return
	}

	c.responder.JSON(w, http.StatusOK, models.PasskeyRegisterChallengeResponse{
		Success: true,
		Options: options,
	})
}

// handleCLIBrowserPasskeyRegisterVerify handles passkey registration verification for browser-based CLI bootstrap.
// This is a public endpoint (no auth) for the initial bootstrap where no credentials exist yet.
// It creates a web session and binds it to the CLI session ID after successful registration.
func (c *AuthController) handleCLIBrowserPasskeyRegisterVerify(w http.ResponseWriter, r *http.Request) {
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
		CLISessionID        string               `json:"cli_session_id"`
		AttestationResponse *AttestationResponse `json:"attestation_response"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.UserID == "" {
		c.responder.Error(w, http.StatusBadRequest, "user_id required")
		return
	}

	// Enforce first-credential-only for CLI bootstrap
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
		c.responder.Error(w, http.StatusForbidden, "first-credential registration only; user already has credentials")
		return
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
		c.logger.Warn("CLI browser passkey register verify failed", "error", err, "userID", req.UserID)
		c.responder.JSON(w, http.StatusOK, models.PasskeyVerifyResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Create web session and bind it to CLI session
	webSession, err := c.webSessionSvc.CreateWebSession(req.UserID)
	if err != nil {
		c.logger.Error("Failed to create web session after browser registration", "error", err, "userID", req.UserID)
		c.responder.JSON(w, http.StatusOK, models.PasskeyVerifyResponse{
			Success: false,
			Error:   fmt.Sprintf("registration succeeded but web session creation failed: %v", err),
		})
		return
	}

	// Set web session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "g8e_session",
		Value:    webSession.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	c.responder.JSON(w, http.StatusOK, models.PasskeyVerifyResponse{
		Success:    true,
		Credential: cred,
	})
}

// handleCLIPasskeyAuthenticateChallenge handles passkey authentication challenges for CLI.
// This endpoint requires mTLS authentication via CLI certificate with X-G8E-CLI-Session-ID header.
func (c *AuthController) handleCLIPasskeyAuthenticateChallenge(w http.ResponseWriter, r *http.Request) {
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

	if req.UserID == "" {
		c.responder.Error(w, http.StatusBadRequest, "user_id required")
		return
	}

	options, err := c.passkey.GenerateAuthenticationChallenge(req.UserID)
	if err != nil {
		c.logger.Warn("CLI passkey auth challenge failed", "error", err, "userID", req.UserID)
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

// handleCLIPasskeyAuthenticateVerify handles passkey authentication verification for CLI.
// This endpoint requires mTLS authentication via CLI certificate with X-G8E-CLI-Session-ID header.
func (c *AuthController) handleCLIPasskeyAuthenticateVerify(w http.ResponseWriter, r *http.Request) {
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

	if req.UserID == "" {
		c.responder.Error(w, http.StatusBadRequest, "user_id required")
		return
	}

	responseJSON, err := json.Marshal(req.AssertionResponse)
	if err != nil {
		c.logger.Warn("Failed to marshal assertion response", "error", err, "userID", req.UserID)
		c.responder.JSON(w, http.StatusOK, models.PasskeyAuthVerifyResponse{
			Success: false,
			Error:   "failed to marshal assertion response",
		})
		return
	}

	cred, err := c.passkey.VerifyAuthentication(req.UserID, responseJSON)
	if err != nil {
		c.logger.Warn("CLI passkey auth verify failed", "error", err, "userID", req.UserID)
		c.responder.JSON(w, http.StatusOK, models.PasskeyAuthVerifyResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// CLI authentication doesn't need web session - just return success
	c.responder.JSON(w, http.StatusOK, models.PasskeyAuthVerifyResponse{
		Success:    true,
		UserID:     req.UserID,
		Credential: cred,
	})
}

func (c *AuthController) handleLocalBootstrapWithURL(w http.ResponseWriter, r *http.Request) {
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
		Name              string              `json:"name"`
		CSRPEM            string              `json:"csr_pem"`
		CLICSRPEM         string              `json:"cli_csr_pem,omitempty"`
		SystemFingerprint string              `json:"system_fingerprint"`
		LocalOSUser       *models.LocalOSUser `json:"local_os_user,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	// Check if operator CSR signing is requested for rotation.
	// CLI CSR is handled by the dedicated /api/v1/auth/cli/enroll endpoint
	// to avoid conflating CLI identity with operator identity.
	csrRequested := req.CSRPEM != ""

	// Enforce loopback gate for local bootstrap
	if csrRequested {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			c.logger.Warn("Local bootstrap CSR request rejected: not from loopback", "remote_addr", r.RemoteAddr)
			c.responder.Error(w, http.StatusForbidden, "CSR auto-issue only available over loopback")
			return
		}
	}

	// Check for existing bootstrap user (plan §4.2, §9.1 rotation carve-out)
	bootstrapUser, err := c.userSvc.FindBootstrapUser()
	if err != nil {
		c.logger.Error("Failed to check for existing bootstrap user", "error", err)
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
			c.logger.Error("Failed to check for existing users during bootstrap", "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "bootstrap check failed")
			return
		}
		if hasUsers {
			c.logger.Warn("Bootstrap attempted on non-empty system", "remote_addr", r.RemoteAddr)
			c.responder.Error(w, http.StatusForbidden, "bootstrap only available for initial setup")
			return
		}

		// Create the bootstrap user with client-provided OS user information
		// Zero-PII: Bootstrap user created with only generated ID and OS user info
		user, err = c.userSvc.CreateBootstrapUserWithOSUser(req.LocalOSUser)
		if err != nil {
			c.logger.Error("Failed to create bootstrap user", "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to create user")
			return
		}
	}

	response := models.BootstrapResponse{
		Success: true,
		User:    user,
		UserID:  user.ID,
	}

	// If CSR is requested and loopback, sign and return cert (plan §4.2)
	var operatorID, operatorSessionID, orgID string
	if csrRequested {
		// Create Operator slot for the bootstrap user
		operatorID = uuid.NewString()
		operatorSessionID = uuid.NewString()
		orgID = user.ID // Use user ID as org ID for bootstrap
		now := time.Now().UTC()

		operator := &models.OperatorDocumentGo{
			ID:                operatorID,
			UserID:            user.ID,
			OrganizationID:    orgID,
			Component:         constants.ComponentNameG8EO,
			Name:              "bootstrap-operator",
			Status:            constants.OperatorStatusActive,
			OperatorSessionID: operatorSessionID,
			OperatorType:      constants.OperatorTypeSystem,
			SystemFingerprint: req.SystemFingerprint,
			Claimed:           true,
			ClaimedAt:         &now,
			CreatedAt:         now,
			UpdatedAt:         now,
		}

		// Sign the CSR
		certPEM, chainPEM, err := c.pki.SignCSR(req.CSRPEM, constants.LeafTypeOperator, orgID, operatorID, user.ID, operatorSessionID, "")
		if err != nil {
			c.logger.Error("Failed to sign bootstrap CSR", "error", err, "user_id", user.ID)
			c.responder.Error(w, http.StatusInternalServerError, "failed to sign CSR")
			return
		}

		operator.OperatorCert = certPEM

		// Persist Operator document
		opBytes, err := json.Marshal(operator)
		if err != nil {
			c.logger.Error("Failed to marshal Operator document", "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to create operator")
			return
		}
		if err := c.db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionOperators), operatorID, opBytes); err != nil {
			c.logger.Error("Failed to persist Operator document", "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to create operator")
			return
		}

		response.OperatorCert = certPEM
		response.OperatorCertChain = chainPEM
		response.OperatorSessionID = operatorSessionID
		response.OperatorID = operatorID
	}

	// CLI certificate generation (if provided)
	var cliCertPEM, cliCertChainPEM string
	var cliCertFingerprint, cliCertSerial string

	// Always create a CLI session ID for CLI-only bootstrap (user_id binding is required)
	cliSessionID := uuid.NewString()

	if req.CLICSRPEM != "" {
		cliCertPEM, cliCertChainPEM, err = c.pki.SignCSR(req.CLICSRPEM, constants.LeafTypeCLI, "", "", user.ID, cliSessionID, "")
		if err != nil {
			c.logger.Error("Failed to sign bootstrap CLI CSR", "error", err, "user_id", user.ID)
			c.responder.Error(w, http.StatusInternalServerError, "failed to sign CLI CSR")
			return
		}

		// Calculate CLI certificate fingerprint and serial for L3 verification
		cliCertFingerprint = calculateFingerprintFromPEM(cliCertPEM)
		cliCertSerial = calculateSerialFromPEM(cliCertPEM)

		// Fetch trust bundle
		hubBundle, err := c.pki.GatewayTrustBundle()
		if err != nil {
			c.logger.Warn("Failed to fetch hub trust bundle", "error", err)
			// Non-fatal - continue without bundle
		}

		response.HubTrustBundle = string(hubBundle)
		response.CLICert = cliCertPEM
		response.CLICertChain = cliCertChainPEM
	}

	// Always persist CLI session (even without certificate for CLI-only bootstrap)
	// This ensures user_id binding exists for later CLI enrollment
	err = c.cliSessionSvc.PersistCLISession(
		cliSessionID,
		operatorSessionID, // Empty if no operator CSR
		user.ID,
		"bootstrap-cli",
		cliCertFingerprint,
		cliCertSerial,
		string(constants.HeartbeatTypeBootstrap),
	)
	if err != nil {
		c.logger.Error("Failed to persist CLI session during bootstrap", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to persist CLI session")
		return
	}

	response.CLISessionID = cliSessionID

	// Persist operator session only if operator CSR was requested
	if csrRequested {
		err = c.operatorSessionSvc.PersistOperatorSession(
			operatorSessionID,
			user.ID,
			orgID,
			operatorID,
			string(constants.HeartbeatTypeBootstrap),
		)
		if err != nil {
			c.logger.Error("Failed to persist operator session during bootstrap", "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to persist operator session")
			return
		}
		c.logger.Info("[BOOTSTRAP] System initialized with bootstrap user, operator and CLI session", "user_id", user.ID, "operator_id", operatorID, "cli_session_id_prefix", cliSessionID[:8])
	} else if req.CLICSRPEM != "" {
		c.logger.Info("[BOOTSTRAP] System initialized with bootstrap user and CLI cert (no operator)", "user_id", user.ID, "cli_session_id_prefix", cliSessionID[:8])
	} else {
		c.logger.Info("[BOOTSTRAP] System initialized with bootstrap user and CLI session (no CSR)", "user_id", user.ID, "cli_session_id_prefix", cliSessionID[:8])
	}

	c.responder.JSON(w, http.StatusCreated, response)
}

// handleCLIEnrollment issues a CLI certificate for an already-bootstrapped system.
// This endpoint is strictly for CLI credential recovery when local credentials are
// missing; it does NOT create or rotate operator state. Loopback-only for defense.
func (c *AuthController) handleCLIEnrollment(w http.ResponseWriter, r *http.Request) {
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
		CLICSRPEM         string              `json:"cli_csr_pem"`
		SystemFingerprint string              `json:"system_fingerprint"`
		LocalOSUser       *models.LocalOSUser `json:"local_os_user,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	if req.CLICSRPEM == "" {
		c.responder.Error(w, http.StatusBadRequest, "cli_csr_pem is required")
		return
	}

	// Enforce loopback gate for local CLI enrollment
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		c.logger.Warn("CLI enrollment request rejected: not from loopback", "remote_addr", r.RemoteAddr)
		c.responder.Error(w, http.StatusForbidden, "CLI enrollment only available over loopback")
		return
	}

	// This endpoint only works when the system is already bootstrapped
	bootstrapUser, err := c.userSvc.FindBootstrapUser()
	if err != nil {
		c.logger.Error("Failed to check for existing bootstrap user", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "bootstrap check failed")
		return
	}
	if bootstrapUser == nil {
		c.logger.Warn("CLI enrollment attempted on unbootstrapped system", "remote_addr", r.RemoteAddr)
		c.responder.Error(w, http.StatusForbidden, "CLI enrollment only available after bootstrap")
		return
	}
	if !bootstrapUser.IsActive() {
		c.logger.Warn("Bootstrap user is disabled, refusing CLI enrollment", "user_id", bootstrapUser.ID)
		c.responder.Error(w, http.StatusConflict, constants.ErrBootstrapUserDisabledEnroll.Error())
		return
	}

	// Update bootstrap user with client-provided OS user information if available
	if req.LocalOSUser != nil {
		bootstrapUser.LocalOSUser = req.LocalOSUser
		userData, err := json.Marshal(bootstrapUser)
		if err != nil {
			c.logger.Error("Failed to marshal bootstrap user for OS user update", "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to update user")
			return
		}
		if err := c.db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionUsers), bootstrapUser.ID, userData); err != nil {
			c.logger.Error("Failed to update bootstrap user with OS user info", "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to update user")
			return
		}
		c.logger.Info("[CLI_ENROLLMENT] Updated bootstrap user with OS user information", "user_id", bootstrapUser.ID)
	}

	// Create operator slot associated with this CLI enrollment
	operatorID := uuid.NewString()
	operatorSessionID := uuid.NewString()
	cliSessionID := uuid.NewString()
	orgID := bootstrapUser.ID
	now := time.Now().UTC()

	operator := &models.OperatorDocumentGo{
		ID:                operatorID,
		UserID:            bootstrapUser.ID,
		OrganizationID:    orgID,
		Component:         constants.ComponentNameG8EO,
		Name:              "cli-" + bootstrapUser.ID[:8],
		Status:            constants.OperatorStatusActive,
		OperatorSessionID: operatorSessionID,
		OperatorType:      constants.OperatorTypeSystem,
		SystemFingerprint: req.SystemFingerprint,
		Claimed:           true,
		ClaimedAt:         &now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	opBytes, err := json.Marshal(operator)
	if err != nil {
		c.logger.Error("Failed to marshal operator document during CLI enrollment", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to create operator record")
		return
	}
	if err := c.db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionOperators), operatorID, opBytes); err != nil {
		c.logger.Error("Failed to persist operator document during CLI enrollment", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to persist operator record")
		return
	}

	// Sign the CLI CSR
	cliCertPEM, cliCertChainPEM, err := c.pki.SignCSR(req.CLICSRPEM, constants.LeafTypeCLI, "", "", bootstrapUser.ID, cliSessionID, "")
	if err != nil {
		c.logger.Error("Failed to sign CLI enrollment CSR", "error", err, "user_id", bootstrapUser.ID)
		c.responder.Error(w, http.StatusInternalServerError, "failed to sign CLI CSR")
		return
	}

	// Calculate CLI certificate fingerprint and serial for L3 verification
	cliCertFingerprint := calculateFingerprintFromPEM(cliCertPEM)
	cliCertSerial := calculateSerialFromPEM(cliCertPEM)

	// Persist CLI session linked to operator session
	err = c.cliSessionSvc.PersistCLISession(
		cliSessionID,
		operatorSessionID,
		bootstrapUser.ID,
		req.SystemFingerprint,
		cliCertFingerprint,
		cliCertSerial,
		string(constants.HeartbeatTypeBootstrap),
	)
	if err != nil {
		c.logger.Error("Failed to persist CLI session during enrollment", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to persist session")
		return
	}

	// Persist operator session
	err = c.operatorSessionSvc.PersistOperatorSession(
		operatorSessionID,
		bootstrapUser.ID,
		orgID,
		operatorID,
		string(constants.HeartbeatTypeBootstrap),
	)
	if err != nil {
		c.logger.Error("Failed to persist operator session during CLI enrollment", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to persist operator session")
		return
	}

	// Fetch trust bundle
	hubBundle, err := c.pki.GatewayTrustBundle()
	if err != nil {
		c.logger.Warn("Failed to fetch hub trust bundle", "error", err)
		// Non-fatal - continue without bundle
	}

	c.logger.Info("[CLI_ENROLLMENT] CLI enrolled successfully", "user_id", bootstrapUser.ID, "cli_session_id_prefix", cliSessionID[:8])
	c.responder.JSON(w, http.StatusCreated, models.CLIEnrollmentResponse{
		Success:           true,
		CLISessionID:      cliSessionID,
		CLICert:           cliCertPEM,
		CLICertChain:      cliCertChainPEM,
		HubTrustBundle:    string(hubBundle),
		UserID:            bootstrapUser.ID,
		OperatorSessionID: operatorSessionID,
		OperatorID:        operatorID,
	})
}

func (c *AuthController) handleDeviceEnrollment(w http.ResponseWriter, r *http.Request) {
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
		CSRPEM            string `json:"csr_pem"`
		CLICSRPEM         string `json:"cli_csr_pem,omitempty"`
		SystemFingerprint string `json:"system_fingerprint"`
		Hostname          string `json:"hostname"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	// Validate required fields for device enrollment
	if req.CSRPEM == "" {
		c.responder.Error(w, http.StatusBadRequest, "csr_pem is required")
		return
	}
	if req.SystemFingerprint == "" {
		c.responder.Error(w, http.StatusBadRequest, "system_fingerprint is required")
		return
	}
	if req.Hostname == "" {
		c.responder.Error(w, http.StatusBadRequest, "hostname is required")
		return
	}

	// Check for existing bootstrap user (plan §4.2, §9.1 rotation carve-out)
	bootstrapUser, err := c.userSvc.FindBootstrapUser()
	if err != nil {
		c.logger.Error("Failed to check for existing bootstrap user", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "bootstrap check failed")
		return
	}

	var user *models.User
	if bootstrapUser != nil {
		// Bootstrap user exists - check if rotation is allowed
		if !bootstrapUser.IsActive() {
			c.logger.Warn("Bootstrap user is disabled, refusing device enrollment", "user_id", bootstrapUser.ID)
			c.responder.Error(w, http.StatusConflict, constants.ErrBootstrapUserDisabledEnroll.Error())
			return
		}
		user = bootstrapUser
		c.logger.Info("[DEVICE_ENROLLMENT] Using existing bootstrap user", "user_id", user.ID)
	} else {
		// No bootstrap user exists - create one
		hasUsers, err := c.userSvc.HasAnyUsers()
		if err != nil {
			c.logger.Error("Failed to check for existing users during device enrollment", "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "bootstrap check failed")
			return
		}
		if hasUsers {
			c.logger.Warn("Device enrollment attempted on non-empty system", "remote_addr", r.RemoteAddr)
			c.responder.Error(w, http.StatusForbidden, "device enrollment only available for initial setup")
			return
		}

		user, err = c.userSvc.CreateBootstrapUser()
		if err != nil {
			c.logger.Error("Failed to create bootstrap user for device enrollment", "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to create user")
			return
		}
	}

	// Create Operator slot for the device
	operatorID := uuid.NewString()
	operatorSessionID := uuid.NewString()
	cliSessionID := uuid.NewString()
	orgID := user.ID
	now := time.Now().UTC()

	operator := &models.OperatorDocumentGo{
		ID:                operatorID,
		UserID:            user.ID,
		OrganizationID:    orgID,
		Component:         constants.ComponentNameG8EO,
		Name:              req.Hostname,
		Status:            constants.OperatorStatusActive,
		OperatorSessionID: operatorSessionID,
		OperatorType:      constants.OperatorTypeSystem,
		SystemFingerprint: req.SystemFingerprint,
		Claimed:           true,
		ClaimedAt:         &now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	// Sign the CSR
	certPEM, chainPEM, err := c.pki.SignCSR(req.CSRPEM, constants.LeafTypeOperator, orgID, operatorID, user.ID, operatorSessionID, "")
	if err != nil {
		c.logger.Error("Failed to sign device enrollment CSR", "error", err, "user_id", user.ID)
		c.responder.Error(w, http.StatusInternalServerError, "failed to sign CSR")
		return
	}

	operator.OperatorCert = certPEM

	// CLI certificate generation (mandatory)
	if req.CLICSRPEM == "" {
		c.logger.Error("Device enrollment request missing mandatory CLI CSR", "user_id", user.ID)
		c.responder.Error(w, http.StatusBadRequest, "cli_csr_pem is mandatory")
		return
	}

	cliCertPEM, cliCertChainPEM, err := c.pki.SignCSR(req.CLICSRPEM, constants.LeafTypeCLI, "", "", user.ID, cliSessionID, "")
	if err != nil {
		c.logger.Error("Failed to sign device enrollment CLI CSR", "error", err, "user_id", user.ID)
		c.responder.Error(w, http.StatusInternalServerError, "failed to sign CLI CSR")
		return
	}

	// Calculate CLI certificate fingerprint and serial for L3 verification
	cliCertFingerprint := calculateFingerprintFromPEM(cliCertPEM)
	cliCertSerial := calculateSerialFromPEM(cliCertPEM)

	// Persist Operator document
	opBytes, err := json.Marshal(operator)
	if err != nil {
		c.logger.Error("Failed to marshal Operator document", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to create operator")
		return
	}
	if err := c.db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionOperators), operatorID, opBytes); err != nil {
		c.logger.Error("Failed to persist Operator document", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to create operator")
		return
	}

	// Fetch trust bundle
	hubBundle, err := c.pki.GatewayTrustBundle()
	if err != nil {
		c.logger.Warn("Failed to fetch hub trust bundle", "error", err)
		// Non-fatal - continue without bundle
	}

	// Persist CLI session
	err = c.cliSessionSvc.PersistCLISession(
		cliSessionID,
		operatorSessionID,
		user.ID,
		req.SystemFingerprint,
		cliCertFingerprint,
		cliCertSerial,
		string(constants.HeartbeatTypeBootstrap),
	)
	if err != nil {
		c.logger.Error("Failed to persist CLI session during device enrollment", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to persist CLI session")
		return
	}
	// Persist operator session
	err = c.operatorSessionSvc.PersistOperatorSession(
		operatorSessionID,
		user.ID,
		orgID,
		operatorID,
		string(constants.HeartbeatTypeBootstrap),
	)
	if err != nil {
		c.logger.Error("Failed to persist operator session during device enrollment", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to persist operator session")
		return
	}

	response := models.DeviceEnrollmentResponse{
		Success:           true,
		User:              user,
		OperatorCert:      certPEM,
		OperatorCertChain: chainPEM,
		HubTrustBundle:    string(hubBundle),
		OperatorSessionID: operatorSessionID,
		OperatorID:        operatorID,
		CLISessionID:      cliSessionID,
		CLICert:           cliCertPEM,
		CLICertChain:      cliCertChainPEM,
		UserID:            user.ID,
	}

	// Include Actuator public key so the operator can populate its trusted_signers directory.
	if c.actuatorKeyReader != nil {
		if keyID, publicKey, err := c.actuatorKeyReader.ReadActuatorPublicKey(); err == nil {
			response.ActuatorKeyID = keyID
			response.ActuatorPubKey = publicKey
		}
	}

	c.logger.Info("[DEVICE_ENROLLMENT] Device enrolled successfully", "user_id", user.ID, "operator_id", operatorID, "hostname", req.Hostname)
	c.responder.JSON(w, http.StatusCreated, response)
}

func (c *AuthController) handleBootstrapStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	hasUsers, err := c.userSvc.HasAnyUsers()
	if err != nil {
		c.logger.Error("Failed to check for existing users", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "status check failed")
		return
	}

	c.responder.JSON(w, http.StatusOK, models.BootstrapStatusResponse{
		Bootstrapped: hasUsers,
	})
}
