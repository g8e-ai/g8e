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

package pubsub

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	govpkg "github.com/g8e-ai/g8e/internal/governance"
	"github.com/g8e-ai/g8e/internal/models"
	execution "github.com/g8e-ai/g8e/internal/services/execution"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// PubSubCommandMessage is the inbound wire message received from Operator pub/sub.
type PubSubCommandMessage struct {
	ID                string              `json:"id"`
	EventType         constants.EventType `json:"event_type"`
	CaseID            string              `json:"case_id"`
	TaskID            *string             `json:"task_id"`
	InvestigationID   string              `json:"investigation_id"`
	WebSessionID      string              `json:"web_session_id"`
	CLISessionID      string              `json:"cli_session_id"`
	OperatorSessionID string              `json:"operator_session_id"`
	OperatorID        *string             `json:"operator_id"`
	Payload           json.RawMessage     `json:"payload"`
	DecodedPayload    proto.Message       `json:"-"`
	Timestamp         time.Time           `json:"timestamp"`
}

// OperatorPubSubService manages the Operator pub/sub connection and dispatches inbound
// Operator commands to the appropriate first-class service handler.
type OperatorPubSubService struct {
	client  PubSubClient
	config  *config.Config
	logger  *slog.Logger
	results ResultsPublisher

	heartbeat *HeartbeatService
	commands  *CommandService
	fileOps   *FileOpsService
	ports     *PortService
	audit     *AuditService
	history   *HistoryService

	ShutdownChan chan string

	handlers map[constants.EventType]func(context.Context, *PubSubCommandMessage)

	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.RWMutex

	reconnectBaseDelay time.Duration

	// governance services
	actuator    *governance.L5Actuator
	l4warden    *governance.L4Warden
	signerStore governance.SignerStore

	// MCP gateway for protocol translation egress
	mcpGateway *mcp.GatewayService
}

// GovernanceDeps holds the governance dependencies required for transaction
// verification in both outbound and gateway modes. These interfaces are
// implemented by CanonicalDBService (ReplayStore, StateRootProvider,
// TransactionAuditStore) and the governance L3Notary. In outbound mode,
// ConsensusPolicyStore and FieldReader are wired with no-op implementations
// (NoopConsensusPolicyStore, NoopFieldReader) to eliminate nil fields.
type GovernanceDeps struct {
	ReplayStore          governance.ReplayStore
	StateRootProvider    governance.StateRootProvider
	TransactionAudit     governance.TransactionAuditStore
	L3Notary             governance.L3Notary
	SignerStore          governance.SignerStore
	ConsensusPolicyStore governance.L2ConsensusPolicyStore
	FieldReader          mcp.FieldReader
	Doctrine             *governance.L1Doctrine
}

// CommandServiceConfig holds non-governance dependencies for
// OperatorPubSubService in both outbound and gateway modes. Governance
// dependencies are passed separately via GovernanceDeps. Gateway-only fields
// (MCPGateway, GovDeps) are in GatewayCommandServiceConfig to enforce mode
// bifurcation at the type level.
type CommandServiceConfig struct {
	Config         *config.Config
	Logger         *slog.Logger
	Execution      *execution.ExecutionService
	FileEdit       *execution.FileEditService
	PubSubClient   PubSubClient
	ResultsService ResultsPublisher
	ExecutionVault storage.ExecutionVault
	AuditStore     *storage.SQLAuditStore
	Ledger         *storage.GitLedgerService
	HistoryHandler *storage.HistoryHandler
	Scrubbing      *scrubbing.ScrubbingService

	// Actuator configuration
	ActuatorSigningKey ed25519.PrivateKey
	ActuatorKeyID      string
}

// GatewayCommandServiceConfig embeds CommandServiceConfig and adds
// gateway-only fields that are not applicable in outbound mode.
//
// MCPGateway is the egress dispatcher for protocol translation.
// GovDeps provides the governance dependencies (including FieldReader) that
// are shared between gateway construction and the pubsub command service.
type GatewayCommandServiceConfig struct {
	CommandServiceConfig
	GovDeps                *GovernanceDeps
	MCPGateway             *mcp.GatewayService
	L2ConsensusDeliberator mcp.L2ConsensusDeliberator
}

