// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/gateway/scripts"
	"github.com/g8e-ai/g8e/internal/services/mcp"
)

// HTTPHandlerDependencies groups all dependencies for HTTPHandler as
// per-controller Deps structs. Each controller's dependency surface is
// explicit and reviewable. Shared infrastructure (Cfg, Logger, Auth,
// Responder) is kept as top-level fields because they are cross-cutting,
// not domain-specific.
type HTTPHandlerDependencies struct {
	Cfg    *config.Config
	Logger *slog.Logger
	Auth   *AuthService

	PKIControllerDeps                PKIControllerDeps
	AuditControllerDeps              AuditControllerDeps
	DataControllerDeps               DataControllerDeps
	SignerControllerDeps             SignerControllerDeps
	BootstrapControllerDeps          BootstrapControllerDeps
	CLIRecoveryControllerDeps        CLIRecoveryControllerDeps
	CLIRotationControllerDeps        CLIRotationControllerDeps
	EnrollmentTokenControllerDeps    EnrollmentTokenControllerDeps
	UserControllerDeps               UserControllerDeps
	SessionControllerDeps            SessionControllerDeps
	AdminControllerDeps              AdminControllerDeps
	OperatorControllerDeps           OperatorControllerDeps
	DispatchControllerDeps           DispatchControllerDeps
	SSEControllerDeps                SSEControllerDeps
	HealthControllerDeps             HealthControllerDeps
	GovernanceControllerDeps         GovernanceControllerDeps
	MCPControllerDeps                MCPControllerDeps
	PubSubControllerDeps             PubSubControllerDeps
	PasskeyControllerDeps            PasskeyControllerDeps
	PlatformEnrollmentControllerDeps PlatformEnrollmentControllerDeps
}

// HTTPHandler manages the web API for the gateway service.
type HTTPHandler struct {
	cfg                          *config.Config
	logger                       *slog.Logger
	authMiddleware               *AuthService
	responder                    *response.Writer
	mcpController                *MCPController
	pubsubController             *PubSubController
	passkeyController            *PasskeyController
	platformEnrollmentController *PlatformEnrollmentController
	// Controllers for domain-specific endpoints
	pkiController             *PKIController
	auditController           *AuditController
	dataController            *DataController
	signerController          *SignerController
	bootstrapController       *BootstrapController
	cliRecoveryController     *CLIRecoveryController
	cliRotationController     *CLIRotationController
	enrollmentTokenController *EnrollmentTokenController
	userController            *UserController
	sessionController         *SessionController
	adminController           *AdminController
	operatorController        *OperatorController
	dispatchController        *DispatchController
	sseController             *SSEController
	healthController          *HealthController
	governanceController      *GovernanceController

	// router is the main HTTP router, built once at construction by
	// buildPublicRouter and cached for the lifetime of the handler. It is
	// never invalidated or rebuilt — all route registrations are resolved
	// against the controller set fixed at construction time. If runtime
	// reconfiguration of routes is ever needed (e.g., hot-reloading
	// controllers), this cache contract must be revisited: the handler would
	// need to rebuild the router under a mutex or swap it atomically.
	router http.Handler

	// Rate limiting state
	muLimiters      sync.Mutex
	limiters        map[string]*tokenBucket
	limiterLastUsed map[string]time.Time
}

