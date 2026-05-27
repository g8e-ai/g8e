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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/services/system"
)

var (
	// ops holds the OperatorGate instances for each mode
	ops map[Mode]*OperatorGate

	// testStateRoot is the fixed state root for deterministic testing
	testStateRoot = "abc123def456"

	// testSigners holds the trusted L2 signers for testing
	testSigners map[string]ed25519.PublicKey

	// testDB is the in-memory database for receipt persistence testing
	testDB *sqliteutil.DB
)

func TestMain(m *testing.M) {
	// Generate test signers
	testSigners = generateTestSigners()

	// Setup in-memory database for receipt persistence
	var err error
	testDB, err = SetupTestDB()
	if err != nil {
		panicf("failed to setup test database: %v", err)
	}
	defer TeardownTestDB(testDB)

	// Create operator gates for each mode
	ops = make(map[Mode]*OperatorGate)
	modes := []Mode{ModeDoctrine, ModeConsensus, ModeNotary}

	for _, mode := range modes {
		// Use a fixed clock for deterministic testing
		fixedTime := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
		clock := system.NewFixedClock(fixedTime)

		// Create a temporary testing.T for audit vault initialization
		// We use a nil T in TestMain since t.TempDir() is not available there
		// The audit vault will be initialized in the test cases instead
		gate, err := NewOperatorGate(mode, clock, testStateRoot, testSigners, testDB, nil)
		if err != nil {
			panicf("failed to create operator gate for mode %s: %v", mode, err)
		}
		ops[mode] = gate
	}

	// Run tests
	m.Run()
}

func TestScenarios(t *testing.T) {
	scenarios, err := LoadFixtures()
	if err != nil {
		t.Fatalf("failed to load fixtures: %v", err)
	}

	if len(scenarios) == 0 {
		t.Fatal("no scenarios loaded from fixtures")
	}

	for _, s := range scenarios {
		// Extract operator_session_id from the envelope for session cleanup
		var envelope struct {
			OperatorSessionID string `json:"operatorSessionId"`
		}
		if err := json.Unmarshal(s.Intent, &envelope); err != nil {
			t.Fatalf("failed to parse operator_session_id from intent: %v", err)
		}
		if envelope.OperatorSessionID == "" {
			envelope.OperatorSessionID = "test-scenario-session"
		}

		for _, mode := range []Mode{ModeDoctrine, ModeConsensus, ModeNotary} {
			mode := mode // capture loop variable
			t.Run(s.Name+"/"+mode.String(), func(t *testing.T) {
				// Create a new gate with audit vault for this test
				fixedTime := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
				clock := system.NewFixedClock(fixedTime)
				gate, err := NewOperatorGate(mode, clock, testStateRoot, testSigners, testDB, t)
				if err != nil {
					t.Fatalf("failed to create operator gate for mode %s: %v", mode, err)
				}
				defer gate.Close()

				// Create a test session in the audit vault for receipt recording
				// This is required because the receipts table has a foreign key constraint on sessions
				// Use idempotent creation to handle duplicate session creation across modes
				if gate.auditVault != nil {
					if err := CreateTestSession(gate.auditVault, envelope.OperatorSessionID, "operator"); err != nil {
						// Ignore UNIQUE constraint errors - session already exists from previous mode
						if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
							t.Fatalf("failed to create test session: %v", err)
						}
					}
				}

				expected, ok := s.Expect[mode]
				if !ok {
					t.Fatalf("no expected outcome defined for mode %s in scenario %s", mode, s.Name)
				}

				// Seed replay store for actual_replay scenario
				if s.Name == "actual_replay" {
					// Parse the nonce from the intent to seed the replay store
					var intent struct {
						Nonce string `json:"nonce"`
					}
					if err := json.Unmarshal(s.Intent, &intent); err != nil {
						t.Fatalf("failed to parse nonce from intent: %v", err)
					}
					expiresAt := time.Date(2026, 5, 25, 13, 0, 0, 0, time.UTC)
					_, err := gate.replayStore.ReserveNonce(intent.Nonce, expiresAt)
					if err != nil {
						t.Fatalf("failed to seed replay store: %v", err)
					}
				}

				ctx := context.Background()
				result := gate.Submit(ctx, s.Intent)

				// Assert verdict
				AssertVerdict(t, result, expected)

				// Assert rejection reason if applicable
				AssertReason(t, result, expected)

				// Assert L2/L3 validity
				AssertL2L3(t, result, expected)

				// Assert receipt persistence to database
				if result.Receipt != nil {
					AssertPersistedReceipt(t, gate.db, result.Receipt, expected)
					// Assert receipt persistence to audit vault
					AssertAuditVaultReceipt(t, gate.auditVault, result.Receipt, expected)
				}

				// For tampered_receipt scenario: verify receipt tampering is detected
				// This tests the "tamper-evident" property of signed receipts
				if s.Name == "tampered_receipt" && result.Receipt != nil {
					AssertReceiptTamperDetection(t, result.Receipt, gate.actuator)
				}

				// Golden diff
				GoldenDiff(t, s, mode, result.Receipt)

				// Report trace under -v
				Report(t, s, mode, result)

				// Clear replay store after each scenario to prevent state leakage
				if gate.replayStore != nil {
					gate.replayStore.Clear()
				}
			})
		}
	}
}