// NewOperatorPubSubService creates the dispatcher and all first-class sub-services using the provided config.
func NewOperatorPubSubService(c CommandServiceConfig, govDeps GovernanceDeps) (*OperatorPubSubService, error) {
	client := c.PubSubClient
	if client == nil {
		return nil, fmt.Errorf("%w: PubSubClient is required", constants.ErrPubSubEmptyPayload)
	}

	serviceCtx, cancel := context.WithCancel(context.Background())

	rs := &OperatorPubSubService{
		client:             client,
		config:             c.Config,
		logger:             c.Logger,
		results:            c.ResultsService,
		ctx:                serviceCtx,
		cancel:             cancel,
		ShutdownChan:       make(chan string, 1),
		reconnectBaseDelay: 1 * time.Second,
	}

	rs.heartbeat = NewHeartbeatService(c.Config, c.Logger, &rs.wg)
	rs.heartbeat.ctx = serviceCtx
	rs.heartbeat.results = c.ResultsService
	rs.heartbeat.SetActuator(rs.actuator)

	rs.commands = NewCommandService(c.Config, c.Logger, c.Execution)
	rs.commands.results = c.ResultsService
	rs.commands.scrubbing = c.Scrubbing
	rs.commands.vaultWriter = NewVaultWriter(c.Config, c.Logger, c.ExecutionVault)
	rs.commands.auditStore = c.AuditStore
	rs.commands.ledger = c.Ledger
	rs.commands.historyHandler = c.HistoryHandler

	rs.fileOps = NewFileOpsService(c.Config, c.Logger, c.FileEdit, client)
	rs.fileOps.results = c.ResultsService
	rs.fileOps.SetScrubbingService(c.Scrubbing)
	rs.fileOps.vaultWriter = NewVaultWriter(c.Config, c.Logger, c.ExecutionVault)
	rs.fileOps.auditStore = c.AuditStore
	rs.fileOps.ledger = c.Ledger
	rs.fileOps.auditStoreForObserved = c.AuditStore

	rs.ports = NewPortService(c.Config, c.Logger, client)
	rs.ports.SetScrubbingService(c.Scrubbing)
	rs.ports.auditStore = c.AuditStore

	rs.audit = NewAuditService(c.Config, c.Logger, c.AuditStore)

	rs.history = NewHistoryService(c.Config, c.Logger, client)
	rs.history.executionVault = c.ExecutionVault
	rs.history.historyHandler = c.HistoryHandler
	rs.history.SetScrubbingService(c.Scrubbing)
	rs.history.auditStore = c.AuditStore

	rs.buildHandlers()

	rs.signerStore = govDeps.SignerStore
	if rs.signerStore == nil {
		// Provide a fallback empty signer store instead of loading from filesystem.
		// This ensures outbound mode fails closed if no signer store is provided.
		rs.signerStore = &governance.FailClosedSignerStore{Signers: make(map[string]ed25519.PublicKey)}
		c.Logger.Warn("No SignerStore provided; signed transactions will be rejected")
	}

	// Validate required governance dependencies (fail-closed: missing deps = fatal error)
	if govDeps.ReplayStore == nil {
		return nil, constants.ErrTxReplayStoreMissing
	}
	if govDeps.StateRootProvider == nil {
		return nil, constants.ErrTxStateRootRequired
	}
	// L3Notary is optional for outbound mode (platform verifies L3)
	// Mutations requiring L3 will fail-closed at TransactionVerifier if L3Notary is nil

	// Provide no-op defaults for optional governance deps to eliminate nil fields
	if govDeps.ConsensusPolicyStore == nil {
		govDeps.ConsensusPolicyStore = &governance.NoopConsensusPolicyStore{}
	}
	if govDeps.FieldReader == nil {
		govDeps.FieldReader = &mcp.NoopFieldReader{}
	}

	// Initialize governance services after trusted signers are loaded
	rs.initializeGovernance(c, govDeps)

	c.Logger.Info("g8e connectivity initialized")
	if c.Config.OperatorID != "" {
		c.Logger.Info("Operator identity configured",
			"operator_id", c.Config.OperatorID,
			"operator_session_id", c.Config.OperatorSessionId)
	}
	return rs, nil
}

// NewGatewayOperatorPubSubService creates the dispatcher for gateway mode,
// wiring gateway-only dependencies (MCPGateway, GovDeps) after base
// construction. Use NewOperatorPubSubService for outbound mode.
func NewGatewayOperatorPubSubService(c GatewayCommandServiceConfig) (*OperatorPubSubService, error) {
	if c.GovDeps == nil {
		return nil, fmt.Errorf("%w: GovDeps is required for gateway mode", constants.ErrInternal)
	}

	rs, err := NewOperatorPubSubService(c.CommandServiceConfig, *c.GovDeps)
	if err != nil {
		return nil, err
	}

	rs.mcpGateway = c.MCPGateway

	// Wire the MCP gateway's runtime governance dependencies. This is the single
	// owner of runtime-phase wiring; config-phase fields (A2A downstream and the
	// public base URL) are owned by the gateway's own construction in
	// GatewayModeService.initHandlersAndServers and must not be re-set here.
	// MCPGateway is used as the egress dispatcher for protocol translation.
	if rs.mcpGateway != nil {
		var auditLogger mcp.AuditLogger
		if c.AuditStore != nil {
			auditLogger = &pubsubAuditLogger{store: c.AuditStore, logger: c.Logger}
		}

		fieldReader := c.GovDeps.FieldReader

		rs.mcpGateway.SetRuntimeDeps(mcp.RuntimeDependencies{
			EnvProc:                rs,
			StateRootProvider:      c.GovDeps.StateRootProvider,
			SigningKey:             c.ActuatorSigningKey,
			KeyID:                  c.ActuatorKeyID,
			DownstreamURL:          c.Config.Gateway.MCPDownstreamURL,
			DBService:              fieldReader,
			SessionValidator:       rs,
			AuditLogger:            auditLogger,
			L2ConsensusDeliberator: c.L2ConsensusDeliberator,
		})
	}

	return rs, nil
}

