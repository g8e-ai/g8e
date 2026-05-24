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
		clock := NewFixedClock(fixedTime)

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

	// Generate 3 test tribunal signers
	for i := 1; i <= 3; i++ {
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
