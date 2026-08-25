// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

func setupTestDataController(t *testing.T) (*DataController, *Stores) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)

	dataController := newDataController(DataControllerDeps{
		Cfg:       infra.Cfg,
		Logger:    infra.Logger,
		DocStore:  infra.Stores.DocStore,
		KVStore:   infra.Stores.KVStore,
		SSEStore:  infra.Stores.SSEStore,
		BlobStore: infra.Stores.BlobStore,
		Pubsub:    infra.Pubsub,
		Responder: infra.Responder,
	})

	return dataController, infra.Stores
}

func TestNewDataController_AllDepsProvidedNoNilFields(t *testing.T) {
	infra := setupTestInfrastructure(t, false)

	controller := newDataController(DataControllerDeps{
		Cfg:       infra.Cfg,
		Logger:    infra.Logger,
		DocStore:  infra.Stores.DocStore,
		KVStore:   infra.Stores.KVStore,
		SSEStore:  infra.Stores.SSEStore,
		BlobStore: infra.Stores.BlobStore,
		Pubsub:    infra.Pubsub,
		Responder: infra.Responder,
	})

	assert.NotNil(t, controller.cfg)
	assert.NotNil(t, controller.logger)
	assert.NotNil(t, controller.docStore)
	assert.NotNil(t, controller.kvStore)
	assert.NotNil(t, controller.sseStore)
	assert.NotNil(t, controller.blobStore)
	assert.NotNil(t, controller.pubsub)
	assert.NotNil(t, controller.responder)

	assert.Equal(t, infra.Cfg, controller.cfg)
	assert.Equal(t, infra.Logger, controller.logger)
	assert.Equal(t, infra.Stores.DocStore, controller.docStore)
	assert.Equal(t, infra.Stores.KVStore, controller.kvStore)
	assert.Equal(t, infra.Stores.SSEStore, controller.sseStore)
	assert.Equal(t, infra.Stores.BlobStore, controller.blobStore)
	assert.Equal(t, infra.Pubsub, controller.pubsub)
	assert.Equal(t, infra.Responder, controller.responder)
}