func (rs *OperatorPubSubService) initializeGovernance(c CommandServiceConfig, govDeps GovernanceDeps) {
	// Initialize L5Actuator with trusted nodes and audit store
	// ScrubbingService handles data scrubbing/rehydration at the execution boundary
	rs.actuator = &governance.L5Actuator{
		Logger:            c.Logger,
		SQLAuditStore:     c.AuditStore,
		ConsoleAuditStore: govDeps.TransactionAudit,
		StateRootProvider: govDeps.StateRootProvider,
		ExecutionHandler:  rs, // OperatorPubSubService implements ExecutionHandler
		Scrubbing:         c.Scrubbing,
		SigningKey:        c.ActuatorSigningKey,
		KeyID:             c.ActuatorKeyID,
	}

	// Initialize TransactionVerifier for strict pre-dispatch verification
	knownActionTypes := constants.AllActionTypes
	// Use Gateway.Posture for gateway mode, Config.Posture for outbound mode
	posture := string(c.Config.Gateway.Posture)
	if posture == "" {
		posture = string(c.Config.Posture)
	}
	if posture == "" {
		posture = "notary" // Default to notary for outbound mode since L3Notary is nil
	}
	// Default to NewL1Doctrine if not provided (outbound mode may not configure doctrine)
	doctrine := govDeps.Doctrine
	if doctrine == nil {
		doctrine = governance.NewL1Doctrine()
		c.Logger.Warn("No L1Doctrine provided; using default doctrine")
	}

	rs.l4warden = governance.NewL4Warden(
		c.Logger,
		govDeps.ReplayStore,
		govDeps.StateRootProvider,
		rs.signerStore,
		govDeps.ConsensusPolicyStore,
		govDeps.L3Notary,
		doctrine,
		knownActionTypes,
		posture,
		nil, // Clock defaults to RealClock
	)

	var signerStoreType, l4wardenType string
	if rs.signerStore != nil {
		signerStoreType = reflect.TypeOf(rs.signerStore).Elem().Name()
	}
	if rs.l4warden != nil {
		l4wardenType = reflect.TypeOf(rs.l4warden).Elem().Name()
	}
	c.Logger.Info("governance services initialized",
		"signer_store", signerStoreType,
		"transaction_verifier", l4wardenType)
}

func (rs *OperatorPubSubService) buildHandlers() {
	rs.handlers = map[constants.EventType]func(context.Context, *PubSubCommandMessage){
		constants.Event.Operator.HeartbeatRequested:         rs.heartbeat.HandleRequest,
		constants.Event.Operator.Heartbeat:                  rs.handleHeartbeatEvent,
		constants.Event.Operator.Command.Requested:          rs.commands.HandleExecutionRequest,
		constants.Event.Operator.Command.CancelRequested:    rs.commands.HandleCancelRequest,
		constants.Event.Operator.FileEdit.Requested:         rs.fileOps.HandleFileEditRequest,
		constants.Event.Operator.FsList.Requested:           rs.fileOps.HandleFsListRequest,
		constants.Event.Operator.FsRead.Requested:           rs.fileOps.HandleFsReadRequest,
		constants.Event.Operator.FsGrep.Requested:           rs.fileOps.HandleFsGrepRequest,
		constants.Event.Operator.PortCheck.Requested:        rs.ports.HandlePortCheckRequest,
		constants.Event.Operator.FetchLogs.Requested:        rs.history.HandleFetchLogsRequest,
		constants.Event.Operator.FetchHistory.Requested:     rs.history.HandleFetchHistoryRequest,
		constants.Event.Operator.FetchFileHistory.Requested: rs.history.HandleFetchFileHistoryRequest,
		constants.Event.Operator.RestoreFile.Requested:      rs.history.HandleRestoreFileRequest,
		constants.Event.Operator.ShutdownRequested:          func(ctx context.Context, msg *PubSubCommandMessage) { rs.handleShutdownRequest(msg) },
		constants.Event.Operator.Eval.AnswerRequested:       rs.handleEvalAnswerRequest,
		constants.Event.Operator.Audit.UserMsg:              func(ctx context.Context, msg *PubSubCommandMessage) { _ = rs.audit.HandleUserMsgRequest(ctx, msg) },
		constants.Event.Operator.Audit.AIMsg:                func(ctx context.Context, msg *PubSubCommandMessage) { _ = rs.audit.HandleAIMsgRequest(ctx, msg) },
		constants.Event.Operator.Audit.DirectCmd:            func(ctx context.Context, msg *PubSubCommandMessage) { _ = rs.audit.HandleDirectCmdRequest(ctx, msg) },
		constants.Event.Operator.Audit.DirectCmdResult: func(ctx context.Context, msg *PubSubCommandMessage) {
			_ = rs.audit.HandleDirectCmdResultRequest(ctx, msg)
		},
		constants.Event.Operator.FetchFileDiff.Requested: rs.history.HandleFetchFileDiffRequest,
		constants.Event.Operator.Mcp.CallRequested: func(ctx context.Context, msg *PubSubCommandMessage) {
			if _, err := rs.handleMcpCallRequestSync(ctx, msg); err != nil {
				rs.logger.Error("MCP call request handler failed", "error", err)
			}
		},
		constants.Event.Operator.A2a.CallRequested: func(ctx context.Context, msg *PubSubCommandMessage) {
			if _, err := rs.handleA2aCallRequestSync(ctx, msg); err != nil {
				rs.logger.Error("A2A call request handler failed", "error", err)
			}
		},
		constants.EventAppInvestigationCreated: func(ctx context.Context, msg *PubSubCommandMessage) {
			if _, err := rs.handleAppInvestigationCreatedSync(ctx, msg); err != nil {
				rs.logger.Error("App investigation creation handler failed", "error", err)
			}
		},
	}
}

