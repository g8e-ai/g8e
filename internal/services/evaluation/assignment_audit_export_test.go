// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func sampleAssignmentAuditEvent(t *testing.T) AssignmentAuditSliceEvent {
	payload, err := ProjectModelRoleInvocationEvent(PublicModelRoleInvocationSignal{
		RunID:        "run-1",
		AssignmentID: "assignment-1",
		VariantID:    "variant-a",
		Role:         "primary",
		TaskID:       "scenario-1",
		ObservedAt:   "2026-09-17T00:00:00Z",
		EventID:      "event-1",
		Completed:    1,
		Total:        2,
	})
	require.NoError(t, err)
	body, err := MarshalPublicLiveEvent(payload)
	require.NoError(t, err)
	return AssignmentAuditSliceEvent{
		Type:      "stage_updated",
		Timestamp: "2026-09-17T00:00:00Z",
		Payload:   body,
	}
}

func TestBuildAssignmentAuditSlice_VaultKeyIsDeterministic(t *testing.T) {
	events := []AssignmentAuditSliceEvent{sampleAssignmentAuditEvent(t)}
	first, err := BuildAssignmentAuditSlice("assignment-1", events)
	require.NoError(t, err)
	second, err := BuildAssignmentAuditSlice("assignment-1", events)
	require.NoError(t, err)
	assert.Equal(t, first.VaultKeySHA256, second.VaultKeySHA256)
}

func TestBuildAssignmentAuditSlice_RoundTripAndBindings(t *testing.T) {
	artifacts, err := BuildAssignmentAuditSlice("assignment-1", []AssignmentAuditSliceEvent{sampleAssignmentAuditEvent(t)})
	require.NoError(t, err)
	require.NotEmpty(t, artifacts.Database)
	require.NotEmpty(t, artifacts.VaultKey)
	assert.Equal(t, hashBytes(artifacts.Database), artifacts.DatabaseSHA256)
	assert.Equal(t, hashBytes(artifacts.VaultKey), artifacts.VaultKeySHA256)
	require.NoError(t, VerifyAssignmentAuditSlice(artifacts.Database, artifacts.VaultKey))

	bindings := AssignmentAuditEvidenceBindings(artifacts)
	require.Len(t, bindings, 2)
	assert.Equal(t, AssignmentAuditSliceKind, bindings[0].GetKind())
	assert.Equal(t, AssignmentAuditVaultKeyKind, bindings[1].GetKind())

	_, bindingsFromEvidence, err := BuildPublicAssignmentEvidence(PublicAssignmentEvidenceInput{
		Result:       &evalv1.EvaluationAssignmentResult{},
		PublicProofs: bindings,
	})
	require.NoError(t, err)
	assert.Len(t, bindingsFromEvidence, 2)
}

func TestVerifyAssignmentAuditSlice_WrongKeyFailsDecrypt(t *testing.T) {
	artifacts, err := BuildAssignmentAuditSlice("assignment-1", []AssignmentAuditSliceEvent{sampleAssignmentAuditEvent(t)})
	require.NoError(t, err)

	wrongKey := []byte(strings.Repeat("a", 64) + "\n")
	err = VerifyAssignmentAuditSlice(artifacts.Database, wrongKey)
	require.Error(t, err)
}

func TestBuildAssignmentAuditSlice_ProhibitedFieldFailsClosed(t *testing.T) {
	event := sampleAssignmentAuditEvent(t)
	fields := map[string]any{}
	require.NoError(t, json.Unmarshal(event.Payload, &fields))
	fields["prompt"] = "secret"
	payload, err := json.Marshal(fields)
	require.NoError(t, err)
	event.Payload = payload

	_, err = BuildAssignmentAuditSlice("assignment-1", []AssignmentAuditSliceEvent{event})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt")
}

func TestVerifyAssignmentAuditSlice_FlippedEventByteBreaksChain(t *testing.T) {
	artifacts, err := BuildAssignmentAuditSlice("assignment-1", []AssignmentAuditSliceEvent{sampleAssignmentAuditEvent(t)})
	require.NoError(t, err)

	tmpFile, err := os.CreateTemp("", "assignment-audit-tamper-*.db")
	require.NoError(t, err)
	dbPath := tmpFile.Name()
	_, err = tmpFile.Write(artifacts.Database)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())
	defer os.Remove(dbPath)

	db, err := sqliteutil.OpenDB(sqliteutil.DefaultDBConfig(dbPath), slog.Default())
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE commitment_ledger SET hash = ? WHERE id = 1`, strings.Repeat("0", 64))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	tampered, err := os.ReadFile(dbPath)
	require.NoError(t, err)

	err = VerifyAssignmentAuditSlice(tampered, artifacts.VaultKey)
	require.Error(t, err)
}
