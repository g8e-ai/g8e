// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvalExplorerHandler_ServesEmbeddedAssetsAndRuntime(t *testing.T) {
	handler, err := NewEvalExplorerHandler("", "https://opendevops.ai")
	require.NoError(t, err)

	indexReq := httptest.NewRequest(http.MethodGet, "/", nil)
	indexRes := httptest.NewRecorder()
	handler.ServeHTTP(indexRes, indexReq)
	require.Equal(t, http.StatusOK, indexRes.Code)
	assert.Contains(t, indexRes.Body.String(), "<!doctype html>")

	runtimeReq := httptest.NewRequest(http.MethodGet, "/runtime.json", nil)
	runtimeRes := httptest.NewRecorder()
	handler.ServeHTTP(runtimeRes, runtimeReq)
	require.Equal(t, http.StatusOK, runtimeRes.Code)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(runtimeRes.Body.Bytes(), &payload))
	assert.Equal(t, "https://opendevops.ai", payload["mirror_origin"])
}

func TestCombinePublicSpectatorHandler_RoutesMirrorAndExplorer(t *testing.T) {
	explorer, err := NewEvalExplorerHandler("", "https://opendevops.ai")
	require.NoError(t, err)
	mirror := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	handler := combinePublicSpectatorHandler(mirror, explorer)

	bootstrapReq := httptest.NewRequest(http.MethodGet, "/bootstrap", nil)
	bootstrapRes := httptest.NewRecorder()
	handler.ServeHTTP(bootstrapRes, bootstrapReq)
	require.Equal(t, http.StatusOK, bootstrapRes.Code)
	body, err := io.ReadAll(bootstrapRes.Body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, string(body))

	indexReq := httptest.NewRequest(http.MethodGet, "/", nil)
	indexRes := httptest.NewRecorder()
	handler.ServeHTTP(indexRes, indexReq)
	require.Equal(t, http.StatusOK, indexRes.Code)
	assert.Contains(t, indexRes.Body.String(), "<!doctype html>")

	ingestReq := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader("{}"))
	ingestRes := httptest.NewRecorder()
	handler.ServeHTTP(ingestRes, ingestReq)
	require.Equal(t, http.StatusNotFound, ingestRes.Code)
}

func TestResolveEvalExplorerMirrorOrigin_PrefersPublicBaseURL(t *testing.T) {
	assert.Equal(t, "https://opendevops.ai", resolveEvalExplorerMirrorOrigin("https://opendevops.ai", "127.0.0.1:8082"))
	assert.Equal(t, "http://127.0.0.1:8082", resolveEvalExplorerMirrorOrigin("", "127.0.0.1:8082"))
}