func (rs *OperatorPubSubService) Start(ctx context.Context) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.running {
		return constants.ErrGatewayAlreadyRunning
	}

	rs.ctx, rs.cancel = context.WithCancel(ctx)
	rs.running = true

	rs.heartbeat.ctx = rs.ctx

	channelName := CmdChannel(rs.config.OperatorID, rs.config.OperatorSessionId)

	// Only subscribe to pub/sub channel when running as a traditional Operator (with identity)
	// In gateway mode, commands arrive via HTTP/WebSocket endpoints directly
	if rs.config.OperatorID != "" && rs.config.OperatorSessionId != "" {
		rs.logger.Info("Command service subscribing to Operator channel",
			"operator_id", rs.config.OperatorID,
			"operator_session_id", rs.config.OperatorSessionId,
			"cmd_channel", channelName)

		rs.wg.Add(1)
		go func() {
			defer rs.wg.Done()
			rs.listenForCommands(channelName)
		}()
	} else {
		rs.logger.Info("Command service starting in Gateway mode (no pub/sub subscription)",
			"mode", string(constants.GatewayModeGateway))
	}

	rs.heartbeat.StartSchedulerUnlocked()

	rs.logger.Info("Command service ready")
	return nil
}

func (rs *OperatorPubSubService) Stop() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if !rs.running {
		return nil
	}

	rs.logger.Info("Command service shutting down...")
	rs.heartbeat.StopSchedulerUnlocked()

	if rs.cancel != nil {
		rs.cancel()
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		rs.wg.Wait()
	}()

	select {
	case <-shutdownDone:
		rs.logger.Info("Command service stopped gracefully")
	case <-time.After(10 * time.Second):
		rs.logger.Error("Command service shutdown timeout")
	}

	if rs.client != nil {
		rs.client.Close()
	}

	rs.running = false
	return nil
}

func (rs *OperatorPubSubService) listenForCommands(channelName string) {
	const maxReconnectAttempts = 3

	reconnectDelay := rs.reconnectBaseDelay
	maxReconnectDelay := 30 * rs.reconnectBaseDelay
	attempts := 0

	for {
		select {
		case <-rs.ctx.Done():
			rs.logger.Info("Command gateway stopped (context cancelled)")
			return
		default:
		}

		if rs.client == nil {
			rs.logger.Error("[RECONNECT] No Operator pub/sub client configured")
			return
		}

		rs.logger.Info("Subscribing to Operator command channel",
			"operator_session_id", rs.config.OperatorSessionId,
			"channel_name", channelName)

		msgCh, err := rs.client.Subscribe(rs.ctx, channelName)
		if err != nil {
			if err == context.Canceled {
				rs.logger.Info("Command gateway stopped (context cancelled during connection)")
				return
			}
			if IsTLSCertError(err) {
				rs.logger.Error("Server certificate verification failed during reconnect - this Node binary has outdated certificates",
					"action", "SSL Failure. Requesting shutdown.",
					"resolution", "download a new Node binary from https://"+constants.DefaultEndpoint)
				rs.ShutdownChan <- "SSL_CERT_FAILURE"
				return
			}
			attempts++
			if shouldGiveUp(attempts, maxReconnectAttempts) {
				rs.logger.Error("[RECONNECT] Max reconnection attempts reached, giving up",
					"attempts", attempts, string(constants.ConnectionStateError), err)
				return
			}
			rs.logger.Warn("[RECONNECT] Failed to connect, will retry...",
				"attempt", attempts, "max", maxReconnectAttempts, string(constants.ConnectionStateError), err)
			time.Sleep(reconnectDelay)
			reconnectDelay = nextReconnectDelay(reconnectDelay, maxReconnectDelay)
			continue
		}

		rs.logger.Info("Channel established - Ready to receive")
		reconnectDelay = rs.reconnectBaseDelay
		attempts = 0

		if err := rs.heartbeat.SendAutomatic(); err != nil {
			rs.logger.Error("Failed to send automatic heartbeat", "error", err)
		}

		disconnected := false
		receivedMessage := false

		for !disconnected {
			select {
			case <-rs.ctx.Done():
				rs.logger.Info("Command gateway stopped")
				return
			case payload, ok := <-msgCh:
				if !ok {
					attempts++
					if shouldGiveUp(attempts, maxReconnectAttempts) {
						rs.logger.Error("[RECONNECT] Channel closed repeatedly, max attempts reached - giving up",
							"attempts", attempts)
						return
					}
					rs.logger.Warn("[RECONNECT] Channel closed, reconnecting...",
						"attempt", attempts, "max", maxReconnectAttempts)
					disconnected = true
					break
				}
				if !receivedMessage {
					receivedMessage = true
					attempts = 0
				}
				rs.wg.Add(1)
				go func(p []byte) {
					defer rs.wg.Done()
					rs.handleCommandPayload(p)
				}(payload)
			}
		}

		rs.logger.Info("[RECONNECT] Waiting before reconnection attempt...", "delay_seconds", reconnectDelay.Seconds())
		time.Sleep(reconnectDelay)
		reconnectDelay = nextReconnectDelay(reconnectDelay, maxReconnectDelay)
	}
}

