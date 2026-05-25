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
	"embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/require"
)

//go:embed fixtures
var fixturesFS embed.FS

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

// Outcome describes the expected result for a scenario in a given mode.
type Outcome struct {
	Verdict      Verdict `json:"verdict"`
	RejectReason string  `json:"reject_reason,omitempty"`
	L2Status     int32   `json:"l2_status"` // L2Status enum: 0=unspecified, 1=not_required, 2=required_valid, 3=required_failed
	L3Status     int32   `json:"l3_status"` // L3Status enum: 0=unspecified, 1=not_required, 2=required_valid, 3=required_failed
}

// Evidence describes which governance proofs are present in the envelope.
type Evidence struct {
	L2SignaturePresent bool   `json:"l2_signature_present"`
	L2KeyID            string `json:"l2_key_id,omitempty"`
	L3ProofPresent     bool   `json:"l3_proof_present"`
	SignerID           string `json:"signer_id,omitempty"`
}

// RawIntent is the raw envelope payload (JSON bytes).
type RawIntent []byte

// Scenario is a pure data structure describing a test case.
type Scenario struct {
	Name      string           `json:"name"`
	Vertical  string           `json:"vertical"`
	Narrative string           `json:"narrative"`
	Intent    json.RawMessage  `json:"intent"`
	Evidence  Evidence         `json:"evidence"`
	Expect    map[Mode]Outcome `json:"expect"`
}

// LoadFixtures loads all scenario fixtures from the embedded filesystem.
func LoadFixtures() ([]Scenario, error) {
	var scenarios []Scenario

	return loadFixturesRecursive("fixtures", scenarios)
}

func loadFixturesRecursive(dir string, scenarios []Scenario) ([]Scenario, error) {
	entries, err := fixturesFS.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read fixtures directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())

		if entry.IsDir() {
			var err error
			scenarios, err = loadFixturesRecursive(path, scenarios)
			if err != nil {
				return nil, err
			}
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := fixturesFS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read fixture %s: %w", path, err)
		}

		var s Scenario
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("failed to unmarshal fixture %s: %w", path, err)
		}

		scenarios = append(scenarios, s)
	}

	return scenarios, nil
}

// AssertPersistedReceipt verifies that receipts are persisted to the database correctly.
// For accepting scenarios, receipts MUST be persisted to console_audit.
// For rejecting scenarios, receipts MUST NOT be persisted.
func AssertPersistedReceipt(t *testing.T, db *sqliteutil.DB, receipt *operatorv1.ActionReceipt, expected Outcome) {
	if expected.Verdict == VerdictReject {
		// Rejected transactions should NOT persist receipts
		doc, err := QueryReceipt(db, receipt.TransactionId)
		require.NoError(t, err, "QueryReceipt should not error for rejected transaction")
		require.Nil(t, doc, "receipt should not be persisted for rejected transaction")
		return
	}

	// Accepted transactions MUST persist receipts
	doc, err := QueryReceipt(db, receipt.TransactionId)
	require.NoError(t, err, "QueryReceipt should not error for accepted transaction")
	require.NotNil(t, doc, "receipt must be persisted for accepted transaction")

	// The receipt is stored as the entire document data map
	// Marshal the map back to JSON, then unmarshal to ActionReceiptRecord
	dataJSON, err := json.Marshal(doc.Data)
	require.NoError(t, err, "failed to marshal document data")

	var record models.ActionReceiptRecord
	err = json.Unmarshal(dataJSON, &record)
	require.NoError(t, err, "persisted receipt should unmarshal to ActionReceiptRecord")

	require.Equal(t, receipt.TransactionId, record.TransactionID, "transaction_id must match")
	require.Equal(t, receipt.TransactionHash, record.TransactionHash, "transaction_hash must match")
	require.Equal(t, receipt.Status, record.Status, "status must match")
	require.Equal(t, receipt.Signature, record.Signature, "signature must match")
	require.Equal(t, receipt.L2Status == operatorv1.L2Status_L2_STATUS_REQUIRED_VALID, record.L2Valid, "l2_valid must match l2_status")
	require.Equal(t, receipt.L3Status == operatorv1.L3Status_L3_STATUS_REQUIRED_VALID, record.L3Valid, "l3_valid must match l3_status")
}
