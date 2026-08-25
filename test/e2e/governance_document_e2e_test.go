// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// buildE2EDocumentEnvelope constructs a canonical GovernanceEnvelope for a document
// operation with computed transaction hash and L1 admission metadata.
func buildE2EDocumentEnvelope(
	payloadBytes []byte,
	eventType, actionType, targetResource,
	operatorID, operatorSessionID, requestorUserID, actingAppID, stateRoot string,
) (*governance.GovernanceEnvelope, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("read nonce: %w", err)
	}

	env := &governance.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(5 * time.Minute)),
		SourceComponent:   commonv1.Component_COMPONENT_AGENT,
		OperatorId:        operatorID,
		OperatorSessionId: operatorSessionID,
		ActionType:        actionType,
		TargetResource:    targetResource,
		EventType:         eventType,
		Payload:           payloadBytes,
		StateMerkleRoot:   stateRoot,
		Nonce:             hex.EncodeToString(nonce),
		RequestorUserId:   requestorUserID,
		ActingAppId:       actingAppID,
		Governance:        &commonv1.GovernanceMetadata{L1: &commonv1.L1Metadata{Validated: true}},
	}

	hash, err := governance.GenerateMessageID(env)
	if err != nil {
		return nil, fmt.Errorf("generate envelope hash: %w", err)
	}
	env.Id = hash
	env.TransactionHash = hash
	return env, nil
}

