// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package authcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sseApproveServer returns an httptest.Server that serves a single
// approval.completed SSE event with the given userID and txHash.
func sseApproveServer(t *testing.T, userID, txHash string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		eventPayload, err := json.Marshal(models.ApprovalCompletedEvent{
			Type:   constants.SSEEventTypeApprovalCompleted,
			UserID: userID,
			TxHash: txHash,
		})
		require.NoError(t, err)
		envelope := models.SSEPushPayload{
			UserID: userID,
			Event:  eventPayload,
		}
		envelopeJSON, err := json.Marshal(envelope)
		require.NoError(t, err)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", constants.SSEEventTypeApprovalCompleted, string(envelopeJSON))
	}))
}

// sseNoEventServer returns an httptest.Server that accepts SSE connections
// but never sends any events, causing the client to block until context cancel.
// The server registers its own cleanup to ensure the handler unblocks before
// Close is called, which is necessary on Windows where the TCP stack does not
// promptly notify the server of client-side connection closure.
func sseNoEventServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		select {
		case <-r.Context().Done():
		case <-ctx.Done():
		}
	}))
	t.Cleanup(func() {
		cancel()
		srv.Close()
	})
	return srv
}

func TestApproveCmd_SSE_HappyPath(t *testing.T) {
	cfg, _, fileSvc := cmdtest.SetupApproveSSETestEnv(t)

	srv := sseApproveServer(t, "user-test", "txhash123")
	t.Cleanup(srv.Close)
	cmdtest.WithEndpointOverride(t, srv.URL)

	mockClient := &cmdtest.MockAPIClient{
		GetResp: []byte(`{"status":"approved","result_summary":"success"}`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (APIClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory, cmdtest.FileSvcFactoryFor(fileSvc), func(string) error { return nil })
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash123"})
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "approved successfully")
	assert.Contains(t, buf.String(), "txhash123")
}

func TestApproveCmd_SSE_Timeout(t *testing.T) {
	cfg, _, fileSvc := cmdtest.SetupApproveSSETestEnv(t)

	srv := sseNoEventServer(t)
	cmdtest.WithEndpointOverride(t, srv.URL)

	mockClient := &cmdtest.MockAPIClient{
		GetResp: []byte(`{"status":"approved"}`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (APIClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory, cmdtest.FileSvcFactoryFor(fileSvc), func(string) error { return nil })
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, []string{"txhash123"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestApproveCmd_SSE_Success_GetError(t *testing.T) {
	cfg, _, fileSvc := cmdtest.SetupApproveSSETestEnv(t)

	srv := sseApproveServer(t, "user-test", "txhash123")
	t.Cleanup(srv.Close)
	cmdtest.WithEndpointOverride(t, srv.URL)

	mockClient := &cmdtest.MockAPIClient{
		GetErr: fmt.Errorf("network failure"),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (APIClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory, cmdtest.FileSvcFactoryFor(fileSvc), func(string) error { return nil })
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash123"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify status")
}

func TestApproveCmd_SSE_Success_InvalidJSONStatus(t *testing.T) {
	cfg, _, fileSvc := cmdtest.SetupApproveSSETestEnv(t)

	srv := sseApproveServer(t, "user-test", "txhash123")
	t.Cleanup(srv.Close)
	cmdtest.WithEndpointOverride(t, srv.URL)

	mockClient := &cmdtest.MockAPIClient{
		GetResp: []byte(`not json {{{`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (APIClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory, cmdtest.FileSvcFactoryFor(fileSvc), func(string) error { return nil })
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash123"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse status response")
}

func TestApproveCmd_SSE_Success_EmptyStatus(t *testing.T) {
	cfg, _, fileSvc := cmdtest.SetupApproveSSETestEnv(t)

	srv := sseApproveServer(t, "user-test", "txhash456")
	t.Cleanup(srv.Close)
	cmdtest.WithEndpointOverride(t, srv.URL)

	mockClient := &cmdtest.MockAPIClient{
		GetResp: []byte(`{}`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (APIClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory, cmdtest.FileSvcFactoryFor(fileSvc), func(string) error { return nil })
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash456"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status")
	assert.Contains(t, buf.String(), "txhash456")
}

func TestApproveCmd_SSE_Success_StatusNotApproved(t *testing.T) {
	cfg, _, fileSvc := cmdtest.SetupApproveSSETestEnv(t)

	srv := sseApproveServer(t, "user-test", "txhash789")
	t.Cleanup(srv.Close)
	cmdtest.WithEndpointOverride(t, srv.URL)

	mockClient := &cmdtest.MockAPIClient{
		GetResp: []byte(`{"status":"pending"}`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (APIClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory, cmdtest.FileSvcFactoryFor(fileSvc), func(string) error { return nil })
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash789"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status")
}
