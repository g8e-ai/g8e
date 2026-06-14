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
	"github.com/g8e-ai/g8e/internal/interfaces"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	govpkg "github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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
	envProc           governance.EnvelopeProcessor
	stateRootProvider StateRootProvider
	signingKey        ed25519.PrivateKey
	keyID             string
	downstreamURL     string
	a2aDownstreamURL  string
	publicBaseURL     string
	suspendedStore    interfaces.SuspendedTransactionStore
	fieldPathRegistry *FieldPathRegistry
	dbService         FieldReader
	sessionValidator  SessionValidator
	auditLogger       AuditLogger
	auditStore        *storage.SQLAuditStore
	nativeToolHandler *NativeToolHandler
	scrubbingService  *scrubbing.ScrubbingService
	posture           string // Gateway posture: doctrine, consensus, or notary

	// Circuit breaker state
	mu               sync.RWMutex
	failureCount     int
	lastFailure      time.Time
	circuitOpen      bool
	cooldownDuration time.Duration
	maxFailures      int

	maxPayloadBytes int64
}

// FieldReader provides read access to individual document fields, backing the
// MCP gateway's read_field operation. It is implemented by the gateway's
// document store (DocumentStoreService).
type FieldReader interface {
	GetField(collection, id, fieldPath string) (interface{}, error)
}

// SessionValidator validates Operator sessions for L3 authorization
type SessionValidator interface {
	ValidateSession(operatorSessionID string) (bool, error)
}

// AuditLogger logs field read operations to the audit vault
type AuditLogger interface {
	LogFieldRead(operatorSessionID, collection, documentID, fieldPath string, value interface{}) error
}

// Dependencies groups all dependencies for NewGatewayService to reduce constructor bloat.
type Dependencies struct {
	Logger           *slog.Logger
	Responder        *response.Writer
	SuspendedStore   interfaces.SuspendedTransactionStore
	ScrubbingService *scrubbing.ScrubbingService
	MaxPayloadBytes  int64
	Posture          string // Gateway posture: doctrine, consensus, or notary
}

func NewGatewayService(deps Dependencies) (*GatewayService, error) {
	// Validate posture parameter
	validPostures := map[string]bool{
		"doctrine":  true,
		"consensus": true,
		"notary":    true,
	}
	if deps.Posture != "" && !validPostures[deps.Posture] {
		return nil, fmt.Errorf("invalid posture '%s': must be one of doctrine, consensus, or notary", deps.Posture)
	}

	fieldPathRegistry, err := NewFieldPathRegistry(deps.Logger)
	if err != nil {
		deps.Logger.Error("Failed to initialize field path registry", "error", err)
		// Continue without field path registry - read_field will be disabled
	}

	nativeToolHandler, err := NewNativeToolHandler(deps.Logger)
	if err != nil {
		return nil, fmt.Errorf("initialize native tool handler: %w", err)
	}

	g := &GatewayService{
		logger:            deps.Logger,
		responder:         deps.Responder,
		suspendedStore:    deps.SuspendedStore,
		fieldPathRegistry: fieldPathRegistry,
		nativeToolHandler: nativeToolHandler,
		scrubbingService:  deps.ScrubbingService,
		posture:           deps.Posture,
		maxFailures:       5,
		cooldownDuration:  1 * time.Minute,
		maxPayloadBytes:   deps.MaxPayloadBytes,
	}
	return g, nil
}

// RunMaintenance periodically prunes expired suspended transactions.
// Although the underlying store may perform its own cleanup (e.g., CanonicalDBService
// does this via RunMaintenance), the GatewayService provides this routine to
// ensure memory and state consistency regardless of the store implementation.
func (g *GatewayService) RunMaintenance(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// If the store is the CanonicalDBService, it already prunes.
			// If it's another implementation, we might need an explicit cleanup call.
			// For now, we rely on the store's internal expiration logic during GET,
			// but we can add an explicit DELETE call here if the interface is expanded.
			g.logger.Debug("Gateway maintenance: pruning expired transactions")
		}
	}
}

func (g *GatewayService) SetDependencies(p governance.EnvelopeProcessor, srp StateRootProvider, key ed25519.PrivateKey, keyID string, downstreamURL string) {
	g.envProc = p
	g.stateRootProvider = srp
	g.signingKey = key
	g.keyID = keyID
	g.downstreamURL = downstreamURL
}

func (g *GatewayService) SetA2ADependencies(downstreamURL string) {
	g.a2aDownstreamURL = downstreamURL
}

