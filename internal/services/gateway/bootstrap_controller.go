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
	"time"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/gateway/embedded"
	"github.com/g8e-ai/g8e/v2/internal/uuid"
)

// The gateway stores satisfy the embedded substrate interfaces.
var (
	_ embedded.Store            = (*DocumentStoreService)(nil)
	_ embedded.SessionPersister = (*OperatorSessionService)(nil)
	_ embeddedOperatorClaimer   = (*embedded.Service)(nil)
)

// BootstrapControllerDeps groups all dependencies for BootstrapController.
type BootstrapControllerDeps struct {
	Cfg                *config.Config
	Logger             *slog.Logger
	DocStore           *DocumentStoreService
	UserSvc            *UserService
	PKI                *PKIAuthority
	CLISessionSvc      *CLISessionService
	OperatorSessionSvc *OperatorSessionService
	EmbeddedOperator   *embedded.Service
	Responder          *response.Writer
}

// BootstrapController handles system bootstrap, CLI enrollment, operator
// enrollment, and bootstrap status endpoints.
type BootstrapController struct {
	cfg                *config.Config
	logger             *slog.Logger
	docStore           *DocumentStoreService
	userSvc            *UserService
	pki                *PKIAuthority
	cliSessionSvc      *CLISessionService
	operatorSessionSvc *OperatorSessionService
	embeddedOperator   *embedded.Service
	responder          *response.Writer
}

func newBootstrapController(deps BootstrapControllerDeps) *BootstrapController {
	embeddedOperator := deps.EmbeddedOperator
	if embeddedOperator == nil {
		// Test fixtures that build a controller without the substrate
		// still claim through the same package, over the same stores.
		embeddedOperator = embedded.New(deps.DocStore, deps.OperatorSessionSvc)
	}
	return &BootstrapController{
		cfg:                deps.Cfg,
		logger:             deps.Logger,
		docStore:           deps.DocStore,
		userSvc:            deps.UserSvc,
		pki:                deps.PKI,
		cliSessionSvc:      deps.CLISessionSvc,
		operatorSessionSvc: deps.OperatorSessionSvc,
		embeddedOperator:   embeddedOperator,
		responder:          deps.Responder,
	}
}

func (c *BootstrapController) handleLocalBootstrapWithURL(w http.ResponseWriter, r *http.Request) {
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
		Name              string              `json:"name"`
		CLICSRPEM         string              `json:"cli_csr_pem,omitempty"`
		SystemFingerprint string              `json:"system_fingerprint"`
		LocalOSUser       *models.LocalOSUser `json:"local_os_user,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	// Defense-in-depth: refuse if any user already exists, so bootstrap can
	// only run on a genuinely empty system. The first `auth enroll user`
	// creates the first real (admin) user; no other path creates the first
	// user.
	hasUsers, err := c.userSvc.HasAnyUsers()
	if err != nil {
		c.logger.Error("Failed to check for existing users during bootstrap", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "bootstrap check failed")
		return
	}
	if hasUsers {
		c.logger.Warn("Bootstrap attempted on non-empty system", "remote_addr", r.RemoteAddr)
		c.responder.Error(w, http.StatusForbidden, constants.ErrBootstrapInitialSetupOnly.Error())
		return
	}

	// Create the first real user with client-provided OS user information.
	// Zero-PII: the user is created with only a generated ID and OS user info.
	// This user IS the first human enrollee and the gateway owner (assigned the
	// owner role); there is no ephemeral bootstrap-user concept and no
	// retirement flow.
	user, err := c.userSvc.CreateUserWithOSUser(req.LocalOSUser)
	if err != nil {
		c.logger.Error("Failed to create user", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	response := models.BootstrapResponse{
		Success: true,
		User:    user,
		UserID:  user.ID,
	}

	// CLI certificate generation (if provided). Signing runs before any
	// session or operator document writes so a signing failure strands
	// nothing (a signing failure must not leave an orphaned document).
	var cliCertPEM, cliCertChainPEM string
	var cliCertFingerprint, cliCertSerial string

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

	// Claim the gateway's embedded operator. The first user's bootstrap is
	// the explicit human act that enrolls and binds the embedded operator:
	// it sets the owner binding and mints the operator session ID every
	// bootstrap-issued session is bound to. The embedded operator is
	// certless — no CSR is signed for it.
	operatorID, operatorSessionID, err := c.embeddedOperator.Claim(user.ID, req.SystemFingerprint, time.Now().UTC())
	if err != nil {
		c.logger.Error("Failed to claim embedded operator during bootstrap", "error", err, "user_id", user.ID)
		c.responder.Error(w, http.StatusInternalServerError, "failed to bind embedded operator")
		return
	}

	// Persist the operator session the CLI session binds to.
	err = c.operatorSessionSvc.PersistOperatorSession(
		operatorSessionID,
		user.ID,
		user.ID, // Use user ID as org ID for bootstrap
		operatorID,
		string(constants.HeartbeatTypeBootstrap),
	)
	if err != nil {
		c.logger.Error("Failed to persist operator session during bootstrap", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to persist operator session")
		return
	}

	// Always persist the CLI session bound to the embedded operator's
	// session. The persisted binding is authoritative: the auth middleware
	// stamps operator identity from it, never from request headers.
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
		c.logger.Error("Failed to persist CLI session during bootstrap", "error", err)
		c.responder.Error(w, http.StatusInternalServerError, "failed to persist CLI session")
		return
	}

	response.CLISessionID = cliSessionID
	response.OperatorID = operatorID
	response.OperatorSessionID = operatorSessionID

	c.logger.Info("[BOOTSTRAP] System initialized with user, embedded operator and CLI session", "user_id", user.ID, "operator_id", operatorID, "cli_session_id_prefix", safeTruncateID(cliSessionID, 8))

	c.responder.JSON(w, http.StatusCreated, response)
}

func (c *BootstrapController) handleBootstrapStatus(w http.ResponseWriter, r *http.Request) {
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

	// bootstrapped is true when the gateway has been started and at least one
	// owner user exists (HasAnyUsers). The endpoint responding proves the
	// gateway is started; HasAnyUsers proves an owner has been enrolled.
	c.responder.JSON(w, http.StatusOK, models.BootstrapStatusResponse{
		Bootstrapped: hasUsers,
	})
}
