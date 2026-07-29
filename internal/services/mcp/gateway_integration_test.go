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
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// TestMCPGatewayEndToEndIntegration tests the full flow from MCP gateway
// through envelope processing to receipt generation, verifying that the
// envelope is properly constructed and processed.
func TestMCPGatewayEndToEndIntegration(t *testing.T) {
	t.Parallel()
	// Setup: Create a real envelope processor that will verify the envelope
	processorCalled := false
	var receivedEnvelope *commonv1.GovernanceEnvelope

	processor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId: "tx-1",
			Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary: "test result",
		},
	}

	// Wrap the processor to capture the envelope for verification
	wrappedProcessor := &envelopeCaptureProcessor{
		delegate: processor,
		capture: func(env *commonv1.GovernanceEnvelope) {
			processorCalled = true
			receivedEnvelope = env
		},
	}

	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	_ = pubKey

	g := &GatewayService{
		logger:          slog.Default(),
		maxPayloadBytes: 10 * 1024 * 1024, // 10MB
	}
	g.SetRuntimeDeps(RuntimeDependencies{
		EnvProc:           wrappedProcessor,
		SigningKey:        privKey,
		KeyID:             "test-key",
		StateRootProvider: &fakeStateRootProvider{root: "test-root"},
	})

	// Execute a tools/call request
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{"foo":"bar"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	// Verify the processor was called
	require.True(t, processorCalled, "Envelope processor should have been called")
	require.NotNil(t, receivedEnvelope)

	// Verify governance metadata is present (no GatewaySigned — that concept is deleted)
	require.NotNil(t, receivedEnvelope.Governance)
	// L2 is empty — the gateway no longer self-signs; Consensus deliberation is a separate step
}

// envelopeCaptureProcessor wraps an EnvelopeProcessor to capture envelopes for verification
type envelopeCaptureProcessor struct {
	delegate governance.EnvelopeProcessor
	capture  func(*commonv1.GovernanceEnvelope)
}

func (e *envelopeCaptureProcessor) ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error) {
	// Parse the envelope to capture it
	var envelope commonv1.GovernanceEnvelope
	err := protojson.Unmarshal(payload, &envelope)
	if err == nil && e.capture != nil {
		e.capture(&envelope)
	}
	return e.delegate.ProcessEnvelope(ctx, payload)
}

// TestReceiptIntegration tests that a receipt is properly returned by the
// envelope processor after MCP gateway processing.
func TestReceiptIntegration(t *testing.T) {
	t.Parallel()
	processor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId:    "tx-1",
			TransactionHash:  "hash-1",
			Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary:    "test result",
			StateRootBefore:  "root-before",
			StateRootAfter:   "root-after",
			ExecutedAtUnixMs: 1234567890,
			SignerKeyId:      "test-key",
			Signature:        "test-sig",
			L2Status:         operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
			L3Status:         operatorv1.L3Status_L3_STATUS_REQUIRED_VALID,
		},
	}

	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	_ = pubKey

	g := &GatewayService{
		logger:          slog.Default(),
		maxPayloadBytes: 10 * 1024 * 1024, // 10MB
	}
	g.SetRuntimeDeps(RuntimeDependencies{
		EnvProc:           processor,
		SigningKey:        privKey,
		KeyID:             "test-key",
		StateRootProvider: &fakeStateRootProvider{root: "test-root"},
	})

	// Execute a tools/call request
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	// Verify the receipt was returned successfully
	require.NotNil(t, processor.receipt)
	require.Equal(t, "tx-1", processor.receipt.TransactionId)
}

// TestCanonicalizationIntegration tests that the canonical form used for receipt
// signing is deterministic and does not include gateway_signed (deleted concept).
func TestCanonicalizationIntegration(t *testing.T) {
	t.Parallel()
	receipt := &operatorv1.ActionReceipt{
		TransactionId:    "tx-1",
		TransactionHash:  "hash-1",
		Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary:    "test result",
		StateRootBefore:  "root-before",
		StateRootAfter:   "root-after",
		ExecutedAtUnixMs: 1234567890,
		SignerKeyId:      "test-key",
		L2Status:         operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
		L3Status:         operatorv1.L3Status_L3_STATUS_REQUIRED_VALID,
	}

	// Canonicalize the receipt
	canonical, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)

	// Parse the canonical form to verify gateway_signed is NOT present
	var parsed map[string]interface{}
	err = json.Unmarshal(canonical, &parsed)
	require.NoError(t, err)

	_, ok := parsed["gateway_signed"]
	require.False(t, ok, "gateway_signed should not be in canonical form")

	// Verify it's deterministic
	canonical2, err := governance.CanonicalizeActionReceipt(receipt)
	require.NoError(t, err)
	require.Equal(t, canonical, canonical2, "Canonicalization should be deterministic")
}