func (g *GatewayService) SetPublicBaseURL(baseURL string) {
	g.publicBaseURL = baseURL
}

func (g *GatewayService) SetDBService(dbService FieldReader) {
	g.dbService = dbService
}

// SetSessionValidator sets the L3 session validator for field read operations
func (g *GatewayService) SetSessionValidator(validator SessionValidator) {
	g.sessionValidator = validator
}

// SetAuditLogger sets the audit logger for field read operations
func (g *GatewayService) SetAuditLogger(logger AuditLogger) {
	g.auditLogger = logger
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

// @Summary		List MCP tools
// @Description	Returns the list of available MCP tools
// @Tags			mcp
// @Accept			json
// @Produce		json
// @Success		200	{object}	map[string]interface{}
// @Router			/api/v1/mcp/tools/list [get]
func (g *GatewayService) HandleToolsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if g.downstreamURL == "" {
		// Return native tools if no downstream configured
		var nativeTools []NativeTool
		if g.nativeToolHandler != nil {
			nativeTools = g.nativeToolHandler.registry.List()
		}
		tools := make([]Tool, 0, len(nativeTools))
		for _, nt := range nativeTools {
			tools = append(tools, Tool{
				Name:        nt.Name(),
				Description: nt.Description(),
				InputSchema: nt.InputSchema().ToMap(),
			})
		}
		g.responder.RPCResponse(w, 1, ToolsListResult{Tools: tools})
		return
	}

	if g.isCircuitOpen() {
		g.logger.Warn("MCP downstream circuit is open, returning native tools only", "url", g.downstreamURL)
		// Return native tools when downstream is unavailable
		var nativeTools []NativeTool
		if g.nativeToolHandler != nil {
			nativeTools = g.nativeToolHandler.registry.List()
		}
		tools := make([]Tool, 0, len(nativeTools))
		for _, nt := range nativeTools {
			tools = append(tools, Tool{
				Name:        nt.Name(),
				Description: nt.Description(),
				InputSchema: nt.InputSchema().ToMap(),
			})
		}
		g.responder.RPCResponse(w, 1, ToolsListResult{Tools: tools})
		return
	}

	// Proxy to downstream MCP server
	client := &http.Client{Timeout: 10 * time.Second}
	var resp *http.Response
	var err error
	var body []byte

	// For MCP tools/list, we usually expect a JSON-RPC request even for listing
	// If it's a GET, we might be looking for SSE or a simplified discovery
	if r.Method == http.MethodPost {
		// Ensure we pass through the body if present, or create a valid tools/list request
		body, err = io.ReadAll(r.Body)
		if err != nil {
			g.logger.Error("Failed to read request body", "error", err)
			g.responder.RPCError(w, 1, -32603, "failed to read request body")
			return
		}
		if len(body) == 0 {
			body = []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
		}
		resp, err = client.Post(g.downstreamURL, "application/json", strings.NewReader(string(body)))
	} else {
		// Map GET to a standard tools/list POST if the downstream is a standard MCP server
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
		resp, err = client.Post(g.downstreamURL, "application/json", strings.NewReader(reqBody))
	}

	if err != nil {
		g.recordFailure()
		g.logger.Error("Failed to query downstream MCP server, returning native tools only", "url", g.downstreamURL, "error", err)
		// Return native tools when downstream is unavailable
		var nativeTools []NativeTool
		if g.nativeToolHandler != nil {
			nativeTools = g.nativeToolHandler.registry.List()
		}
		tools := make([]Tool, 0, len(nativeTools))
		for _, nt := range nativeTools {
			tools = append(tools, Tool{
				Name:        nt.Name(),
				Description: nt.Description(),
				InputSchema: nt.InputSchema().ToMap(),
			})
		}
		g.responder.RPCResponse(w, 1, ToolsListResult{Tools: tools})
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			g.logger.Error("Failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode >= 500 {
		g.recordFailure()
		// Return native tools as fallback when downstream returns error
		var nativeTools []NativeTool
		if g.nativeToolHandler != nil {
			nativeTools = g.nativeToolHandler.registry.List()
		}
		tools := make([]Tool, 0, len(nativeTools))
		for _, nt := range nativeTools {
			tools = append(tools, Tool{
				Name:        nt.Name(),
				Description: nt.Description(),
				InputSchema: nt.InputSchema().ToMap(),
			})
		}
		g.responder.RPCResponse(w, 1, ToolsListResult{Tools: tools})
		return
	} else {
		g.recordSuccess()
	}

	// Parse downstream response
	var downstreamJSONRPC JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&downstreamJSONRPC); err != nil {
		g.logger.Error("Failed to decode downstream tools/list response, returning native tools only", "error", err)
		// Return native tools as fallback when response is invalid
		var nativeTools []NativeTool
		if g.nativeToolHandler != nil {
			nativeTools = g.nativeToolHandler.registry.List()
		}
		tools := make([]Tool, 0, len(nativeTools))
		for _, nt := range nativeTools {
			tools = append(tools, Tool{
				Name:        nt.Name(),
				Description: nt.Description(),
				InputSchema: nt.InputSchema().ToMap(),
			})
		}
		g.responder.RPCResponse(w, 1, ToolsListResult{Tools: tools})
		return
	}

	// Extract result from JSON-RPC response
	var downstreamResult ToolsListResult
	if downstreamJSONRPC.Result != nil {
		resultBytes, err := json.Marshal(downstreamJSONRPC.Result)
		if err != nil {
			g.logger.Error("Failed to marshal downstream result, returning native tools only", "error", err)
			var nativeTools []NativeTool
			if g.nativeToolHandler != nil {
				nativeTools = g.nativeToolHandler.registry.List()
			}
			tools := make([]Tool, 0, len(nativeTools))
			for _, nt := range nativeTools {
				tools = append(tools, Tool{
					Name:        nt.Name(),
					Description: nt.Description(),
					InputSchema: nt.InputSchema().ToMap(),
				})
			}
			g.responder.RPCResponse(w, 1, ToolsListResult{Tools: tools})
			return
		}
		if err := json.Unmarshal(resultBytes, &downstreamResult); err != nil {
			g.logger.Error("Failed to unmarshal downstream result, returning native tools only", "error", err)
			var nativeTools []NativeTool
			if g.nativeToolHandler != nil {
				nativeTools = g.nativeToolHandler.registry.List()
			}
			tools := make([]Tool, 0, len(nativeTools))
			for _, nt := range nativeTools {
				tools = append(tools, Tool{
					Name:        nt.Name(),
					Description: nt.Description(),
					InputSchema: nt.InputSchema().ToMap(),
				})
			}
			g.responder.RPCResponse(w, 1, ToolsListResult{Tools: tools})
			return
		}
	}

	// Merge native tools with downstream tools
	var nativeTools []NativeTool
	if g.nativeToolHandler != nil {
		nativeTools = g.nativeToolHandler.registry.List()
	}

	// Create a map of downstream tool names for deduplication
	downstreamToolNames := make(map[string]bool)
	for _, tool := range downstreamResult.Tools {
		downstreamToolNames[tool.Name] = true
	}

	// Add native tools that aren't in downstream
	for _, nt := range nativeTools {
		if !downstreamToolNames[nt.Name()] {
			downstreamResult.Tools = append(downstreamResult.Tools, Tool{
				Name:        nt.Name(),
				Description: nt.Description(),
				InputSchema: nt.InputSchema().ToMap(),
			})
		}
	}

	g.responder.RPCResponse(w, 1, downstreamResult)
}

