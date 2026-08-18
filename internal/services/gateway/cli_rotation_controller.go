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

// CLIRotationControllerDeps groups all dependencies for CLIRotationController.
type CLIRotationControllerDeps struct {
	Cfg           *config.Config
	Logger        *slog.Logger
	PKI           *PKIAuthority
	CLISessionSvc *CLISessionService
	UserSvc       *UserService
	Responder     *response.Writer
}

// CLIRotationController handles the mTLS-protected CLI certificate rotation
// endpoint. Identity (user ID + active CLI session ID) is derived strictly
// from the authenticated mTLS certificate context stamped by the unified
// auth middleware — request body fields are NOT trusted for identity.
//
// Auth classification (enforced by the unified auth middleware via
// NewRouteAuthRegistry):
//   - rotation (POST /api/v1/auth/cli/rotate): RouteAuthMTLS
//     (requires a verified CLI client certificate whose URI SAN matches an
//     active CLI session; the caller can only rotate their own session)
//
// Rotation is a single transactional replacement: the new certificate is
// signed BEFORE the old session is deactivated, and the old certificate is
// revoked AFTER the session replacement commits. See ReplaceCLISession for
// the partial-failure recovery contract.
type CLIRotationController struct {
	cfg           *config.Config
	logger        *slog.Logger
	pki           *PKIAuthority
	cliSessionSvc *CLISessionService
	userSvc       *UserService
	responder     *response.Writer
}

func newCLIRotationController(deps CLIRotationControllerDeps) *CLIRotationController {
	return &CLIRotationController{
		cfg:           deps.Cfg,
		logger:        deps.Logger,
		pki:           deps.PKI,
		cliSessionSvc: deps.CLISessionSvc,
		userSvc:       deps.UserSvc,
		responder:     deps.Responder,
	}
}

