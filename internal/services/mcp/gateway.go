// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package mcp provides Model Context Protocol (MCP) and Agent-to-Agent (A2A) services.
//
// This package contains GatewayService, which is a shared service used in both gateway mode
// and outbound mode for MCP/A2A protocol translation and downstream dispatch. The service is
// truly polymorphic - the same implementation is used in both modes.
//
// For more information on service modes, see docs/architecture/service_modes.md.
package mcp

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	govpkg "github.com/g8e-ai/g8e/internal/governance"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/uuid"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ApprovalRequestTTL is the lifetime of a human-approval request created when a
// state-changing action is paused under the notary posture. It is deliberately
// short: notary enforces *proof of human presence*, so the passkey (WebAuthn)
// authorization must be completed close in time to the action it authorizes.
// Once this window lapses the suspended transaction expires (the store filters on
// expires_at) and a fresh approval must be requested by retrying the action.
const ApprovalRequestTTL = 2 * time.Minute

// approvalPausedMessage builds the directive returned to the calling agent when a
// mutation is paused awaiting human passkey authorization under notary posture.
// It is phrased as an instruction to the AI: what happened, what a human must do,
// the short deadline, and—critically—what to do once that deadline passes (retry
// the call to open a fresh approval request).
func approvalPausedMessage(approvalURL string) string {
	return fmt.Sprintf("Execution paused: this is a state-changing action and the gateway is running in "+
		"notary posture, which REQUIRES live human passkey (WebAuthn) authorization before it can run. "+
		"A human must approve this exact change at %s within %d minutes. "+
		"Wait for the human to approve, then retry this identical tool call to proceed. "+
		"This window is intentionally short so the approval proves a human was present for this specific action. "+
		"If it lapses before approval the request is voided for security — call this tool again to open a fresh approval request.",
		approvalURL, int(ApprovalRequestTTL.Minutes()))
}

// L2ConsensusDeliberator sends an envelope to an L2 consensus service for deliberation.
// The consensus service collects signed votes from its members and returns the envelope
// with L2 metadata populated. This interface is implemented by an HTTP client that calls
// the consensus service's /consensus/v1/deliberate endpoint, or by an in-process adapter.
type L2ConsensusDeliberator interface {
	Deliberate(ctx context.Context, envelopeBytes []byte) ([]byte, error)
}

// StateRootProvider defines the interface for obtaining the current state root.
type StateRootProvider interface {
	GetCurrentStateRoot() (string, error)
}

// GatewayService handles MCP/A2A protocol translation and downstream dispatch.
// This service is shared across both gateway mode and outbound mode - the same
// implementation is used in both contexts (truly polymorphic).
//
// In gateway mode, it is created by GatewayModeService as field mcpGateway.
// In outbound mode, it is used by OperatorPubSubService as field mcpGateway.
type GatewayService struct {
	logger            *slog.Logger
	responder         *response.Writer
	suspendedStore    storage.SuspendedTransactionStore
	fieldPathRegistry *FieldPathRegistry
	auditStore        AuditEventRecorder
	nativeToolHandler *NativeToolHandler
	scrubbingService  *scrubbing.ScrubbingService
	threatScanner     ThreatScanner
	posture           string // Gateway posture: doctrine, consensus, or notary
	a2aDownstreamURL  string // construction-phase (immutable after NewGatewayService)
	publicBaseURL     string // construction-phase (immutable after NewGatewayService)
	maxPayloadBytes   int64

	envProc                governance.EnvelopeProcessor
	stateRootProvider      StateRootProvider
	signingKey             ed25519.PrivateKey
	keyID                  string
	downstreamURL          string
	dbService              FieldReader
	sessionValidator       SessionValidator
	auditLogger            AuditLogger
	l2ConsensusDeliberator L2ConsensusDeliberator

	// Circuit breaker state
	mu               sync.RWMutex
	failureCount     int
	lastFailure      time.Time
	circuitOpen      bool
	cooldownDuration time.Duration
	maxFailures      int
}

// FieldReader provides read access to individual document fields, backing the
// MCP gateway's read_field operation. It is implemented by the gateway's
// document store (DocumentStoreService).
type FieldReader interface {
	GetField(collection, id, fieldPath string) (FieldValue, error)
}

// NoopFieldReader is a no-op implementation of FieldReader.
// It returns an empty FieldValue with no error. Used in tests;
// production outbound mode uses nil with explicit nil-checks.
type NoopFieldReader struct{}

func (NoopFieldReader) GetField(string, string, string) (FieldValue, error) {
	return FieldValue{}, nil
}

// SessionValidator validates Operator sessions for L3 authorization
type SessionValidator interface {
	ValidateSession(operatorSessionID string) (bool, error)
}

// AuditLogger logs field read operations to the audit vault
type AuditLogger interface {
	LogFieldRead(operatorSessionID, collection, documentID, fieldPath string, value FieldValue) error
}

// ThreatScanner scans input strings for security threats using L1 doctrine patterns.
// Implemented by governance.L1Doctrine.
type ThreatScanner interface {
	AnalyzeCommand(input string) []governance.ThreatSignal
}

// AuditEventRecorder records audit events. Implemented by storage.SQLAuditStore
// in production. NewGatewayService rejects a nil AuditStore at construction
// (constants.ErrAuditStoreRequired) — a missing audit store is a wiring bug
// and fail-open on a security control, not a no-op condition. The
// noopAuditEventRecorder type is retained for test helpers that construct
// GatewayService structs directly (bypassing the constructor).
type AuditEventRecorder interface {
	RecordEvent(event *storage.Event) (int64, error)
}

