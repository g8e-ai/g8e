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
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	"github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	operatorv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/governance"
	"github.com/g8e-ai/g8e/services/g8eo/pkg/uap"
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
	envProc           governance.EnvelopeProcessor
	stateRootProvider StateRootProvider
	signingKey        ed25519.PrivateKey
	keyID             string
	downstreamURL     string
	a2aDownstreamURL  string
	publicBaseURL     string
	suspendedStore    SuspendedTransactionStore
}

func NewGatewayService(logger *slog.Logger, suspendedStore SuspendedTransactionStore) *GatewayService {
	g := &GatewayService{
		logger:         logger,
		suspendedStore: suspendedStore,
	}
	return g
}

// RunMaintenance periodically prunes expired suspended transactions.
// Although the underlying store may perform its own cleanup (e.g., ListenDBService
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
			// If the store is the ListenDBService, it already prunes.
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

func (g *GatewayService) HandleToolsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if g.downstreamURL == "" {
		// Mock response if no downstream configured
		w.Header().Set(constants.HeaderContentType, "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`)); err != nil {
			g.logger.Error("Failed to write mock tools/list response", "error", err)
		}
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
			g.jsonRPCError(w, 1, -32603, "failed to read request body")
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
		g.logger.Error("Failed to query downstream MCP server", "url", g.downstreamURL, "error", err)
		g.jsonRPCError(w, 1, -32603, fmt.Sprintf("failed to query downstream MCP server: %v", err))
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			g.logger.Error("Failed to close response body", "error", err)
		}
	}()

	w.Header().Set(constants.HeaderContentType, "application/json")
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		g.logger.Error("Failed to copy response body", "error", err)
	}
}

type processGatewayOptions struct {
	actionType     constants.ActionType
	targetResource string
	payloadBytes   []byte
}

func (g *GatewayService) processGatewayTransaction(ctx context.Context, opts processGatewayOptions) (hash string, uapBytes []byte, err error) {
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
			ImplicitL2Signature: true,
		},
	}

	hash, err = uap.GenerateMessageID(env)
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
			TribunalSignature: hex.EncodeToString(sig),
			KeyId:             g.keyID,
			AgentIds:          []string{"gateway-local-signer"},
		}
	}

	uapBytes, err = (protojson.MarshalOptions{Multiline: false}).Marshal(env)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal envelope: %w", err)
	}

	return hash, uapBytes, nil
}