// @Summary		List MCP resources
// @Description	Returns the list of available MCP resources
// @Tags			mcp
// @Accept			json
// @Produce		json
// @Success		200	{object}	map[string]interface{}
// @Router			/api/v1/mcp/resources/list [get]
func (g *GatewayService) HandleResourcesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if g.downstreamURL == "" {
		// Mock response if no downstream configured
		g.responder.RPCResponse(w, 1, ResourcesListResult{Resources: []Resource{}})
		return
	}

	if g.isCircuitOpen() {
		g.logger.Warn("MCP downstream circuit is open, rejecting resources/list", "url", g.downstreamURL)
		g.responder.RPCError(w, 1, -32603, "downstream MCP server is temporarily unavailable (circuit open)")
		return
	}

	// Proxy to downstream MCP server
	client := &http.Client{Timeout: 10 * time.Second}
	var resp *http.Response
	var err error
	var body []byte

	if r.Method == http.MethodPost {
		body, err = io.ReadAll(r.Body)
		if err != nil {
			g.logger.Error("Failed to read request body", "error", err)
			g.responder.RPCError(w, 1, -32603, "failed to read request body")
			return
		}
		if len(body) == 0 {
			body = []byte(`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`)
		}
		resp, err = client.Post(g.downstreamURL, "application/json", strings.NewReader(string(body)))
	} else {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`
		resp, err = client.Post(g.downstreamURL, "application/json", strings.NewReader(reqBody))
	}

	if err != nil {
		g.recordFailure()
		g.logger.Error("Failed to query downstream MCP server", "url", g.downstreamURL, "error", err)
		g.responder.RPCError(w, 1, -32603, fmt.Sprintf("failed to query downstream MCP server: %v", err))
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			g.logger.Error("Failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode >= 500 {
		g.recordFailure()
	} else {
		g.recordSuccess()
	}

	w.Header().Set(constants.HeaderContentType, "application/json")
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		g.logger.Error("Failed to copy response body", "error", err)
	}
}

func (g *GatewayService) handleMCPRequest(w http.ResponseWriter, r *http.Request, method string, handler func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error)) {
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
		g.responder.RPCError(w, nil, -32700, "parse error: invalid JSON")
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

// @Summary		Call MCP tool
// @Description	Calls an MCP tool with the provided arguments
// @Tags			mcp
// @Accept			json
// @Produce		json
// @Success		200	{object}	map[string]interface{}
// @Router			/api/v1/mcp/tools/call [post]
func (g *GatewayService) HandleToolsCall(w http.ResponseWriter, r *http.Request) {
	g.handleMCPRequest(w, r, "tools/call", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
		return g.callTool(ctx, r, params)
	})
}

// callTool executes a governed MCP tools/call. It is shared by the per-method
// REST handler (HandleToolsCall) and the unified Streamable HTTP dispatcher
// (HandleMCP). The *http.Request is used only to extract identity headers and
// the client certificate fingerprint when a transaction is suspended for L3
// approval.
func (g *GatewayService) callTool(ctx context.Context, r *http.Request, params json.RawMessage) (interface{}, error) {
	var callParams CallToolRequest
	if err := json.Unmarshal(params, &callParams); err != nil {
		return nil, fmt.Errorf("invalid tools/call params: %w", err)
	}

	if callParams.Name == "" {
		return nil, errors.New("tool name required")
	}

	argumentsJSON := "{}"
	if len(callParams.Arguments) > 0 {
		var probe interface{}
		if err := json.Unmarshal(callParams.Arguments, &probe); err != nil {
			return nil, errors.New("invalid tool arguments: must be a valid JSON object")
		}
		argumentsJSON = string(callParams.Arguments)
	}

	mcpPayload := &operatorv1.McpCallRequested{
		ToolName:      callParams.Name,
		ArgumentsJson: argumentsJSON,
		ExecutionId:   uuid.New().String(),
	}
	payloadBytes, err := proto.Marshal(mcpPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal MCP payload: %w", err)
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
						Text: fmt.Sprintf("Execution paused. Please visit %s to authorize via WebAuthn, then retry.", approvalURL),
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
		return nil, errors.New("field path registry not initialized")
	}

	if g.dbService == nil {
		return nil, errors.New("database service not configured")
	}

	var req FieldReadRequest
	if err := json.Unmarshal(arguments, &req); err != nil {
		return nil, fmt.Errorf("invalid read_field arguments: %w", err)
	}

	// Validate required fields
	if req.Collection == "" {
		return nil, errors.New("collection required")
	}
	if req.DocumentID == "" {
		return nil, errors.New("document_id required")
	}
	if req.FieldPath == "" {
		return nil, errors.New("field_path required")
	}
	if req.OperatorSessionID == "" {
		return nil, errors.New("operator_session_id required")
	}

	// L1: Validate field path against schema registry
	if err := g.fieldPathRegistry.ValidateFieldPath(req.Collection, req.FieldPath); err != nil {
		return nil, fmt.Errorf("field path validation failed: %w", err)
	}

	// L3: Validate Operator session
	if g.sessionValidator != nil {
		valid, err := g.sessionValidator.ValidateSession(req.OperatorSessionID)
		if err != nil {
			return nil, fmt.Errorf("session validation failed: %w", err)
		}
		if !valid {
			return nil, errors.New("operator session is invalid or expired")
		}
	}

	// Extract field value from database
	value, err := g.dbService.GetField(req.Collection, req.DocumentID, req.FieldPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get field: %w", err)
	}

	// L1: Scan field value for forbidden patterns
	if err := g.scanForForbiddenPatterns(value); err != nil {
		return nil, fmt.Errorf("field value contains forbidden patterns: %w", err)
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
				Text: fmt.Sprintf("%v", value),
			},
		},
	}, nil
}

// scanForbiddenPatterns checks if a value contains forbidden patterns (L1 hard gates)
func (g *GatewayService) scanForForbiddenPatterns(value interface{}) error {
	if value == nil {
		return nil
	}

	valueStr := fmt.Sprintf("%v", value)

	// Forbidden patterns from L1 hard gates with context describing the threat category
	forbiddenPatterns := []struct {
		pattern string
		context string
	}{
		{"sudo", "privilege escalation"},
		{"su ", "privilege escalation"},
		{"rm -rf /", "destructive file operation"},
		{"://", "external URL (potential exfiltration)"},
		{"password", "credential leak"},
		{"api_key", "credential leak"},
		{"secret", "credential leak"},
		{"token", "credential leak"},
		{"private_key", "credential leak"},
	}

	for _, fp := range forbiddenPatterns {
		if strings.Contains(strings.ToLower(valueStr), fp.pattern) {
			return fmt.Errorf("L1 hard gate: forbidden pattern detected (%s): %s", fp.context, fp.pattern)
		}
	}

	return nil
}

// @Summary		Read MCP resource
// @Description	Reads a specific MCP resource
// @Tags			mcp
// @Accept			json
// @Produce		json
// @Success		200	{object}	map[string]interface{}
// @Router			/api/v1/mcp/resources/read [post]
func (g *GatewayService) HandleResourcesRead(w http.ResponseWriter, r *http.Request) {
	g.handleMCPRequest(w, r, "resources/read", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
		return g.readResource(ctx, params)
	})
}

// readResource executes a governed MCP resources/read. Shared by the per-method
// REST handler (HandleResourcesRead) and the unified dispatcher (HandleMCP).
func (g *GatewayService) readResource(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var readParams ReadResourceRequest
	if err := json.Unmarshal(params, &readParams); err != nil {
		return nil, fmt.Errorf("invalid resources/read params: %w", err)
	}

	if readParams.URI == "" {
		return nil, errors.New("uri required")
	}

	mcpPayload := &operatorv1.McpResourceReadRequested{
		Uri:         readParams.URI,
		ExecutionId: uuid.New().String(),
	}
	payloadBytes, err := proto.Marshal(mcpPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal MCP payload: %w", err)
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

// @Summary		List MCP prompts
// @Description	Returns the list of available MCP prompts
// @Tags			mcp
// @Accept			json
// @Produce		json
// @Success		200	{object}	map[string]interface{}
// @Router			/api/v1/mcp/prompts/list [get]
func (g *GatewayService) HandlePromptsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if g.downstreamURL == "" {
		// Mock response if no downstream configured
		g.responder.RPCResponse(w, 1, PromptsListResult{Prompts: []Prompt{}})
		return
	}

	if g.isCircuitOpen() {
		g.logger.Warn("MCP downstream circuit is open, rejecting prompts/list", "url", g.downstreamURL)
		g.responder.RPCError(w, 1, -32603, "downstream MCP server is temporarily unavailable (circuit open)")
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	var resp *http.Response
	var err error
	var body []byte

	if r.Method == http.MethodPost {
		body, err = io.ReadAll(r.Body)
		if err != nil {
			g.logger.Error("Failed to read request body", "error", err)
			g.responder.RPCError(w, 1, -32603, "failed to read request body")
			return
		}
		if len(body) == 0 {
			body = []byte(`{"jsonrpc":"2.0","id":1,"method":"prompts/list"}`)
		}
		resp, err = client.Post(g.downstreamURL, "application/json", strings.NewReader(string(body)))
	} else {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"prompts/list"}`
		resp, err = client.Post(g.downstreamURL, "application/json", strings.NewReader(reqBody))
	}

	if err != nil {
		g.recordFailure()
		g.logger.Error("Failed to query downstream MCP server", "url", g.downstreamURL, "error", err)
		g.responder.RPCError(w, 1, -32603, fmt.Sprintf("failed to query downstream MCP server: %v", err))
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			g.logger.Error("Failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode >= 500 {
		g.recordFailure()
	} else {
		g.recordSuccess()
	}

	w.Header().Set(constants.HeaderContentType, "application/json")
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		g.logger.Error("Failed to copy response body", "error", err)
	}
}

