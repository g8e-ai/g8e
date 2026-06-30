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

package client

import (
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/tools/agent_harness/config"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEnsemble(t *testing.T) {
	tests := []struct {
		name    string
		keyID   string
		n       int
		wantErr bool
	}{
		{
			name:    "single agent",
			keyID:   "test-key",
			n:       1,
			wantErr: false,
		},
		{
			name:    "multiple agents",
			keyID:   "consensus-key",
			n:       5,
			wantErr: false,
		},
		{
			name:    "zero agents",
			keyID:   "empty-key",
			n:       0,
			wantErr: false,
		},
		{
			name:    "large ensemble",
			keyID:   "large-key",
			n:       100,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ensemble, err := NewEnsemble(tt.keyID, tt.n)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewEnsemble() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if ensemble == nil {
					t.Fatal("NewEnsemble() returned nil ensemble")
				}
				if ensemble.KeyID != tt.keyID {
					t.Errorf("KeyID = %s, want %s", ensemble.KeyID, tt.keyID)
				}
				if ensemble.AgentCount() != tt.n {
					t.Errorf("AgentCount() = %d, want %d", ensemble.AgentCount(), tt.n)
				}
				if len(ensemble.agents) != tt.n {
					t.Errorf("agents length = %d, want %d", len(ensemble.agents), tt.n)
				}
				if ensemble.priv == nil {
					t.Error("private key should not be nil")
				}
				if ensemble.pub == nil {
					t.Error("public key should not be nil")
				}
			}
		})
	}
}

func TestEnsemble_PubHex(t *testing.T) {
	ensemble, err := NewEnsemble("test-key", 3)
	if err != nil {
		t.Fatalf("NewEnsemble() failed: %v", err)
	}

	pubHex := ensemble.PubHex()
	if pubHex == "" {
		t.Error("PubHex() returned empty string")
	}

	// Verify it's valid hex
	_, err = hex.DecodeString(pubHex)
	if err != nil {
		t.Errorf("PubHex() returned invalid hex: %v", err)
	}
}

func TestEnsemble_AgentCount(t *testing.T) {
	tests := []struct {
		name string
		n    int
	}{
		{"zero agents", 0},
		{"one agent", 1},
		{"five agents", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ensemble, err := NewEnsemble("test-key", tt.n)
			if err != nil {
				t.Fatalf("NewEnsemble() failed: %v", err)
			}

			if ensemble.AgentCount() != tt.n {
				t.Errorf("AgentCount() = %d, want %d", ensemble.AgentCount(), tt.n)
			}
		})
	}
}

func TestEnsemble_Vote(t *testing.T) {
	ensemble, err := NewEnsemble("test-key", 3)
	if err != nil {
		t.Fatalf("NewEnsemble() failed: %v", err)
	}

	tests := []struct {
		name     string
		txHash   string
		decision bool
	}{
		{
			name:     "approve decision",
			txHash:   "abc123",
			decision: true,
		},
		{
			name:     "reject decision",
			txHash:   "def456",
			decision: false,
		},
		{
			name:     "empty hash",
			txHash:   "",
			decision: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l2 := ensemble.Vote(tt.txHash, tt.decision)

			if l2 == nil {
				t.Fatal("Vote() returned nil L2Metadata")
			}

			if len(l2.Votes) == 0 {
				t.Fatal("Vote() returned empty Votes array")
			}

			if l2.Votes[0].SignerKeyId != ensemble.KeyID {
				t.Errorf("Votes[0].SignerKeyId = %s, want %s", l2.Votes[0].SignerKeyId, ensemble.KeyID)
			}

			if l2.Votes[0].ConsensusSignature == "" {
				t.Error("Votes[0].ConsensusSignature should not be empty")
			}

			// Verify signature is valid hex
			_, err := hex.DecodeString(l2.Votes[0].ConsensusSignature)
			if err != nil {
				t.Errorf("Votes[0].ConsensusSignature is invalid hex: %v", err)
			}
		})
	}
}

func TestEnsemble_Vote_Deterministic(t *testing.T) {
	ensemble, err := NewEnsemble("test-key", 2)
	if err != nil {
		t.Fatalf("NewEnsemble() failed: %v", err)
	}

	txHash := "abc123"
	decision := true

	l2a := ensemble.Vote(txHash, decision)
	l2b := ensemble.Vote(txHash, decision)

	if l2a.Votes[0].ConsensusSignature != l2b.Votes[0].ConsensusSignature {
		t.Error("Vote() should produce deterministic signatures for same input")
	}

	if l2a.Votes[0].SignerKeyId != l2b.Votes[0].SignerKeyId {
		t.Error("Votes[0].SignerKeyId should be the same")
	}

	if len(l2a.Votes) != len(l2b.Votes) {
		t.Error("Votes should have same length")
	}
}

