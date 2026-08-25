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
	"os"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/uuid"
)

// actuatorKeyReader reads the actuator public key from storage.
type actuatorKeyReader interface {
	ReadActuatorPublicKey() (keyID, publicKey string, err error)
}

// fileActuatorKeyReader reads the actuator public key from a file.
type fileActuatorKeyReader struct {
	path string
}

func (r *fileActuatorKeyReader) ReadActuatorPublicKey() (keyID, publicKey string, err error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return "", "", err
	}
	var ap struct {
		KeyID     string `json:"key_id"`
		PublicKey string `json:"public_key"`
	}
	if err := json.Unmarshal(data, &ap); err != nil {
		return "", "", err
	}
	return ap.KeyID, ap.PublicKey, nil
}

// BootstrapControllerDeps groups all dependencies for BootstrapController.
type BootstrapControllerDeps struct {
	Cfg                *config.Config
	Logger             *slog.Logger
	DocStore           *DocumentStoreService
	UserSvc            *UserService
	PKI                *PKIAuthority
	CLISessionSvc      *CLISessionService
	OperatorSessionSvc *OperatorSessionService
	Responder          *response.Writer
	ActuatorKeyReader  actuatorKeyReader
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
	responder          *response.Writer
	actuatorKeyReader  actuatorKeyReader
}

func newBootstrapController(deps BootstrapControllerDeps) *BootstrapController {
	return &BootstrapController{
		cfg:                deps.Cfg,
		logger:             deps.Logger,
		docStore:           deps.DocStore,
		userSvc:            deps.UserSvc,
		pki:                deps.PKI,
		cliSessionSvc:      deps.CLISessionSvc,
		operatorSessionSvc: deps.OperatorSessionSvc,
		responder:          deps.Responder,
		actuatorKeyReader:  deps.ActuatorKeyReader,
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
		CSRPEM            string              `json:"csr_pem"`
		CLICSRPEM         string              `json:"cli_csr_pem,omitempty"`
		SystemFingerprint string              `json:"system_fingerprint"`
		LocalOSUser       *models.LocalOSUser `json:"local_os_user,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrInvalidJSONBody.Error())
		return
	}

	// Check if operator CSR signing is requested for rotation.
	// CLI CSR is handled by the dedicated /api/v1/auth/cli/enroll endpoint
	// to avoid conflating CLI identity with operator identity.
	csrRequested := req.CSRPEM != ""

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

	// If CSR is requested, sign and return the operator cert
	var operatorID, operatorSessionID, orgID string
	if csrRequested {
		// Create Operator slot for the first user
		operatorID = uuid.NewString()
		operatorSessionID = uuid.NewString()
		orgID = user.ID // Use user ID as org ID for bootstrap
		now := time.Now().UTC()

		operator := &models.OperatorDocumentGo{
			ID:                operatorID,
			UserID:            user.ID,
			OrganizationID:    orgID,
			Component:         constants.ComponentNameG8EO,
			Name:              "bootstrap-operator",
			Status:            constants.OperatorStatusActive,
			OperatorSessionID: operatorSessionID,
			OperatorType:      constants.OperatorTypeSystem,
			SystemFingerprint: req.SystemFingerprint,
			Claimed:           true,
			ClaimedAt:         &now,
			CreatedAt:         now,
			UpdatedAt:         now,
		}

		// Sign the CSR
		certPEM, chainPEM, err := c.pki.SignCSR(req.CSRPEM, constants.LeafTypeOperator, orgID, operatorID, user.ID, operatorSessionID, "")
		if err != nil {
			c.logger.Error("Failed to sign bootstrap CSR", "error", err, "user_id", user.ID)
			c.responder.Error(w, http.StatusInternalServerError, "failed to sign CSR")
			return
		}

		operator.OperatorCert = certPEM

		// Persist Operator document
		opBytes, err := json.Marshal(operator)
		if err != nil {
			c.logger.Error("Failed to marshal Operator document", "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to create operator")
			return
		}
		if err := c.docStore.DocSet(marshaler.CollectionName(constants.CollectionOperators), operatorID, opBytes); err != nil {
			c.logger.Error("Failed to persist Operator document", "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to create operator")
			return
		}

		response.OperatorCert = certPEM
		response.OperatorCertChain = chainPEM
		response.OperatorSessionID = operatorSessionID
		response.OperatorID = operatorID
	}

	// CLI certificate generation (if provided)
	var cliCertPEM, cliCertChainPEM string
	var cliCertFingerprint, cliCertSerial string

	// Always create a CLI session ID for CLI-only bootstrap (user_id binding is required)
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

	// Always persist CLI session (even without certificate for CLI-only bootstrap)
	// This ensures user_id binding exists for later CLI enrollment
	err = c.cliSessionSvc.PersistCLISession(
		cliSessionID,
		operatorSessionID, // Empty if no operator CSR
		user.ID,
		"bootstrap-cli",
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

	// Persist operator session only if operator CSR was requested
	if csrRequested {
		err = c.operatorSessionSvc.PersistOperatorSession(
			operatorSessionID,
			user.ID,
			orgID,
			operatorID,
			string(constants.HeartbeatTypeBootstrap),
		)
		if err != nil {
			c.logger.Error("Failed to persist operator session during bootstrap", "error", err)
			c.responder.Error(w, http.StatusInternalServerError, "failed to persist operator session")
			return
		}
		c.logger.Info("[BOOTSTRAP] System initialized with user, operator and CLI session", "user_id", user.ID, "operator_id", operatorID, "cli_session_id_prefix", cliSessionID[:8])
	} else if req.CLICSRPEM != "" {
		c.logger.Info("[BOOTSTRAP] System initialized with user and CLI cert (no operator)", "user_id", user.ID, "cli_session_id_prefix", cliSessionID[:8])
	} else {
		c.logger.Info("[BOOTSTRAP] System initialized with user and CLI session (no CSR)", "user_id", user.ID, "cli_session_id_prefix", cliSessionID[:8])
	}

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