// @Summary		Get MCP prompt
// @Description	Returns a specific MCP prompt template
// @Tags			mcp
// @Accept			json
// @Produce		json
// @Success		200	{object}	map[string]interface{}
// @Router			/api/v1/mcp/prompts/get [post]
func (g *GatewayService) HandlePromptsGet(w http.ResponseWriter, r *http.Request) {
	g.handleMCPRequest(w, r, "prompts/get", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
		return g.getPrompt(ctx, params)
	})
}

// getPrompt executes a governed MCP prompts/get. Shared by the per-method REST
// handler (HandlePromptsGet) and the unified dispatcher (HandleMCP).
func (g *GatewayService) getPrompt(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var getParams GetPromptRequest
	if err := json.Unmarshal(params, &getParams); err != nil {
		return nil, fmt.Errorf("invalid prompts/get params: %w", err)
	}

	if getParams.Name == "" {
		return nil, errors.New("name required")
	}

	mcpPayload := &operatorv1.McpPromptGetRequested{
		Name:        getParams.Name,
		ExecutionId: uuid.New().String(),
	}
	payloadBytes, err := proto.Marshal(mcpPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal MCP payload: %w", err)
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

// @Summary		Call MCP tool (SSE)
// @Description	Calls an MCP tool with SSE streaming response
// @Tags			mcp
// @Accept			json
// @Produce		text/event-stream
// @Success		200	{string}	string
// @Router			/api/v1/mcp/tools/call/sse [post]
func (g *GatewayService) HandleToolsCallSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if g.isCircuitOpen() {
		g.logger.Warn("MCP downstream circuit is open, rejecting SSE request", "url", g.downstreamURL)
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
		g.responder.RPCError(w, nil, -32700, "parse error: invalid JSON")
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

	if req.Method != "tools/call" {
		g.responder.RPCError(w, req.ID, -32601, fmt.Sprintf("method not found: expected tools/call, got %s", req.Method))
		return
	}

	var callParams CallToolRequest
	if err := json.Unmarshal(req.Params, &callParams); err != nil {
		g.responder.RPCError(w, req.ID, -32600, fmt.Sprintf("invalid tools/call params: %v", err))
		return
	}

	if callParams.Name == "" {
		g.responder.RPCError(w, req.ID, -32600, "tool name required")
		return
	}

	argumentsJSON := "{}"
	if len(callParams.Arguments) > 0 {
		var probe interface{}
		if err := json.Unmarshal(callParams.Arguments, &probe); err != nil {
			g.responder.RPCError(w, req.ID, -32600, "invalid tool arguments")
			return
		}
		argumentsJSON = string(callParams.Arguments)
	}

	mcpPayload := &operatorv1.McpCallRequested{
		ToolName:      callParams.Name,
		ArgumentsJson: argumentsJSON,
		ExecutionId:   uuid.New().String(),
	}
	payloadBytes, err := proto.Marshal(mcpPayload)
	if err != nil {
		g.responder.RPCError(w, req.ID, -32603, fmt.Sprintf("failed to marshal MCP payload: %v", err))
		return
	}

	hash, envelopeBytes, err := g.processGatewayTransaction(r.Context(), processGatewayOptions{
		actionType:     constants.ActionTypeMcpCall,
		targetResource: callParams.Name,
		payloadBytes:   payloadBytes,
	})
	if err != nil {
		code, msg := g.mapGatewayError(err)
		if code == 0 && msg == "" {
			code = -32603
			msg = err.Error()
		}
		g.responder.RPCError(w, req.ID, code, msg)
		return
	}

	receipt, err := g.envProc.ProcessEnvelope(r.Context(), envelopeBytes)
	if err != nil {
		if errors.Is(err, governance.ErrL3ProofMissing) {
			userID, _ := r.Context().Value(constants.ContextKeyUserID).(string)
			operatorID, _ := r.Context().Value(constants.ContextKeyAppID).(string)
			certFingerprint := extractCertFingerprint(r)

			g.StoreSuspendedTransaction(r.Context(), hash, envelopeBytes, callParams.Name, callParams.Arguments, userID, operatorID, certFingerprint)

			approvalURL := fmt.Sprintf("%s/approve/%s", g.publicBaseURL, hash)
			g.responder.RPCResponse(w, req.ID, CallToolResult{
				Content: []TextContent{
					{
						Type: "text",
						Text: fmt.Sprintf("Execution paused. Please visit %s to authorize via WebAuthn, then retry.", approvalURL),
					},
				},
			})
			return
		}
		code, msg := g.mapGatewayError(err)
		if code == 0 && msg == "" {
			code = -32603
			msg = err.Error()
		}
		g.responder.RPCError(w, req.ID, code, msg)
		return
	}

	// Set SSE headers before writing response
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		g.responder.RPCError(w, req.ID, -32603, "streaming not supported")
		return
	}

	chunk := CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: receipt.ResultSummary,
			},
		},
	}
	if receipt.Status != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
		chunk.IsError = true
	}

	chunkBytes, err := json.Marshal(chunk)
	if err != nil {
		g.logger.Error("Failed to marshal SSE chunk", "error", err)
		return
	}

	fmt.Fprintf(w, "data: %s\n\n", chunkBytes)
	flusher.Flush()
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
		Nonce:           uuid.New().String(),
		StateMerkleRoot: stateRoot,
		Governance: &commonv1.GovernanceMetadata{
			GatewaySigned: true,
		},
	}

	// In doctrine and consensus postures, L3 is audited not enforced, so we auto-approve
	// to avoid WebAuthn prompts for local MCP agents. In notary posture, L3 is strictly
	// enforced and requires human authorization.
	if g.posture == "doctrine" || g.posture == "consensus" {
		if env.Governance == nil {
			env.Governance = &commonv1.GovernanceMetadata{}
		}
		env.Governance.L3 = &commonv1.L3Metadata{
			AutoApproved: true,
		}
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
		return "", nil, fmt.Errorf("failed to compute transaction hash: %w", err)
	}
	env.Id = hash
	env.TransactionHash = hash

	if len(g.signingKey) > 0 {
		l2Payload := fmt.Sprintf("%s|true", hash)
		sig := ed25519.Sign(g.signingKey, []byte(l2Payload))
		if env.Governance == nil {
			env.Governance = &commonv1.GovernanceMetadata{}
		}
		env.Governance.L2 = &commonv1.L2Metadata{
			ConsensusSignature: hex.EncodeToString(sig),
			KeyId:              g.keyID,
			AgentIds:           []string{"gateway-local-signer"},
		}
	}

	envelopeBytes, err = (protojson.MarshalOptions{Multiline: false}).Marshal(env)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal envelope: %w", err)
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
	g.handleMCPRequest(w, r, "a2a/call", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
		return g.a2aCall(ctx, r, params)
	})
}

