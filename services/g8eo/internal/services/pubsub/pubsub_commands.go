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
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/config"
	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	commonv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	execution "github.com/g8e-ai/g8e/services/g8eo/internal/services/execution"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/governance"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/sentinel"
	storage "github.com/g8e-ai/g8e/services/g8eo/internal/services/storage"
	"github.com/g8e-ai/g8e/services/g8eo/pkg/uap"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// PubSubCommandMessage is the inbound wire message received from operator pub/sub.
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

// PubSubCommandService manages the operator pub/sub connection and dispatches inbound
// operator commands to the appropriate first-class service handler.
type PubSubCommandService struct {
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

	handlers map[constants.EventType]func(context.Context, PubSubCommandMessage)

	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.RWMutex

	reconnectBaseDelay time.Duration

	// UAP governance services for Phase 3 integration
	tribunal            *governance.Tribunal
	warden              *governance.Warden
	transactionVerifier *governance.TransactionVerifier
	signerStore         governance.SignerStore
}

// CommandServiceConfig holds all dependencies for PubSubCommandService.
type CommandServiceConfig struct {
	Config            *config.Config
	Logger            *slog.Logger
	Execution         *execution.ExecutionService
	FileEdit          *execution.FileEditService
	PubSubClient      PubSubClient
	ResultsService    ResultsPublisher
	LocalStore        *storage.LocalStoreService
	RawVault          *storage.RawVaultService
	AuditVault        *storage.AuditVaultService
	Ledger            *storage.LedgerService
	HistoryHandler    *storage.HistoryHandler
	Sentinel          *sentinel.Sentinel
	L3Verifier        governance.L3Verifier
	ReplayStore       governance.ReplayStore
	StateRootProvider governance.StateRootProvider
	TransactionAudit  governance.TransactionAuditStore
	SignerStore       governance.SignerStore

	// Warden configuration
	WardenSigningKey ed25519.PrivateKey
	WardenKeyID      string
}

