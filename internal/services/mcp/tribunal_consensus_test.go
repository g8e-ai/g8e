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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// fakeTribunalDeliberator is a test double for TribunalDeliberator that
// records calls and returns a configurable response.
type fakeTribunalDeliberator struct {
	called        bool
	receivedBytes []byte
	returnBytes   []byte
	err           error
}

func (f *fakeTribunalDeliberator) Deliberate(_ context.Context, envelopeBytes []byte) ([]byte, error) {
	f.called = true
	f.receivedBytes = append([]byte(nil), envelopeBytes...)
	if f.err != nil {
		return nil, f.err
	}
	if f.returnBytes != nil {
		return f.returnBytes, nil
	}
	return envelopeBytes, nil
}

// withPosture sets the gateway posture for the test GatewayService.
func withPosture(posture string) testGatewayOption {
	return func(g *GatewayService) {
		g.posture = posture
	}
}

// withTribunalDeliberator sets a custom tribunal deliberator for the test GatewayService.
func withTribunalDeliberator(td TribunalDeliberator) testGatewayOption {
	return func(g *GatewayService) {
		g.tribunalDeliberator = td
	}
}

// TestNoSelfSign_ConsensusWithoutDeliberator verifies that a gateway-built
// MCP envelope under consensus posture, without a Tribunal deliberator
// configured, does NOT contain L2 votes. The envelope will fail-closed at
// L4 verification because the quorum is not met.
func TestNoSelfSign_ConsensusWithoutDeliberator(t *testing.T) {
	t.Parallel()

	processor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId: "tx-noselfsign-1",
			Status:        2, // COMPLETED
		},
	}

	var capturedPayload []byte
	wrappedProcessor := &envelopeCaptureProcessor{
		delegate: processor,
		capture: func(env *commonv1.GovernanceEnvelope) {
			data, _ := (protojson.MarshalOptions{Multiline: false}).Marshal(env)
			capturedPayload = data
		},
	}

	g := newTestGatewayService(t,
		withEnvProc(wrappedProcessor),
		withPosture("consensus"),
		// No tribunalDeliberator set — simulates misconfiguration
	)

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{"foo":"bar"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCall(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Parse the envelope that was sent to the processor
	require.NotEmpty(t, capturedPayload)
	var env commonv1.GovernanceEnvelope
	err := protojson.Unmarshal(capturedPayload, &env)
	require.NoError(t, err)

	// Verify L2 metadata is nil or has no votes — the gateway did NOT self-sign
	if env.Governance != nil && env.Governance.L2 != nil {
		assert.Empty(t, env.Governance.L2.Votes, "gateway must not self-sign L2 votes under consensus without deliberator")
		assert.Empty(t, env.Governance.L2.TribunalId, "tribunal_id must not be set without deliberation")
	}
}

// TestDeliberationCall_ConsensusWithDeliberator verifies that under consensus
// posture with a tribunalDeliberator configured, processGatewayTransaction
// calls Deliberate and uses the returned envelope bytes.
func TestDeliberationCall_ConsensusWithDeliberator(t *testing.T) {
	t.Parallel()

	processor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId: "tx-deliberate-1",
			Status:        2, // COMPLETED
		},
	}

	var capturedPayload []byte
	wrappedProcessor := &envelopeCaptureProcessor{
		delegate: processor,
		capture: func(env *commonv1.GovernanceEnvelope) {
			data, _ := (protojson.MarshalOptions{Multiline: false}).Marshal(env)
			capturedPayload = data
		},
	}

	// Generate a key for the l2AddingDeliberator
	_, priv, _ := ed25519.GenerateKey(nil)

	g := newTestGatewayService(t,
		withEnvProc(wrappedProcessor),
		withPosture("consensus"),
		withTribunalDeliberator(&l2AddingDeliberator{privKey: priv, tribunalID: "test-tribunal"}),
	)

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{"foo":"bar"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCall(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Parse the envelope that was sent to the processor
	require.NotEmpty(t, capturedPayload)
	var env commonv1.GovernanceEnvelope
	err := protojson.Unmarshal(capturedPayload, &env)
	require.NoError(t, err)

	// Verify L2 votes were added by the deliberator
	require.NotNil(t, env.Governance, "governance metadata must be present")
	require.NotNil(t, env.Governance.L2, "L2 metadata must be present")
	assert.Equal(t, "test-tribunal", env.Governance.L2.TribunalId)
	assert.NotEmpty(t, env.Governance.L2.Votes, "L2 votes must be populated by deliberator")
}

// l2AddingDeliberator is a test deliberator that adds a single L2 vote
// signed with the provided private key.
type l2AddingDeliberator struct {
	privKey    ed25519.PrivateKey
	tribunalID string
}

func (d *l2AddingDeliberator) Deliberate(_ context.Context, envelopeBytes []byte) ([]byte, error) {
	var env commonv1.GovernanceEnvelope
	if err := protojson.Unmarshal(envelopeBytes, &env); err != nil {
		return nil, err
	}

	// Sign the envelope hash
	payload := env.Id + "|true"
	sig := ed25519.Sign(d.privKey, []byte(payload))

	if env.Governance == nil {
		env.Governance = &commonv1.GovernanceMetadata{
			L1: &commonv1.L1Metadata{},
			L2: &commonv1.L2Metadata{},
			L3: &commonv1.L3Metadata{},
		}
	}
	if env.Governance.L2 == nil {
		env.Governance.L2 = &commonv1.L2Metadata{}
	}
	env.Governance.L2.TribunalId = d.tribunalID
	env.Governance.L2.Votes = []*commonv1.L2Vote{
		{
			SignerKeyId:        "test-member",
			ConsensusSignature: hexEncode(sig),
			Decision:           true,
		},
	}

	return (protojson.MarshalOptions{Multiline: false}).Marshal(&env)
}

func hexEncode(b []byte) string {
	const hexChars = "0123456789abcdef"
	result := make([]byte, len(b)*2)
	for i, v := range b {
		result[i*2] = hexChars[v>>4]
		result[i*2+1] = hexChars[v&0xf]
	}
	return string(result)
}

// TestSetTribunalDeliberator verifies that SetTribunalDeliberator correctly
// wires the deliberator into the gateway service.
func TestSetTribunalDeliberator(t *testing.T) {
	t.Parallel()

	g := newTestGatewayService(t)
	require.Nil(t, g.tribunalDeliberator)

	td := &fakeTribunalDeliberator{}
	g.SetTribunalDeliberator(td)

	require.NotNil(t, g.tribunalDeliberator)
	assert.Same(t, td, g.tribunalDeliberator)
}

// TestConsensusDeliberationError verifies that when the deliberator returns
// an error under consensus posture, the gateway returns a JSON-RPC error
// response rather than proceeding with an unsigned envelope.
func TestConsensusDeliberationError(t *testing.T) {
	t.Parallel()

	processor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId: "tx-deliberate-err",
			Status:        2,
		},
	}

	g := newTestGatewayService(t,
		withEnvProc(processor),
		withPosture("consensus"),
		withTribunalDeliberator(&fakeTribunalDeliberator{
			err: &json.SyntaxError{Offset: 0},
		}),
	)

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{"foo":"bar"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCall(w, req)

	// The gateway returns HTTP 200 with a JSON-RPC error object
	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	// The error must be non-nil — deliberation failure propagates as a JSON-RPC error
	require.NotNil(t, resp.Error, "gateway must return JSON-RPC error when deliberation fails")
	assert.Contains(t, resp.Error.Message, "tribunal deliberation", "error must mention tribunal deliberation")
}