// a2aCall executes a governed A2A skill invocation. Shared by the per-method
// REST handler (HandleA2aCall) and the unified dispatcher (HandleMCP).
func (g *GatewayService) a2aCall(ctx context.Context, r *http.Request, params json.RawMessage) (interface{}, error) {
	var req struct {
		SkillName   string          `json:"skill_name"`
		PayloadJSON json.RawMessage `json:"payload"`
		ExecutionID string          `json:"execution_id,omitempty"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid a2a/call params: %w", err)
	}

	if req.SkillName == "" {
		return nil, errors.New("skill_name required")
	}

	payloadStr := "{}"
	if len(req.PayloadJSON) > 0 {
		payloadStr = string(req.PayloadJSON)
	}

	a2aPayload := &operatorv1.A2ACallRequested{
		SkillName:   req.SkillName,
		PayloadJson: payloadStr,
		ExecutionId: req.ExecutionID,
	}
	payloadBytes, err := proto.Marshal(a2aPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal A2A payload: %w", err)
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

			g.StoreSuspendedTransaction(ctx, hash, envelopeBytes, req.SkillName, req.PayloadJSON, userID, operatorID, certFingerprint)

			approvalURL := fmt.Sprintf("%s/approve/%s", g.publicBaseURL, hash)
			return A2ASuspensionResponse{
				ID:          hash,
				Status:      "suspended",
				TxHash:      hash,
				ApprovalURL: approvalURL,
				Message:     "Execution paused for L3 authorization",
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
		errors.Is(err, governance.ErrL2KeyNotConfigured):
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
	tx := &models.SuspendedTransaction{
		TransactionHash:         txHash,
		Envelope:                json.RawMessage(envelope),
		CreatedAt:               time.Now().UTC(),
		ExpiresAt:               time.Now().UTC().Add(5 * time.Minute),
		ToolName:                toolName,
		ToolArguments:           toolArgs,
		UserID:                  userID,
		OperatorID:              operatorID,
		ExpectedCertFingerprint: certFingerprint,
	}
	if err := g.suspendedStore.StoreSuspendedTransaction(ctx, tx); err != nil {
		g.logger.Error("Failed to store suspended transaction", "tx_hash", txHash, "error", err)
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
		return nil, fmt.Errorf("g8e Gateway not ready")
	}
	if proof == nil {
		return nil, fmt.Errorf("L3 proof required")
	}
	if userID == "" {
		return nil, fmt.Errorf("user_id required")
	}

	tx, ok, err := g.GetSuspendedTransaction(ctx, txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get suspended transaction: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("suspended transaction %s not found or expired", txHash)
	}

	// Re-parse the stored envelope JSON so we can attach L3 metadata without
	// touching the hashed fields.
	env := &commonv1.GovernanceEnvelope{}
	if err := protojson.Unmarshal([]byte(tx.Envelope), env); err != nil {
		return nil, fmt.Errorf("failed to re-parse suspended envelope: %w", err)
	}

	env.OperatorId = userID
	if env.Governance == nil {
		env.Governance = &commonv1.GovernanceMetadata{}
	}
	env.Governance.L3 = &commonv1.L3Metadata{Proof: proof}

	resubmitted, err := (protojson.MarshalOptions{Multiline: false}).Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("failed to re-marshal resumed envelope: %w", err)
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
			return "", fmt.Errorf("read_field execution failed: %w", err)
		}
		// Extract text content for summary
		callResult, ok := result.(CallToolResult)
		if !ok {
			return "", fmt.Errorf("read_field returned unexpected type: %T", result)
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
			return "", fmt.Errorf("native tool execution failed: %w", err)
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
		return "", fmt.Errorf("no downstream MCP server configured")
	}

	if g.isCircuitOpen() {
		return "", fmt.Errorf("downstream MCP server is temporarily unavailable (circuit open)")
	}

	// Construct MCP tools/call request
	mcpReq := &response.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  toolArgs,
		ID:      1,
	}

	reqBody, err := json.Marshal(mcpReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal MCP request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(g.downstreamURL, "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		g.recordFailure()
		return "", fmt.Errorf("failed to call downstream MCP server: %w", err)
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
		return "", fmt.Errorf("downstream MCP server returned status %d", resp.StatusCode)
	}

	g.recordSuccess()

	// Parse MCP response
	var mcpResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&mcpResp); err != nil {
		return "", fmt.Errorf("failed to decode MCP response: %w", err)
	}

	if mcpResp.Error != nil {
		return "", fmt.Errorf("MCP error: %s", mcpResp.Error.Message)
	}

	// Extract result from MCP response
	var callResult CallToolResult
	if err := json.Unmarshal(mcpResp.Result, &callResult); err != nil {
		return "", fmt.Errorf("failed to unmarshal MCP result: %w", err)
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

	// Audit downstream MCP call execution
	if g.auditStore != nil {
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
	}

	return resultSummary, nil
}

// DispatchToA2ADownstream forwards a verified A2A call to the downstream A2A server.
func (g *GatewayService) DispatchToA2ADownstream(ctx context.Context, skillName string, payload json.RawMessage) (string, error) {
	if g.a2aDownstreamURL == "" {
		return "", fmt.Errorf("no downstream A2A server configured")
	}

	// TODO: Circuit breaker for A2A if needed. For now we use the same cooldown logic
	// if we decide to share the state or add a separate one.
	if g.isCircuitOpen() {
		return "", fmt.Errorf("downstream A2A server is temporarily unavailable (circuit open)")
	}

	// Construct A2A request
	a2aReq := A2ADownstreamRequest{
		SkillName:   skillName,
		PayloadJSON: payload,
	}

	reqBody, err := json.Marshal(a2aReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal A2A request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(g.a2aDownstreamURL, "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		g.recordFailure()
		return "", fmt.Errorf("failed to call downstream A2A server: %w", err)
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
		return "", fmt.Errorf("downstream A2A server returned status %d", resp.StatusCode)
	}

	g.recordSuccess()

	// Parse A2A response
	var a2aResp struct {
		Result  string `json:"result"`
		Error   string `json:"error,omitempty"`
		Summary string `json:"summary,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&a2aResp); err != nil {
		return "", fmt.Errorf("failed to decode A2A response: %w", err)
	}

	if a2aResp.Error != "" {
		return "", fmt.Errorf("A2A error: %s", a2aResp.Error)
	}

	if a2aResp.Summary != "" {
		return a2aResp.Summary, nil
	}

	if a2aResp.Result != "" {
		return a2aResp.Result, nil
	}

	return "completed", nil
}
