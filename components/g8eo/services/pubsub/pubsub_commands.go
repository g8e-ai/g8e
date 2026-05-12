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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	"github.com/g8e-ai/g8e/components/g8eo/services/governance"
	"github.com/g8e-ai/g8e/components/g8eo/services/sentinel"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
	"google.golang.org/protobuf/proto"
)

// PubSubCommandMessage is the inbound wire message received from operator pub/sub.
type PubSubCommandMessage struct {
	ID                string          `json:"id"`
	EventType         string          `json:"event_type"`
	CaseID            string          `json:"case_id"`
	TaskID            *string         `json:"task_id"`
	InvestigationID   string          `json:"investigation_id"`
	OperatorSessionID string          `json:"operator_session_id"`
	OperatorID        *string         `json:"operator_id"`
	Payload           json.RawMessage `json:"payload"`
	Timestamp         time.Time       `json:"timestamp"`
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

	handlers map[string]func(context.Context, PubSubCommandMessage)

	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.RWMutex

	reconnectBaseDelay time.Duration

	// auditorHMACKey is the shared Tribunal signing key used for L2
	// governance verification. Loaded once at startup from
	// <SecretsDir>/auditor_hmac_key. When absent the dispatcher logs a
	// warning and accepts envelopes without L2 enforcement (deployment
	// bootstrap phase). When present, all inbound envelopes are
	// strictly verified and rejected on signature mismatch.
	auditorHMACKey string

	// trustedSigners holds ED25519 public keys for external L2 signers.
	// Loaded from <PKIDir>/trusted_signers/*.pub
	trustedSigners map[string]ed25519.PublicKey

	// UAP governance services for Phase 3 integration
	tribunal *governance.Tribunal
	warden   *governance.Warden
}

// CommandServiceConfig holds all dependencies for PubSubCommandService.
type CommandServiceConfig struct {
	Config         *config.Config
	Logger         *slog.Logger
	Execution      *execution.ExecutionService
	FileEdit       *execution.FileEditService
	PubSubClient   PubSubClient
	ResultsService ResultsPublisher
	LocalStore     *storage.LocalStoreService
	RawVault       *storage.RawVaultService
	AuditVault     *storage.AuditVaultService
	Ledger         *storage.LedgerService
	HistoryHandler *storage.HistoryHandler
	Sentinel       *sentinel.Sentinel
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

	// Initialize trusted signers map
	rs.trustedSigners = make(map[string]ed25519.PublicKey)

	// Load L2 Tribunal HMAC key for governance verification
	auditorHMACKeyPath := filepath.Join(c.Config.SecretsDir, "auditor_hmac_key")
	keyBytes, err := os.ReadFile(auditorHMACKeyPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.Logger.Warn("L2 Tribunal HMAC key not found - L2 governance enforcement disabled (deployment bootstrap)",
				"path", auditorHMACKeyPath,
				"action", "commands will be accepted without L2 signature verification")
			rs.auditorHMACKey = ""
		} else {
			c.Logger.Error("Failed to read L2 Tribunal HMAC key",
				"path", auditorHMACKeyPath,
				"error", err)
			return nil, fmt.Errorf("failed to read auditor_hmac_key: %w", err)
		}
	} else {
		rs.auditorHMACKey = strings.TrimSpace(string(keyBytes))
		c.Logger.Info("L2 Tribunal HMAC key loaded - L2 governance enforcement enabled",
			"path", auditorHMACKeyPath)
	}

	// Load ED25519 trusted signers from <PKIDir>/trusted_signers/*.pub
	signersDir := filepath.Join(c.Config.PKIDir, "trusted_signers")
	if entries, err := os.ReadDir(signersDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".pub") {
				pubPath := filepath.Join(signersDir, entry.Name())
				if pubHex, err := os.ReadFile(pubPath); err == nil {
					pubBytes, err := hex.DecodeString(strings.TrimSpace(string(pubHex)))
					if err == nil && len(pubBytes) == ed25519.PublicKeySize {
						name := strings.TrimSuffix(entry.Name(), ".pub")
						rs.trustedSigners[name] = ed25519.PublicKey(pubBytes)
						c.Logger.Info("Loaded trusted L2 signer", "name", name, "path", pubPath)
					}
				}
			}
		}
	}

	// Initialize UAP governance services after trusted signers are loaded
	rs.initializeUAPGovernance(c)

	c.Logger.Info("g8e connectivity initialized",
		"config_operator_id", c.Config.OperatorID,
		"config_operator_session_id", c.Config.OperatorSessionId)
	return rs, nil
}