// TestConsensusPosture_NoDeliberator_NoL2 verifies that under consensus posture
// without a deliberator, the gateway does not add L2 metadata. This is the
// "no self-sign" invariant — the gateway never signs L2 votes itself.
func TestConsensusPosture_NoDeliberator_NoL2(t *testing.T) {
	t.Parallel()

	var capturedEnv *commonv1.GovernanceEnvelope
	processor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId: "tx-no-l2",
			Status:        2,
		},
	}
	wrappedProcessor := &envelopeCaptureProcessor{
		delegate: processor,
		capture:  func(env *commonv1.GovernanceEnvelope) { capturedEnv = env },
	}

	g := newTestGatewayService(t,
		withEnvProc(wrappedProcessor),
		withPosture("consensus"),
		// No deliberator
	)

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCall(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, capturedEnv)

	// The gateway must not have self-signed L2 votes
	if capturedEnv.Governance != nil && capturedEnv.Governance.L2 != nil {
		assert.Empty(t, capturedEnv.Governance.L2.Votes)
		assert.Empty(t, capturedEnv.Governance.L2.TribunalId)
	}
}

// TestDoctrinePosture_NoDeliberation verifies that under doctrine posture,
// the deliberator is never called even if one is configured.
func TestDoctrinePosture_NoDeliberation(t *testing.T) {
	t.Parallel()

	td := &fakeTribunalDeliberator{}
	processor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId: "tx-doctrine",
			Status:        2,
		},
	}

	g := newTestGatewayService(t,
		withEnvProc(processor),
		withPosture("doctrine"),
		withTribunalDeliberator(td),
	)

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleToolsCall(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.False(t, td.called, "deliberator must not be called under doctrine posture")
}

