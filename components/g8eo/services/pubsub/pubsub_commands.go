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

	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	"github.com/g8e-ai/g8e/components/g8eo/services/sentinel"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
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

	c.Logger.Info("g8e connectivity initialized",
		"config_operator_id", c.Config.OperatorID,
		"config_operator_session_id", c.Config.OperatorSessionId)
	return rs, nil
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

	var cmdMsg PubSubCommandMessage
	if len(payload) > MaxPayloadSize {
		rs.logger.Error("Command payload exceeds maximum size limit",
			"size", len(payload),
			"limit", MaxPayloadSize)
		return
	}

	// Parse as GovernanceEnvelope (protobuf) - protocol-first architecture
	var env commonv1.GovernanceEnvelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		rs.logger.Error("Failed to parse command message as protobuf GovernanceEnvelope", "error", err)
		return
	}
	if env.Id == "" {
		rs.logger.Error("Invalid GovernanceEnvelope: missing id field")
		return
	}

	rs.logger.Info("Parsed request via Protobuf (GovernanceEnvelope)",
		"state_merkle_root", env.StateMerkleRoot)
	taskID := env.TaskId
	operatorID := env.OperatorId

	cmdMsg = PubSubCommandMessage{
		ID:                env.Id,
		EventType:         env.EventType,
		CaseID:            env.CaseId,
		TaskID:            &taskID,
		InvestigationID:   env.InvestigationId,
		OperatorSessionID: env.OperatorSessionId,
		OperatorID:        &operatorID,
		Payload:           env.Payload,
		Timestamp:         env.Timestamp.AsTime(),
	}

	// 1. Basic Protocol Gates: Expiry and Replay Protection
	if env.ExpiresAt != nil {
		if time.Now().UTC().After(env.ExpiresAt.AsTime()) {
			rs.logger.Error("Transaction expired: command rejected",
				"expiry", env.ExpiresAt.AsTime(),
				"execution_id", env.Id)
			return
		}
	}

	if env.Nonce != "" {
		// Replay protection check
		nonceKey := "g8e:nonce:" + env.Nonce
		if _, found := rs.commands.localStore.KVGet(nonceKey); found {
			rs.logger.Error("Replay detected: command rejected",
				"nonce", env.Nonce,
				"execution_id", env.Id)
			return
		}
		// Record nonce with a 24h TTL
		rs.commands.localStore.KVSet(nonceKey, "used", 86400)
	}

	// 2. BFT Verification: Verify state merkle root if present
	if env.StateMerkleRoot != "" {
		if rs.commands.ledger != nil {
			currentRoot, err := rs.commands.ledger.GetStateMerkleRoot()
			if err != nil {
				rs.logger.Warn("Failed to get current state merkle root for BFT verification", "error", err)
				// Continue without verification - non-fatal error
			} else if currentRoot == "" {
				rs.logger.Info("Ledger not available for BFT verification, accepting command without verification")
			} else if env.StateMerkleRoot != currentRoot {
				rs.logger.Error("BFT verification failed: State merkle root mismatch - command based on stale state",
					"expected_root", currentRoot,
					"received_root", env.StateMerkleRoot,
					"execution_id", env.Id)
				// Reject the command - it was generated based on stale state
				return
			} else {
				rs.logger.Info("BFT verification passed: State merkle root matches",
					"merkle_root", env.StateMerkleRoot,
					"execution_id", env.Id)
			}
		} else {
			rs.logger.Info("Ledger service not configured, skipping BFT verification")
		}
	}

	rs.logger.Info("Processing request")

	// L1 Governance: Enforce forbidden patterns via reflection (Protocol-First architecture)
	payloadMsg, err := unmarshalPayload(env.EventType, env.Payload)
	if err == nil {
		violations := validateL1Governance(payloadMsg)
		if len(violations) > 0 {
			rs.logger.Error("L1 Governance violation: command rejected",
				"event_type", env.EventType,
				"violations", violations,
				"execution_id", env.Id)

			// We reject the command here. In a production system, we would also
			// publish a failure result to the gateway, but for now we follow
			// the central enforcement rule.
			return
		}
		rs.logger.Info("L1 Governance validation passed", "event_type", env.EventType)
	} else {
		// Log warning but continue; not all events may have proto-message definitions yet
		rs.logger.Warn("Skipping L1 Governance validation: failed to unmarshal payload", "error", err)
	}

	// L2 Governance: Verify Tribunal signature (HMAC or ED25519)
	if rs.auditorHMACKey != "" || len(rs.trustedSigners) > 0 {
		if err := VerifyL2Governance(&env, rs.auditorHMACKey, rs.trustedSigners); err != nil {
			rs.logger.Error("L2 Governance verification failed: command rejected",
				"event_type", env.EventType,
				"error", err,
				"execution_id", env.Id)
			return
		}
		rs.logger.Info("L2 Governance verification passed", "event_type", env.EventType)
	} else {
		rs.logger.Debug("L2 Governance verification skipped: no HMAC key or trusted signers configured", "event_type", env.EventType)
	}

	// L3 Governance: Verify Human Approval signature for mutation requests
	// Note: In a production system, we'd lookup trusted L3 keys for the user.
	// For now, we enforce that mutation requests (Command, FileEdit) must have L3 or be auto-approved.
	isMutation := env.EventType == constants.Event.Operator.Command.Requested ||
		env.EventType == constants.Event.Operator.FileEdit.Requested ||
		env.EventType == constants.Event.Operator.RestoreFile.Requested ||
		env.EventType == constants.Event.Operator.ShutdownRequested

	if isMutation {
		// For Phase 3, we require L3Metadata to be present.
		// If AutoApproved=true, we still allow it (policy check is L1).
		if err := VerifyL3Governance(&env, nil); err != nil {
			rs.logger.Error("L3 Governance verification failed: mutation rejected",
				"event_type", env.EventType,
				"error", err,
				"execution_id", env.Id)
			return
		}
		rs.logger.Info("L3 Governance verification passed", "event_type", env.EventType)
	}

	rs.dispatchCommand(cmdMsg)
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
	rs.logger.Info("Shutdown command received (via Protobuf)")
	var sp operatorv1.ShutdownRequested
	if err := proto.Unmarshal(msg.Payload, &sp); err != nil {
		rs.logger.Warn("Failed to decode shutdown payload as protobuf ShutdownRequested", "error", err)
	}
	reason := sp.Reason
	if reason == "" {
		reason = "No reason provided"
	}
	rs.logger.Info("Shutting down operator", "reason", reason)
	rs.ShutdownChan <- reason
}

// SendAutomaticHeartbeat publishes an automatic heartbeat immediately.
func (rs *PubSubCommandService) SendAutomaticHeartbeat() {
	rs.heartbeat.SendAutomatic()
}
