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

package governance

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// hashVector is a single test case from the shared hash_vectors.json file.
type hashVector struct {
	Name             string          `json:"name"`
	ActionType       string          `json:"action_type"`
	TargetResource   string          `json:"target_resource"`
	PayloadB64       string          `json:"payload_b64"`
	StateMerkleRoot  string          `json:"state_merkle_root"`
	Nonce            string          `json:"nonce"`
	ExpiresAt        string          `json:"expires_at"`
	IntentData       map[string]any  `json:"intent_data"`
	RequestorUserID  *string         `json:"requestor_user_id"`
	ActingAppID      *string         `json:"acting_app_id"`
	ExpectedHash     string          `json:"expected_hash"`
}

type hashVectorsFile struct {
	Description string       `json:"description"`
	Vectors     []hashVector `json:"vectors"`
}

func loadHashVectors(t *testing.T) []hashVector {
	t.Helper()
	// Resolve path from the governance package directory to the protocol conformance directory.
	// internal/governance/ -> ../../protocol/conformance/hash_vectors.json
	path := filepath.Join("..", "..", "protocol", "conformance", "hash_vectors.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read hash_vectors.json: %v", err)
	}
	var file hashVectorsFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("Failed to parse hash_vectors.json: %v", err)
	}
	if len(file.Vectors) == 0 {
		t.Fatal("hash_vectors.json contains no vectors")
	}
	return file.Vectors
}

func TestHashParity_VectorFile(t *testing.T) {
	vectors := loadHashVectors(t)

	for _, v := range vectors {
		t.Run(v.Name, func(t *testing.T) {
			env := buildEnvelopeFromVector(t, v)
			hash, err := GenerateMessageID(env)
			if err != nil {
				t.Fatalf("GenerateMessageID failed: %v", err)
			}
			if hash != v.ExpectedHash {
				t.Errorf("Hash mismatch for vector %q:\n  expected: %s\n  got:      %s",
					v.Name, v.ExpectedHash, hash)
			}
		})
	}
}

func TestHashParity_UnknownTypeReturnsError(t *testing.T) {
	// canonicalizeValue should reject unsupported types rather than silently
	// falling back to JSON, which would produce mismatched hashes across
	// Go and Python.
	_, err := canonicalizeValue(complex128(1 + 2i))
	if err == nil {
		t.Error("expected error for unsupported type complex128, got nil")
	}
}

func TestHashParity_TimestampNormalization(t *testing.T) {
	// Verify that timestamps with and without fractional seconds produce the same hash,
	// matching Go's timesvc.FormatTimestamp normalization.
	vectors := loadHashVectors(t)
	var noFrac, withFrac *hashVector
	for i := range vectors {
		if vectors[i].Name == "timestamp_no_fractional" {
			noFrac = &vectors[i]
		}
		if vectors[i].Name == "timestamp_with_fractional" {
			withFrac = &vectors[i]
		}
	}
	if noFrac == nil || withFrac == nil {
		t.Fatal("Missing timestamp test vectors")
	}

	env1 := buildEnvelopeFromVector(t, *noFrac)
	env2 := buildEnvelopeFromVector(t, *withFrac)

	hash1, err := GenerateMessageID(env1)
	if err != nil {
		t.Fatalf("hash1 failed: %v", err)
	}
	hash2, err := GenerateMessageID(env2)
	if err != nil {
		t.Fatalf("hash2 failed: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("Timestamp normalization failed: %s != %s", hash1, hash2)
	}
}

func buildEnvelopeFromVector(t *testing.T, v hashVector) *GovernanceEnvelope {
	t.Helper()

	// Decode payload from base64 to raw bytes (Go takes []byte, re-encodes internally).
	var payload []byte
	if v.PayloadB64 != "" {
		var err error
		payload, err = base64.StdEncoding.DecodeString(v.PayloadB64)
		if err != nil {
			t.Fatalf("Failed to decode payload_b64 for vector %q: %v", v.Name, err)
		}
	}

	// Parse expires_at using RFC3339Nano (same parser as timesvc.ParseTimestamp).
	expiresAt, err := time.Parse(time.RFC3339Nano, v.ExpiresAt)
	if err != nil {
		t.Fatalf("Failed to parse expires_at %q for vector %q: %v", v.ExpiresAt, v.Name, err)
	}

	// Convert intent_data to structpb.Struct.
	var intentData *structpb.Struct
	if len(v.IntentData) > 0 {
		intentData, err = structpb.NewStruct(v.IntentData)
		if err != nil {
			t.Fatalf("Failed to create structpb.Struct for vector %q: %v", v.Name, err)
		}
	}

	env := &GovernanceEnvelope{
		ActionType:      v.ActionType,
		TargetResource:  v.TargetResource,
		Payload:         payload,
		StateMerkleRoot: v.StateMerkleRoot,
		Nonce:           v.Nonce,
		ExpiresAt:       timestamppb.New(expiresAt),
		IntentData:      intentData,
	}

	if v.RequestorUserID != nil && *v.RequestorUserID != "" {
		env.RequestorUserId = *v.RequestorUserID
	}
	if v.ActingAppID != nil && *v.ActingAppID != "" {
		env.ActingAppId = *v.ActingAppID
	}

	return env
}