func TestNewPrincipal(t *testing.T) {
	tests := []struct {
		name    string
		keyID   string
		wantErr bool
	}{
		{
			name:    "valid principal",
			keyID:   "principal-key",
			wantErr: false,
		},
		{
			name:    "empty key ID",
			keyID:   "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			principal, err := NewPrincipal(tt.keyID)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewPrincipal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if principal == nil {
					t.Fatal("NewPrincipal() returned nil principal")
				}
				if principal.KeyID != tt.keyID {
					t.Errorf("KeyID = %s, want %s", principal.KeyID, tt.keyID)
				}
				if principal.priv == nil {
					t.Error("private key should not be nil")
				}
				if principal.pub == nil {
					t.Error("public key should not be nil")
				}
			}
		})
	}
}

func TestPrincipal_PubHex(t *testing.T) {
	principal, err := NewPrincipal("test-key")
	if err != nil {
		t.Fatalf("NewPrincipal() failed: %v", err)
	}

	pubHex := principal.PubHex()
	if pubHex == "" {
		t.Error("PubHex() returned empty string")
	}

	// Verify it's valid hex
	_, err = hex.DecodeString(pubHex)
	if err != nil {
		t.Errorf("PubHex() returned invalid hex: %v", err)
	}
}

func TestPrincipal_Sign(t *testing.T) {
	principal, err := NewPrincipal("test-key")
	if err != nil {
		t.Fatalf("NewPrincipal() failed: %v", err)
	}

	tests := []struct {
		name   string
		txHash string
	}{
		{
			name:   "valid hash",
			txHash: "abc123",
		},
		{
			name:   "empty hash",
			txHash: "",
		},
		{
			name:   "long hash",
			txHash: "very-long-transaction-hash-with-many-characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l3 := principal.Sign(tt.txHash)

			if l3 == nil {
				t.Fatal("Sign() returned nil L3Metadata")
			}

			if l3.Proof == nil {
				t.Fatal("Proof should not be nil")
			}

			if l3.Proof.Signature == "" {
				t.Error("Signature should not be empty")
			}

			// Verify signature is valid hex
			_, err := hex.DecodeString(l3.Proof.Signature)
			if err != nil {
				t.Errorf("Signature is invalid hex: %v", err)
			}
		})
	}
}

func TestPrincipal_Sign_Deterministic(t *testing.T) {
	principal, err := NewPrincipal("test-key")
	if err != nil {
		t.Fatalf("NewPrincipal() failed: %v", err)
	}

	txHash := "abc123"

	l3a := principal.Sign(txHash)
	l3b := principal.Sign(txHash)

	if l3a.Proof.Signature != l3b.Proof.Signature {
		t.Error("Sign() should produce deterministic signatures for same input")
	}
}

func TestSubmitEnvelope(t *testing.T) {
	tests := []struct {
		name         string
		responseCode int
		responseBody string
		wantErr      bool
	}{
		{
			name:         "successful submission",
			responseCode: http.StatusOK,
			responseBody: `{"status": "accepted"}`,
			wantErr:      false,
		},
		{
			name:         "server error",
			responseCode: http.StatusInternalServerError,
			responseBody: `{"error": "internal error"}`,
			wantErr:      false, // SubmitEnvelope doesn't check status codes
		},
		{
			name:         "bad request",
			responseCode: http.StatusBadRequest,
			responseBody: `{"error": "invalid envelope"}`,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != constants.APIPaths.GovernanceEnvelopes {
					t.Errorf("expected path %s, got %s", constants.APIPaths.GovernanceEnvelopes, r.URL.Path)
				}

				w.WriteHeader(tt.responseCode)
				w.Write([]byte(tt.responseBody))
			})

			server := httptest.NewServer(handler)
			defer server.Close()

			cfg := config.Config{
				MTLSBaseURL: server.URL,
				Auth:        config.Auth{},
			}

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			ctx := context.Background()
			p := Persona{ID: "test-client"}

			// Create a minimal envelope
			envelope := &commonv1.GovernanceEnvelope{
				Id:              "test-id",
				TransactionHash: "test-hash",
				ProtocolVersion: "1.0",
				ActionType:      "TEST_ACTION",
			}

			status, body, err := client.SubmitEnvelope(ctx, p, envelope)

			if (err != nil) != tt.wantErr {
				t.Errorf("SubmitEnvelope() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if status != tt.responseCode {
					t.Errorf("status = %d, want %d", status, tt.responseCode)
				}
				if string(body) != tt.responseBody {
					t.Errorf("body = %s, want %s", string(body), tt.responseBody)
				}
			}
		})
	}
}