// initializeUAPGovernance sets up the Tribunal and Warden services for UAP Phase 3 integration.
func (rs *PubSubCommandService) initializeUAPGovernance(c CommandServiceConfig) {
	// Initialize Tribunal with Sentinel for MITRE checks
	rs.tribunal = &governance.Tribunal{
		NodeID:   c.Config.OperatorID,
		Sentinel: c.Sentinel,
		// PrivateKey would be loaded from PKI directory in production
	}

	// Initialize Warden with trusted nodes and audit vault
	rs.warden = &governance.Warden{
		Logger:       c.Logger,
		TrustedNodes: rs.trustedSigners,
		Execution:    c.Execution,
		AuditVault:   c.AuditVault,
	}

	c.Logger.Info("UAP governance services initialized",
		"tribunal_node_id", c.Config.OperatorID,
		"trusted_signers_count", len(rs.trustedSigners))
}

func (rs *PubSubCommandService) buildHandlers() {
	rs.handlers = map[string]func(context.Context, PubSubCommandMessage){
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
	rs.logger.Info("Establishing connection with g8ed",
		"operator_id", rs.config.OperatorID,
		"operator_session_id", rs.config.OperatorSessionId,
		"cmd_channel", channelName)

	rs.wg.Add(1)
	go func() {
		defer rs.wg.Done()
		rs.listenForCommands(channelName)
	}()

	rs.heartbeat.StartSchedulerUnlocked()

	rs.logger.Info("Connection established - Standing by")
	return nil
}

func (rs *PubSubCommandService) Stop() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if !rs.running {
		return nil
	}

	rs.logger.Info("Disconnecting from g8e...")
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
		rs.logger.Info("g8e connection closed gracefully")
	case <-time.After(10 * time.Second):
		rs.logger.Error("g8e disconnection timeout")
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

	// Phase 3: Try to parse as UAPEnvelope (JSON) first
	var uapEnv uap.UAPEnvelope
	if err := json.Unmarshal(payload, &uapEnv); err == nil {
		rs.logger.Info("Parsed request as UAPEnvelope (JSON)",
			"message_id", uapEnv.MessageID,
			"protocol_version", uapEnv.ProtocolVersion)
		rs.handleUAPEnvelope(&uapEnv)
		return
	}

	// Temporary migration path: Try to parse as GovernanceEnvelope (protobuf) and map to UAP
	// This allows incremental test migration during Phase 3 transition.
	var protoEnv commonv1.GovernanceEnvelope
	if err := proto.Unmarshal(payload, &protoEnv); err == nil {
		rs.logger.Warn("Parsed request as GovernanceEnvelope (protobuf) - migrating to UAP (temporary compatibility layer)",
			"id", protoEnv.Id,
			"action", "update tests to use UAPEnvelope JSON format")
		mappedEnv, caseID, investigationID, err := rs.mapGovernanceToUAP(&protoEnv)
		if err != nil {
			rs.logger.Error("Failed to map GovernanceEnvelope to UAP - command rejected",
				"error", err,
				"id", protoEnv.Id)
			return
		}
		rs.handleUAPEnvelopeWithIDs(mappedEnv, caseID, investigationID, protoEnv.TaskId)
		return
	}

	// Reject unknown envelope formats
	rs.logger.Error("Failed to parse as UAPEnvelope or GovernanceEnvelope - unknown format rejected",
		"error", "envelope_format_unknown",
		"action", "use UAPEnvelope JSON format")
	return
}

// handleUAPEnvelope processes a UAPEnvelope using the Tribunal and Warden services.
func (rs *PubSubCommandService) handleUAPEnvelope(env *uap.UAPEnvelope) {
	rs.handleUAPEnvelopeWithIDs(env, "", "", nil)
}

