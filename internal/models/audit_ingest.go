// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

// AuditRecordIngestRequest is the HTTP body for POST /api/v1/audit/records.
type AuditRecordIngestRequest struct {
	EventType          string `json:"event_type"`
	OperatorID         string `json:"operator_id"`
	OperatorSessionID  string `json:"operator_session_id"`
	IdempotencyKey     string `json:"idempotency_key"`
	Payload            []byte `json:"payload"`
	CaseID             string `json:"case_id,omitempty"`
	InvestigationID    string `json:"investigation_id,omitempty"`
	TaskID             string `json:"task_id,omitempty"`
	WebSessionID       string `json:"web_session_id,omitempty"`
	CliSessionID       string `json:"cli_session_id,omitempty"`
	RequestorUserID    string `json:"requestor_user_id,omitempty"`
}

// AuditRecordIngestResponse is returned after the operator acknowledges the append.
type AuditRecordIngestResponse struct {
	Seq  int64  `json:"seq"`
	Hash string `json:"hash"`
}

// AuditRecordPublish is the wire payload on audit:<operator_id>:<session_id>.
type AuditRecordPublish struct {
	EventType         string `json:"event_type"`
	OperatorID        string `json:"operator_id"`
	OperatorSessionID string `json:"operator_session_id"`
	IdempotencyKey    string `json:"idempotency_key"`
	Payload           []byte `json:"payload"`
	CaseID            string `json:"case_id,omitempty"`
	InvestigationID   string `json:"investigation_id,omitempty"`
	TaskID            string `json:"task_id,omitempty"`
	WebSessionID      string `json:"web_session_id,omitempty"`
	CliSessionID      string `json:"cli_session_id,omitempty"`
	RequestorUserID   string `json:"requestor_user_id,omitempty"`
}

// AuditRecordAck is published by the operator on results: after a successful append.
type AuditRecordAck struct {
	IdempotencyKey string `json:"idempotency_key"`
	Seq            int64  `json:"seq"`
	Hash           string `json:"hash"`
}
