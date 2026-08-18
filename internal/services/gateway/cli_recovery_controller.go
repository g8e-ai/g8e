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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/network"
	"github.com/g8e-ai/g8e/internal/uuid"
)

// CLIRecoveryControllerDeps groups all dependencies for CLIRecoveryController.
type CLIRecoveryControllerDeps struct {
	Cfg                *config.Config
	Logger             *slog.Logger
	RecoverySvc        *CLIRecoveryService
	UserSvc            *UserService
	PKI                *PKIAuthority
	CLISessionSvc      *CLISessionService
	OperatorSessionSvc *OperatorSessionService
	DocStore           *DocumentStoreService
	Responder          *response.Writer
}

// CLIRecoveryController handles the human-approved CLI recovery flow:
// request creation, status polling, browser and mTLS approval/denial, and
// proof-of-possession-gated completion that issues a new CLI certificate.
//
// Auth classification (enforced by the unified auth middleware via
// NewRouteAuthRegistry):
//   - recovery request  (POST /api/v1/auth/cli/recovery/request):       RouteAuthNone
//     (public discovery surface; the CSR is the proof-of-possession anchor)
//   - recovery status   (GET  /api/v1/auth/cli/recovery/status):        RouteAuthNone
//     (public; the opaque token itself is the lookup key)
//   - recovery approve  (POST /api/v1/auth/cli/recovery/approve):       RouteAuthWebSession
//     (browser console; authenticated existing user authorizes the new CLI)
//   - recovery approve-cli (POST /api/v1/auth/cli/recovery/approve-cli): RouteAuthMTLS
//     (already-enrolled CLI; approver user ID derived from mTLS cert URI SAN)
//   - recovery complete (POST /api/v1/auth/cli/recovery/complete):      RouteAuthNone
//     (public; requires both the opaque token AND a valid proof-of-possession
//     signature over the request ID using the CSR private key)
type CLIRecoveryController struct {
	cfg                *config.Config
	logger             *slog.Logger
	recoverySvc        *CLIRecoveryService
	userSvc            *UserService
	pki                *PKIAuthority
	cliSessionSvc      *CLISessionService
	operatorSessionSvc *OperatorSessionService
	docStore           *DocumentStoreService
	responder          *response.Writer
}

func newCLIRecoveryController(deps CLIRecoveryControllerDeps) *CLIRecoveryController {
	return &CLIRecoveryController{
		cfg:                deps.Cfg,
		logger:             deps.Logger,
		recoverySvc:        deps.RecoverySvc,
		userSvc:            deps.UserSvc,
		pki:                deps.PKI,
		cliSessionSvc:      deps.CLISessionSvc,
		operatorSessionSvc: deps.OperatorSessionSvc,
		docStore:           deps.DocStore,
		responder:          deps.Responder,
	}
}

// handleRecoveryRequest creates a new pending CLI recovery request.
// The new CLI generates a CSR and posts it over the discovery surface.
// No certificate is issued yet; the gateway returns an opaque one-time
// token and a browser approval URL. The token is only returned once.
//
// POST /api/v1/auth/cli/recovery/request
func (c *CLIRecoveryController) handleRecoveryRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req models.CLIRecoveryRequestRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	if req.CLICSRPEM == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrCLICSRRequired.Error())
		return
	}

	// Recovery is only available on an already-bootstrapped gateway.
	// An unbootstrapped gateway must use the bootstrap endpoint instead.
	hasUsers, err := c.userSvc.HasAnyUsers()
	if err != nil {
		c.logger.Error("CLI recovery: failed to check bootstrap state", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to check gateway state")
		return
	}
	if !hasUsers {
		c.responder.Error(w, http.StatusForbidden, "CLI recovery only available after bootstrap")
		return
	}

	requestID, token, expiresAt, err := c.recoverySvc.CreateRequest(req.CLICSRPEM, req.SystemFingerprint, req.LocalOSUser)
	if err != nil {
		c.logger.Error("CLI recovery: failed to create request", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to create recovery request")
		return
	}

	approvalURL := c.buildApprovalURL(token)

	c.logger.Info("CLI recovery request created via controller",
		"request_id", requestID,
		"token_prefix", safePrefix(token),
		"expires_at", expiresAt,
	)

	c.responder.JSON(w, http.StatusCreated, models.CLIRecoveryRequestResponse{
		Success:     true,
		RequestID:   requestID,
		Token:       token,
		ApprovalURL: approvalURL,
		ExpiresAt:   expiresAt,
	})
}

// handleRecoveryStatus returns the current lifecycle state of a CLI recovery
// request. The opaque token is passed as the "token" query parameter.
//
// GET /api/v1/auth/cli/recovery/status?token=<opaque-token>
func (c *CLIRecoveryController) handleRecoveryStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		c.responder.Error(w, http.StatusBadRequest, "token query parameter is required")
		return
	}

	state, err := c.recoverySvc.GetStatus(token)
	if err != nil {
		c.writeRecoveryError(w, err)
		return
	}

	c.responder.JSON(w, http.StatusOK, models.CLIRecoveryStatusResponse{
		Success: true,
		State:   state,
	})
}

