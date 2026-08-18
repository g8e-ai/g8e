// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/response"
)

// EnrollmentTokenControllerDeps groups all dependencies for EnrollmentTokenController.
type EnrollmentTokenControllerDeps struct {
	Cfg                *config.Config
	Logger             *slog.Logger
	EnrollmentTokenSvc *EnrollmentTokenService
	Responder          *response.Writer
}

// EnrollmentTokenController handles enrollment token generation and validation.
type EnrollmentTokenController struct {
	cfg                *config.Config
	logger             *slog.Logger
	enrollmentTokenSvc *EnrollmentTokenService
	responder          *response.Writer
}

func newEnrollmentTokenController(deps EnrollmentTokenControllerDeps) *EnrollmentTokenController {
	return &EnrollmentTokenController{
		cfg:                deps.Cfg,
		logger:             deps.Logger,
		enrollmentTokenSvc: deps.EnrollmentTokenSvc,
		responder:          deps.Responder,
	}
}

// handleEnrollmentTokenGenerate generates a one-time enrollment token for secure passkey registration.
// This endpoint requires mTLS authentication with a valid CLI session.
func (c *EnrollmentTokenController) handleEnrollmentTokenGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract user_id and cli_session_id stamped by the mTLS CLI-session middleware
	userIDStr, userOK := r.Context().Value(constants.ContextKeyUserID).(string)
	cliSessionIDStr, cliOK := r.Context().Value(constants.ContextKeyCLISessionID).(string)

	if !userOK || !cliOK || userIDStr == "" || cliSessionIDStr == "" {
		c.logger.Warn("Enrollment token generation requested without mTLS CLI session context")
		c.responder.Error(w, http.StatusUnauthorized, "mTLS authentication required")
		return
	}

	// Generate the enrollment token
	token, err := c.enrollmentTokenSvc.GenerateToken(userIDStr, cliSessionIDStr)
	if err != nil {
		c.logger.Error("Failed to generate enrollment token", "error", err, "user_id", userIDStr)
		c.responder.Error(w, http.StatusInternalServerError, "failed to generate enrollment token")
		return
	}

	c.responder.JSON(w, http.StatusCreated, map[string]string{
		"token": token.Token,
	})
}

// handleEnrollmentTokenValidate validates a one-time enrollment token and returns the associated
// user_id and cli_session_id. This endpoint is public (no mTLS required) since the token itself
// provides the authentication context for the browser-based passkey registration flow.
func (c *EnrollmentTokenController) handleEnrollmentTokenValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	if req.Token == "" {
		c.responder.Error(w, http.StatusBadRequest, "token is required")
		return
	}

	// Validate and consume the token
	enrollmentToken, err := c.enrollmentTokenSvc.ValidateAndConsumeToken(req.Token)
	if err != nil {
		tokenPrefix := req.Token
		if len(tokenPrefix) > 8 {
			tokenPrefix = tokenPrefix[:8]
		}
		c.logger.Warn("Enrollment token validation failed", "error", err, "token_prefix", tokenPrefix)
		switch err {
		case constants.ErrEnrollmentTokenExpired:
			c.responder.Error(w, http.StatusGone, "enrollment token has expired")
		case constants.ErrEnrollmentTokenConsumed:
			c.responder.Error(w, http.StatusConflict, "enrollment token has already been used")
		default:
			c.responder.Error(w, http.StatusUnauthorized, "invalid enrollment token")
		}
		return
	}

	c.responder.JSON(w, http.StatusOK, map[string]string{
		"user_id":        enrollmentToken.UserID,
		"cli_session_id": enrollmentToken.CLISessionID,
	})
}