// nextReconnectDelay doubles the current delay, capped at max. This implements
// exponential backoff for the reconnect loop.
func nextReconnectDelay(current, max time.Duration) time.Duration {
	return min(current*2, max)
}

// shouldGiveUp returns true when the attempt count has reached or exceeded the
// maximum allowed reconnect attempts.
func shouldGiveUp(attempts, maxAttempts int) bool {
	return attempts >= maxAttempts
}

func (rs *OperatorPubSubService) handleCommandPayload(payload []byte) {
	rs.logger.Info("Received message from g8e",
		"operator_session_id", rs.config.OperatorSessionId,
		"payload_size", len(payload))

	if len(payload) > MaxPayloadSize {
		rs.logger.Error("Command payload exceeds maximum size limit",
			"size", len(payload),
			"limit", MaxPayloadSize)
		return
	}

	// Decode as GovernanceEnvelope - this is the only canonical mutation transport.
	//Node Node Binary protobuf bytes and other formats are explicitly rejected.
	envelope := &govpkg.GovernanceEnvelope{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(payload, envelope); err != nil {
		rs.logger.Error("envelope: non-JSON payload rejected",
			string(constants.ConnectionStateError), err,
			"action", "use canonical JSON (protojson) GovernanceEnvelope")
		return
	}

	rs.logger.Info("Decoded request as GovernanceEnvelope",
		"message_id", envelope.Id,
		"protocol_version", envelope.ProtocolVersion)
	rs.handleGovernanceEnvelope(envelope)
}

// ProcessEnvelope is the public, synchronous entry point for fail-closed
// Gateway transaction processing. It is used by the listen-mode HTTP surface
// (POST /api/v1/governance/envelopes) to verify a GovernanceEnvelope and execute it
// through the Actuator, returning the signed ActionReceipt or a verification
// error.
//
// The receipt is returned even on execution failure (status=FAILED) so callers
// receive cryptographic evidence of the attempt. A nil receipt is only
// returned when verification fails before execution begins, in which case the
// returned error wraps the corresponding governance.ErrXxx sentinel.
func (rs *OperatorPubSubService) ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error) {
	if len(payload) == 0 {
		return nil, constants.ErrPubSubEmptyPayload
	}
	if len(payload) > MaxPayloadSize {
		return nil, fmt.Errorf("payload exceeds %d byte limit: %w", MaxPayloadSize, constants.ErrPayloadExceedsLimit)
	}

	envelope := &govpkg.GovernanceEnvelope{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(payload, envelope); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrTxInvalidEnvelope, err)
	}

	if rs.l4warden == nil {
		return nil, constants.ErrPubSubTransactionVerifier
	}
	verified, err := rs.l4warden.VerifyEnvelope(ctx, envelope)
	if err != nil {
		rs.logBlockedTransaction(envelope, err)
		return nil, err
	}

	if rs.actuator == nil {
		return nil, constants.ErrPubSubActuator
	}

	eventType := constants.MapActionTypeToEventType(verified.ActionType)
	cmdMsg := &PubSubCommandMessage{
		ID:                envelope.Id,
		EventType:         eventType,
		CaseID:            envelope.CaseId,
		TaskID:            &envelope.TaskId,
		InvestigationID:   envelope.InvestigationId,
		WebSessionID:      envelope.WebSessionId,
		CLISessionID:      envelope.CliSessionId,
		OperatorSessionID: envelope.OperatorSessionId,
		OperatorID:        &envelope.OperatorId,
		Payload:           envelope.Payload,
		Timestamp:         envelope.Timestamp.AsTime(),
	}

	receipt, execErr := rs.actuator.Execute(ctx, verified, cmdMsg)
	return receipt, execErr
}