// NoopAuditEventRecorder is a no-op implementation of AuditEventRecorder,
// retained for test helpers that construct GatewayService via NewGatewayService
// or directly. NewGatewayService never defaults to this type; a nil AuditStore
// is a construction error (constants.ErrAuditStoreRequired). Tests that do not
// exercise audit recording pass NoopAuditEventRecorder{} explicitly.
type NoopAuditEventRecorder struct{}

func (NoopAuditEventRecorder) RecordEvent(*storage.Event) (int64, error) { return 0, nil }

// noopAuditEventRecorder is an alias retained for existing in-package test
// helpers that construct GatewayService structs directly.
type noopAuditEventRecorder = NoopAuditEventRecorder

// Dependencies groups all dependencies for NewGatewayService.
type Dependencies struct {
	Logger           *slog.Logger
	Responder        *response.Writer
	SuspendedStore   storage.SuspendedTransactionStore
	ScrubbingService *scrubbing.ScrubbingService
	ThreatScanner    ThreatScanner
	MaxPayloadBytes  int64
	Posture          string // Gateway posture: doctrine, consensus, or notary
	A2ADownstreamURL string // A2A downstream server URL
	PublicBaseURL    string // Public base URL for approval links

	// AuditStore records audit events for suspended transactions and downstream
	// MCP calls. Required: NewGatewayService returns constants.ErrAuditStoreRequired
	// when nil — a missing audit store is a wiring bug, not a no-op condition.
	AuditStore AuditEventRecorder

	// FieldPathRegistryFactory overrides the default NewFieldPathRegistry constructor.
	// When nil, NewFieldPathRegistry is used. This allows tests to inject a failing
	// factory to verify error handling in NewGatewayService.
	FieldPathRegistryFactory func(*slog.Logger) (*FieldPathRegistry, error)

	EnvProc                governance.EnvelopeProcessor
	StateRootProvider      StateRootProvider
	SigningKey             ed25519.PrivateKey
	KeyID                  string
	DownstreamURL          string
	DBService              FieldReader
	SessionValidator       SessionValidator
	AuditLogger            AuditLogger
	L2ConsensusDeliberator L2ConsensusDeliberator
}

func NewGatewayService(deps Dependencies) (*GatewayService, error) {
	// Validate posture parameter
	validPostures := map[string]bool{
		constants.PostureDoctrine:  true,
		constants.PostureConsensus: true,
		constants.PostureNotary:    true,
	}
	if deps.Posture != "" && !validPostures[deps.Posture] {
		return nil, fmt.Errorf("gateway: invalid posture '%s': must be one of doctrine, consensus, or notary: %w", deps.Posture, constants.ErrGatewayInvalidPosture)
	}

	if deps.AuditStore == nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrAuditStoreRequired)
	}

	registryFactory := deps.FieldPathRegistryFactory
	if registryFactory == nil {
		registryFactory = NewFieldPathRegistry
	}
	fieldPathRegistry, err := registryFactory(deps.Logger)
	if err != nil {
		return nil, fmt.Errorf("gateway: initialize field path registry: %w", err)
	}

	nativeToolHandler, err := NewNativeToolHandler(deps.Logger)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	g := &GatewayService{
		logger:                 deps.Logger,
		responder:              deps.Responder,
		suspendedStore:         deps.SuspendedStore,
		fieldPathRegistry:      fieldPathRegistry,
		auditStore:             deps.AuditStore,
		nativeToolHandler:      nativeToolHandler,
		scrubbingService:       deps.ScrubbingService,
		threatScanner:          deps.ThreatScanner,
		posture:                deps.Posture,
		a2aDownstreamURL:       deps.A2ADownstreamURL,
		publicBaseURL:          deps.PublicBaseURL,
		maxFailures:            5,
		cooldownDuration:       1 * time.Minute,
		maxPayloadBytes:        deps.MaxPayloadBytes,
		envProc:                deps.EnvProc,
		stateRootProvider:      deps.StateRootProvider,
		signingKey:             deps.SigningKey,
		keyID:                  deps.KeyID,
		downstreamURL:          deps.DownstreamURL,
		dbService:              deps.DBService,
		sessionValidator:       deps.SessionValidator,
		auditLogger:            deps.AuditLogger,
		l2ConsensusDeliberator: deps.L2ConsensusDeliberator,
	}
	return g, nil
}

// runMaintenanceSweep performs a single maintenance sweep to audit and delete expired transactions.
func (g *GatewayService) runMaintenanceSweep(ctx context.Context) error {
	if g.suspendedStore == nil {
		return nil
	}

	// Get expired transactions for audit before deletion
	expiredTxs, err := g.suspendedStore.GetExpiredSuspendedTransactions(ctx)
	if err != nil {
		return fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	// Audit each expired transaction to the originating session's chain
	for _, tx := range expiredTxs {
		// Use the originating operator session ID for the audit event
		// so the agent reading its own session chain can detect the expiry
		operatorSessionID := tx.OperatorID
		if operatorSessionID == "" {
			operatorSessionID = tx.UserID
		}

		if operatorSessionID != "" {
			event := &storage.Event{
				OperatorSessionID: operatorSessionID,
				Timestamp:         time.Now().UTC(),
				Type:              constants.Event.Operator.Notary.TransactionExpired,
				ContentText:       fmt.Sprintf("Transaction %s approval expired (tool: %s)", tx.TransactionHash, tx.ToolName),
			}
			if _, err := g.auditStore.RecordEvent(event); err != nil {
				g.logger.Warn("Failed to record transaction expiry event", "error", err, "tx_hash", tx.TransactionHash, "operator_session_id", operatorSessionID)
			}
		}
	}

	// Delete expired transactions after audit
	deletedCount, err := g.suspendedStore.CleanupExpiredSuspendedTransactions(ctx)
	if err != nil {
		return fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	if deletedCount > 0 {
		g.logger.Info("Pruned expired suspended transactions", "count", deletedCount)
	}

	return nil
}

// RunMaintenance periodically prunes expired suspended transactions.
// It audits expired transactions before deletion by recording expiry events
// to the originating session's audit-vault chain, then deletes them.
func (g *GatewayService) RunMaintenance(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := g.runMaintenanceSweep(ctx); err != nil {
				g.logger.Error("Maintenance sweep failed", "error", err)
			}
		}
	}
}

