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
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	"github.com/g8e-ai/g8e/components/g8eo/services/mcp"
	"github.com/g8e-ai/g8e/components/g8eo/services/sentinel"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
)

// PubSubCommandMessage is the inbound wire message received from VSODB pub/sub.
type PubSubCommandMessage struct {
	ID                string          `json:"id"`
	EventType         string          `json:"event_type"`
	CaseID            string          `json:"case_id"`
	TaskID            *string         `json:"task_id,omitempty"`
	InvestigationID   string          `json:"investigation_id"`
	OperatorSessionID string          `json:"operator_session_id"`
	OperatorID        *string         `json:"operator_id,omitempty"`
	Payload           json.RawMessage `json:"payload"`
	Timestamp         time.Time       `json:"timestamp"`
}

// PubSubCommandService manages the VSODB pub/sub connection and dispatches inbound
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

	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
	mu      sync.RWMutex

	reconnectBaseDelay time.Duration
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
		client, err = NewVSODBPubSubClient(c.Config.PubSubURL, c.Config.TLSServerName, c.Logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create VSODB pub/sub client: %w", err)
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

	c.Logger.Info("g8e connectivity initialized")
	return rs, nil
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
	rs.logger.Info("Establishing connection with VSOD",
		"operator_id", rs.config.OperatorID,
		"operator_session_id", rs.config.OperatorSessionId)

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
			rs.logger.Error("[RECONNECT] No VSODB pub/sub client configured")
			return
		}

		rs.logger.Info("Subscribing to operator command channel", "operator_session_id", rs.config.OperatorSessionId)

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
	if err := json.Unmarshal(payload, &cmdMsg); err != nil {
		rs.logger.Error("Failed to parse command message", "error", err)
		return
	}

	rs.logger.Info("Processing request")
	rs.dispatchCommand(cmdMsg)
}

func (rs *PubSubCommandService) dispatchCommand(cmdMsg PubSubCommandMessage) {
	switch cmdMsg.EventType {
	case constants.Event.Operator.HeartbeatRequested:
		rs.heartbeat.HandleRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.Command.Requested:
		rs.commands.HandleExecutionRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.Command.CancelRequested:
		rs.commands.HandleCancelRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.FileEdit.Requested:
		rs.fileOps.HandleFileEditRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.FsList.Requested:
		rs.fileOps.HandleFsListRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.FsRead.Requested:
		rs.fileOps.HandleFsReadRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.PortCheck.Requested:
		rs.ports.HandlePortCheckRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.FetchLogs.Requested:
		rs.history.HandleFetchLogsRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.FetchHistory.Requested:
		rs.history.HandleFetchHistoryRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.FetchFileHistory.Requested:
		rs.history.HandleFetchFileHistoryRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.RestoreFile.Requested:
		rs.history.HandleRestoreFileRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.ShutdownRequested:
		rs.handleShutdownRequest(cmdMsg)
	case constants.Event.Operator.Audit.UserMsg:
		rs.audit.HandleUserMsgRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.Audit.AIMsg:
		rs.audit.HandleAIMsgRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.Audit.DirectCmd:
		rs.audit.HandleDirectCmdRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.Audit.DirectCmdResult:
		rs.audit.HandleDirectCmdResultRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.FetchFileDiff.Requested:
		rs.history.HandleFetchFileDiffRequest(rs.ctx, cmdMsg)
	case constants.Event.Operator.MCP.ToolsCall:
		rs.handleMCPToolsCall(rs.ctx, cmdMsg)
	default:
		rs.logger.Warn("Unknown request type")
	}
}

func (rs *PubSubCommandService) handleMCPToolsCall(ctx context.Context, msg PubSubCommandMessage) {
	rs.logger.Info("Handling MCP tools call", "id", msg.ID)

	var req mcp.JSONRPCRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		rs.logger.Error("Failed to parse MCP JSON-RPC request", "error", err)
		return
	}

	vsoMsg, err := mcp.TranslateToolCallToCommand(&req)
	if err != nil {
		rs.logger.Error("Failed to translate MCP tool call", "error", err)

		// Send MCP error response back
		errResp := &mcp.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &mcp.JSONRPCError{
				Code:    mcp.MethodNotFound, // Or InvalidParams depending on err
				Message: err.Error(),
			},
		}

		// We need a way to publish this back. We can use rs.results.PublishResult
		// but we need to build a VSOMessage for it.
		if msg.OperatorID == nil {
			rs.logger.Error("Cannot send MCP error response: operator_id is nil (required for routing)")
			return
		}
		payloadRaw, _ := json.Marshal(errResp)
		respMsg := &models.VSOMessage{
			ID:                req.ID,
			EventType:         constants.Event.Operator.MCP.ToolsResult,
			CaseID:            msg.CaseID,
			InvestigationID:   msg.InvestigationID,
			OperatorID:        *msg.OperatorID,
			OperatorSessionID: msg.OperatorSessionID,
			Payload:           payloadRaw,
		}
		if rs.results != nil {
			_ = rs.results.PublishResult(ctx, respMsg)
		}
		return
	}

	// Update the message with translated data but keep the EventType as MCP.ToolsCall
	// so the results publisher knows to wrap the result.
	// We only change the Payload to what the handlers expect.
	msg.Payload = vsoMsg.Payload
	msg.ID = vsoMsg.ID // MCP request ID

	// Now dispatch based on the translated EventType
	switch vsoMsg.EventType {
	case constants.Event.Operator.Command.Requested:
		rs.commands.HandleExecutionRequest(ctx, msg)
	case constants.Event.Operator.FileEdit.Requested:
		rs.fileOps.HandleFileEditRequest(ctx, msg)
	case constants.Event.Operator.FsList.Requested:
		rs.fileOps.HandleFsListRequest(ctx, msg)
	case constants.Event.Operator.FsRead.Requested:
		rs.fileOps.HandleFsReadRequest(ctx, msg)
	case constants.Event.Operator.PortCheck.Requested:
		rs.ports.HandlePortCheckRequest(ctx, msg)
	case constants.Event.Operator.FetchFileHistory.Requested:
		rs.history.HandleFetchFileHistoryRequest(ctx, msg)
	case constants.Event.Operator.FetchFileDiff.Requested:
		rs.history.HandleFetchFileDiffRequest(ctx, msg)
	default:
		rs.logger.Warn("Unsupported MCP tool event type", "event_type", vsoMsg.EventType)
	}
}

func (rs *PubSubCommandService) handleShutdownRequest(msg PubSubCommandMessage) {
	rs.logger.Info("Shutdown command received")
	var sp models.ShutdownRequestPayload
	if err := json.Unmarshal(msg.Payload, &sp); err != nil {
		rs.logger.Warn("Failed to decode shutdown payload", "error", err)
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
