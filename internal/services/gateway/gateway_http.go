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
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/consensus"
	"github.com/g8e-ai/g8e/internal/services/gateway/scripts"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/mcp"
)

// HTTPHandlerDependencies groups all dependencies for HTTPHandler to reduce constructor bloat.
type HTTPHandlerDependencies struct {
	Cfg                *config.Config
	Logger             *slog.Logger
	Stores             *Stores
	Pubsub             *GatewayWebSocketHandler
	Auth               *AuthService
	PKI                *PKIAuthority
	CLISessionSvc      *CLISessionService
	OperatorSessionSvc *OperatorSessionService
	WebSessionSvc      *WebSessionService
	Reg                *RegistrationService
	Passkey            *PasskeyHandler
	UserSvc            *UserService
	Responder          *response.Writer
	MCPGateway         *mcp.GatewayService
	AppEnrollment      *AppEnrollmentService
	Consensus          *consensus.ConsensusService
	EnvProc            governance.EnvelopeProcessor
	IsReady            func() bool
	IsGovernanceReady  func() bool
}

// HTTPHandler manages the web API for the gateway service.
type HTTPHandler struct {
	cfg       *config.Config
	logger    *slog.Logger
	pubsub    *GatewayWebSocketHandler
	auth      *AuthService
	passkey   *PasskeyHandler
	responder *response.Writer
	mcp       *mcp.GatewayService
	// Controllers for domain-specific endpoints
	pkiController             *PKIController
	auditController           *AuditController
	dataController            *DataController
	signerController          *SignerController
	bootstrapController       *BootstrapController
	enrollmentTokenController *EnrollmentTokenController
	userController            *UserController
	sessionController         *SessionController
	adminController           *AdminController
	operatorController        *OperatorController
	sseController             *SSEController
	healthController          *HealthController
	governanceController      *GovernanceController

	// Main router cached at construction to avoid rebuilding on every request
	router http.Handler

	// Rate limiting state
	muLimiters      sync.Mutex
	limiters        map[string]*tokenBucket
	limiterLastUsed map[string]time.Time
}

func newHTTPHandler(deps HTTPHandlerDependencies) (*HTTPHandler, error) {
	h := &HTTPHandler{
		cfg:             deps.Cfg,
		logger:          deps.Logger,
		pubsub:          deps.Pubsub,
		auth:            deps.Auth,
		passkey:         deps.Passkey,
		responder:       deps.Responder,
		mcp:             deps.MCPGateway,
		limiters:        make(map[string]*tokenBucket),
		limiterLastUsed: make(map[string]time.Time),
	}

	// Initialize script templates
	if err := scripts.Init(deps.Logger); err != nil {
		return nil, fmt.Errorf("gateway: failed to initialize script templates: %w", err)
	}

	// Initialize controllers
	h.pkiController = newPKIController(deps.Cfg, deps.Logger, deps.PKI, deps.AppEnrollment, deps.Reg, deps.Responder)
	h.auditController = newAuditController(AuditControllerDeps{
		Cfg:        deps.Cfg,
		Logger:     deps.Logger,
		AuditStore: deps.Stores.AuditStore,
		Responder:  deps.Responder,
	})
	h.dataController = newDataController(DataControllerDeps{
		Cfg:       deps.Cfg,
		Logger:    deps.Logger,
		DocStore:  deps.Stores.DocStore,
		KVStore:   deps.Stores.KVStore,
		SSEStore:  deps.Stores.SSEStore,
		BlobStore: deps.Stores.BlobStore,
		Pubsub:    deps.Pubsub,
		Responder: deps.Responder,
	})
	h.signerController = newSignerController(SignerControllerDeps{
		Cfg:         deps.Cfg,
		Logger:      deps.Logger,
		DocStore:    deps.Stores.DocStore,
		SignerStore: deps.Stores.SignerStore,
		Responder:   deps.Responder,
	})

	// Initialize auth sub-controllers
	enrollmentTokenSvc := NewEnrollmentTokenService(deps.Stores.DocStore, deps.Logger)
	h.bootstrapController = newBootstrapController(BootstrapControllerDeps{
		Cfg:                deps.Cfg,
		Logger:             deps.Logger,
		DocStore:           deps.Stores.DocStore,
		UserSvc:            deps.UserSvc,
		PKI:                deps.PKI,
		CLISessionSvc:      deps.CLISessionSvc,
		OperatorSessionSvc: deps.OperatorSessionSvc,
		Responder:          deps.Responder,
		ActuatorKeyReader:  &fileActuatorKeyReader{path: paths.Infra.ActuatorPubJSONPath},
	})
	h.enrollmentTokenController = newEnrollmentTokenController(EnrollmentTokenControllerDeps{
		Cfg:                deps.Cfg,
		Logger:             deps.Logger,
		EnrollmentTokenSvc: enrollmentTokenSvc,
		Responder:          deps.Responder,
	})
	h.userController = newUserController(UserControllerDeps{
		Cfg:       deps.Cfg,
		Logger:    deps.Logger,
		UserSvc:   deps.UserSvc,
		Responder: deps.Responder,
	})
	h.sessionController = newSessionController(SessionControllerDeps{
		Logger:      deps.Logger,
		DocStore:    deps.Stores.DocStore,
		Responder:   deps.Responder,
		CrossOrigin: len(deps.Cfg.Gateway.AllowedOrigins) > 0,
	})
	h.adminController = newAdminController(deps.Cfg, deps.Logger, deps.Stores.DocStore, deps.Stores.SignerStore, deps.Stores.ConsensusStore, deps.UserSvc, deps.Responder)
	h.operatorController = newOperatorController(deps.Cfg, deps.Logger, deps.Reg, deps.Auth, deps.Responder)

	h.sseController = newSSEController(deps.Cfg, deps.Logger, deps.Stores.DocStore, deps.Stores.KVStore, deps.Stores.SSEStore, deps.Pubsub, deps.Auth, deps.Responder, 0)
	h.healthController = newHealthController(deps.Cfg, deps.Logger, deps.Stores.DocStore, deps.Stores.StateRootSvc, deps.Responder, deps.IsReady, deps.IsGovernanceReady)
	h.governanceController = newGovernanceController(deps.Cfg, deps.Logger, deps.Responder, deps.Consensus, deps.EnvProc)

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
	return h.mcp
}

func (h *HTTPHandler) GetPasskeyHandler() *PasskeyHandler {
	return h.passkey
}

func (h *HTTPHandler) GetGatewayWebSocketHandler() *GatewayWebSocketHandler {
	return h.pubsub
}