// NewPubSubCommandService creates the dispatcher and all first-class sub-services using the provided config.
func NewPubSubCommandService(c CommandServiceConfig) (*PubSubCommandService, error) {
	client := c.PubSubClient
	if client == nil && c.Config.PubSubURL != "" {
		var err error
		client, err = NewOperatorPubSubClient(c.Config.PubSubURL, c.Config.TLSServerName, c.Logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create operator pub/sub client: %w", err)
		}
	}

	serviceCtx, cancel := context.WithCancel(context.Background())

	rs := &PubSubCommandService{
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

	rs.commands = NewCommandService(c.Config, c.Logger, c.Execution)
	rs.commands.results = c.ResultsService
	rs.commands.sentinel = c.Sentinel
	rs.commands.vaultWriter = NewVaultWriter(c.Config, c.Logger, c.Sentinel, c.RawVault, c.LocalStore)
	rs.commands.auditVault = c.AuditVault
	rs.commands.localStore = c.LocalStore
	rs.commands.rawVault = c.RawVault
	rs.commands.ledger = c.Ledger
	rs.commands.historyHandler = c.HistoryHandler

	rs.fileOps = NewFileOpsService(c.Config, c.Logger, c.FileEdit, client)
	rs.fileOps.results = c.ResultsService
	rs.fileOps.sentinel = c.Sentinel
	rs.fileOps.vaultWriter = NewVaultWriter(c.Config, c.Logger, c.Sentinel, c.RawVault, c.LocalStore)
	rs.fileOps.auditVault = c.AuditVault
	rs.fileOps.ledger = c.Ledger

	rs.ports = NewPortService(c.Config, c.Logger, client)

	rs.audit = NewAuditService(c.Config, c.Logger)
	rs.audit.auditVault = c.AuditVault

	rs.history = NewHistoryService(c.Config, c.Logger, client)
	rs.history.localStore = c.LocalStore
	rs.history.rawVault = c.RawVault
	rs.history.historyHandler = c.HistoryHandler

	rs.buildHandlers()

	rs.signerStore = c.SignerStore
	if rs.signerStore == nil {
		// Provide a fallback empty signer store instead of loading from filesystem.
		// This ensures outbound mode fails closed if no signer store is provided.
		rs.signerStore = &governance.SimpleSignerStore{Signers: make(map[string]ed25519.PublicKey)}
		c.Logger.Warn("No SignerStore provided; signed transactions will be rejected")
	}

	// Validate required governance dependencies (fail-closed: missing deps = fatal error)
	if c.ReplayStore == nil {
		return nil, fmt.Errorf("ReplayStore is required for transaction verification")
	}
	if c.StateRootProvider == nil {
		return nil, fmt.Errorf("StateRootProvider is required for transaction verification")
	}
	// L3Verifier is optional for outbound mode (platform verifies L3)
	// Mutations requiring L3 will fail-closed at TransactionVerifier if L3Verifier is nil

	// Initialize UAP governance services after trusted signers are loaded
	rs.initializeUAPGovernance(c, serviceCtx)

	c.Logger.Info("g8e connectivity initialized",
		"config_operator_id", c.Config.OperatorID,
		"config_operator_session_id", c.Config.OperatorSessionId)
	return rs, nil
}

func (rs *PubSubCommandService) initializeUAPGovernance(c CommandServiceConfig, serviceCtx context.Context) {
	// Initialize Tribunal with Sentinel for MITRE checks
	rs.tribunal = &governance.Tribunal{
		NodeID:   c.Config.OperatorID,
		Sentinel: c.Sentinel,
		// PrivateKey would be loaded from PKI directory in production
	}

	// Initialize Warden with trusted nodes and audit vault
	rs.warden = &governance.Warden{
		Logger:            c.Logger,
		SignerStore:       rs.signerStore,
		Execution:         c.Execution,
		AuditVault:        c.AuditVault,
		AuditStore:        c.TransactionAudit,
		L3Verifier:        c.L3Verifier,
		StateRootProvider: c.StateRootProvider,
		Ctx:               serviceCtx,
		ExecutionHandler:  rs, // PubSubCommandService implements ExecutionHandler
		SigningKey:        c.WardenSigningKey,
		KeyID:             c.WardenKeyID,
	}

	// Initialize TransactionVerifier for strict pre-dispatch verification
	knownActionTypes := []constants.ActionType{
		constants.ActionTypeExecuteBash,
		constants.ActionTypeFileEdit,
		constants.ActionTypeRestoreFile,
		constants.ActionTypeShutdown,
		constants.ActionTypeFsList,
		constants.ActionTypeFsRead,
		constants.ActionTypeFsGrep,
		constants.ActionTypePortCheck,
		constants.ActionTypeFetchLogs,
		constants.ActionTypeEvalAnswer,
	}
	rs.transactionVerifier = governance.NewTransactionVerifier(
		c.Logger,
		c.ReplayStore,
		c.StateRootProvider,
		rs.signerStore,
		c.L3Verifier,
		knownActionTypes,
	)

	c.Logger.Info("UAP governance services initialized",
		"tribunal_node_id", c.Config.OperatorID,
		"signer_store_configured", rs.signerStore != nil,
		"transaction_verifier_enabled", rs.transactionVerifier != nil)
}

func (rs *PubSubCommandService) buildHandlers() {
	rs.handlers = map[constants.EventType]func(context.Context, PubSubCommandMessage){
		constants.Event.Operator.HeartbeatRequested:         rs.heartbeat.HandleRequest,
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
		constants.Event.Operator.ShutdownRequested:          func(ctx context.Context, msg PubSubCommandMessage) { rs.handleShutdownRequest(msg) },
		constants.Event.Operator.Eval.AnswerRequested:       rs.handleEvalAnswerRequest,
		constants.Event.Operator.Audit.UserMsg:              rs.audit.HandleUserMsgRequest,
		constants.Event.Operator.Audit.AIMsg:                rs.audit.HandleAIMsgRequest,
		constants.Event.Operator.Audit.DirectCmd:            rs.audit.HandleDirectCmdRequest,
		constants.Event.Operator.Audit.DirectCmdResult:      rs.audit.HandleDirectCmdResultRequest,
		constants.Event.Operator.FetchFileDiff.Requested:    rs.history.HandleFetchFileDiffRequest,
	}
}

func (rs *PubSubCommandService) Start(ctx context.Context) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.running {
		return fmt.Errorf("command service is already running")
	}

	rs.ctx, rs.cancel = context.WithCancel(ctx)
	rs.running = true

	rs.heartbeat.ctx = rs.ctx

	channelName := constants.CmdChannel(rs.config.OperatorID, rs.config.OperatorSessionId)

	// Only subscribe to pub/sub channel when running as a traditional operator (with identity)
	// In listen mode, commands arrive via HTTP/WebSocket endpoints directly
	if rs.config.OperatorID != "" && rs.config.OperatorSessionId != "" {
		rs.logger.Info("Command service subscribing to operator channel",
			"operator_id", rs.config.OperatorID,
			"operator_session_id", rs.config.OperatorSessionId,
			"cmd_channel", channelName)

		rs.wg.Add(1)
		go func() {
			defer rs.wg.Done()
			rs.listenForCommands(channelName)
		}()
	} else {
		rs.logger.Info("Command service starting in substrate mode (no pub/sub subscription)",
			"mode", "listen")
	}

	rs.heartbeat.StartSchedulerUnlocked()

	rs.logger.Info("Command service ready")
	return nil
}