// SetAuditLogger sets the audit logger. Called by NewGatewayOperatorPubSubService
// after construction.
func (g *GatewayService) SetAuditLogger(l AuditLogger) {
	g.auditLogger = l
}

// SetL2ConsensusDeliberator sets the L2 consensus deliberator. Called
// after construction when consensus is bootstrapped.
func (g *GatewayService) SetL2ConsensusDeliberator(d L2ConsensusDeliberator) {
	g.l2ConsensusDeliberator = d
}

// isNativeTool checks if a tool name is a native tool compiled into the Operator.
func (g *GatewayService) isNativeTool(name string) bool {
	if g.nativeToolHandler == nil {
		return false
	}
	_, ok := g.nativeToolHandler.registry.Get(name)
	return ok
}

func (g *GatewayService) isCircuitOpen() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if !g.circuitOpen {
		return false
	}

	// Check if cooldown period has elapsed
	if time.Since(g.lastFailure) > g.cooldownDuration {
		return false // Half-open state implicitly (next request will either succeed or re-open)
	}

	return true
}

func (g *GatewayService) recordFailure() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.failureCount++
	g.lastFailure = time.Now()
	if g.failureCount >= g.maxFailures {
		if !g.circuitOpen {
			if g.logger != nil {
				g.logger.Warn("MCP downstream circuit breaker OPENED", "url", g.downstreamURL, "failures", g.failureCount)
			}
		}
		g.circuitOpen = true
	}
}

func (g *GatewayService) recordSuccess() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.circuitOpen {
		if g.logger != nil {
			g.logger.Info("MCP downstream circuit breaker CLOSED", "url", g.downstreamURL)
		}
	}
	g.failureCount = 0
	g.circuitOpen = false
}

func (g *GatewayService) handleA2ARequest(w http.ResponseWriter, r *http.Request, method string, handler func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error)) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if g.isCircuitOpen() {
		g.logger.Warn("MCP downstream circuit is open, rejecting request", "method", method, "url", g.downstreamURL)
		g.responder.RPCError(w, nil, -32603, "downstream MCP server is temporarily unavailable (circuit open)")
		return
	}

	// P1-1: Enforce payload limits from config
	r.Body = http.MaxBytesReader(w, r.Body, g.maxPayloadBytes)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			maxMB := g.maxPayloadBytes / (1024 * 1024)
			g.responder.RPCError(w, nil, -32600, fmt.Sprintf("request payload too large (max %dMB)", maxMB))
			return
		}
		g.responder.RPCError(w, nil, -32603, "failed to read request body")
		return
	}

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		g.responder.RPCError(w, nil, constants.JSONRPCErrorCodeParseError, constants.JSONRPCErrorMessageParseError+": invalid JSON")
		return
	}

	// Validate JSON-RPC 2.0
	if req.JSONRPC != "2.0" {
		g.responder.RPCError(w, req.ID, -32600, "invalid request: jsonrpc version must be 2.0")
		return
	}

	if req.Method == "" {
		g.responder.RPCError(w, req.ID, -32600, "invalid request: method required")
		return
	}

	if req.Method != method {
		g.responder.RPCError(w, req.ID, -32601, fmt.Sprintf("method not found: expected %s, got %s", method, req.Method))
		return
	}

	result, err := handler(r.Context(), req.ID, req.Params)
	if err != nil {
		code, msg := g.mapGatewayError(err)
		// If it's a standard error we don't recognize, use Internal Error
		if code == 0 && msg == "" {
			code = -32603
			msg = err.Error()
		}
		g.responder.RPCError(w, req.ID, code, msg)
		return
	}

	if result != nil {
		g.responder.RPCResponse(w, req.ID, result)
	}
}