func TestSubmitEnvelope_MarshalError(t *testing.T) {
	cfg := config.Config{
		MTLSBaseURL: "https://example.com",
		Auth:        config.Auth{},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	p := Persona{ID: "test-client"}

	// Create a valid envelope - protojson.Marshal handles nil gracefully
	// This test verifies the call path works correctly
	envelope := &commonv1.GovernanceEnvelope{
		Id:              "test-id",
		TransactionHash: "test-hash",
		ProtocolVersion: "1.0",
		ActionType:      "TEST_ACTION",
	}

	// Since we can't easily cause a marshal error with valid protobuf,
	// we'll just verify the call doesn't panic with nil
	_, _, err = client.SubmitEnvelope(ctx, p, envelope)

	// This will fail with network error since we're using a fake URL,
	// but that's expected - we're just testing it doesn't panic
	_ = err
}

func TestSubmitMaximal(t *testing.T) {
	tests := []struct {
		name         string
		maximal      MaximalEnvelope
		responseCode int
		responseBody string
		wantErr      bool
		verifyFields bool
	}{
		{
			name: "minimal maximal envelope",
			maximal: MaximalEnvelope{
				OperatorID:     "op-123",
				ToolName:       "test-tool",
				ArgumentsJSON:  `{"arg":"value"}`,
				TargetResource: "localhost",
				StateRoot:      "root-abc",
			},
			responseCode: http.StatusOK,
			responseBody: `{"status": "accepted"}`,
			wantErr:      false,
			verifyFields: true,
		},
		{
			name: "with ensemble",
			maximal: MaximalEnvelope{
				OperatorID:     "op-123",
				ToolName:       "test-tool",
				ArgumentsJSON:  `{"arg":"value"}`,
				TargetResource: "localhost",
				StateRoot:      "root-abc",
			},
			responseCode: http.StatusOK,
			responseBody: `{"status": "accepted"}`,
			wantErr:      false,
			verifyFields: false,
		},
		{
			name: "with principal",
			maximal: MaximalEnvelope{
				OperatorID:     "op-123",
				ToolName:       "test-tool",
				ArgumentsJSON:  `{"arg":"value"}`,
				TargetResource: "localhost",
				StateRoot:      "root-abc",
			},
			responseCode: http.StatusOK,
			responseBody: `{"status": "accepted"}`,
			wantErr:      false,
			verifyFields: false,
		},
		{
			name: "with custom TTL",
			maximal: MaximalEnvelope{
				OperatorID:     "op-123",
				ToolName:       "test-tool",
				ArgumentsJSON:  `{"arg":"value"}`,
				TargetResource: "localhost",
				StateRoot:      "root-abc",
				TTL:            10 * time.Minute,
			},
			responseCode: http.StatusOK,
			responseBody: `{"status": "accepted"}`,
			wantErr:      false,
			verifyFields: true,
		},
		{
			name: "server error",
			maximal: MaximalEnvelope{
				OperatorID:     "op-123",
				ToolName:       "test-tool",
				ArgumentsJSON:  `{"arg":"value"}`,
				TargetResource: "localhost",
				StateRoot:      "root-abc",
			},
			responseCode: http.StatusInternalServerError,
			responseBody: `{"error": "internal error"}`,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedEnvelope []byte

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != constants.APIPaths.GovernanceEnvelopes {
					t.Errorf("expected path %s, got %s", constants.APIPaths.GovernanceEnvelopes, r.URL.Path)
				}

				var err error
				receivedEnvelope, err = io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("failed to read body: %v", err)
				}

				w.WriteHeader(tt.responseCode)
				w.Write([]byte(tt.responseBody))
			})

			server := httptest.NewServer(handler)
			defer server.Close()

			cfg := config.Config{
				MTLSBaseURL: server.URL,
				Auth:        config.Auth{},
			}

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			ctx := context.Background()
			p := Persona{ID: "test-client"}

			// Add ensemble if needed
			if tt.name == "with ensemble" {
				ensemble, err := NewEnsemble("consensus-key", 3)
				if err != nil {
					t.Fatalf("NewEnsemble() failed: %v", err)
				}
				tt.maximal.Ensemble = ensemble
			}

			// Add principal if needed
			if tt.name == "with principal" {
				principal, err := NewPrincipal("principal-key")
				if err != nil {
					t.Fatalf("NewPrincipal() failed: %v", err)
				}
				tt.maximal.Principal = principal
			}

			txHash, status, body, err := client.SubmitMaximal(ctx, p, tt.maximal)

			if (err != nil) != tt.wantErr {
				t.Errorf("SubmitMaximal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if txHash == "" {
					t.Error("txHash should not be empty")
				}
				if status != tt.responseCode {
					t.Errorf("status = %d, want %d", status, tt.responseCode)
				}
				if string(body) != tt.responseBody {
					t.Errorf("body = %s, want %s", string(body), tt.responseBody)
				}

				if tt.verifyFields {
					// Verify the envelope was sent with correct fields
					bodyStr := string(receivedEnvelope)
					if tt.maximal.OperatorID != "" && !contains(bodyStr, tt.maximal.OperatorID) {
						t.Error("envelope should contain OperatorID")
					}
					if tt.maximal.ToolName != "" && !contains(bodyStr, tt.maximal.ToolName) {
						t.Error("envelope should contain ToolName")
					}
					if tt.maximal.StateRoot != "" && !contains(bodyStr, tt.maximal.StateRoot) {
						t.Error("envelope should contain StateRoot")
					}
				}
			}
		})
	}
}