func TestDataControllerHandleDB(t *testing.T) {
	dataController, stores := setupTestDataController(t)

	t.Run("BadRequest - no collection", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/data/", nil)
		rr := httptest.NewRecorder()
		dataController.handleDataDB(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - no ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/data/users/", nil)
		rr := httptest.NewRecorder()
		dataController.handleDataDB(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("PUT and GET", func(t *testing.T) {
		data := map[string]string{"name": "alice"}
		reqPut := httptest.NewRequest(http.MethodPut, "/api/v1/data/settings/u1", bytes.NewReader(mustDocJSON(t, data)))
		rrPut := httptest.NewRecorder()
		dataController.handleDataDB(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/data/settings/u1", nil)
		rrGet := httptest.NewRecorder()
		dataController.handleDataDB(rrGet, reqGet)
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
		dataController.handleDataDB(rrPatch, reqPatch)
		assert.Equal(t, http.StatusOK, rrPatch.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/data/settings/u1", nil)
		rrGet := httptest.NewRecorder()
		dataController.handleDataDB(rrGet, reqGet)
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
		dataController.handleDataDB(rrDel, reqDel)
		assert.Equal(t, http.StatusOK, rrDel.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/data/settings/u1", nil)
		rrGet := httptest.NewRecorder()
		dataController.handleDataDB(rrGet, reqGet)
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
		dataController.handleDataDB(rrQuery, reqQuery)
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
		dataController.handleDataDB(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"invalid JSON body"}`, rr.Body.String())
	})

	t.Run("PATCH not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/data/settings/nonexistent", bytes.NewReader(mustDocJSON(t, map[string]string{"foo": "bar"})))
		rr := httptest.NewRecorder()
		dataController.handleDataDB(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("DELETE not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/data/users/nonexistent", nil)
		rr := httptest.NewRecorder()
		dataController.handleDataDB(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/data/users/u1", nil)
		rr := httptest.NewRecorder()
		dataController.handleDataDB(rr, req)
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
			dataController.handleDataDB(rr, req)
			assert.Equal(t, http.StatusConflict, rr.Code, "method=%s", tt.method)
			assert.JSONEq(t, `{"error":"submit via POST /api/v1/governance/envelopes"}`, rr.Body.String())
		}
	})

	t.Run("Query validation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/data/items/_query", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()
		dataController.handleDBQuery(rr, req, "items")
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("SSE Events count", func(t *testing.T) {
		stores.SSEStore.SSEEventsAppend(SSERoute{WebSessionID: "s1"}, "T", "{}", "")
		req := httptest.NewRequest(http.MethodGet, "/api/v1/data/_sse_events/count", nil)
		rr := httptest.NewRecorder()
		dataController.handleSSEEvents(rr, req, "count")
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("SSE Events wipe", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/data/_sse_events", nil)
		rr := httptest.NewRecorder()
		dataController.handleSSEEvents(rr, req, "")
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("SSE Events invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/data/_sse_events/invalid", nil)
		rr := httptest.NewRecorder()
		dataController.handleSSEEvents(rr, req, "invalid")
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestDataControllerHandleKV(t *testing.T) {
	dataController, stores := setupTestDataController(t)

	t.Run("PUT and GET", func(t *testing.T) {
		reqPut := httptest.NewRequest(http.MethodPut, "/api/v1/kv/k1", bytes.NewReader(mustDocJSON(t, models.KVSetRequest{Value: "g8e"})))
		rrPut := httptest.NewRecorder()
		dataController.handleKV(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/kv/k1", nil)
		rrGet := httptest.NewRecorder()
		dataController.handleKV(rrGet, reqGet)
		assert.Equal(t, http.StatusOK, rrGet.Code)
		assert.Contains(t, rrGet.Body.String(), `"value":"g8e"`)
	})

	t.Run("TTL and Expire", func(t *testing.T) {
		reqTtl := httptest.NewRequest(http.MethodGet, "/api/v1/kv/k1/_ttl", nil)
		rrTtl := httptest.NewRecorder()
		dataController.handleKV(rrTtl, reqTtl)
		assert.Equal(t, http.StatusOK, rrTtl.Code)

		reqExp := httptest.NewRequest(http.MethodPut, "/api/v1/kv/k1/_expire", bytes.NewReader(mustDocJSON(t, models.KVExpireRequest{TTL: 100})))
		rrExp := httptest.NewRecorder()
		dataController.handleKV(rrExp, reqExp)
		assert.Equal(t, http.StatusOK, rrExp.Code)
	})

	t.Run("Scan and DeletePattern", func(t *testing.T) {
		stores.KVStore.KVSet("pref:1", "a", 0)
		stores.KVStore.KVSet("pref:2", "b", 0)

		reqScan := httptest.NewRequest(http.MethodPost, "/api/v1/kv/_scan", bytes.NewReader(mustDocJSON(t, models.KVPatternRequest{Pattern: "pref:*"})))
		rrScan := httptest.NewRecorder()
		dataController.handleKV(rrScan, reqScan)
		assert.Equal(t, http.StatusOK, rrScan.Code)
		assert.Contains(t, rrScan.Body.String(), "pref:1")

		reqDel := httptest.NewRequest(http.MethodPost, "/api/v1/kv/_delete_pattern", bytes.NewReader(mustDocJSON(t, models.KVPatternRequest{Pattern: "pref:*"})))
		rrDel := httptest.NewRecorder()
		dataController.handleKV(rrDel, reqDel)
		assert.Equal(t, http.StatusOK, rrDel.Code)
		assert.Contains(t, rrDel.Body.String(), `"deleted":2`)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/kv/k1", strings.NewReader("{invalid-json}"))
		rr := httptest.NewRecorder()
		dataController.handleKV(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("TTL required for expire", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/kv/k1/_expire", strings.NewReader(`{"ttl":0}`))
		rr := httptest.NewRecorder()
		dataController.handleKV(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Keys", func(t *testing.T) {
		stores.KVStore.KVSet("key1", "val1", 0)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/_keys", strings.NewReader(`{"pattern":"key*"}`))
		rr := httptest.NewRecorder()
		dataController.handleKVKeys(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "key1")
	})

	t.Run("KV Keys Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/_keys", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		dataController.handleKVKeys(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Scan Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/_scan", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		dataController.handleKVScan(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Delete Pattern Missing Pattern", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/_delete_pattern", strings.NewReader(`{}`))
		rr := httptest.NewRecorder()
		dataController.handleKVDeletePattern(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Delete Pattern Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/_delete_pattern", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		dataController.handleKVDeletePattern(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/kv/k1", strings.NewReader(`{"value":"x"}`))
		rr := httptest.NewRecorder()
		dataController.handleKV(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestDataControllerHandleBlob(t *testing.T) {
	dataController, _ := setupTestDataController(t)

	t.Run("PUT and GET", func(t *testing.T) {
		content := []byte("blob-data")
		reqPut := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/putget-b1", bytes.NewReader(content))
		reqPut.Header.Set("Content-Type", "text/plain")
		reqPut = reqPut.WithContext(context.WithValue(reqPut.Context(), constants.ContextKeyUserID, "user-1"))
		rrPut := httptest.NewRecorder()
		dataController.handleBlob(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/blobs/temp/putget-b1", nil)
		rrGet := httptest.NewRecorder()
		dataController.handleBlob(rrGet, reqGet)
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
		dataController.handleBlob(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqMeta := httptest.NewRequest(http.MethodGet, "/api/v1/blobs/cache/meta-b2/meta", nil)
		rrMeta := httptest.NewRecorder()
		dataController.handleBlob(rrMeta, reqMeta)
		assert.Equal(t, http.StatusOK, rrMeta.Code)
		assert.Contains(t, rrMeta.Body.String(), `"id":"meta-b2"`)
	})

	t.Run("Too Large", func(t *testing.T) {
		largeBody := make([]byte, maxBlobBodySize+1)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/large-test", bytes.NewReader(largeBody))
		req.Header.Set("Content-Type", "application/octet-stream")
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
		rr := httptest.NewRecorder()
		dataController.handleBlob(rr, req)
		assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	})

	t.Run("Blob_meta_not_found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/blobs/temp/nonexistent/meta", nil)
		rr := httptest.NewRecorder()
		dataController.handleBlob(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("X-Blob-TTL_invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/ttl-invalid", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("X-Blob-TTL", "invalid")
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
		rr := httptest.NewRecorder()
		dataController.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("X-Blob-TTL_valid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/ttl-valid", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("X-Blob-TTL", "3600")
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
		rr := httptest.NewRecorder()
		dataController.handleBlob(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Blob_PUT_empty_body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/empty-body", bytes.NewReader([]byte{}))
		req.Header.Set("Content-Type", "text/plain")
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
		rr := httptest.NewRecorder()
		dataController.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Blob_get_not_found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/blobs/temp/nonexistent", nil)
		rr := httptest.NewRecorder()
		dataController.handleBlob(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Invalid_namespace", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/../ns1/b1", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		dataController.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Blob_id_invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/ns1/../b1", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		dataController.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Namespace_delete_deletes_all_blobs_in_namespace", func(t *testing.T) {
		content := []byte("blob-data")
		reqPut1 := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/scratch/del-b1", bytes.NewReader(content))
		reqPut1.Header.Set("Content-Type", "text/plain")
		reqPut1 = reqPut1.WithContext(context.WithValue(reqPut1.Context(), constants.ContextKeyUserID, "user-1"))
		rrPut1 := httptest.NewRecorder()
		dataController.handleBlob(rrPut1, reqPut1)
		assert.Equal(t, http.StatusOK, rrPut1.Code)

		reqPut2 := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/scratch/del-b2", bytes.NewReader(content))
		reqPut2.Header.Set("Content-Type", "text/plain")
		reqPut2 = reqPut2.WithContext(context.WithValue(reqPut2.Context(), constants.ContextKeyUserID, "user-1"))
		rrPut2 := httptest.NewRecorder()
		dataController.handleBlob(rrPut2, reqPut2)
		assert.Equal(t, http.StatusOK, rrPut2.Code)

		reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/blobs/scratch", nil)
		reqDel = reqDel.WithContext(context.WithValue(reqDel.Context(), constants.ContextKeyUserID, "user-1"))
		rrDel := httptest.NewRecorder()
		dataController.handleBlob(rrDel, reqDel)
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
		dataController.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Governance: Non-allowlisted namespace rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/governed-ns/b1", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		dataController.handleBlob(rr, req)
		assert.Equal(t, http.StatusConflict, rr.Code)
		assert.Contains(t, rr.Body.String(), "submit via POST /api/v1/governance/envelopes")
	})

	t.Run("Governance: Allowlisted namespace accepted without identity", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/b1", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		dataController.handleBlob(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "unauthorized")
	})

	t.Run("Governance: Cross-namespace ownership rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/governed-ns/b1", bytes.NewReader([]byte("data")))
		req.Header.Set("Content-Type", "text/plain")
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, "user-1"))
		rr := httptest.NewRecorder()
		dataController.handleBlob(rr, req)
		assert.Equal(t, http.StatusConflict, rr.Code)
		assert.Contains(t, rr.Body.String(), "submit via POST /api/v1/governance/envelopes")
	})

	t.Run("Governance: Delete non-allowlisted namespace rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/blobs/governed-ns", nil)
		rr := httptest.NewRecorder()
		dataController.handleBlob(rr, req)
		assert.Equal(t, http.StatusConflict, rr.Code)
		assert.Contains(t, rr.Body.String(), "submit via POST /api/v1/governance/envelopes")
	})
}

func TestDataControllerHandlePubSubPublish(t *testing.T) {
	dataController, _ := setupTestDataController(t)

	t.Run("Publish valid", func(t *testing.T) {
		pubReq := models.PubSubPublishRequest{
			Channel: pubsub.ResultsChannel("op-1", "session-1"),
			Data:    mustDocJSON(t, map[string]string{"foo": "bar"}),
		}
		body := mustMarshalJSON(t, pubReq)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pubsub/publish", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		dataController.handlePubSubPublish(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"receivers":0`)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pubsub/publish", nil)
		rr := httptest.NewRecorder()
		dataController.handlePubSubPublish(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pubsub/publish", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()
		dataController.handlePubSubPublish(rr, req)
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
			dataController.handlePubSubPublish(rr, req)
			assert.Equal(t, http.StatusConflict, rr.Code, "channel=%s", channel)
			assert.JSONEq(t, `{"error":"submit via POST /api/v1/governance/envelopes"}`, rr.Body.String())
		}
	})
}

func TestDataControllerHandleRevokeApp(t *testing.T) {
	_, stores := setupTestDataController(t)
	userSvc := NewUserService(stores.DocStore, testutil.NewTestLogger())
	logger := testutil.NewTestLogger()
	cfg := testutil.NewTestConfig(t)
	resp := response.NewWriter(logger)
	adminController := newAdminController(AdminControllerDeps{Cfg: cfg, Logger: logger, DocStore: stores.DocStore, SignerStore: stores.SignerStore, ConsensusStore: stores.ConsensusStore, UserSvc: userSvc, Responder: resp})

	// The first user created is the gateway admin (IsFirstUser). The second
	// user is a non-admin regular user.
	adminUser, err := userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, adminUser)
	t.Cleanup(func() { stores.DocStore.DocDelete("users", adminUser.ID) })

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
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, adminUser.ID))

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
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, adminUser.ID))

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
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, adminUser.ID))

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
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, adminUser.ID))

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

