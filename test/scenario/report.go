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
	"os"
	"path/filepath"
	"testing"

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/encoding/protojson"
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
	t.Logf("  L2 Status: %v", result.Receipt.L2Status)
	t.Logf("  L3 Status: %v", result.Receipt.L3Status)
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

// AssertReason checks that the rejection reason matches the expected reason exactly.
// This prevents false passes where a transaction rejects for the wrong reason.
func AssertReason(t *testing.T, result Result, expected Outcome) {
	if expected.Verdict == VerdictReject {
		if result.Error == nil {
			t.Errorf("expected REJECT with reason %q but got ACCEPT (no error)", expected.RejectReason)
			return
		}
		errMsg := result.Error.Error()
		if expected.RejectReason == "" {
			t.Errorf("expected REJECT but no reject_reason specified in fixture")
			return
		}
		if !containsSubstring(errMsg, expected.RejectReason) {
			t.Errorf("expected rejection reason to contain %q, got %q", expected.RejectReason, errMsg)
		}
	}
}

// AssertL2L3 checks that L2/L3 validity matches expectations.
func AssertL2L3(t *testing.T, result Result, expected Outcome) {
	if result.Receipt != nil {
		if int32(result.Receipt.L2Status) != expected.L2Status {
			t.Errorf("expected L2Status=%v, got %v", expected.L2Status, result.Receipt.L2Status)
		}
		if int32(result.Receipt.L3Status) != expected.L3Status {
			t.Errorf("expected L3Status=%v, got %v", expected.L3Status, result.Receipt.L3Status)
		}
	}
}

// GoldenDiff compares the receipt against the golden file.
// Set G8E_UPDATE_GOLDEN=1 environment variable to refresh golden files.
// Only compares deterministic fields (excludes timestamp, signer key, signature).
func GoldenDiff(t *testing.T, s Scenario, mode Mode, receipt *operatorv1.ActionReceipt) {
	if receipt == nil {
		return
	}

	// Create a deterministic receipt for comparison
	deterministicReceipt := &operatorv1.ActionReceipt{
		Status:          receipt.Status,
		ResultSummary:   receipt.ResultSummary,
		StateRootBefore: receipt.StateRootBefore,
		StateRootAfter:  receipt.StateRootAfter,
		L2Status:        receipt.L2Status,
		L3Status:        receipt.L3Status,
		// Exclude: TransactionId, TransactionHash, ExecutedAtUnixMs, SignerKeyId, Signature, GatewaySigned
	}

	// Serialize deterministic receipt to JSON for comparison
	marshaler := &protojson.MarshalOptions{Indent: "  "}
	receiptJSON, err := marshaler.Marshal(deterministicReceipt)
	if err != nil {
		t.Fatalf("failed to marshal receipt to JSON: %v", err)
	}

	// Golden file path
	goldenPath := filepath.Join("golden", s.Name+"_"+mode.String()+".golden.json")

	// Check for G8E_UPDATE_GOLDEN environment variable
	update := os.Getenv("G8E_UPDATE_GOLDEN") == "1"

	if update {
		// Write the golden file
		if err := os.WriteFile(goldenPath, receiptJSON, 0644); err != nil {
			t.Fatalf("failed to write golden file %s: %v", goldenPath, err)
		}
		t.Logf("Updated golden file: %s", goldenPath)
		return
	}

	// Read the golden file
	goldenJSON, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v (run with G8E_UPDATE_GOLDEN=1 to create)", goldenPath, err)
	}

	// Compare
	if string(receiptJSON) != string(goldenJSON) {
		t.Errorf("Golden mismatch for %s/%s\nGot:\n%s\n\nWant:\n%s", s.Name, mode, receiptJSON, goldenJSON)
	}
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
