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
	"fmt"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/services/system"
)

var (
	// ops holds the OperatorGate instances for each mode
	ops map[Mode]*OperatorGate

	// testStateRoot is the fixed state root for deterministic testing
	testStateRoot = "abc123def456"

	// testSigners holds the trusted L2 signers for testing
	testSigners map[string]ed25519.PublicKey
)

func TestMain(m *testing.M) {
	// Generate test signers
	testSigners = generateTestSigners()

	// Create operator gates for each mode
	ops = make(map[Mode]*OperatorGate)
	modes := []Mode{ModeDoctrine, ModeConsensus, ModeNotary}

	for _, mode := range modes {
		// Use a fixed clock for deterministic testing
		fixedTime := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
		clock := system.NewFixedClock(fixedTime)

		gate, err := NewOperatorGate(mode, clock, testStateRoot, testSigners)
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
		for _, mode := range []Mode{ModeDoctrine, ModeConsensus, ModeNotary} {
			t.Run(s.Name+"/"+mode.String(), func(t *testing.T) {
				gate, ok := ops[mode]
				if !ok {
					t.Fatalf("operator gate not initialized for mode %s", mode)
				}

				expected, ok := s.Expect[mode]
				if !ok {
					t.Fatalf("no expected outcome defined for mode %s in scenario %s", mode, s.Name)
				}

				ctx := context.Background()
				result := gate.Submit(ctx, s.Intent)

				// Assert verdict
				AssertVerdict(t, result, expected)

				// Assert rejection reason if applicable
				AssertReason(t, result, expected)

				// Assert L2/L3 validity
				AssertL2L3(t, result, expected)

				// Golden diff
				GoldenDiff(t, s, mode, result.Receipt)

				// Report trace under -v
				Report(t, s, mode, result)
			})
		}
	}
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
