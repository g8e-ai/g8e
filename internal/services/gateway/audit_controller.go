// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	storage "github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
)

// AuditController handles audit receipt, event, summary, and report endpoints.
type AuditController struct {
	cfg        *config.Config
	logger     *slog.Logger
	auditStore *storage.SQLAuditStore
	responder  *response.Writer
}

// AuditControllerDeps groups all dependencies for AuditController.
type AuditControllerDeps struct {
	Cfg        *config.Config
	Logger     *slog.Logger
	AuditStore *storage.SQLAuditStore
	Responder  *response.Writer
}

func newAuditController(d AuditControllerDeps) *AuditController {
	return &AuditController{
		cfg:        d.Cfg,
		logger:     d.Logger,
		auditStore: d.AuditStore,
		responder:  d.Responder,
	}
}

// @Summary		Get or list audit receipts
// @Description	Returns one canonical protojson ActionReceipt when tx_id is set or when investigation_id and action_type uniquely correlate a receipt. Without those selectors, returns searchable receipt records with the complete receipt nested under action_receipt.
// @Tags			audit
// @Produce		json
// @Param			tx_id			query		string	false	"Transaction ID for a single canonical ActionReceipt"
// @Param			investigation_id	query		string	false	"Investigation ID for a unique correlation lookup"
// @Param			action_type		query		string	false	"Action type required with investigation_id"
// @Param			operator_session_id	query	string	false	"Operator session filter for list results"
// @Param			limit			query		int		false	"Maximum list results"
// @Param			offset			query		int		false	"List offset"
// @Success		200				{object}	models.AuditReceiptsResponse	"List wrapper; unique selectors instead return a bare operatorv1.ActionReceipt"
// @Failure		400				{string}	string	"Bad Request — conflicting or incomplete selectors"
// @Failure		404				{string}	string	"Receipt not found"
// @Failure		405				{string}	string	"Method Not Allowed"
// @Failure		409				{string}	string	"Conflict — correlation matched multiple receipts"
// @Failure		500				{string}	string	"Internal Server Error"
// @Router			/api/v1/audit/receipts [get]
func (c *AuditController) handleAuditReceipts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	txID := r.URL.Query().Get("tx_id")
	investigationID := r.URL.Query().Get("investigation_id")
	actionType := constants.ActionType(r.URL.Query().Get("action_type"))
	if txID != "" && (investigationID != "" || actionType != "") {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrAuditStoreReceiptLookupConflict.Error())
		return
	}
	if investigationID != "" && actionType == "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrMissingRequiredField.Error())
		return
	}
	if investigationID == "" && actionType != "" {
		c.responder.Error(w, http.StatusBadRequest, constants.ErrAuditStoreReceiptLookupConflict.Error())
		return
	}
	if txID != "" {
		receipt, err := c.auditStore.GetActionReceipt(txID)
		if err != nil {
			c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("audit_controller: handleAuditReceipts: %w", err).Error())
			return
		}
		if receipt == nil {
			c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
			return
		}
		if receipt.ActionReceipt == nil {
			c.responder.Error(w, http.StatusInternalServerError, constants.ErrMissingRequiredField.Error())
			return
		}
		c.responder.ProtoJSON(w, http.StatusOK, receipt.ActionReceipt)
		return
	}
	if investigationID != "" {
		receipt, err := c.auditStore.GetActionReceiptByInvestigationID(investigationID, actionType)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, constants.ErrAuditStoreReceiptCorrelationDuplicate) {
				status = http.StatusConflict
			}
			c.responder.Error(w, status, fmt.Errorf("audit_controller: handleAuditReceipts: %w", err).Error())
			return
		}
		if receipt == nil {
			c.responder.Error(w, http.StatusNotFound, constants.ErrNotFound.Error())
			return
		}
		if receipt.ActionReceipt == nil {
			c.responder.Error(w, http.StatusInternalServerError, constants.ErrMissingRequiredField.Error())
			return
		}
		c.responder.ProtoJSON(w, http.StatusOK, receipt.ActionReceipt)
		return
	}

	operatorSessionID := r.URL.Query().Get("operator_session_id")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}
	offset := 0
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	receipts, err := c.auditStore.ListActionReceipts(operatorSessionID, limit, offset)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("audit_controller: handleAuditReceipts: %w", err).Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, models.AuditReceiptsResponse{
		Success:  true,
		Receipts: receipts,
	})
}

