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
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/emulator/client"
	emulatorconfig "github.com/g8e-ai/g8e/internal/emulator/config"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/g8e-ai/g8e/test/fixtures"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	defaultHTTPRetryMaxAttempts = 60
	defaultHTTPRetryInterval    = 1 * time.Second
	defaultHTTPRetryTimeout     = 5 * time.Second
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
	Fixture           *fixtures.GatewayFixture
	Identity          *fixtures.ClientIdentity
}

// setupTestContext spins up an in-process gateway via GatewayFixture
// and enrolls a client identity for mTLS authentication.
// Returns a TestContext with mTLS client ready for use.
func setupTestContext(t *testing.T) *TestContext {
	t.Helper()

	// Create in-process gateway
	f := fixtures.NewGatewayFixture(t, fixtures.GatewayFixtureOptions{
		TestName: "scenario-test",
		Posture:  config.PostureDoctrine,
	})

	// Enroll a client identity for mTLS authentication
	identity := fixtures.EnrollClientIdentity(t, f, "scenario-user", "scenario-org", "scenario-fingerprint", "scenario-host")

	// Write certificates to temp files for emulator client
	certFile, err := os.CreateTemp("", "scenario-cert-*.pem")
	if err != nil {
		t.Fatalf("failed to create temp cert file: %v", err)
	}
	defer os.Remove(certFile.Name())
	if _, err := certFile.Write(identity.Certificate); err != nil {
		t.Fatalf("failed to write cert file: %v", err)
	}
	certFile.Close()

	keyFile, err := os.CreateTemp("", "scenario-key-*.pem")
	if err != nil {
		t.Fatalf("failed to create temp key file: %v", err)
	}
	defer os.Remove(keyFile.Name())
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: identity.PrivateKey})
	if _, err := keyFile.Write(keyPEM); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}
	keyFile.Close()

	// Read CA bundle from PKI dir
	caBundlePath := f.PKIDir + "/trust/g8eg-ca-bundle.pem"

	// Create emulator client for HTTP submission
	mtlsURL := constants.LocalhostHTTPSURL(f.Service.GetHTTPSPort())
	auditorCfg := emulatorconfig.Default()
	auditorCfg.UseCLIConfig = false
	auditorCfg.MTLSBaseURL = mtlsURL
	auditorCfg.PublicBaseURL = mtlsURL
	auditorCfg.Auth.ClientCert = certFile.Name()
	auditorCfg.Auth.ClientKey = keyFile.Name()
	auditorCfg.Auth.CABundle = caBundlePath
	auditorCfg.Auth.Insecure = true
	auditorCfg.Verbose = true

	t.Logf("Creating auditor client with Cert: %s, Key: %s, CA: %s", certFile.Name(), keyFile.Name(), caBundlePath)
	testClient, err := client.New(auditorCfg)
	if err != nil {
		t.Fatalf("failed to create auditor client: %v", err)
	}

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate test client keys: %v", err)
	}

	return &TestContext{
		Client:            testClient,
		BaseURL:           mtlsURL,
		CertPath:          certFile.Name(),
		KeyPath:           keyFile.Name(),
		CAPath:            caBundlePath,
		PrivKey:           priv,
		PubKey:            pub,
		OperatorSessionID: identity.OperatorSessionID,
		CLISessionID:      identity.OperatorSessionID,
		Fixture:           f,
		Identity:          identity,
	}
}

func TestScenarios(t *testing.T) {
	// Setup test infrastructure
	ctx := setupTestContext(t)
	defer ctx.Fixture.Cleanup()

	// Fetch the current state root via the public health API so the envelope
	// binds to the same state the gateway will verify against.
	stateRoot, err := ctx.Client.StateRoot(context.Background())
	if err != nil {
		t.Fatalf("failed to fetch state root: %v", err)
	}
	if stateRoot == "" {
		t.Fatal("gateway returned empty state root")
	}

	// Build a valid envelope using the builder
	intentBytes, err := New().
		WithCommand("echo hello").
		WithOperatorID(ctx.Identity.OperatorID).
		WithOperatorSessionID(ctx.OperatorSessionID).
		WithStateRoot(stateRoot).
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
	defer ctx.Fixture.Cleanup()

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