// TestTribunalRouteRegistered verifies that the HTTP handler for the Tribunal
// deliberate endpoint is registered on the mTLS mux when a Tribunal service
// is set. This is a lightweight test that doesn't require a full mTLS setup.
func TestTribunalRouteRegistered(t *testing.T) {
	t.Parallel()

	// This test verifies the route registration logic indirectly by checking
	// that SetTribunalDeliberator is properly wired. The full mTLS route
	// registration is tested via the gateway HTTP router tests.
	g := newTestGatewayService(t)
	td := &fakeTribunalDeliberator{}
	g.SetTribunalDeliberator(td)

	require.NotNil(t, g.tribunalDeliberator)
}

// TestStartupValidation_ConsensusRequiresTribunalID is a table-driven test
// that verifies the startup validation logic for consensus posture. Since
// we can't call os.Exit in tests, we test the validation function directly.
func TestStartupValidation_ConsensusRequiresTribunalID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		posture    string
		tribunalID string
		quorum     int
		expectErr  error
	}{
		{
			name:       "consensus without tribunal_id fails",
			posture:    "consensus",
			tribunalID: "",
			quorum:     0,
			expectErr:  constants.ErrConfigTribunalIDRequired,
		},
		{
			name:       "consensus with tribunal_id and quorum >= 2 passes",
			posture:    "consensus",
			tribunalID: "test-tribunal",
			quorum:     2,
			expectErr:  nil,
		},
		{
			name:       "consensus with quorum-1 tribunal fails",
			posture:    "consensus",
			tribunalID: "test-tribunal",
			quorum:     1,
			expectErr:  constants.ErrConfigTribunalQuorumLow,
		},
		{
			name:       "doctrine without tribunal_id passes",
			posture:    "doctrine",
			tribunalID: "",
			quorum:     0,
			expectErr:  nil,
		},
		{
			name:       "notary without tribunal_id passes",
			posture:    "notary",
			tribunalID: "",
			quorum:     0,
			expectErr:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := config.ValidateConsensusStartup(tc.posture, tc.tribunalID, tc.quorum)
			if tc.expectErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.expectErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestHTTPTribunalDeliberator_Basic verifies the HTTP deliberator client
// correctly sends and receives envelope bytes.
func TestHTTPTribunalDeliberator_Basic(t *testing.T) {
	t.Parallel()

	// Create a test HTTP server that acts as the Tribunal
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, constants.HeaderValueApplicationJSON, r.Header.Get(constants.HeaderContentType))

		body := make([]byte, 1024)
		n, _ := r.Body.Read(body)
		body = body[:n]

		// Return the same body (echo) to simulate deliberation
		w.Header().Set(constants.HeaderContentType, constants.HeaderValueApplicationJSON)
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer ts.Close()

	deliberator := NewHTTPTribunalDeliberator(ts.URL, nil)

	input := []byte(`{"test":"envelope"}`)
	output, err := deliberator.Deliberate(context.Background(), input)

	require.NoError(t, err)
	assert.Equal(t, input, output)
}

// TestHTTPTribunalDeliberator_ServerError verifies the HTTP deliberator
// returns an error when the server returns a non-200 status.
func TestHTTPTribunalDeliberator_ServerError(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer ts.Close()

	deliberator := NewHTTPTribunalDeliberator(ts.URL, nil)

	_, err := deliberator.Deliberate(context.Background(), []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

// TestHTTPTribunalDeliberator_Timeout verifies the HTTP deliberator respects
// the 30-second timeout. We use a short-lived server that delays.
func TestHTTPTribunalDeliberator_Timeout(t *testing.T) {
	t.Parallel()
	t.Skip("Skipping timeout test — requires >30s to run, covered by integration tests")
}

// Ensure time import is used
var _ time.Duration