// @Summary		Export audit receipts
// @Description	Returns an AuditReceiptsResponse list wrapper. Every searchable record embeds the complete canonical protojson receipt under action_receipt.
// @Tags			audit
// @Produce		json
// @Param			since			query		string	false	"Inclusive receipt timestamp in RFC3339 format"
// @Param			operator_session_id	query	string	false	"Operator session filter"
// @Param			limit			query		int		false	"Maximum results"
// @Success		200				{object}	models.AuditReceiptsResponse
// @Failure		405				{string}	string	"Method Not Allowed"
// @Failure		500				{string}	string	"Internal Server Error"
// @Router			/api/v1/audit/receipts/export [get]
func (c *AuditController) handleAuditReceiptsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	sinceStr := r.URL.Query().Get("since")
	limitStr := r.URL.Query().Get("limit")

	since := time.Time{}
	if sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		} else if t, err := timesvc.ParseTimestamp(sinceStr); err == nil {
			since = t
		}
	}

	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	operatorSessionID := r.URL.Query().Get("operator_session_id")
	var receipts []*models.ActionReceiptRecord
	var err error
	if operatorSessionID != "" {
		receipts, err = c.auditStore.ListActionReceipts(operatorSessionID, limit, 0)
	} else {
		receipts, err = c.auditStore.ListActionReceiptsSince(since, limit)
	}
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("audit_controller: handleAuditReceiptsExport: %w", err).Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, models.AuditReceiptsResponse{
		Success:  true,
		Receipts: receipts,
	})
}

func (c *AuditController) handleAuditEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	operatorSessionID := r.URL.Query().Get("operator_session_id")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}
	offset := 0
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	events, err := c.auditStore.GetEvents(operatorSessionID, limit, offset)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("audit_controller: handleAuditEvents: %w", err).Error())
		return
	}

	eventRows := make([]models.AuditEventRow, len(events))
	for i, e := range events {
		eventRows[i] = toAuditEventRow(e)
	}

	c.responder.JSON(w, http.StatusOK, models.AuditEventsResponse{
		Success: true,
		Events:  eventRows,
		Count:   len(eventRows),
	})
}

// toAuditEventRow converts a storage.Event to the API-facing AuditEventRow.
func toAuditEventRow(e *storage.Event) models.AuditEventRow {
	if e == nil {
		return models.AuditEventRow{}
	}
	return models.AuditEventRow{
		ID:                e.ID,
		OperatorSessionID: e.OperatorSessionID,
		Timestamp:         e.Timestamp.Format(time.RFC3339),
		Type:              string(e.Type),
		CommandRaw:        e.CommandRaw,
		CommandExitCode:   e.CommandExitCode,
	}
}

const maxAuditQueryLimit = 10000

func (c *AuditController) handleAuditSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	operatorSessionID := r.URL.Query().Get("operator_session_id")

	// Query events summary
	events, err := c.auditStore.GetEvents(operatorSessionID, maxAuditQueryLimit, 0)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("audit_controller: handleAuditSummary: %w", err).Error())
		return
	}

	eventSummary := make(map[string]int)
	for _, event := range events {
		eventSummary[string(event.Type)]++
	}

	// Query receipts summary
	receipts, err := c.auditStore.ListActionReceipts(operatorSessionID, maxAuditQueryLimit, 0)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("audit_controller: handleAuditSummary: %w", err).Error())
		return
	}

	receiptSummary := make(map[string]int)
	for _, receipt := range receipts {
		key := string(receipt.ActionType) + ":" + receipt.Status.String()
		receiptSummary[key]++
	}

	c.responder.JSON(w, http.StatusOK, models.AuditSummaryResponse{
		Success:         true,
		EventsSummary:   eventSummary,
		EventsTotal:     len(events),
		ReceiptsSummary: receiptSummary,
		ReceiptsTotal:   len(receipts),
		TotalRecords:    len(events) + len(receipts),
	})
}

func (c *AuditController) handleAuditReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	operatorSessionID := r.URL.Query().Get("operator_session_id")

	// Fetch events
	events, err := c.auditStore.GetEvents(operatorSessionID, maxAuditQueryLimit, 0)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("audit_controller: handleAuditReport: %w", err).Error())
		return
	}

	// Fetch receipts
	receipts, err := c.auditStore.ListActionReceipts(operatorSessionID, maxAuditQueryLimit, 0)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("audit_controller: handleAuditReport: %w", err).Error())
		return
	}

	// Build comprehensive report
	eventRows := make([]json.RawMessage, len(events))
	for i, e := range events {
		data, err := json.Marshal(toAuditEventRow(e))
		if err != nil {
			c.logger.Error("audit_controller: handleAuditReport: marshal event", string(constants.ConnectionStateError), err)
			eventRows[i] = json.RawMessage("null")
			continue
		}
		eventRows[i] = data
	}

	receiptRows := make([]json.RawMessage, len(receipts))
	for i, r := range receipts {
		data, err := json.Marshal(r)
		if err != nil {
			c.logger.Error("audit_controller: handleAuditReport: marshal receipt", string(constants.ConnectionStateError), err)
			receiptRows[i] = json.RawMessage("null")
			continue
		}
		receiptRows[i] = data
	}

	report := models.AuditReportData{
		GeneratedAt:       time.Now().Format(time.RFC3339),
		OperatorSessionID: operatorSessionID,
		Events:            eventRows,
		EventsCount:       len(events),
		Receipts:          receiptRows,
		ReceiptsCount:     len(receipts),
		TotalRecords:      len(events) + len(receipts),
	}

	c.responder.JSON(w, http.StatusOK, models.AuditReportResponse{
		Success: true,
		Report:  report,
	})
}
