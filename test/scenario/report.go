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

package scenario

import (
	"testing"

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// Report prints a detailed trace of the scenario execution under -v.
// This is the "theater" - the same test that gates the pipeline is the demo.
func Report(t *testing.T, s Scenario, mode Mode, result Result) {
	t.Logf("=== Scenario: %s (%s mode) ===", s.Name, mode)
	t.Logf("Vertical: %s", s.Vertical)
	t.Logf("Narrative: %s", s.Narrative)
	t.Logf("Evidence: L2=%v (key=%s), L3=%v, signer=%s",
		s.Evidence.L2SignaturePresent,
		s.Evidence.L2KeyID,
		s.Evidence.L3ProofPresent,
		s.Evidence.SignerID)

	if result.Error != nil {
		t.Logf("Result: REJECTED - %s", result.Error)
		return
	}

	if result.Receipt == nil {
		t.Logf("Result: ERROR - nil receipt without error")
		return
	}

	t.Logf("Result: ACCEPTED")
	t.Logf("Receipt:")
	t.Logf("  Transaction ID: %s", result.Receipt.TransactionId)
	t.Logf("  Transaction Hash: %s", result.Receipt.TransactionHash)
	t.Logf("  Status: %s", result.Receipt.Status)
	t.Logf("  Result Summary: %s", result.Receipt.ResultSummary)
	t.Logf("  State Root Before: %s", result.Receipt.StateRootBefore)
	t.Logf("  State Root After: %s", result.Receipt.StateRootAfter)
	t.Logf("  Signer Key ID: %s", result.Receipt.SignerKeyId)
	t.Logf("  Signature: %s", result.Receipt.Signature)
	t.Logf("  Gateway Signed: %v", result.Receipt.GatewaySigned)
	t.Logf("  L2 Valid: %v", result.Receipt.L2Valid)
	t.Logf("  L3 Valid: %v", result.Receipt.L3Valid)
	t.Logf("  Executed At: %d", result.Receipt.ExecutedAtUnixMs)
}

// AssertVerdict checks that the result matches the expected verdict.
func AssertVerdict(t *testing.T, result Result, expected Outcome) {
	if expected.Verdict == VerdictAccept {
		if result.Error != nil {
			t.Errorf("expected ACCEPT but got error: %v", result.Error)
		}
		if result.Receipt == nil {
			t.Errorf("expected ACCEPT but got nil receipt")
		}
	} else {
		if result.Error == nil {
			t.Errorf("expected REJECT but got ACCEPT (receipt returned)")
		}
	}
}

// AssertReason checks that the rejection reason matches the expected reason.
func AssertReason(t *testing.T, result Result, expected Outcome) {
	if expected.Verdict == VerdictReject && result.Error != nil {
		errMsg := result.Error.Error()
		if expected.RejectReason != "" && !containsSubstring(errMsg, expected.RejectReason) {
			t.Errorf("expected rejection reason to contain %q, got %q", expected.RejectReason, errMsg)
		}
	}
}

// AssertL2L3 checks that L2/L3 validity matches expectations.
func AssertL2L3(t *testing.T, result Result, expected Outcome) {
	if result.Receipt != nil {
		if result.Receipt.L2Valid != expected.L2Valid {
			t.Errorf("expected L2Valid=%v, got %v", expected.L2Valid, result.Receipt.L2Valid)
		}
		if result.Receipt.L3Valid != expected.L3Valid {
			t.Errorf("expected L3Valid=%v, got %v", expected.L3Valid, result.Receipt.L3Valid)
		}
	}
}

// GoldenDiff compares the receipt against the golden file.
// Use -update flag to refresh golden files.
func GoldenDiff(t *testing.T, s Scenario, mode Mode, receipt *operatorv1.ActionReceipt) {
	if receipt == nil {
		return
	}

	// TODO: Implement golden file diffing
	// This will serialize the receipt to JSON and compare against test/scenario/golden/{scenario}_{mode}.json
	t.Logf("Golden diff not yet implemented for %s/%s", s.Name, mode)
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
