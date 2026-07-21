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
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/gateway/scripts"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/services/tribunal"
)

// HTTPHandlerDependencies groups all dependencies for HTTPHandler to reduce constructor bloat.
type HTTPHandlerDependencies struct {
	Cfg                  *config.Config
	Logger               *slog.Logger
	Stores               *Stores
	Pubsub               *GatewayWebSocketHandler
	Auth                 *AuthService
	PKI                  *PKIAuthority
	CLISessionSvc        *CLISessionService
	OperatorSessionSvc   *OperatorSessionService
	WebSessionSvc        *WebSessionService
	Reg                  *RegistrationService
	Passkey              *PasskeyHandler
	UserSvc              *UserService
	Responder            *response.Writer
	MCPGateway           *mcp.GatewayService
	AppEnrollment        *AppEnrollmentService
	Tribunal             *tribunal.TribunalService
	IsReady              func() bool
	IsGovernanceReady    func() bool
	SSEHeartbeatInterval time.Duration
}

// HTTPHandler manages the web API for the gateway service.
type HTTPHandler struct {
	cfg                  *config.Config
	logger               *slog.Logger
	pubsub               *GatewayWebSocketHandler
	auth                 *AuthService
	pki                  *PKIAuthority
	cliSessionSvc        *CLISessionService
	operatorSessionSvc   *OperatorSessionService
	webSessionSvc        *WebSessionService
	reg                  *RegistrationService
	passkey              *PasskeyHandler
	userSvc              *UserService
	responder            *response.Writer
	mcp                  *mcp.GatewayService
	appEnrollment        *AppEnrollmentService
	isReady              func() bool
	isGovernanceReady    func() bool
	sseHeartbeatInterval time.Duration
	// Controllers for domain-specific endpoints
	pkiController        *PKIController
	dbController         *DBController
	authController       *AuthController
	adminController      *AdminController
	operatorController   *OperatorController
	sseController        *SSEController
	healthController     *HealthController
	governanceController *GovernanceController

	// Main router cached at construction to avoid rebuilding on every request
	router http.Handler

	// Rate limiting state
	muLimiters      sync.Mutex
	limiters        map[string]*tokenBucket
	limiterLastUsed map[string]time.Time
}

func newHTTPHandler(deps HTTPHandlerDependencies) (*HTTPHandler, error) {
	h := &HTTPHandler{
		cfg:                  deps.Cfg,
		logger:               deps.Logger,
		pubsub:               deps.Pubsub,
		auth:                 deps.Auth,
		pki:                  deps.PKI,
		cliSessionSvc:        deps.CLISessionSvc,
		operatorSessionSvc:   deps.OperatorSessionSvc,
		webSessionSvc:        deps.WebSessionSvc,
		reg:                  deps.Reg,
		passkey:              deps.Passkey,
		userSvc:              deps.UserSvc,
		responder:            deps.Responder,
		mcp:                  deps.MCPGateway,
		appEnrollment:        deps.AppEnrollment,
		isReady:              deps.IsReady,
		isGovernanceReady:    deps.IsGovernanceReady,
		sseHeartbeatInterval: deps.SSEHeartbeatInterval,
		limiters:             make(map[string]*tokenBucket),
		limiterLastUsed:      make(map[string]time.Time),
	}

	if h.sseHeartbeatInterval == 0 {
		h.sseHeartbeatInterval = 30 * time.Second
	}

	// Initialize script templates
	if err := scripts.Init(deps.Logger); err != nil {
		return nil, fmt.Errorf("gateway: failed to initialize script templates: %w", err)
	}

	// Initialize controllers
	h.pkiController = newPKIController(deps.Cfg, deps.Logger, deps.PKI, deps.AppEnrollment, deps.Reg, deps.Responder)
	h.dbController = newDBController(deps.Cfg, deps.Logger, deps.Stores.DocStore, deps.Stores.KVStore, deps.Stores.SSEStore, deps.Stores.BlobStore, deps.Stores.AuditStore, deps.Stores.SignerStore, deps.Auth, deps.Pubsub, deps.UserSvc, deps.Responder)

	// Initialize actuator key reader for device enrollment
	actuatorKeyReader := &fileActuatorKeyReader{path: paths.Infra.ActuatorPubJSONPath}
	enrollmentTokenSvc := NewEnrollmentTokenService(deps.Stores.DocStore, deps.Logger)
	h.authController = newAuthController(AuthControllerDeps{
		Cfg:                deps.Cfg,
		Logger:             deps.Logger,
		DocStore:           deps.Stores.DocStore,
		Auth:               deps.Auth,
		Passkey:            deps.Passkey,
		UserSvc:            deps.UserSvc,
		Reg:                deps.Reg,
		PKI:                deps.PKI,
		WebSessionSvc:      deps.WebSessionSvc,
		CLISessionSvc:      deps.CLISessionSvc,
		OperatorSessionSvc: deps.OperatorSessionSvc,
		EnrollmentTokenSvc: enrollmentTokenSvc,
		Responder:          deps.Responder,
		ActuatorKeyReader:  actuatorKeyReader,
		CrossOrigin:        len(deps.Cfg.Gateway.AllowedOrigins) > 0,
	})
	h.adminController = newAdminController(deps.Cfg, deps.Logger, deps.Stores.DocStore, deps.Stores.SignerStore, deps.Stores.TribunalStore, deps.UserSvc, deps.Responder)
	h.operatorController = newOperatorController(deps.Cfg, deps.Logger, deps.Reg, deps.Auth, deps.Responder)

	h.sseController = newSSEController(deps.Cfg, deps.Logger, deps.Stores.DocStore, deps.Stores.KVStore, deps.Stores.SSEStore, deps.Pubsub, deps.Auth, deps.Responder, deps.SSEHeartbeatInterval)
	h.healthController = newHealthController(deps.Cfg, deps.Logger, deps.Stores.DocStore, deps.Stores.StateRootSvc, deps.Responder, deps.IsReady, deps.IsGovernanceReady)
	h.governanceController = newGovernanceController(deps.Cfg, deps.Logger, deps.Responder, deps.Tribunal)

	// Build router once to avoid per-request overhead
	h.router = h.buildPublicRouter()

	return h, nil
}

func (h *HTTPHandler) readBody(r *http.Request) ([]byte, error) {
	return readRequestBody(r, h.cfg.Gateway.MaxPayloadBytes)
}

func readRequestBody(r *http.Request, maxPayloadBytes int64) ([]byte, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxPayloadBytes)
	return io.ReadAll(r.Body)
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.router.ServeHTTP(w, r)
}

func (h *HTTPHandler) GetMCPGateway() *mcp.GatewayService {
	return h.mcp
}

// SetTribunal sets the Tribunal service for L2 consensus deliberation.
// Called by the boot sequence after the TribunalService is constructed.
// Thread-safe via atomic.Pointer on GovernanceController — no router rebuild
// needed because the tribunal deliberate route is always registered and the
// handler checks the atomic pointer at request time.
func (h *HTTPHandler) SetTribunal(ts *tribunal.TribunalService) {
	h.governanceController.SetTribunal(ts)
}

func (h *HTTPHandler) GetPasskeyHandler() *PasskeyHandler {
	return h.passkey
}

func (h *HTTPHandler) GetGatewayWebSocketHandler() *GatewayWebSocketHandler {
	return h.pubsub
}