// TestDataController_BlobNamespaceAllowlist verifies that blob namespace
// allowlist enforcement is working correctly.
// This is a regression test for Finding 4: Blob store bypasses governance.
func TestDataController_BlobNamespaceAllowlist(t *testing.T) {
	dataController, _ := setupTestDataController(t)

	t.Run("allowlisted namespace accepted", func(t *testing.T) {
		allowlistedNamespaces := []string{"temp", "uploads", "cache", "scratch"}
		for _, ns := range allowlistedNamespaces {
			assert.True(t, blobNamespaceAllowed(ns), "Namespace %s should be allowlisted", ns)
		}
	})

	t.Run("non-allowlisted namespace rejected", func(t *testing.T) {
		nonAllowlistedNamespaces := []string{"private", "secret", "config", "data"}
		for _, ns := range nonAllowlistedNamespaces {
			assert.False(t, blobNamespaceAllowed(ns), "Namespace %s should not be allowlisted", ns)
		}
	})

	t.Run("blob ownership verification - app identity", func(t *testing.T) {
		appID := "test-app-123"
		userID := "user-456"

		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/app/"+appID+"/test.txt", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyAppID, appID))
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, userID))

		err := dataController.verifyBlobOwnership(req, "app/"+appID)
		require.NoError(t, err, "App should be able to write to its own namespace")

		err = dataController.verifyBlobOwnership(req, "app/other-app")
		require.Error(t, err, "App should not be able to write to another app's namespace")
	})

	t.Run("blob ownership verification - user identity", func(t *testing.T) {
		userID := "user-789"

		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/user/"+userID+"/test.txt", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, userID))

		err := dataController.verifyBlobOwnership(req, "user/"+userID)
		require.NoError(t, err, "User should be able to write to their own namespace")

		err = dataController.verifyBlobOwnership(req, "user/other-user")
		require.Error(t, err, "User should not be able to write to another user's namespace")
	})

	t.Run("blob ownership verification - allowlisted namespace", func(t *testing.T) {
		userID := "user-999"

		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/test.txt", nil)
		req = req.WithContext(context.WithValue(req.Context(), constants.ContextKeyUserID, userID))

		err := dataController.verifyBlobOwnership(req, "temp")
		require.NoError(t, err, "Authenticated identity should be able to write to allowlisted namespace")

		err = dataController.verifyBlobOwnership(req, "uploads")
		require.NoError(t, err, "Authenticated identity should be able to write to allowlisted namespace")
	})

	t.Run("blob ownership verification - no identity rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/blobs/temp/test.txt", nil)

		err := dataController.verifyBlobOwnership(req, "temp")
		require.Error(t, err, "Request without identity should be rejected")
	})
}