// handleRecoveryApprove is called from the browser console by an
// authenticated existing user. The user approves or denies the pending
// recovery request, binding the decision to their user ID. Only a pending
// request can be approved or denied.
//
// POST /api/v1/auth/cli/recovery/approve  (RouteAuthWebSession)
func (c *CLIRecoveryController) handleRecoveryApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req models.CLIRecoveryApproveRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	if req.Token == "" {
		c.responder.Error(w, http.StatusBadRequest, "token is required")
		return
	}

	// The web-session middleware stamps the authenticated user ID.
	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.logger.Warn("CLI recovery approve: missing authenticated user context")
		c.responder.Error(w, http.StatusUnauthorized, constants.ErrWebSessionAuthRequired.Error())
		return
	}

	// Verify the approving user is still active before binding the decision.
	approvingUser, err := c.userSvc.GetByID(userID)
	if err != nil {
		c.logger.Error("CLI recovery approve: failed to look up approving user", "error", err, "user_id", userID)
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify user")
		return
	}
	if approvingUser == nil || !approvingUser.IsActive() {
		c.logger.Warn("CLI recovery approve: approving user is not active", "user_id", userID)
		c.responder.Error(w, http.StatusForbidden, "approving user is not active")
		return
	}

	var state models.CLIRecoveryState
	if req.Approve {
		if err := c.recoverySvc.Approve(req.Token, userID); err != nil {
			c.writeRecoveryError(w, err)
			return
		}
		state = models.CLIRecoveryStateApproved
	} else {
		if err := c.recoverySvc.Deny(req.Token, userID); err != nil {
			c.writeRecoveryError(w, err)
			return
		}
		state = models.CLIRecoveryStateDenied
	}

	c.responder.JSON(w, http.StatusOK, models.CLIRecoveryApproveResponse{
		Success: true,
		State:   state,
	})
}

// handleRecoveryApproveCLI is the mTLS counterpart to handleRecoveryApprove.
// It is called by an already-enrolled CLI (via `g8e auth approve-recovery
// <token>`) to approve or deny a pending recovery request created by another
// CLI's `auth enroll --headless` run. The approver user ID is derived from the
// verified CLI certificate URI SAN — stamped into the request context by the
// unified auth middleware (handleMTLSAuth → handleCLIAuth). The request body
// carries only the token and approve/deny flag; no identity fields.
//
// POST /api/v1/auth/cli/recovery/approve-cli  (RouteAuthMTLS)
func (c *CLIRecoveryController) handleRecoveryApproveCLI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req models.CLIRecoveryApproveRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	if req.Token == "" {
		c.responder.Error(w, http.StatusBadRequest, "token is required")
		return
	}

	// Identity comes from the mTLS context, never from the body. The unified
	// auth middleware verified the CLI cert URI SAN via wid.MatchesCLI and
	// confirmed the session is active before stamping ContextKeyUserID.
	userID, ok := r.Context().Value(constants.ContextKeyUserID).(string)
	if !ok || userID == "" {
		c.logger.Warn("CLI recovery approve-cli: missing authenticated user context")
		c.responder.Error(w, http.StatusUnauthorized, constants.ErrMTLSCertRequired.Error())
		return
	}

	// Verify the approving user is still active before binding the decision.
	approvingUser, err := c.userSvc.GetByID(userID)
	if err != nil {
		c.logger.Error("CLI recovery approve-cli: failed to look up approving user", "error", err, "user_id", userID)
		c.responder.Error(w, http.StatusInternalServerError, "failed to verify user")
		return
	}
	if approvingUser == nil || !approvingUser.IsActive() {
		c.logger.Warn("CLI recovery approve-cli: approving user is not active", "user_id", userID)
		c.responder.Error(w, http.StatusForbidden, "approving user is not active")
		return
	}

	var state models.CLIRecoveryState
	if req.Approve {
		if err := c.recoverySvc.Approve(req.Token, userID); err != nil {
			c.writeRecoveryError(w, err)
			return
		}
		state = models.CLIRecoveryStateApproved
	} else {
		if err := c.recoverySvc.Deny(req.Token, userID); err != nil {
			c.writeRecoveryError(w, err)
			return
		}
		state = models.CLIRecoveryStateDenied
	}

	c.responder.JSON(w, http.StatusOK, models.CLIRecoveryApproveResponse{
		Success: true,
		State:   state,
	})
}

