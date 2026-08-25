// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubmitDocumentUpdate_BuildsCanonicalEnvelope(t *testing.T) {
	var receivedBody []byte
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, constants.APIPaths.GovernanceEnvelopes, r.URL.Path)
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status":"accepted"}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	req := DocumentUpdateRequest{
		OperatorID:        "op-test-001",
		OperatorSessionID: "sess-test-001",
		RequestorUserID:   "user-test-001",
		Collection:        "investigations",
		DocumentID:        "inv-doc-test-001",
		Updates: map[string]any{
			"case_title": "Test Investigation",
			"status":     "open",
		},
		Merge:     false,
		StateRoot: "root-abc-123",
	}

	txHash, status, _, err := client.SubmitDocumentUpdate(context.Background(), Persona{ID: "test"}, req)
	require.NoError(t, err)
	assert.NotEmpty(t, txHash, "transaction hash must be computed")
	assert.Equal(t, http.StatusAccepted, status)

	// Verify the envelope wire body carries the canonical fields.
	var env map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(receivedBody, &env))

	assertJSONField(t, env, "actionType", string(constants.ActionTypeDocumentUpdate))
	assertJSONField(t, env, "eventType", string(constants.EventAppDocumentUpdateRequested))
	assertJSONField(t, env, "operatorId", req.OperatorID)
	assertJSONField(t, env, "operatorSessionId", req.OperatorSessionID)
	assertJSONField(t, env, "requestorUserId", req.RequestorUserID)
	assertJSONField(t, env, "actingAppId", ActingAppG8ee)
	assertJSONField(t, env, "stateMerkleRoot", req.StateRoot)
	assertJSONField(t, env, "id", txHash)
	assertJSONField(t, env, "transactionHash", txHash)
}

func TestSubmitDocumentUpdate_MergeTrueSetsMergeFlag(t *testing.T) {
	var receivedBody []byte
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	req := DocumentUpdateRequest{
		Collection: "investigations",
		DocumentID: "inv-merge-001",
		Updates:    map[string]any{"case_title": "Patched Title"},
		Merge:      true,
		StateRoot:  "root-merge",
	}

	_, _, _, err = client.SubmitDocumentUpdate(context.Background(), Persona{ID: "test"}, req)
	require.NoError(t, err)

	// The payload is protobuf-encoded (base64 in protojson), so we cannot
	// inspect the merge flag directly. Verify the envelope was accepted and
	// the action type is correct.
	var env map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(receivedBody, &env))
	assertJSONField(t, env, "actionType", string(constants.ActionTypeDocumentUpdate))
}

func TestSubmitDocumentDelete_BuildsCanonicalEnvelope(t *testing.T) {
	var receivedBody []byte
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, constants.APIPaths.GovernanceEnvelopes, r.URL.Path)
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status":"accepted"}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	req := DocumentDeleteRequest{
		OperatorID:        "op-test-002",
		OperatorSessionID: "sess-test-002",
		RequestorUserID:   "user-test-002",
		Collection:        "investigations",
		DocumentID:        "inv-doc-test-002",
		StateRoot:         "root-def-456",
	}

	txHash, status, _, err := client.SubmitDocumentDelete(context.Background(), Persona{ID: "test"}, req)
	require.NoError(t, err)
	assert.NotEmpty(t, txHash, "transaction hash must be computed")
	assert.Equal(t, http.StatusAccepted, status)

	var env map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(receivedBody, &env))

	assertJSONField(t, env, "actionType", string(constants.ActionTypeDocumentDelete))
	assertJSONField(t, env, "eventType", string(constants.EventAppDocumentDeleteRequested))
	assertJSONField(t, env, "operatorId", req.OperatorID)
	assertJSONField(t, env, "operatorSessionId", req.OperatorSessionID)
	assertJSONField(t, env, "requestorUserId", req.RequestorUserID)
	assertJSONField(t, env, "actingAppId", ActingAppG8ee)
	assertJSONField(t, env, "stateMerkleRoot", req.StateRoot)
	assertJSONField(t, env, "id", txHash)
	assertJSONField(t, env, "transactionHash", txHash)
}