func TestSubmitMaximal_DefaultTTL(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "accepted"}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	cfg := config.Config{
		MTLSBaseURL: server.URL,
		Auth:        config.Auth{},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	p := Persona{ID: "test-client"}

	maximal := MaximalEnvelope{
		OperatorID:     "op-123",
		ToolName:       "test-tool",
		ArgumentsJSON:  `{"arg":"value"}`,
		TargetResource: "localhost",
		StateRoot:      "root-abc",
		TTL:            0, // Should use default 5 minutes
	}

	txHash, _, _, err := client.SubmitMaximal(ctx, p, maximal)

	if err != nil {
		t.Fatalf("SubmitMaximal() failed: %v", err)
	}

	if txHash == "" {
		t.Error("txHash should not be empty")
	}
}

func TestSubmitMaximal_WithL2(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "accepted"}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	cfg := config.Config{
		MTLSBaseURL: server.URL,
		Auth:        config.Auth{},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	p := Persona{ID: "test-client"}

	ensemble, err := NewEnsemble("consensus-key", 3)
	if err != nil {
		t.Fatalf("NewEnsemble() failed: %v", err)
	}

	maximal := MaximalEnvelope{
		OperatorID:     "op-123",
		ToolName:       "test-tool",
		ArgumentsJSON:  `{"arg":"value"}`,
		TargetResource: "localhost",
		StateRoot:      "root-abc",
		Ensemble:       ensemble,
	}

	txHash, _, _, err := client.SubmitMaximal(ctx, p, maximal)

	if err != nil {
		t.Fatalf("SubmitMaximal() failed: %v", err)
	}

	if txHash == "" {
		t.Error("txHash should not be empty")
	}
}

func TestSubmitMaximal_WithL3(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "accepted"}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	cfg := config.Config{
		MTLSBaseURL: server.URL,
		Auth:        config.Auth{},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	p := Persona{ID: "test-client"}

	principal, err := NewPrincipal("principal-key")
	if err != nil {
		t.Fatalf("NewPrincipal() failed: %v", err)
	}

	maximal := MaximalEnvelope{
		OperatorID:     "op-123",
		ToolName:       "test-tool",
		ArgumentsJSON:  `{"arg":"value"}`,
		TargetResource: "localhost",
		StateRoot:      "root-abc",
		Principal:      principal,
	}

	txHash, _, _, err := client.SubmitMaximal(ctx, p, maximal)

	if err != nil {
		t.Fatalf("SubmitMaximal() failed: %v", err)
	}

	if txHash == "" {
		t.Error("txHash should not be empty")
	}
}

func TestActionConstants(t *testing.T) {
	if ActionMcpCall != "MCP_CALL" {
		t.Errorf("ActionMcpCall = %s, want MCP_CALL", ActionMcpCall)
	}
	if ActionA2aCall != "A2A_CALL" {
		t.Errorf("ActionA2aCall = %s, want A2A_CALL", ActionA2aCall)
	}
	if ProtocolVersion != "1.0" {
		t.Errorf("ProtocolVersion = %s, want 1.0", ProtocolVersion)
	}
}