// handleRecoveryComplete is polled by the new CLI with bounded backoff.
// Only after the request has been approved AND the caller proves possession
// of the CSR private key (by signing the request ID) does the gateway
// atomically transition the request to completed and issue a new CLI
// certificate bound to the approving user. The token is consumed once.
//
// POST /api/v1/auth/cli/recovery/complete
func (c *CLIRecoveryController) handleRecoveryComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := readRequestBody(r, c.cfg.Gateway.MaxPayloadBytes)
	if err != nil {
		c.responder.Error(w, http.StatusBadRequest, "failed to read body")
		return
	}

	var req models.CLIRecoveryCompleteRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	if req.Token == "" {
		c.responder.Error(w, http.StatusBadRequest, "token is required")
		return
	}
	if req.Signature == "" {
		c.responder.Error(w, http.StatusBadRequest, "signature is required")
		return
	}

	// Look up the request by token. GetByToken auto-expires stale requests.
	recoveryReq, err := c.recoverySvc.GetByToken(req.Token)
	if err != nil {
		c.writeRecoveryError(w, err)
		return
	}

	// Only approved requests can be completed. Give precise errors for
	// other states before attempting proof-of-possession verification.
	switch recoveryReq.State {
	case models.CLIRecoveryStatePending:
		c.writeRecoveryError(w, constants.ErrCLIRecoveryNotApproved)
		return
	case models.CLIRecoveryStateDenied:
		c.writeRecoveryError(w, constants.ErrCLIRecoveryRequestDenied)
		return
	case models.CLIRecoveryStateCompleted:
		c.writeRecoveryError(w, constants.ErrCLIRecoveryRequestConsumed)
		return
	case models.CLIRecoveryStateExpired:
		c.writeRecoveryError(w, constants.ErrCLIRecoveryRequestExpired)
		return
	case models.CLIRecoveryStateApproved:
		// proceed
	default:
		c.writeRecoveryError(w, constants.ErrCLIRecoveryRequestFailed)
		return
	}

	// Decode the base64-encoded signature.
	signature, err := base64.StdEncoding.DecodeString(req.Signature)
	if err != nil {
		c.logger.Warn("CLI recovery complete: failed to decode signature", "error", err, "token_prefix", safePrefix(req.Token))
		c.responder.Error(w, http.StatusBadRequest, constants.ErrCLIRecoveryProofInvalid.Error())
		return
	}

	// Verify proof-of-possession of the CSR private key BEFORE the atomic
	// state transition. A failed proof must not consume the token — the
	// legitimate CLI may still complete the request.
	if err := c.recoverySvc.VerifyProofOfPossession(recoveryReq, signature); err != nil {
		c.writeRecoveryError(w, err)
		return
	}

	// Atomically transition approved → completed. Only one concurrent
	// caller can succeed; others receive ErrCLIRecoveryRequestConsumed.
	completedReq, err := c.recoverySvc.Complete(req.Token)
	if err != nil {
		c.writeRecoveryError(w, err)
		return
	}

	// Issue the CLI certificate bound to the approving user.
	resp, err := c.issueCLIIdentity(completedReq)
	if err != nil {
		c.logger.Error("CLI recovery complete: failed to issue CLI identity",
			"error", err,
			"request_id", completedReq.ID,
			"approving_user_id", completedReq.ApprovingUserID,
		)
		c.responder.Error(w, http.StatusInternalServerError, "failed to issue CLI certificate")
		return
	}

	c.logger.Info("CLI recovery completed via controller",
		"request_id", completedReq.ID,
		"token_prefix", safePrefix(req.Token),
		"approving_user_id", completedReq.ApprovingUserID,
		"cli_session_id_prefix", safePrefix(resp.CLISessionID),
	)

	c.responder.JSON(w, http.StatusCreated, resp)
}

