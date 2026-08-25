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
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// TestEnsemble_ChatFileCreate verifies the full chat-to-file-change flow
// end to end against the running platform using the fake LLM provider:
//  1. The client sends a chat request to the ensemble with instructions to create a file.
//  2. The ensemble reasons about the request, dispatches a file_create tool call to the operator.
//  3. The auto-approver approves the file edit request delivered via gateway SSE.
//  4. The operator executes the FILE_EDIT via L5Actuator, publishes signed ActionReceipt.
//  5. The gateway receipts relay verifies the signature and records the receipt in the audit store.
//  6. The test polls for the correlated FILE_EDIT receipt and asserts identity and signature.
//  7. The test performs a governed FS_READ read-back on the operator to verify the file content.
func TestEnsemble_ChatFileCreate(t *testing.T) {
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

	runID := fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
	filePath := fmt.Sprintf("/tmp/g8e-e2e-smoke-%s.txt", runID)
	fileContent := fmt.Sprintf("g8e e2e governed file write smoke test run=%s", runID)
	caseTitle := fmt.Sprintf("E2E chat file create %s", runID)
	message := fmt.Sprintf("Create a new file at %s with the content: %s", filePath, fileContent)

	approver := e2eClient.StartApprovalAutoApprover(ctx, e2eCfg.ensembleURL)
	defer approver.Stop()

	connectCtx, cancelConnect := context.WithTimeout(ctx, 15*time.Second)
	err = approver.WaitForConnection(connectCtx)
	cancelConnect()
	require.NoError(t, err, "approval auto-approver SSE connection must succeed")

	notBefore := time.Now().Add(-2 * time.Second)

	chatReq := EnsembleChatRequest{
		Context: EnsembleRequestContext{
			CLISessionID:    e2eClient.cliSessionID,
			UserID:          e2eClient.userID,
			SourceComponent: "CLIENT",
			BoundOperators: []EnsembleBoundOperator{
				{
					OperatorID:        targetOperatorID,
					OperatorSessionID: targetSessionID,
					Status:            string(constants.OperatorStatusBound),
				},
			},
		},
		Message:              message,
		SentinelMode:         true,
		ResourceCreation:     &EnsembleResourceCreation{CreateCase: true, CaseTitle: caseTitle},
		LLMPrimaryProvider:   "fake",
		LLMPrimaryModel:      "fake",
		LLMAssistantProvider: "fake",
		LLMAssistantModel:    "fake",
		LLMLiteProvider:      "fake",
		LLMLiteModel:         "fake",
	}

	chatResp, err := e2eClient.SendChatRequest(ctx, e2eCfg.ensembleURL, chatReq)
	require.NoError(t, err, "ensemble chat request must succeed")
	require.True(t, chatResp.Success, "ensemble chat must return success=true")
	require.NotEmpty(t, chatResp.CaseID, "ensemble chat must return case_id")
	require.NotEmpty(t, chatResp.InvestigationID, "ensemble chat must return investigation_id")
	t.Logf("chat started: case_id=%s investigation_id=%s", chatResp.CaseID, chatResp.InvestigationID)

	var foundReceipt *struct {
		TransactionID   string
		ActionType      string
		TargetResource  string
		Signature       string
		RequestorUserID string
		ActingAppID     string
	}

	require.Eventually(t, func() bool {
		receiptsResp, err := e2eClient.GetAuditReceipts(ctx, "")
		if err != nil {
			t.Logf("GetAuditReceipts poll error: %v", err)
			return false
		}
		for _, r := range receiptsResp.Receipts {
			if r.ActionType != constants.ActionTypeFileEdit {
				continue
			}
			if r.TargetResource != filePath {
				continue
			}
			if !r.ExecutedAt.IsZero() && r.ExecutedAt.Before(notBefore) {
				continue
			}
			if r.Signature == "" {
				continue
			}
			if r.Status != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
				continue
			}
			foundReceipt = &struct {
				TransactionID   string
				ActionType      string
				TargetResource  string
				Signature       string
				RequestorUserID string
				ActingAppID     string
			}{
				TransactionID:   r.TransactionID,
				ActionType:      string(r.ActionType),
				TargetResource:  r.TargetResource,
				Signature:       r.Signature,
				RequestorUserID: r.RequestorUserID,
				ActingAppID:     r.ActingAppID,
			}
			return true
		}
		return false
	}, 60*time.Second, 2*time.Second, "FILE_EDIT receipt for %s must be recorded within 60s", filePath)

	require.NotNil(t, foundReceipt, "correlated FILE_EDIT receipt must be found")
	assert.NotEmpty(t, foundReceipt.TransactionID, "receipt must carry transaction_id")
	assert.GreaterOrEqual(t, len(foundReceipt.Signature), 64, "receipt signature must be valid hex Ed25519 signature")
	assert.Equal(t, e2eClient.userID, foundReceipt.RequestorUserID, "receipt requestor_user_id must match authenticated user")
	assert.NotEmpty(t, foundReceipt.ActingAppID, "receipt acting_app_id must not be empty")
	t.Logf("correlated receipt: tx=%s signature_len=%d requestor=%s app=%s",
		foundReceipt.TransactionID, len(foundReceipt.Signature), foundReceipt.RequestorUserID, foundReceipt.ActingAppID)

	fsReadReq := &operatorv1.FsReadRequested{Path: filePath}
	payload, err := proto.Marshal(fsReadReq)
	require.NoError(t, err, "marshal FsReadRequested payload")

	readReqBody := dispatchRequestJSON{
		TargetOperatorSessionID: targetSessionID,
		ActionType:              string(constants.ActionTypeFsRead),
		Payload:                 payload,
		TargetResource:          filePath,
	}

	var readResp dispatchResponseJSON
	require.Eventually(t, func() bool {
		dispatchCtx, dispatchCancel := context.WithTimeout(ctx, 30*time.Second)
		defer dispatchCancel()
		r, err := e2eClient.DispatchCommand(dispatchCtx, readReqBody, 30*time.Second)
		if err != nil {
			t.Logf("dispatch read error: %v", err)
			return false
		}
		readResp = r
		if !r.Success {
			t.Logf("dispatch read unsuccess: error=%q, event_type=%s, resp=%+v", r.Error, r.EventType, r)
		}
		return readResp.Success
	}, 30*time.Second, 2*time.Second, "governed FS_READ read-back must succeed")

	var fsReadResult operatorv1.FsReadResult
	require.NoError(t, proto.Unmarshal(readResp.ResultPayload, &fsReadResult), "unmarshal FsReadResult")
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, fsReadResult.Status, "fs.read status must be COMPLETED")
	assert.True(t, strings.Contains(string(fsReadResult.Content), fileContent),
		"read-back file content %q must contain expected %q", string(fsReadResult.Content), fileContent)
	t.Logf("governed read-back verified exact content at %s", filePath)
}
