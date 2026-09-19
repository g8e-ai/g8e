// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"log/slog"
	"net/http"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
)

// CLISessionControllerDeps groups all dependencies for CLISessionController.
type CLISessionControllerDeps struct {
	Logger    *slog.Logger
	Responder *response.Writer
}

// CLISessionController handles the mTLS-protected CLI session info
// endpoint. It reports the authenticated session's persisted identity
// binding verbatim so the CLI can resync its local credentials against
// the authoritative server-side state.
//
// Auth classification (enforced by the unified auth middleware via
// NewRouteAuthRegistry):
//   - session info (GET /api/v1/auth/cli/session): RouteAuthMTLS
//     (requires a verified CLI client certificate bound to an active CLI
//     session; expired or missing sessions fail closed — the caller must
//     refresh or re-enroll)
type CLISessionController struct {
	logger    *slog.Logger
	responder *response.Writer
}

func newCLISessionController(deps CLISessionControllerDeps) *CLISessionController {
	return &CLISessionController{
		logger:    deps.Logger,
		responder: deps.Responder,
	}
}

// handleSessionInfo returns the authenticated CLI session's identity
// binding. All values are read from the request context stamped by the
// auth middleware: user_id and cli_session_id come from the persisted CLI
// session, and the operator pair comes from the session's persisted
// operator binding resolved against the operators collection — never from
// request headers. A session with no operator binding reports empty
// operator fields; the caller decides whether that is actionable.
//
// GET /api/v1/auth/cli/session  (RouteAuthMTLS)
func (c *CLISessionController) handleSessionInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, _ := r.Context().Value(constants.ContextKeyUserID).(string)
	cliSessionID, _ := r.Context().Value(constants.ContextKeyCLISessionID).(string)
	if userID == "" || cliSessionID == "" {
		c.logger.Warn("CLI session info: missing authenticated session context")
		c.responder.Error(w, http.StatusUnauthorized, constants.ErrNotAuthenticated.Error())
		return
	}

	operatorSessionID, _ := r.Context().Value(constants.ContextKeyOperatorSessionID).(string)
	operatorID, _ := r.Context().Value(constants.ContextKeyOperatorID).(string)

	c.responder.JSON(w, http.StatusOK, models.CLISessionInfoResponse{
		Success:           true,
		CLISessionID:      cliSessionID,
		UserID:            userID,
		OperatorSessionID: operatorSessionID,
		OperatorID:        operatorID,
	})
}