// callTool executes a governed MCP tools/call. It is called by the unified
// Streamable HTTP dispatcher (HandleMCP) via dispatchMCP.
// The *http.Request is used to extract identity headers and the client
// certificate fingerprint when a transaction is suspended for L3 approval.
func (g *GatewayService) callTool(ctx context.Context, r *http.Request, params json.RawMessage) (interface{}, error) {
	var callParams CallToolRequest
	if err := json.Unmarshal(params, &callParams); err != nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrGatewayInvalidToolArguments)
	}

	if callParams.Name == "" {
		return nil, constants.ErrGatewayToolNameRequired
	}

	argumentsJSON := "{}"
	if len(callParams.Arguments) > 0 {
		var probe interface{}
		if err := json.Unmarshal(callParams.Arguments, &probe); err != nil {
			return nil, constants.ErrGatewayInvalidToolArguments
		}
		argumentsJSON = string(callParams.Arguments)
	}

	mcpPayload := &operatorv1.McpCallRequested{
		ToolName:      callParams.Name,
		ArgumentsJson: argumentsJSON,
		ExecutionId:   uuid.NewString(),
	}
	payloadBytes, err := proto.Marshal(mcpPayload)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	hash, envelopeBytes, err := g.processGatewayTransaction(ctx, processGatewayOptions{
		actionType:     constants.ActionTypeMcpCall,
		targetResource: callParams.Name,
		payloadBytes:   payloadBytes,
	})
	if err != nil {
		return nil, err
	}

	receipt, err := g.envProc.ProcessEnvelope(ctx, envelopeBytes)
	if err != nil {
		if errors.Is(err, governance.ErrL3ProofMissing) {
			userID, _ := r.Context().Value(constants.ContextKeyUserID).(string)
			operatorID, _ := r.Context().Value(constants.ContextKeyAppID).(string)
			certFingerprint := extractCertFingerprint(r)

			g.StoreSuspendedTransaction(ctx, hash, envelopeBytes, callParams.Name, callParams.Arguments, userID, operatorID, certFingerprint)

			approvalURL := fmt.Sprintf("%s/approve/%s", g.publicBaseURL, hash)
			return CallToolResult{
				Content: []TextContent{
					{
						Type: "text",
						Text: approvalPausedMessage(approvalURL),
					},
				},
			}, nil
		}
		return nil, err
	}

	mcpRes := CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: receipt.ResultSummary,
			},
		},
	}
	if receipt.Status != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
		mcpRes.IsError = true
	}
	return mcpRes, nil
}

// handleReadField processes the read_field tool with governed access controls
func (g *GatewayService) handleReadField(ctx context.Context, arguments json.RawMessage) (interface{}, error) {
	if g.fieldPathRegistry == nil {
		return nil, constants.ErrGatewayFieldPathRegistryNotInit
	}

	if g.dbService == nil {
		return nil, constants.ErrGatewayDatabaseServiceNotConfigured
	}

	var req FieldReadRequest
	if err := json.Unmarshal(arguments, &req); err != nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrInvalidJSONBody)
	}

	// Validate required fields
	if req.Collection == "" {
		return nil, constants.ErrGatewayCollectionRequired
	}
	if req.DocumentID == "" {
		return nil, constants.ErrGatewayDocumentIDRequired
	}
	if req.FieldPath == "" {
		return nil, constants.ErrGatewayFieldPathRequired
	}
	if req.OperatorSessionID == "" {
		return nil, constants.ErrGatewayOperatorSessionIDRequired
	}

	// L1: Validate field path against schema registry
	if err := g.fieldPathRegistry.ValidateFieldPath(req.Collection, req.FieldPath); err != nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	// L3: Validate Operator session
	if g.sessionValidator != nil {
		valid, err := g.sessionValidator.ValidateSession(req.OperatorSessionID)
		if err != nil {
			return nil, fmt.Errorf("gateway: %w", constants.ErrInternal)
		}
		if !valid {
			return nil, constants.ErrGatewayOperatorSessionInvalid
		}
	}

	// Extract field value from database
	value, err := g.dbService.GetField(req.Collection, req.DocumentID, req.FieldPath)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	// L1: Scan field value for forbidden patterns
	if err := g.scanForForbiddenPatterns(value); err != nil {
		return nil, err
	}

	// Audit vault logging
	if g.auditLogger != nil {
		if err := g.auditLogger.LogFieldRead(req.OperatorSessionID, req.Collection, req.DocumentID, req.FieldPath, value); err != nil {
			g.logger.Warn("Failed to log field read to audit vault", "error", err, "collection", req.Collection, "field_path", req.FieldPath)
		}
	}

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: value.String(),
			},
		},
	}, nil
}

// scanForForbiddenPatterns checks if a FieldValue contains security threats
// using the L1 doctrine threat detection system. Delegates to the ThreatScanner
// (governance.L1Doctrine) which uses regex-based word-boundary matching instead
// of the former hardcoded substring patterns.
func (g *GatewayService) scanForForbiddenPatterns(value FieldValue) error {
	if value.Null {
		return nil
	}

	if g.threatScanner == nil {
		return nil
	}

	valueStr := value.String()
	signals := g.threatScanner.AnalyzeCommand(valueStr)
	for _, sig := range signals {
		if sig.BlockRecommended {
			return fmt.Errorf("gateway: L1 hard gate: threat detected (%s): %s: %w", sig.Category, sig.Indicator, constants.ErrGatewayForbiddenPattern)
		}
	}

	return nil
}

// readResource executes a governed MCP resources/read. Shared by the unified
// dispatcher (HandleMCP).
func (g *GatewayService) readResource(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var readParams ReadResourceRequest
	if err := json.Unmarshal(params, &readParams); err != nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrInvalidJSONBody)
	}

	if readParams.URI == "" {
		return nil, constants.ErrGatewayURIRequired
	}

	mcpPayload := &operatorv1.McpResourceReadRequested{
		Uri:         readParams.URI,
		ExecutionId: uuid.NewString(),
	}
	payloadBytes, err := proto.Marshal(mcpPayload)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	_, envelopeBytes, err := g.processGatewayTransaction(ctx, processGatewayOptions{
		actionType:     constants.ActionTypeMcpResourceRead,
		targetResource: readParams.URI,
		payloadBytes:   payloadBytes,
	})
	if err != nil {
		return nil, err
	}

	receipt, err := g.envProc.ProcessEnvelope(ctx, envelopeBytes)
	if err != nil {
		return nil, err
	}

	mcpRes := ReadResourceResult{
		Contents: []TextContent{
			{
				Type: "text",
				Text: receipt.ResultSummary,
			},
		},
	}
	if receipt.Status != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
		mcpRes.Contents[0].Text = fmt.Sprintf("Error: %s", receipt.ResultSummary)
	}
	return mcpRes, nil
}