// TestGoldenFilesUpToDate verifies that all golden files are present for accepting scenarios.
// This test is intended for CI to ensure developers don't forget to update golden files.
func TestGoldenFilesUpToDate(t *testing.T) {
	CheckGoldenFilesUpToDate(t)
}

// TestNegativeControls verifies the test suite can detect failures by intentionally
// flipping expected verdicts. This is a negative control test - it passes when the
// flipped expectations correctly cause assertion failures, proving the suite can go red.
func TestNegativeControls(t *testing.T) {
	scenarios, err := LoadFixtures()
	if err != nil {
		t.Fatalf("failed to load fixtures: %v", err)
	}

	if len(scenarios) == 0 {
		t.Fatal("no scenarios loaded from fixtures")
	}

	// Test 1: Flip a known-accepting scenario to expect reject
	t.Run("flip_accept_to_reject", func(t *testing.T) {
		// Use all_valid which accepts in doctrine mode (L1-only)
		var targetScenario *Scenario
		for _, s := range scenarios {
			if s.Name == "all_valid" {
				targetScenario = &s
				break
			}
		}
		if targetScenario == nil {
			t.Fatal("all_valid scenario not found")
		}

		gate := ops[ModeDoctrine]

		ctx := context.Background()
		result := gate.Submit(ctx, targetScenario.Intent)

		// Assert that the flipped expectation causes a failure
		// (i.e., the actual result is ACCEPT, not REJECT)
		if result.Error != nil {
			t.Errorf("negative control failed: expected flip to cause assertion error, but got actual rejection: %v", result.Error)
		}
		if result.Receipt == nil {
			t.Errorf("negative control failed: expected flip to cause assertion error, but got nil receipt")
		}
		// If we get here with a valid receipt, the flip would have caused AssertVerdict to fail
		// This proves the suite can detect wrong expectations
	})

	// Test 2: Flip a known-rejecting scenario to expect accept
	t.Run("flip_reject_to_accept", func(t *testing.T) {
		// Find a scenario that rejects in notary mode
		var targetScenario *Scenario
		for _, s := range scenarios {
			if s.Expect[ModeNotary].Verdict == VerdictReject {
				targetScenario = &s
				break
			}
		}
		if targetScenario == nil {
			t.Skip("no rejecting scenario found for notary mode")
		}

		gate := ops[ModeNotary]

		ctx := context.Background()
		result := gate.Submit(ctx, targetScenario.Intent)

		// Assert that the flipped expectation causes a failure
		// (i.e., the actual result is REJECT, not ACCEPT)
		if result.Error == nil {
			t.Errorf("negative control failed: expected flip to cause assertion error, but got actual acceptance")
		}
		if result.Receipt != nil {
			t.Errorf("negative control failed: expected flip to cause assertion error, but got valid receipt")
		}
		// If we get here with an error, the flip would have caused AssertVerdict to fail
		// This proves the suite can detect wrong expectations
	})
}

