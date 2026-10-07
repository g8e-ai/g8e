// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

const savedSettings = `{"providers":[],"primary":{"provider":"g8e","model":"qwen3:4b"},` +
	`"assistant":{},"lite":{"provider":"g8e","model":"qwen3:1.7b"},"eval_judge_model":"qwen3:1.7b"}`

func runInference(t *testing.T, client *cmdtest.MockAPIClient, args ...string) (string, error) {
	t.Helper()
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := cmdWithConfig(
		cmdtest.ConfigLoaderFor(cfg),
		func(fs.RuntimeFileService, *config.Config) (authcmd.APIClient, error) { return client, nil },
		cmdtest.FileSvcFactoryFor(fileSvc),
	)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func postedBody(t *testing.T, client *cmdtest.MockAPIClient) map[string]any {
	t.Helper()
	require.Len(t, client.PostCalls, 1)
	raw, err := json.Marshal(client.PostCalls[0].Body)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	return body
}

func TestShowPrintsEveryRoleAndTheJudge(t *testing.T) {
	client := &cmdtest.MockAPIClient{PostResp: []byte(savedSettings)}

	out, err := runInference(t, client, "show")

	require.NoError(t, err)
	assert.Equal(t, constants.APIPaths.EnsembleSettingsPrefix+"/llm/get", client.PostCalls[0].Path)
	assert.Contains(t, out, "primary    g8e:qwen3:4b")
	assert.Contains(t, out, "assistant  (unset)")
	assert.Contains(t, out, "lite       g8e:qwen3:1.7b")
	assert.Contains(t, out, "judge      qwen3:1.7b")
}

func TestSetSendsOnlyTheRolesThatWerePassed(t *testing.T) {
	client := &cmdtest.MockAPIClient{PostResp: []byte(savedSettings)}

	_, err := runInference(t, client, "set", "--primary", "g8e:qwen3:4b", "--judge", "qwen3:1.7b")

	require.NoError(t, err)
	assert.Equal(t, constants.APIPaths.EnsembleSettingsPrefix+"/llm", client.PostCalls[0].Path)
	body := postedBody(t, client)
	assert.Equal(t, map[string]any{"provider": "g8e", "model": "qwen3:4b"}, body["primary"])
	assert.Equal(t, map[string]any{"model": "qwen3:1.7b"}, body["eval_judge"])
	assert.NotContains(t, body, "assistant")
	assert.NotContains(t, body, "lite")
}

func TestSetSplitsTheRoleAtTheFirstColonSoModelTagsKeepTheirColons(t *testing.T) {
	client := &cmdtest.MockAPIClient{PostResp: []byte(savedSettings)}

	_, err := runInference(t, client, "set", "--lite", "ollama:qwen3:1.7b")

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"provider": "ollama", "model": "qwen3:1.7b"}, postedBody(t, client)["lite"])
}

func TestSetWithAnEmptyValueClearsAnOptionalRoleAndTheJudge(t *testing.T) {
	client := &cmdtest.MockAPIClient{PostResp: []byte(savedSettings)}

	_, err := runInference(t, client, "set", "--lite", "", "--judge", "")

	require.NoError(t, err)
	body := postedBody(t, client)
	assert.Equal(t, map[string]any{"provider": nil}, body["lite"])
	assert.Equal(t, map[string]any{"model": ""}, body["eval_judge"])
}

func TestSetRejectsBadInputBeforeAnyRequest(t *testing.T) {
	for name, args := range map[string][]string{
		"nothing to set":        {"set"},
		"clearing primary":      {"set", "--primary", ""},
		"role without provider": {"set", "--lite", "qwen3"},
		"role without model":    {"set", "--lite", "g8e:"},
	} {
		t.Run(name, func(t *testing.T) {
			client := &cmdtest.MockAPIClient{PostResp: []byte(savedSettings)}

			_, err := runInference(t, client, args...)

			require.Error(t, err)
			assert.Empty(t, client.PostCalls)
		})
	}
}

func TestSetReportsAGatewayFailure(t *testing.T) {
	client := &cmdtest.MockAPIClient{PostErr: fmt.Errorf("boom")}

	_, err := runInference(t, client, "set", "--judge", "qwen3:1.7b")

	require.ErrorIs(t, err, constants.ErrHTTPRequestExecuteFailed)
}
