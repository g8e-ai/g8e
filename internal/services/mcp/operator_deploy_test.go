// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/require"
)

// successScript returns a platform-appropriate script that exits 0 and prints "deployed".
func successScript(t *testing.T, dir, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		p := filepath.Join(dir, name+".bat")
		require.NoError(t, os.WriteFile(p, []byte("@echo off\necho deployed\n"), 0755))
		return p
	}
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\necho 'deployed'\nexit 0"), 0755))
	return p
}

func TestOperatorDeployTool_Name(t *testing.T) {
	tool := &OperatorDeployTool{}
	require.Equal(t, "operator_deploy", tool.Name())
}

func TestOperatorDeployTool_Description(t *testing.T) {
	tool := &OperatorDeployTool{}
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "operator")
}

func TestOperatorDeployTool_InputSchema(t *testing.T) {
	tool := &OperatorDeployTool{}
	schema := tool.InputSchema()

	require.Equal(t, "object", schema.Type)
	require.Contains(t, schema.Required, "hostnames")

	properties := schema.Properties
	require.Contains(t, properties, "hostnames")
	require.Contains(t, properties, "operator_binary")
	require.Contains(t, properties, "operator_args")
	require.Contains(t, properties, "timeout")

	require.Equal(t, "array", properties["hostnames"].Type)
	require.Equal(t, "string", properties["operator_binary"].Type)
	require.Equal(t, "array", properties["operator_args"].Type)
	require.Equal(t, "integer", properties["timeout"].Type)
}

func TestOperatorDeployTool_Execute_Validation(t *testing.T) {
	tool := &OperatorDeployTool{}
	ctx := context.Background()

	tests := []struct {
		name    string
		req     OperatorDeployRequest
		wantErr string
	}{
		{
			name: "empty hostnames",
			req: OperatorDeployRequest{
				Hostnames: []string{},
			},
			wantErr: "hostnames list cannot be empty",
		},
		{
			name: "invalid hostname (shell injection)",
			req: OperatorDeployRequest{
				Hostnames: []string{"host; rm -rf /"},
			},
			wantErr: "hostname contains dangerous character",
		},
		{
			name: "invalid operator_binary (path traversal)",
			req: OperatorDeployRequest{
				Hostnames:      []string{"localhost"},
				OperatorBinary: "../../etc/passwd",
			},
			wantErr: "path must not contain parent directory references",
		},
		{
			name: "invalid operator_args (shell injection)",
			req: OperatorDeployRequest{
				Hostnames:    []string{"localhost"},
				OperatorArgs: []string{"arg; rm -rf /"},
			},
			wantErr: "argument contains dangerous character",
		},
		{
			name: "timeout too large",
			req: OperatorDeployRequest{
				Hostnames: []string{"localhost"},
				Timeout:   601,
			},
			wantErr: "timeout cannot exceed 600 seconds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := json.Marshal(tt.req)
			require.NoError(t, err)

			_, err = tool.Execute(ctx, args)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestOperatorDeployTool_Execute_Localhost_Success(t *testing.T) {
	tool := &OperatorDeployTool{}
	ctx := context.Background()

	tmpDir := testutil.TempDir(t)
	dummyBinary := successScript(t, tmpDir, "dummy-operator")

	req := OperatorDeployRequest{
		Hostnames:      []string{"localhost"},
		OperatorBinary: dummyBinary,
	}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var deployResult OperatorDeployResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &deployResult)
	require.NoError(t, err)

	require.Len(t, deployResult.Deployments, 1)
	require.True(t, deployResult.Deployments[0].Success)
	require.Equal(t, "localhost", deployResult.Deployments[0].Hostname)
	require.Contains(t, deployResult.Deployments[0].Message, "Operator deployed successfully")
	require.Contains(t, deployResult.Deployments[0].Output, "deployed")
}

func TestOperatorDeployTool_Execute_Localhost_Failure(t *testing.T) {
	tool := &OperatorDeployTool{}
	ctx := context.Background()

	// Create a dummy "operator" binary that fails
	tmpDir := testutil.TempDir(t)
	dummyBinary := filepath.Join(tmpDir, "dummy-operator-fail")
	content := "#!/bin/sh\necho 'failed' >&2\nexit 1"
	err := os.WriteFile(dummyBinary, []byte(content), 0755)
	require.NoError(t, err)

	req := OperatorDeployRequest{
		Hostnames:      []string{"localhost"},
		OperatorBinary: dummyBinary,
	}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var deployResult OperatorDeployResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &deployResult)
	require.NoError(t, err)

	require.Len(t, deployResult.Deployments, 1)
	require.False(t, deployResult.Deployments[0].Success)
	require.Equal(t, "localhost", deployResult.Deployments[0].Hostname)
	require.Contains(t, deployResult.Deployments[0].Message, "Local deployment failed")
	require.Contains(t, deployResult.Deployments[0].Message, "failed")
}

func TestOperatorDeployTool_Execute_BinaryNotFound(t *testing.T) {
	tool := &OperatorDeployTool{}
	ctx := context.Background()

	req := OperatorDeployRequest{
		Hostnames:      []string{"localhost"},
		OperatorBinary: "/tmp/non-existent-binary-12345",
	}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "operator binary not found")
}

func TestOperatorDeployTool_Execute_SSH_ResolveFailure(t *testing.T) {
	tool := &OperatorDeployTool{}
	ctx := context.Background()

	// We'll use a hostname that is unlikely to be in SSH config or resolvable
	req := OperatorDeployRequest{
		Hostnames: []string{"non-existent-host-999.invalid"},
	}

	// We need a real operator binary for the check to pass before it attempts SSH
	execPath, err := os.Executable()
	require.NoError(t, err)
	req.OperatorBinary = execPath

	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var deployResult OperatorDeployResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &deployResult)
	require.NoError(t, err)

	require.Len(t, deployResult.Deployments, 1)
	require.False(t, deployResult.Deployments[0].Success)
	// The failure could be DNS resolution (dial failure), missing keys, or missing known_hosts
	require.True(t, deployResult.Deployments[0].Message != "", "Should have a failure message")
}

func TestOperatorDeployTool_Execute_MultiHost(t *testing.T) {
	tool := &OperatorDeployTool{}
	ctx := context.Background()

	tmpDir := testutil.TempDir(t)
	dummyBinary := successScript(t, tmpDir, "dummy-multi-operator")

	req := OperatorDeployRequest{
		Hostnames:      []string{"localhost", "127.0.0.1"},
		OperatorBinary: dummyBinary,
	}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var deployResult OperatorDeployResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &deployResult)
	require.NoError(t, err)

	require.Len(t, deployResult.Deployments, 2)
	require.Equal(t, "localhost", deployResult.Deployments[0].Hostname)
	require.Equal(t, "127.0.0.1", deployResult.Deployments[1].Hostname)
	require.True(t, deployResult.Deployments[0].Success)
	require.True(t, deployResult.Deployments[1].Success)
}