func newHTTPHandler(deps HTTPHandlerDependencies) (*HTTPHandler, error) {
	// Initialize script templates
	if err := scripts.Init(deps.Logger); err != nil {
		return nil, fmt.Errorf("gateway: failed to initialize script templates: %w", err)
	}

	// Derive the responder from any controller Deps that carry it. All
	// controllers share the same responder instance, so we can read it
	// from any one. PKIControllerDeps is first in the struct.
	responder := deps.PKIControllerDeps.Responder

	// Initialize enrollment token service (shared dependency, not a controller).
	// If the caller already provided one (e.g., GatewayModeService shares the
	// same instance with the passkey handler), reuse it instead of creating a
	// second service backed by the same store.
	enrollmentTokenSvc := deps.EnrollmentTokenControllerDeps.EnrollmentTokenSvc
	if enrollmentTokenSvc == nil {
		enrollmentTokenSvc = NewEnrollmentTokenService(deps.BootstrapControllerDeps.DocStore, deps.Logger)
		deps.EnrollmentTokenControllerDeps.EnrollmentTokenSvc = enrollmentTokenSvc
	}

	// Initialize CLI recovery service (shared dependency, not a controller)
	cliRecoverySvc := NewCLIRecoveryService(deps.BootstrapControllerDeps.DocStore, deps.Logger)
	deps.CLIRecoveryControllerDeps.RecoverySvc = cliRecoverySvc
	if deps.CLIRecoveryControllerDeps.Cfg == nil {
		deps.CLIRecoveryControllerDeps.Cfg = deps.Cfg
	}
	if deps.CLIRecoveryControllerDeps.Logger == nil {
		deps.CLIRecoveryControllerDeps.Logger = deps.Logger
	}
	if deps.CLIRecoveryControllerDeps.UserSvc == nil {
		deps.CLIRecoveryControllerDeps.UserSvc = deps.BootstrapControllerDeps.UserSvc
	}
	if deps.CLIRecoveryControllerDeps.PKI == nil {
		deps.CLIRecoveryControllerDeps.PKI = deps.BootstrapControllerDeps.PKI
	}
	if deps.CLIRecoveryControllerDeps.CLISessionSvc == nil {
		deps.CLIRecoveryControllerDeps.CLISessionSvc = deps.BootstrapControllerDeps.CLISessionSvc
	}
	if deps.CLIRecoveryControllerDeps.OperatorSessionSvc == nil {
		deps.CLIRecoveryControllerDeps.OperatorSessionSvc = deps.BootstrapControllerDeps.OperatorSessionSvc
	}
	if deps.CLIRecoveryControllerDeps.DocStore == nil {
		deps.CLIRecoveryControllerDeps.DocStore = deps.BootstrapControllerDeps.DocStore
	}
	if deps.CLIRecoveryControllerDeps.Responder == nil {
		deps.CLIRecoveryControllerDeps.Responder = responder
	}

	// CLIRotationController shares the same shared services as recovery
	// (PKI, CLISessionSvc, UserSvc). Defaults are filled from the
	// bootstrap deps so callers only need to set overrides.
	if deps.CLIRotationControllerDeps.Cfg == nil {
		deps.CLIRotationControllerDeps.Cfg = deps.Cfg
	}
	if deps.CLIRotationControllerDeps.Logger == nil {
		deps.CLIRotationControllerDeps.Logger = deps.Logger
	}
	if deps.CLIRotationControllerDeps.PKI == nil {
		deps.CLIRotationControllerDeps.PKI = deps.BootstrapControllerDeps.PKI
	}
	if deps.CLIRotationControllerDeps.CLISessionSvc == nil {
		deps.CLIRotationControllerDeps.CLISessionSvc = deps.BootstrapControllerDeps.CLISessionSvc
	}
	if deps.CLIRotationControllerDeps.UserSvc == nil {
		deps.CLIRotationControllerDeps.UserSvc = deps.BootstrapControllerDeps.UserSvc
	}
	if deps.CLIRotationControllerDeps.Responder == nil {
		deps.CLIRotationControllerDeps.Responder = responder
	}

	h := &HTTPHandler{
		cfg:                          deps.Cfg,
		logger:                       deps.Logger,
		authMiddleware:               deps.Auth,
		responder:                    responder,
		pkiController:                newPKIController(deps.PKIControllerDeps),
		auditController:              newAuditController(deps.AuditControllerDeps),
		dataController:               newDataController(deps.DataControllerDeps),
		signerController:             newSignerController(deps.SignerControllerDeps),
		bootstrapController:          newBootstrapController(deps.BootstrapControllerDeps),
		cliRecoveryController:        newCLIRecoveryController(deps.CLIRecoveryControllerDeps),
		cliRotationController:        newCLIRotationController(deps.CLIRotationControllerDeps),
		enrollmentTokenController:    newEnrollmentTokenController(deps.EnrollmentTokenControllerDeps),
		userController:               newUserController(deps.UserControllerDeps),
		sessionController:            newSessionController(deps.SessionControllerDeps),
		adminController:              newAdminController(deps.AdminControllerDeps),
		operatorController:           newOperatorController(deps.OperatorControllerDeps),
		dispatchController:           newDispatchController(deps.DispatchControllerDeps),
		sseController:                newSSEController(deps.SSEControllerDeps),
		healthController:             newHealthController(deps.HealthControllerDeps),
		governanceController:         newGovernanceController(deps.GovernanceControllerDeps),
		mcpController:                newMCPController(deps.MCPControllerDeps),
		pubsubController:             newPubSubController(deps.PubSubControllerDeps),
		passkeyController:            newPasskeyController(deps.PasskeyControllerDeps),
		platformEnrollmentController: newPlatformEnrollmentController(deps.PlatformEnrollmentControllerDeps),
		limiters:                     make(map[string]*tokenBucket),
		limiterLastUsed:              make(map[string]time.Time),
	}

	// Build router once to avoid per-request overhead
	h.router = h.buildPublicRouter()

	return h, nil
}

func readRequestBody(r *http.Request, maxPayloadBytes int64) ([]byte, error) {
	limited := io.LimitReader(r.Body, maxPayloadBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxPayloadBytes {
		return nil, fmt.Errorf("gateway: %w (limit %d bytes)", constants.ErrPayloadExceedsLimit, maxPayloadBytes)
	}
	return data, nil
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.router.ServeHTTP(w, r)
}

func (h *HTTPHandler) GetMCPGateway() *mcp.GatewayService {
	return h.mcpController.MCPGateway()
}

func (h *HTTPHandler) GetPasskeyHandler() *PasskeyHandler {
	return h.passkeyController.PasskeyHandler()
}

func (h *HTTPHandler) GetGatewayWebSocketHandler() *GatewayWebSocketHandler {
	return h.pubsubController.WebSocketHandler()
}
