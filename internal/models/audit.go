// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import "encoding/json"

// AuditReceiptsResponse is the typed response for GET /api/audit/receipts.
type AuditReceiptsResponse struct {
	Success  bool                   `json:"success"`
	Receipts []*ActionReceiptRecord `json:"receipts"`
}

// AuditEventRow is a single audit event row in the events response.
type AuditEventRow struct {
	ID                int64  `json:"id"`
	OperatorSessionID string `json:"operator_session_id"`
	Timestamp         string `json:"timestamp"`
	Type              string `json:"type"`
	CommandRaw        string `json:"command_raw"`
	CommandExitCode   int    `json:"command_exit_code"`
}

// AuditEventsResponse is the typed response for GET /api/audit/events.
type AuditEventsResponse struct {
	Success bool            `json:"success"`
	Events  []AuditEventRow `json:"events"`
	Count   int             `json:"count"`
}

// AuditReportData is the report payload within AuditReportResponse.
type AuditReportData struct {
	GeneratedAt       string            `json:"generated_at"`
	OperatorSessionID string            `json:"operator_session_id"`
	Events            []json.RawMessage `json:"events"`
	EventsCount       int               `json:"events_count"`
	Receipts          []json.RawMessage `json:"receipts"`
	ReceiptsCount     int               `json:"receipts_count"`
	TotalRecords      int               `json:"total_records"`
}

// AuditReportResponse is the typed response for GET /api/audit/report.
type AuditReportResponse struct {
	Success bool            `json:"success"`
	Report  AuditReportData `json:"report"`
}

// AuditSummaryResponse is the typed response for GET /api/audit/summary.
type AuditSummaryResponse struct {
	Success         bool           `json:"success"`
	EventsSummary   map[string]int `json:"events_summary"`
	EventsTotal     int            `json:"events_total"`
	ReceiptsSummary map[string]int `json:"receipts_summary"`
	ReceiptsTotal   int            `json:"receipts_total"`
	TotalRecords    int            `json:"total_records"`
}
