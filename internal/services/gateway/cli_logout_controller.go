// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// handleLogout terminates the authenticated user's sessions: every web and CLI
// session (scope "all", the default), or only one kind. It unbinds the
// operators bound to those sessions and revokes the CLI certificates behind
// the terminated CLI sessions. Operator sessions themselves are not touched.
//
// The user and CLI session come from the mTLS context, never the body. Like
// refresh, this endpoint is admitted on the certificate alone (see
// isCLISessionCertIdentityPath), so a user whose CLI session already expired
// can still log out.
//
// POST /api/v1/auth/cli/logout  (RouteAuthMTLS)
func (c *CLIRefreshController) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.logger.Warn("CLI logout: missing authenticated user context")
		c.responder.Error(w, http.StatusUnauthorized, "mTLS authentication required")
		return
	}
	cliSessionID, _ := r.Context().Value(constants.ContextKeyCLISessionID).(string)

	body, err := readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var req models.CLILogoutRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	if req.Scope == "" {
		req.Scope = constants.LogoutScopeAll
	}
	if !req.Scope.Valid() {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrLogoutScopeInvalid.Error())
		return
	}

	if c.logoutSvc == nil {
		c.logger.Error("CLI logout: logout service unavailable")
		c.responder.Error(w, http.StatusInternalServerError, "failed to log out")
		return
	}

	var presentedCertSerial string
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		presentedCertSerial = r.TLS.PeerCertificates[0].SerialNumber.String()
	}

	result, err := c.logoutSvc.Logout(userID, cliSessionID, presentedCertSerial, req.Scope)
	if err != nil {
		if errors.Is(err, constants.ErrLogoutScopeInvalid) {
			c.responder.Error(w, http.StatusBadRequest, constants.ErrLogoutScopeInvalid.Error())
			return
		}
		c.logger.Error("CLI logout failed",
			"error", err,
			"user_id", userID,
			"scope", string(req.Scope),
			"cli_session_id_prefix", safeTruncateID(cliSessionID),
		)
		c.responder.Error(w, http.StatusInternalServerError, "logout incomplete; retry 'g8e logout'")
		return
	}

	c.responder.JSON(w, http.StatusOK, models.CLILogoutResponse{
		Success:                   true,
		UserID:                    userID,
		Scope:                     req.Scope,
		WebSessionsTerminated:     result.WebSessionsTerminated,
		CLISessionsTerminated:     result.CLISessionsTerminated,
		CLICertificatesRevoked:    result.CertificatesRevoked,
		UnboundOperatorSessionIDs: result.UnboundOperatorSessionIDs,
	})
}
