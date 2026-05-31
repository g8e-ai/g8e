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

//go:build integration

package scenario

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/auditor/client"
	"github.com/g8e-ai/g8e/internal/auditor/config"
	harnesspkg "github.com/g8e-ai/g8e/internal/auditor/harness"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// TestContext holds the test infrastructure for a single test run.
// No global state - each test gets its own isolated context.
type TestContext struct {
	Harness  *harnesspkg.TestHarness
	Client   *client.Client
	BaseURL  string
	CertPath string
	KeyPath  string
	CAPath   string
	PrivKey  ed25519.PrivateKey
	PubKey   ed25519.PublicKey
}

// setupTestContext creates and starts the test harness with a real operator/gateway.
// Returns a TestContext with mTLS client ready for use.
func setupTestContext(t *testing.T) *TestContext {
	t.Helper()

	// Create harness config
	harnessCfg := harnesspkg.DefaultConfig()

	// Use absolute path to binary from repo root
	repoRoot, _ := os.Getwd()
	for i := 0; i < 2; i++ {
		repoRoot = filepath.Dir(repoRoot)
	}
	binaryPath := filepath.Join(repoRoot, "bin", "g8e")
	harnessCfg.Binary = binaryPath
	harnessCfg.Posture = "doctrine" // Start in doctrine mode for fastest tests

	// Create test harness
	harness, err := harnesspkg.New(harnessCfg)
	if err != nil {
		t.Fatalf("failed to create test harness: %v", err)
	}

	// Start harness
	if err := harness.Start(harnessCfg.Posture); err != nil {
		t.Fatalf("failed to start test harness: %v", err)
	}

	// Ensure cleanup
	t.Cleanup(func() {
		harness.Stop()
	})

	// Generate test client keys for signing
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate test client keys: %v", err)
	}

	// Enroll test client via real gateway PKI system
	sessionID := fmt.Sprintf("test-cli-session-%d", time.Now().UnixNano())
	clientCertPath, clientKeyPath, caBundlePath, err := harness.EnrollTestClient("test-user", sessionID)
	if err != nil {
		t.Fatalf("failed to enroll test client: %v", err)
	}

	// Create auditor client for HTTP submission
	auditorCfg := config.Default()
	auditorCfg.MTLSBaseURL = harness.GatewayURL()
	auditorCfg.Auth.ClientCert = clientCertPath
	auditorCfg.Auth.ClientKey = clientKeyPath
	auditorCfg.Auth.CABundle = caBundlePath
	auditorCfg.Auth.Insecure = false // Use real TLS verification

	testClient, err := client.New(auditorCfg)
	if err != nil {
		t.Fatalf("failed to create auditor client: %v", err)
	}

	return &TestContext{
		Harness:  harness,
		Client:   testClient,
		BaseURL:  harness.GatewayURL(),
		CertPath: clientCertPath,
		KeyPath:  clientKeyPath,
		CAPath:   caBundlePath,
		PrivKey:  priv,
		PubKey:   pub,
	}
}

func TestScenarios(t *testing.T) {
	// Setup test infrastructure
	ctx := setupTestContext(t)

	// Build a valid envelope using the builder
	intentBytes, err := New().
		WithCommand("echo hello").
		WithOperatorSessionID("test-scenario-session").
		WithL2(ctx.PrivKey, true).
		Build()
	if err != nil {
		t.Fatalf("failed to build test envelope: %v", err)
	}

	// Submit via real HTTP client
	result := submitViaHTTP(t, ctx.Client, intentBytes)

	// Assert acceptance (doctrine mode accepts valid L1 commands)
	if result.Error != nil {
		t.Errorf("expected acceptance, got error: %v", result.Error)
	}
	if result.Receipt == nil {
		t.Error("expected receipt, got nil")
	}

	// Assert receipt persistence via API
	assertReceiptPersisted(t, ctx.Client, result.Receipt.TransactionId)
}

