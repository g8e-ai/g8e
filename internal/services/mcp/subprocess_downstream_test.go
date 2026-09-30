// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// TestHelperSubprocess is the entry point for subprocess test helpers.
// When GO_WANT_HELPER_PROCESS=1 is set, it reads JSON-RPC requests from stdin
// and writes responses to stdout.
func TestHelperSubprocess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	mode := os.Getenv("HELPER_MODE")
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		switch mode {
		case "echo":
			var req downstreamJSONRPCRequest
			if err := json.Unmarshal([]byte(line), &req); err != nil {
				fmt.Println(`{"jsonrpc":"2.0","error":{"code":-32700,"message":"parse error"}}`)
				continue
			}
			fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":{\"echo\":\"%s\"}}\n", req.ID, req.Method)
		case "error":
			fmt.Println(`{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"downstream failed"}}`)
		case "invalid_json":
			fmt.Println(`{invalid json line}`)
		case "hang":
			time.Sleep(10 * time.Second)
		case "exit_immediately":
			os.Exit(1)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "scanner err: %v\n", err)
	}
}

func helperCommand(t *testing.T, mode string) (string, []string) {
	t.Helper()
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	t.Setenv("HELPER_MODE", mode)
	return os.Args[0], []string{"-test.run=TestHelperSubprocess"}
}

func TestSubprocessDownstream_StartAndCallSuccess(t *testing.T) {
	cmd, args := helperCommand(t, "echo")
	logger := testutil.NewTestLogger()
	ds := NewSubprocessDownstream(cmd, args, logger)
	defer func() {
		require.NoError(t, ds.Close())
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ds.Call(ctx, "tools/list", nil)
	require.NoError(t, err)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(result, &payload))
	assert.Equal(t, "tools/list", payload["echo"])
	assert.True(t, ds.IsStarted())
	assert.Equal(t, cmd, ds.Command())
	assert.Equal(t, args, ds.Args())
}

func TestSubprocessDownstream_ErrorResponse(t *testing.T) {
	cmd, args := helperCommand(t, "error")
	logger := testutil.NewTestLogger()
	ds := NewSubprocessDownstream(cmd, args, logger)
	defer func() {
		require.NoError(t, ds.Close())
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ds.Call(ctx, "tools/call", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrGatewayMCPError)
	assert.Contains(t, err.Error(), "downstream failed")
}

func TestSubprocessDownstream_InvalidJSONResponse(t *testing.T) {
	cmd, args := helperCommand(t, "invalid_json")
	logger := testutil.NewTestLogger()
	ds := NewSubprocessDownstream(cmd, args, logger)
	defer func() {
		require.NoError(t, ds.Close())
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ds.Call(ctx, "tools/list", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}

func TestSubprocessDownstream_StartFailure(t *testing.T) {
	logger := testutil.NewTestLogger()
	ds := NewSubprocessDownstream("nonexistent-binary-cmd-xyz-9999", nil, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ds.Call(ctx, "tools/list", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrProcessStartFailed)
}

func TestSubprocessDownstream_ContextCancel(t *testing.T) {
	cmd, args := helperCommand(t, "hang")
	logger := testutil.NewTestLogger()
	ds := NewSubprocessDownstream(cmd, args, logger)
	defer func() {
		require.NoError(t, ds.Close())
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := ds.Call(ctx, "tools/list", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestSubprocessDownstream_CloseMultipleTimes(t *testing.T) {
	cmd, args := helperCommand(t, "echo")
	logger := testutil.NewTestLogger()
	ds := NewSubprocessDownstream(cmd, args, logger)

	require.NoError(t, ds.Start())
	assert.True(t, ds.IsStarted())

	require.NoError(t, ds.Close())
	assert.False(t, ds.IsStarted())

	// Closing again should not panic or error
	require.NoError(t, ds.Close())

	// Call after close should return error
	_, err := ds.Call(context.Background(), "tools/list", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}