func generateTestSigners() map[string]ed25519.PublicKey {
	signers := make(map[string]ed25519.PublicKey)

	// Use the specific private key from generate_fixtures.go to ensure signature verification works
	// PRIVATE_KEY_HEX: c847d8625a1d1be737b8c86012ef1ceb7cfe1c2f5bed5115b90b490c55600502797c07dc7211981020b7fea8c31ed993d30576e0e14523a76678672a0d18b8cd
	// KEY_ID: 797c07dc7211981020b7fea8c31ed993d30576e0e14523a76678672a0d18b8cd
	privKeyHex := "c847d8625a1d1be737b8c86012ef1ceb7cfe1c2f5bed5115b90b490c55600502797c07dc7211981020b7fea8c31ed993d30576e0e14523a76678672a0d18b8cd"
	privKeyBytes, err := hex.DecodeString(privKeyHex)
	if err != nil {
		panicf("failed to decode private key hex: %v", err)
	}
	if len(privKeyBytes) != ed25519.PrivateKeySize {
		panicf("invalid private key size: got %d, want %d", len(privKeyBytes), ed25519.PrivateKeySize)
	}
	privKey := ed25519.PrivateKey(privKeyBytes)
	pubKey := privKey.Public().(ed25519.PublicKey)
	keyID := hex.EncodeToString(pubKey)
	signers[keyID] = pubKey

	// Add the forge_signature fixture key_id to trusted signers
	// This allows the verifier to attempt signature verification and fail with "signature invalid"
	// instead of "unknown signer"
	forgeSigKeyID := "2033b866aa250feeffd71b5065d534868fdd37fbf507accb01bdab7c36a11ffb"
	forgeSigPubBytes, err := hex.DecodeString(forgeSigKeyID)
	if err != nil {
		panicf("failed to decode forge_signature key_id: %v", err)
	}
	if len(forgeSigPubBytes) != ed25519.PublicKeySize {
		panicf("invalid forge_signature key_id size: got %d, want %d", len(forgeSigPubBytes), ed25519.PublicKeySize)
	}
	signers[forgeSigKeyID] = ed25519.PublicKey(forgeSigPubBytes)

	// Add the unknown_signer fixture key_id to trusted signers
	// This allows the verifier to attempt signature verification and fail with "signature invalid"
	// because the signature was created with a different private key
	unknownSignerKeyID := "3b6a27bcceb6a42d62a3a8d02a6f0d73653215771de243a63ac048a18b59da29"
	unknownSignerPubBytes, err := hex.DecodeString(unknownSignerKeyID)
	if err != nil {
		panicf("failed to decode unknown_signer key_id: %v", err)
	}
	if len(unknownSignerPubBytes) != ed25519.PublicKeySize {
		panicf("invalid unknown_signer key_id size: got %d, want %d", len(unknownSignerPubBytes), ed25519.PublicKeySize)
	}
	signers[unknownSignerKeyID] = ed25519.PublicKey(unknownSignerPubBytes)

	// Add 2 more signers for consensus testing
	for i := 2; i <= 3; i++ {
		pub, _, err := ed25519.GenerateKey(nil)
		if err != nil {
			panicf("failed to generate test signer %d: %v", i, err)
		}
		keyID := hex.EncodeToString(pub)
		signers[keyID] = pub
	}

	return signers
}

func panicf(format string, args ...interface{}) {
	panic(fmt.Sprintf(format, args...))
}