// getPrompt executes a governed MCP prompts/get. Shared by the unified
// dispatcher (HandleMCP).
func (g *GatewayService) getPrompt(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var getParams GetPromptRequest
	if err := json.Unmarshal(params, &getParams); err != nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrInvalidJSONBody)
	}

	if getParams.Name == "" {
		return nil, constants.ErrGatewayNameRequired
	}

	mcpPayload := &operatorv1.McpPromptGetRequested{
		Name:        getParams.Name,
		ExecutionId: uuid.NewString(),
	}
	payloadBytes, err := proto.Marshal(mcpPayload)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	_, envelopeBytes, err := g.processGatewayTransaction(ctx, processGatewayOptions{
		actionType:     constants.ActionTypeMcpPromptGet,
		targetResource: getParams.Name,
		payloadBytes:   payloadBytes,
	})
	if err != nil {
		return nil, err
	}

	receipt, err := g.envProc.ProcessEnvelope(ctx, envelopeBytes)
	if err != nil {
		return nil, err
	}

	mcpRes := GetPromptResult{
		Description: receipt.ResultSummary,
		Messages: []PromptMessage{
			{
				Role: string(constants.UserRoleUser),
				Content: TextContent{
					Type: "text",
					Text: receipt.ResultSummary,
				},
			},
		},
	}
	if receipt.Status != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
		mcpRes.Description = fmt.Sprintf("Error: %s", receipt.ResultSummary)
	}
	return mcpRes, nil
}

type processGatewayOptions struct {
	actionType     constants.ActionType
	targetResource string
	payloadBytes   []byte
}

func (g *GatewayService) processGatewayTransaction(ctx context.Context, opts processGatewayOptions) (hash string, envelopeBytes []byte, err error) {
	stateRoot := ""
	if g.stateRootProvider != nil {
		var err error
		stateRoot, err = g.stateRootProvider.GetCurrentStateRoot()
		if err != nil {
			g.logger.Warn("Failed to get current state root", "error", err)
		}
	}

	now := time.Now().UTC()
	env := &commonv1.GovernanceEnvelope{
		Timestamp:       timestamppb.New(now),
		ExpiresAt:       timestamppb.New(now.Add(5 * time.Minute)),
		SourceComponent: commonv1.Component_COMPONENT_CLIENT,
		ActionType:      string(opts.actionType),
		TargetResource:  opts.targetResource,
		Payload:         opts.payloadBytes,
		ProtocolVersion: "1.0",
		Nonce:           uuid.NewString(),
		StateMerkleRoot: stateRoot,
		Governance:      &commonv1.GovernanceMetadata{},
	}

	// Enrich from context if present
	if tenantID, ok := ctx.Value(constants.ContextKeyTenantID).(string); ok {
		env.TenantId = tenantID
	}
	if persona, ok := ctx.Value(constants.ContextKeyBindingPersona).(string); ok {
		env.BindingPersona = persona
	}
	// Bind both the app identity and the human requestor to the envelope.
	// For delegated credentials the auth middleware extracts both SANs from the cert
	// and places them in context — no trusted headers.
	if appID, ok := ctx.Value(constants.ContextKeyAppID).(string); ok && appID != "" {
		env.OperatorId = appID
		env.OperatorSessionId = appID
		env.ActingAppId = appID
	} else {
		if opID, ok := ctx.Value(constants.ContextKeyOperatorID).(string); ok && opID != "" {
			env.OperatorId = opID
		}
		if opSessionID, ok := ctx.Value(constants.ContextKeyOperatorSessionID).(string); ok && opSessionID != "" {
			env.OperatorSessionId = opSessionID
		}
	}
	if userID, ok := ctx.Value(constants.ContextKeyUserID).(string); ok && userID != "" {
		env.RequestorUserId = userID
	}

	hash, err = govpkg.GenerateMessageID(env)
	if err != nil {
		return "", nil, fmt.Errorf("gateway: %w", constants.ErrInternal)
	}
	env.Id = hash
	env.TransactionHash = hash

	envelopeBytes, err = (protojson.MarshalOptions{Multiline: false}).Marshal(env)
	if err != nil {
		return "", nil, fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	// Under any posture that requires L2 signatures (consensus and notary),
	// send the envelope to the Consensus for L2 deliberation before dispatch.
	// The Consensus collects signed votes from its members and returns the
	// envelope with L2 metadata populated. If the deliberator is not configured,
	// the envelope proceeds without L2 votes and will fail-closed at L4 verification.
	if (g.posture == "consensus" || g.posture == "notary") && g.l2ConsensusDeliberator != nil {
		deliberatedBytes, err := g.l2ConsensusDeliberator.Deliberate(ctx, envelopeBytes)
		if err != nil {
			g.logger.Error("L2 consensus deliberation failed", "tx_hash", hash, "error", err)
			return "", nil, fmt.Errorf("gateway: l2 consensus deliberation: %w", err)
		}
		envelopeBytes = deliberatedBytes
	}

	return hash, envelopeBytes, nil
}

// @Summary		A2A call
// @Description	Calls an A2A agent endpoint
// @Tags			a2a
// @Accept			json
// @Produce		json
// @Success		200	{object}	map[string]interface{}
// @Router			/api/v1/a2a/call [post]
func (g *GatewayService) HandleA2aCall(w http.ResponseWriter, r *http.Request) {
	if g.envProc == nil {
		g.responder.RPCError(w, nil, -32603, constants.ErrGatewayNotReady.Error())
		return
	}
	g.handleA2ARequest(w, r, "a2a/call", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
		return g.a2aCall(ctx, r, params)
	})
}