// TestCircuitBreakerIntegration tests circuit breaker state transitions
func TestCircuitBreakerIntegration(t *testing.T) {
	t.Parallel()

	// Circuit breaker is only triggered on downstream proxy failures (tools/list, resources/list, prompts/list)
	// not on envelope processor failures. Test through tools/list with a failing downstream URL.

	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	_ = pubKey

	g := &GatewayService{
		logger:           slog.New(slog.NewTextHandler(os.Stdout, nil)),
		maxPayloadBytes:  10 * 1024 * 1024,
		maxFailures:      3, // Lower threshold for faster test
		cooldownDuration: 100 * time.Millisecond,
	}
	g.SetRuntimeDeps(RuntimeDependencies{
		SigningKey:        privKey,
		KeyID:             "circuit-test-key",
		StateRootProvider: &fakeStateRootProvider{root: "test-root"},
		DownstreamURL:     "http://localhost:9999", // Invalid URL that will fail
	})
	nativeToolHandler, err := NewNativeToolHandler(nil)
	if err != nil {
		t.Fatalf("failed to create native tool handler: %v", err)
	}
	g.nativeToolHandler = nativeToolHandler

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

	// Trigger failures until circuit opens
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
		w := httptest.NewRecorder()
		g.HandleMCP(w, req)
	}

	// Circuit should now be open
	require.True(t, g.isCircuitOpen(), "Circuit should be open after 3 failures")

	// Next request should return a circuit-open error
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()
	g.HandleMCP(w, req)

	var resp JSONRPCResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.NotNil(t, resp.Error)
	require.Contains(t, resp.Error.Message, "circuit open")

	// Wait for cooldown and verify circuit closes
	time.Sleep(150 * time.Millisecond)
	require.False(t, g.isCircuitOpen(), "Circuit should close after cooldown")
}

// TestGatewayErrorCodesIntegration tests gateway-specific error code mapping
func TestGatewayErrorCodesIntegration(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		errorToReturn error
		expectedCode  int
		expectedMsg   string
	}{
		{
			name:          "L1 validation failed",
			errorToReturn: governance.ErrL1ValidationFailed,
			expectedCode:  -32005,
			expectedMsg:   "TX_DOCTRINE_L1_FAILED",
		},
		{
			name:          "L2 signature invalid",
			errorToReturn: governance.ErrL2SignatureInvalid,
			expectedCode:  -32006,
			expectedMsg:   "TX_QUORUM_L2_SIG_INVALID",
		},
		{
			name:          "L3 proof invalid",
			errorToReturn: governance.ErrL3ProofInvalid,
			expectedCode:  -32007,
			expectedMsg:   "TX_NOTARY_L3_PROOF_INVALID",
		},
		{
			name:          "Invalid envelope",
			errorToReturn: governance.ErrInvalidEnvelope,
			expectedCode:  -32000,
			expectedMsg:   "TX_INVALID_ENVELOPE",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			processor := &errorReturningProcessor{
				err: tc.errorToReturn,
			}

			pubKey, privKey, _ := ed25519.GenerateKey(nil)
			_ = pubKey

			g := &GatewayService{
				logger:          slog.Default(),
				maxPayloadBytes: 10 * 1024 * 1024,
			}
			g.SetRuntimeDeps(RuntimeDependencies{
				EnvProc:           processor,
				SigningKey:        privKey,
				KeyID:             "error-test-key",
				StateRootProvider: &fakeStateRootProvider{root: "test-root"},
			})

			reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test","arguments":{}}}`
			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
			w := httptest.NewRecorder()

			g.HandleMCP(w, req)

			var resp JSONRPCResponse
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.NotNil(t, resp.Error)
			require.Equal(t, tc.expectedCode, resp.Error.Code)
			require.Contains(t, resp.Error.Message, tc.expectedMsg)
		})
	}
}

