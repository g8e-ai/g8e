// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// TestOperatorDocumentRoundTripFullyPopulated asserts that a fully populated
// OperatorDocument round-trips through JSON and compares as proto.Equal.
func TestOperatorDocumentRoundTripFullyPopulated(t *testing.T) {
	t.Parallel()

	// Use fixed timestamps for determinism
	createdAt := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC)
	startedAt := time.Date(2026, 1, 15, 11, 30, 0, 0, time.UTC)
	claimedAt := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	lastHeartbeatAt := time.Date(2026, 1, 15, 12, 30, 0, 0, time.UTC)

	heartbeatSnapshot := &operatorv1.HeartbeatResult{
		OperatorId:       "op-1",
		OperatorSessionId: "session-1",
		Timestamp:        "2026-01-15T12:30:00Z",
		Status:           "healthy",
		EventType:        "heartbeat_sent",
		SourceComponent:  "operator",
		SystemFingerprint: "fingerprint-123",
	}

	runtimeConfig := &operatorv1.OperatorRuntimeConfig{
		CloudMode:            false,
		CloudProvider:        "aws",
		LocalStorageEnabled:  true,
		NoGit:                false,
		LogLevel:             "info",
		HttpPort:             8080,
		Roles:                []string{string(constants.OperatorRoleData), string(constants.OperatorRoleInference)},
		LocalDir:             "/home/user/.g8e",
		Account:              "user@example.com",
		InferenceEnabled:     true,
		InferenceOllamaEndpoint: "http://localhost:11434",
		ProviderBoundaryObserverEnabled: true,
		ProvenanceOperatorEnabled:       true,
		ProvenanceOperatorModelStorageRoot: "/models",
		Platform:             "linux",
	}

	doc := &operatorv1.OperatorDocument{
		Id:               "op-1",
		UserId:           "user-1",
		OrganizationId:   "org-1",
		Component:        string(constants.ComponentNameG8EO),
		Name:             "test-operator",
		Status:           string(constants.OperatorStatusActive),
		OperatorSessionId: "session-1",
		BoundWebSessionId: "web-session-1",
		OperatorCert:     "-----BEGIN CERTIFICATE-----\nMIIC...\n-----END CERTIFICATE-----",
		OperatorCertSerial: "1234567890",
		SlotNumber:       1,
		IsSlot:           true,
		Claimed:          true,
		OperatorType:     string(constants.OperatorTypeRemote),
		SystemFingerprint: "fingerprint-123",
		CreatedAt:        timestamppb.New(createdAt),
		UpdatedAt:        timestamppb.New(updatedAt),
		StartedAt:        timestamppb.New(startedAt),
		ClaimedAt:        timestamppb.New(claimedAt),
		LastHeartbeatAt:  timestamppb.New(lastHeartbeatAt),
		OperatorRoles:    []string{string(constants.OperatorRoleData), string(constants.OperatorRoleInference)},
		LocalDir:         "/home/user/.g8e",
		Account:          "user@example.com",
		Port:             8080,
		StopReason:       "normal_shutdown",
		LatestHeartbeatSnapshot: heartbeatSnapshot,
		CurrentHostname:  "host-1",
		RuntimeConfig:    runtimeConfig,
		ConsumedByOperatorId: "parent-op-1",
		OperatorCertChain: "chain-pem",
		TerminationReason: "user_requested",
	}

	// Marshal to JSON
	data, err := MarshalOperatorDocument(doc)
	require.NoError(t, err)

	// Unmarshal back
	decoded, err := UnmarshalOperatorDocument(data)
	require.NoError(t, err)

	// Compare with proto.Equal
	assert.True(t, proto.Equal(doc, decoded), "fully populated document should round-trip as proto.Equal")
}

