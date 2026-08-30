// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func setupTestAuditController(t *testing.T) *AuditController {
	t.Helper()
	infra := setupTestInfrastructure(t, false)

	return newAuditController(AuditControllerDeps{
		Cfg:        infra.Cfg,
		Logger:     infra.Logger,
		AuditStore: infra.Stores.AuditStore,
		Responder:  infra.Responder,
	})
}

func TestNewAuditController_AllDepsProvidedNoNilFields(t *testing.T) {
	infra := setupTestInfrastructure(t, false)

	controller := newAuditController(AuditControllerDeps{
		Cfg:        infra.Cfg,
		Logger:     infra.Logger,
		AuditStore: infra.Stores.AuditStore,
		Responder:  infra.Responder,
	})

	assert.NotNil(t, controller.cfg)
	assert.NotNil(t, controller.logger)
	assert.NotNil(t, controller.auditStore)
	assert.NotNil(t, controller.responder)

	assert.Equal(t, infra.Cfg, controller.cfg)
	assert.Equal(t, infra.Logger, controller.logger)
	assert.Equal(t, infra.Stores.AuditStore, controller.auditStore)
	assert.Equal(t, infra.Responder, controller.responder)
}

func TestAuditControllerHandleAuditReceipts(t *testing.T) {
	auditController := setupTestAuditController(t)

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/receipts", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReceipts(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("GET by tx_id - not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts?tx_id=nonexistent", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReceipts(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("GET by tx_id returns canonical action receipt", func(t *testing.T) {
		receipt := &operatorv1.ActionReceipt{
			TransactionId:   "tx-evidence",
			TransactionHash: "hash-evidence",
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			SignerKeyId:     "warden-key",
			Signature:       "signature",
			DeterministicStageEvidence: []*operatorv1.DeterministicStageEvidence{{
				StageId:         "tx-evidence:l4",
				Kind:            operatorv1.DeterministicStageKind_DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
				Outcome:         operatorv1.DeterministicStageOutcome_DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
				TransactionId:   "tx-evidence",
				TransactionHash: "hash-evidence",
			}},
		}
		err := auditController.auditStore.RecordActionReceipt(&models.ActionReceiptRecord{
			TransactionID:   receipt.TransactionId,
			TransactionHash: receipt.TransactionHash,
			InvestigationID: "investigation-evidence",
			OperatorID:      "operator-1",
			ActionType:      constants.ActionTypeExecuteBash,
			Status:          receipt.Status,
			ExecutedAt:      time.Now(),
			SignerKeyID:     receipt.SignerKeyId,
			Signature:       receipt.Signature,
			Timestamp:       time.Now(),
			ActionReceipt:   receipt,
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts?tx_id=tx-evidence", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReceipts(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, constants.HeaderValueApplicationJSON, rr.Header().Get(constants.HeaderContentType))
		assert.Equal(t, constants.HeaderValueNoSniff, rr.Header().Get(constants.HeaderXContentTypeOptions))
		assert.Equal(t, constants.HeaderValueDeny, rr.Header().Get(constants.HeaderXFrameOptions))
		assert.Equal(t, constants.HeaderValueCSPNone, rr.Header().Get(constants.HeaderContentSecurityPolicy))
		parsed := &operatorv1.ActionReceipt{}
		require.NoError(t, protojson.Unmarshal(rr.Body.Bytes(), parsed))
		assert.Equal(t, receipt.TransactionId, parsed.TransactionId)
		require.Len(t, parsed.DeterministicStageEvidence, 1)
		assert.Equal(t, receipt.DeterministicStageEvidence[0], parsed.DeterministicStageEvidence[0])
	})

	t.Run("GET by investigation_id and action_type returns canonical action receipt", func(t *testing.T) {
		documentReceipt := &operatorv1.ActionReceipt{
			TransactionId:   "tx-document-update",
			TransactionHash: "hash-document-update",
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			SignerKeyId:     "warden-key",
			Signature:       "signature",
		}
		err := auditController.auditStore.RecordActionReceipt(&models.ActionReceiptRecord{
			TransactionID:   documentReceipt.TransactionId,
			TransactionHash: documentReceipt.TransactionHash,
			InvestigationID: "investigation-evidence",
			OperatorID:      "operator-1",
			ActionType:      constants.ActionTypeDocumentUpdate,
			Status:          documentReceipt.Status,
			ExecutedAt:      time.Now(),
			SignerKeyID:     documentReceipt.SignerKeyId,
			Signature:       documentReceipt.Signature,
			Timestamp:       time.Now(),
			ActionReceipt:   documentReceipt,
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts?investigation_id=investigation-evidence&action_type=EXECUTE_BASH", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReceipts(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		parsed := &operatorv1.ActionReceipt{}
		require.NoError(t, protojson.Unmarshal(rr.Body.Bytes(), parsed))
		assert.Equal(t, "tx-evidence", parsed.TransactionId)
	})

	t.Run("GET by investigation_id rejects duplicate correlation", func(t *testing.T) {
		receipt := &operatorv1.ActionReceipt{
			TransactionId:   "tx-evidence-duplicate",
			TransactionHash: "hash-evidence-duplicate",
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			SignerKeyId:     "warden-key",
			Signature:       "signature",
		}
		err := auditController.auditStore.RecordActionReceipt(&models.ActionReceiptRecord{
			TransactionID:   receipt.TransactionId,
			TransactionHash: receipt.TransactionHash,
			InvestigationID: "investigation-evidence",
			OperatorID:      "operator-1",
			ActionType:      constants.ActionTypeExecuteBash,
			Status:          receipt.Status,
			ExecutedAt:      time.Now(),
			SignerKeyID:     receipt.SignerKeyId,
			Signature:       receipt.Signature,
			Timestamp:       time.Now(),
			ActionReceipt:   receipt,
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts?investigation_id=investigation-evidence&action_type=EXECUTE_BASH", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReceipts(rr, req)

		assert.Equal(t, http.StatusConflict, rr.Code)
		assert.Contains(t, rr.Body.String(), constants.ErrAuditStoreReceiptCorrelationDuplicate.Error())
	})

	t.Run("GET with multiple correlation fields is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts?tx_id=tx-evidence&investigation_id=investigation-evidence&action_type=EXECUTE_BASH", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReceipts(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("GET by investigation_id without action_type is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts?investigation_id=investigation-evidence", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReceipts(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("GET list - success with defaults", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReceipts(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"success":true`)
	})

	t.Run("GET list - with operator_session_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts?operator_session_id=op-123", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReceipts(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("GET list - with limit and offset", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts?limit=10&offset=5", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReceipts(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

func TestAuditControllerHandleAuditReceiptsExport(t *testing.T) {
	auditController := setupTestAuditController(t)

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/receipts/export", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReceiptsExport(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("GET - success with defaults", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts/export", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReceiptsExport(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
		assert.True(t, json.Valid(rr.Body.Bytes()))

		var response models.AuditReceiptsResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
		assert.True(t, response.Success)
	})

	t.Run("GET - with since parameter RFC3339", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts/export?since=2026-01-01T00:00:00Z", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReceiptsExport(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("GET - with since parameter timestamp", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts/export?since=1704067200000", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReceiptsExport(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("GET - with limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts/export?limit=50", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReceiptsExport(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

func TestAuditControllerHandleAuditEvents(t *testing.T) {
	auditController := setupTestAuditController(t)

	t.Run("Failure - method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/events", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditEvents(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Success - empty events", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditEvents(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.Equal(t, 0, int(resp["count"].(float64)))
	})

	t.Run("Success - with operator_session_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events?operator_session_id=op-session-123", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditEvents(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
	})

	t.Run("Success - with limit and offset", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events?limit=10&offset=5", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditEvents(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
	})
}

func TestAuditControllerHandleAuditSummary(t *testing.T) {
	auditController := setupTestAuditController(t)

	t.Run("Failure - method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/summary", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditSummary(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Success - empty summary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/summary", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditSummary(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.Equal(t, 0, int(resp["events_total"].(float64)))
		assert.Equal(t, 0, int(resp["receipts_total"].(float64)))
		assert.Equal(t, 0, int(resp["total_records"].(float64)))
	})

	t.Run("Success - with operator_session_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/summary?operator_session_id=op-session-123", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditSummary(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.Contains(t, resp, "events_summary")
		assert.Contains(t, resp, "receipts_summary")
	})
}

func TestAuditControllerHandleAuditReport(t *testing.T) {
	auditController := setupTestAuditController(t)

	t.Run("Failure - method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/report", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReport(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Success - empty report", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/report", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReport(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.Contains(t, resp, "report")
		report := resp["report"].(map[string]interface{})
		assert.Equal(t, 0, int(report["events_count"].(float64)))
		assert.Equal(t, 0, int(report["receipts_count"].(float64)))
		assert.Equal(t, 0, int(report["total_records"].(float64)))
		assert.Contains(t, report, "generated_at")
	})

	t.Run("Success - with operator_session_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/report?operator_session_id=op-session-123", nil)
		rr := httptest.NewRecorder()
		auditController.handleAuditReport(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.Contains(t, resp, "report")
		report := resp["report"].(map[string]interface{})
		assert.Equal(t, "op-session-123", report["operator_session_id"])
	})
}