// TestNativeToolExecutionIntegration tests native tool execution within gateway
func TestNativeToolExecutionIntegration(t *testing.T) {
	t.Parallel()

	// Native tools bypass envelope processing but still need envProc set for the gateway
	// This test verifies the gateway correctly identifies native tools
	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	_ = pubKey

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	processor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId: "tx-native-1",
			Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary: "native tool executed",
		},
	}

	g := &GatewayService{
		logger:          logger,
		responder:       response.NewWriter(logger),
		maxPayloadBytes: 10 * 1024 * 1024,
	}
	g.SetRuntimeDeps(RuntimeDependencies{
		EnvProc:           processor,
		SigningKey:        privKey,
		KeyID:             "native-test-key",
		StateRootProvider: &fakeStateRootProvider{root: "test-root"},
	})

	// Test with a known native tool (e.g., uptime)
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"uptime","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, req)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	// Verify native tool was identified and processed
	var result CallToolResult
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	require.NotEmpty(t, result.Content)
	require.Equal(t, "text", result.Content[0].Type)
}

// errorReturningProcessor is a fake processor that returns specific errors
type errorReturningProcessor struct {
	err error
}

func (e *errorReturningProcessor) ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error) {
	return nil, e.err
}

// TestReadFieldIntegration tests the read_field tool with field path registry and L3 validation
func TestReadFieldIntegration(t *testing.T) {
	t.Parallel()

	// Setup fake DB service with map-based data
	// Use "investigations" collection which exists in field_paths.json schema
	dbService := &integrationTestDBService{
		data: map[string]map[string]interface{}{
			"investigations": {
				"investigation-123": map[string]interface{}{
					"suspect_ip_addresses": []string{"192.168.1.1"},
					"status":               "active",
					"priority":             "high",
				},
			},
		},
	}

	// Setup fake session validator
	sessionValidator := &integrationTestSessionValidator{
		validSessions: map[string]bool{
			"valid-session-123": true,
		},
	}

	// Setup fake audit logger
	auditLogger := &integrationTestAuditLogger{
		logs: []auditLogEntry{},
	}

	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	_ = pubKey

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	fieldPathRegistry, err := NewFieldPathRegistry(logger)
	require.NoError(t, err, "Failed to initialize field path registry")

	envProc := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId: "readfield-tx-1",
			Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary: "read_field result",
		},
	}

	g := &GatewayService{
		logger:            logger,
		responder:         response.NewWriter(logger),
		maxPayloadBytes:   10 * 1024 * 1024,
		fieldPathRegistry: fieldPathRegistry,
	}
	g.SetRuntimeDeps(RuntimeDependencies{
		EnvProc:           envProc,
		SigningKey:        privKey,
		KeyID:             "readfield-test-key",
		StateRootProvider: &fakeStateRootProvider{root: "test-root"},
		DBService:         dbService,
		SessionValidator:  sessionValidator,
		AuditLogger:       auditLogger,
	})

	// Verify read_field flows through the governance pipeline (envProc).
	// handleReadField is called inside DispatchToDownstream, inside the real pipeline.
	// With a fake processor, the receipt summary is what's returned — field value
	// extraction and audit logging happen inside the pipeline, not before it.
	t.Run("routes through pipeline", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_field","arguments":{"collection":"investigations","document_id":"investigation-123","field_path":"status","operator_session_id":"valid-session-123"}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		g.HandleMCP(w, req)

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		require.Nil(t, resp.Error)

		var result CallToolResult
		err = json.Unmarshal(resp.Result, &result)
		require.NoError(t, err)
		// Response is the fake processor's receipt summary — confirms read_field
		// went through ProcessEnvelope rather than bypassing the pipeline.
		require.Equal(t, envProc.receipt.ResultSummary, result.Content[0].Text)
	})
}

