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
	"strings"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/network"
)

// PlatformEnrollmentController handles the owner-approved platform
// workload enrollment flow: request creation, status polling,
// proof-of-possession-gated completion, authenticated pending-list
// discovery, and owner decisions (approve/deny).
//
// Auth classification (enforced by the unified auth middleware via
// NewRouteAuthRegistry):
//   - request   (POST /api/v1/auth/platform-enrollments/request):    RouteAuthNone
//     (public discovery surface; activation is checked by the service)
//   - status    (GET  /api/v1/auth/platform-enrollments/status):     RouteAuthNone
//     (public; the opaque token is the lookup key)
//   - complete  (POST /api/v1/auth/platform-enrollments/complete):   RouteAuthNone
//     (public; requires the opaque token AND valid proof-of-possession
//     signatures over the canonical completion transcript for every key)
//   - pending   (GET  /api/v1/auth/platform-enrollments/pending):    RouteAuthDual
//     (owner: web session cookie or mTLS CLI; active-first-user enforced)
//   - decision  (POST /api/v1/auth/platform-enrollments/decision):   RouteAuthDual
//     (owner: web session cookie or mTLS CLI; active-first-user enforced)
//
// request, status, and complete are registered on both the HTTPS router
// and the plain HTTP discovery router (an unenrolled workload has no
// client certificate). pending and decision are registered on HTTPS only.
type PlatformEnrollmentController struct {
	cfg       *config.Config
	logger    *slog.Logger
	enrollSvc *PlatformEnrollmentService
	userSvc   *UserService
	responder *response.Writer
}

// PlatformEnrollmentControllerDeps groups all dependencies for
// PlatformEnrollmentController.
type PlatformEnrollmentControllerDeps struct {
	Cfg       *config.Config
	Logger    *slog.Logger
	EnrollSvc *PlatformEnrollmentService
	UserSvc   *UserService
	Responder *response.Writer
}

func newPlatformEnrollmentController(deps PlatformEnrollmentControllerDeps) *PlatformEnrollmentController {
	return &PlatformEnrollmentController{
		cfg:       deps.Cfg,
		logger:    deps.Logger,
		enrollSvc: deps.EnrollSvc,
		userSvc:   deps.UserSvc,
		responder: deps.Responder,
	}
}

// handlePlatformEnrollmentRequest creates a new pending platform
// enrollment request. The requester (dashboard, ensemble, or operator)
// generates its key pair(s), builds a CSR, and posts the typed request.
// The gateway validates activation and CSRs, deduplicates a live request,
// and returns the request ID, requester token, approval URL, fingerprints,
// and expiry. The raw token is returned once and never persisted.
//
// POST /api/v1/auth/platform-enrollments/request  (RouteAuthNone)
func (c *PlatformEnrollmentController) handlePlatformEnrollmentRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req models.PlatformEnrollmentCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	if err := req.ValidateShape(); err != nil {
		c.writeEnrollmentError(w, err)
		return
	}

	resp, err := c.enrollSvc.CreateRequest(r.Context(), req, c.approvalURLBase())
	if err != nil {
		c.logger.Warn("platform enrollment: create request failed",
			"error", err,
			"component_kind", string(req.ComponentKind),
			"instance_id", req.InstanceID)
		c.writeEnrollmentError(w, err)
		return
	}

	c.logger.Info("platform enrollment request created via controller",
		"request_id", resp.RequestID,
		"component_kind", string(req.ComponentKind),
		"instance_id", req.InstanceID,
		"token_present", resp.Token != "",
	)

	c.responder.JSON(w, http.StatusCreated, resp)
}