// handleUAPEnvelopeWithIDs processes a UAPEnvelope with application-layer IDs (CaseID, InvestigationID, TaskID).
// This is used during migration to preserve these IDs from GovernanceEnvelope.
func (rs *PubSubCommandService) handleUAPEnvelopeWithIDs(env *uap.UAPEnvelope, caseID string, investigationID string, taskID *string) {
	// Generate and verify MessageID (Tribunal hash verification)
	expectedID, err := env.GenerateMessageID()
	if err != nil {
		rs.logger.Error("Failed to generate MessageID for verification", "error", err)
		return
	}
	if env.MessageID != expectedID {
		rs.logger.Error("UAPEnvelope MessageID mismatch - payload hash verification failed",
			"expected_id", expectedID,
			"received_id", env.MessageID)
		return
	}
	rs.logger.Info("UAPEnvelope MessageID verified", "message_id", env.MessageID)

	// Tribunal evaluation (L1/L2 governance: hash verification + MITRE checks + voting)
	// During migration, governance enforcement is optional - fail-open if not configured
	if rs.tribunal != nil && rs.tribunal.PrivateKey != nil {
		if err := rs.tribunal.EvaluatePayload(env); err != nil {
			rs.logger.Error("Tribunal evaluation failed - command rejected",
				"error", err,
				"message_id", env.MessageID)
			return
		}
		rs.logger.Info("Tribunal evaluation passed", "message_id", env.MessageID)
	} else {
		rs.logger.Debug("Tribunal service not fully configured - skipping evaluation (migration mode)")
	}

	// Warden authorization (L3 governance: quorum enforcement + execution authorization)
	// During migration, governance enforcement is optional - fail-open if not configured
	if rs.warden != nil && len(rs.warden.TrustedNodes) > 0 {
		if err := rs.warden.AuthorizeExecution(env); err != nil {
			rs.logger.Error("Warden authorization failed - command rejected",
				"error", err,
				"message_id", env.MessageID)
			return
		}
		rs.logger.Info("Warden authorization passed", "message_id", env.MessageID)
	} else {
		rs.logger.Debug("Warden service not fully configured - skipping authorization (migration mode)")
	}

	// Convert UAPEnvelope to PubSubCommandMessage for dispatch
	// Map UAP action types back to protobuf event types for handler dispatch
	eventType := mapActionTypeToEventType(env.Intent.ActionType)

	cmdMsg := PubSubCommandMessage{
		ID:                env.MessageID,
		EventType:         eventType,
		CaseID:            caseID,
		TaskID:            taskID,
		InvestigationID:   investigationID,
		OperatorSessionID: env.Metadata.SenderID,
		OperatorID:        nil,
		Payload:           []byte(env.Context.DataBlob),
		Timestamp:         env.Metadata.Timestamp,
	}

	rs.dispatchCommand(cmdMsg)
}

// mapGovernanceToUAP maps a legacy GovernanceEnvelope to a UAPEnvelope for migration.
// This is a temporary compatibility layer during Phase 3 transition.
// Returns the UAPEnvelope and the original CaseID/InvestigationID for threading through dispatch.
func (rs *PubSubCommandService) mapGovernanceToUAP(protoEnv *commonv1.GovernanceEnvelope) (*uap.UAPEnvelope, string, string, error) {
	// Extract event type and payload for mapping
	eventType := protoEnv.EventType
	payload := protoEnv.Payload

	// Preserve application-layer IDs for threading through dispatch
	caseID := protoEnv.CaseId
	investigationID := protoEnv.InvestigationId

	// Map protobuf event types to UAP action types
	actionType := mapEventTypeToActionType(eventType)
	targetResource := "localhost" // Default target for migration

	// Encode application-layer IDs into DataBlob for preservation during UAP processing
	// This is a temporary workaround until UAP envelope supports these fields natively
	dataBlob := string(payload)

	// Create UAPEnvelope from GovernanceEnvelope fields
	uapEnv := &uap.UAPEnvelope{
		ProtocolVersion: "1.0",
		MessageID:       protoEnv.Id, // Will be regenerated by GenerateMessageID
		Metadata: uap.Metadata{
			SenderID:  protoEnv.OperatorSessionId,
			Timestamp: protoEnv.Timestamp.AsTime(),
			Signature: "", // GovernanceEnvelope signatures are different format
		},
		Intent: uap.Intent{
			ActionType:     actionType,
			TargetResource: targetResource,
		},
		Context: uap.Context{
			DataFormat: "raw",
			DataBlob:   dataBlob,
		},
		Consensus: uap.ConsensusState{
			RequiredVotes: 1, // Migration mode: single vote required
			CurrentVotes:  []uap.Vote{},
			Status:        "PENDING",
		},
	}

	// Generate proper MessageID from Intent+Context
	if id, err := uapEnv.GenerateMessageID(); err != nil {
		return nil, "", "", fmt.Errorf("failed to generate MessageID: %w", err)
	} else {
		uapEnv.MessageID = id
	}

	return uapEnv, caseID, investigationID, nil
}

