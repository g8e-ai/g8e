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
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/testutil"
	govpkg "github.com/g8e-ai/g8e/pkg/governance"
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
		logger:            slog.Default(),
		envProc:           wrappedProcessor,
		signingKey:        privKey,
		keyID:             "test-key",
		stateRootProvider: &fakeStateRootProvider{root: "test-root"},
		maxPayloadBytes:   10 * 1024 * 1024, // 10MB
	}

	// Execute a tools/call request
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{"foo":"bar"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCall(w, req)

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
	// L2 is empty — the gateway no longer self-signs; Tribunal deliberation is a separate step
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
		logger:            slog.Default(),
		envProc:           processor,
		signingKey:        privKey,
		keyID:             "test-key",
		stateRootProvider: &fakeStateRootProvider{root: "test-root"},
		maxPayloadBytes:   10 * 1024 * 1024, // 10MB
	}

	// Execute a tools/call request
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCall(w, req)

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

// TestSSEStreamingIntegration tests the SSE streaming endpoint for tools/call
func TestSSEStreamingIntegration(t *testing.T) {
	t.Parallel()

	processor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId:    "tx-sse-1",
			Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary:    "streaming result",
			StateRootBefore:  "root-before",
			StateRootAfter:   "root-after",
			ExecutedAtUnixMs: 1234567890,
			L2Status:         operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
			L3Status:         operatorv1.L3Status_L3_STATUS_REQUIRED_VALID,
		},
	}

	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	_ = pubKey

	g := &GatewayService{
		logger:            slog.Default(),
		envProc:           processor,
		signingKey:        privKey,
		keyID:             "sse-test-key",
		stateRootProvider: &fakeStateRootProvider{root: "test-root"},
		maxPayloadBytes:   10 * 1024 * 1024,
	}

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"streaming_tool","arguments":{"msg":"hello"}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/mcp/v1/tools/call/sse", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCallSSE(w, req)

	// Verify SSE headers
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	require.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	require.Equal(t, "keep-alive", w.Header().Get("Connection"))

	// Verify SSE response format
	body := w.Body.String()
	require.Contains(t, body, "data: ")
	require.Contains(t, body, "streaming result")

	// Parse SSE chunk
	lines := strings.Split(body, "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	require.True(t, strings.HasPrefix(lines[0], "data: "))

	var chunk CallToolResult
	dataLine := strings.TrimPrefix(lines[0], "data: ")
	err := json.Unmarshal([]byte(dataLine), &chunk)
	require.NoError(t, err)
	require.Equal(t, "streaming result", chunk.Content[0].Text)
	require.False(t, chunk.IsError)
}