// a2aCall executes a governed A2A skill invocation. Shared by the A2A REST
// handler (HandleA2aCall) and the unified dispatcher (HandleMCP).
func (g *GatewayService) a2aCall(ctx context.Context, r *http.Request, params json.RawMessage) (interface{}, error) {
	var req A2ACallRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrInvalidJSONBody)
	}

	if req.SkillName == "" {
		return nil, constants.ErrGatewaySkillNameRequired
	}

	payloadStr := "{}"
	if len(req.Payload) > 0 {
		payloadStr = string(req.Payload)
	}

	a2aPayload := &operatorv1.A2ACallRequested{
		SkillName:   req.SkillName,
		PayloadJson: payloadStr,
		ExecutionId: req.ExecutionID,
	}
	payloadBytes, err := proto.Marshal(a2aPayload)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	hash, envelopeBytes, err := g.processGatewayTransaction(ctx, processGatewayOptions{
		actionType:     constants.ActionTypeA2aCall,
		targetResource: req.SkillName,
		payloadBytes:   payloadBytes,
	})
	if err != nil {
		return nil, err
	}

	receipt, err := g.envProc.ProcessEnvelope(ctx, envelopeBytes)
	if err != nil {
		if errors.Is(err, governance.ErrL3ProofMissing) || errors.Is(err, governance.ErrL3ProofInvalid) {
			userID, _ := r.Context().Value(constants.ContextKeyUserID).(string)
			operatorID, _ := r.Context().Value(constants.ContextKeyAppID).(string)
			certFingerprint := extractCertFingerprint(r)

			g.StoreSuspendedTransaction(ctx, hash, envelopeBytes, req.SkillName, req.Payload, userID, operatorID, certFingerprint)

			approvalURL := fmt.Sprintf("%s/approve/%s", g.publicBaseURL, hash)
			return A2ASuspensionResponse{
				ID:          hash,
				Status:      string(constants.GatewayResponseStatusSuspended),
				TxHash:      hash,
				ApprovalURL: approvalURL,
				Message:     approvalPausedMessage(approvalURL),
			}, nil
		}
		return nil, err
	}

	return A2ASuccessResponse{
		ID:     hash,
		Result: receipt,
	}, nil
}

// mapGatewayError maps governance verification errors to granular JSON-RPC codes.
func (g *GatewayService) mapGatewayError(err error) (int, string) {
	if err == nil {
		return 0, ""
	}

	msg := err.Error()

	switch {
	case errors.Is(err, governance.ErrInvalidEnvelope),
		errors.Is(err, governance.ErrTransactionIDMissing),
		errors.Is(err, governance.ErrPayloadMissing),
		errors.Is(err, governance.ErrUnknownActionType):
		return constants.ErrCodeInvalidEnvelope, msg

	case errors.Is(err, governance.ErrPayloadDecodeFailed):
		return constants.ErrCodePayloadDecodeFailed, msg

	case errors.Is(err, governance.ErrTransactionHashMissing),
		errors.Is(err, governance.ErrTransactionHashMismatch):
		return constants.ErrCodeHashMismatch, msg

	case errors.Is(err, governance.ErrTransactionExpired):
		return constants.ErrCodeExpired, msg

	case errors.Is(err, governance.ErrTransactionReplay):
		return constants.ErrCodeReplay, msg

	case errors.Is(err, governance.ErrStateRootMissing),
		errors.Is(err, governance.ErrStateRootRequired),
		errors.Is(err, governance.ErrStateRootMismatch):
		return constants.ErrCodeStateMismatch, msg

	case errors.Is(err, governance.ErrL1ValidationFailed):
		return constants.ErrCodeL1ValidationFailed, msg

	case errors.Is(err, governance.ErrL2SignatureMissing),
		errors.Is(err, governance.ErrL2SignatureInvalid),
		errors.Is(err, governance.ErrL2ConsensusNotConfigured),
		errors.Is(err, governance.ErrL2QuorumNotMet),
		errors.Is(err, governance.ErrL2DuplicateSigner):
		return constants.ErrCodeL2SignatureInvalid, msg

	case errors.Is(err, governance.ErrL3ProofInvalid),
		errors.Is(err, governance.ErrL3NotaryNotConfigured):
		return constants.ErrCodeL3ProofInvalid, msg
	}

	// Map other Gateway errors back to JSON-RPC error
	return -32603, msg
}

// extractCertFingerprint extracts the SHA-256 fingerprint of the client certificate from an HTTP request.
// Returns empty string if no valid certificate is present.
func extractCertFingerprint(r *http.Request) string {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return ""
	}
	cert := r.TLS.PeerCertificates[0]
	hash := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(hash[:])
}