// TestNegativeControls verifies the test suite can detect failures by intentionally
// submitting malformed envelopes. This is a negative control test - it passes when
// malformed envelopes are correctly rejected.
func TestNegativeControls(t *testing.T) {
	ctx := setupTestContext(t)

	t.Run("bad_id_rejection", func(t *testing.T) {
		intentBytes, err := New().
			WithCommand("echo hello").
			WithBadID().
			Build()
		if err != nil {
			t.Fatalf("failed to build envelope: %v", err)
		}

		result := submitViaHTTP(t, ctx.Client, intentBytes)
		if result.Error == nil {
			t.Error("expected rejection for bad ID, got acceptance")
		}
		if result.Receipt != nil {
			t.Error("expected nil receipt for bad ID")
		}
	})

	t.Run("bad_hash_rejection", func(t *testing.T) {
		intentBytes, err := New().
			WithCommand("echo hello").
			WithBadHash().
			Build()
		if err != nil {
			t.Fatalf("failed to build envelope: %v", err)
		}

		result := submitViaHTTP(t, ctx.Client, intentBytes)
		if result.Error == nil {
			t.Error("expected rejection for bad hash, got acceptance")
		}
		if result.Receipt != nil {
			t.Error("expected nil receipt for bad hash")
		}
	})

	t.Run("bad_signature_rejection", func(t *testing.T) {
		intentBytes, err := New().
			WithCommand("echo hello").
			WithL2(ctx.PrivKey, true).
			WithBadSignature().
			Build()
		if err != nil {
			t.Fatalf("failed to build envelope: %v", err)
		}

		result := submitViaHTTP(t, ctx.Client, intentBytes)
		if result.Error == nil {
			t.Error("expected rejection for bad signature, got acceptance")
		}
		if result.Receipt != nil {
			t.Error("expected nil receipt for bad signature")
		}
	})
}

// Result represents the outcome of submitting a scenario through the admission path.
type Result struct {
	Receipt         *operatorv1.ActionReceipt
	Error           error
	ComputedID      string
	EnvelopeID      string
	TransactionHash string
}

// submitViaHTTP submits an envelope via the auditor client and returns the result.
func submitViaHTTP(t *testing.T, auditorClient *client.Client, intent []byte) Result {
	t.Helper()

	ctx := context.Background()
	persona := client.Persona{ID: "scenario-test", UserAgent: "g8e-scenario-tests"}

	// Decode intent to get envelope for submission
	var envelope commonv1.GovernanceEnvelope
	if err := protojson.Unmarshal(intent, &envelope); err != nil {
		return Result{Error: fmt.Errorf("failed to unmarshal envelope: %w", err)}
	}

	status, body, err := auditorClient.SubmitEnvelope(ctx, persona, &envelope)

	res := Result{
		EnvelopeID:      envelope.Id,
		TransactionHash: envelope.TransactionHash,
	}

	if err != nil {
		res.Error = fmt.Errorf("HTTP submission failed: %w", err)
		return res
	}

	if status >= 400 {
		res.Error = fmt.Errorf("gateway rejected with status %d: %s", status, string(body))
		return res
	}

	// Parse response to extract receipt if successful
	var response struct {
		Receipt *operatorv1.ActionReceipt `json:"receipt"`
	}
	if err := json.Unmarshal(body, &response); err == nil && response.Receipt != nil {
		res.Receipt = response.Receipt
	}

	return res
}

// assertReceiptPersisted verifies that a receipt is persisted via the API.
func assertReceiptPersisted(t *testing.T, auditorClient *client.Client, transactionID string) {
	t.Helper()

	ctx := context.Background()
	receipt, _, err := auditorClient.GetReceipt(ctx, transactionID)
	if err != nil {
		t.Fatalf("failed to query receipt: %v", err)
	}
	if receipt == nil {
		t.Fatalf("receipt not found for transaction ID %s", transactionID)
	}
	if receipt.TransactionID == "" {
		t.Fatalf("receipt has empty transaction_id")
	}
	if receipt.TransactionHash == "" {
		t.Fatalf("receipt has empty transaction_hash")
	}
}
