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
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"

	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// TestBYOClientEndToEndProof tests the complete BYO (Bring-Your-Own) client lifecycle:
// 1. Standard JSON client sends MCP request to Gateway
// 2. Gateway intercepts and suspends transaction (OOB) due to missing L3 proof
// 3. Client receives approval URL
// 4. Human approves via WebAuthn (simulated)
// 5. Transaction is resumed and executed
// 6. Client receives final result
func TestBYOClientEndToEndProof(t *testing.T) {
	t.Parallel()

	// Setup: Create envelope processor that requires L3 approval
	executionCount := 0
	var executedEnvelope *commonv1.GovernanceEnvelope

	processor := &l3RequiringProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId:    "tx-byo-1",
			Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary:    "file edited successfully",
			GatewaySigned:    true,
			StateRootBefore:  "root-before",
			StateRootAfter:   "root-after",
			ExecutedAtUnixMs: time.Now().UnixMilli(),
			L2Status:         operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
			L3Status:         operatorv1.L3Status_L3_STATUS_REQUIRED_VALID,
		},
		onExecute: func(env *commonv1.GovernanceEnvelope) {
			executionCount++
			executedEnvelope = env
		},
	}

	// Setup: Suspended transaction store
	suspendedStore := &fakeSuspendedStore{txs: make(map[string]*models.SuspendedTransaction)}

	// Setup: Gateway service with signing key
	_, privKey, _ := ed25519.GenerateKey(nil)
	tmpDir := t.TempDir()

	g := &GatewayService{
		logger:            slog.Default(),
		envProc:           processor,
		suspendedStore:    suspendedStore,
		signingKey:        privKey,
		keyID:             "byo-test-key",
		stateRootProvider: &fakeStateRootProvider{root: "test-root"},
		publicBaseURL:     fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorHttps),
		maxPayloadBytes:   10 * 1024 * 1024, // 10MB
	}

	// Step 1: BYO client sends standard JSON-RPC MCP request
	clientRequest := fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "tools/call",
		"params": {
			"name": "file_edit",
			"arguments": {
				"path": "%s/test.txt",
				"content": "Hello, World!"
			}
		}
	}`, tmpDir)

	req := httptest.NewRequest(http.MethodPost, "/api/mcp/v1/tools/call", strings.NewReader(clientRequest))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-G8E-User-ID", "user-byo-123")
	req.Header.Set("X-G8E-Operator-ID", "op-byo-456")
	w := httptest.NewRecorder()

	g.HandleToolsCall(w, req)

	// Step 2: Verify Gateway intercepted and suspended the transaction
	require.Equal(t, http.StatusOK, w.Code)

	var initialResp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &initialResp)
	require.NoError(t, err)
	require.Nil(t, initialResp.Error, "Initial request should not error, should suspend")

	// Unmarshal the result from json.RawMessage
	var result map[string]interface{}
	err = json.Unmarshal(initialResp.Result, &result)
	require.NoError(t, err)

	// Verify suspension response contains approval URL
	content, ok := result["content"].([]interface{})
	require.True(t, ok)
	require.Len(t, content, 1)

	textContent := content[0].(map[string]interface{})
	require.Contains(t, textContent["text"].(string), "Execution paused")
	require.Contains(t, textContent["text"].(string), "/approve/")

	// Extract transaction hash from the approval URL
	var txHash string
	for k := range suspendedStore.txs {
		txHash = k
		break
	}
	require.NotEmpty(t, txHash, "Transaction should be stored in suspended store")

	// Verify suspended transaction details
	suspendedTx, found := suspendedStore.GetSuspendedTransaction(txHash)
	require.True(t, found)
	require.Equal(t, "file_edit", suspendedTx.ToolName)
	require.Equal(t, "user-byo-123", suspendedTx.UserID)
	require.Equal(t, "op-byo-456", suspendedTx.OperatorID)
	require.NotEmpty(t, suspendedTx.Envelope)

	// Verify envelope has GatewaySigned=true
	var envelope commonv1.GovernanceEnvelope
	err = protojson.Unmarshal(suspendedTx.Envelope, &envelope)
	require.NoError(t, err)
	require.NotNil(t, envelope.Governance)
	require.True(t, envelope.Governance.GatewaySigned, "Suspended envelope should have GatewaySigned=true")

	// Step 3: Simulate WebAuthn approval (challenge + verify)
	// In a real flow, this would involve browser WebAuthn API
	l3Proof := &commonv1.L3Proof{
		CredentialId:      "webauthn-cred-byo-123",
		Signature:         "simulated-webauthn-sig",
		AuthenticatorData: "auth-data",
		ClientDataJson:    fmt.Sprintf(`{"type":"webauthn.get","challenge":"challenge","origin":"https://localhost:%d"}`, constants.Ports.OperatorHttps),
	}

	// Step 4: Resume transaction with L3 proof
	receipt, err := g.ResumeWithL3Proof(context.Background(), txHash, "user-byo-123", l3Proof)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// Step 5: Verify transaction was executed
	require.Equal(t, 1, executionCount, "Transaction should have been executed exactly once")
	require.NotNil(t, executedEnvelope)
	require.Equal(t, txHash, executedEnvelope.Id)

	// Verify L3 proof was attached to the envelope
	require.NotNil(t, executedEnvelope.Governance)
	require.NotNil(t, executedEnvelope.Governance.L3)
	require.NotNil(t, executedEnvelope.Governance.L3.Proof)
	require.Equal(t, "webauthn-cred-byo-123", executedEnvelope.Governance.L3.Proof.CredentialId)

	// Verify receipt details
	require.Equal(t, "tx-byo-1", receipt.TransactionId)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)
	require.Equal(t, "file edited successfully", receipt.ResultSummary)
	require.True(t, receipt.GatewaySigned)
	require.Equal(t, operatorv1.L2Status_L2_STATUS_REQUIRED_VALID, receipt.L2Status)
	require.Equal(t, operatorv1.L3Status_L3_STATUS_REQUIRED_VALID, receipt.L3Status)

	// Step 6: Verify transaction was deleted from suspended store after execution
	_, found = suspendedStore.GetSuspendedTransaction(txHash)
	require.False(t, found, "Transaction should be deleted after successful execution")

	// Step 7: Simulate client retry after approval (should now succeed)
	retryRequest := fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": 2,
		"method": "tools/call",
		"params": {
			"name": "file_edit",
			"arguments": {
				"path": "%s/test.txt",
				"content": "Hello, World!"
			}
		}
	}`, tmpDir)

	// For the retry, we'll use a processor that succeeds immediately
	successProcessor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId: "tx-byo-2",
			Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary: "file edited successfully",
			GatewaySigned: true,
			L2Status:      operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
			L3Status:      operatorv1.L3Status_L3_STATUS_REQUIRED_VALID,
		},
	}

	g.envProc = successProcessor

	req2 := httptest.NewRequest(http.MethodPost, "/api/mcp/v1/tools/call", strings.NewReader(retryRequest))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-G8E-User-ID", "user-byo-123")
	req2.Header.Set("X-G8E-Operator-ID", "op-byo-456")
	w2 := httptest.NewRecorder()

	g.HandleToolsCall(w2, req2)

	require.Equal(t, http.StatusOK, w2.Code)

	var retryResp JSONRPCResponse
	err = json.Unmarshal(w2.Body.Bytes(), &retryResp)
	require.NoError(t, err)
	require.Nil(t, retryResp.Error, "Retry after approval should succeed")

	// Unmarshal the result from json.RawMessage
	var result2 map[string]interface{}
	err = json.Unmarshal(retryResp.Result, &result2)
	require.NoError(t, err)

	// Verify the result contains the execution summary
	content2, ok := result2["content"].([]interface{})
	require.True(t, ok)
	require.Len(t, content2, 1)

	textContent2 := content2[0].(map[string]interface{})
	require.Equal(t, "file edited successfully", textContent2["text"])
}