func (rs *PubSubCommandService) Stop() error {
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

func (rs *PubSubCommandService) listenForCommands(channelName string) {
	const maxReconnectAttempts = 3

	reconnectDelay := rs.reconnectBaseDelay
	maxReconnectDelay := 30 * rs.reconnectBaseDelay
	attempts := 0

	for {
		select {
		case <-rs.ctx.Done():
			rs.logger.Info("Command listener stopped (context cancelled)")
			return
		default:
		}

		if rs.client == nil {
			rs.logger.Error("[RECONNECT] No operator pub/sub client configured")
			return
		}

		rs.logger.Info("Subscribing to operator command channel",
			"operator_session_id", rs.config.OperatorSessionId,
			"channel_name", channelName)

		msgCh, err := rs.client.Subscribe(rs.ctx, channelName)
		if err != nil {
			if err == context.Canceled {
				rs.logger.Info("Command listener stopped (context cancelled during connection)")
				return
			}
			if IsTLSCertError(err) {
				rs.logger.Error("Server certificate verification failed during reconnect - this operator binary has outdated certificates",
					"action", "SSL Failure. Requesting shutdown.",
					"resolution", "download a new operator binary from https://"+constants.DefaultEndpoint)
				rs.ShutdownChan <- "SSL_CERT_FAILURE"
				return
			}
			attempts++
			if attempts >= maxReconnectAttempts {
				rs.logger.Error("[RECONNECT] Max reconnection attempts reached, giving up",
					"attempts", attempts, "error", err)
				return
			}
			rs.logger.Warn("[RECONNECT] Failed to connect, will retry...",
				"attempt", attempts, "max", maxReconnectAttempts, "error", err)
			time.Sleep(reconnectDelay)
			reconnectDelay = min(reconnectDelay*2, maxReconnectDelay)
			continue
		}

		rs.logger.Info("Channel established - Ready to receive")
		reconnectDelay = rs.reconnectBaseDelay

		rs.heartbeat.SendAutomatic()

		disconnected := false
		receivedMessage := false

		for !disconnected {
			select {
			case <-rs.ctx.Done():
				rs.logger.Info("Command listener stopped")
				return
			case payload, ok := <-msgCh:
				if !ok {
					attempts++
					if attempts >= maxReconnectAttempts {
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
		reconnectDelay = min(reconnectDelay*2, maxReconnectDelay)
	}
}

// HandleCommandData processes a typed command message from the Gateway transport.
func (rs *PubSubCommandService) HandleCommandData(msg *PubSubCommandMessage) {
	rs.logger.Info("Processing request (via Gateway)")
	rs.dispatchCommand(*msg)
}

func (rs *PubSubCommandService) handleCommandPayload(payload []byte) {
	rs.logger.Info("Received message from g8e",
		"operator_session_id", rs.config.OperatorSessionId,
		"payload_size", len(payload))

	if len(payload) > MaxPayloadSize {
		rs.logger.Error("Command payload exceeds maximum size limit",
			"size", len(payload),
			"limit", MaxPayloadSize)
		return
	}

	// Decode as UAP JSON envelope - this is the only canonical mutation transport.
	// Binary protobuf bytes and other formats are explicitly rejected.
	envelope := &uap.UAPEnvelope{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(payload, (*commonv1.GovernanceEnvelope)(envelope)); err != nil {
		rs.logger.Error("envelope: non-JSON payload rejected",
			"error", err,
			"action", "use canonical JSON (protojson) GovernanceEnvelope")
		return
	}

	rs.logger.Info("Decoded request as UAP JSON envelope",
		"message_id", envelope.Id,
		"protocol_version", envelope.ProtocolVersion)
	rs.handleUAPEnvelope((*uap.UAPEnvelope)(envelope))
}

// handleUAPEnvelope processes a UAPEnvelope using the TransactionVerifier, Tribunal and Warden services.
func (rs *PubSubCommandService) handleUAPEnvelope(env *uap.UAPEnvelope) {
	var verified *governance.VerifiedTransaction

	// Strict transaction verification (P0: fail-closed gate before any dispatch)
	if rs.transactionVerifier != nil {
		var err error
		verified, err = rs.transactionVerifier.VerifyEnvelope(env)
		if err != nil {
			rs.logger.Error("Transaction verification failed - command rejected",
				"error", err,
				"message_id", env.Id)
			// Log blocked transaction to audit vault
			rs.logBlockedTransaction(env, err)
			return
		}
		rs.logger.Info("Transaction verification passed", "message_id", verified.Envelope.Id)
	} else {
		rs.logger.Error("FATAL: TransactionVerifier missing - command rejected", "message_id", env.Id)
		rs.logBlockedTransaction(env, errors.New("TransactionVerifier not configured"))
		return
	}

	// Convert UAPEnvelope to PubSubCommandMessage for execution through Warden
	// Map UAP action types back to protobuf event types for handler dispatch
	eventType := constants.MapActionTypeToEventType(verified.ActionType)

	payload := env.Payload
	if len(payload) == 0 {
		rs.logger.Error("UAPEnvelope missing required binary Payload bytes - request rejected", "message_id", env.Id)
		return
	}

	cmdMsg := PubSubCommandMessage{
		ID:                env.Id,
		EventType:         constants.EventType(eventType),
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

	// Execute through Warden (execution boundary)
	if rs.warden != nil {
		receipt, err := rs.warden.Execute(rs.ctx, verified, cmdMsg)
		if err != nil {
			rs.logger.Error("Warden execution failed",
				"error", err,
				"message_id", env.Id,
				"receipt_status", receipt.Status.String())
			return
		}
		rs.logger.Info("Warden execution succeeded",
			"message_id", env.Id,
			"receipt_status", receipt.Status.String())
	} else {
		rs.logger.Error("FATAL: Warden service missing - cannot execute", "message_id", env.Id)
		return
	}
}

func (rs *PubSubCommandService) dispatchCommand(cmdMsg PubSubCommandMessage) {
	handler, ok := rs.handlers[cmdMsg.EventType]
	if !ok {
		rs.logger.Warn("Unknown request type", "event_type", cmdMsg.EventType)
		return
	}
	handler(rs.ctx, cmdMsg)
}

// ExecuteVerifiedTransaction implements governance.ExecutionHandler.
// This is called by Warden to execute verified transactions, making Warden the execution boundary.
func (rs *PubSubCommandService) ExecuteVerifiedTransaction(ctx context.Context, eventType constants.EventType, cmdMsg interface{}) (string, error) {
	handler, ok := rs.handlers[eventType]
	if !ok {
		rs.logger.Error("No handler registered for event type", "event_type", string(eventType))
		return "", fmt.Errorf("no handler for event type: %s", string(eventType))
	}

	// Type assert to PubSubCommandMessage
	pubsubMsg, ok := cmdMsg.(PubSubCommandMessage)
	if !ok {
		rs.logger.Error("Invalid cmdMsg type", "expected", "PubSubCommandMessage", "got", fmt.Sprintf("%T", cmdMsg))
		return "", fmt.Errorf("invalid cmdMsg type: %T", cmdMsg)
	}

	rs.logger.Info("Executing verified transaction through Warden", "event_type", eventType)

	// Special case for EVAL_ANSWER which is synchronous and returns the answer as summary
	if eventType == constants.Event.Operator.Eval.AnswerRequested {
		return rs.handleEvalAnswerRequestSync(ctx, pubsubMsg)
	}

	handler(ctx, pubsubMsg)
	return "", nil
}

func (rs *PubSubCommandService) handleShutdownRequest(msg PubSubCommandMessage) {
	rs.logger.Info("Shutdown command received")

	req, err := unmarshalPayload(msg.EventType, msg.Payload)
	if err != nil {
		rs.logger.Error("Failed to unmarshal shutdown request", "error", err)
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
	rs.logger.Info("Shutting down operator (UAP)", "reason", reason)
	rs.ShutdownChan <- reason
}

func (rs *PubSubCommandService) handleEvalAnswerRequest(ctx context.Context, msg PubSubCommandMessage) {
	_, _ = rs.handleEvalAnswerRequestSync(ctx, msg)
}

func (rs *PubSubCommandService) handleEvalAnswerRequestSync(ctx context.Context, msg PubSubCommandMessage) (string, error) {
	rs.logger.Info("Eval answer request received")

	req, err := unmarshalPayload(msg.EventType, msg.Payload)
	if err != nil {
		rs.logger.Error("Failed to unmarshal eval answer request", "error", err)
		return "", err
	}

	evalReq, ok := req.(*operatorv1.EvalAnswerRequested)
	if !ok {
		rs.logger.Error("Invalid payload type for eval answer request", "got", fmt.Sprintf("%T", req))
		return "", fmt.Errorf("invalid payload type: %T", req)
	}

	rs.logger.Info("Eval answer recorded",
		"prompt_id", evalReq.PromptId,
		"benchmark", evalReq.Benchmark,
		"model", evalReq.Model,
		"answer_length", len(evalReq.Answer))

	// Truncate answer to sane bound for receipt (4 KiB per plan)
	summary := evalReq.Answer
	if len(summary) > 4096 {
		summary = summary[:4096]
	}

	return summary, nil
}

// SendAutomaticHeartbeat publishes an automatic heartbeat immediately.
func (rs *PubSubCommandService) SendAutomaticHeartbeat() {
	rs.heartbeat.SendAutomatic()
}

// logBlockedTransaction records a blocked/rejected transaction using the ActionReceiptRecord schema.
// This ensures consistency with accepted/failed Warden receipts - all transaction outcomes use the same canonical schema.
func (rs *PubSubCommandService) logBlockedTransaction(env *uap.UAPEnvelope, rejectionReason error) {
	if rs.audit == nil || rs.audit.auditVault == nil {
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
		SignerKeyID:       "", // No Warden signature for blocked transactions
		Signature:         "", // No signature for blocked transactions
		Timestamp:         time.Now().UTC(),
	}

	// Log to audit vault using canonical RecordActionReceipt for unified query experience
	if err := rs.audit.auditVault.RecordActionReceipt(&record); err != nil {
		rs.logger.Error("Failed to record blocked transaction in audit vault", "error", err, "message_id", env.Id)
	}
}
