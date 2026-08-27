// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/governance"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/g8e-ai/g8e/v2/test/fixtures"
)

// buildTestEnvelope builds and hashes a canonical GovernanceEnvelope for testing.
func buildTestEnvelope(
	t *testing.T,
	eventType string,
	actionType string,
	targetResource string,
	payload proto.Message,
	stateRoot string,
	userID string,
	appID string,
) []byte {
	t.Helper()

	payloadBytes, err := proto.Marshal(payload)
	require.NoError(t, err)

	env := &commonv1.GovernanceEnvelope{
		Timestamp:       timestamppb.Now(),
		EventType:       eventType,
		ActionType:      actionType,
		TargetResource:  targetResource,
		Payload:         payloadBytes,
		StateMerkleRoot: stateRoot,
		Nonce:           uuid.New().String(),
		ExpiresAt:       timestamppb.New(time.Now().UTC().Add(5 * time.Minute)),
		SourceComponent: commonv1.Component_COMPONENT_AGENT,
		RequestorUserId: userID,
		ActingAppId:     appID,
		Posture:         constants.PostureDoctrine,
	}

	hash, err := governance.GenerateMessageID(env)
	require.NoError(t, err)
	env.Id = hash
	env.TransactionHash = hash

	wire, err := protojson.Marshal(env)
	require.NoError(t, err)
	return wire
}

// rawJSONToString unmarshals a JSON raw message to a string.
func rawJSONToString(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var s string
	require.NoError(t, json.Unmarshal(raw, &s))
	return s
}

// rawJSONToBool unmarshals a JSON raw message to a bool.
func rawJSONToBool(t *testing.T, raw json.RawMessage) bool {
	t.Helper()
	var b bool
	require.NoError(t, json.Unmarshal(raw, &b))
	return b
}

