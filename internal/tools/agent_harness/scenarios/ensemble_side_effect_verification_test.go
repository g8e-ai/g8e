// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package scenarios

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	clientpkg "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
)

// fakeReadFileMCPServer returns an httptest server simulating the read_file tool via MCP.
func fakeReadFileMCPServer(t *testing.T, fileContent string, errStr string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != constants.APIPaths.MCPEndpoint {
			http.NotFound(w, r)
			return
		}

		fileResult := map[string]any{
			"content": fileContent,
			"error":   errStr,
		}
		fileResultBytes, _ := json.Marshal(fileResult)

		callResult := map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": string(fileResultBytes),
				},
			},
			"isError": false,
		}
		callResultBytes, _ := json.Marshal(callResult)

		mcpResp := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  json.RawMessage(callResultBytes),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mcpResp)
	}))
}

func TestGovernedReadBack_FailsWhenFileContentMismatches(t *testing.T) {
	server := fakeReadFileMCPServer(t, "completely different file content", "")
	defer server.Close()

	client, err := clientpkg.New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	ctx := context.Background()
	res := &Result{}
	persona := clientpkg.Persona{ID: "test"}

	content, err := governedReadBack(ctx, client, res, persona, "/tmp/smoke.txt")
	require.NoError(t, err)

	expectedContent := "expected unique smoke content"
	assert.NotContains(t, content, expectedContent, "content mismatch must be detected by caller assertion")
}

func TestGovernedReadBack_FailsWhenFileIsMissing(t *testing.T) {
	server := fakeReadFileMCPServer(t, "", "file not found")
	defer server.Close()

	client, err := clientpkg.New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	ctx := context.Background()
	res := &Result{}
	persona := clientpkg.Persona{ID: "test"}

	_, err = governedReadBack(ctx, client, res, persona, "/tmp/nonexistent.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "file not found")
}

func TestGovernedDocumentReadBack_FailsWhenDocumentIsMissing(t *testing.T) {
	// Server returns 404 for GET /api/v1/data/{collection}/{id}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := clientpkg.New(config.Config{MTLSBaseURL: server.URL})
	require.NoError(t, err)

	ctx := context.Background()
	res := &Result{}
	persona := clientpkg.Persona{ID: "test"}

	doc, err := governedDocumentReadBack(ctx, client, res, persona, "investigations", "nonexistent-doc")
	assert.NoError(t, err, "GetDocument returns nil, nil on 404")
	assert.Nil(t, doc, "governedDocumentReadBack must return nil doc when document is missing (404)")
}
