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

package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigDiffMaskExecute_InvalidJSONReturnsError(t *testing.T) {
	t.Parallel()

	tool := &ConfigDiffMaskTool{}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid arguments")
}

func TestConfigDiffMaskExecute_MissingConfigPathReturnsError(t *testing.T) {
	t.Parallel()

	tool := &ConfigDiffMaskTool{}
	args, _ := json.Marshal(ConfigDiffMaskRequest{
		Baseline: "some config",
	})
	_, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config_path and baseline required")
}

func TestConfigDiffMaskExecute_MissingBaselineReturnsError(t *testing.T) {
	t.Parallel()

	tool := &ConfigDiffMaskTool{}
	args, _ := json.Marshal(ConfigDiffMaskRequest{
		ConfigPath: "/some/path",
	})
	_, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config_path and baseline required")
}

func TestConfigDiffMaskExecute_NonExistentFileReturnsError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tool := &ConfigDiffMaskTool{}
	args, _ := json.Marshal(ConfigDiffMaskRequest{
		ConfigPath: filepath.Join(dir, "nonexistent.yaml"),
		Baseline:   "some config",
	})
	_, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read config file")
}

func TestConfigDiffMaskExecute_IdenticalContentReturnsNoDifferences(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	content := "server:\n  port: 8080\n  host: localhost\n"
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0644))

	tool := &ConfigDiffMaskTool{}
	args, _ := json.Marshal(ConfigDiffMaskRequest{
		ConfigPath: configPath,
		Baseline:   content,
	})

	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var decoded ConfigDiffMaskResult
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &decoded))
	assert.Empty(t, decoded.Differences)
}

func TestConfigDiffMaskExecute_AddedLinesClassifiedAsAdded(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("line1\nline2\nline3\n"), 0644))

	tool := &ConfigDiffMaskTool{}
	args, _ := json.Marshal(ConfigDiffMaskRequest{
		ConfigPath: configPath,
		Baseline:   "line1\n",
	})

	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var decoded ConfigDiffMaskResult
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &decoded))

	// line_0 is identical, line_1 and line_2 are new
	var addedDiffs []ConfigDiff
	for _, d := range decoded.Differences {
		if d.Type == "added" {
			addedDiffs = append(addedDiffs, d)
		}
	}
	assert.Len(t, addedDiffs, 2)
}

func TestConfigDiffMaskExecute_RemovedLinesClassifiedAsRemoved(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("line1\n"), 0644))

	tool := &ConfigDiffMaskTool{}
	args, _ := json.Marshal(ConfigDiffMaskRequest{
		ConfigPath: configPath,
		Baseline:   "line1\nline2\nline3\n",
	})

	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var decoded ConfigDiffMaskResult
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &decoded))

	var removedDiffs []ConfigDiff
	for _, d := range decoded.Differences {
		if d.Type == "removed" {
			removedDiffs = append(removedDiffs, d)
		}
	}
	assert.Len(t, removedDiffs, 2)
}

func TestConfigDiffMaskExecute_ChangedLinesClassifiedAsChanged(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("server:\n  port: 9090\n"), 0644))

	tool := &ConfigDiffMaskTool{}
	args, _ := json.Marshal(ConfigDiffMaskRequest{
		ConfigPath: configPath,
		Baseline:   "server:\n  port: 8080\n",
	})

	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var decoded ConfigDiffMaskResult
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &decoded))

	require.Len(t, decoded.Differences, 1)
	assert.Equal(t, "changed", decoded.Differences[0].Type)
	assert.Equal(t, "line_1", decoded.Differences[0].Key)
}

func TestConfigDiffMaskExecute_MasksPasswordSecrets(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("password: supersecret123\n"), 0644))

	tool := &ConfigDiffMaskTool{}
	args, _ := json.Marshal(ConfigDiffMaskRequest{
		ConfigPath: configPath,
		Baseline:   "password: oldpassword\n",
	})

	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var decoded ConfigDiffMaskResult
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &decoded))

	require.Len(t, decoded.Differences, 1)
	assert.Equal(t, "REDACTED", decoded.Differences[0].Current)
	assert.Equal(t, "REDACTED", decoded.Differences[0].Baseline)
}

func TestConfigDiffMaskExecute_MasksTokenSecrets(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("api_token: abc123\n"), 0644))

	tool := &ConfigDiffMaskTool{}
	args, _ := json.Marshal(ConfigDiffMaskRequest{
		ConfigPath: configPath,
		Baseline:   "api_token: oldtoken\n",
	})

	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var decoded ConfigDiffMaskResult
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &decoded))

	require.Len(t, decoded.Differences, 1)
	assert.Equal(t, "REDACTED", decoded.Differences[0].Current)
	assert.Equal(t, "REDACTED", decoded.Differences[0].Baseline)
}

func TestConfigDiffMaskExecute_MasksKeySecrets(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("private_key: abc123\n"), 0644))

	tool := &ConfigDiffMaskTool{}
	args, _ := json.Marshal(ConfigDiffMaskRequest{
		ConfigPath: configPath,
		Baseline:   "private_key: oldkey\n",
	})

	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var decoded ConfigDiffMaskResult
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &decoded))

	require.Len(t, decoded.Differences, 1)
	assert.Equal(t, "REDACTED", decoded.Differences[0].Current)
	assert.Equal(t, "REDACTED", decoded.Differences[0].Baseline)
}

func TestConfigDiffMaskExecute_DoesNotMaskNonSensitiveValues(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("server:\n  port: 9090\n"), 0644))

	tool := &ConfigDiffMaskTool{}
	args, _ := json.Marshal(ConfigDiffMaskRequest{
		ConfigPath: configPath,
		Baseline:   "server:\n  port: 8080\n",
	})

	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var decoded ConfigDiffMaskResult
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &decoded))

	require.Len(t, decoded.Differences, 1)
	assert.Equal(t, "port: 9090", decoded.Differences[0].Current)
	assert.Equal(t, "port: 8080", decoded.Differences[0].Baseline)
}

func TestConfigDiffMaskExecute_PathTraversalRejected(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("safe\n"), 0644))

	tool := &ConfigDiffMaskTool{}
	args, _ := json.Marshal(ConfigDiffMaskRequest{
		ConfigPath: "../../../etc/passwd",
		Baseline:   "safe\n",
	})

	_, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid config path")
}

func TestConfigDiffMaskExecute_DiffKeyUsesLineIndex(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("a\nb\nc\nd\n"), 0644))

	tool := &ConfigDiffMaskTool{}
	args, _ := json.Marshal(ConfigDiffMaskRequest{
		ConfigPath: configPath,
		Baseline:   "a\nX\nc\nd\n",
	})

	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var decoded ConfigDiffMaskResult
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &decoded))

	require.Len(t, decoded.Differences, 1)
	assert.Equal(t, "line_1", decoded.Differences[0].Key)
}