// handleRotate issues a replacement CLI certificate for the caller's active
// CLI session. The caller authenticates via mTLS; the user ID and CLI
// session ID are read from the request context (stamped by the auth
// middleware from the verified certificate URI SAN). The request body
// carries only the new CLI CSR — no identity fields.
//
// Order of operations (per the 5e ReplaceCLISession contract):
//  1. Validate the request body (CSR present, parseable).
//  2. Load the caller's active CLI session; reject if missing or inactive.
//  3. Verify the active session's user is still permitted to authenticate.
//  4. Pre-generate the new CLI session ID and sign the new CLI CSR with
//     that ID in the URI SAN.
//  5. ReplaceCLISession: persist the new session (with the pre-generated
//     ID), atomically deactivate the old one. On the concurrent-loss path,
//     the orphaned new session is cleaned up and the typed error is
//     returned so the caller can revoke the cert it just signed.
//  6. Revoke the old certificate (best-effort, idempotent retry on failure).
//  7. Return the new certificate, chain, session ID, and full trust bundle.
//
// POST /api/v1/auth/cli/rotate  (RouteAuthMTLS)
func (c *CLIRotationController) handleRotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Identity comes from the mTLS context, never from the body.
	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.logger.Warn("CLI rotation: missing authenticated user context")
		c.responder.Error(w, http.StatusUnauthorized, "mTLS authentication required")
		return
	}
	oldCLISessionID, ok := r.Context().Value(constants.ContextKeyCLISessionID).(string)
	if !ok || oldCLISessionID == "" {
		c.logger.Warn("CLI rotation: missing CLI session context", "user_id", userID)
		c.responder.Error(w, http.StatusUnauthorized, "active CLI session required")
		return
	}

	body, err := readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req models.CLIRotationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}
	if req.CLICSRPEM == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrCLIRotationCSRRequired.Error())
		return
	}

	// Load the caller's active CLI session. ReplaceCLISession re-checks
	// activity atomically, but loading first gives precise errors for
	// missing/inactive sessions before we sign anything.
	oldSession, err := c.cliSessionSvc.loadCLISession(oldCLISessionID)
	if err != nil {
		c.writeRotationError(w, err)
		return
	}
	if !oldSession.IsActive {
		c.writeRotationError(w, constants.ErrCLISessionAlreadyDeactivated)
		return
	}

	// Defense in depth: the auth middleware already verified the cert URI
	// SAN matches the session, but the session's stored user ID must also
	// match the context user ID. A mismatch would indicate a stale context
	// or a session re-binding attack.
	if oldSession.UserID != userID {
		c.logger.Warn("CLI rotation: session user mismatch",
			"context_user_id", userID,
			"session_user_id", oldSession.UserID,
			"cli_session_id_prefix", safeTruncateID(oldCLISessionID, 8),
		)
		c.responder.Error(w, http.StatusForbidden, constants.ErrMTLSIdentityMismatch.Error())
		return
	}

	// Verify the user is still active. The auth middleware does this on
	// every request, but a race between middleware and rotation could
	// leave a disabled user with a still-active session.
	user, err := c.userSvc.GetByID(userID)
	if err != nil {
		c.logger.Error("CLI rotation: failed to look up user", "error", err, "user_id", userID)
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify user")
		return
	}
	if user == nil || !user.IsActive() {
		c.logger.Warn("CLI rotation: user is not active", "user_id", userID)
		c.responder.Error(w, http.StatusForbidden, "user is not active")
		return
	}

	// Sign the new CLI CSR BEFORE replacing the session. A signing failure
	// leaves the old session untouched and the caller can retry. The new
	// session ID is pre-generated here so the certificate URI SAN binds to
	// it, and the same ID is passed to ReplaceCLISession so the persisted
	// session and the signed cert stay bound.
	newCLISessionID := uuid.NewString()
	newCertPEM, newCertChainPEM, err := c.pki.SignCSR(
		req.CLICSRPEM,
		constants.LeafTypeCLI,
		"",
		"",
		userID,
		newCLISessionID,
		"",
	)
	if err != nil {
		c.logger.Error("CLI rotation: failed to sign new CLI CSR",
			"error", err,
			"user_id", userID,
			"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID, 8),
		)
		c.writeRotationError(w, fmt.Errorf("%s: %w", constants.ErrCLIRotationFailed, err))
		return
	}

	newCertFingerprint := calculateFingerprintFromPEM(newCertPEM)
	newCertSerial := calculateSerialFromPEM(newCertPEM)

	// Transactionally replace the old session with the new one. The new
	// session inherits the operator binding and system fingerprint from
	// the old session so the caller's routing namespace is preserved. The
	// new session ID is the one we signed the cert's URI SAN against, so
	// the cert and session stay bound.
	_, err = c.cliSessionSvc.ReplaceCLISession(
		oldCLISessionID,
		newCLISessionID,
		newCertFingerprint,
		newCertSerial,
		CLISessionFields{
			OperatorSessionID: oldSession.OperatorSessionID,
			UserID:            oldSession.UserID,
			SystemFingerprint: oldSession.SystemFingerprint,
			CertFingerprint:   newCertFingerprint,
			CertSerial:        newCertSerial,
			LoginMethod:       oldSession.LoginMethod,
		},
	)
	if err != nil {
		// ReplaceCLISession cleans up the orphaned new session on the
		// concurrent-loss path, so the cert we just signed has no
		// matching active session. Revoke it best-effort so it cannot
		// be used, then surface the typed error to the caller.
		c.logger.Warn("CLI rotation: ReplaceCLISession failed",
			"error", err,
			"user_id", userID,
			"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID, 8),
			"new_cli_session_id_prefix", safeTruncateID(newCLISessionID, 8),
		)
		if revokeErr := c.pki.RevokeCertificate(newCertSerial, "rotation_race_lost"); revokeErr != nil {
			c.logger.Error("CLI rotation: failed to revoke orphaned new cert after race loss",
				"error", revokeErr, "new_cert_serial", newCertSerial)
		}
		c.writeRotationError(w, err)
		return
	}

	// Revoke the OLD certificate now that the old session is committed
	// inactive. This is best-effort: if revocation fails, the old session
	// is already inactive so the caller is not locked out, but the old
	// cert may still pass a CRL-skipping verifier. Log loudly and surface
	// a non-fatal warning to the caller via the response — the rotation
	// itself succeeded.
	if oldSession.CertSerial != "" {
		if revokeErr := c.pki.RevokeCertificate(oldSession.CertSerial, "cli_rotation"); revokeErr != nil {
			c.logger.Error("CLI rotation: failed to revoke old cert (rotation succeeded; old session inactive)",
				"error", revokeErr,
				"old_cert_serial", oldSession.CertSerial,
				"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID, 8),
			)
			// Do not fail the request — the new identity is usable.
		}
	}

	// Fetch the full runtime trust bundle so the caller can refresh its
	// local trust store. Non-fatal if unavailable (matches recovery/bootstrap).
	hubBundle, err := c.pki.GatewayTrustBundle()
	if err != nil {
		c.logger.Warn("CLI rotation: failed to fetch hub trust bundle", "error", err)
	}

	c.logger.Info("CLI rotation completed via controller",
		"user_id", userID,
		"old_cli_session_id_prefix", safeTruncateID(oldCLISessionID, 8),
		"new_cli_session_id_prefix", safeTruncateID(newCLISessionID, 8),
	)

	c.responder.JSON(w, http.StatusCreated, models.CLIRotationResponse{
		Success:        true,
		CLISessionID:   newCLISessionID,
		CLICert:        newCertPEM,
		CLICertChain:   newCertChainPEM,
		HubTrustBundle: string(hubBundle),
		UserID:         userID,
	})
}

// writeRotationError maps a typed rotation/session error to the appropriate
// HTTP status code and writes it to the response. Unknown errors default to
// 500 Internal Server Error. Mirrors writeRecoveryError.
func (c *CLIRotationController) writeRotationError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, constants.ErrCLISessionNotFound):
		c.responder.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, constants.ErrCLISessionAlreadyDeactivated):
		c.responder.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, constants.ErrCLIRotationCSRRequired):
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, constants.ErrCLIRotationFailed):
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
	case errors.Is(err, constants.ErrMTLSIdentityMismatch):
		c.responder.Error(w, http.StatusForbidden, err.Error())
	default:
		c.logger.Error("CLI rotation: unhandled error", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "internal error")
	}
}
