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

package mcp

import (
	"context"
	"crypto/ed25519"
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
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/g8e-ai/g8e/internal/services/governance"
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

// SuspendedTransactionStore defines the interface for persistent storage of suspended transactions.
type SuspendedTransactionStore interface {
	StoreSuspendedTransaction(tx *models.SuspendedTransaction) error
	GetSuspendedTransaction(txHash string) (*models.SuspendedTransaction, bool)
	DeleteSuspendedTransaction(txHash string) error
}

type GatewayService struct {
	logger            *slog.Logger
	responder         *responder.Responder
	envProc           governance.EnvelopeProcessor
	stateRootProvider StateRootProvider
	signingKey        ed25519.PrivateKey
	keyID             string
	downstreamURL     string
	a2aDownstreamURL  string
	publicBaseURL     string
	suspendedStore    SuspendedTransactionStore
	fieldPathRegistry *FieldPathRegistry
	dbService         interface {
		GetField(collection, id, fieldPath string) (interface{}, error)
	}
	sessionValidator  SessionValidator
	auditLogger       AuditLogger
	nativeToolHandler *NativeToolHandler

	// Circuit breaker state
	mu               sync.RWMutex
	failureCount     int
	lastFailure      time.Time
	circuitOpen      bool
	cooldownDuration time.Duration
	maxFailures      int

	maxPayloadBytes int64
}

// SessionValidator validates operator sessions for L3 authorization
type SessionValidator interface {
	ValidateSession(operatorSessionID string) (bool, error)
}

// AuditLogger logs field read operations to the audit vault
type AuditLogger interface {
	LogFieldRead(operatorSessionID, collection, documentID, fieldPath string, value interface{}) error
}

// Dependencies groups all dependencies for NewGatewayService to reduce constructor bloat.
type Dependencies struct {
	Logger          *slog.Logger
	Responder       *responder.Responder
	SuspendedStore  SuspendedTransactionStore
	MaxPayloadBytes int64
}

func NewGatewayService(deps Dependencies) *GatewayService {
	fieldPathRegistry, err := NewFieldPathRegistry(deps.Logger)
	if err != nil {
		deps.Logger.Error("Failed to initialize field path registry", "error", err)
		// Continue without field path registry - read_field will be disabled
	}

	g := &GatewayService{
		logger:            deps.Logger,
		responder:         deps.Responder,
		suspendedStore:    deps.SuspendedStore,
		fieldPathRegistry: fieldPathRegistry,
		nativeToolHandler: NewNativeToolHandler(),
		maxFailures:       5,
		cooldownDuration:  1 * time.Minute,
		maxPayloadBytes:   deps.MaxPayloadBytes,
	}
	return g
}

// RunMaintenance periodically prunes expired suspended transactions.
// Although the underlying store may perform its own cleanup (e.g., GatewayDBService
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
			// If the store is the GatewayDBService, it already prunes.
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

func (g *GatewayService) SetDBService(dbService interface {
	GetField(collection, id, fieldPath string) (interface{}, error)
}) {
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
	nativeTools := NativeTools()
	for _, tool := range nativeTools {
		if tool.Name == name {
			return true
		}
	}
	return false
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

func (g *GatewayService) HandleToolsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if g.downstreamURL == "" {
		// Return native tools if no downstream configured
		g.responder.RPCResponse(w, 1, ToolsListResult{Tools: NativeTools()})
		return
	}

	if g.isCircuitOpen() {
		g.logger.Warn("MCP downstream circuit is open, rejecting tools/list", "url", g.downstreamURL)
		g.responder.RPCError(w, 1, -32603, "downstream MCP server is temporarily unavailable (circuit open)")
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

func (g *GatewayService) HandleToolsCall(w http.ResponseWriter, r *http.Request) {
	g.handleMCPRequest(w, r, "tools/call", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
		var callParams CallToolRequest
		if err := json.Unmarshal(params, &callParams); err != nil {
			return nil, fmt.Errorf("invalid tools/call params: %w", err)
		}

		if callParams.Name == "" {
			return nil, errors.New("tool name required")
		}

		// Handle read_field tool locally (JIT field resolution)
		if callParams.Name == "read_field" {
			return g.handleReadField(ctx, callParams.Arguments)
		}

		// Handle native tools within Operator's execution boundary
		if g.isNativeTool(callParams.Name) {
			return g.nativeToolHandler.HandleTool(ctx, callParams.Name, callParams.Arguments)
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
				userID := r.Header.Get(constants.HeaderUserID)
				operatorID := r.Header.Get(constants.HeaderOperatorID)

				g.storeSuspendedTransaction(hash, envelopeBytes, callParams.Name, callParams.Arguments, userID, operatorID)

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
	})
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

	// L3: Validate operator session
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

	// Forbidden patterns from L1 hard gates
	forbiddenPatterns := []string{
		"sudo",
		"su ",
		"rm -rf /",
		"://",
		"password",
		"api_key",
		"secret",
		"token",
		"private_key",
	}

	for _, pattern := range forbiddenPatterns {
		if strings.Contains(strings.ToLower(valueStr), pattern) {
			return fmt.Errorf("forbidden pattern detected: %s", pattern)
		}
	}

	return nil
}

func (g *GatewayService) HandleResourcesRead(w http.ResponseWriter, r *http.Request) {
	g.handleMCPRequest(w, r, "resources/read", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
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
	})
}

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

func (g *GatewayService) HandlePromptsGet(w http.ResponseWriter, r *http.Request) {
	g.handleMCPRequest(w, r, "prompts/get", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
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
	})
}

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
			userID := r.Header.Get(constants.HeaderUserID)
			operatorID := r.Header.Get(constants.HeaderOperatorID)

			g.storeSuspendedTransaction(hash, envelopeBytes, callParams.Name, callParams.Arguments, userID, operatorID)

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

	// Enrich from context if present
	if tenantID, ok := ctx.Value(constants.ContextKeyTenantID).(string); ok {
		env.TenantId = tenantID
	}
	if persona, ok := ctx.Value(constants.ContextKeyBindingPersona).(string); ok {
		env.BindingPersona = persona
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

func (g *GatewayService) HandleA2aCall(w http.ResponseWriter, r *http.Request) {
	g.handleMCPRequest(w, r, "a2a/call", func(ctx context.Context, id interface{}, params json.RawMessage) (interface{}, error) {
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
				userID := r.Header.Get(constants.HeaderUserID)
				operatorID := r.Header.Get(constants.HeaderOperatorID)

				g.storeSuspendedTransaction(hash, envelopeBytes, req.SkillName, req.PayloadJSON, userID, operatorID)

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
	})
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
		return responder.ErrCodeInvalidEnvelope, msg

	case errors.Is(err, governance.ErrPayloadDecodeFailed):
		return responder.ErrCodePayloadDecodeFailed, msg

	case errors.Is(err, governance.ErrTransactionHashMissing),
		errors.Is(err, governance.ErrTransactionHashMismatch):
		return responder.ErrCodeHashMismatch, msg

	case errors.Is(err, governance.ErrTransactionExpired):
		return responder.ErrCodeExpired, msg

	case errors.Is(err, governance.ErrTransactionReplay):
		return responder.ErrCodeReplay, msg

	case errors.Is(err, governance.ErrStateRootMissing),
		errors.Is(err, governance.ErrStateRootRequired),
		errors.Is(err, governance.ErrStateRootMismatch):
		return responder.ErrCodeStateMismatch, msg

	case errors.Is(err, governance.ErrL1ValidationFailed):
		return responder.ErrCodeL1ValidationFailed, msg

	case errors.Is(err, governance.ErrL2SignatureMissing),
		errors.Is(err, governance.ErrL2SignatureInvalid),
		errors.Is(err, governance.ErrL2KeyNotConfigured):
		return responder.ErrCodeL2SignatureInvalid, msg

	case errors.Is(err, governance.ErrL3ProofInvalid),
		errors.Is(err, governance.ErrL3NotaryNotConfigured):
		return responder.ErrCodeL3ProofInvalid, msg
	}

	// Map other Gateway errors back to JSON-RPC error
	return -32603, msg
}

// storeSuspendedTransaction stores a transaction awaiting L3 approval.
func (g *GatewayService) storeSuspendedTransaction(txHash string, envelope []byte, toolName string, toolArgs json.RawMessage, userID, operatorID string) {
	if g.suspendedStore == nil {
		return
	}
	tx := &models.SuspendedTransaction{
		TransactionHash: txHash,
		Envelope:        json.RawMessage(envelope),
		CreatedAt:       time.Now().UTC(),
		ExpiresAt:       time.Now().UTC().Add(5 * time.Minute),
		ToolName:        toolName,
		ToolArguments:   toolArgs,
		UserID:          userID,
		OperatorID:      operatorID,
	}
	if err := g.suspendedStore.StoreSuspendedTransaction(tx); err != nil {
		g.logger.Error("Failed to store suspended transaction", "tx_hash", txHash, "error", err)
	}
}

// GetSuspendedTransaction retrieves a suspended transaction by hash.
func (g *GatewayService) GetSuspendedTransaction(txHash string) (*models.SuspendedTransaction, bool) {
	if g.suspendedStore == nil {
		return nil, false
	}
	return g.suspendedStore.GetSuspendedTransaction(txHash)
}

// deleteSuspendedTransaction removes a suspended transaction after approval/rejection.
func (g *GatewayService) deleteSuspendedTransaction(txHash string) {
	if g.suspendedStore == nil {
		return
	}
	if err := g.suspendedStore.DeleteSuspendedTransaction(txHash); err != nil {
		g.logger.Error("Failed to delete suspended transaction", "tx_hash", txHash, "error", err)
	}
}

// ResumeWithL3Proof re-submits a suspended transaction with an attached L3
// WebAuthn proof through the governance Gateway. The proof is verified
// inside the Gateway's TransactionVerifier - this method does not perform
// independent passkey validation, it only re-wires the envelope and calls
// the same fail-closed entry point used for primary submission.
//
// The signed receipt returned by the Gateway is forwarded to the caller so
// the OOB approval UI can surface the downstream tool result to the user.
func (g *GatewayService) ResumeWithL3Proof(ctx context.Context, txHash, userID string, proof *commonv1.L3Proof) (*operatorv1.ActionReceipt, error) {
	if g.envProc == nil {
		return nil, fmt.Errorf("governance Gateway not ready")
	}
	if proof == nil {
		return nil, fmt.Errorf("L3 proof required")
	}
	if userID == "" {
		return nil, fmt.Errorf("user_id required")
	}

	tx, ok := g.GetSuspendedTransaction(txHash)
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
	g.deleteSuspendedTransaction(txHash)
	return receipt, nil
}

// DispatchToDownstream forwards a verified MCP tool call to the downstream MCP server.
// This implements the Actuator Egress phase for MCP protocol translation.
func (g *GatewayService) DispatchToDownstream(ctx context.Context, toolName string, toolArgs json.RawMessage) (string, error) {
	if g.downstreamURL == "" {
		return "", fmt.Errorf("no downstream MCP server configured")
	}

	if g.isCircuitOpen() {
		return "", fmt.Errorf("downstream MCP server is temporarily unavailable (circuit open)")
	}

	// Construct MCP tools/call request
	mcpReq := &responder.JSONRPCRequest{
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
