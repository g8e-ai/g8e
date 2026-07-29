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

package gateway

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/timesvc"
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

func (c *AuditController) handleAuditReceipts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		c.responder.Error(w, http.StatusMethodNotAllowed, constants.ErrMethodNotAllowed.Error())
		return
	}

	txID := r.URL.Query().Get("tx_id")
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
		c.responder.JSON(w, http.StatusOK, receipt)
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

	receipts, err := c.auditStore.ListActionReceiptsSince(since, limit)
	if err != nil {
		c.responder.Error(w, http.StatusInternalServerError, fmt.Errorf("audit_controller: handleAuditReceiptsExport: %w", err).Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(w)
	for _, r := range receipts {
		if err := encoder.Encode(r); err != nil {
			c.logger.Error("Failed to encode audit receipt for export", "transaction_id", r.TransactionID, string(constants.ConnectionStateError), err)
			break
		}
	}
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

	c.responder.JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"events":  events,
		"count":   len(events),
	})
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

	c.responder.JSON(w, http.StatusOK, map[string]interface{}{
		"success":          true,
		"events_summary":   eventSummary,
		"events_total":     len(events),
		"receipts_summary": receiptSummary,
		"receipts_total":   len(receipts),
		"total_records":    len(events) + len(receipts),
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
	report := map[string]interface{}{
		"generated_at":        time.Now().Format(time.RFC3339),
		"operator_session_id": operatorSessionID,
		"events":              events,
		"events_count":        len(events),
		"receipts":            receipts,
		"receipts_count":      len(receipts),
		"total_records":       len(events) + len(receipts),
	}

	c.responder.JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"report":  report,
	})
}
