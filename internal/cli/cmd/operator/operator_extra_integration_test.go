// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package operatorcmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperatorModelReleaseCmd_UsesEmbeddedOllamaClient(t *testing.T) {
	var request struct {
		Model     string          `json:"model"`
		KeepAlive json.RawMessage `json:"keep_alive"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/generate", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.NoError(t, json.NewEncoder(w).Encode(struct {
			Model      string `json:"model"`
			Done       bool   `json:"done"`
			DoneReason string `json:"done_reason"`
		}{
			Model:      request.Model,
			Done:       true,
			DoneReason: "unload",
		}))
	}))
	t.Cleanup(server.Close)
	t.Setenv("OLLAMA_HOST", server.URL)

	cmd := operatorModelReleaseCmd()
	cmd.SetArgs([]string{"qwen3:0.6b"})
	require.NoError(t, cmd.ExecuteContext(t.Context()))
	assert.Equal(t, "qwen3:0.6b", request.Model)
	assert.JSONEq(t, "0", string(request.KeepAlive))
}
