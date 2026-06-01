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

	"github.com/g8e-ai/g8e/internal/auditor/client"
	"github.com/g8e-ai/g8e/internal/auditor/config"
	"github.com/g8e-ai/g8e/internal/cli/auth"
	cliconfig "github.com/g8e-ai/g8e/internal/cli/config"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// TestContext holds the test infrastructure for a single test run.
type TestContext struct {
	Client            *client.Client
	BaseURL           string
	CertPath          string
	KeyPath           string
	CAPath            string
	PrivKey           ed25519.PrivateKey
	PubKey            ed25519.PublicKey
	OperatorSessionID string
	CLISessionID      string
}

// setupTestContext connects to a real running operator/gateway.
// Returns a TestContext with mTLS client ready for use.
func setupTestContext(t *testing.T) *TestContext {
	t.Helper()

	// Load CLI config to get ports and paths (use project root, not test directory)
	projectRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	// Navigate to project root (test/scenario -> g8e root)
	for len(projectRoot) > 0 && filepath.Base(projectRoot) != "g8e" {
		projectRoot = filepath.Dir(projectRoot)
		if projectRoot == "/" || projectRoot == "." {
			t.Fatalf("could not find g8e project root from %s", projectRoot)
		}
	}

	cliCfg, err := cliconfig.Load(projectRoot)
	if err != nil {
		t.Fatalf("failed to load CLI config: %v", err)
	}

	// Paths to local PKI material (bootstrapped via ./g8e auth login)
	clientCertPath := cliCfg.CLICertFile()
	clientKeyPath := cliCfg.CLIKeyFile()
	// Use the CA bundle saved to credentials directory during auth login
	// This is the CA that actually signed the client cert
	caBundlePath := filepath.Join(cliCfg.CredentialsDir, "g8eg-ca-bundle.pem")

	// Verify certificates exist
	if _, err := os.Stat(clientCertPath); os.IsNotExist(err) {
		t.Fatalf("client cert not found at %s - run './g8e auth login' first", clientCertPath)
	}
	if _, err := os.Stat(caBundlePath); os.IsNotExist(err) {
		t.Fatalf("CA bundle not found at %s - run './g8e auth login' first", caBundlePath)
	}

	// Create auditor client for HTTP submission
	auditorCfg := config.Default()
	auditorCfg.UseCLIConfig = false // Don't auto-load from CLI config, we set paths explicitly
	auditorCfg.MTLSBaseURL = fmt.Sprintf("https://localhost:%d", cliCfg.Paths.Ports.OperatorHTTPS)
	auditorCfg.PublicBaseURL = fmt.Sprintf("https://localhost:%d", cliCfg.Paths.Ports.OperatorPublicHTTPS)
	auditorCfg.Auth.ClientCert = clientCertPath
	auditorCfg.Auth.ClientKey = clientKeyPath
	auditorCfg.Auth.CABundle = caBundlePath
	auditorCfg.Auth.Insecure = false

	// Load operator session ID from CLI credentials
	creds, err := auth.LoadCredentials(cliCfg)
	if err != nil {
		t.Fatalf("failed to load CLI credentials: %v", err)
	}
	if creds == nil {
		t.Fatalf("no CLI credentials found - run './g8e auth login' first")
	}
	auditorCfg.OperatorSessionID = creds.OperatorSessionID

	testClient, err := client.New(auditorCfg)
	if err != nil {
		t.Fatalf("failed to create auditor client: %v", err)
	}

	// Discover live operator session (should use the one we loaded)
	ctx := context.Background()
	operatorSessionID := testClient.DiscoverOperatorSession(ctx)
	if operatorSessionID == "" {
		t.Fatal("failed to discover live operator session - is the platform running? (./g8e gw start)")
	}

	// Generate test client keys for signing (these are for L2 consensus simulation in tests)
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate test client keys: %v", err)
	}

	return &TestContext{
		Client:            testClient,
		BaseURL:           auditorCfg.MTLSBaseURL,
		CertPath:          clientCertPath,
		KeyPath:           clientKeyPath,
		CAPath:            caBundlePath,
		PrivKey:           priv,
		PubKey:            pub,
		OperatorSessionID: operatorSessionID,
		CLISessionID:      "test-cli-session", // Can be anything for these tests
	}
}

func TestScenarios(t *testing.T) {
	// Setup test infrastructure
	ctx := setupTestContext(t)

	// Fetch actual state root from gateway
	stateRoot, err := ctx.Client.StateRoot(context.Background())
	if err != nil {
		t.Fatalf("failed to fetch state root: %v", err)
	}

	// Build a valid envelope using the builder
	intentBytes, err := New().
		WithCommand("echo hello").
		WithOperatorSessionID(ctx.OperatorSessionID).
		WithStateRoot(stateRoot).
		WithL2(ctx.PrivKey, true).
		Build()
	if err != nil {
		t.Fatalf("failed to build test envelope: %v", err)
	}

	// Submit via real HTTP client
	result := submitViaHTTP(t, ctx.Client, intentBytes, ctx.OperatorSessionID)

	// Assert acceptance (doctrine mode accepts valid L1 commands)
	if result.Error != nil {
		t.Errorf("expected acceptance, got error: %v", result.Error)
	}
	if result.Receipt == nil {
		t.Error("expected receipt, got nil")
		return
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

		result := submitViaHTTP(t, ctx.Client, intentBytes, ctx.OperatorSessionID)
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

		result := submitViaHTTP(t, ctx.Client, intentBytes, ctx.OperatorSessionID)
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

		result := submitViaHTTP(t, ctx.Client, intentBytes, ctx.OperatorSessionID)
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
func submitViaHTTP(t *testing.T, auditorClient *client.Client, intent []byte, operatorSessionID string) Result {
	t.Helper()

	ctx := context.Background()
	persona := client.Persona{ID: "scenario-test", UserAgent: "g8e-scenario-tests", OperatorSessionID: operatorSessionID}

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
