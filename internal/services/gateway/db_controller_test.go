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
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDBController(t *testing.T) (*DBController, *GatewayDBService) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)

	dbController := newDBController(infra.Cfg, infra.Logger, infra.DB, infra.Auth, infra.Pubsub, infra.UserSvc, infra.Responder)

	return dbController, infra.DB
}

func TestDBControllerHandleDB(t *testing.T) {
	dbController, db := setupTestDBController(t)

	t.Run("BadRequest - no collection", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/db/", nil)
		rr := httptest.NewRecorder()
		dbController.handleDB(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - no ID", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/db/users/", nil)
		rr := httptest.NewRecorder()
		dbController.handleDB(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("PUT and GET", func(t *testing.T) {
		data := map[string]string{"name": "alice"}
		reqPut := httptest.NewRequest(http.MethodPut, "/db/settings/u1", bytes.NewReader(mustDocJSON(t, data)))
		rrPut := httptest.NewRecorder()
		dbController.handleDB(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/db/settings/u1", nil)
		rrGet := httptest.NewRecorder()
		dbController.handleDB(rrGet, reqGet)
		assert.Equal(t, http.StatusOK, rrGet.Code)

		var doc map[string]interface{}
		err := json.Unmarshal(rrGet.Body.Bytes(), &doc)
		require.NoError(t, err)
		assert.Equal(t, "alice", doc["name"])
	})

	t.Run("PATCH", func(t *testing.T) {
		patch := map[string]string{"role": "admin"}
		reqPatch := httptest.NewRequest(http.MethodPatch, "/db/settings/u1", bytes.NewReader(mustDocJSON(t, patch)))
		rrPatch := httptest.NewRecorder()
		dbController.handleDB(rrPatch, reqPatch)
		assert.Equal(t, http.StatusOK, rrPatch.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/db/settings/u1", nil)
		rrGet := httptest.NewRecorder()
		dbController.handleDB(rrGet, reqGet)
		assert.Equal(t, http.StatusOK, rrGet.Code)

		var doc map[string]interface{}
		err := json.Unmarshal(rrGet.Body.Bytes(), &doc)
		require.NoError(t, err)
		assert.Equal(t, "alice", doc["name"])
		assert.Equal(t, "admin", doc["role"])
	})

	t.Run("DELETE", func(t *testing.T) {
		reqDel := httptest.NewRequest(http.MethodDelete, "/db/settings/u1", nil)
		rrDel := httptest.NewRecorder()
		dbController.handleDB(rrDel, reqDel)
		assert.Equal(t, http.StatusOK, rrDel.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/db/settings/u1", nil)
		rrGet := httptest.NewRecorder()
		dbController.handleDB(rrGet, reqGet)
		assert.Equal(t, http.StatusNotFound, rrGet.Code)
	})

	t.Run("Query", func(t *testing.T) {
		db.DocSet("items", "i1", mustDocJSON(t, map[string]int{"val": 10}))
		db.DocSet("items", "i2", mustDocJSON(t, map[string]int{"val": 20}))

		query := models.DocQueryRequest{
			Limit: 1,
		}
		body, _ := json.Marshal(query)
		reqQuery := httptest.NewRequest(http.MethodPost, "/db/items/_query", bytes.NewReader(body))
		rrQuery := httptest.NewRecorder()
		dbController.handleDB(rrQuery, reqQuery)
		assert.Equal(t, http.StatusOK, rrQuery.Code)

		var results []map[string]interface{}
		err := json.Unmarshal(rrQuery.Body.Bytes(), &results)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/db/settings/u1", strings.NewReader("{invalid-json}"))
		rr := httptest.NewRecorder()
		dbController.handleDB(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"invalid JSON body"}`, rr.Body.String())
	})

	t.Run("PATCH not found", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPatch, "/db/settings/nonexistent", bytes.NewReader(mustDocJSON(t, map[string]string{"foo": "bar"})))
		rr := httptest.NewRecorder()
		dbController.handleDB(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("DELETE not found", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodDelete, "/db/users/nonexistent", nil)
		rr := httptest.NewRecorder()
		dbController.handleDB(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/db/users/u1", nil)
		rr := httptest.NewRecorder()
		dbController.handleDB(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Non-bootstrap mutations redirect to governance envelope", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			method string
			body   []byte
		}{
			{method: http.MethodPut, body: mustDocJSON(t, map[string]string{"name": "alice"})},
			{method: http.MethodPatch, body: mustDocJSON(t, map[string]string{"role": "admin"})},
			{method: http.MethodDelete},
		}

		for _, tt := range tests {
			req := httptest.NewRequest(tt.method, "/db/items/i1", bytes.NewReader(tt.body))
			rr := httptest.NewRecorder()
			dbController.handleDB(rr, req)
			assert.Equal(t, http.StatusConflict, rr.Code, "method=%s", tt.method)
			assert.JSONEq(t, `{"error":"submit via POST /api/governance/envelope"}`, rr.Body.String())
		}
	})

	t.Run("Query validation", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/db/items/_query", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()
		dbController.handleDBQuery(rr, req, "items")
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("SSE Events count", func(t *testing.T) {
		t.Parallel()
		db.SSEEventsAppend(SSERoute{WebSessionID: "s1"}, "T", "{}", "")
		req := httptest.NewRequest(http.MethodGet, "/db/_sse_events/count", nil)
		rr := httptest.NewRecorder()
		dbController.handleSSEEvents(rr, req, "count")
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("SSE Events wipe", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodDelete, "/db/_sse_events", nil)
		rr := httptest.NewRecorder()
		dbController.handleSSEEvents(rr, req, "")
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("SSE Events invalid", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/db/_sse_events/invalid", nil)
		rr := httptest.NewRecorder()
		dbController.handleSSEEvents(rr, req, "invalid")
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestDBControllerHandleKV(t *testing.T) {
	dbController, db := setupTestDBController(t)

	t.Run("PUT and GET", func(t *testing.T) {
		reqPut := httptest.NewRequest(http.MethodPut, "/kv/k1", bytes.NewReader(mustDocJSON(t, models.KVSetRequest{Value: "g8e"})))
		rrPut := httptest.NewRecorder()
		dbController.handleKV(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/kv/k1", nil)
		rrGet := httptest.NewRecorder()
		dbController.handleKV(rrGet, reqGet)
		assert.Equal(t, http.StatusOK, rrGet.Code)
		assert.Contains(t, rrGet.Body.String(), `"value":"g8e"`)
	})

	t.Run("TTL and Expire", func(t *testing.T) {
		reqTtl := httptest.NewRequest(http.MethodGet, "/kv/k1/_ttl", nil)
		rrTtl := httptest.NewRecorder()
		dbController.handleKV(rrTtl, reqTtl)
		assert.Equal(t, http.StatusOK, rrTtl.Code)

		reqExp := httptest.NewRequest(http.MethodPut, "/kv/k1/_expire", bytes.NewReader(mustDocJSON(t, models.KVExpireRequest{TTL: 100})))
		rrExp := httptest.NewRecorder()
		dbController.handleKV(rrExp, reqExp)
		assert.Equal(t, http.StatusOK, rrExp.Code)
	})

	t.Run("Scan and DeletePattern", func(t *testing.T) {
		db.KVSet("pref:1", "a", 0)
		db.KVSet("pref:2", "b", 0)

		reqScan := httptest.NewRequest(http.MethodPost, "/kv/_scan", bytes.NewReader(mustDocJSON(t, models.KVPatternRequest{Pattern: "pref:*"})))
		rrScan := httptest.NewRecorder()
		dbController.handleKV(rrScan, reqScan)
		assert.Equal(t, http.StatusOK, rrScan.Code)
		assert.Contains(t, rrScan.Body.String(), "pref:1")

		reqDel := httptest.NewRequest(http.MethodPost, "/kv/_delete_pattern", bytes.NewReader(mustDocJSON(t, models.KVPatternRequest{Pattern: "pref:*"})))
		rrDel := httptest.NewRecorder()
		dbController.handleKV(rrDel, reqDel)
		assert.Equal(t, http.StatusOK, rrDel.Code)
		assert.Contains(t, rrDel.Body.String(), `"deleted":2`)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/kv/k1", strings.NewReader("{invalid-json}"))
		rr := httptest.NewRecorder()
		dbController.handleKV(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("TTL required for expire", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/kv/k1/_expire", strings.NewReader(`{"ttl":0}`))
		rr := httptest.NewRecorder()
		dbController.handleKV(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Keys", func(t *testing.T) {
		t.Parallel()
		db.KVSet("key1", "val1", 0)
		req := httptest.NewRequest(http.MethodPost, "/kv/_keys", strings.NewReader(`{"pattern":"key*"}`))
		rr := httptest.NewRecorder()
		dbController.handleKVKeys(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "key1")
	})

	t.Run("KV Keys Invalid JSON", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/kv/_keys", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		dbController.handleKVKeys(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Scan Invalid JSON", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/kv/_scan", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		dbController.handleKVScan(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Delete Pattern Missing Pattern", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/kv/_delete_pattern", strings.NewReader(`{}`))
		rr := httptest.NewRecorder()
		dbController.handleKVDeletePattern(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Delete Pattern Invalid JSON", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/kv/_delete_pattern", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		dbController.handleKVDeletePattern(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/kv/k1", strings.NewReader(`{"value":"x"}`))
		rr := httptest.NewRecorder()
		dbController.handleKV(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestDBControllerHandleBlob(t *testing.T) {
	dbController, _ := setupTestDBController(t)

	t.Run("PUT and GET", func(t *testing.T) {
		t.Parallel()
		content := []byte("blob-data")
		reqPut := httptest.NewRequest(http.MethodPut, "/blob/temp/putget-b1", bytes.NewReader(content))
		reqPut.Header.Set("Content-Type", "text/plain")
		reqPut = reqPut.WithContext(context.WithValue(reqPut.Context(), userIDKey, "user-1"))
		rrPut := httptest.NewRecorder()
		dbController.handleBlob(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/blob/temp/putget-b1", nil)
		rrGet := httptest.NewRecorder()
		dbController.handleBlob(rrGet, reqGet)
		assert.Equal(t, http.StatusOK, rrGet.Code)
		assert.Equal(t, content, rrGet.Body.Bytes())
		assert.Equal(t, "text/plain", rrGet.Header().Get("Content-Type"))
	})

	t.Run("Metadata", func(t *testing.T) {
		t.Parallel()
		content := []byte("blob-data")
		reqPut := httptest.NewRequest(http.MethodPut, "/blob/cache/meta-b2", bytes.NewReader(content))
		reqPut.Header.Set("Content-Type", "text/plain")
		reqPut = reqPut.WithContext(context.WithValue(reqPut.Context(), userIDKey, "user-1"))
		rrPut := httptest.NewRecorder()
		dbController.handleBlob(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqMeta := httptest.NewRequest(http.MethodGet, "/blob/cache/meta-b2/meta", nil)
		rrMeta := httptest.NewRecorder()
		dbController.handleBlob(rrMeta, reqMeta)
		assert.Equal(t, http.StatusOK, rrMeta.Code)
		assert.Contains(t, rrMeta.Body.String(), `"id":"meta-b2"`)
	})

	t.Run("Too Large", func(t *testing.T) {
		t.Parallel()
		largeBody := make([]byte, maxBlobBodySize+1)
		req := httptest.NewRequest(http.MethodPut, "/blob/temp/large-test", bytes.NewReader(largeBody))
		req.Header.Set("Content-Type", "application/octet-stream")
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, "user-1"))
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	})

	t.Run("Blob_meta_not_found", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/blob/temp/nonexistent/meta", nil)
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("X-Blob-TTL_invalid", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/blob/temp/ttl-invalid", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("X-Blob-TTL", "invalid")
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, "user-1"))
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("X-Blob-TTL_valid", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/blob/temp/ttl-valid", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("X-Blob-TTL", "3600")
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, "user-1"))
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Blob_PUT_empty_body", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/blob/temp/empty-body", bytes.NewReader([]byte{}))
		req.Header.Set("Content-Type", "text/plain")
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, "user-1"))
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Blob_get_not_found", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/blob/temp/nonexistent", nil)
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Invalid_namespace", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/blob/../ns1/b1", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Blob_id_invalid", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/blob/ns1/../b1", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Namespace_delete_deletes_all_blobs_in_namespace", func(t *testing.T) {
		t.Parallel()
		// First create some blobs in the namespace
		content := []byte("blob-data")
		reqPut1 := httptest.NewRequest(http.MethodPut, "/blob/scratch/del-b1", bytes.NewReader(content))
		reqPut1.Header.Set("Content-Type", "text/plain")
		reqPut1 = reqPut1.WithContext(context.WithValue(reqPut1.Context(), userIDKey, "user-1"))
		rrPut1 := httptest.NewRecorder()
		dbController.handleBlob(rrPut1, reqPut1)
		assert.Equal(t, http.StatusOK, rrPut1.Code)

		reqPut2 := httptest.NewRequest(http.MethodPut, "/blob/scratch/del-b2", bytes.NewReader(content))
		reqPut2.Header.Set("Content-Type", "text/plain")
		reqPut2 = reqPut2.WithContext(context.WithValue(reqPut2.Context(), userIDKey, "user-1"))
		rrPut2 := httptest.NewRecorder()
		dbController.handleBlob(rrPut2, reqPut2)
		assert.Equal(t, http.StatusOK, rrPut2.Code)

		// Delete the namespace
		reqDel := httptest.NewRequest(http.MethodDelete, "/blob/scratch", nil)
		reqDel = reqDel.WithContext(context.WithValue(reqDel.Context(), userIDKey, "user-1"))
		rrDel := httptest.NewRecorder()
		dbController.handleBlob(rrDel, reqDel)
		assert.Equal(t, http.StatusOK, rrDel.Code)

		var resp models.BlobDeleteResponse
		err := json.Unmarshal(rrDel.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, int64(2), resp.Deleted)
	})

	t.Run("Missing_Content-Type", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/blob/temp/missing-ct", bytes.NewReader([]byte("data")))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, "user-1"))
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Governance: Non-allowlisted namespace rejected", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/blob/governed-ns/b1", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusConflict, rr.Code)
		assert.Contains(t, rr.Body.String(), "submit via POST /api/governance/envelope")
	})

	t.Run("Governance: Allowlisted namespace accepted without identity", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/blob/temp/b1", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		// Should fail with 403 due to missing identity (ownership check)
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "unauthorized")
	})

	t.Run("Governance: Cross-namespace ownership rejected", func(t *testing.T) {
		t.Parallel()
		// Set up context with user_id
		// Use a non-allowlisted namespace
		req := httptest.NewRequest(http.MethodPut, "/blob/governed-ns/b1", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, "user-1"))
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusConflict, rr.Code) // Non-allowlisted namespace
		assert.Contains(t, rr.Body.String(), "submit via POST /api/governance/envelope")
	})

	t.Run("Governance: Delete non-allowlisted namespace rejected", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodDelete, "/blob/governed-ns", nil)
		rr := httptest.NewRecorder()
		dbController.handleBlob(rr, req)
		assert.Equal(t, http.StatusConflict, rr.Code)
		assert.Contains(t, rr.Body.String(), "submit via POST /api/governance/envelope")
	})
}

func TestDBControllerHandlePubSubPublish(t *testing.T) {
	dbController, _ := setupTestDBController(t)

	t.Run("Publish valid", func(t *testing.T) {
		t.Parallel()
		pubReq := models.PubSubPublishRequest{
			Channel: constants.ResultsChannel("op-1", "session-1"),
			Data:    mustDocJSON(t, map[string]string{"foo": "bar"}),
		}
		body, _ := json.Marshal(pubReq)
		req := httptest.NewRequest(http.MethodPost, "/pubsub/publish", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		dbController.handlePubSubPublish(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"receivers":0`)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/pubsub/publish", nil)
		rr := httptest.NewRecorder()
		dbController.handlePubSubPublish(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/pubsub/publish", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()
		dbController.handlePubSubPublish(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Reject mutation channels", func(t *testing.T) {
		t.Parallel()
		for _, channel := range []string{constants.CmdChannel("op-1", "session-1"), "auditor:op-1:session-1"} {
			pubReq := models.PubSubPublishRequest{
				Channel: channel,
				Data:    mustDocJSON(t, map[string]string{"foo": "bar"}),
			}
			body, _ := json.Marshal(pubReq)
			req := httptest.NewRequest(http.MethodPost, "/pubsub/publish", bytes.NewReader(body))
			rr := httptest.NewRecorder()
			dbController.handlePubSubPublish(rr, req)
			assert.Equal(t, http.StatusConflict, rr.Code, "channel=%s", channel)
			assert.JSONEq(t, `{"error":"submit via POST /api/governance/envelope"}`, rr.Body.String())
		}
	})
}

func TestDBControllerHandleAppPolicySigner(t *testing.T) {
	dbController, db := setupTestDBController(t)
	userSvc := NewUserService(db, testutil.NewTestLogger())

	bootstrapUser, err := userSvc.CreateBootstrapUser()
	require.NoError(t, err)
	require.NotNil(t, bootstrapUser)
	t.Cleanup(func() { db.DocDelete("users", bootstrapUser.ID) })

	regularUser, err := userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, regularUser)
	t.Cleanup(func() { db.DocDelete("users", regularUser.ID) })

	t.Run("reject signer registration without admin authorization", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-no-auth"
		pubKeyHex := "a" + strings.Repeat("0", 63)

		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet("app_policies", appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { db.DocDelete("app_policies", appID) })

		reqBody := map[string]string{"public_key_hex": pubKeyHex}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/app-policies/"+appID+"/signer", bytes.NewReader(bodyBytes))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, regularUser.ID))

		rr := httptest.NewRecorder()
		dbController.handleAppPolicySigner(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "admin-only")
	})

	t.Run("reject signer registration without user context", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-no-context"
		pubKeyHex := "a" + strings.Repeat("0", 63)

		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet("app_policies", appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { db.DocDelete("app_policies", appID) })

		reqBody := map[string]string{"public_key_hex": pubKeyHex}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/app-policies/"+appID+"/signer", bytes.NewReader(bodyBytes))

		rr := httptest.NewRecorder()
		dbController.handleAppPolicySigner(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "unauthorized")
	})

	t.Run("reject signer registration without AppPolicy", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-no-policy"
		pubKeyHex := "a" + strings.Repeat("0", 63)

		reqBody := map[string]string{"public_key_hex": pubKeyHex}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/app-policies/"+appID+"/signer", bytes.NewReader(bodyBytes))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		rr := httptest.NewRecorder()
		dbController.handleAppPolicySigner(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "app policy not found")
	})

	t.Run("reject signer registration with invalid public key size", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-invalid-key"

		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet("app_policies", appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { db.DocDelete("app_policies", appID) })

		pubKeyHex := "a" + strings.Repeat("0", 30)

		reqBody := map[string]string{"public_key_hex": pubKeyHex}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/app-policies/"+appID+"/signer", bytes.NewReader(bodyBytes))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		rr := httptest.NewRecorder()
		dbController.handleAppPolicySigner(rr, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "invalid hex")
	})

	t.Run("successfully register signer with valid AppPolicy", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-valid-signer"

		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet("app_policies", appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { db.DocDelete("app_policies", appID) })

		pubKeyHex := strings.Repeat("0", 64)

		reqBody := map[string]string{"public_key_hex": pubKeyHex}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/app-policies/"+appID+"/signer", bytes.NewReader(bodyBytes))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		rr := httptest.NewRecorder()
		dbController.handleAppPolicySigner(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)

		signerDoc, err := db.DocGet("trusted_signers", appID)
		require.NoError(t, err)
		require.NotNil(t, signerDoc)
		assert.Equal(t, appID, signerDoc.ID)

		var signer models.TrustedSigner
		signerData, _ := json.Marshal(signerDoc.Data)
		err = json.Unmarshal(signerData, &signer)
		require.NoError(t, err)
		assert.Equal(t, pubKeyHex, signer.PublicKey)
		assert.True(t, signer.Enabled)

		t.Cleanup(func() { db.DocDelete("trusted_signers", appID) })
	})

	t.Run("successfully register signer with SPIFFE ID containing colons", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-mcp-client"

		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet("app_policies", appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { db.DocDelete("app_policies", appID) })

		pubKeyHex := strings.Repeat("a", 64)

		reqBody := map[string]string{"public_key_hex": pubKeyHex}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/app-policies/"+appID+"/signer", bytes.NewReader(bodyBytes))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		rr := httptest.NewRecorder()
		dbController.handleAppPolicySigner(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)

		signerDoc, err := db.DocGet("trusted_signers", appID)
		require.NoError(t, err)
		require.NotNil(t, signerDoc)
		assert.Equal(t, appID, signerDoc.ID)

		var signer models.TrustedSigner
		signerData, _ := json.Marshal(signerDoc.Data)
		err = json.Unmarshal(signerData, &signer)
		require.NoError(t, err)
		assert.Equal(t, pubKeyHex, signer.PublicKey)
		assert.True(t, signer.Enabled)

		t.Cleanup(func() { db.DocDelete("trusted_signers", appID) })
	})
}