// handleGovernanceEnvelope processes a GovernanceEnvelope using the TransactionVerifier, Consensus and Actuator services.
func (rs *OperatorPubSubService) handleGovernanceEnvelope(env *govpkg.GovernanceEnvelope) {
	var verified *governance.VerifiedTransaction

	// Strict transaction verification (P0: fail-closed gate before any dispatch)
	if rs.l4warden != nil {
		var err error
		verified, err = rs.l4warden.VerifyEnvelope(context.Background(), env)
		if err != nil {
			rs.logger.Error("Transaction verification failed - command rejected",
				string(constants.ConnectionStateError), err,
				"message_id", env.Id)
			// Log blocked transaction to audit store
			rs.logBlockedTransaction(env, err)
			return
		}
		rs.logger.Info("Transaction verification passed", "message_id", verified.Envelope.Id)
	} else {
		rs.logger.Error("FATAL: L4Warden missing - command rejected", "message_id", env.Id)
		rs.logBlockedTransaction(env, constants.ErrPubSubL4Warden)
		return
	}

	// Convert GovernanceEnvelope to PubSubCommandMessage for execution through Actuator
	// Map GovernanceEnvelope action types back to protobuf event types for handler dispatch
	eventType := constants.MapActionTypeToEventType(verified.ActionType)

	payload := env.Payload
	if len(payload) == 0 {
		rs.logger.Error("GovernanceEnvelope missing required binary Payload bytes - request rejected", "message_id", env.Id)
		return
	}

	cmdMsg := &PubSubCommandMessage{
		ID:                env.Id,
		EventType:         eventType,
		CaseID:            env.CaseId,
		TaskID:            &env.TaskId,
		InvestigationID:   env.InvestigationId,
		WebSessionID:      env.WebSessionId,
		CLISessionID:      env.CliSessionId,
		OperatorSessionID: env.OperatorSessionId,
		OperatorID:        &env.OperatorId,
		Payload:           payload,
		Timestamp:         env.Timestamp.AsTime(),
	}

	// Execute through Actuator (execution boundary)
	if rs.actuator != nil {
		receipt, err := rs.actuator.Execute(rs.ctx, verified, cmdMsg)
		if err != nil {
			rs.logger.Error("Actuator execution failed",
				string(constants.ConnectionStateError), err,
				"message_id", env.Id,
				"receipt_status", receipt.Status.String())
			return
		}
		rs.logger.Info("Actuator execution succeeded",
			"message_id", env.Id,
			"receipt_status", receipt.Status.String())
	} else {
		rs.logger.Error("FATAL: Actuator service missing - cannot execute", "message_id", env.Id)
		return
	}
}

// Actuator returns the current L5 actuator.
func (rs *OperatorPubSubService) Actuator() *governance.L5Actuator {
	return rs.actuator
}

// SetActuator sets the L5 actuator (used for testing).
func (rs *OperatorPubSubService) SetActuator(a *governance.L5Actuator) {
	rs.actuator = a
}

// SetL4Warden sets the L4 warden (used for testing).
func (rs *OperatorPubSubService) SetL4Warden(w *governance.L4Warden) {
	rs.l4warden = w
}

// HeartbeatService returns the heartbeat service for external components
// (e.g. the Lattice adapter) to register periodic sink callbacks.
func (rs *OperatorPubSubService) HeartbeatService() *HeartbeatService {
	return rs.heartbeat
}

// ExecuteVerifiedTransaction implements governance.ExecutionHandler.
// This is called by Actuator to execute verified transactions, making Actuator the execution boundary.
func (rs *OperatorPubSubService) ExecuteVerifiedTransaction(ctx context.Context, eventType constants.EventType, cmdMsg governance.CommandMessage) (string, error) {
	handler, ok := rs.handlers[eventType]
	if !ok {
		rs.logger.Error("No handler registered for event type", "event_type", string(eventType))
		return "", fmt.Errorf("no handler for event type %s: %w", string(eventType), constants.ErrTxUnknownActionType)
	}

	pubsubMsg, ok := cmdMsg.(*PubSubCommandMessage)
	if !ok {
		rs.logger.Error("Invalid cmdMsg type", "expected", "*PubSubCommandMessage", "got", fmt.Sprintf("%T", cmdMsg))
		return "", fmt.Errorf("invalid cmdMsg type %T: %w", cmdMsg, constants.ErrTxPayloadDecodeFailed)
	}
	rs.logger.Info("Executing verified transaction through Actuator", "event_type", eventType)

	// Special case for EVAL_ANSWER which is synchronous and returns the answer as summary
	if eventType == constants.Event.Operator.Eval.AnswerRequested {
		return rs.handleEvalAnswerRequestSync(ctx, pubsubMsg)
	}

	// MCP_CALL must return the downstream MCP server's result text as the
	// receipt summary so the gateway can forward it back to the client.
	if eventType == constants.Event.Operator.Mcp.CallRequested {
		return rs.handleMcpCallRequestSync(ctx, pubsubMsg)
	}
	if eventType == constants.Event.Operator.A2a.CallRequested {
		return rs.handleA2aCallRequestSync(ctx, pubsubMsg)
	}

	handler(ctx, pubsubMsg)
	return "", nil
}