// handlePlatformEnrollmentStatus returns the requester-visible state and
// expiry for a request identified by its opaque token. The token is
// passed as the "token" query parameter and is hashed for lookup; the
// raw token is never stored. The response carries no CSR PEM,
// certificates, or identity details — only state, expiry, and
// retry-after for issuing-state requests.
//
// GET /api/v1/auth/platform-enrollments/status?token=<token>  (RouteAuthNone)
func (c *PlatformEnrollmentController) handlePlatformEnrollmentStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrPlatformEnrollmentTokenRequired.Error())
		return
	}

	resp, err := c.enrollSvc.GetStatus(r.Context(), token)
	if err != nil {
		c.writeEnrollmentError(w, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	c.responder.JSON(w, http.StatusOK, resp)
}

// handlePlatformEnrollmentComplete is polled by the requester with
// bounded backoff after approval. The caller proves possession of every
// CSR private key by signing the canonical completion transcript. The
// gateway verifies the token, state, expiry, CSR fingerprints, and
// proofs before issuing or resuming issuance. A completed request
// returns the stored response (idempotent); an issuing request returns
// a typed retryable response.
//
// POST /api/v1/auth/platform-enrollments/complete  (RouteAuthNone)
func (c *PlatformEnrollmentController) handlePlatformEnrollmentComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req models.PlatformEnrollmentCompleteRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	if req.Token == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrPlatformEnrollmentTokenRequired.Error())
		return
	}

	resp, err := c.enrollSvc.Complete(r.Context(), req.Token, req.Proofs)
	if err != nil {
		c.writeEnrollmentError(w, err)
		return
	}

	c.logger.Info("platform enrollment completed via controller",
		"request_id", resp.RequestID,
		"component_kind", string(resp.ComponentKind),
	)

	w.Header().Set("Cache-Control", "no-store")
	c.responder.JSON(w, http.StatusCreated, resp)
}

// handlePlatformEnrollmentPending returns owner-visible metadata for all
// non-terminal platform enrollment requests. The response never includes
// token hashes, CSR PEM, certificates, or raw tokens. The caller must be
// authenticated as the active first user (enforced here after the auth
// middleware stamps the user ID from the web session or mTLS cert).
//
// GET /api/v1/auth/platform-enrollments/pending  (RouteAuthDual)
func (c *PlatformEnrollmentController) handlePlatformEnrollmentPending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.responder.Error(w, http.StatusUnauthorized, constants.ErrWebSessionAuthRequired.Error())
		return
	}

	if err := c.requireActiveFirstUser(userID); err != nil {
		c.writeEnrollmentError(w, err)
		return
	}

	resp, err := c.enrollSvc.ListPending(r.Context())
	if err != nil {
		c.logger.Error("platform enrollment: list pending failed", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to list pending requests")
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	c.responder.JSON(w, http.StatusOK, resp)
}

// handlePlatformEnrollmentDecision records an owner decision (approve or
// deny) on a pending request. The actor user ID is derived from
// authenticated context (web session or mTLS CLI) by the auth middleware;
// the request body carries only the request ID, typed decision, and
// optional bounded reason. Only the active first user may decide.
//
// POST /api/v1/auth/platform-enrollments/decision  (RouteAuthDual)
func (c *PlatformEnrollmentController) handlePlatformEnrollmentDecision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req models.PlatformEnrollmentDecisionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	if err := req.Validate(); err != nil {
		c.writeEnrollmentError(w, err)
		return
	}

	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.responder.Error(w, http.StatusUnauthorized, constants.ErrWebSessionAuthRequired.Error())
		return
	}

	if err := c.requireActiveFirstUser(userID); err != nil {
		c.writeEnrollmentError(w, err)
		return
	}

	resp, err := c.enrollSvc.Decide(r.Context(), userID, req)
	if err != nil {
		c.writeEnrollmentError(w, err)
		return
	}

	c.logger.Info("platform enrollment decision via controller",
		"request_id", resp.RequestID,
		"state", string(resp.State),
		"actor_user_id", userID,
	)

	c.responder.JSON(w, http.StatusOK, resp)
}

// requireActiveFirstUser verifies that the given user ID is the active
// first user (the persistent owner). Returns a typed error if the user
// is not the first user, is not active, or cannot be looked up.
func (c *PlatformEnrollmentController) requireActiveFirstUser(userID string) error {
	user, err := c.userSvc.GetByID(userID)
	if err != nil {
		c.logger.Error("platform enrollment: failed to look up user", "error", err, "user_id", userID)
		return constants.ErrPlatformEnrollmentInvalidDecision
	}
	if user == nil || !user.IsActive() {
		c.logger.Warn("platform enrollment: user is not active", "user_id", userID)
		return constants.ErrPlatformEnrollmentInvalidDecision
	}
	isFirst, err := c.userSvc.IsFirstUser(userID)
	if err != nil {
		c.logger.Error("platform enrollment: failed to check first user", "error", err, "user_id", userID)
		return constants.ErrPlatformEnrollmentInvalidDecision
	}
	if !isFirst {
		return constants.ErrPlatformEnrollmentInvalidDecision
	}
	return nil
}

