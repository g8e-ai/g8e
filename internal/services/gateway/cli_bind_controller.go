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
	"github.com/g8e-ai/g8e/v2/internal/uuid"
)

// handleBind pins the authenticated CLI session to the requested operator
// session. When the CLI session is already bound to that operator session,
// the current binding is returned without issuing a replacement session.
//
// POST /api/v1/auth/cli/bind  (RouteAuthMTLS)
func (c *CLIRefreshController) handleBind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.logger.Warn("CLI bind: missing authenticated user context")
		c.responder.Error(w, http.StatusUnauthorized, "mTLS authentication required")
		return
	}
	oldCLISessionID, _ := r.Context().Value(constants.ContextKeyCLISessionID).(string)

	body, err := readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}
	var req models.CLIBindRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	if req.OperatorSessionID == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrGatewayOperatorSessionIDRequired.Error())
		return
	}

	user, err := c.userSvc.GetByID(userID)
	if err != nil {
		c.logger.Error("CLI bind: failed to look up user", "error", err, "user_id", userID)
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify user")
		return
	}
	if user == nil || !user.IsActive() {
		c.logger.Warn("CLI bind: user is not active", "user_id", userID)
		c.responder.Error(w, http.StatusForbidden, "user is not active")
		return
	}

	if c.auth == nil {
		c.logger.Error("CLI bind: auth service unavailable")
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify operator session")
		return
	}
	op, err := c.auth.ValidateOperatorSession(req.OperatorSessionID)
	if err != nil {
		var authErr *AuthError
		if errors.As(err, &authErr) {
			c.responder.Error(w, authErr.Status, authErr.Message)
			return
		}
		c.logger.Error("CLI bind: validate operator session", "error", err, "operator_session_id_prefix", safeTruncateID(req.OperatorSessionID, 8))
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify operator session")
		return
	}
	if op.UserID != userID {
		c.logger.Warn("CLI bind: operator session does not belong to authenticated user",
			"user_id", userID,
			"operator_user_id", op.UserID,
			"operator_session_id_prefix", safeTruncateID(req.OperatorSessionID, 8),
		)
		c.responder.Error(w, http.StatusForbidden, "operator session does not belong to the authenticated user")
		return
	}

	var oldSession *models.CLISession
	if oldCLISessionID != "" {
		oldSession, err = c.cliSessionSvc.loadCLISession(oldCLISessionID)
		if err != nil && !errors.Is(err, constants.ErrCLISessionNotFound) {
			c.logger.Error("CLI bind: failed to load old session",
				"error", err,
				"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID, 8),
			)
			c.responder.Error(w, http.StatusInternalServerError, "failed to load CLI session")
			return
		}
	}
	if oldSession != nil && oldSession.IsActive && oldSession.OperatorSessionID == req.OperatorSessionID {
		c.responder.JSON(w, http.StatusOK, models.CLIBindResponse{
			Success:           true,
			CLISessionID:      oldCLISessionID,
			UserID:            userID,
			OperatorSessionID: req.OperatorSessionID,
			OperatorID:        op.ID,
			AlreadyBound:      true,
		})
		return
	}

	var systemFingerprint, certFingerprint, certSerial, loginMethod string
	if oldSession != nil {
		systemFingerprint = oldSession.SystemFingerprint
		certFingerprint = oldSession.CertFingerprint
		certSerial = oldSession.CertSerial
		loginMethod = oldSession.LoginMethod
	}

	newCLISessionID := uuid.NewString()
	_, err = c.cliSessionSvc.RefreshCLISession(
		oldCLISessionID,
		newCLISessionID,
		CLISessionFields{
			OperatorSessionID: req.OperatorSessionID,
			UserID:            userID,
			SystemFingerprint: systemFingerprint,
			CertFingerprint:   certFingerprint,
			CertSerial:        certSerial,
			LoginMethod:       loginMethod,
		},
	)
	if err != nil {
		c.logger.Warn("CLI bind: RefreshCLISession failed",
			"error", err,
			"user_id", userID,
			"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID, 8),
			"new_cli_session_id_prefix", safeTruncateID(newCLISessionID, 8),
		)
		c.writeRefreshError(w, err)
		return
	}

	c.logger.Info("CLI session bound to operator session via controller",
		"user_id", userID,
		"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID, 8),
		"new_cli_session_id_prefix", safeTruncateID(newCLISessionID, 8),
		"operator_session_id_prefix", safeTruncateID(req.OperatorSessionID, 8),
	)

	c.responder.JSON(w, http.StatusCreated, models.CLIBindResponse{
		Success:           true,
		CLISessionID:      newCLISessionID,
		UserID:            userID,
		OperatorSessionID: req.OperatorSessionID,
		OperatorID:        op.ID,
	})
}