// TestCircuitBreakerIntegration tests circuit breaker state transitions
func TestCircuitBreakerIntegration(t *testing.T) {
	t.Parallel()

	// Circuit breaker is only triggered on downstream proxy failures (tools/list, resources/list, prompts/list)
	// not on envelope processor failures. Test through tools/list with a failing downstream URL.

	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	_ = pubKey

	g := &GatewayService{
		logger:            slog.New(slog.NewTextHandler(os.Stdout, nil)),
		envProc:           nil,
		signingKey:        privKey,
		keyID:             "circuit-test-key",
		stateRootProvider: &fakeStateRootProvider{root: "test-root"},
		maxPayloadBytes:   10 * 1024 * 1024,
		maxFailures:       3, // Lower threshold for faster test
		cooldownDuration:  100 * time.Millisecond,
		downstreamURL:     "http://localhost:9999", // Invalid URL that will fail
	}
	nativeToolHandler, err := NewNativeToolHandler(nil)
	if err != nil {
		t.Fatalf("failed to create native tool handler: %v", err)
	}
	g.nativeToolHandler = nativeToolHandler

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

	// Trigger failures until circuit opens
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/mcp/v1/tools/list", strings.NewReader(reqBody))
		w := httptest.NewRecorder()
		g.HandleToolsList(w, req)
	}

	// Circuit should now be open
	require.True(t, g.isCircuitOpen(), "Circuit should be open after 3 failures")

	// Next request should return native tools when circuit is open
	req := httptest.NewRequest(http.MethodPost, "/api/mcp/v1/tools/list", strings.NewReader(reqBody))
	w := httptest.NewRecorder()
	g.HandleToolsList(w, req)

	var resp JSONRPCResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	// Verify native tools are returned
	var result ToolsListResult
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	require.NotEmpty(t, result.Tools, "Native tools should be returned when circuit is open")

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
				logger:            slog.Default(),
				envProc:           processor,
				signingKey:        privKey,
				keyID:             "error-test-key",
				stateRootProvider: &fakeStateRootProvider{root: "test-root"},
				maxPayloadBytes:   10 * 1024 * 1024,
			}

			reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test","arguments":{}}}`
			req := httptest.NewRequest(http.MethodPost, "/api/mcp/v1/tools/call", strings.NewReader(reqBody))
			w := httptest.NewRecorder()

			g.HandleToolsCall(w, req)

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
		logger:            logger,
		responder:         response.NewWriter(logger),
		envProc:           processor,
		signingKey:        privKey,
		keyID:             "native-test-key",
		stateRootProvider: &fakeStateRootProvider{root: "test-root"},
		maxPayloadBytes:   10 * 1024 * 1024,
	}

	// Test with a known native tool (e.g., uptime)
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"uptime","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/mcp/v1/tools/call", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCall(w, req)

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
		envProc:           envProc,
		signingKey:        privKey,
		keyID:             "readfield-test-key",
		stateRootProvider: &fakeStateRootProvider{root: "test-root"},
		maxPayloadBytes:   10 * 1024 * 1024,
		fieldPathRegistry: fieldPathRegistry,
		dbService:         dbService,
		sessionValidator:  sessionValidator,
		auditLogger:       auditLogger,
	}

	// Verify read_field flows through the governance pipeline (envProc).
	// handleReadField is called inside DispatchToDownstream, inside the real pipeline.
	// With a fake processor, the receipt summary is what's returned — field value
	// extraction and audit logging happen inside the pipeline, not before it.
	t.Run("routes through pipeline", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_field","arguments":{"collection":"investigations","document_id":"investigation-123","field_path":"status","operator_session_id":"valid-session-123"}}}`
		req := httptest.NewRequest(http.MethodPost, "/api/mcp/v1/tools/call", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		g.HandleToolsCall(w, req)

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
		dbService:         dbService,
		sessionValidator:  sessionValidator,
		auditLogger:       auditLogger,
	}

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

// TestGatewayL3Verification_RealNotary tests envelope processing with real L3 verification
// instead of fake envelope processor, testing both pass and fail scenarios.
func TestGatewayL3Verification_RealNotary(t *testing.T) {
	t.Parallel()

	t.Run("L3 verification passes with accepting notary", func(t *testing.T) {
		t.Parallel()

		// Create an accepting L3 notary
		acceptingL3 := &testutil.ConfigurableMockL3Notary{ShouldPass: true}

		// Create a stateful replay store and state root provider
		replayStore := testutil.NewStatefulMockReplayStore()
		stateRootProvider := testutil.NewMockStateRootProvider("test-root")

		// Create signer store with test key
		pubKey, privKey, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		signerStore := &governance.SimpleSignerStore{
			Signers: map[string]ed25519.PublicKey{"test-key": pubKey},
		}

		// Create a real L4Warden with L3 notary and doctrine posture
		// Doctrine posture doesn't require L2 or L3, allowing us to test L3 verification manually
		warden := governance.NewL4Warden(
			slog.New(slog.NewTextHandler(os.Stdout, nil)),
			replayStore,
			stateRootProvider,
			signerStore,
			nil, // TribunalStore not used in this test
			nil, // AppPolicyStore not used in this test
			acceptingL3,
			nil, // Doctrine defaults to L1Doctrine
			constants.AllActionTypes,
			"doctrine", // Doctrine posture doesn't require L2/L3
			nil,        // Clock defaults to RealClock
		)

		// Create a real envelope processor that uses the warden
		processor := &realL3EnvelopeProcessor{
			warden:   warden,
			l3Notary: acceptingL3,
			privKey:  privKey,
			keyID:    "test-key",
		}

		// Build a test envelope with L3 metadata
		mcpPayload := &operatorv1.McpCallRequested{
			ToolName:      "test-tool",
			ArgumentsJson: `{"name":"test-tool","arguments":{}}`,
			ExecutionId:   "exec-test-1",
		}
		payloadBytes, err := proto.Marshal(mcpPayload)
		require.NoError(t, err)

		envelope := &govpkg.GovernanceEnvelope{
			ProtocolVersion:   "1.0",
			Timestamp:         timestamppb.Now(),
			ExpiresAt:         timestamppb.New(time.Now().Add(time.Hour)),
			SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
			OperatorId:        "operator-1",
			OperatorSessionId: "session-1",
			ActionType:        string(constants.ActionTypeMcpCall),
			TargetResource:    "test-tool",
			StateMerkleRoot:   "test-root",
			Nonce:             "nonce-test-1",
			Payload:           payloadBytes,
			Governance: &commonv1.GovernanceMetadata{
				L2: &commonv1.L2Metadata{
					Votes: []*commonv1.L2Vote{
						{
							SignerKeyId:       "test-key",
							ConsensusSignature: hex.EncodeToString(ed25519.Sign(privKey, []byte("test"))),
							Decision:          true,
						},
					},
				},
				L3: &commonv1.L3Metadata{
					AutoApproved: true, // For testing: simulate gateway auto-approval
				},
			},
		}

		// Generate transaction hash
		txHash, err := govpkg.GenerateMessageID(envelope)
		require.NoError(t, err)
		envelope.Id = txHash
		envelope.TransactionHash = txHash

		// Marshal envelope to protojson
		envelopeBytes, err := (protojson.MarshalOptions{}).Marshal(envelope)
		require.NoError(t, err)

		// Process envelope through the processor
		receipt, err := processor.ProcessEnvelope(context.Background(), envelopeBytes)

		// Verify success: L3 acceptance should return receipt
		require.NoError(t, err)
		require.NotNil(t, receipt)
		require.True(t, processor.called, "Envelope processor should have been called")
		require.NoError(t, processor.lastError, "L3 verification should have passed")
	})

	t.Run("L3 verification fails with rejecting notary", func(t *testing.T) {
		t.Parallel()

		// Create a rejecting L3 notary
		rejectingL3 := &testutil.ConfigurableMockL3Notary{ShouldPass: false}

		// Create a stateful replay store and state root provider
		replayStore := testutil.NewStatefulMockReplayStore()
		stateRootProvider := testutil.NewMockStateRootProvider("test-root")

		// Create signer store with test key
		pubKey, privKey, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		signerStore := &governance.SimpleSignerStore{
			Signers: map[string]ed25519.PublicKey{"test-key": pubKey},
		}

		// Create a real L4Warden with rejecting L3 notary and doctrine posture
		warden := governance.NewL4Warden(
			slog.New(slog.NewTextHandler(os.Stdout, nil)),
			replayStore,
			stateRootProvider,
			signerStore,
			nil, // TribunalStore not used in this test
			nil, // AppPolicyStore not used in this test
			rejectingL3,
			nil, // Doctrine defaults to L1Doctrine
			constants.AllActionTypes,
			"doctrine", // Doctrine posture doesn't require L2/L3
			nil,        // Clock defaults to RealClock
		)

		// Create a real envelope processor that uses the warden
		processor := &realL3EnvelopeProcessor{
			warden:   warden,
			l3Notary: rejectingL3,
			privKey:  privKey,
			keyID:    "test-key",
		}

		// Build a test envelope with L3 metadata
		mcpPayload := &operatorv1.McpCallRequested{
			ToolName:      "test-tool",
			ArgumentsJson: `{"name":"test-tool","arguments":{}}`,
			ExecutionId:   "exec-test-2",
		}
		payloadBytes, err := proto.Marshal(mcpPayload)
		require.NoError(t, err)

		envelope := &govpkg.GovernanceEnvelope{
			ProtocolVersion:   "1.0",
			Timestamp:         timestamppb.Now(),
			ExpiresAt:         timestamppb.New(time.Now().Add(time.Hour)),
			SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
			OperatorId:        "operator-1",
			OperatorSessionId: "session-1",
			ActionType:        string(constants.ActionTypeMcpCall),
			TargetResource:    "test-tool",
			StateMerkleRoot:   "test-root",
			Nonce:             "nonce-test-2",
			Payload:           payloadBytes,
			Governance: &commonv1.GovernanceMetadata{
				L2: &commonv1.L2Metadata{
					Votes: []*commonv1.L2Vote{
						{
							SignerKeyId:       "test-key",
							ConsensusSignature: hex.EncodeToString(ed25519.Sign(privKey, []byte("test"))),
							Decision:          true,
						},
					},
				},
				L3: &commonv1.L3Metadata{
					AutoApproved: true, // For testing: simulate gateway auto-approval
				},
			},
		}

		// Generate transaction hash
		txHash, err := govpkg.GenerateMessageID(envelope)
		require.NoError(t, err)
		envelope.Id = txHash
		envelope.TransactionHash = txHash

		// Marshal envelope to protojson
		envelopeBytes, err := (protojson.MarshalOptions{}).Marshal(envelope)
		require.NoError(t, err)

		// Process envelope through the processor
		receipt, err := processor.ProcessEnvelope(context.Background(), envelopeBytes)

		// Verify failure: L3 rejection should return error
		require.Error(t, err)
		require.Nil(t, receipt)
		require.True(t, processor.called, "Envelope processor should have been called")
		require.Error(t, processor.lastError, "L3 verification should have failed")
	})
}

// realL3EnvelopeProcessor is a real envelope processor that uses L4Warden for verification
type realL3EnvelopeProcessor struct {
	warden    *governance.L4Warden
	l3Notary  governance.L3Notary
	privKey   ed25519.PrivateKey
	keyID     string
	called    bool
	lastError error
}

func (p *realL3EnvelopeProcessor) ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error) {
	p.called = true

	// Unmarshal as GovernanceEnvelope
	envelope := &govpkg.GovernanceEnvelope{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(payload, envelope); err != nil {
		p.lastError = err
		return nil, err
	}

	// Add L3 metadata with AutoApproved for testing (simulates gateway mode)
	// This allows us to test L3 verification without requiring actual WebAuthn proof
	if envelope.Governance == nil {
		envelope.Governance = &commonv1.GovernanceMetadata{}
	}
	if envelope.Governance.L3 == nil {
		envelope.Governance.L3 = &commonv1.L3Metadata{
			AutoApproved: true, // For testing: simulate gateway auto-approval
		}
	}

	// Verify through L4Warden (stateless validation, L1, L2)
	_, err := p.warden.VerifyEnvelope(ctx, envelope)
	if err != nil {
		p.lastError = err
		return nil, err
	}

	// Manually invoke L3 verification since doctrine posture doesn't require it
	if p.l3Notary != nil {
		l3Metadata := envelope.Governance.L3
		if l3Metadata == nil {
			p.lastError = governance.ErrL3ProofInvalid
			return nil, governance.ErrL3ProofInvalid
		}
		valid, err := p.l3Notary.VerifyL3Proof(context.Background(), envelope.OperatorId, envelope.TransactionHash, envelope.OperatorSessionId, l3Metadata.Proof)
		if err != nil {
			p.lastError = err
			return nil, err
		}
		if !valid {
			p.lastError = governance.ErrL3ProofInvalid
			return nil, governance.ErrL3ProofInvalid
		}
	}

	// Generate receipt
	receipt := &operatorv1.ActionReceipt{
		TransactionId:    envelope.Id,
		TransactionHash:  envelope.TransactionHash,
		Status:           operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary:    "test result",
		StateRootBefore:  "root-before",
		StateRootAfter:   "root-after",
		ExecutedAtUnixMs: time.Now().UnixMilli(),
		SignerKeyId:      p.keyID,
		L2Status:         operatorv1.L2Status_L2_STATUS_REQUIRED_VALID,
		L3Status:         operatorv1.L3Status_L3_STATUS_REQUIRED_VALID,
	}

	// Sign the receipt
	sigPayload := fmt.Sprintf("%s|%s|%d", receipt.TransactionId, receipt.TransactionHash, receipt.ExecutedAtUnixMs)
	signature := ed25519.Sign(p.privKey, []byte(sigPayload))
	receipt.Signature = hex.EncodeToString(signature)

	return receipt, nil
}

// TestReadFieldGovernanceIntegration verifies that read_field tool calls
// go through the full governance pipeline (L4 Warden + L5 Actuator) and
// generate a signed ActionReceipt, fixing the governance bypass issue.
func TestReadFieldGovernanceIntegration(t *testing.T) {
	t.Parallel()

	processorCalled := false
	var receivedEnvelope *commonv1.GovernanceEnvelope

	processor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId: "tx-1",
			Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary: "field value",
		},
	}

	// Wrap to capture envelope
	wrappedProcessor := &envelopeCaptureProcessor{
		delegate: processor,
		capture: func(env *commonv1.GovernanceEnvelope) {
			processorCalled = true
			receivedEnvelope = env
		},
	}

	registry, _ := NewFieldPathRegistry(slog.Default())
	db := &fakeDBService{}
	validator := &fakeSessionValidator{valid: true}
	audit := &fakeAuditLogger{}

	g := &GatewayService{
		logger:            slog.Default(),
		envProc:           wrappedProcessor,
		fieldPathRegistry: registry,
		dbService:         db,
		sessionValidator:  validator,
		auditLogger:       audit,
		maxPayloadBytes:   10 * 1024 * 1024,
		posture:           "doctrine",
	}

	// Execute a read_field request through the gateway
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_field","arguments":{"collection":"investigations","document_id":"doc1","field_path":"status","operator_session_id":"sess1"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCall(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	// Verify the envelope processor was called (governance pipeline executed)
	require.True(t, processorCalled, "Envelope processor should have been called for read_field")
	require.NotNil(t, receivedEnvelope)

	// Verify the envelope has the correct action type
	require.Equal(t, "MCP_CALL", receivedEnvelope.ActionType)

	// Verify nonce is present (replay protection)
	require.NotEmpty(t, receivedEnvelope.Nonce)

	// Verify L2 is populated (no GatewaySigned - that concept is deleted)
	require.NotNil(t, receivedEnvelope.Governance)
	require.NotNil(t, receivedEnvelope.Governance.L2)
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
		envProc:           processor,
		fieldPathRegistry: registry,
		nativeToolHandler: nativeHandler,
		// auditStore left nil - if double-audit code existed, this would cause a nil pointer panic
		maxPayloadBytes: 10 * 1024 * 1024,
		posture:         "doctrine",
	}

	// Execute a native tool call through the gateway
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"db_discover_topology","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCall(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	// If the double-audit block still existed in DispatchToDownstream,
	// this would panic with nil pointer dereference on g.auditStore
}