// handleMcpCallRequestSync is the Actuator egress for MCP_CALL transactions:
// it decodes the typed payload, dispatches to the configured downstream MCP
// server via the gateway, and returns the textual result so the Actuator can
// stamp it into the signed ActionReceipt.
func (rs *OperatorPubSubService) handleMcpCallRequestSync(ctx context.Context, msg *PubSubCommandMessage) (string, error) {
	if rs.mcpGateway == nil {
		return "", constants.ErrPubSubMCPGateway
	}

	req, err := unmarshalPayload(msg.EventType, msg.Payload)
	if err != nil {
		rs.logger.Error("Failed to unmarshal MCP call payload", string(constants.ConnectionStateError), err)
		return "", err
	}
	mcpReq, ok := req.(*operatorv1.McpCallRequested)
	if !ok {
		return "", fmt.Errorf("invalid payload type for MCP call %T: %w", req, constants.ErrTxPayloadActionMismatch)
	}
	if mcpReq.ToolName == "" {
		return "", constants.ErrPubSubMCPMissingToolName
	}

	args := json.RawMessage(mcpReq.ArgumentsJson)
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}

	rs.logger.Info("Dispatching verified MCP call to downstream",
		"tool", mcpReq.ToolName,
		"transaction_id", msg.ID)

	summary, err := rs.mcpGateway.DispatchToDownstream(ctx, mcpReq.ToolName, args, msg.OperatorSessionID)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrGatewayDownstreamHTTPError, err)
	}
	// Bound the receipt summary to avoid unbounded growth on chatty tools.
	if len(summary) > constants.ReceiptSummaryMaxBytes {
		summary = summary[:constants.ReceiptSummaryMaxBytes]
	}

	return summary, nil
}

// handleA2aCallRequestSync is the Actuator egress for A2A_CALL transactions:
// it decodes the typed payload, dispatches to the configured downstream A2A
// server via the gateway, and returns the textual result so the Actuator can
// stamp it into the signed ActionReceipt.
func (rs *OperatorPubSubService) handleA2aCallRequestSync(ctx context.Context, msg *PubSubCommandMessage) (string, error) {
	if rs.mcpGateway == nil {
		return "", constants.ErrPubSubA2AGateway
	}

	req, err := unmarshalPayload(msg.EventType, msg.Payload)
	if err != nil {
		rs.logger.Error("Failed to unmarshal A2A call payload", string(constants.ConnectionStateError), err)
		return "", err
	}
	a2aReq, ok := req.(*operatorv1.A2ACallRequested)
	if !ok {
		return "", fmt.Errorf("invalid payload type for A2A call %T: %w", req, constants.ErrTxPayloadActionMismatch)
	}
	if a2aReq.SkillName == "" {
		return "", constants.ErrPubSubA2AMissingSkillName
	}

	payload := json.RawMessage(a2aReq.PayloadJson)
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}

	rs.logger.Info("Dispatching verified A2A call to downstream",
		"skill", a2aReq.SkillName,
		"transaction_id", msg.ID)

	summary, err := rs.mcpGateway.DispatchToA2ADownstream(ctx, a2aReq.SkillName, payload)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrGatewayDownstreamHTTPError, err)
	}
	// Bound the receipt summary to avoid unbounded growth on chatty tools.
	if len(summary) > constants.ReceiptSummaryMaxBytes {
		summary = summary[:constants.ReceiptSummaryMaxBytes]
	}
	return summary, nil
}

func (rs *OperatorPubSubService) handleAppInvestigationCreatedSync(ctx context.Context, msg *PubSubCommandMessage) (string, error) {
	rs.logger.Info("App investigation creation request received", "investigation_id", msg.ID)

	if rs.actuator == nil || rs.actuator.ConsoleAuditStore == nil {
		return "", constants.ErrPubSubActuatorOrAuditStore
	}

	// DocSet expects collection, id, and data.
	// For APP_INVESTIGATION_CREATED, the ID is the investigation ID from the envelope.
	if err := rs.actuator.ConsoleAuditStore.DocSet(string(constants.CollectionInvestigations), msg.ID, msg.Payload); err != nil {
		rs.logger.Error("Failed to create investigation document", string(constants.ConnectionStateError), err, "investigation_id", msg.ID)
		return "", fmt.Errorf("%w: %w", constants.ErrAuditRecordUserMsg, err)
	}

	rs.logger.Info("Investigation document created successfully", "investigation_id", msg.ID)
	return "investigation created", nil
}

func (rs *OperatorPubSubService) handleShutdownRequest(msg *PubSubCommandMessage) {
	rs.logger.Info("Shutdown command received")

	req, err := unmarshalPayload(msg.EventType, msg.Payload)
	if err != nil {
		rs.logger.Error("Failed to unmarshal shutdown request", string(constants.ConnectionStateError), err)
		return
	}

	shutdownReq, ok := req.(*operatorv1.ShutdownRequested)
	if !ok {
		rs.logger.Error("Invalid payload type for shutdown request", "got", fmt.Sprintf("%T", req))
		return
	}

	reason := shutdownReq.Reason
	if reason == "" {
		reason = "No reason provided"
	}
	rs.logger.Info("Shutting down Operator", "reason", reason)
	rs.ShutdownChan <- reason
}

func (rs *OperatorPubSubService) handleEvalAnswerRequest(ctx context.Context, msg *PubSubCommandMessage) {
	_, _ = rs.handleEvalAnswerRequestSync(ctx, msg)
}

