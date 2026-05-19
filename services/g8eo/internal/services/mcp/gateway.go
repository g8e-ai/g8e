package mcp

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	operatorv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/g8e-ai/g8e/services/g8eo/pkg/uap"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// StateRootProvider defines the interface for obtaining the current state root.
type StateRootProvider interface {
	GetCurrentStateRoot() (string, error)
}

// EnvelopeProcessor verifies and executes UAP JSON envelopes synchronously.
// This matches the interface in the listen package but avoids circular imports.
type EnvelopeProcessor interface {
	ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error)
}

type GatewayService struct {
	logger            *slog.Logger
	envProc           EnvelopeProcessor
	stateRootProvider StateRootProvider
	signingKey        ed25519.PrivateKey
	keyID             string
}

func NewGatewayService(logger *slog.Logger) *GatewayService {
	return &GatewayService{
		logger: logger,
	}
}

func (g *GatewayService) SetDependencies(p EnvelopeProcessor, srp StateRootProvider, key ed25519.PrivateKey, keyID string) {
	g.envProc = p
	g.stateRootProvider = srp
	g.signingKey = key
	g.keyID = keyID
}

func (g *GatewayService) HandleToolsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set(constants.HeaderContentType, "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`))
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

	// 1. Translate MCP to UniversalEnvelope (proto message)
	// Map MCP tool name to ActionType. For now, assume it's a bash command.
	// TODO: Support dynamic mapping of tool names to action types.
	actionType := callParams.Name
	if actionType == "ls" || actionType == "cat" || actionType == "grep" {
		// Mock mapping for standard shell-like tools to EXECUTE_BASH
		actionType = string(constants.ActionTypeExecuteBash)
	}

	// Prepare intent data
	var intentData map[string]interface{}
	if len(callParams.Arguments) > 0 {
		if err := json.Unmarshal(callParams.Arguments, &intentData); err != nil {
			g.jsonRPCError(w, req.ID, -32602, "invalid tool arguments")
			return
		}
	}

	intentStruct, err := structpb.NewStruct(intentData)
	if err != nil {
		g.jsonRPCError(w, req.ID, -32603, "failed to build intent data")
		return
	}

	// Get current state root
	stateRoot := ""
	if g.stateRootProvider != nil {
		stateRoot, _ = g.stateRootProvider.GetCurrentStateRoot()
	}

	now := time.Now().UTC()
	env := &commonv1.GovernanceEnvelope{
		Timestamp:       timestamppb.New(now),
		ExpiresAt:       timestamppb.New(now.Add(5 * time.Minute)),
		SourceComponent: commonv1.Component_COMPONENT_CLIENT,
		ActionType:      actionType,
		IntentData:      intentStruct,
		ProtocolVersion: "1.0",
		Nonce:           uuid.New().String(),
		StateMerkleRoot: stateRoot,
	}

	// Compute transaction hash
	hash, err := uap.GenerateMessageID((*uap.UAPEnvelope)(env))
	if err != nil {
		g.jsonRPCError(w, req.ID, -32603, "failed to compute transaction hash")
		return
	}
	env.Id = hash
	env.TransactionHash = hash

	// 2. Implicit L2 Signing (Local Gateway Signer)
	if len(g.signingKey) > 0 {
		// L2 payload: hash|decision
		l2Payload := fmt.Sprintf("%s|true", hash)
		sig := ed25519.Sign(g.signingKey, []byte(l2Payload))
		env.Governance = &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{
				TribunalSignature: hex.EncodeToString(sig),
				KeyId:             g.keyID,
				AgentIds:          []string{"gateway-local-signer"},
			},
		}
	}

	// Serialize to protojson (canonical UAP JSON)
	uapBytes, err := (protojson.MarshalOptions{Multiline: false}).Marshal(env)
	if err != nil {
		g.jsonRPCError(w, req.ID, -32603, "failed to marshal envelope")
		return
	}

	// 3. Process via governance substrate
	receipt, err := g.envProc.ProcessEnvelope(r.Context(), uapBytes)
	if err != nil {
		// Handle L3 suspension (proof missing)
		if strings.Contains(err.Error(), "TX_L3_PROOF_MISSING") {
			pausedRes := CallToolResult{
				Content: []TextContent{
					{
						Type: "text",
						Text: fmt.Sprintf("Execution paused. Please visit http://localhost:9000/approve/%s to authorize via WebAuthn, then retry.", hash),
					},
				},
			}
			g.jsonRPCResponse(w, req.ID, pausedRes)
			return
		}

		// Map other substrate errors back to JSON-RPC error
		g.jsonRPCError(w, req.ID, -32603, err.Error())
		return
	}

	// 4. Translate ActionReceipt back to MCP response
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
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (g *GatewayService) jsonRPCResponse(w http.ResponseWriter, id interface{}, result interface{}) {
	res := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
	}
	b, _ := json.Marshal(result)
	res.Result = json.RawMessage(b)

	w.Header().Set(constants.HeaderContentType, "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(res)
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
	json.NewEncoder(w).Encode(res)
}