// TestBYOClientA2AEndToEndProof tests the A2A protocol variant of the BYO client flow
func TestBYOClientA2AEndToEndProof(t *testing.T) {
	t.Parallel()

	// Setup: Create envelope processor that requires L3 approval
	processor := &l3RequiringProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId: "tx-a2a-1",
			Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary: "A2A skill executed successfully",
			GatewaySigned: true,
			L2Status:      operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
			L3Status:      operatorv1.L3Status_L3_STATUS_REQUIRED_VALID,
		},
	}

	suspendedStore := &fakeSuspendedStore{txs: make(map[string]*models.SuspendedTransaction)}

	_, privKey, _ := ed25519.GenerateKey(nil)

	g := &GatewayService{
		logger:            slog.Default(),
		envProc:           processor,
		suspendedStore:    suspendedStore,
		signingKey:        privKey,
		keyID:             "a2a-test-key",
		stateRootProvider: &fakeStateRootProvider{root: "test-root"},
		publicBaseURL:     fmt.Sprintf("https://localhost:%d", constants.Ports.OperatorHttps),
		maxPayloadBytes:   10 * 1024 * 1024,
	}

	// Step 1: BYO client sends A2A request
	a2aRequest := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "a2a/call",
		"params": {
			"skill_name": "data_analysis",
			"payload": {
				"query": "SELECT * FROM users"
			}
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/a2a/v1/call", strings.NewReader(a2aRequest))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-G8E-User-ID", "user-a2a-123")
	req.Header.Set("X-G8E-Operator-ID", "op-a2a-456")
	w := httptest.NewRecorder()

	g.HandleA2aCall(w, req)

	// Step 2: Verify suspension response
	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	// Unmarshal the result from json.RawMessage
	var result map[string]interface{}
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)

	require.Equal(t, "suspended", result["status"])
	require.Contains(t, result["approval_url"].(string), "/approve/")
	require.Equal(t, "Execution paused for L3 authorization", result["message"])

	// Extract transaction hash
	txHash := result["id"].(string)
	require.NotEmpty(t, txHash)

	// Step 3: Resume with L3 proof
	l3Proof := &commonv1.L3Proof{
		CredentialId: "webauthn-cred-a2a-123",
		Signature:    "simulated-webauthn-sig",
	}

	receipt, err := g.ResumeWithL3Proof(context.Background(), txHash, "user-a2a-123", l3Proof)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, "A2A skill executed successfully", receipt.ResultSummary)

	// Verify cleanup
	_, found := suspendedStore.GetSuspendedTransaction(txHash)
	require.False(t, found)
}

// l3RequiringProcessor is a fake envelope processor that requires L3 proof
type l3RequiringProcessor struct {
	receipt   *operatorv1.ActionReceipt
	onExecute func(*commonv1.GovernanceEnvelope)
}

func (l *l3RequiringProcessor) ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error) {
	var envelope commonv1.GovernanceEnvelope
	if err := protojson.Unmarshal(payload, &envelope); err != nil {
		return nil, governance.ErrInvalidEnvelope
	}

	// Check if L3 proof is present
	if envelope.Governance == nil || envelope.Governance.L3 == nil || envelope.Governance.L3.Proof == nil {
		return nil, governance.ErrL3ProofMissing
	}

	// Validate L3 proof
	if envelope.Governance.L3.Proof.CredentialId == "" {
		return nil, governance.ErrL3ProofInvalid
	}

	// Execute the transaction
	if l.onExecute != nil {
		l.onExecute(&envelope)
	}

	return l.receipt, nil
}
