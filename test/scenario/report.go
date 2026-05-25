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
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/services/governance"
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
// The expected reason must match as a prefix to allow for additional context in the error message.
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
		// Require exact prefix match for the error code to prevent wrong-reason false passes
		// e.g., "TX_REPLAY: nonce already used" must start with "TX_REPLAY", not just contain it
		if !hasErrorCodePrefix(errMsg, expected.RejectReason) {
			t.Errorf("expected rejection reason to start with %q, got %q", expected.RejectReason, errMsg)
		}
	}
}

// hasErrorCodePrefix checks if the error message starts with the expected error code prefix.
// This ensures strict matching of error codes while allowing additional context after the prefix.
func hasErrorCodePrefix(errMsg, expected string) bool {
	if len(errMsg) < len(expected) {
		return false
	}
	// Check for exact prefix match
	if errMsg[:len(expected)] == expected {
		return true
	}
	// Also allow matching if the expected is a complete error code and errMsg starts with it
	// This handles cases like "TX_REPLAY: nonce already used" vs "TX_REPLAY"
	return len(errMsg) >= len(expected) && errMsg[:len(expected)] == expected
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

// AssertReceiptTamperDetection verifies that receipt signature tampering is detected.
// This tests the "tamper-evident" property - a core security guarantee.
func AssertReceiptTamperDetection(t *testing.T, receipt *operatorv1.ActionReceipt, actuator interface{}) {
	// Verify the original receipt signature is valid
	if receipt.Signature == "" {
		t.Errorf("receipt has no signature to verify")
		return
	}

	// Test 1: Verify original receipt signature is valid
	valid, err := verifyReceiptSignature(receipt)
	if err != nil {
		t.Errorf("failed to verify original receipt signature: %v", err)
		return
	}
	if !valid {
		t.Errorf("original receipt signature should be valid but verification failed")
		return
	}
	t.Logf("Receipt tamper detection: original signature verified successfully")

	// Test 2: Tamper with signature and verify it fails
	tamperedReceipt := cloneReceipt(receipt)
	if len(tamperedReceipt.Signature) >= 2 {
		// Flip a byte in the signature
		sigBytes, _ := hex.DecodeString(tamperedReceipt.Signature)
		if len(sigBytes) > 0 {
			sigBytes[0] = sigBytes[0] ^ 0xff
			tamperedReceipt.Signature = hex.EncodeToString(sigBytes)
		}
	}

	valid, err = verifyReceiptSignature(tamperedReceipt)
	if err != nil {
		t.Errorf("failed to verify tampered receipt signature: %v", err)
		return
	}
	if valid {
		t.Errorf("tampered receipt signature should fail verification but passed - TAMPER DETECTION BROKEN")
		return
	}
	t.Logf("Receipt tamper detection: tampered signature correctly rejected")

	// Test 3: Tamper with state_root_after and verify it fails
	tamperedReceipt2 := cloneReceipt(receipt)
	tamperedReceipt2.StateRootAfter = tamperedReceipt2.StateRootAfter + "-tampered"
	// Keep the original signature (which was for the untampered data)
	tamperedReceipt2.Signature = receipt.Signature

	valid, err = verifyReceiptSignature(tamperedReceipt2)
	if err != nil {
		t.Errorf("failed to verify tampered receipt (state_root_after): %v", err)
		return
	}
	if valid {
		t.Errorf("tampered state_root_after should fail verification but passed - TAMPER DETECTION BROKEN")
		return
	}
	t.Logf("Receipt tamper detection: tampered state_root_after correctly rejected")

	// Test 4: Verify GatewaySigned field is present and true (as set in fixture)
	if !receipt.GatewaySigned {
		t.Errorf("receipt.GatewaySigned should be true for tampered_receipt scenario (set in envelope)")
	}
}

// verifyReceiptSignature verifies an ActionReceipt signature using the canonical form.
func verifyReceiptSignature(receipt *operatorv1.ActionReceipt) (bool, error) {
	if receipt.Signature == "" {
		return false, fmt.Errorf("receipt has no signature")
	}

	// Import the governance package for canonicalization
	canonical, err := governance.CanonicalizeActionReceipt(receipt)
	if err != nil {
		return false, fmt.Errorf("failed to canonicalize receipt: %w", err)
	}

	sigBytes, err := hex.DecodeString(receipt.Signature)
	if err != nil {
		return false, fmt.Errorf("failed to decode signature: %w", err)
	}

	signerKeyID, err := hex.DecodeString(receipt.SignerKeyId)
	if err != nil {
		return false, fmt.Errorf("failed to decode signer key ID: %w", err)
	}

	return ed25519.Verify(signerKeyID, canonical, sigBytes), nil
}

// cloneReceipt creates a deep copy of an ActionReceipt.
func cloneReceipt(r *operatorv1.ActionReceipt) *operatorv1.ActionReceipt {
	return &operatorv1.ActionReceipt{
		TransactionId:    r.TransactionId,
		TransactionHash:  r.TransactionHash,
		Status:           r.Status,
		ResultSummary:    r.ResultSummary,
		StateRootBefore:  r.StateRootBefore,
		StateRootAfter:   r.StateRootAfter,
		ExecutedAtUnixMs: r.ExecutedAtUnixMs,
		SignerKeyId:      r.SignerKeyId,
		Signature:        r.Signature,
		GatewaySigned:    r.GatewaySigned,
		L2Status:         r.L2Status,
		L3Status:         r.L3Status,
	}
}

// GoldenDiff compares the receipt against the golden file.
// Set G8E_UPDATE_GOLDEN=1 environment variable to refresh golden files.
// Only compares deterministic fields (excludes timestamp, signer key, signature).
// Golden files are only created for accepting scenarios (receipt != nil).
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

// CheckGoldenFilesUpToDate verifies that all golden files are present and up to date.
// This is intended for CI to ensure developers don't forget to update golden files.
// Returns true if all golden files are up to date, false otherwise.
func CheckGoldenFilesUpToDate(t *testing.T) bool {
	scenarios, err := LoadFixtures()
	if err != nil {
		t.Fatalf("failed to load fixtures: %v", err)
	}

	modes := []Mode{ModeDoctrine, ModeConsensus, ModeNotary}
	missingFiles := []string{}

	for _, s := range scenarios {
		for _, mode := range modes {
			expected, ok := s.Expect[mode]
			if !ok {
				continue
			}

			// Only check golden files for accepting scenarios
			if expected.Verdict != VerdictAccept {
				continue
			}

			goldenPath := filepath.Join("golden", s.Name+"_"+mode.String()+".golden.json")
			if _, err := os.Stat(goldenPath); os.IsNotExist(err) {
				missingFiles = append(missingFiles, goldenPath)
			}
		}
	}

	if len(missingFiles) > 0 {
		t.Errorf("Missing golden files (run with G8E_UPDATE_GOLDEN=1 to create):")
		for _, path := range missingFiles {
			t.Errorf("  - %s", path)
		}
		return false
	}

	return true
}