// handleUnbind clears the authenticated CLI session's operator binding.
// When the session is already unbound, the current session is returned
// without issuing a replacement.
//
// POST /api/v1/auth/cli/unbind  (RouteAuthMTLS)
func (c *CLIRefreshController) handleUnbind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.logger.Warn("CLI unbind: missing authenticated user context")
		c.responder.Error(w, http.StatusUnauthorized, "mTLS authentication required")
		return
	}
	oldCLISessionID, _ := r.Context().Value(constants.ContextKeyCLISessionID).(string)

	user, err := c.userSvc.GetByID(userID)
	if err != nil {
		c.logger.Error("CLI unbind: failed to look up user", "error", err, "user_id", userID)
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify user")
		return
	}
	if user == nil || !user.IsActive() {
		c.logger.Warn("CLI unbind: user is not active", "user_id", userID)
		c.responder.Error(w, http.StatusForbidden, "user is not active")
		return
	}

	var oldSession *models.CLISession
	if oldCLISessionID != "" {
		oldSession, err = c.cliSessionSvc.loadCLISession(oldCLISessionID)
		if err != nil && !errors.Is(err, constants.ErrCLISessionNotFound) {
			c.logger.Error("CLI unbind: failed to load old session",
				"error", err,
				"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID, 8),
			)
			c.responder.Error(w, http.StatusInternalServerError, "failed to load CLI session")
			return
		}
	}
	if oldSession != nil && oldSession.IsActive && oldSession.OperatorSessionID == "" {
		c.responder.JSON(w, http.StatusOK, models.CLIUnbindResponse{
			Success:        true,
			CLISessionID:   oldCLISessionID,
			UserID:         userID,
			AlreadyUnbound: true,
		})
		return
	}

	var systemFingerprint, certFingerprint, certSerial, loginMethod string
	if oldSession != nil {
		systemFingerprint = oldSession.SystemFingerprint
		certFingerprint = oldSession.CertFingerprint
		certSerial = oldSession.CertSerial
		loginMethod = oldSession.LoginMethod
	}

	newCLISessionID := uuid.NewString()
	_, err = c.cliSessionSvc.UnbindCLISession(
		oldCLISessionID,
		newCLISessionID,
		CLISessionFields{
			UserID:            userID,
			SystemFingerprint: systemFingerprint,
			CertFingerprint:   certFingerprint,
			CertSerial:        certSerial,
			LoginMethod:       loginMethod,
		},
	)
	if err != nil {
		c.logger.Warn("CLI unbind: UnbindCLISession failed",
			"error", err,
			"user_id", userID,
			"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID, 8),
			"new_cli_session_id_prefix", safeTruncateID(newCLISessionID, 8),
		)
		c.writeRefreshError(w, err)
		return
	}

	c.logger.Info("CLI session unbound from operator via controller",
		"user_id", userID,
		"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID, 8),
		"new_cli_session_id_prefix", safeTruncateID(newCLISessionID, 8),
	)

	c.responder.JSON(w, http.StatusCreated, models.CLIUnbindResponse{
		Success:      true,
		CLISessionID: newCLISessionID,
		UserID:       userID,
	})
}