// TestOperatorDocumentJSONFormat asserts that marshaled JSON uses snake_case
// keys and RFC3339 strings for all five timestamp fields.
func TestOperatorDocumentJSONFormat(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 1, 15, 10, 30, 45, 123456000, time.UTC)
	updatedAt := time.Date(2026, 1, 15, 11, 0, 45, 654321000, time.UTC)
	startedAt := time.Date(2026, 1, 15, 11, 30, 45, 0, time.UTC)
	claimedAt := time.Date(2026, 1, 15, 12, 0, 45, 0, time.UTC)
	lastHeartbeatAt := time.Date(2026, 1, 15, 12, 30, 45, 0, time.UTC)

	doc := &operatorv1.OperatorDocument{
		Id:            "op-1",
		UserId:        "user-1",
		Component:     string(constants.ComponentNameG8EO),
		Status:        string(constants.OperatorStatusActive),
		CreatedAt:     timestamppb.New(createdAt),
		UpdatedAt:     timestamppb.New(updatedAt),
		StartedAt:     timestamppb.New(startedAt),
		ClaimedAt:     timestamppb.New(claimedAt),
		LastHeartbeatAt: timestamppb.New(lastHeartbeatAt),
	}

	data, err := MarshalOperatorDocument(doc)
	require.NoError(t, err)

	// Decode into map to check field names
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	// Check snake_case keys exist
	assert.Contains(t, decoded, "id")
	assert.Contains(t, decoded, "user_id")
	assert.Contains(t, decoded, "component")
	assert.Contains(t, decoded, "status")
	assert.Contains(t, decoded, "created_at")
	assert.Contains(t, decoded, "updated_at")
	assert.Contains(t, decoded, "started_at")
	assert.Contains(t, decoded, "claimed_at")
	assert.Contains(t, decoded, "last_heartbeat_at")

	// Check timestamps are RFC3339 formatted strings
	createdAtStr := decoded["created_at"].(string)
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.*Z$`, createdAtStr)

	updatedAtStr := decoded["updated_at"].(string)
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.*Z$`, updatedAtStr)

	startedAtStr := decoded["started_at"].(string)
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.*Z$`, startedAtStr)

	claimedAtStr := decoded["claimed_at"].(string)
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.*Z$`, claimedAtStr)

	lastHeartbeatAtStr := decoded["last_heartbeat_at"].(string)
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.*Z$`, lastHeartbeatAtStr)
}

// TestOperatorDocumentBooleanDefaults asserts that is_slot and claimed are
// present as false when unset in the JSON output.
func TestOperatorDocumentBooleanDefaults(t *testing.T) {
	t.Parallel()

	doc := &operatorv1.OperatorDocument{
		Id:        "op-1",
		UserId:    "user-1",
		Component: string(constants.ComponentNameG8EO),
		Status:    string(constants.OperatorStatusActive),
		// IsSlot and Claimed left unset (false by default in proto3)
	}

	data, err := MarshalOperatorDocument(doc)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	// Check they are explicitly present as false
	assert.Contains(t, decoded, "is_slot")
	assert.Equal(t, false, decoded["is_slot"])

	assert.Contains(t, decoded, "claimed")
	assert.Equal(t, false, decoded["claimed"])
}

// TestOperatorDocumentInvalidRoleOperatorRoles asserts that invalid roles in
// operator_roles trigger validation errors.
func TestOperatorDocumentInvalidRoleOperatorRoles(t *testing.T) {
	t.Parallel()

	invalidData := []byte(`{
		"id": "op-1",
		"user_id": "user-1",
		"component": "g8eo",
		"status": "active",
		"operator_roles": ["data", "invalid_role"]
	}`)

	_, err := UnmarshalOperatorDocument(invalidData)
	assert.Error(t, err, "should reject invalid role in operator_roles")
	assert.Contains(t, err.Error(), "invalid_role")
}

// TestOperatorDocumentInvalidRoleRuntimeConfig asserts that invalid roles in
// runtime_config.roles trigger validation errors via UnmarshalOperatorRuntimeConfig.
func TestOperatorDocumentInvalidRoleRuntimeConfig(t *testing.T) {
	t.Parallel()

	invalidData := []byte(`{
		"roles": ["data", "unknown_role"]
	}`)

	_, err := UnmarshalOperatorRuntimeConfig(invalidData)
	assert.Error(t, err, "should reject invalid role in runtime_config.roles")
	assert.Contains(t, err.Error(), "unknown_role")
}

// TestOperatorDocumentLegacyStoredRow asserts that legacy stored JSON (with
// RFC3339 timestamps including microseconds, +00:00 suffix, omitted false fields
// in runtime_config, and unknown keys) decodes without error.
func TestOperatorDocumentLegacyStoredRow(t *testing.T) {
	t.Parallel()

	// Legacy format with microseconds, +00:00 suffix, omitted false fields, unknown key
	legacyData := []byte(`{
		"id": "op-legacy-1",
		"user_id": "user-1",
		"organization_id": "org-1",
		"component": "g8eo",
		"name": "legacy-op",
		"status": "active",
		"operator_session_id": "session-1",
		"created_at": "2026-01-15T10:30:00.123456+00:00",
		"updated_at": "2026-01-15T11:00:00.654321+00:00",
		"operator_roles": ["data", "inference"],
		"runtime_config": {
			"cloud_mode": false,
			"local_storage_enabled": true,
			"http_port": 8080,
			"roles": ["data"]
		},
		"latest_heartbeat_snapshot": {
			"operator_id": "op-legacy-1",
			"timestamp": "2026-01-15T12:00:00Z",
			"status": "healthy"
		},
		"unknown_field": "this should be ignored"
	}`)

	doc, err := UnmarshalOperatorDocument(legacyData)
	require.NoError(t, err, "legacy stored row should decode without error")
	assert.Equal(t, "op-legacy-1", doc.GetId())
	assert.Equal(t, "user-1", doc.GetUserId())
	assert.Equal(t, "legacy-op", doc.GetName())
	assert.Len(t, doc.GetOperatorRoles(), 2)
	assert.NotNil(t, doc.GetRuntimeConfig())
	assert.NotNil(t, doc.GetLatestHeartbeatSnapshot())
}

// TestOperatorDocumentFromStoreWithMissingMetadata asserts that
// OperatorDocumentFromStore populates id and created_at from Document
// metadata when they are missing from the stored JSON data.
func TestOperatorDocumentFromStoreWithMissingMetadata(t *testing.T) {
	t.Parallel()

	// Document with id and created_at in metadata
	createdAt := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	doc := &Document{
		ID:        "doc-id-from-metadata",
		CreatedAt: createdAt,
		Data: map[string]json.RawMessage{
			"user_id":   json.RawMessage(`"user-1"`),
			"component": json.RawMessage(`"g8eo"`),
			"status":    json.RawMessage(`"active"`),
			// id and created_at intentionally omitted from Data
		},
	}

	opDoc, err := OperatorDocumentFromStore(doc)
	require.NoError(t, err)

	// The values should come from Document metadata
	assert.Equal(t, "doc-id-from-metadata", opDoc.GetId())
	assert.NotNil(t, opDoc.GetCreatedAt())
}

// TestOperatorSlotResponseRoundTrip asserts that OperatorSlotResponse with
// multiple operators round-trips through encoding/json.
func TestOperatorSlotResponseRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	op1 := &operatorv1.OperatorDocument{
		Id:        "op-1",
		UserId:    "user-1",
		Component: string(constants.ComponentNameG8EO),
		Status:    string(constants.OperatorStatusActive),
		CreatedAt: timestamppb.New(now),
	}

	op2 := &operatorv1.OperatorDocument{
		Id:        "op-2",
		UserId:    "user-1",
		Component: string(constants.ComponentNameG8EO),
		Status:    string(constants.OperatorStatusOffline),
		CreatedAt: timestamppb.New(now),
	}

	resp := OperatorSlotResponse{
		Success:   true,
		Operators: []*operatorv1.OperatorDocument{op1, op2},
	}

	// Marshal
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	// Unmarshal
	var decoded OperatorSlotResponse
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, resp.Success, decoded.Success)
	assert.Len(t, decoded.Operators, 2)
	assert.True(t, proto.Equal(op1, decoded.Operators[0]))
	assert.True(t, proto.Equal(op2, decoded.Operators[1]))
}

// TestOperatorResponseRoundTrip asserts that OperatorResponse round-trips
// through encoding/json, including the nil operator case.
func TestOperatorResponseRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("with operator", func(t *testing.T) {
		now := time.Now().UTC()
		op := &operatorv1.OperatorDocument{
			Id:        "op-1",
			UserId:    "user-1",
			Component: string(constants.ComponentNameG8EO),
			Status:    string(constants.OperatorStatusActive),
			CreatedAt: timestamppb.New(now),
		}

		resp := OperatorResponse{
			Success:  true,
			Operator: op,
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var decoded OperatorResponse
		require.NoError(t, json.Unmarshal(data, &decoded))

		assert.Equal(t, resp.Success, decoded.Success)
		assert.True(t, proto.Equal(op, decoded.Operator))
	})

	t.Run("nil operator", func(t *testing.T) {
		resp := OperatorResponse{
			Success:  false,
			Operator: nil,
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var decoded OperatorResponse
		require.NoError(t, json.Unmarshal(data, &decoded))

		assert.Equal(t, resp.Success, decoded.Success)
		assert.Nil(t, decoded.Operator)
	})
}

// TestReauthResponseRoundTrip asserts that ReauthResponse round-trips through
// encoding/json, including the nil operator case.
func TestReauthResponseRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("with operator", func(t *testing.T) {
		now := time.Now().UTC()
		op := &operatorv1.OperatorDocument{
			Id:        "op-1",
			UserId:    "user-1",
			Component: string(constants.ComponentNameG8EO),
			Status:    string(constants.OperatorStatusActive),
			CreatedAt: timestamppb.New(now),
		}

		resp := ReauthResponse{
			Success:  true,
			Operator: op,
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var decoded ReauthResponse
		require.NoError(t, json.Unmarshal(data, &decoded))

		assert.Equal(t, resp.Success, decoded.Success)
		assert.True(t, proto.Equal(op, decoded.Operator))
	})

	t.Run("nil operator", func(t *testing.T) {
		resp := ReauthResponse{
			Success:  false,
			Operator: nil,
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var decoded ReauthResponse
		require.NoError(t, json.Unmarshal(data, &decoded))

		assert.Equal(t, resp.Success, decoded.Success)
		assert.Nil(t, decoded.Operator)
	})
}

// TestOperatorDocumentGoldenFixture marshals a fully populated OperatorDocument
// and compares to the golden fixture. When G8E_UPDATE_GOLDEN=1 is set, rewrites
// the fixture instead.
func TestOperatorDocumentGoldenFixture(t *testing.T) {
	t.Parallel()

	// Use fixed timestamps for determinism
	createdAt := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC)
	startedAt := time.Date(2026, 1, 15, 11, 30, 0, 0, time.UTC)
	claimedAt := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	lastHeartbeatAt := time.Date(2026, 1, 15, 12, 30, 0, 0, time.UTC)

	heartbeatSnapshot := &operatorv1.HeartbeatResult{
		OperatorId:       "op-golden-1",
		OperatorSessionId: "session-golden-1",
		Timestamp:        "2026-01-15T12:30:00Z",
		Status:           "healthy",
		EventType:        "heartbeat_sent",
		SourceComponent:  "operator",
		SystemFingerprint: "fingerprint-golden-123",
	}

	runtimeConfig := &operatorv1.OperatorRuntimeConfig{
		CloudMode:                          false,
		CloudProvider:                      "aws",
		LocalStorageEnabled:                true,
		NoGit:                              false,
		LogLevel:                           "info",
		HttpPort:                           8080,
		Roles:                              []string{string(constants.OperatorRoleData), string(constants.OperatorRoleInference)},
		LocalDir:                           "/home/user/.g8e",
		Account:                            "user@example.com",
		InferenceEnabled:                   true,
		InferenceOllamaEndpoint:            "http://localhost:11434",
		ProviderBoundaryObserverEnabled:    true,
		ProvenanceOperatorEnabled:          true,
		ProvenanceOperatorModelStorageRoot: "/models",
		Platform:                           "linux",
	}

	doc := &operatorv1.OperatorDocument{
		Id:               "op-golden-1",
		UserId:           "user-golden-1",
		OrganizationId:   "org-golden-1",
		Component:        string(constants.ComponentNameG8EO),
		Name:             "golden-operator",
		Status:           string(constants.OperatorStatusActive),
		OperatorSessionId: "session-golden-1",
		BoundWebSessionId: "web-session-golden-1",
		OperatorCert:     "-----BEGIN CERTIFICATE-----\nMIICljCCAX4CCQCKz0Rx...\n-----END CERTIFICATE-----",
		OperatorCertSerial: "1234567890abcdef",
		SlotNumber:       1,
		IsSlot:           true,
		Claimed:          true,
		OperatorType:     string(constants.OperatorTypeRemote),
		SystemFingerprint: "fingerprint-golden-123",
		CreatedAt:        timestamppb.New(createdAt),
		UpdatedAt:        timestamppb.New(updatedAt),
		StartedAt:        timestamppb.New(startedAt),
		ClaimedAt:        timestamppb.New(claimedAt),
		LastHeartbeatAt:  timestamppb.New(lastHeartbeatAt),
		OperatorRoles:    []string{string(constants.OperatorRoleData), string(constants.OperatorRoleInference)},
		LocalDir:         "/home/user/.g8e",
		Account:          "user@example.com",
		Port:             8080,
		StopReason:       "normal_shutdown",
		LatestHeartbeatSnapshot: heartbeatSnapshot,
		CurrentHostname:  "host-golden-1",
		RuntimeConfig:    runtimeConfig,
		ConsumedByOperatorId: "parent-op-golden-1",
		OperatorCertChain: "-----BEGIN CERTIFICATE-----\nchain...\n-----END CERTIFICATE-----",
		TerminationReason: "user_requested",
	}

	// Marshal to JSON
	data, err := MarshalOperatorDocument(doc)
	require.NoError(t, err)

	// Pretty-print with indentation
	var formatted map[string]any
	require.NoError(t, json.Unmarshal(data, &formatted))
	prettyData, err := json.MarshalIndent(formatted, "", "  ")
	require.NoError(t, err)

	// Add trailing newline
	prettyData = append(prettyData, '\n')

	fixturePath := "../../protocol/test-fixtures/operator-document.json"

	// Check for update flag
	if os.Getenv("G8E_UPDATE_GOLDEN") == "1" {
		err := os.WriteFile(fixturePath, prettyData, 0o644)
		require.NoError(t, err, "failed to write golden fixture")
		t.Logf("Updated golden fixture at %s", fixturePath)
		return
	}

	// Read and compare
	storedData, err := os.ReadFile(fixturePath)
	require.NoError(t, err, "failed to read golden fixture")

	assert.Equal(t, string(storedData), string(prettyData), "operator document JSON should match golden fixture")
}
