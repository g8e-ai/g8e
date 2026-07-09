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
	"sync/atomic"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/gateway/scripts"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/services/tribunal"
)

// HTTPHandlerDependencies groups all dependencies for HTTPHandler to reduce constructor bloat.
type HTTPHandlerDependencies struct {
	Cfg                *config.Config
	Logger             *slog.Logger
	DB                 *CanonicalDBService
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
	Tribunal           *tribunal.TribunalService
	IsReady            func() bool
	IsGovernanceReady  func() bool
}

// HTTPHandler manages the web API for the gateway service.
type HTTPHandler struct {
	cfg                *config.Config
	logger             *slog.Logger
	db                 *CanonicalDBService
	pubsub             *GatewayWebSocketHandler
	auth               *AuthService
	pki                *PKIAuthority
	cliSessionSvc      *CLISessionService
	operatorSessionSvc *OperatorSessionService
	webSessionSvc      *WebSessionService
	reg                *RegistrationService
	passkey            *PasskeyHandler
	userSvc            *UserService
	responder          *response.Writer
	mcp                *mcp.GatewayService
	tribunal           atomic.Pointer[tribunal.TribunalService]
	appEnrollment      *AppEnrollmentService
	isReady            func() bool
	isGovernanceReady  func() bool
	// envProc is the synchronous fail-closed Gateway mutation gate. It is
	// nil until SetEnvelopeProcessor is called by the boot sequence after
	// the in-process command service has initialized the verifier and
	// Actuator. While nil, /api/v1/governance/envelopes returns 503.
	envProc atomic.Pointer[governance.EnvelopeProcessor]

	// Controllers for domain-specific endpoints
	pkiController      *PKIController
	dbController       *DBController
	authController     *AuthController
	adminController    *AdminController
	operatorController *OperatorController

	// Main router cached at construction to avoid rebuilding on every request
	router http.Handler

	// Rate limiting state
	muLimiters      sync.Mutex
	limiters        map[string]*tokenBucket
	limiterLastUsed map[string]time.Time
}

func newHTTPHandler(deps HTTPHandlerDependencies) (*HTTPHandler, error) {
	h := &HTTPHandler{
		cfg:                deps.Cfg,
		logger:             deps.Logger,
		db:                 deps.DB,
		pubsub:             deps.Pubsub,
		auth:               deps.Auth,
		pki:                deps.PKI,
		cliSessionSvc:      deps.CLISessionSvc,
		operatorSessionSvc: deps.OperatorSessionSvc,
		webSessionSvc:      deps.WebSessionSvc,
		reg:                deps.Reg,
		passkey:            deps.Passkey,
		userSvc:            deps.UserSvc,
		responder:          deps.Responder,
		mcp:                deps.MCPGateway,
		appEnrollment:      deps.AppEnrollment,
		isReady:            deps.IsReady,
		isGovernanceReady:  deps.IsGovernanceReady,
		limiters:           make(map[string]*tokenBucket),
		limiterLastUsed:    make(map[string]time.Time),
	}

	// Initialize script templates
	if err := scripts.Init(deps.Logger); err != nil {
		return nil, fmt.Errorf("gateway: failed to initialize script templates: %w", err)
	}

	// Initialize controllers
	h.pkiController = newPKIController(deps.Cfg, deps.Logger, deps.DB, deps.PKI, deps.AppEnrollment, deps.Reg, deps.Responder)
	h.dbController = newDBController(deps.Cfg, deps.Logger, deps.DB, deps.Auth, deps.Pubsub, deps.UserSvc, deps.Responder)

	// Initialize actuator key reader for device enrollment
	actuatorKeyReader := &fileActuatorKeyReader{path: paths.Infra.ActuatorPubJSONPath}
	enrollmentTokenSvc := NewEnrollmentTokenService(deps.DB, deps.Logger)
	h.authController = newAuthController(deps.Cfg, deps.Logger, deps.DB, deps.Auth, deps.Passkey, deps.UserSvc, deps.Reg, deps.PKI, deps.WebSessionSvc, deps.CLISessionSvc, deps.OperatorSessionSvc, enrollmentTokenSvc, deps.Responder, actuatorKeyReader)
	h.adminController = newAdminController(deps.Cfg, deps.Logger, deps.DB, deps.UserSvc, deps.Responder)
	h.operatorController = newOperatorController(deps.Cfg, deps.Logger, deps.Reg, deps.Auth, deps.Responder)

	// Wire tribunal if provided at construction time (may be nil — set
	// later via SetTribunal during boot).
	if deps.Tribunal != nil {
		h.tribunal.Store(deps.Tribunal)
	}

	// Build router once to avoid per-request overhead
	h.router = h.buildPublicRouter()

	return h, nil
}

func (h *HTTPHandler) readBody(r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, h.cfg.Gateway.MaxPayloadBytes)
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
// Thread-safe via atomic.Pointer — no router rebuild needed because the
// tribunal deliberate route is always registered and the handler checks
// the atomic pointer at request time.
func (h *HTTPHandler) SetTribunal(ts *tribunal.TribunalService) {
	h.tribunal.Store(ts)
}

// handleTribunalDeliberate is the always-registered HTTP handler for the
// tribunal deliberate endpoint. It loads the atomic pointer and delegates
// to the TribunalService if wired, or returns 503 if not yet configured.
func (h *HTTPHandler) handleTribunalDeliberate(w http.ResponseWriter, r *http.Request) {
	ts := h.tribunal.Load()
	if ts == nil {
		h.responder.Error(w, http.StatusServiceUnavailable, constants.ErrTribunalNotConfigured.Error())
		return
	}
	(*ts).HandleDeliberate(w, r)
}

func (h *HTTPHandler) GetPasskeyHandler() *PasskeyHandler {
	return h.passkey
}

func (h *HTTPHandler) GetGatewayWebSocketHandler() *GatewayWebSocketHandler {
	return h.pubsub
}