// mapEventTypeToActionType maps protobuf event types to UAP action types.
func mapEventTypeToActionType(eventType string) string {
	switch eventType {
	case constants.Event.Operator.Command.Requested:
		return "EXECUTE_BASH"
	case constants.Event.Operator.FileEdit.Requested:
		return "FILE_EDIT"
	case constants.Event.Operator.FsList.Requested:
		return "FS_LIST"
	case constants.Event.Operator.FsRead.Requested:
		return "FS_READ"
	case constants.Event.Operator.FsGrep.Requested:
		return "FS_GREP"
	case constants.Event.Operator.PortCheck.Requested:
		return "PORT_CHECK"
	case constants.Event.Operator.FetchLogs.Requested:
		return "FETCH_LOGS"
	case constants.Event.Operator.FetchHistory.Requested:
		return "FETCH_HISTORY"
	case constants.Event.Operator.FetchFileHistory.Requested:
		return "FETCH_FILE_HISTORY"
	case constants.Event.Operator.RestoreFile.Requested:
		return "RESTORE_FILE"
	case constants.Event.Operator.ShutdownRequested:
		return "SHUTDOWN"
	case constants.Event.Operator.HeartbeatRequested:
		return "HEARTBEAT"
	default:
		// For unknown event types, use the event type itself as action type
		return eventType
	}
}

// mapActionTypeToEventType maps UAP action types back to protobuf event types for handler dispatch.
func mapActionTypeToEventType(actionType string) string {
	switch actionType {
	case "EXECUTE_BASH":
		return constants.Event.Operator.Command.Requested
	case "FILE_EDIT":
		return constants.Event.Operator.FileEdit.Requested
	case "FS_LIST":
		return constants.Event.Operator.FsList.Requested
	case "FS_READ":
		return constants.Event.Operator.FsRead.Requested
	case "FS_GREP":
		return constants.Event.Operator.FsGrep.Requested
	case "PORT_CHECK":
		return constants.Event.Operator.PortCheck.Requested
	case "FETCH_LOGS":
		return constants.Event.Operator.FetchLogs.Requested
	case "FETCH_HISTORY":
		return constants.Event.Operator.FetchHistory.Requested
	case "FETCH_FILE_HISTORY":
		return constants.Event.Operator.FetchFileHistory.Requested
	case "RESTORE_FILE":
		return constants.Event.Operator.RestoreFile.Requested
	case "SHUTDOWN":
		return constants.Event.Operator.ShutdownRequested
	case "HEARTBEAT":
		return constants.Event.Operator.HeartbeatRequested
	default:
		// For unknown action types, use the action type itself as event type
		return actionType
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

func (rs *PubSubCommandService) handleShutdownRequest(msg PubSubCommandMessage) {
	rs.logger.Info("Shutdown command received")
	// Try to parse as protobuf ShutdownRequested first (for migration compatibility)
	var sp operatorv1.ShutdownRequested
	if err := proto.Unmarshal(msg.Payload, &sp); err == nil {
		reason := sp.Reason
		if reason == "" {
			reason = "No reason provided"
		}
		rs.logger.Info("Shutting down operator (protobuf)", "reason", reason)
		rs.ShutdownChan <- reason
		return
	}

	// Fallback: treat payload as plain string (UAP format)
	reason := string(msg.Payload)
	if reason == "" {
		reason = "No reason provided"
	}
	rs.logger.Info("Shutting down operator (UAP)", "reason", reason)
	rs.ShutdownChan <- reason
}

// SendAutomaticHeartbeat publishes an automatic heartbeat immediately.
func (rs *PubSubCommandService) SendAutomaticHeartbeat() {
	rs.heartbeat.SendAutomatic()
}