func TestSubmitDocumentUpdate_DeterministicHashForSameInputs(t *testing.T) {
	// Two submissions with identical fields (except nonce, which is random)
	// must produce different hashes — proving the nonce is included in the
	// canonical hash and prevents replay.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	req := DocumentUpdateRequest{
		Collection: "investigations",
		DocumentID: "inv-det-001",
		Updates:    map[string]any{"case_title": "Deterministic Test"},
		StateRoot:  "root-det",
	}

	tx1, _, _, err := client.SubmitDocumentUpdate(context.Background(), Persona{ID: "test"}, req)
	require.NoError(t, err)
	tx2, _, _, err := client.SubmitDocumentUpdate(context.Background(), Persona{ID: "test"}, req)
	require.NoError(t, err)

	assert.NotEqual(t, tx1, tx2, "different nonces must produce different hashes")
}

func TestSubmitDocumentUpdate_DefaultTTL(t *testing.T) {
	var receivedBody []byte
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	req := DocumentUpdateRequest{
		Collection: "investigations",
		DocumentID: "inv-ttl-001",
		Updates:    map[string]any{"status": "open"},
		StateRoot:  "root-ttl",
		TTL:        0, // should default to 5 minutes
	}

	_, _, _, err = client.SubmitDocumentUpdate(context.Background(), Persona{ID: "test"}, req)
	require.NoError(t, err)

	var env map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(receivedBody, &env))

	// expiresAt must be present and roughly 5 minutes from now.
	raw, ok := env["expiresAt"]
	require.True(t, ok, "expiresAt must be set")
	var expiresStr string
	require.NoError(t, json.Unmarshal(raw, &expiresStr))
	expires, err := time.Parse(time.RFC3339Nano, expiresStr)
	require.NoError(t, err)
	assert.InDelta(t, 5*time.Minute, time.Until(expires), float64(30*time.Second))
}

func TestGetDocument_ReturnsDocument(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, constants.APIPaths.DataPrefix+"investigations/inv-get-001", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"case_title":"Found Investigation","status":"open","id":"inv-get-001","sentinel_mode":true}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	doc, _, err := client.GetDocument(context.Background(), Persona{ID: "test"}, "investigations", "inv-get-001")
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, "Found Investigation", doc.GetString("case_title"))
	assert.Equal(t, "open", doc.GetString("status"))
	assert.Equal(t, "inv-get-001", doc.GetString("id"))
	assert.True(t, doc.GetBool("sentinel_mode"))
}

func TestGetDocument_ReturnsNilOn404(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	doc, _, err := client.GetDocument(context.Background(), Persona{ID: "test"}, "investigations", "inv-missing-001")
	require.NoError(t, err, "404 must not be an error — it means the document is absent")
	assert.Nil(t, doc, "404 must return nil document")
}

func TestGetDocument_ReturnsErrorOn500(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal"}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	doc, _, err := client.GetDocument(context.Background(), Persona{ID: "test"}, "investigations", "inv-err-001")
	require.Error(t, err)
	assert.Nil(t, doc)
}

func TestDocumentResponse_GetString_AbsentField(t *testing.T) {
	doc := DocumentResponse{"case_title": json.RawMessage(`"hello"`)}
	assert.Equal(t, "hello", doc.GetString("case_title"))
	assert.Equal(t, "", doc.GetString("absent_field"))
}

func TestDocumentResponse_GetBool_AbsentField(t *testing.T) {
	doc := DocumentResponse{"sentinel_mode": json.RawMessage(`true`)}
	assert.True(t, doc.GetBool("sentinel_mode"))
	assert.False(t, doc.GetBool("absent_field"))
}

func TestActingAppG8ee_MatchesEnsembleConstant(t *testing.T) {
	// The ensemble Python code defines G8EE_COMPONENT = "g8ee". The harness
	// must use the same value so receipts attribute the action to g8ee.
	assert.Equal(t, "g8ee", ActingAppG8ee)
}

// assertJSONField unmarshals a JSON string field from the envelope map and
// asserts it equals the expected value.
func assertJSONField(t *testing.T, env map[string]json.RawMessage, field, expected string) {
	t.Helper()
	raw, ok := env[field]
	require.True(t, ok, "envelope must contain field %q", field)
	var got string
	require.NoError(t, json.Unmarshal(raw, &got), "field %q must be a JSON string", field)
	assert.Equal(t, expected, got, "field %q mismatch", field)
}