func (rs *OperatorPubSubService) handleEvalAnswerRequestSync(ctx context.Context, msg *PubSubCommandMessage) (string, error) {
	rs.logger.Info("Eval answer request received")

	req, err := unmarshalPayload(msg.EventType, msg.Payload)
	if err != nil {
		rs.logger.Error("Failed to unmarshal eval answer request", string(constants.ConnectionStateError), err)
		return "", err
	}

	evalReq, ok := req.(*operatorv1.EvalAnswerRequested)
	if !ok {
		rs.logger.Error("Invalid payload type for eval answer request", "got", fmt.Sprintf("%T", req))
		return "", fmt.Errorf("invalid payload type %T: %w", req, constants.ErrTxPayloadActionMismatch)
	}

	rs.logger.Info("Eval answer recorded",
		"prompt_id", evalReq.PromptId,
		"benchmark", evalReq.Benchmark,
		"model", evalReq.Model,
		"answer_length", len(evalReq.Answer))

	// Truncate answer to sane bound for receipt (4 KiB per plan)
	summary := evalReq.Answer
	if len(summary) > constants.ReceiptSummaryMaxBytes {
		summary = summary[:constants.ReceiptSummaryMaxBytes]
	}

	return summary, nil
}

// handleHeartbeatEvent processes a heartbeat event through the heartbeat service for publication.
func (rs *OperatorPubSubService) handleHeartbeatEvent(ctx context.Context, msg *PubSubCommandMessage) {
	var heartbeat operatorv1.HeartbeatResult
	if err := proto.Unmarshal(msg.Payload, &heartbeat); err != nil {
		rs.logger.Error("Failed to unmarshal heartbeat payload", "error", err)
		return
	}
	if err := rs.heartbeat.Publish(ctx, &heartbeat); err != nil {
		rs.logger.Error("Failed to publish heartbeat", "error", err)
	}
}

// SendAutomaticHeartbeat publishes an automatic heartbeat immediately.
func (rs *OperatorPubSubService) SendAutomaticHeartbeat() error {
	return rs.heartbeat.SendAutomatic()
}

// pubsubAuditLogger implements mcp.AuditLogger using the SQLAuditStore so that
// read_field tool calls produce audit records in operator mode.
type pubsubAuditLogger struct {
	store  AuditEventRecorder
	logger *slog.Logger
}

func (l *pubsubAuditLogger) LogFieldRead(operatorSessionID, collection, documentID, fieldPath string, value mcp.FieldValue) error {
	event := &storage.Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              constants.EventOperatorFieldReadRequested,
		ContentText:       fmt.Sprintf("%s/%s.%s", collection, documentID, fieldPath),
		CommandStdout:     value.String(),
	}
	if _, err := l.store.RecordEvent(event); err != nil {
		l.logger.Warn("Failed to record field read in audit store", "error", err,
			"session", operatorSessionID, "collection", collection, "field", fieldPath)
		return err
	}
	return nil
}

// ValidateSession implements mcp.SessionValidator for operator mode.
// In operator mode, session validation is handled by the L3Notary during
// envelope verification, so this is a no-op that always returns true.
func (rs *OperatorPubSubService) ValidateSession(operatorSessionID string) (bool, error) {
	// Operator mode validates sessions via L3Notary during envelope verification
	// This is a placeholder for the SessionValidator interface
	return true, nil
}

// logBlockedTransaction records a blocked/rejected transaction using the ActionReceiptRecord schema.
// This ensures consistency with accepted/failed Actuator receipts - all transaction outcomes use the same canonical schema.
func (rs *OperatorPubSubService) logBlockedTransaction(env *govpkg.GovernanceEnvelope, rejectionReason error) {
	if rs.audit == nil || rs.audit.auditStore == nil {
		return
	}

	// Create ActionReceiptRecord with BLOCKED status for canonical audit trail
	record := models.ActionReceiptRecord{
		TransactionID:     env.Id,
		TransactionHash:   env.TransactionHash,
		OperatorID:        env.OperatorId,
		OperatorSessionID: env.OperatorSessionId,
		ActionType:        constants.ActionType(env.ActionType),
		TargetResource:    env.TargetResource,
		Status:            operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
		ResultSummary:     fmt.Sprintf("blocked: %v", rejectionReason),
		StateRootBefore:   "", // Not available for blocked transactions
		StateRootAfter:    "", // Not available for blocked transactions
		ExecutedAt:        time.Now().UTC(),
		SignerKeyID:       "", // No Actuator signature for blocked transactions
		Signature:         "", // No signature for blocked transactions
		Timestamp:         time.Now().UTC(),
	}

	// Log to audit store using canonical RecordActionReceipt for unified query experience
	if err := rs.audit.auditStore.RecordActionReceipt(&record); err != nil {
		rs.logger.Error("Failed to record blocked transaction in audit store", string(constants.ConnectionStateError), err, "message_id", env.Id)
	}
}
func (m *PubSubCommandMessage) GetPayload() []byte {
	return []byte(m.Payload)
}

func (m *PubSubCommandMessage) SetPayload(p []byte) {
	m.Payload = p
}
