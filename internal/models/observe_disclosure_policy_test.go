// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// decodeStrict decodes JSON using DisallowUnknownFields so that any field not
// present on the typed struct is rejected. This enforces the disclosure
// contract: producers cannot smuggle restricted fields through the typed
// observe models.
func decodeStrict(t *testing.T, data []byte, target any) error {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}

// TestObservePayloadsRejectUnknownFields asserts that each observe event
// payload struct rejects JSON carrying fields not defined on the typed model.
// This is the first line of defense against restricted data leaking through
// the typed payload surface.
func TestObservePayloadsRejectUnknownFields(t *testing.T) {
	cases := []struct {
		name    string
		payload any
		json    string
	}{
		{
			name:    "AgentStatusUpdatedPayload rejects user_email",
			payload: &AgentStatusUpdatedPayload{},
			json:    `{"schema_version":"1.0.0","agent_id":"a","display_name":"A","role":"triage","status":"running","observed_at":"2026-09-08T12:00:00Z","user_email":"leak@example.com"}`,
		},
		{
			name:    "AgentStatusUpdatedPayload rejects web_session_id",
			payload: &AgentStatusUpdatedPayload{},
			json:    `{"schema_version":"1.0.0","agent_id":"a","display_name":"A","role":"triage","status":"running","observed_at":"2026-09-08T12:00:00Z","web_session_id":"sess-123"}`,
		},
		{
			name:    "AgentStatusUpdatedPayload rejects raw_prompt",
			payload: &AgentStatusUpdatedPayload{},
			json:    `{"schema_version":"1.0.0","agent_id":"a","display_name":"A","role":"triage","status":"running","observed_at":"2026-09-08T12:00:00Z","raw_prompt":"secret prompt"}`,
		},
		{
			name:    "RunStatusUpdatedPayload rejects host_path",
			payload: &RunStatusUpdatedPayload{},
			json:    `{"schema_version":"1.0.0","run_id":"r","run_kind":"investigation","display_name":"R","status":"running","completed_tasks":0,"total_tasks":1,"observed_at":"2026-09-08T12:00:00Z","host_path":"/home/user/.g8e"}`,
		},
		{
			name:    "RunStatusUpdatedPayload rejects evidence_key",
			payload: &RunStatusUpdatedPayload{},
			json:    `{"schema_version":"1.0.0","run_id":"r","run_kind":"investigation","display_name":"R","status":"running","completed_tasks":0,"total_tasks":1,"observed_at":"2026-09-08T12:00:00Z","evidence_key":"restricted"}`,
		},
		{
			name:    "EvalRunCompletedPayload rejects raw_output",
			payload: &EvalRunCompletedPayload{},
			json:    `{"schema_version":"1.0.0","run_id":"r","suite_id":"s","suite_version":"1.0.0","arm_id":"direct","terminal_attempts":1,"assigned_tasks":1,"receipt_count":1,"verification_status":"projection_validated","published_projection_sha256":"abc","completed_at":"2026-09-08T12:00:00Z","raw_output":"model output"}`,
		},
		{
			name:    "EvalMetricRecordedPayload rejects session_id",
			payload: &EvalMetricRecordedPayload{},
			json:    `{"schema_version":"1.0.0","run_id":"r","metric_id":"accuracy","metric_version":"1.0.0","unit":"ratio","eligible":1,"denominator":1,"verification_status":"projection_validated","recorded_at":"2026-09-08T12:00:00Z","session_id":"sess-123"}`,
		},
		{
			name:    "ObservedMeasurement rejects secret",
			payload: &ObservedMeasurement{},
			json:    `{"schema_version":"1.0.0","metric_id":"m","value":1.0,"unit":"x","source_component":"c","observed_at":"2026-09-08T12:00:00Z","status":"observed","secret":"key"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := decodeStrict(t, []byte(tc.json), tc.payload)
			require.Error(t, err, "unknown field must be rejected")
			assert.Contains(t, err.Error(), "unknown field")
		})
	}
}

// TestObserveReadModelsRejectUnknownFields asserts that the observe API read
// models reject JSON carrying restricted or unknown fields.
func TestObserveReadModelsRejectUnknownFields(t *testing.T) {
	cases := []struct {
		name   string
		target any
		json   string
	}{
		{
			name:   "AgentStateProjection rejects user_email",
			target: &AgentStateProjection{},
			json:   `{"schema_version":"1.0.0","agent_id":"a","display_name":"A","role":"triage","status":"running","freshness":"observed","observed_at":"2026-09-08T12:00:00Z","user_email":"leak@example.com"}`,
		},
		{
			name:   "RunSummary rejects web_session_id",
			target: &RunSummary{},
			json:   `{"schema_version":"1.0.0","run_id":"r","run_kind":"investigation","display_name":"R","status":"running","completed_tasks":0,"total_tasks":1,"has_receipts":false,"evidence_count":0,"observed_at":"2026-09-08T12:00:00Z","web_session_id":"sess-123"}`,
		},
		{
			name:   "RunDetail rejects raw_prompt",
			target: &RunDetail{},
			json:   `{"schema_version":"1.0.0","run_id":"r","run_kind":"investigation","display_name":"R","status":"running","completed_tasks":0,"total_tasks":1,"tasks":[],"evidence_safe_links":[],"observed_at":"2026-09-08T12:00:00Z","raw_prompt":"secret"}`,
		},
		{
			name:   "EvalSummary rejects evidence_key",
			target: &EvalSummary{},
			json:   `{"schema_version":"1.0.0","run_id":"r","suite_id":"s","suite_version":"1.0.0","arm_id":"direct","status":"completed","verification_status":"projection_validated","receipt_count":1,"metric_count":1,"observed_at":"2026-09-08T12:00:00Z","evidence_key":"restricted"}`,
		},
		{
			name:   "EvalDetail rejects host_path",
			target: &EvalDetail{},
			json:   `{"schema_version":"1.0.0","run_id":"r","suite_id":"s","suite_version":"1.0.0","arm_id":"direct","status":"completed","verification_status":"projection_validated","receipt_count":1,"assigned_tasks":1,"terminal_attempts":1,"metrics":[],"observed_at":"2026-09-08T12:00:00Z","host_path":"/home/user"}`,
		},
		{
			name:   "DownloadArtifact rejects raw_content",
			target: &DownloadArtifact{},
			json:   `{"schema_version":"1.0.0","artifact_id":"a","filename":"f.json","media_type":"application/json","byte_size":1,"sha256":"abc","privacy_classification":"public_safe","download_url":"/api/v1/observe/downloads/a","generated_at":"2026-09-08T12:00:00Z","raw_content":"secret"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := decodeStrict(t, []byte(tc.json), tc.target)
			require.Error(t, err, "unknown field must be rejected")
			assert.Contains(t, err.Error(), "unknown field")
		})
	}
}