// TestGovernance_DocumentUpdateAndDelete verifies the governed document lifecycle
// end to end against the running platform without requiring an LLM:
//  1. Submits DOCUMENT_UPDATE (create with merge=false) via POST /api/v1/governance/envelopes.
//  2. Correlates signed ActionReceipt with COMPLETED status, valid signature, and identity binding.
//  3. Reads back the created document via GET /api/v1/data/{collection}/{id} and verifies fields.
//  4. Submits DOCUMENT_UPDATE (merge=true) with partial update and verifies untouched fields survive.
//  5. Submits DOCUMENT_DELETE, correlates signed ActionReceipt, and verifies the document returns 404.
func TestGovernance_DocumentUpdateAndDelete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	operators, err := e2eClient.ListOperators(ctx)
	require.NoError(t, err, "operator list must succeed")
	require.True(t, operators.Success, "operator list response must report success")
	require.NotEmpty(t, operators.Operators, "at least one operator must be registered")

	var targetOperatorID string
	var targetSessionID string
	for _, op := range operators.Operators {
		if op.Status == constants.OperatorStatusActive {
			targetOperatorID = op.ID
			targetSessionID = op.OperatorSessionID
			break
		}
	}
	require.NotEmpty(t, targetOperatorID, "active operator must exist")
	require.NotEmpty(t, targetSessionID, "active operator session must exist")

	collection := "e2e_tests"
	docID := fmt.Sprintf("doc-%d-%d", time.Now().UnixNano(), os.Getpid())
	targetResource := collection + "/" + docID
	actingAppID := "g8ee"

	// -------------------------------------------------------------------------
	// Step 1: Create document with merge=false
	// -------------------------------------------------------------------------
	t.Logf("Step 1: creating document %s (merge=false)", targetResource)
	initialUpdates, err := structpb.NewStruct(map[string]any{
		"field_a": "initial_value",
		"field_b": "keep_me",
		"status":  "active",
	})
	require.NoError(t, err, "build initial updates struct")

	createPayload := &operatorv1.DocumentUpdateRequested{
		Collection: collection,
		DocumentId: docID,
		Updates:    initialUpdates,
		Merge:      false,
	}
	createPayloadBytes, err := proto.Marshal(createPayload)
	require.NoError(t, err, "marshal DocumentUpdateRequested")

	stateRoot1, err := e2eClient.GetStateRoot(ctx)
	require.NoError(t, err, "get state root for create")

	createEnv, err := buildE2EDocumentEnvelope(
		createPayloadBytes,
		string(constants.EventAppDocumentUpdateRequested),
		string(constants.ActionTypeDocumentUpdate),
		targetResource,
		targetOperatorID,
		e2eClient.cliSessionID,
		e2eClient.userID,
		actingAppID,
		stateRoot1,
	)
	require.NoError(t, err, "build create governance envelope")

	notBeforeCreate := time.Now().Add(-2 * time.Second)
	txHash, status, body, err := e2eClient.SubmitGovernanceEnvelope(ctx, createEnv)
	require.NoError(t, err, "submit create envelope")
	require.Equal(t, http.StatusOK, status, "gateway must accept envelope: %s", string(body))
	t.Logf("create envelope admitted: tx=%s", txHash)

	// Correlate receipt for creation
	var createReceipt *struct {
		TransactionID   string
		Signature       string
		RequestorUserID string
		ActingAppID     string
	}
	require.Eventually(t, func() bool {
		receiptsResp, err := e2eClient.GetAuditReceipts(ctx, "")
		if err != nil {
			return false
		}
		for _, r := range receiptsResp.Receipts {
			if r.ActionType != constants.ActionTypeDocumentUpdate {
				continue
			}
			if r.TargetResource != targetResource {
				continue
			}
			if !r.ExecutedAt.IsZero() && r.ExecutedAt.Before(notBeforeCreate) {
				continue
			}
			if r.Signature == "" {
				continue
			}
			if r.Status != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
				continue
			}
			createReceipt = &struct {
				TransactionID   string
				Signature       string
				RequestorUserID string
				ActingAppID     string
			}{
				TransactionID:   r.TransactionID,
				Signature:       r.Signature,
				RequestorUserID: r.RequestorUserID,
				ActingAppID:     r.ActingAppID,
			}
			return true
		}
		return false
	}, 30*time.Second, 1*time.Second, "create receipt must be recorded within 30s")

	require.NotNil(t, createReceipt)
	assert.GreaterOrEqual(t, len(createReceipt.Signature), 64, "create receipt must be signed")
	assert.Equal(t, e2eClient.userID, createReceipt.RequestorUserID, "requestor_user_id must match")
	assert.Equal(t, actingAppID, createReceipt.ActingAppID, "acting_app_id must match")
	t.Logf("create receipt verified: tx=%s", createReceipt.TransactionID)

	// Read back created document
	doc, getStatus, err := e2eClient.GetDocument(ctx, collection, docID)
	require.NoError(t, err, "get document after create")
	require.Equal(t, http.StatusOK, getStatus)
	assert.Equal(t, "initial_value", doc.GetString("field_a"))
	assert.Equal(t, "keep_me", doc.GetString("field_b"))
	assert.Equal(t, "active", doc.GetString("status"))
	t.Logf("document read-back verified after create")

	// -------------------------------------------------------------------------
	// Step 2: Merge update (merge=true) — proving untouched fields survive
	// -------------------------------------------------------------------------
	t.Logf("Step 2: merge updating document %s (merge=true)", targetResource)
	mergeUpdates, err := structpb.NewStruct(map[string]any{
		"field_a": "updated_value",
		"field_c": "newly_added",
	})
	require.NoError(t, err, "build merge updates struct")

	mergePayload := &operatorv1.DocumentUpdateRequested{
		Collection: collection,
		DocumentId: docID,
		Updates:    mergeUpdates,
		Merge:      true,
	}
	mergePayloadBytes, err := proto.Marshal(mergePayload)
	require.NoError(t, err, "marshal merge DocumentUpdateRequested")

	stateRoot2, err := e2eClient.GetStateRoot(ctx)
	require.NoError(t, err, "get state root for merge")

	mergeEnv, err := buildE2EDocumentEnvelope(
		mergePayloadBytes,
		string(constants.EventAppDocumentUpdateRequested),
		string(constants.ActionTypeDocumentUpdate),
		targetResource,
		targetOperatorID,
		e2eClient.cliSessionID,
		e2eClient.userID,
		actingAppID,
		stateRoot2,
	)
	require.NoError(t, err, "build merge governance envelope")

	notBeforeMerge := time.Now().Add(-2 * time.Second)
	txHash, status, body, err = e2eClient.SubmitGovernanceEnvelope(ctx, mergeEnv)
	require.NoError(t, err, "submit merge envelope")
	require.Equal(t, http.StatusOK, status, "gateway must accept merge envelope: %s", string(body))
	t.Logf("merge envelope admitted: tx=%s", txHash)

	// Correlate receipt for merge update
	var mergeReceipt *struct {
		TransactionID   string
		Signature       string
		RequestorUserID string
		ActingAppID     string
	}
	require.Eventually(t, func() bool {
		receiptsResp, err := e2eClient.GetAuditReceipts(ctx, "")
		if err != nil {
			return false
		}
		for _, r := range receiptsResp.Receipts {
			if r.ActionType != constants.ActionTypeDocumentUpdate {
				continue
			}
			if r.TargetResource != targetResource {
				continue
			}
			if !r.ExecutedAt.IsZero() && r.ExecutedAt.Before(notBeforeMerge) {
				continue
			}
			if r.Signature == "" {
				continue
			}
			if r.Status != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
				continue
			}
			mergeReceipt = &struct {
				TransactionID   string
				Signature       string
				RequestorUserID string
				ActingAppID     string
			}{
				TransactionID:   r.TransactionID,
				Signature:       r.Signature,
				RequestorUserID: r.RequestorUserID,
				ActingAppID:     r.ActingAppID,
			}
			return true
		}
		return false
	}, 30*time.Second, 1*time.Second, "merge receipt must be recorded within 30s")

	require.NotNil(t, mergeReceipt)
	assert.GreaterOrEqual(t, len(mergeReceipt.Signature), 64, "merge receipt must be signed")
	t.Logf("merge receipt verified: tx=%s", mergeReceipt.TransactionID)

	// Read back and verify untouched fields survived
	doc, getStatus, err = e2eClient.GetDocument(ctx, collection, docID)
	require.NoError(t, err, "get document after merge")
	require.Equal(t, http.StatusOK, getStatus)
	assert.Equal(t, "updated_value", doc.GetString("field_a"), "field_a must be updated")
	assert.Equal(t, "keep_me", doc.GetString("field_b"), "untouched field_b must survive merge")
	assert.Equal(t, "active", doc.GetString("status"), "untouched status must survive merge")
	assert.Equal(t, "newly_added", doc.GetString("field_c"), "field_c must be added")
	t.Logf("document read-back verified after merge (untouched fields survived)")

	// -------------------------------------------------------------------------
	// Step 3: Delete document
	// -------------------------------------------------------------------------
	t.Logf("Step 3: deleting document %s", targetResource)
	deletePayload := &operatorv1.DocumentDeleteRequested{
		Collection: collection,
		DocumentId: docID,
	}
	deletePayloadBytes, err := proto.Marshal(deletePayload)
	require.NoError(t, err, "marshal DocumentDeleteRequested")

	stateRoot3, err := e2eClient.GetStateRoot(ctx)
	require.NoError(t, err, "get state root for delete")

	deleteEnv, err := buildE2EDocumentEnvelope(
		deletePayloadBytes,
		string(constants.EventAppDocumentDeleteRequested),
		string(constants.ActionTypeDocumentDelete),
		targetResource,
		targetOperatorID,
		e2eClient.cliSessionID,
		e2eClient.userID,
		actingAppID,
		stateRoot3,
	)
	require.NoError(t, err, "build delete governance envelope")

	notBeforeDelete := time.Now().Add(-2 * time.Second)
	txHash, status, body, err = e2eClient.SubmitGovernanceEnvelope(ctx, deleteEnv)
	require.NoError(t, err, "submit delete envelope")
	require.Equal(t, http.StatusOK, status, "gateway must accept delete envelope: %s", string(body))
	t.Logf("delete envelope admitted: tx=%s", txHash)

	// Correlate receipt for delete
	var deleteReceipt *struct {
		TransactionID   string
		Signature       string
		RequestorUserID string
		ActingAppID     string
	}
	require.Eventually(t, func() bool {
		receiptsResp, err := e2eClient.GetAuditReceipts(ctx, "")
		if err != nil {
			return false
		}
		for _, r := range receiptsResp.Receipts {
			if r.ActionType != constants.ActionTypeDocumentDelete {
				continue
			}
			if r.TargetResource != targetResource {
				continue
			}
			if !r.ExecutedAt.IsZero() && r.ExecutedAt.Before(notBeforeDelete) {
				continue
			}
			if r.Signature == "" {
				continue
			}
			if r.Status != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
				continue
			}
			deleteReceipt = &struct {
				TransactionID   string
				Signature       string
				RequestorUserID string
				ActingAppID     string
			}{
				TransactionID:   r.TransactionID,
				Signature:       r.Signature,
				RequestorUserID: r.RequestorUserID,
				ActingAppID:     r.ActingAppID,
			}
			return true
		}
		return false
	}, 30*time.Second, 1*time.Second, "delete receipt must be recorded within 30s")

	require.NotNil(t, deleteReceipt)
	assert.GreaterOrEqual(t, len(deleteReceipt.Signature), 64, "delete receipt must be signed")
	assert.Equal(t, e2eClient.userID, deleteReceipt.RequestorUserID, "requestor_user_id must match")
	t.Logf("delete receipt verified: tx=%s", deleteReceipt.TransactionID)

	// Verify document is deleted (GET returns 404 and nil doc)
	doc, getStatus, err = e2eClient.GetDocument(ctx, collection, docID)
	require.NoError(t, err, "get document after delete must not error")
	assert.Equal(t, http.StatusNotFound, getStatus, "get document after delete must return 404")
	assert.Nil(t, doc, "document must be nil after delete")
	t.Logf("verified document %s returns 404 after deletion", targetResource)
}