// approvalURLBase constructs the base URL for the approval console. The
// request ID is placed in the URL fragment so it never reaches
// server-side access logs. The base is derived from the configured
// PublicBaseURL or the localhost HTTPS URL.
func (c *PlatformEnrollmentController) approvalURLBase() string {
	base := c.cfg.Gateway.PublicBaseURL
	if base == "" {
		base = network.LocalhostHTTPSURL(c.cfg.Gateway.HTTPSPort)
	}
	return strings.TrimRight(base, "/") + constants.APIPaths.ConsolePrefix
}

// writeEnrollmentError maps a platform enrollment typed error to the
// appropriate HTTP status code and writes it to the response. Unknown
// errors default to 500 Internal Server Error.
func (c *PlatformEnrollmentController) writeEnrollmentError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	switch err {
	case constants.ErrPlatformEnrollmentRequiresBootstrap:
		c.responder.Error(w, http.StatusForbidden, err.Error())
	case constants.ErrPlatformEnrollmentInvalidComponent:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrPlatformEnrollmentInstanceIDRequired:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrPlatformEnrollmentInvalidInstanceID:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrPlatformEnrollmentHostnameRequired:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrPlatformEnrollmentInvalidHostname:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrPlatformEnrollmentFingerprintRequired:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrPlatformEnrollmentInvalidPayload:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrPlatformEnrollmentInvalidCSR:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrPlatformEnrollmentUnsupportedKey:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrPlatformEnrollmentDuplicateKey:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrPlatformEnrollmentRequestIDRequired:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrPlatformEnrollmentTokenRequired:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrPlatformEnrollmentInvalidToken:
		c.responder.Error(w, http.StatusNotFound, err.Error())
	case constants.ErrPlatformEnrollmentRequestNotFound:
		c.responder.Error(w, http.StatusNotFound, err.Error())
	case constants.ErrPlatformEnrollmentRequestExpired:
		c.responder.Error(w, http.StatusGone, err.Error())
	case constants.ErrPlatformEnrollmentRequestDenied:
		c.responder.Error(w, http.StatusForbidden, err.Error())
	case constants.ErrPlatformEnrollmentNotApproved:
		c.responder.Error(w, http.StatusForbidden, err.Error())
	case constants.ErrPlatformEnrollmentAlreadyDecided:
		c.responder.Error(w, http.StatusConflict, err.Error())
	case constants.ErrPlatformEnrollmentIssuanceInProgress:
		w.Header().Set("Retry-After", "5")
		c.responder.Error(w, http.StatusTooManyRequests, err.Error())
	case constants.ErrPlatformEnrollmentInvalidState:
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
	case constants.ErrPlatformEnrollmentInvalidDecision:
		c.responder.Error(w, http.StatusForbidden, err.Error())
	case constants.ErrPlatformEnrollmentReasonTooLong:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrPlatformEnrollmentProofRequired:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrPlatformEnrollmentProofInvalid:
		c.responder.Error(w, http.StatusUnauthorized, err.Error())
	case constants.ErrPlatformEnrollmentCSRMismatch:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrPlatformEnrollmentQuotaExceeded:
		c.responder.Error(w, http.StatusTooManyRequests, err.Error())
	case constants.ErrPlatformEnrollmentRateLimited:
		w.Header().Set("Retry-After", "5")
		c.responder.Error(w, http.StatusTooManyRequests, err.Error())
	case constants.ErrPlatformEnrollmentGovernanceRejected:
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
	case constants.ErrPlatformEnrollmentPersistenceFailed:
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
	case constants.ErrPlatformEnrollmentIssuanceFailed:
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
	case constants.ErrPlatformEnrollmentStoredRequestInvalid:
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
	default:
		c.logger.Error("platform enrollment: unhandled error", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "internal error")
	}
}