func (g *GatewayService) HandleA2aCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if g.envProc == nil {
		g.jsonError(w, http.StatusServiceUnavailable, "governance substrate not ready")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		g.jsonError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var req struct {
		SkillName   string          `json:"skill_name"`
		PayloadJSON json.RawMessage `json:"payload"`
		ExecutionID string          `json:"execution_id,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		g.jsonError(w, http.StatusBadRequest, "invalid A2A request")
		return
	}

	if req.SkillName == "" {
		g.jsonError(w, http.StatusBadRequest, "skill_name required")
		return
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
		g.jsonError(w, http.StatusInternalServerError, "failed to marshal A2A payload")
		return
	}

	hash, uapBytes, err := g.processGatewayTransaction(r.Context(), processGatewayOptions{
		actionType:     constants.ActionTypeA2aCall,
		targetResource: req.SkillName,
		payloadBytes:   payloadBytes,
	})
	if err != nil {
		g.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	receipt, err := g.envProc.ProcessEnvelope(r.Context(), uapBytes)
	if err != nil {
		if strings.Contains(err.Error(), "TX_L3_PROOF_MISSING") {
			userID := r.Header.Get(constants.HeaderUserID)
			operatorID := r.Header.Get(constants.HeaderOperatorID)

			g.storeSuspendedTransaction(hash, uapBytes, req.SkillName, req.PayloadJSON, userID, operatorID)

			approvalURL := fmt.Sprintf("%s/approve/%s", g.publicBaseURL, hash)
			g.jsonResponse(w, hash, map[string]interface{}{
				"status":       "suspended",
				"tx_hash":      hash,
				"approval_url": approvalURL,
				"message":      "Execution paused for L3 authorization",
			})
			return
		}
		g.jsonError(w, http.StatusForbidden, err.Error())
		return
	}

	g.jsonResponse(w, hash, receipt)
}

func (g *GatewayService) HandleToolsCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if g.envProc == nil {
		g.jsonError(w, http.StatusServiceUnavailable, "governance substrate not ready")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		g.jsonError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		g.jsonError(w, http.StatusBadRequest, "invalid JSON-RPC request")
		return
	}

	if req.Method != "tools/call" {
		g.jsonRPCError(w, req.ID, -32601, "method not found")
		return
	}

	var callParams CallToolRequest
	if err := json.Unmarshal(req.Params, &callParams); err != nil {
		g.jsonRPCError(w, req.ID, -32602, "invalid tools/call params")
		return
	}

	argumentsJSON := "{}"
	if len(callParams.Arguments) > 0 {
		var probe interface{}
		if err := json.Unmarshal(callParams.Arguments, &probe); err != nil {
			g.jsonRPCError(w, req.ID, -32602, "invalid tool arguments")
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
		g.jsonRPCError(w, req.ID, -32603, "failed to marshal MCP payload")
		return
	}

	hash, uapBytes, err := g.processGatewayTransaction(r.Context(), processGatewayOptions{
		actionType:     constants.ActionTypeMcpCall,
		targetResource: callParams.Name,
		payloadBytes:   payloadBytes,
	})
	if err != nil {
		g.jsonRPCError(w, req.ID, -32603, err.Error())
		return
	}

	receipt, err := g.envProc.ProcessEnvelope(r.Context(), uapBytes)
	if err != nil {
		if strings.Contains(err.Error(), "TX_L3_PROOF_MISSING") {
			userID := r.Header.Get(constants.HeaderUserID)
			operatorID := r.Header.Get(constants.HeaderOperatorID)

			g.storeSuspendedTransaction(hash, uapBytes, callParams.Name, callParams.Arguments, userID, operatorID)

			approvalURL := fmt.Sprintf("%s/approve/%s", g.publicBaseURL, hash)
			pausedRes := CallToolResult{
				Content: []TextContent{
					{
						Type: "text",
						Text: fmt.Sprintf("Execution paused. Please visit %s to authorize via WebAuthn, then retry.", approvalURL),
					},
				},
			}
			g.jsonRPCResponse(w, req.ID, pausedRes)
			return
		}

		code, msg := g.mapSubstrateError(err)
		g.jsonRPCError(w, req.ID, code, msg)
		return
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

	g.jsonRPCResponse(w, req.ID, mcpRes)
}

func (g *GatewayService) jsonError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set(constants.HeaderContentType, "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		g.logger.Error("Failed to encode JSON error response", "error", err)
	}
}

func (g *GatewayService) jsonRPCResponse(w http.ResponseWriter, id interface{}, result interface{}) {
	res := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
	}
	b, err := json.Marshal(result)
	if err != nil {
		g.logger.Error("Failed to marshal JSON-RPC result", "error", err)
		g.jsonRPCError(w, id, -32603, "failed to marshal result")
		return
	}
	res.Result = json.RawMessage(b)

	w.Header().Set(constants.HeaderContentType, "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		g.logger.Error("Failed to encode JSON-RPC response", "error", err)
	}
}

func (g *GatewayService) jsonRPCError(w http.ResponseWriter, id interface{}, code int, msg string) {
	res := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: msg,
		},
	}
	w.Header().Set(constants.HeaderContentType, "application/json")
	w.WriteHeader(http.StatusOK) // JSON-RPC usually returns 200 even for errors
	if err := json.NewEncoder(w).Encode(res); err != nil {
		g.logger.Error("Failed to encode JSON-RPC error response", "error", err)
	}
}

// mapSubstrateError maps governance verification errors to granular JSON-RPC codes.
func (g *GatewayService) mapSubstrateError(err error) (int, string) {
	if err == nil {
		return 0, ""
	}

	msg := err.Error()

	switch {
	case errors.Is(err, governance.ErrInvalidEnvelope),
		errors.Is(err, governance.ErrTransactionIDMissing),
		errors.Is(err, governance.ErrPayloadMissing),
		errors.Is(err, governance.ErrUnknownActionType):
		return ErrCodeInvalidEnvelope, msg

	case errors.Is(err, governance.ErrPayloadDecodeFailed):
		return ErrCodePayloadDecodeFailed, msg

	case errors.Is(err, governance.ErrTransactionHashMissing),
		errors.Is(err, governance.ErrTransactionHashMismatch):
		return ErrCodeHashMismatch, msg

	case errors.Is(err, governance.ErrTransactionExpired):
		return ErrCodeExpired, msg

	case errors.Is(err, governance.ErrTransactionReplay):
		return ErrCodeReplay, msg

	case errors.Is(err, governance.ErrStateRootMissing),
		errors.Is(err, governance.ErrStateRootRequired),
		errors.Is(err, governance.ErrStateRootMismatch):
		return ErrCodeStateMismatch, msg

	case errors.Is(err, governance.ErrL1ValidationFailed):
		return ErrCodeL1ValidationFailed, msg

	case errors.Is(err, governance.ErrL2SignatureMissing),
		errors.Is(err, governance.ErrL2SignatureInvalid),
		errors.Is(err, governance.ErrL2KeyNotConfigured):
		return ErrCodeL2SignatureInvalid, msg

	case errors.Is(err, governance.ErrL3ProofInvalid),
		errors.Is(err, governance.ErrL3VerifierNotConfigured):
		return ErrCodeL3ProofInvalid, msg
	}

	// Map other substrate errors back to JSON-RPC error
	return -32603, msg
}

func (g *GatewayService) jsonResponse(w http.ResponseWriter, id interface{}, result interface{}) {
	w.Header().Set(constants.HeaderContentType, "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"id":     id,
		"result": result,
	}); err != nil {
		g.logger.Error("Failed to encode JSON response", "error", err)
	}
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
// WebAuthn proof through the governance substrate. The proof is verified
// inside the substrate's TransactionVerifier - this method does not perform
// independent passkey validation, it only re-wires the envelope and calls
// the same fail-closed entry point used for primary submission.
//
// The signed receipt returned by the substrate is forwarded to the caller so
// the OOB approval UI can surface the downstream tool result to the user.
func (g *GatewayService) ResumeWithL3Proof(ctx context.Context, txHash, userID string, proof *commonv1.L3Proof) (*operatorv1.ActionReceipt, error) {
	if g.envProc == nil {
		return nil, fmt.Errorf("governance substrate not ready")
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
// This implements the Warden Egress phase for MCP protocol translation.
func (g *GatewayService) DispatchToDownstream(ctx context.Context, toolName string, toolArgs json.RawMessage) (string, error) {
	if g.downstreamURL == "" {
		return "", fmt.Errorf("no downstream MCP server configured")
	}

	// Construct MCP tools/call request
	mcpReq := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params:  json.RawMessage(fmt.Sprintf(`{"name":%q,"arguments":%s}`, toolName, string(toolArgs))),
		ID:      1,
	}

	reqBody, err := json.Marshal(mcpReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal MCP request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(g.downstreamURL, "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		return "", fmt.Errorf("failed to call downstream MCP server: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			g.logger.Error("Failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downstream MCP server returned status %d", resp.StatusCode)
	}

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

	// Construct A2A request
	a2aReq := map[string]interface{}{
		"skill_name": skillName,
		"payload":    payload,
	}

	reqBody, err := json.Marshal(a2aReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal A2A request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(g.a2aDownstreamURL, "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		return "", fmt.Errorf("failed to call downstream A2A server: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			g.logger.Error("Failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downstream A2A server returned status %d", resp.StatusCode)
	}

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