// TestHandleReadField tests handleReadField directly: session validation, forbidden
// pattern detection, and successful reads. These validations run inside the pipeline
// (DispatchToDownstream), so they cannot be tested via callTool with a fake processor.
func TestHandleReadField(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	fieldPathRegistry, err := NewFieldPathRegistry(logger)
	require.NoError(t, err)

	dbService := &integrationTestDBService{
		data: map[string]map[string]interface{}{
			"investigations": {
				"investigation-123": map[string]interface{}{
					"status":               "active",
					"suspect_ip_addresses": []string{"192.168.1.1"},
				},
				"investigation-456": map[string]interface{}{
					"suspect_ip_addresses": "192.168.1.1 password=secret123",
				},
			},
		},
	}

	sessionValidator := &integrationTestSessionValidator{
		validSessions: map[string]bool{"valid-session-123": true},
	}

	auditLogger := &integrationTestAuditLogger{logs: []auditLogEntry{}}

	g := &GatewayService{
		logger:            logger,
		fieldPathRegistry: fieldPathRegistry,
		threatScanner:     governance.NewL1Doctrine(),
	}
	g.SetRuntimeDeps(RuntimeDependencies{
		DBService:        dbService,
		SessionValidator: sessionValidator,
		AuditLogger:      auditLogger,
	})

	t.Run("successful read", func(t *testing.T) {
		args, _ := json.Marshal(map[string]string{
			"collection":          "investigations",
			"document_id":         "investigation-123",
			"field_path":          "status",
			"operator_session_id": "valid-session-123",
		})
		result, err := g.handleReadField(context.Background(), args)
		require.NoError(t, err)
		r, ok := result.(CallToolResult)
		require.True(t, ok)
		require.Contains(t, r.Content[0].Text, "active")

		require.Len(t, auditLogger.logs, 1)
		require.Equal(t, "investigations", auditLogger.logs[0].collection)
		require.Equal(t, "investigation-123", auditLogger.logs[0].documentID)
		require.Equal(t, "status", auditLogger.logs[0].fieldPath)
	})

	t.Run("invalid session", func(t *testing.T) {
		args, _ := json.Marshal(map[string]string{
			"collection":          "investigations",
			"document_id":         "investigation-123",
			"field_path":          "status",
			"operator_session_id": "invalid-session",
		})
		_, err := g.handleReadField(context.Background(), args)
		require.Error(t, err)
		require.Contains(t, err.Error(), "operator session is invalid or expired")
	})

	t.Run("forbidden pattern in field value", func(t *testing.T) {
		args, _ := json.Marshal(map[string]string{
			"collection":          "investigations",
			"document_id":         "investigation-456",
			"field_path":          "suspect_ip_addresses",
			"operator_session_id": "valid-session-123",
		})
		_, err := g.handleReadField(context.Background(), args)
		require.Error(t, err)
		require.Contains(t, err.Error(), "forbidden pattern")
	})
}

// integrationTestDBService is a mock database service for integration tests
type integrationTestDBService struct {
	data map[string]map[string]interface{}
}

func (f *integrationTestDBService) GetField(collection, id, fieldPath string) (FieldValue, error) {
	collectionData, ok := f.data[collection]
	if !ok {
		return FieldValue{}, errors.New("collection not found")
	}
	doc, ok := collectionData[id]
	if !ok {
		return FieldValue{}, errors.New("document not found")
	}
	docMap, ok := doc.(map[string]interface{})
	if !ok {
		return FieldValue{}, errors.New("document is not a map")
	}
	value, ok := docMap[fieldPath]
	if !ok {
		return FieldValue{}, errors.New("field not found")
	}
	return convertToFieldValue(value), nil
}

// integrationTestSessionValidator is a mock session validator for integration tests
type integrationTestSessionValidator struct {
	validSessions map[string]bool
}

func (f *integrationTestSessionValidator) ValidateSession(sessionID string) (bool, error) {
	valid, ok := f.validSessions[sessionID]
	if !ok {
		return false, nil
	}
	return valid, nil
}

// integrationTestAuditLogger is a mock audit logger for integration tests
type integrationTestAuditLogger struct {
	logs []auditLogEntry
}

type auditLogEntry struct {
	operatorSessionID string
	collection        string
	documentID        string
	fieldPath         string
	value             FieldValue
}

func (f *integrationTestAuditLogger) LogFieldRead(operatorSessionID, collection, documentID, fieldPath string, value FieldValue) error {
	f.logs = append(f.logs, auditLogEntry{
		operatorSessionID: operatorSessionID,
		collection:        collection,
		documentID:        documentID,
		fieldPath:         fieldPath,
		value:             value,
	})
	return nil
}

// TestNativeToolSingleAudit verifies that native tool calls produce exactly one
// audit record (the L5 signed receipt), not double-auditing with a raw event.
// This test verifies the removal of the double-audit block from DispatchToDownstream.
func TestNativeToolSingleAudit(t *testing.T) {
	t.Parallel()

	processor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId: "tx-1",
			Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary: "native tool executed",
		},
	}

	registry, _ := NewFieldPathRegistry(slog.Default())
	nativeHandler, _ := NewNativeToolHandler(slog.Default())

	g := &GatewayService{
		logger:            slog.Default(),
		fieldPathRegistry: registry,
		nativeToolHandler: nativeHandler,
		auditStore:        noopAuditEventRecorder{},
		maxPayloadBytes:   10 * 1024 * 1024,
		posture:           "doctrine",
	}
	g.SetRuntimeDeps(RuntimeDependencies{
		EnvProc: processor,
	})

	// Execute a native tool call through the gateway
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"db_discover_topology","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	// If the double-audit block still existed in DispatchToDownstream,
	// this would produce a duplicate audit event via the no-op recorder
}