// StoreSuspendedTransaction stores a transaction awaiting L3 approval.
func (g *GatewayService) StoreSuspendedTransaction(ctx context.Context, txHash string, envelope []byte, toolName string, toolArgs json.RawMessage, userID, operatorID string, certFingerprint string) {
	if g.suspendedStore == nil {
		return
	}
	cliSessionID, _ := ctx.Value(constants.ContextKeyCLISessionID).(string)
	tx := &models.SuspendedTransaction{
		TransactionHash:         txHash,
		Envelope:                json.RawMessage(envelope),
		CreatedAt:               time.Now().UTC(),
		ExpiresAt:               time.Now().UTC().Add(ApprovalRequestTTL),
		ToolName:                toolName,
		ToolArguments:           toolArgs,
		UserID:                  userID,
		OperatorID:              operatorID,
		SubmitterCLISessionID:   cliSessionID,
		ExpectedCertFingerprint: certFingerprint,
	}
	if err := g.suspendedStore.StoreSuspendedTransaction(ctx, tx); err != nil {
		g.logger.Error("Failed to store suspended transaction", "tx_hash", txHash, "error", err)
		return
	}

	// Emit approval.requested event to the originating session's audit-vault chain
	// This is the opening bookend for the approval lifecycle
	operatorSessionID := operatorID
	if operatorSessionID == "" {
		operatorSessionID = userID
	}

	if operatorSessionID != "" {
		event := &storage.Event{
			OperatorSessionID: operatorSessionID,
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.Notary.ApprovalRequested,
			ContentText:       fmt.Sprintf("Transaction %s approval requested (tool: %s, expires at: %s)", txHash, toolName, tx.ExpiresAt.Format(time.RFC3339)),
		}
		if _, err := g.auditStore.RecordEvent(event); err != nil {
			g.logger.Warn("Failed to record approval requested event", "error", err, "tx_hash", txHash, "operator_session_id", operatorSessionID)
		}
	}
}

// GetSuspendedTransaction retrieves a suspended transaction by hash.
func (g *GatewayService) GetSuspendedTransaction(ctx context.Context, txHash string) (*models.SuspendedTransaction, bool, error) {
	if g.suspendedStore == nil {
		return nil, false, nil
	}
	return g.suspendedStore.GetSuspendedTransaction(ctx, txHash)
}

// DeleteSuspendedTransaction removes a suspended transaction after approval/rejection.
func (g *GatewayService) DeleteSuspendedTransaction(ctx context.Context, txHash string) {
	if g.suspendedStore == nil {
		return
	}
	if err := g.suspendedStore.DeleteSuspendedTransaction(ctx, txHash); err != nil {
		g.logger.Error("Failed to delete suspended transaction", "tx_hash", txHash, "error", err)
	}
}

// ResumeWithL3Proof re-submits a suspended transaction with an attached L3
// WebAuthn proof through the g8e Gateway. The proof is verified
// inside the Gateway's TransactionVerifier - this method does not perform
// independent passkey validation, it only re-wires the envelope and calls
// the same fail-closed entry point used for primary submission.
//
// The signed receipt returned by the Gateway is forwarded to the caller so
// the OOB approval UI can surface the downstream tool result to the user.
func (g *GatewayService) ResumeWithL3Proof(ctx context.Context, txHash, userID string, proof *commonv1.L3Proof) (*operatorv1.ActionReceipt, error) {
	if g.envProc == nil {
		return nil, constants.ErrGatewayNotReady
	}
	if proof == nil {
		return nil, constants.ErrGatewayL3ProofRequired
	}
	if userID == "" {
		return nil, constants.ErrGatewayUserIDRequired
	}

	tx, ok, err := g.GetSuspendedTransaction(ctx, txHash)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrInternal)
	}
	if !ok {
		// The maintenance sweep now owns expiry event recording.
		// ResumeWithL3Proof cannot positively confirm the not-found reason
		// (expired vs never-existed vs already-approved), so it returns
		// ErrTransactionExpired without writing to the audit vault.
		return nil, fmt.Errorf("gateway: %w", constants.ErrTransactionExpired)
	}

	// Re-parse the stored envelope JSON so we can attach L3 metadata without
	// touching the hashed fields.
	env := &commonv1.GovernanceEnvelope{}
	if err := protojson.Unmarshal([]byte(tx.Envelope), env); err != nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	env.OperatorId = userID
	if env.Governance == nil {
		env.Governance = &commonv1.GovernanceMetadata{}
	}
	env.Governance.L3 = &commonv1.L3Metadata{Proof: proof}

	resubmitted, err := (protojson.MarshalOptions{Multiline: false}).Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	receipt, procErr := g.envProc.ProcessEnvelope(ctx, resubmitted)
	if procErr != nil {
		// Keep the suspension in place so the user can retry the proof
		// without re-issuing the upstream MCP call.
		return receipt, procErr
	}

	// Successful execution - remove from the suspension list.
	g.DeleteSuspendedTransaction(ctx, txHash)
	return receipt, nil
}

