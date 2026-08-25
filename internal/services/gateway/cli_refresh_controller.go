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
	"log/slog"
	"net/http"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/uuid"
)

// CLIRefreshControllerDeps groups all dependencies for CLIRefreshController.
type CLIRefreshControllerDeps struct {
	Cfg                *config.Config
	Logger             *slog.Logger
	CLISessionSvc      *CLISessionService
	OperatorSessionSvc *OperatorSessionService
	UserSvc            *UserService
	Responder          *response.Writer
}

// CLIRefreshController handles the mTLS-protected CLI session refresh
// endpoint. It issues a new CLI session bound to the same user as the
// caller's authenticated cert, deactivating the old session if it still
// exists. The cert is NOT rotated — the cert is the proof of identity, and
// the cert's URI SAN binds the new session to the same user.
//
// Auth classification (enforced by the unified auth middleware via
// NewRouteAuthRegistry):
//   - refresh (POST /api/v1/auth/cli/refresh): RouteAuthMTLS
//     (requires a verified CLI client certificate whose URI SAN matches a
//     CLI session; the caller can only refresh their own session)
//
// This is the recovery path for an expired CLI session with a still-valid
// cert. An expired cert cannot authenticate via mTLS, so it can never
// reach this endpoint — the caller must use the recovery flow instead.
type CLIRefreshController struct {
	cfg                *config.Config
	logger             *slog.Logger
	cliSessionSvc      *CLISessionService
	operatorSessionSvc *OperatorSessionService
	userSvc            *UserService
	responder          *response.Writer
}

func newCLIRefreshController(deps CLIRefreshControllerDeps) *CLIRefreshController {
	return &CLIRefreshController{
		cfg:                deps.Cfg,
		logger:             deps.Logger,
		cliSessionSvc:      deps.CLISessionSvc,
		operatorSessionSvc: deps.OperatorSessionSvc,
		userSvc:            deps.UserSvc,
		responder:          deps.Responder,
	}
}