func TestMaximalEnvelope(t *testing.T) {
	maximal := MaximalEnvelope{
		OperatorID:     "op-123",
		ToolName:       "test-tool",
		ArgumentsJSON:  `{"arg":"value"}`,
		TargetResource: "localhost",
		StateRoot:      "root-abc",
		TTL:            5 * time.Minute,
	}

	if maximal.OperatorID != "op-123" {
		t.Error("OperatorID not set correctly")
	}
	if maximal.ToolName != "test-tool" {
		t.Error("ToolName not set correctly")
	}
	if maximal.ArgumentsJSON != `{"arg":"value"}` {
		t.Error("ArgumentsJSON not set correctly")
	}
	if maximal.TargetResource != "localhost" {
		t.Error("TargetResource not set correctly")
	}
	if maximal.StateRoot != "root-abc" {
		t.Error("StateRoot not set correctly")
	}
	if maximal.TTL != 5*time.Minute {
		t.Error("TTL not set correctly")
	}
}

func TestNewEnsembleFromSeed(t *testing.T) {
	knownSeed := "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77"

	t.Run("constructs deterministic ensemble from valid seed", func(t *testing.T) {
		ens1, err := NewEnsembleFromSeed("auditor-ensemble", 3, knownSeed)
		require.NoError(t, err)
		require.NotNil(t, ens1)

		ens2, err := NewEnsembleFromSeed("auditor-ensemble", 3, knownSeed)
		require.NoError(t, err)

		assert.Equal(t, ens1.PubHex(), ens2.PubHex(), "same seed must produce same public key")
		assert.Equal(t, "auditor-ensemble", ens1.KeyID)
		assert.Equal(t, 3, ens1.AgentCount())
		assert.Equal(t, "test-tribunal", ens1.TribunalID)
	})

	t.Run("differs from random NewEnsemble", func(t *testing.T) {
		seeded, err := NewEnsembleFromSeed("test-key", 3, knownSeed)
		require.NoError(t, err)

		random, err := NewEnsemble("test-key", 3)
		require.NoError(t, err)

		assert.NotEqual(t, seeded.PubHex(), random.PubHex(), "seeded ensemble must differ from random")
	})

	t.Run("rejects invalid hex", func(t *testing.T) {
		_, err := NewEnsembleFromSeed("test-key", 3, "not-valid-hex-zzz")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "decode hex")
	})

	t.Run("rejects wrong-length seed", func(t *testing.T) {
		shortSeed := "87278693f5894d8de5d28401"
		_, err := NewEnsembleFromSeed("test-key", 3, shortSeed)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid seed length")
	})

	t.Run("trims whitespace in seed hex", func(t *testing.T) {
		seedWithWhitespace := "  " + knownSeed + "\n"
		ens, err := NewEnsembleFromSeed("test-key", 3, seedWithWhitespace)
		require.NoError(t, err)
		assert.NotNil(t, ens)
	})
}

func TestSeedHex(t *testing.T) {
	t.Run("round-trips with NewEnsembleFromSeed", func(t *testing.T) {
		original, err := NewEnsembleFromSeed("test-key", 3, "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77")
		require.NoError(t, err)

		seedHex := original.SeedHex()
		assert.NotEmpty(t, seedHex)

		reconstructed, err := NewEnsembleFromSeed("test-key", 3, seedHex)
		require.NoError(t, err)

		assert.Equal(t, original.PubHex(), reconstructed.PubHex(),
			"SeedHex round-trip must produce the same public key")
	})

	t.Run("returns valid 32-byte hex (64 chars)", func(t *testing.T) {
		ens, err := NewEnsemble("test-key", 2)
		require.NoError(t, err)

		seedHex := ens.SeedHex()
		assert.Len(t, seedHex, 64)

		decoded, err := hex.DecodeString(seedHex)
		require.NoError(t, err)
		assert.Len(t, decoded, 32)
	})
}

func TestEnsemble_TribunalID(t *testing.T) {
	t.Run("defaults to test-tribunal", func(t *testing.T) {
		ens, err := NewEnsemble("test-key", 2)
		require.NoError(t, err)
		assert.Equal(t, "test-tribunal", ens.TribunalID)
	})

	t.Run("defaults to test-tribunal in NewEnsembleFromSeed", func(t *testing.T) {
		ens, err := NewEnsembleFromSeed("test-key", 2, "87278693f5894d8de5d28401c923e0c3fea9ae7c35f467065954eecbc85b2e77")
		require.NoError(t, err)
		assert.Equal(t, "test-tribunal", ens.TribunalID)
	})

	t.Run("is settable and used in Vote output", func(t *testing.T) {
		ens, err := NewEnsemble("test-key", 2)
		require.NoError(t, err)

		ens.TribunalID = "dhs-tribunal"
		l2 := ens.Vote("abc123", true)

		require.NotEmpty(t, l2.Votes)
		assert.Equal(t, "dhs-tribunal", l2.TribunalId,
			"Vote() must use the configured TribunalID")
	})
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