// TestObservePayloadSchemasExcludeRestrictedFields asserts that the canonical
// protocol JSON schema files do not define any restricted field names in
// browser-facing read models. This is a static disclosure check: if a
// restricted field appears in a browser-facing read model schema entry, the
// schema itself has been corrupted.
//
// The producer request entries (observe_producer_agent_state_request,
// observe_producer_run_state_request, observe_producer_eval_publication_request)
// are mTLS-internal and legitimately carry web_session_id/cli_session_id as
// SSE routing targets — the gateway derives user_id from the mTLS peer
// certificate, never from the request body. They are excluded from this
// browser-disclosure check.
func TestObservePayloadSchemasExcludeRestrictedFields(t *testing.T) {
	restrictedFieldNames := []string{
		"user_email", "email", "web_session_id", "cli_session_id", "session_id",
		"raw_prompt", "raw_output", "chain_of_thought", "thinking",
		"evidence_key", "evidence_encryption_key", "evidence_nonce",
		"host_path", "host_secret", "enrollment_token", "api_key",
		"password", "credential", "private_key", "secret",
	}
	// Producer request entries are mTLS-internal and carry routing fields
	// (web_session_id, cli_session_id) that are not browser-disclosed. They
	// are excluded from the browser-disclosure check.
	producerEntries := []string{
		"observe_producer_agent_state_request",
		"observe_producer_run_state_request",
		"observe_producer_eval_publication_request",
	}
	schemaFiles := []string{
		"../../protocol/models/observe_event_payloads.json",
		"../../protocol/models/observe_api.json",
	}
	for _, path := range schemaFiles {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(data, &raw))
		for entryName, entryData := range raw {
			if strings.HasPrefix(entryName, "_") {
				continue
			}
			isProducer := false
			for _, p := range producerEntries {
				if entryName == p {
					isProducer = true
					break
				}
			}
			if isProducer {
				continue
			}
			entryContent := string(entryData)
			for _, field := range restrictedFieldNames {
				assert.NotContains(t, entryContent, "\""+field+"\"",
					"restricted field %q must not appear in browser-facing entry %s of %s", field, entryName, path)
			}
		}
	}
}

// TestObservePayloadSchemasExcludeRestrictedDescriptions asserts that the
// canonical protocol JSON schema files do not reference restricted content
// categories in their descriptions, which would indicate the schema is
// documenting disclosure of restricted data.
func TestObservePayloadSchemasExcludeRestrictedDescriptions(t *testing.T) {
	restrictedTerms := []string{
		"raw prompt", "raw output", "chain-of-thought",
		"enrollment token", "evidence encryption key",
		"user email", "session id", "host path",
	}
	schemaFiles := []string{
		"../../protocol/models/observe_event_payloads.json",
		"../../protocol/models/observe_api.json",
	}
	for _, path := range schemaFiles {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		lower := strings.ToLower(string(data))
		for _, term := range restrictedTerms {
			assert.NotContains(t, lower, term,
				"restricted term %q must not appear in %s", term, path)
		}
	}
}

// TestObserveEventPayloadSchemaVersionConstant asserts that the schema version
// constant used by the Go payloads matches the schema version documented in
// the canonical protocol JSON.
func TestObserveEventPayloadSchemaVersionConstant(t *testing.T) {
	assert.Equal(t, "1.0.0", constants.ObserveEventPayloadSchemaVersion,
		"ObserveEventPayloadSchemaVersion must be 1.0.0")
	assert.Equal(t, "1.0.0", constants.ObserveMeasurementSchemaVersion,
		"ObserveMeasurementSchemaVersion must be 1.0.0")
	assert.Equal(t, "1.0.0", constants.ObserveAPIReadModelSchemaVersion,
		"ObserveAPIReadModelSchemaVersion must be 1.0.0")
}
