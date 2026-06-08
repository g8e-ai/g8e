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
	"context"
	"testing"

	"github.com/g8e-ai/g8e/internal/emulator/client"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// Mode represents a governance posture mode for testing.
type Mode string

const (
	ModeDoctrine  Mode = "doctrine"
	ModeConsensus Mode = "consensus"
	ModeNotary    Mode = "notary"
)

func (m Mode) String() string {
	return string(m)
}

// Verdict represents the expected outcome of a scenario.
type Verdict string

const (
	VerdictAccept Verdict = "accept"
	VerdictReject Verdict = "reject"
)

// AssertPersistedReceipt verifies that receipts are persisted via the API.
// For accepting scenarios, receipts MUST be queryable via the API.
// For rejecting scenarios, receipts MUST NOT be queryable.
func AssertPersistedReceipt(t *testing.T, client *client.Client, receipt *operatorv1.ActionReceipt, expectedVerdict Verdict, transactionID string) {
	t.Helper()

	ctx := context.Background()

	if expectedVerdict == VerdictAccept {
		if receipt == nil {
			t.Fatal("expected receipt for accepted transaction, got nil")
		}
		persisted, _, err := client.GetReceipt(ctx, receipt.TransactionId)
		if err != nil {
			t.Fatalf("failed to query receipt: %v", err)
		}
		if persisted == nil {
			t.Fatalf("receipt not persisted for accepted transaction %s", receipt.TransactionId)
		}
		if persisted.TransactionID != receipt.TransactionId {
			t.Fatalf("receipt transaction_id mismatch: persisted=%s, expected=%s", persisted.TransactionID, receipt.TransactionId)
		}
		if persisted.TransactionHash != receipt.TransactionHash {
			t.Fatalf("receipt transaction_hash mismatch: persisted=%s, expected=%s", persisted.TransactionHash, receipt.TransactionHash)
		}
	} else {
		if receipt != nil {
			t.Fatal("expected nil receipt for rejected transaction, got non-nil")
		}
		if transactionID == "" {
			t.Fatal("transactionID required for negative control verification")
		}
		persisted, _, err := client.GetReceipt(ctx, transactionID)
		if err != nil {
			t.Fatalf("failed to query receipt for negative control: %v", err)
		}
		if persisted != nil {
			t.Fatalf("receipt should not be persisted for rejected transaction %s, but found receipt", transactionID)
		}
	}
}