// DispatchToDownstream forwards a verified MCP tool call to the downstream MCP server.
// This implements the Actuator Egress phase for MCP protocol translation.
// Native tools are executed locally so every call — native or downstream — passes through
// the full L1-L5 governance pipeline and produces a signed receipt.
func (g *GatewayService) DispatchToDownstream(ctx context.Context, toolName string, toolArgs json.RawMessage, operatorSessionID string) (string, error) {
	// Handle read_field tool locally (JIT field resolution)
	if toolName == "read_field" {
		result, err := g.handleReadField(ctx, toolArgs)
		if err != nil {
			return "", err
		}
		// Extract text content for summary
		callResult, ok := result.(CallToolResult)
		if !ok {
			return "", fmt.Errorf("gateway: %w", constants.ErrInternal)
		}
		var sb strings.Builder
		for _, c := range callResult.Content {
			if c.Type == "text" {
				sb.WriteString(c.Text)
				sb.WriteString("\n")
			}
		}
		summary := strings.TrimRight(sb.String(), "\n")
		if summary == "" {
			summary = "completed"
		}
		return summary, nil
	}

	if g.isNativeTool(toolName) && g.nativeToolHandler != nil {
		result, err := g.nativeToolHandler.HandleTool(ctx, toolName, toolArgs)
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		for _, c := range result.Content {
			if c.Type == "text" {
				sb.WriteString(c.Text)
				sb.WriteString("\n")
			}
		}
		summary := strings.TrimRight(sb.String(), "\n")
		if summary == "" {
			summary = "completed"
		}

		// Scrub native tool output to prevent sensitive data leakage
		if g.scrubbingService != nil {
			summary = g.scrubbingService.ScrubText(summary)
		}

		return summary, nil
	}

	if g.downstreamURL == "" {
		return "", constants.ErrGatewayNoDownstreamConfigured
	}

	if g.isCircuitOpen() {
		return "", constants.ErrGatewayDownstreamUnavailable
	}

	// Construct MCP tools/call request with proper params envelope.
	// The MCP tools/call protocol requires Params: {"name": toolName, "arguments": toolArgs}.
	// Sending raw toolArgs as Params causes downstream servers to fail tool name lookup.
	mcpParams, err := json.Marshal(CallToolRequest{
		Name:      toolName,
		Arguments: toolArgs,
	})
	if err != nil {
		return "", fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	mcpReq := &response.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  mcpParams,
		ID:      1,
	}

	reqBody, err := json.Marshal(mcpReq)
	if err != nil {
		return "", fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(g.downstreamURL, "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		g.recordFailure()
		return "", fmt.Errorf("gateway: %w", constants.ErrGatewayDownstreamUnavailable)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			g.logger.Error("Failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode >= 500 {
			g.recordFailure()
		}
		return "", fmt.Errorf("gateway: %w", constants.ErrGatewayDownstreamHTTPError)
	}

	g.recordSuccess()

	// Parse MCP response
	var mcpResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&mcpResp); err != nil {
		return "", fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	if mcpResp.Error != nil {
		return "", fmt.Errorf("gateway: %w", constants.ErrGatewayMCPError)
	}

	// Extract result from MCP response
	var callResult CallToolResult
	if err := json.Unmarshal(mcpResp.Result, &callResult); err != nil {
		return "", fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	// Concatenate text content for result summary
	var summary strings.Builder
	for _, content := range callResult.Content {
		if content.Type == "text" {
			summary.WriteString(content.Text)
			summary.WriteString("\n")
		}
	}

	resultSummary := summary.String()
	if resultSummary == "" {
		resultSummary = "completed"
	}

	// Scrub downstream MCP output to prevent sensitive data leakage
	if g.scrubbingService != nil {
		resultSummary = g.scrubbingService.ScrubText(resultSummary)
	}

	// Audit downstream MCP call execution
	event := &storage.Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.McpCall,
		ContentText:       toolName,
		CommandRaw:        string(toolArgs),
		CommandStdout:     resultSummary,
	}
	if _, err := g.auditStore.RecordEvent(event); err != nil {
		g.logger.Warn("Failed to record downstream MCP call event in audit store", "error", err, "tool", toolName)
	}

	return resultSummary, nil
}

// DispatchToA2ADownstream forwards a verified A2A call to the downstream A2A server.
func (g *GatewayService) DispatchToA2ADownstream(ctx context.Context, skillName string, payload json.RawMessage) (string, error) {
	if g.a2aDownstreamURL == "" {
		return "", constants.ErrGatewayNoA2ADownstreamConfigured
	}

	// TODO: Circuit breaker for A2A if needed. For now we use the same cooldown logic
	// if we decide to share the state or add a separate one.
	if g.isCircuitOpen() {
		return "", constants.ErrGatewayDownstreamUnavailable
	}

	// Construct A2A request
	a2aReq := A2ADownstreamRequest{
		SkillName: skillName,
		Payload:   payload,
	}

	reqBody, err := json.Marshal(a2aReq)
	if err != nil {
		return "", fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(g.a2aDownstreamURL, "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		g.recordFailure()
		return "", fmt.Errorf("gateway: %w", constants.ErrGatewayDownstreamUnavailable)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			g.logger.Error("Failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode >= 500 {
			g.recordFailure()
		}
		return "", fmt.Errorf("gateway: %w", constants.ErrGatewayDownstreamHTTPError)
	}

	g.recordSuccess()

	// Parse A2A response
	var a2aResp struct {
		Result  string `json:"result"`
		Error   string `json:"error,omitempty"`
		Summary string `json:"summary,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&a2aResp); err != nil {
		return "", fmt.Errorf("gateway: %w", constants.ErrInternal)
	}

	if a2aResp.Error != "" {
		return "", fmt.Errorf("gateway: %w", constants.ErrGatewayA2AError)
	}

	if a2aResp.Summary != "" {
		return a2aResp.Summary, nil
	}

	if a2aResp.Result != "" {
		return a2aResp.Result, nil
	}

	return "completed", nil
}