func TestDBControllerHandleRevokeApp(t *testing.T) {
	dbController, db := setupTestDBController(t)
	userSvc := NewUserService(db, testutil.NewTestLogger())

	bootstrapUser, err := userSvc.CreateBootstrapUser()
	require.NoError(t, err)
	require.NotNil(t, bootstrapUser)
	t.Cleanup(func() { db.DocDelete("users", bootstrapUser.ID) })

	regularUser, err := userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, regularUser)
	t.Cleanup(func() { db.DocDelete("users", regularUser.ID) })

	t.Run("reject app revocation without admin authorization", func(t *testing.T) {
		appID := "spiffe://g8e.local/app/test-no-auth"

		policy := models.AppPolicy{
			AppID:              appID,
			AllowedCollections: []string{"test_collection"},
			AllowedIntents:     []string{"read"},
			CreatedAt:          time.Now().UTC(),
			UpdatedAt:          time.Now().UTC(),
		}
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet("app_policies", appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { db.DocDelete("app_policies", appID) })

		reqBody := map[string]string{"app_id": appID}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/revoke-app", bytes.NewReader(bodyBytes))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, regularUser.ID))

		rr := httptest.NewRecorder()
		dbController.handleRevokeApp(rr, req)

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
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet("app_policies", appID, policyBytes)
		require.NoError(t, err)
		t.Cleanup(func() { db.DocDelete("app_policies", appID) })

		reqBody := map[string]string{"app_id": appID}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/revoke-app", bytes.NewReader(bodyBytes))

		rr := httptest.NewRecorder()
		dbController.handleRevokeApp(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "unauthorized")
	})

	t.Run("reject app revocation with missing app_id", func(t *testing.T) {
		reqBody := map[string]string{}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/revoke-app", bytes.NewReader(bodyBytes))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		rr := httptest.NewRecorder()
		dbController.handleRevokeApp(rr, req)

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
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet("app_policies", appID, policyBytes)
		require.NoError(t, err)

		policyDoc, err := db.DocGet("app_policies", appID)
		require.NoError(t, err)
		require.NotNil(t, policyDoc)

		reqBody := map[string]string{"app_id": appID}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/revoke-app", bytes.NewReader(bodyBytes))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		rr := httptest.NewRecorder()
		dbController.handleRevokeApp(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		policyDoc, err = db.DocGet("app_policies", appID)
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
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet("app_policies", appID, policyBytes)
		require.NoError(t, err)

		signer := models.TrustedSigner{
			ID:        appID,
			PublicKey: strings.Repeat("a", 64),
			AddedAt:   time.Now().UTC(),
			Enabled:   true,
		}
		signerBytes, _ := json.Marshal(signer)
		err = db.DocSet("trusted_signers", appID, signerBytes)
		require.NoError(t, err)

		policyDoc, err := db.DocGet("app_policies", appID)
		require.NoError(t, err)
		require.NotNil(t, policyDoc)

		signerDoc, err := db.DocGet("trusted_signers", appID)
		require.NoError(t, err)
		require.NotNil(t, signerDoc)

		reqBody := map[string]string{"app_id": appID}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/revoke-app", bytes.NewReader(bodyBytes))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		rr := httptest.NewRecorder()
		dbController.handleRevokeApp(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		policyDoc, err = db.DocGet("app_policies", appID)
		require.NoError(t, err)
		assert.Nil(t, policyDoc)

		signerDoc, err = db.DocGet("trusted_signers", appID)
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
		policyBytes, _ := json.Marshal(policy)
		err := db.DocSet("app_policies", appID, policyBytes)
		require.NoError(t, err)

		signer := models.TrustedSigner{
			ID:        appID,
			PublicKey: strings.Repeat("b", 64),
			AddedAt:   time.Now().UTC(),
			Enabled:   true,
		}
		signerBytes, _ := json.Marshal(signer)
		err = db.DocSet("trusted_signers", appID, signerBytes)
		require.NoError(t, err)

		reqBody := map[string]string{"app_id": appID}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/revoke-app", bytes.NewReader(bodyBytes))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, bootstrapUser.ID))

		rr := httptest.NewRecorder()
		dbController.handleRevokeApp(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		policyDoc, err := db.DocGet("app_policies", appID)
		require.NoError(t, err)
		assert.Nil(t, policyDoc)

		signerDoc, err := db.DocGet("trusted_signers", appID)
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
		t.Parallel()
		// Test that allowlisted namespaces are accepted
		allowlistedNamespaces := []string{"temp", "uploads", "cache", "scratch"}
		for _, ns := range allowlistedNamespaces {
			assert.True(t, blobNamespaceAllowed(ns), "Namespace %s should be allowlisted", ns)
		}
	})

	t.Run("non-allowlisted namespace rejected", func(t *testing.T) {
		t.Parallel()
		// Test that non-allowlisted namespaces are rejected
		nonAllowlistedNamespaces := []string{"private", "secret", "config", "data"}
		for _, ns := range nonAllowlistedNamespaces {
			assert.False(t, blobNamespaceAllowed(ns), "Namespace %s should not be allowlisted", ns)
		}
	})

	t.Run("blob ownership verification - app identity", func(t *testing.T) {
		t.Parallel()
		appID := "test-app-123"
		userID := "user-456"

		// Create a request with app identity
		req := httptest.NewRequest(http.MethodPut, "/blob/app/"+appID+"/test.txt", nil)
		req = req.WithContext(context.WithValue(req.Context(), appIDKey, appID))
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, userID))

		// App should be able to write to its own namespace
		err := dbController.verifyBlobOwnership(req, "app/"+appID)
		assert.NoError(t, err, "App should be able to write to its own namespace")

		// App should not be able to write to another app's namespace
		err = dbController.verifyBlobOwnership(req, "app/other-app")
		assert.Error(t, err, "App should not be able to write to another app's namespace")
	})

	t.Run("blob ownership verification - user identity", func(t *testing.T) {
		t.Parallel()
		userID := "user-789"

		// Create a request with user identity
		req := httptest.NewRequest(http.MethodPut, "/blob/user/"+userID+"/test.txt", nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, userID))

		// User should be able to write to their own namespace
		err := dbController.verifyBlobOwnership(req, "user/"+userID)
		assert.NoError(t, err, "User should be able to write to their own namespace")

		// User should not be able to write to another user's namespace
		err = dbController.verifyBlobOwnership(req, "user/other-user")
		assert.Error(t, err, "User should not be able to write to another user's namespace")
	})

	t.Run("blob ownership verification - allowlisted namespace", func(t *testing.T) {
		t.Parallel()
		userID := "user-999"

		// Create a request with user identity
		req := httptest.NewRequest(http.MethodPut, "/blob/temp/test.txt", nil)
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, userID))

		// Any authenticated identity should be able to write to allowlisted namespaces
		err := dbController.verifyBlobOwnership(req, "temp")
		assert.NoError(t, err, "Authenticated identity should be able to write to allowlisted namespace")

		err = dbController.verifyBlobOwnership(req, "uploads")
		assert.NoError(t, err, "Authenticated identity should be able to write to allowlisted namespace")
	})

	t.Run("blob ownership verification - no identity rejected", func(t *testing.T) {
		t.Parallel()
		// Create a request with no identity
		req := httptest.NewRequest(http.MethodPut, "/blob/temp/test.txt", nil)

		// Request without identity should be rejected
		err := dbController.verifyBlobOwnership(req, "temp")
		assert.Error(t, err, "Request without identity should be rejected")
	})
}