// issueCLIIdentity signs the stored CLI CSR, creates an operator slot and
// sessions, and returns the typed completion response. The new identity is
// bound to the approving user (not the bootstrap user).
func (c *CLIRecoveryController) issueCLIIdentity(req *models.CLIRecoveryRequest) (models.CLIRecoveryCompleteResponse, error) {
	userID := req.ApprovingUserID
	if userID == "" {
		return models.CLIRecoveryCompleteResponse{}, fmt.Errorf("recovery request has no approving user")
	}

	// Validate the approving user still exists and is active.
	user, err := c.userSvc.GetByID(userID)
	if err != nil {
		return models.CLIRecoveryCompleteResponse{}, fmt.Errorf("look up approving user: %w", err)
	}
	if user == nil || !user.IsActive() {
		return models.CLIRecoveryCompleteResponse{}, fmt.Errorf("approving user is not active")
	}

	operatorID := uuid.NewString()
	operatorSessionID := uuid.NewString()
	cliSessionID := uuid.NewString()
	orgID := user.ID // Use user ID as org ID (matches bootstrap/CLI-enroll pattern)
	now := time.Now().UTC()

	// Sign the CLI CSR stored in the recovery request BEFORE persisting any
	// documents. A signing failure must not leave an orphaned operator
	// document in the database.
	cliCertPEM, cliCertChainPEM, err := c.pki.SignCSR(req.CLICSRPEM, constants.LeafTypeCLI, "", "", user.ID, cliSessionID, "")
	if err != nil {
		return models.CLIRecoveryCompleteResponse{}, fmt.Errorf("sign CLI CSR: %w", err)
	}

	cliCertFingerprint := calculateFingerprintFromPEM(cliCertPEM)
	cliCertSerial := calculateSerialFromPEM(cliCertPEM)

	// Create operator slot associated with this recovered CLI identity.
	operator := &models.OperatorDocumentGo{
		ID:                operatorID,
		UserID:            user.ID,
		OrganizationID:    orgID,
		Component:         constants.ComponentNameG8EO,
		Name:              "cli-recovery-" + safePrefix(user.ID),
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
		return models.CLIRecoveryCompleteResponse{}, fmt.Errorf("marshal operator document: %w", err)
	}
	if err := c.docStore.DocSet(marshaler.CollectionName(constants.CollectionOperators), operatorID, opBytes); err != nil {
		return models.CLIRecoveryCompleteResponse{}, fmt.Errorf("persist operator document: %w", err)
	}

	// Persist CLI session linked to the operator session.
	if err := c.cliSessionSvc.PersistCLISession(
		cliSessionID,
		operatorSessionID,
		user.ID,
		req.SystemFingerprint,
		cliCertFingerprint,
		cliCertSerial,
		string(constants.HeartbeatTypeBootstrap),
	); err != nil {
		return models.CLIRecoveryCompleteResponse{}, fmt.Errorf("persist CLI session: %w", err)
	}

	// Persist operator session.
	if err := c.operatorSessionSvc.PersistOperatorSession(
		operatorSessionID,
		user.ID,
		orgID,
		operatorID,
		string(constants.HeartbeatTypeBootstrap),
	); err != nil {
		return models.CLIRecoveryCompleteResponse{}, fmt.Errorf("persist operator session: %w", err)
	}

	// Fetch the full runtime trust bundle.
	hubBundle, err := c.pki.GatewayTrustBundle()
	if err != nil {
		c.logger.Warn("CLI recovery: failed to fetch hub trust bundle", "error", err)
		// Non-fatal — continue without bundle (matches bootstrap/CLI-enroll behavior).
	}

	return models.CLIRecoveryCompleteResponse{
		Success:           true,
		CLISessionID:      cliSessionID,
		CLICert:           cliCertPEM,
		CLICertChain:      cliCertChainPEM,
		HubTrustBundle:    string(hubBundle),
		UserID:            user.ID,
		OperatorSessionID: operatorSessionID,
		OperatorID:        operatorID,
	}, nil
}

// buildApprovalURL constructs the browser console URL for recovery approval.
// The opaque token is placed in the URL fragment so it never reaches the
// server-side access log and is cleared by the console SPA immediately
// after reading it.
func (c *CLIRecoveryController) buildApprovalURL(token string) string {
	base := c.cfg.Gateway.PublicBaseURL
	if base == "" {
		base = network.LocalhostHTTPSURL(c.cfg.Gateway.HTTPSPort)
	}
	base = strings.TrimRight(base, "/")
	return base + constants.APIPaths.ConsolePrefix + "#recovery=" + token
}

// writeRecoveryError maps a CLIRecoveryService/typed error to the appropriate
// HTTP status code and writes it to the response. Unknown errors default to
// 500 Internal Server Error.
func (c *CLIRecoveryController) writeRecoveryError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	switch err {
	case constants.ErrCLIRecoveryRequestNotFound:
		c.responder.Error(w, http.StatusNotFound, err.Error())
	case constants.ErrCLIRecoveryRequestExpired:
		c.responder.Error(w, http.StatusGone, err.Error())
	case constants.ErrCLIRecoveryRequestConsumed:
		c.responder.Error(w, http.StatusConflict, err.Error())
	case constants.ErrCLIRecoveryRequestDenied:
		c.responder.Error(w, http.StatusForbidden, err.Error())
	case constants.ErrCLIRecoveryNotApproved:
		c.responder.Error(w, http.StatusForbidden, err.Error())
	case constants.ErrCLIRecoveryProofInvalid:
		c.responder.Error(w, http.StatusUnauthorized, err.Error())
	case constants.ErrCLIRecoveryCSRMismatch:
		c.responder.Error(w, http.StatusBadRequest, err.Error())
	case constants.ErrCLIRecoveryRequestFailed:
		c.responder.Error(w, http.StatusInternalServerError, err.Error())
	default:
		c.logger.Error("CLI recovery: unhandled error", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "internal error")
	}
}
