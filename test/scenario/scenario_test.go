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

//go:build e2e

package scenario

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	cliconfig "github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/emulator/client"
	"github.com/g8e-ai/g8e/internal/emulator/config"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	defaultHTTPRetryMaxAttempts = 60
	defaultHTTPRetryInterval    = 1 * time.Second
	defaultHTTPRetryTimeout     = 70 * time.Second
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

	// Initialize paths relative to project root
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	// If running from test/scenario, base is ../../. If from root, base is ./
	base := "./"
	if filepath.Base(cwd) == "scenario" {
		base = "../../"
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		t.Fatalf("failed to get absolute path for base %s: %v", base, err)
	}

	if err := constants.InitPathsWithBase(absBase); err != nil {
		t.Fatalf("failed to initialize paths: %v", err)
	}
	// The projectRoot should be the directory containing .g8e, not the .g8e directory itself,
	// because cliconfig.Load and constants.InitPathsWithBase expect the base directory.
	projectRoot := absBase

	cliCfg, err := cliconfig.Load(projectRoot)
	if err != nil {
		t.Fatalf("failed to load CLI config: %v", err)
	}

	// Bootstrap CLI authentication if not already done (matches ./g8e gw start behavior)
	// Check if credentials are stale (> 45 min old) and re-enroll if needed
	credsPath := filepath.Join(projectRoot, ".g8e", "credentials")
	if info, statErr := os.Stat(credsPath); statErr == nil {
		if time.Since(info.ModTime()) >= 45*time.Minute {
			t.Logf("Credentials are stale (%v old), re-enrolling...", time.Since(info.ModTime()).Round(time.Second))
			// Delete stale credentials to force re-enrollment
			os.Remove(credsPath)
			os.Remove(cliCfg.CLICertFile())
			os.Remove(cliCfg.CLIKeyFile())
		}
	}

	// CLI authentication must be performed explicitly via 'g8e auth login'
	// Tests should set up credentials via the proper auth flow
	creds, err := auth.LoadCredentials(cliCfg)
	if err != nil || creds == nil {
		t.Fatalf("CLI credentials required for tests. Please authenticate via 'g8e auth login'")
	}

	// Ensure gateway is running and governance is ready before proceeding
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/health", constants.Ports.OperatorHttp)
	t.Logf("Waiting for gateway to be governance-ready at %s...", healthURL)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(healthURL)
		if err != nil {
			t.Logf("Gateway not ready yet, retrying... (%v)", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Logf("Gateway returned status %d, retrying...", resp.StatusCode)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		var health struct {
			Status          string `json:"status"`
			GovernanceReady bool   `json:"governance_ready"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
			t.Logf("Failed to decode health response, retrying... (%v)", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if health.GovernanceReady {
			t.Logf("Gateway is governance-ready")
			break
		}

		t.Logf("Gateway not governance-ready yet, retrying...")
		time.Sleep(500 * time.Millisecond)
	}

	if time.Now().After(deadline) {
		t.Fatal("gateway did not become governance-ready within timeout - run './g8e gw start' first")
	}

	// Paths to local PKI material (bootstrapped via ./g8e gw start)
	clientCertPath := cliCfg.CLICertFile()
	clientKeyPath := cliCfg.CLIKeyFile()
	// Use the CA bundle from centralized constants as per docs/devs/tests.md
	caBundlePath := constants.Paths.Infra.CaCertPath

	// Verify certificates exist
	if _, err := os.Stat(clientCertPath); os.IsNotExist(err) {
		t.Fatalf("client cert not found at %s - run './g8e gw start' first", clientCertPath)
	}
	if _, err := os.Stat(caBundlePath); os.IsNotExist(err) {
		t.Fatalf("CA bundle not found at %s - run './g8e gw start' first", caBundlePath)
	}

	// Create auditor client for HTTP submission
	auditorCfg := config.Default()
	auditorCfg.UseCLIConfig = false // Don't auto-load from CLI config, we set paths explicitly
	auditorCfg.MTLSBaseURL = constants.LocalhostHTTPSURL(constants.Ports.OperatorHttps)
	auditorCfg.PublicBaseURL = constants.LocalhostHTTPSURL(constants.Ports.OperatorHttps)
	auditorCfg.Auth.ClientCert = clientCertPath
	auditorCfg.Auth.ClientKey = clientKeyPath
	auditorCfg.Auth.CABundle = caBundlePath
	auditorCfg.Auth.Insecure = true // Skip verify for local dev with self-signed certs
	auditorCfg.Verbose = true       // Echo requests to stderr for debugging

	// Load CLI credentials (bootstrapped by ./g8e gw start)
	creds, err = auth.LoadCredentials(cliCfg)
	if err != nil {
		t.Fatalf("failed to load CLI credentials: %v", err)
	}
	if creds == nil {
		t.Fatalf("no CLI credentials found - run './g8e gw start' first")
	}

	t.Logf("Creating auditor client with Cert: %s, Key: %s, CA: %s", clientCertPath, clientKeyPath, caBundlePath)
	testClient, err := client.New(auditorCfg)
	if err != nil {
		t.Fatalf("failed to create auditor client: %v", err)
	}

	// For gateway-only testing, use CLI session ID as operator session ID
	// The gateway validates envelopes using the session ID in the envelope body
	operatorSessionID := creds.CLISessionID
	if operatorSessionID == "" {
		t.Fatal("CLI session ID is empty - run './g8e gw start' first")
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
		CLISessionID:      creds.CLISessionID,
	}
}

func TestScenarios(t *testing.T) {
	// Setup test infrastructure
	ctx := setupTestContext(t)

	// Fetch actual state root from gateway via mTLS port
	// Note: StateRoot uses PublicBaseURL by default, but in full cert mode
	// all ports require mTLS, so we need to use the mTLS endpoint directly
	stateRoot, err := ctx.Client.StateRootFromMTLS(context.Background())
	if err != nil {
		t.Fatalf("failed to fetch state root: %v", err)
	}
	t.Logf("State root from gateway: %q", stateRoot)

	// Build a valid envelope using the builder
	intentBytes, err := New().
		WithCommand("echo hello").
		WithOperatorID("").
		WithOperatorSessionID(ctx.OperatorSessionID).
		WithStateRoot(stateRoot).
		WithL2(ctx.PrivKey, true).
		Build()
	if err != nil {
		t.Fatalf("failed to build test envelope: %v", err)
	}

	// Submit via real HTTP client
	creds := &auth.Credentials{
		CLISessionID:      ctx.CLISessionID,
		UserID:            "", // Not used in gateway-only mode
		OperatorID:        "", // Not used in gateway-only mode
		OperatorSessionID: ctx.OperatorSessionID,
	}
	result := submitViaHTTP(t, ctx.Client, intentBytes, creds)

	// Assert acceptance (doctrine mode accepts valid L1 commands)
	if result.Error != nil {
		t.Errorf("expected acceptance, got error: %v", result.Error)
	}
	if result.Receipt == nil {
		t.Error("expected receipt, got nil")
		return
	}

	// Assert receipt has required fields
	if result.Receipt.TransactionId == "" {
		t.Error("receipt has empty transaction_id")
	}
	if result.Receipt.TransactionHash == "" {
		t.Error("receipt has empty transaction_hash")
	}
	if result.Receipt.Signature == "" {
		t.Error("receipt has empty signature")
	}

	// TODO: Re-enable receipt persistence check once audit API mTLS is fixed
	// assertReceiptPersisted(t, ctx.Client, result.Receipt.TransactionId)
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

		creds := &auth.Credentials{
			CLISessionID:      ctx.CLISessionID,
			UserID:            "",
			OperatorID:        "",
			OperatorSessionID: ctx.OperatorSessionID,
		}
		result := submitViaHTTP(t, ctx.Client, intentBytes, creds)
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

		creds := &auth.Credentials{
			CLISessionID:      ctx.CLISessionID,
			UserID:            "",
			OperatorID:        "",
			OperatorSessionID: ctx.OperatorSessionID,
		}
		result := submitViaHTTP(t, ctx.Client, intentBytes, creds)
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

		creds := &auth.Credentials{
			CLISessionID:      ctx.CLISessionID,
			UserID:            "",
			OperatorID:        "",
			OperatorSessionID: ctx.OperatorSessionID,
		}
		result := submitViaHTTP(t, ctx.Client, intentBytes, creds)
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
// Retries on 503 (envelope processor not initialized) up to the configured timeout.
func submitViaHTTP(t *testing.T, auditorClient *client.Client, intent []byte, creds *auth.Credentials) Result {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), defaultHTTPRetryTimeout)
	defer cancel()

	// Authenticate as CLI session since we're using a CLI certificate
	// The envelope body contains operator_session_id for governance validation
	persona := client.Persona{
		ID:                "scenario-test",
		UserAgent:         "g8e-scenario-tests",
		CLISessionID:      creds.CLISessionID,
		UserID:            creds.UserID,
		OperatorID:        creds.OperatorID,
		OperatorSessionID: creds.OperatorSessionID,
	}

	// Decode intent to get envelope for submission
	var envelope commonv1.GovernanceEnvelope
	if err := protojson.Unmarshal(intent, &envelope); err != nil {
		return Result{Error: fmt.Errorf("failed to unmarshal envelope: %w", err)}
	}

	// Retry on 503 (envelope processor not initialized)
	for i := 0; i < defaultHTTPRetryMaxAttempts; i++ {
		select {
		case <-ctx.Done():
			return Result{Error: fmt.Errorf("envelope processor not ready after %v (operator may not be running or command service not started)", defaultHTTPRetryTimeout)}
		default:
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

		if status == http.StatusServiceUnavailable {
			t.Logf("Envelope processor not ready, retrying (%d/%d)...", i+1, defaultHTTPRetryMaxAttempts)
			time.Sleep(defaultHTTPRetryInterval)
			continue
		}

		if status >= 400 {
			res.Error = fmt.Errorf("gateway rejected with status %d: %s", status, string(body))
			return res
		}

		// Parse response to extract receipt if successful
		// Gateway returns receipt directly as JSON, not wrapped
		var receipt operatorv1.ActionReceipt
		if err := json.Unmarshal(body, &receipt); err == nil && receipt.TransactionId != "" {
			res.Receipt = &receipt
		}

		return res
	}

	return Result{Error: fmt.Errorf("envelope processor not ready after %v (operator may not be running or command service not started)", defaultHTTPRetryTimeout)}
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