// TestProcessEnvelope_ConcurrentInvestigationTitleMergePreservesFields proves that
// the full gateway governance gauntlet correctly executes DOCUMENT_UPDATE envelopes
// with merge=false for initial creation and merge=true for partial title updates.
// Untouched fields (case_id, user_id, sentinel_mode, status) must survive the merge.
func TestProcessEnvelope_ConcurrentInvestigationTitleMergePreservesFields(t *testing.T) {
	f := fixtures.NewGatewayFixture(t, fixtures.GatewayFixtureOptions{
		TestName:          "governed-doc-merge",
		Posture:           config.PostureDoctrine,
		AllowTestPortZero: true,
	})

	ctx := context.Background()
	stateRootSvc := f.Service.GetGovernanceDeps().StateRootProvider
	envProc := f.Service.GetCommandService()
	require.NotNil(t, envProc, "CommandService must be wired")

	const (
		collection    = "investigations"
		documentID    = "inv-integration-001"
		targetRes     = "investigations/inv-integration-001"
		testUserID    = "user-smoke-test-123"
		testAppID     = "spiffe://g8e.local/app/g8ee"
		initialCaseID = "case-original-456"
	)

	// Step 1: Create initial investigation document with merge=false (full model).
	stateRoot, err := stateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)

	initialFields, err := structpb.NewStruct(map[string]interface{}{
		"case_id":       initialCaseID,
		"user_id":       testUserID,
		"sentinel_mode": true,
		"case_title":    "Initial Title: Create smoke test file",
		"status":        "open",
	})
	require.NoError(t, err)

	createPayload := &operatorv1.DocumentUpdateRequested{
		Collection: collection,
		DocumentId: documentID,
		Updates:    initialFields,
		Merge:      false,
	}

	createWire := buildTestEnvelope(
		t,
		string(constants.EventAppInvestigationCreated),
		string(constants.ActionTypeDocumentUpdate),
		targetRes,
		createPayload,
		stateRoot,
		testUserID,
		testAppID,
	)
	receipt, err := envProc.ProcessEnvelope(ctx, createWire)
	require.NoError(t, err, "ProcessEnvelope should succeed for initial document creation")
	require.NotNil(t, receipt)
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, receipt.Status)
	assert.NotEmpty(t, receipt.Signature, "Receipt must be signed by actuator")

	// Verify initial document was stored in the real SQLite document store.
	doc, err := f.Service.GetDocStore().DocGet(collection, documentID)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, initialCaseID, rawJSONToString(t, doc.Data["case_id"]))
	assert.Equal(t, testUserID, rawJSONToString(t, doc.Data["user_id"]))
	assert.Equal(t, true, rawJSONToBool(t, doc.Data["sentinel_mode"]))
	assert.Equal(t, "Initial Title: Create smoke test file", rawJSONToString(t, doc.Data["case_title"]))
	assert.Equal(t, "open", rawJSONToString(t, doc.Data["status"]))

	// Step 2: Apply a concurrent title-only patch with merge=true.
	stateRoot, err = stateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)

	patchFields, err := structpb.NewStruct(map[string]interface{}{
		"case_title": "AI Generated: Create smoke test file (Refined)",
	})
	require.NoError(t, err)

	mergePayload := &operatorv1.DocumentUpdateRequested{
		Collection: collection,
		DocumentId: documentID,
		Updates:    patchFields,
		Merge:      true,
	}

	mergeWire := buildTestEnvelope(
		t,
		string(constants.EventAppInvestigationUpdated),
		string(constants.ActionTypeDocumentUpdate),
		targetRes,
		mergePayload,
		stateRoot,
		testUserID,
		testAppID,
	)
	mergeReceipt, err := envProc.ProcessEnvelope(ctx, mergeWire)
	require.NoError(t, err, "ProcessEnvelope should succeed for merge update")
	require.NotNil(t, mergeReceipt)
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, mergeReceipt.Status)

	// Step 3: Read back the document and verify untouched fields survived (Bug 10 regression assertion).
	docAfterMerge, err := f.Service.GetDocStore().DocGet(collection, documentID)
	require.NoError(t, err)
	require.NotNil(t, docAfterMerge)

	assert.Equal(t, initialCaseID, rawJSONToString(t, docAfterMerge.Data["case_id"]), "case_id must survive title merge")
	assert.Equal(t, testUserID, rawJSONToString(t, docAfterMerge.Data["user_id"]), "user_id must survive title merge")
	assert.Equal(t, true, rawJSONToBool(t, docAfterMerge.Data["sentinel_mode"]), "sentinel_mode must survive title merge")
	assert.Equal(t, "open", rawJSONToString(t, docAfterMerge.Data["status"]), "status must survive title merge")
	assert.Equal(t, "AI Generated: Create smoke test file (Refined)", rawJSONToString(t, docAfterMerge.Data["case_title"]), "case_title must be updated")

	// Step 4: Delete the document via DOCUMENT_DELETE envelope.
	stateRoot, err = stateRootSvc.GetCurrentStateRoot()
	require.NoError(t, err)

	deletePayload := &operatorv1.DocumentDeleteRequested{
		Collection: collection,
		DocumentId: documentID,
	}

	deleteWire := buildTestEnvelope(
		t,
		string(constants.EventAppInvestigationDeleted),
		string(constants.ActionTypeDocumentDelete),
		targetRes,
		deletePayload,
		stateRoot,
		testUserID,
		testAppID,
	)
	deleteReceipt, err := envProc.ProcessEnvelope(ctx, deleteWire)
	require.NoError(t, err, "ProcessEnvelope should succeed for document deletion")
	require.NotNil(t, deleteReceipt)
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, deleteReceipt.Status)

	// Verify document is deleted from store.
	deletedDoc, err := f.Service.GetDocStore().DocGet(collection, documentID)
	require.NoError(t, err)
	assert.Nil(t, deletedDoc, "Document must not exist after DOCUMENT_DELETE")
}
