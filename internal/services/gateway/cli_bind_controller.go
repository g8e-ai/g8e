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
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/uuid"
)

// handleBind binds the authenticated CLI session to the requested operator
// session(s) in a single call. The request may carry one operator session or
// many; every target is validated (active, owned by the caller) before any
// binding changes, so a bad target rejects the whole request. The first
// target becomes the primary binding the auth middleware stamps on requests.
// When the CLI session is already bound to exactly that list, the current
// binding is returned without issuing a replacement session.
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
	targetSessionIDs := normalizeOperatorSessionIDs(req)
	if len(targetSessionIDs) == 0 {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrGatewayOperatorSessionIDRequired.Error())
		return
	}
	if len(targetSessionIDs) > constants.CLIBindMaxOperators {
		c.responder.Error(w, http.StatusBadRequest, fmt.Sprintf("too many operator sessions: %d (max %d)", len(targetSessionIDs), constants.CLIBindMaxOperators))
		return
	}

	user, err := c.userSvc.GetByID(r.Context(),userID)
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
	bound := make([]models.CLIBoundOperator, 0, len(targetSessionIDs))
	for _, sessionID := range targetSessionIDs {
		op, err := c.auth.ValidateOperatorSession(sessionID)
		if err != nil {
			var authErr *AuthError
			if errors.As(err, &authErr) {
				c.responder.Error(w, authErr.Status, fmt.Sprintf("operator session %s: %s", safeTruncateID(sessionID), authErr.Message))
				return
			}
			c.logger.Error("CLI bind: validate operator session", "error", err, "operator_session_id_prefix", safeTruncateID(sessionID))
			c.responder.Error(w, http.StatusInternalServerError, "failed to verify operator session")
			return
		}
		if op.UserId != userID {
			c.logger.Warn("CLI bind: operator session does not belong to authenticated user",
				"user_id", userID,
				"operator_user_id", op.UserId,
				"operator_session_id_prefix", safeTruncateID(sessionID),
			)
			c.responder.Error(w, http.StatusForbidden, "operator session does not belong to the authenticated user")
			return
		}
		bound = append(bound, models.CLIBoundOperator{OperatorSessionID: sessionID, OperatorID: op.Id})
	}
	primary := bound[0]

	var oldSession *models.CLISession
	if oldCLISessionID != "" {
		oldSession, err = c.cliSessionSvc.loadCLISession(r.Context(),oldCLISessionID)
		if err != nil && !errors.Is(err, constants.ErrCLISessionNotFound) {
			c.logger.Error("CLI bind: failed to load old session",
				"error", err,
				"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID),
			)
			c.responder.Error(w, http.StatusInternalServerError, "failed to load CLI session")
			return
		}
	}
	if oldSession != nil && oldSession.IsActive && sameOperatorSessionIDs(boundSessionIDsOf(oldSession), targetSessionIDs) {
		c.responder.JSON(w, http.StatusOK, models.CLIBindResponse{
			Success:           true,
			CLISessionID:      oldCLISessionID,
			UserID:            userID,
			OperatorSessionID: primary.OperatorSessionID,
			OperatorID:        primary.OperatorID,
			AlreadyBound:      true,
			Bound:             bound,
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

	newCLISessionID, err := uuid.NewString()
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to generate session ID")
		return
	}
	_, err = c.cliSessionSvc.RefreshCLISession(r.Context(),
		oldCLISessionID,
		newCLISessionID,
		CLISessionFields{
			OperatorSessionID:       primary.OperatorSessionID,
			BoundOperatorSessionIDs: targetSessionIDs,
			UserID:                  userID,
			SystemFingerprint:       systemFingerprint,
			CertFingerprint:         certFingerprint,
			CertSerial:              certSerial,
			LoginMethod:             loginMethod,
		},
	)
	if err != nil {
		c.logger.Warn("CLI bind: RefreshCLISession failed",
			"error", err,
			"user_id", userID,
			"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID),
			"new_cli_session_id_prefix", safeTruncateID(newCLISessionID),
		)
		c.writeRefreshError(w, err)
		return
	}

	c.logger.Info("CLI session bound to operator session(s) via controller",
		"user_id", userID,
		"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID),
		"new_cli_session_id_prefix", safeTruncateID(newCLISessionID),
		"primary_operator_session_id_prefix", safeTruncateID(primary.OperatorSessionID),
		"operator_count", len(bound),
	)

	c.responder.JSON(w, http.StatusCreated, models.CLIBindResponse{
		Success:           true,
		CLISessionID:      newCLISessionID,
		UserID:            userID,
		OperatorSessionID: primary.OperatorSessionID,
		OperatorID:        primary.OperatorID,
		Bound:             bound,
	})
}

// normalizeOperatorSessionIDs returns the de-duplicated, trimmed target list of
// a bind request in request order (operator_session_id first, then
// operator_session_ids).
func normalizeOperatorSessionIDs(req models.CLIBindRequest) []string {
	candidates := make([]string, 0, len(req.OperatorSessionIDs)+1)
	candidates = append(candidates, req.OperatorSessionID)
	candidates = append(candidates, req.OperatorSessionIDs...)
	seen := make(map[string]struct{}, len(candidates))
	ids := make([]string, 0, len(candidates))
	for _, id := range candidates {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

// boundSessionIDsOf returns a CLI session's bound operator sessions, primary
// first. Sessions persisted before multi-bind carry only the primary binding.
func boundSessionIDsOf(session *models.CLISession) []string {
	if len(session.BoundOperatorSessionIDs) > 0 {
		return session.BoundOperatorSessionIDs
	}
	if session.OperatorSessionID != "" {
		return []string{session.OperatorSessionID}
	}
	return nil
}

// cliSessionBindsOperator reports whether operatorSessionID is one of the
// operator sessions bound to the CLI session, primary or not.
func cliSessionBindsOperator(session *models.CLISession, operatorSessionID string) bool {
	return operatorSessionID != "" && slices.Contains(boundSessionIDsOf(session), operatorSessionID)
}

func sameOperatorSessionIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

	user, err := c.userSvc.GetByID(r.Context(),userID)
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
		oldSession, err = c.cliSessionSvc.loadCLISession(r.Context(),oldCLISessionID)
		if err != nil && !errors.Is(err, constants.ErrCLISessionNotFound) {
			c.logger.Error("CLI unbind: failed to load old session",
				"error", err,
				"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID),
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

	newCLISessionID, err := uuid.NewString()
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, "failed to generate session ID")
		return
	}
	_, err = c.cliSessionSvc.UnbindCLISession(r.Context(),
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
			"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID),
			"new_cli_session_id_prefix", safeTruncateID(newCLISessionID),
		)
		c.writeRefreshError(w, err)
		return
	}

	c.logger.Info("CLI session unbound from operator via controller",
		"user_id", userID,
		"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID),
		"new_cli_session_id_prefix", safeTruncateID(newCLISessionID),
	)

	c.responder.JSON(w, http.StatusCreated, models.CLIUnbindResponse{
		Success:      true,
		CLISessionID: newCLISessionID,
		UserID:       userID,
	})
}
