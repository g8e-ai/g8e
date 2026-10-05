// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	govpkg "github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
)

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
				ProtocolVersion: govpkg.GovernanceProtocolVersionV2,
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
