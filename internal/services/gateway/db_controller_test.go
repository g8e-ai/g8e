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

//go:build integration

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDBController(t *testing.T) (*DBController, *Stores) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)

	dbController := newDBController(DBControllerDeps{
		Cfg:         infra.Cfg,
		Logger:      infra.Logger,
		DocStore:    infra.Stores.DocStore,
		KVStore:     infra.Stores.KVStore,
		SSEStore:    infra.Stores.SSEStore,
		BlobStore:   infra.Stores.BlobStore,
		AuditStore:  infra.Stores.AuditStore,
		SignerStore: infra.Stores.SignerStore,
		Auth:        infra.Auth,
		Pubsub:      infra.Pubsub,
		UserSvc:     infra.UserSvc,
		Responder:   infra.Responder,
	})

	return dbController, infra.Stores
}

func TestDBControllerHandleDB(t *testing.T) {
	dbController, stores := setupTestDBController(t)

	t.Run("BadRequest - no collection", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/data/", nil)
		rr := httptest.NewRecorder()
		dbController.handleDataDB(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - no ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/data/users/", nil)
		rr := httptest.NewRecorder()
		dbController.handleDataDB(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("PUT and GET", func(t *testing.T) {
		data := map[string]string{"name": "alice"}
		reqPut := httptest.NewRequest(http.MethodPut, "/api/v1/data/settings/u1", bytes.NewReader(mustDocJSON(t, data)))
		rrPut := httptest.NewRecorder()
		dbController.handleDataDB(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/data/settings/u1", nil)
		rrGet := httptest.NewRecorder()
		dbController.handleDataDB(rrGet, reqGet)
		assert.Equal(t, http.StatusOK, rrGet.Code)

		type testDoc struct {
			Name string `json:"name"`
		}
		var doc testDoc
		err := json.Unmarshal(rrGet.Body.Bytes(), &doc)
		require.NoError(t, err)
		assert.Equal(t, "alice", doc.Name)
	})

	t.Run("PATCH", func(t *testing.T) {
		patch := map[string]string{"role": "admin"}
		reqPatch := httptest.NewRequest(http.MethodPatch, "/api/v1/data/settings/u1", bytes.NewReader(mustDocJSON(t, patch)))
		rrPatch := httptest.NewRecorder()
		dbController.handleDataDB(rrPatch, reqPatch)
		assert.Equal(t, http.StatusOK, rrPatch.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/data/settings/u1", nil)
		rrGet := httptest.NewRecorder()
		dbController.handleDataDB(rrGet, reqGet)
		assert.Equal(t, http.StatusOK, rrGet.Code)

		type testDocWithRole struct {
			Name string `json:"name"`
			Role string `json:"role"`
		}
		var doc testDocWithRole
		err := json.Unmarshal(rrGet.Body.Bytes(), &doc)
		require.NoError(t, err)
		assert.Equal(t, "alice", doc.Name)
		assert.Equal(t, "admin", doc.Role)
	})

	t.Run("DELETE", func(t *testing.T) {
		reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/data/settings/u1", nil)
		rrDel := httptest.NewRecorder()
		dbController.handleDataDB(rrDel, reqDel)
		assert.Equal(t, http.StatusOK, rrDel.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/data/settings/u1", nil)
		rrGet := httptest.NewRecorder()
		dbController.handleDataDB(rrGet, reqGet)
		assert.Equal(t, http.StatusNotFound, rrGet.Code)
	})

	t.Run("Query", func(t *testing.T) {
		stores.DocStore.DocSet("items", "i1", mustDocJSON(t, map[string]int{"val": 10}))
		stores.DocStore.DocSet("items", "i2", mustDocJSON(t, map[string]int{"val": 20}))

		query := models.DocQueryRequest{
			Limit: 1,
		}
		body := mustMarshalJSON(t, query)
		reqQuery := httptest.NewRequest(http.MethodPost, "/api/v1/data/items/_query", bytes.NewReader(body))
		rrQuery := httptest.NewRecorder()
		dbController.handleDataDB(rrQuery, reqQuery)
		assert.Equal(t, http.StatusOK, rrQuery.Code)

		type queryResult struct {
			Val int `json:"val"`
		}
		var results []queryResult
		err := json.Unmarshal(rrQuery.Body.Bytes(), &results)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/data/settings/u1", strings.NewReader("{invalid-json}"))
		rr := httptest.NewRecorder()
		dbController.handleDataDB(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"invalid JSON body"}`, rr.Body.String())
	})

	t.Run("PATCH not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/data/settings/nonexistent", bytes.NewReader(mustDocJSON(t, map[string]string{"foo": "bar"})))
		rr := httptest.NewRecorder()
		dbController.handleDataDB(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("DELETE not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/data/users/nonexistent", nil)
		rr := httptest.NewRecorder()
		dbController.handleDataDB(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/data/users/u1", nil)
		rr := httptest.NewRecorder()
		dbController.handleDataDB(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Non-bootstrap mutations redirect to governance envelope", func(t *testing.T) {
		tests := []struct {
			method string
			body   []byte
		}{
			{method: http.MethodPut, body: mustDocJSON(t, map[string]string{"name": "alice"})},
			{method: http.MethodPatch, body: mustDocJSON(t, map[string]string{"role": "admin"})},
			{method: http.MethodDelete},
		}

		for _, tt := range tests {
			req := httptest.NewRequest(tt.method, "/api/v1/data/items/i1", bytes.NewReader(tt.body))
			rr := httptest.NewRecorder()
			dbController.handleDataDB(rr, req)
			assert.Equal(t, http.StatusConflict, rr.Code, "method=%s", tt.method)
			assert.JSONEq(t, `{"error":"submit via POST /api/v1/governance/envelopes"}`, rr.Body.String())
		}
	})

	t.Run("Query validation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/data/items/_query", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()
		dbController.handleDBQuery(rr, req, "items")
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("SSE Events count", func(t *testing.T) {
		stores.SSEStore.SSEEventsAppend(SSERoute{WebSessionID: "s1"}, "T", "{}", "")
		req := httptest.NewRequest(http.MethodGet, "/api/v1/data/_sse_events/count", nil)
		rr := httptest.NewRecorder()
		dbController.handleSSEEvents(rr, req, "count")
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("SSE Events wipe", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/data/_sse_events", nil)
		rr := httptest.NewRecorder()
		dbController.handleSSEEvents(rr, req, "")
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("SSE Events invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/data/_sse_events/invalid", nil)
		rr := httptest.NewRecorder()
		dbController.handleSSEEvents(rr, req, "invalid")
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestDBControllerHandleKV(t *testing.T) {
	dbController, stores := setupTestDBController(t)

	t.Run("PUT and GET", func(t *testing.T) {
		reqPut := httptest.NewRequest(http.MethodPut, "/api/v1/kv/k1", bytes.NewReader(mustDocJSON(t, models.KVSetRequest{Value: "g8e"})))
		rrPut := httptest.NewRecorder()
		dbController.handleKV(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/kv/k1", nil)
		rrGet := httptest.NewRecorder()
		dbController.handleKV(rrGet, reqGet)
		assert.Equal(t, http.StatusOK, rrGet.Code)
		assert.Contains(t, rrGet.Body.String(), `"value":"g8e"`)
	})

	t.Run("TTL and Expire", func(t *testing.T) {
		reqTtl := httptest.NewRequest(http.MethodGet, "/api/v1/kv/k1/_ttl", nil)
		rrTtl := httptest.NewRecorder()
		dbController.handleKV(rrTtl, reqTtl)
		assert.Equal(t, http.StatusOK, rrTtl.Code)

		reqExp := httptest.NewRequest(http.MethodPut, "/api/v1/kv/k1/_expire", bytes.NewReader(mustDocJSON(t, models.KVExpireRequest{TTL: 100})))
		rrExp := httptest.NewRecorder()
		dbController.handleKV(rrExp, reqExp)
		assert.Equal(t, http.StatusOK, rrExp.Code)
	})

	t.Run("Scan and DeletePattern", func(t *testing.T) {
		stores.KVStore.KVSet("pref:1", "a", 0)
		stores.KVStore.KVSet("pref:2", "b", 0)

		reqScan := httptest.NewRequest(http.MethodPost, "/api/v1/kv/_scan", bytes.NewReader(mustDocJSON(t, models.KVPatternRequest{Pattern: "pref:*"})))
		rrScan := httptest.NewRecorder()
		dbController.handleKV(rrScan, reqScan)
		assert.Equal(t, http.StatusOK, rrScan.Code)
		assert.Contains(t, rrScan.Body.String(), "pref:1")

		reqDel := httptest.NewRequest(http.MethodPost, "/api/v1/kv/_delete_pattern", bytes.NewReader(mustDocJSON(t, models.KVPatternRequest{Pattern: "pref:*"})))
		rrDel := httptest.NewRecorder()
		dbController.handleKV(rrDel, reqDel)
		assert.Equal(t, http.StatusOK, rrDel.Code)
		assert.Contains(t, rrDel.Body.String(), `"deleted":2`)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/kv/k1", strings.NewReader("{invalid-json}"))
		rr := httptest.NewRecorder()
		dbController.handleKV(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("TTL required for expire", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/kv/k1/_expire", strings.NewReader(`{"ttl":0}`))
		rr := httptest.NewRecorder()
		dbController.handleKV(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Keys", func(t *testing.T) {
		stores.KVStore.KVSet("key1", "val1", 0)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/_keys", strings.NewReader(`{"pattern":"key*"}`))
		rr := httptest.NewRecorder()
		dbController.handleKVKeys(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "key1")
	})

	t.Run("KV Keys Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/_keys", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		dbController.handleKVKeys(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Scan Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/_scan", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		dbController.handleKVScan(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Delete Pattern Missing Pattern", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/_delete_pattern", strings.NewReader(`{}`))
		rr := httptest.NewRecorder()
		dbController.handleKVDeletePattern(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Delete Pattern Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/_delete_pattern", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		dbController.handleKVDeletePattern(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/k1", strings.NewReader(`{"value":"x"}`))
		rr := httptest.NewRecorder()
		dbController.handleKV(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestDBControllerHandleBlob(t *testing.T) {
	dbController, _ := setupTestDBController(t)

	t.Run("PUT and GET", func(t *testing.T) {
		content := []byte("blob-data")
		reqPut := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/putget-b1", bytes.NewReader(content))
		reqPut.Header.Set("Content-Type", "text/plain")
		reqPut = reqPut.WithContext(context.WithValue(reqPut.Context(), constants.ContextKeyUserID, "user-1"))
		rrPut := httptest.NewRecorder()
		dbController.handleBlob(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/blobs/temp/putget-b1", nil)
		rrGet := httptest.NewRecorder()
		dbController.handleBlob(rrGet, reqGet)
		assert.Equal(t, http.StatusOK, rrGet.Code)
		assert.Equal(t, content, rrGet.Body.Bytes())
		assert.Equal(t, "text/plain", rrGet.Header().Get("Content-Type"))
	})

	t.Run("Metadata", func(t *testing.T) {
		content := []byte("blob-data")
		reqPut := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/cache/meta-b2", bytes.NewReader(content))
		reqPut.Header.Set("Content-Type", "text/plain")
		reqPut = reqPut.WithContext(context.WithValue(reqPut.Context(), constants.ContextKeyUserID, "user-1"))
		rrPut := httptest.NewRecorder()
		dbController.handleBlob(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqMeta := httptest.NewRequest(http.MethodGet, "/api/v1/blobs/cache/meta-b2/meta", nil)
		rrMeta := httptest.NewRecorder()
		dbController.handleBlob(rrMeta, reqMeta)
		assert.Equal(t, http.StatusOK, rrMeta.Code)
		assert.Contains(t, rrMeta.Body.String(), `"id":"meta-b2"`)
	})

	t.Run("Too Large", func(t *testing.T) {
		largeBody := make([]byte, maxBlobBodySize+1)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/large-test", bytes.NewReader(largeBody))
		req.Header.Set("Content-Type", "application/octet-stream")
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	})

	t.Run("Blob_meta_not_found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/blobs/temp/nonexistent/meta", nil)
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("X-Blob-TTL_invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/ttl-invalid", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("X-Blob-TTL", "invalid")
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("X-Blob-TTL_valid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/ttl-valid", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("X-Blob-TTL", "3600")
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Blob_PUT_empty_body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/empty-body", bytes.NewReader([]byte{}))
		req.Header.Set("Content-Type", "text/plain")
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Blob_get_not_found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/blobs/temp/nonexistent", nil)
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Invalid_namespace", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/../ns1/b1", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Blob_id_invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/ns1/../b1", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Namespace_delete_deletes_all_blobs_in_namespace", func(t *testing.T) {
		// First create some blobs in the namespace
		content := []byte("blob-data")
		reqPut1 := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/scratch/del-b1", bytes.NewReader(content))
		reqPut1.Header.Set("Content-Type", "text/plain")
		reqPut1 = reqPut1.WithContext(context.WithValue(reqPut1.Context(), constants.ContextKeyUserID, "user-1"))
		rrPut1 := httptest.NewRecorder()
		dbController.handleBlob(rrPut1, reqPut1)
		assert.Equal(t, http.StatusOK, rrPut1.Code)

		reqPut2 := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/scratch/del-b2", bytes.NewReader(content))
		reqPut2.Header.Set("Content-Type", "text/plain")
		reqPut2 = reqPut2.WithContext(context.WithValue(reqPut2.Context(), constants.ContextKeyUserID, "user-1"))
		rrPut2 := httptest.NewRecorder()
		dbController.handleBlob(rrPut2, reqPut2)
		assert.Equal(t, http.StatusOK, rrPut2.Code)

		// Delete the namespace
		reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/blobs/scratch", nil)
		reqDel = reqDel.WithContext(context.WithValue(reqDel.Context(), constants.ContextKeyUserID, "user-1"))
		rrDel := httptest.NewRecorder()
		dbController.handleBlob(rrDel, reqDel)
		assert.Equal(t, http.StatusOK, rrDel.Code)

		var resp models.BlobDeleteResponse
		err := json.Unmarshal(rrDel.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, int64(2), resp.Deleted)
	})

	t.Run("Missing_Content-Type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/missing-ct", bytes.NewReader([]byte("data")))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Governance: Non-allowlisted namespace rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/governed-ns/b1", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusConflict, rr.Code)
		assert.Contains(t, rr.Body.String(), "submit via POST /api/v1/governance/envelopes")
	})

	t.Run("Governance: Allowlisted namespace accepted without identity", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/b1", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		// Should fail with 403 due to missing identity (ownership check)
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "unauthorized")
	})

	t.Run("Governance: Cross-namespace ownership rejected", func(t *testing.T) {
		// Set up context with user_id
		// Use a non-allowlisted namespace
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/governed-ns/b1", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusConflict, rr.Code) // Non-allowlisted namespace
		assert.Contains(t, rr.Body.String(), "submit via POST /api/v1/governance/envelopes")
	})

	t.Run("Governance: Delete non-allowlisted namespace rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/blobs/governed-ns", nil)
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusConflict, rr.Code)
		assert.Contains(t, rr.Body.String(), "submit via POST /api/v1/governance/envelopes")
	})
}

func TestDBControllerHandlePubSubPublish(t *testing.T) {
	dbController, _ := setupTestDBController(t)

	t.Run("Publish valid", func(t *testing.T) {
		pubReq := models.PubSubPublishRequest{
			Channel: pubsub.ResultsChannel("op-1", "session-1"),
			Data:    mustDocJSON(t, map[string]string{"foo": "bar"}),
		}
		body := mustMarshalJSON(t, pubReq)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pubsub/publish", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		dbController.handlePubSubPublish(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"receivers":0`)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pubsub/publish", nil)
		rr := httptest.NewRecorder()
		dbController.handlePubSubPublish(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pubsub/publish", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()
		dbController.handlePubSubPublish(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Reject mutation channels", func(t *testing.T) {
		for _, channel := range []string{pubsub.CmdChannel("op-1", "session-1"), "auditor:op-1:sessions-1"} {
			pubReq := models.PubSubPublishRequest{
				Channel: channel,
				Data:    mustDocJSON(t, map[string]string{"foo": "bar"}),
			}
			body := mustMarshalJSON(t, pubReq)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/pubsub/publish", bytes.NewReader(body))
			rr := httptest.NewRecorder()
			dbController.handlePubSubPublish(rr, req)
			assert.Equal(t, http.StatusConflict, rr.Code, "channel=%s", channel)
			assert.JSONEq(t, `{"error":"submit via POST /api/v1/governance/envelopes"}`, rr.Body.String())
		}
	})
}

func TestDBControllerHandleRevokeApp(t *testing.T) {
	_, stores := setupTestDBController(t)
	userSvc := NewUserService(stores.DocStore, testutil.NewTestLogger())
	logger := testutil.NewTestLogger()
	cfg := testutil.NewTestConfig(t)
	resp := response.NewWriter(logger)
	adminController := newAdminController(cfg, logger, stores.DocStore, stores.SignerStore, stores.TribunalStore, userSvc, resp)

	bootstrapUser, err := userSvc.CreateBootstrapUserWithOSUser(nil)
	require.NoError(t, err)
	require.NotNil(t, bootstrapUser)
	t.Cleanup(func() { stores.DocStore.DocDelete("users", bootstrapUser.ID) })

	regularUser, err := userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, regularUser)
	t.Cleanup(func() { stores.DocStore.DocDelete("users", regularUser.ID) })

	t.Run("reject app revocation without admin authorization", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-no-auth"

		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes := mustMarshalJSON(t, policy)
		err := stores.DocStore.DocSet("app_policies", appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { stores.DocStore.DocDelete("app_policies", appID) })

		reqBody := map[string]string{"app_id": appID}
		bodyBytes := mustMarshalJSON(t, reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/apps/revoke", bytes.NewReader(bodyBytes))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, regularUser.ID))

		rr := httptest.NewRecorder()
		adminController.handleRevokeApp(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "admin-only")
	})

	t.Run("reject app revocation without user context", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-no-context"

		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes := mustMarshalJSON(t, policy)
		err := stores.DocStore.DocSet("app_policies", appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { stores.DocStore.DocDelete("app_policies", appID) })

		reqBody := map[string]string{"app_id": appID}
		bodyBytes := mustMarshalJSON(t, reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/apps/revoke", bytes.NewReader(bodyBytes))

		rr := httptest.NewRecorder()
		adminController.handleRevokeApp(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "unauthorized")
	})

	t.Run("reject app revocation with missing app_id", func(t *testing.T) {
		reqBody := map[string]string{}
		bodyBytes := mustMarshalJSON(t, reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/apps/revoke", bytes.NewReader(bodyBytes))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, bootstrapUser.ID))

		rr := httptest.NewRecorder()
		adminController.handleRevokeApp(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "app_id required")
	})

	t.Run("successfully revoke app with policy only", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-revoke-policy-only"

		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes := mustMarshalJSON(t, policy)
		err := stores.DocStore.DocSet("app_policies", appID, policyBytes)
		require.NoError(t, err)

		policyDoc, err := stores.DocStore.DocGet("app_policies", appID)
		require.NoError(t, err)
		require.NotNil(t, policyDoc)

		reqBody := map[string]string{"app_id": appID}
		bodyBytes := mustMarshalJSON(t, reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/apps/revoke", bytes.NewReader(bodyBytes))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, bootstrapUser.ID))

		rr := httptest.NewRecorder()
		adminController.handleRevokeApp(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		policyDoc, err = stores.DocStore.DocGet("app_policies", appID)
		require.NoError(t, err)
		assert.Nil(t, policyDoc)
	})

	t.Run("successfully revoke app with policy and signer", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-revoke-with-signer"

		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes := mustMarshalJSON(t, policy)
		err := stores.DocStore.DocSet("app_policies", appID, policyBytes)
		require.NoError(t, err)

		signer := models.TrustedSigner{
			ID:        appID,
			PublicKey: strings.Repeat("a", 64),
			AddedAt:   time.Now().UTC(),
			Enabled:   true,
		}
		signerBytes := mustMarshalJSON(t, signer)
		err = stores.DocStore.DocSet("trusted_signers", appID, signerBytes)
		require.NoError(t, err)

		policyDoc, err := stores.DocStore.DocGet("app_policies", appID)
		require.NoError(t, err)
		require.NotNil(t, policyDoc)

		signerDoc, err := stores.DocStore.DocGet("trusted_signers", appID)
		require.NoError(t, err)
		require.NotNil(t, signerDoc)

		reqBody := map[string]string{"app_id": appID}
		bodyBytes := mustMarshalJSON(t, reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/apps/revoke", bytes.NewReader(bodyBytes))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, bootstrapUser.ID))

		rr := httptest.NewRecorder()
		adminController.handleRevokeApp(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		policyDoc, err = stores.DocStore.DocGet("app_policies", appID)
		require.NoError(t, err)
		assert.Nil(t, policyDoc)

		signerDoc, err = stores.DocStore.DocGet("trusted_signers", appID)
		require.NoError(t, err)
		assert.Nil(t, signerDoc)
	})

	t.Run("successfully revoke app with SPIFFE ID containing colons", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-mcp-client"

		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes := mustMarshalJSON(t, policy)
		err := stores.DocStore.DocSet("app_policies", appID, policyBytes)
		require.NoError(t, err)

		signer := models.TrustedSigner{
			ID:        appID,
			PublicKey: strings.Repeat("b", 64),
			AddedAt:   time.Now().UTC(),
			Enabled:   true,
		}
		signerBytes := mustMarshalJSON(t, signer)
		err = stores.DocStore.DocSet("trusted_signers", appID, signerBytes)
		require.NoError(t, err)

		reqBody := map[string]string{"app_id": appID}
		bodyBytes := mustMarshalJSON(t, reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/apps/revoke", bytes.NewReader(bodyBytes))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, bootstrapUser.ID))

		rr := httptest.NewRecorder()
		adminController.handleRevokeApp(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		policyDoc, err := stores.DocStore.DocGet("app_policies", appID)
		require.NoError(t, err)
		assert.Nil(t, policyDoc)

		signerDoc, err := stores.DocStore.DocGet("trusted_signers", appID)
		require.NoError(t, err)
		assert.Nil(t, signerDoc)
	})
}

// TestDBController_BlobNamespaceAllowlist verifies that blob namespace
// allowlist enforcement is working correctly.
// This is a regression test for Finding 4: Blob store bypasses governance.
func TestDBController_BlobNamespaceAllowlist(t *testing.T) {
	dbController, _ := setupTestDBController(t)

	t.Run("allowlisted namespace accepted", func(t *testing.T) {
		// Test that allowlisted namespaces are accepted
		allowlistedNamespaces := []string{"temp", "uploads", "cache", "scratch"}
		for _, ns := range allowlistedNamespaces {
			assert.True(t, blobNamespaceAllowed(ns), "Namespace %s should be allowlisted", ns)
		}
	})

	t.Run("non-allowlisted namespace rejected", func(t *testing.T) {
		// Test that non-allowlisted namespaces are rejected
		nonAllowlistedNamespaces := []string{"private", "secret", "config", "data"}
		for _, ns := range nonAllowlistedNamespaces {
			assert.False(t, blobNamespaceAllowed(ns), "Namespace %s should not be allowlisted", ns)
		}
	})

	t.Run("blob ownership verification - app identity", func(t *testing.T) {
		appID := "test-app-123"
		userID := "user-456"

		// Create a request with app identity
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/app/"+appID+"/test.txt", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyAppID, appID))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, userID))

		// App should be able to write to its own namespace
		err := dbController.verifyBlobOwnership(req, "app/"+appID)
		require.NoError(t, err, "App should be able to write to its own namespace")

		// App should not be able to write to another app's namespace
		err = dbController.verifyBlobOwnership(req, "app/other-app")
		require.Error(t, err, "App should not be able to write to another app's namespace")
	})

	t.Run("blob ownership verification - user identity", func(t *testing.T) {
		userID := "user-789"

		// Create a request with user identity
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/user/"+userID+"/test.txt", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, userID))

		// User should be able to write to their own namespace
		err := dbController.verifyBlobOwnership(req, "user/"+userID)
		require.NoError(t, err, "User should be able to write to their own namespace")

		// User should not be able to write to another user's namespace
		err = dbController.verifyBlobOwnership(req, "user/other-user")
		require.Error(t, err, "User should not be able to write to another user's namespace")
	})

	t.Run("blob ownership verification - allowlisted namespace", func(t *testing.T) {
		userID := "user-999"

		// Create a request with user identity
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/test.txt", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, userID))

		// Any authenticated identity should be able to write to allowlisted namespaces
		err := dbController.verifyBlobOwnership(req, "temp")
		require.NoError(t, err, "Authenticated identity should be able to write to allowlisted namespace")

		err = dbController.verifyBlobOwnership(req, "uploads")
		require.NoError(t, err, "Authenticated identity should be able to write to allowlisted namespace")
	})

	t.Run("blob ownership verification - no identity rejected", func(t *testing.T) {
		// Create a request with no identity
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/test.txt", nil)

		// Request without identity should be rejected
		err := dbController.verifyBlobOwnership(req, "temp")
		require.Error(t, err, "Request without identity should be rejected")
	})
}

func TestDBControllerHandleDataSettings(t *testing.T) {
	dbController, stores := setupTestDBController(t)

	t.Run("GET - success", func(t *testing.T) {
		// First create settings
		settings := map[string]string{"mode": "test"}
		err := stores.DocStore.DocSet("settings", "platform_settings", mustDocJSON(t, settings))
		require.NoError(t, err)
		t.Cleanup(func() { stores.DocStore.DocDelete("settings", "platform_settings") })

		req := httptest.NewRequest(http.MethodGet, "/api/v1/data/settings", nil)
		rr := httptest.NewRecorder()
		dbController.handleDataSettings(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "test")
	})

	t.Run("PUT - success", func(t *testing.T) {
		settings := map[string]string{"mode": "production"}
		req := httptest.NewRequest(http.MethodPut, "/api/v1/data/settings", bytes.NewReader(mustDocJSON(t, settings)))
		rr := httptest.NewRecorder()
		dbController.handleDataSettings(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		t.Cleanup(func() { stores.DocStore.DocDelete("settings", "platform_settings") })
	})

	t.Run("PATCH - success", func(t *testing.T) {
		// First create settings
		settings := map[string]string{"mode": "test"}
		err := stores.DocStore.DocSet("settings", "platform_settings", mustDocJSON(t, settings))
		require.NoError(t, err)

		patch := map[string]string{"mode": "production"}
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/data/settings", bytes.NewReader(mustDocJSON(t, patch)))
		rr := httptest.NewRecorder()
		dbController.handleDataSettings(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		t.Cleanup(func() { stores.DocStore.DocDelete("settings", "platform_settings") })
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/data/settings", nil)
		rr := httptest.NewRecorder()
		dbController.handleDataSettings(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/data/settings", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()
		dbController.handleDataSettings(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestDBControllerHandleAuditReceipts(t *testing.T) {
	dbController, _ := setupTestDBController(t)

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/receipts", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditReceipts(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("GET by tx_id - not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts?tx_id=nonexistent", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditReceipts(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("GET list - success with defaults", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditReceipts(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"success":true`)
	})

	t.Run("GET list - with operator_session_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts?operator_session_id=op-123", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditReceipts(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("GET list - with limit and offset", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts?limit=10&offset=5", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditReceipts(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

func TestDBControllerHandleAuditReceiptsExport(t *testing.T) {
	dbController, _ := setupTestDBController(t)

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/receipts/export", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditReceiptsExport(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("GET - success with defaults", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts/export", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditReceiptsExport(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "application/x-ndjson", rr.Header().Get("Content-Type"))
	})

	t.Run("GET - with since parameter RFC3339", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts/export?since=2026-01-01T00:00:00Z", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditReceiptsExport(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("GET - with since parameter timestamp", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts/export?since=1704067200000", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditReceiptsExport(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("GET - with limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/receipts/export?limit=50", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditReceiptsExport(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

func TestDBControllerHandleGovernanceSigners(t *testing.T) {
	dbController, stores := setupTestDBController(t)

	t.Run("GET - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/governance/signers", nil)
		rr := httptest.NewRecorder()
		dbController.handleGovernanceSigners(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"success":true`)
	})

	t.Run("POST - success", func(t *testing.T) {
		signer := models.TrustedSigner{
			ID:        "test-signer-1",
			PublicKey: strings.Repeat("a", 64),
			AddedAt:   time.Now().UTC(),
			Enabled:   true,
		}
		body := mustMarshalJSON(t, signer)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/governance/signers", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		dbController.handleGovernanceSigners(rr, req)
		assert.Equal(t, http.StatusCreated, rr.Code)
		t.Cleanup(func() { stores.DocStore.DocDelete("trusted_signers", "test-signer-1") })
	})

	t.Run("POST - missing id", func(t *testing.T) {
		signer := models.TrustedSigner{
			PublicKey: strings.Repeat("a", 64),
		}
		body := mustMarshalJSON(t, signer)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/governance/signers", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		dbController.handleGovernanceSigners(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "missing required field")
	})

	t.Run("POST - missing public_key", func(t *testing.T) {
		signer := models.TrustedSigner{
			ID: "test-signer-2",
		}
		body := mustMarshalJSON(t, signer)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/governance/signers", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		dbController.handleGovernanceSigners(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "missing required field")
	})

	t.Run("POST - invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/governance/signers", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()
		dbController.handleGovernanceSigners(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/governance/signers", nil)
		rr := httptest.NewRecorder()
		dbController.handleGovernanceSigners(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestDBControllerHandleGovernanceSignerByID(t *testing.T) {
	dbController, stores := setupTestDBController(t)

	t.Run("GET - not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/governance/signers/nonexistent", nil)
		rr := httptest.NewRecorder()
		dbController.handleGovernanceSignerByID(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("GET - success", func(t *testing.T) {
		// First create a signer
		signer := models.TrustedSigner{
			ID:        "test-signer-get",
			PublicKey: strings.Repeat("b", 64),
			AddedAt:   time.Now().UTC(),
			Enabled:   true,
		}
		signerBytes := mustMarshalJSON(t, signer)
		err := stores.DocStore.DocSet("trusted_signers", "test-signer-get", signerBytes)
		require.NoError(t, err)
		t.Cleanup(func() { stores.DocStore.DocDelete("trusted_signers", "test-signer-get") })

		req := httptest.NewRequest(http.MethodGet, "/api/v1/governance/signers/test-signer-get", nil)
		rr := httptest.NewRecorder()
		dbController.handleGovernanceSignerByID(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "test-signer-get")
	})

	t.Run("DELETE - success", func(t *testing.T) {
		// First create a signer
		signer := models.TrustedSigner{
			ID:        "test-signer-delete",
			PublicKey: strings.Repeat("c", 64),
			AddedAt:   time.Now().UTC(),
			Enabled:   true,
		}
		signerBytes := mustMarshalJSON(t, signer)
		err := stores.DocStore.DocSet("trusted_signers", "test-signer-delete", signerBytes)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/governance/signers/test-signer-delete", nil)
		rr := httptest.NewRecorder()
		dbController.handleGovernanceSignerByID(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("DELETE - not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/governance/signers/nonexistent", nil)
		rr := httptest.NewRecorder()
		dbController.handleGovernanceSignerByID(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Invalid signer id - empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/governance/signers/", nil)
		rr := httptest.NewRecorder()
		dbController.handleGovernanceSignerByID(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Invalid signer id - contains slash", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/governance/signers/invalid/id", nil)
		rr := httptest.NewRecorder()
		dbController.handleGovernanceSignerByID(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/governance/signers/test-id", nil)
		rr := httptest.NewRecorder()
		dbController.handleGovernanceSignerByID(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestDBControllerHandleAuditEvents(t *testing.T) {
	dbController, _ := setupTestDBController(t)

	t.Run("Failure - method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/events", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditEvents(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Success - empty events", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditEvents(rr, req)
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
		dbController.handleAuditEvents(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
	})

	t.Run("Success - with limit and offset", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events?limit=10&offset=5", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditEvents(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
	})
}

func TestDBControllerHandleAuditSummary(t *testing.T) {
	dbController, _ := setupTestDBController(t)

	t.Run("Failure - method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/summary", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditSummary(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Success - empty summary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/summary", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditSummary(rr, req)
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
		dbController.handleAuditSummary(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.Contains(t, resp, "events_summary")
		assert.Contains(t, resp, "receipts_summary")
	})
}

func TestDBControllerHandleAuditReport(t *testing.T) {
	dbController, _ := setupTestDBController(t)

	t.Run("Failure - method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/report", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditReport(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Success - empty report", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/report", nil)
		rr := httptest.NewRecorder()
		dbController.handleAuditReport(rr, req)
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
		dbController.handleAuditReport(rr, req)
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