// handleRefresh issues a new CLI session for the caller's authenticated
// identity. The user ID and CLI session ID are read from the request
// context (stamped by the auth middleware from the verified certificate
// URI SAN). The request body is empty — the cert is the proof of identity.
//
// Order of operations:
//  1. Extract user ID and old CLI session ID from the mTLS context.
//  2. Verify the user is still active.
//  3. Pre-generate the new CLI session ID.
//  4. RefreshCLISession: deactivate the old session (if it exists and is
//     active), persist the new session bound to the same user and cert.
//  5. Return the new session ID and user ID.
//
// POST /api/v1/auth/cli/refresh  (RouteAuthMTLS)
func (c *CLIRefreshController) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Identity comes from the mTLS context, never from the body.
	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.logger.Warn("CLI refresh: missing authenticated user context")
		c.responder.Error(w, http.StatusUnauthorized, "mTLS authentication required")
		return
	}
	oldCLISessionID, _ := r.Context().Value(constants.ContextKeyCLISessionID).(string)

	// Read and discard the body (the request body is empty, but we must
	// consume it for connection reuse). A non-empty body is not an error —
	// the body is simply ignored.
	if _, err := readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req models.CLIRefreshRequest
	_ = req // request body is empty; decode is a no-op but keeps the contract explicit
	_ = json.Unmarshal([]byte{}, &req)

	// Verify the user is still active. The auth middleware does this on
	// every request, but a race between middleware and refresh could leave
	// a disabled user with a still-valid cert.
	user, err := c.userSvc.GetByID(userID)
	if err != nil {
		c.logger.Error("CLI refresh: failed to look up user", "error", err, "user_id", userID)
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify user")
		return
	}
	if user == nil || !user.IsActive() {
		c.logger.Warn("CLI refresh: user is not active", "user_id", userID)
		c.responder.Error(w, http.StatusForbidden, "user is not active")
		return
	}

	// Load the old session if it exists, to inherit the operator binding
	// and cert fingerprint. If the old session is missing (e.g., after a
	// gateway volume reset), we still issue a new session — the cert is
	// the proof of identity, not the old session's state.
	var oldSession *models.CLISession
	if oldCLISessionID != "" {
		oldSession, err = c.cliSessionSvc.loadCLISession(oldCLISessionID)
		if err != nil && !errors.Is(err, constants.ErrCLISessionNotFound) {
			c.logger.Error("CLI refresh: failed to load old session",
				"error", err,
				"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID, 8),
			)
			c.responder.Error(w, http.StatusInternalServerError, "failed to load old session")
			return
		}
	}

	// Derive the operator binding and cert fingerprint for the new session.
	// When the old session exists, inherit its binding. When it does not
	// (e.g., after a gateway volume reset that wiped CLI sessions but left
	// operator sessions intact), look up the user's active operator session
	// to inherit its binding. If no active operator session exists, return
	// a clear actionable error — the caller must re-enroll to establish a
	// fresh operator binding.
	var operatorSessionID, systemFingerprint, certFingerprint, certSerial, loginMethod string
	if oldSession != nil {
		operatorSessionID = oldSession.OperatorSessionID
		systemFingerprint = oldSession.SystemFingerprint
		certFingerprint = oldSession.CertFingerprint
		certSerial = oldSession.CertSerial
		loginMethod = oldSession.LoginMethod
	} else if c.operatorSessionSvc != nil {
		opSession, opErr := c.operatorSessionSvc.GetActiveSessionForUser(userID)
		if opErr != nil {
			c.logger.Error("CLI refresh: failed to look up active operator session",
				"error", opErr,
				"user_id", userID,
			)
			c.responder.Error(w, http.StatusInternalServerError, "failed to look up operator session")
			return
		}
		if opSession != nil {
			operatorSessionID = opSession.ID
			loginMethod = opSession.LoginMethod
		}
	}
	if operatorSessionID == "" {
		c.logger.Warn("CLI refresh: no operator session binding available",
			"user_id", userID,
			"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID, 8),
		)
		c.responder.Error(w, http.StatusConflict, "no active operator session found for user; re-enroll with 'auth enroll user' to establish a fresh operator binding")
		return
	}

	newCLISessionID := uuid.NewString()
	_, err = c.cliSessionSvc.RefreshCLISession(
		oldCLISessionID,
		newCLISessionID,
		CLISessionFields{
			OperatorSessionID: operatorSessionID,
			UserID:            userID,
			SystemFingerprint: systemFingerprint,
			CertFingerprint:   certFingerprint,
			CertSerial:        certSerial,
			LoginMethod:       loginMethod,
		},
	)
	if err != nil {
		c.logger.Warn("CLI refresh: RefreshCLISession failed",
			"error", err,
			"user_id", userID,
			"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID, 8),
			"new_cli_session_id_prefix", safeTruncateID(newCLISessionID, 8),
		)
		c.writeRefreshError(w, err)
		return
	}

	c.logger.Info("CLI session refreshed via controller",
		"user_id", userID,
		"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID, 8),
		"new_cli_session_id_prefix", safeTruncateID(newCLISessionID, 8),
	)

	c.responder.JSON(w, http.StatusCreated, models.CLIRefreshResponse{
		Success:      true,
		CLISessionID: newCLISessionID,
		UserID:       userID,
	})
}

// writeRefreshError maps a typed refresh/session error to the appropriate
// HTTP status code and writes it to the response. Unknown errors default
// to 500 Internal Server Error. Mirrors writeRotationError.
func (c *CLIRefreshController) writeRefreshError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, constants.ErrCLISessionNotFound):
		c.responder.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, constants.ErrCLISessionAlreadyDeactivated):
		c.responder.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, constants.ErrCLIRefreshFailed):
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
	case errors.Is(err, constants.ErrCLIRefreshCertExpired):
		c.responder.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, constants.ErrMTLSIdentityMismatch):
		c.responder.Error(w, http.StatusForbidden, err.Error())
	default:
		c.logger.Error("CLI refresh: unhandled error", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, fmt.Sprintf("internal error: %v", err))
	}
}
