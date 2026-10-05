// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"encoding/json"
	"testing"
)

func TestParseReceipts(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		wantLen  int
		wantZero bool
	}{
		{
			name:     "wrapped receipts",
			body:     []byte(`{"receipts":[{"transaction_id":"tx1"},{"transaction_id":"tx2"}]}`),
			wantLen:  2,
			wantZero: false,
		},
		{
			name:     "bare array",
			body:     []byte(`[{"transaction_id":"tx1"},{"transaction_id":"tx2"}]`),
			wantLen:  2,
			wantZero: false,
		},
		{
			name:     "empty wrapped array",
			body:     []byte(`{"receipts":[]}`),
			wantLen:  0,
			wantZero: false,
		},
		{
			name:     "empty bare array",
			body:     []byte(`[]`),
			wantLen:  0,
			wantZero: false,
		},
		{
			name:     "invalid JSON",
			body:     []byte(`invalid json`),
			wantLen:  0,
			wantZero: true,
		},
		{
			name:     "empty body",
			body:     []byte{},
			wantLen:  0,
			wantZero: true,
		},
		{
			name:     "nil body",
			body:     nil,
			wantLen:  0,
			wantZero: true,
		},
		{
			name:     "wrapped with empty receipts field",
			body:     []byte(`{"other":"data"}`),
			wantLen:  0,
			wantZero: false,
		},
		{
			name:     "single receipt wrapped",
			body:     []byte(`{"receipts":[{"transaction_id":"tx1","status":"completed"}]}`),
			wantLen:  1,
			wantZero: false,
		},
		{
			name:     "malformed receipt object",
			body:     []byte(`[{"transaction_id":"tx1"},{"invalid}]`),
			wantLen:  0,
			wantZero: true, // invalid JSON object
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseReceipts(tt.body)

			if tt.wantZero && result != nil {
				t.Errorf("parseReceipts() should return nil for invalid input, got %v", result)
			}
			if !tt.wantZero && result == nil {
				t.Error("parseReceipts() returned nil unexpectedly")
			}
			if !tt.wantZero && len(result) != tt.wantLen {
				t.Errorf("parseReceipts() length = %d, want %d", len(result), tt.wantLen)
			}
		})
	}
}

func TestParseReceipts_RawField(t *testing.T) {
	body := []byte(`{"receipts":[{"transaction_id":"tx1","status":"completed"}]}`)
	result := parseReceipts(body)

	if len(result) != 1 {
		t.Fatalf("expected 1 receipt, got %d", len(result))
	}

	if result[0].Raw == nil {
		t.Error("parseReceipts() should set Raw field")
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(result[0].Raw, &decoded); err != nil {
		t.Errorf("failed to unmarshal Raw field: %v", err)
	}

	if decoded["transaction_id"] != "tx1" {
		t.Errorf("Raw field does not contain expected data")
	}
}

func TestReceipt(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
	}{
		{
			name: "minimal receipt",
			data: map[string]interface{}{
				"transaction_id":    "tx-123",
				"transaction_hash":  "hash-abc",
				"action_type":       "EXECUTE_BASH",
				"target_resource":   "localhost",
				"status":            "completed",
				"state_root_before": "root-before",
				"state_root_after":  "root-after",
				"signature":         "sig-def",
			},
		},
		{
			name: "receipt with all fields",
			data: map[string]interface{}{
				"transaction_id":    "tx-456",
				"transaction_hash":  "hash-xyz",
				"action_type":       "MCP_CALL",
				"target_resource":   "remote-host",
				"status":            "pending",
				"state_root_before": "root-1",
				"state_root_after":  "root-2",
				"signature":         "sig-xyz",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.data)
			if err != nil {
				t.Fatalf("failed to marshal test data: %v", err)
			}

			var rec Receipt
			if err := json.Unmarshal(data, &rec); err != nil {
				t.Fatalf("failed to unmarshal Receipt: %v", err)
			}

			if rec.TransactionID != tt.data["transaction_id"] {
				t.Errorf("TransactionID mismatch")
			}
			if rec.TransactionHash != tt.data["transaction_hash"] {
				t.Errorf("TransactionHash mismatch")
			}
			if rec.ActionType != tt.data["action_type"] {
				t.Errorf("ActionType mismatch")
			}
			if rec.TargetResource != tt.data["target_resource"] {
				t.Errorf("TargetResource mismatch")
			}
			if rec.Status != tt.data["status"] {
				t.Errorf("Status mismatch")
			}
			if rec.StateRootBefore != tt.data["state_root_before"] {
				t.Errorf("StateRootBefore mismatch")
			}
			if rec.StateRootAfter != tt.data["state_root_after"] {
				t.Errorf("StateRootAfter mismatch")
			}
			if rec.Signature != tt.data["signature"] {
				t.Errorf("Signature mismatch")
			}
		})
	}
}

func TestReceipt_RawField(t *testing.T) {
	data := map[string]interface{}{
		"transaction_id": "tx-123",
		"status":         "completed",
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var rec Receipt
	if err := json.Unmarshal(jsonData, &rec); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	rec.Raw = jsonData

	if rec.Raw == nil {
		t.Error("Raw field should be set")
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(rec.Raw, &decoded); err != nil {
		t.Errorf("failed to unmarshal Raw field: %v", err)
	}

	if decoded["transaction_id"] != "tx-123" {
		t.Error("Raw field should contain original data")
	}
}

func TestReceipt_MarshalJSON(t *testing.T) {
	rec := Receipt{
		TransactionID:   "tx-123",
		TransactionHash: "hash-abc",
		ActionType:      "EXECUTE_BASH",
		TargetResource:  "localhost",
		Status:          "completed",
		StateRootBefore: "root-before",
		StateRootAfter:  "root-after",
		Signature:       "sig-def",
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("failed to marshal Receipt: %v", err)
	}

	// Raw field should not be included in JSON output
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, exists := decoded["raw"]; exists {
		t.Error("Raw field should not be marshaled to JSON")
	}

	if decoded["transaction_id"] != "tx-123" {
		t.Error("TransactionID should be in JSON output")
	}
}