func TestDataControllerHandleDataSettings(t *testing.T) {
	dataController, stores := setupTestDataController(t)

	t.Run("GET - success", func(t *testing.T) {
		settings := map[string]string{"mode": "test"}
		err := stores.DocStore.DocSet("settings", "platform_settings", mustDocJSON(t, settings))
		require.NoError(t, err)
		t.Cleanup(func() { stores.DocStore.DocDelete("settings", "platform_settings") })

		req := httptest.NewRequest(http.MethodGet, "/api/v1/data/settings", nil)
		rr := httptest.NewRecorder()
		dataController.handleDataSettings(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "test")
	})

	t.Run("PUT - success", func(t *testing.T) {
		settings := map[string]string{"mode": "production"}
		req := httptest.NewRequest(http.MethodPut, "/api/v1/data/settings", bytes.NewReader(mustDocJSON(t, settings)))
		rr := httptest.NewRecorder()
		dataController.handleDataSettings(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		t.Cleanup(func() { stores.DocStore.DocDelete("settings", "platform_settings") })
	})

	t.Run("PATCH - success", func(t *testing.T) {
		settings := map[string]string{"mode": "test"}
		err := stores.DocStore.DocSet("settings", "platform_settings", mustDocJSON(t, settings))
		require.NoError(t, err)

		patch := map[string]string{"mode": "production"}
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/data/settings", bytes.NewReader(mustDocJSON(t, patch)))
		rr := httptest.NewRecorder()
		dataController.handleDataSettings(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		t.Cleanup(func() { stores.DocStore.DocDelete("settings", "platform_settings") })
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/data/settings", nil)
		rr := httptest.NewRecorder()
		dataController.handleDataSettings(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/data/settings", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()
		dataController.handleDataSettings(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}
