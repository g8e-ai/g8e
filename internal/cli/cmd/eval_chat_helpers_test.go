// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatAcceptReporter_WritesEnabledMessages(t *testing.T) {
	var output bytes.Buffer
	reporter := &chatAcceptReporter{out: &output, quiet: false}
	reporter.writeSetup(2, "qwen3:4b", "inf-1", "data-1", "http://127.0.0.1:8000")
	reporter.caseStart(1, 2, "case-a", "assign-1", "attempt-1")
	reporter.chatSubmitted("case-a", "investigation-1")
	reporter.chatSubmitFailed(fmt.Errorf("submit failed"))
	reporter.traceWaiting()
	reporter.traceFetchRetrying(fmt.Errorf("pending"), 2*time.Second)
	reporter.traceFetchFailed(fmt.Errorf("lookup failed"))

	text := output.String()
	assert.Contains(t, text, "Phase 1A chat acceptance")
	assert.Contains(t, text, "qwen3:4b")
	assert.Contains(t, text, "assignment_id=assign-1")
	assert.Contains(t, text, "investigation_id=investigation-1")
	assert.Contains(t, text, "chat submit failed")
	assert.Contains(t, text, "trace lookup failed")
}

func TestChatAcceptReporter_SilentWhenDisabled(t *testing.T) {
	var output bytes.Buffer
	reporter := &chatAcceptReporter{out: &output, quiet: true}
	reporter.writeSetup(1, "qwen3:4b", "inf-1", "data-1", "http://127.0.0.1:8000")
	assert.Empty(t, output.String())
}

func TestParseChatAcceptanceCases(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantLen int
		wantErr string
	}{
		{name: "empty", raw: "", wantLen: 0},
		{name: "single", raw: "basic_chat", wantLen: 1},
		{name: "comma separated", raw: "basic_chat, tool_use", wantLen: 2},
		{name: "no cases", raw: " , ", wantErr: "no cases selected"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseChatAcceptanceCases(test.raw)
			if test.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, test.wantLen)
		})
	}
}

func TestChatEvalLoadRegistry_WithRegistryFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "registry.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"campaign_id": "eval-init-qwen3-4b",
		"model_registry_digest": "digest-1",
		"variants": [{"model": "qwen3:4b", "digest": "abc123"}]
	}`), 0o600))

	selection, err := chatEvalLoadRegistry("", "", path, "qwen3:4b")
	require.NoError(t, err)
	require.NotNil(t, selection)
	assert.Equal(t, "eval-init-qwen3-4b", selection.CampaignID)
	assert.Equal(t, "digest-1", selection.Digest)
	assert.Equal(t, "abc123", selection.ModelDigest)
}

func TestChatEvalLoadRegistry_RequiresRegistryFileForDigestLookup(t *testing.T) {
	_, err := chatEvalLoadRegistry("eval-init-qwen3-4b", "digest-1", "", "qwen3:4b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires --registry-file")

	_, err = chatEvalLoadRegistry("", "", "", "qwen3:4b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "set --registry-file")
}
