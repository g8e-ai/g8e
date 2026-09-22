// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestPublicSpectatorRuntime_StartsMirrorListeners(t *testing.T) {
	privatePort := mustFreePort(t)
	publicPort := mustFreePort(t)
	fileSvc := newProducerFileSvc(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime, err := NewPublicSpectatorRuntime(PublicSpectatorConfig{
		Enabled:               true,
		PrivateListenAddress:  "127.0.0.1:" + privatePort,
		PublicListenAddress:   "127.0.0.1:" + publicPort,
		ExplorerListenAddress: "",
	}, fileSvc, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, runtime.Start(ctx))

	waitForBootstrap(t, "http://127.0.0.1:"+publicPort+"/bootstrap")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	require.NoError(t, runtime.Stop(stopCtx))
}

func TestPublicSpectatorRuntime_StartsDedicatedExplorerListener(t *testing.T) {
	privatePort := mustFreePort(t)
	publicPort := mustFreePort(t)
	explorerPort := mustFreePort(t)
	fileSvc := newProducerFileSvc(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime, err := NewPublicSpectatorRuntime(PublicSpectatorConfig{
		Enabled:               true,
		PrivateListenAddress:  "127.0.0.1:" + privatePort,
		PublicListenAddress:   "127.0.0.1:" + publicPort,
		ExplorerListenAddress: "127.0.0.1:" + explorerPort,
	}, fileSvc, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, runtime.Start(ctx))

	waitForBootstrap(t, "http://127.0.0.1:"+publicPort+"/bootstrap")
	waitForExplorerRuntime(t, "http://127.0.0.1:"+explorerPort+"/runtime.json", "http://127.0.0.1:"+publicPort)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	require.NoError(t, runtime.Stop(stopCtx))
}

func TestTransitionLocalPublicFeed_ArchivesPublisherStateAndStartsFreshSource(t *testing.T) {
	fileSvc := newProducerFileSvc(t)
	ctx := context.Background()
	oldConfig, err := EnsureLocalPublicFeed(ctx, fileSvc, "source-old", "http://127.0.0.1:8081")
	require.NoError(t, err)
	oldKey, err := fileSvc.ReadFile(ctx, constants.PublicFeedSigningKeyPath)
	require.NoError(t, err)
	oldToken, err := fileSvc.ReadFile(ctx, constants.PublicFeedIngestTokenPath)
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(ctx, constants.PublicFeedOutboxPath, []byte("old-outbox\n"), constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(ctx, constants.PublicFeedSnapshotPath, []byte("old-snapshot\n"), constants.PermFilePrivate))

	newConfig, err := TransitionLocalPublicFeed(ctx, fileSvc, "source-new")
	require.NoError(t, err)
	assert.Equal(t, "source-new", newConfig.SourceID)
	assert.Equal(t, oldConfig.MirrorOrigin, newConfig.MirrorOrigin)
	assert.NotEqual(t, oldConfig.SigningKeyID, newConfig.SigningKeyID)

	archivedConfigBytes, err := fileSvc.ReadFile(ctx, constants.PublicFeedArchiveExportConfigPath)
	require.NoError(t, err)
	var archivedConfig models.PublicExportConfig
	require.NoError(t, json.Unmarshal(archivedConfigBytes, &archivedConfig))
	assert.Equal(t, oldConfig, archivedConfig)
	archivedKey, err := fileSvc.ReadFile(ctx, constants.PublicFeedArchiveSigningKeyPath)
	require.NoError(t, err)
	assert.Equal(t, oldKey, archivedKey)
	archivedToken, err := fileSvc.ReadFile(ctx, constants.PublicFeedArchiveIngestTokenPath)
	require.NoError(t, err)
	assert.Equal(t, oldToken, archivedToken)
	archivedOutbox, err := fileSvc.ReadFile(ctx, constants.PublicFeedArchiveOutboxPath)
	require.NoError(t, err)
	assert.Equal(t, "old-outbox\n", string(archivedOutbox))
	archivedSnapshot, err := fileSvc.ReadFile(ctx, constants.PublicFeedArchiveSnapshotPath)
	require.NoError(t, err)
	assert.Equal(t, "old-snapshot\n", string(archivedSnapshot))

	newKey, err := fileSvc.ReadFile(ctx, constants.PublicFeedSigningKeyPath)
	require.NoError(t, err)
	assert.NotEqual(t, oldKey, newKey)
	newToken, err := fileSvc.ReadFile(ctx, constants.PublicFeedIngestTokenPath)
	require.NoError(t, err)
	assert.NotEqual(t, oldToken, newToken)
	outboxExists, err := fileSvc.FileExists(ctx, constants.PublicFeedOutboxPath)
	require.NoError(t, err)
	assert.False(t, outboxExists)
	snapshotExists, err := fileSvc.FileExists(ctx, constants.PublicFeedSnapshotPath)
	require.NoError(t, err)
	assert.False(t, snapshotExists)
}

func TestValidatePublicMirrorListenAddresses_RejectsDuplicate(t *testing.T) {
	err := ValidatePublicMirrorListenAddresses("127.0.0.1:8081", "127.0.0.1:8081", false)
	assert.ErrorIs(t, err, constants.ErrPublicFeedListenAddress)
}

func TestValidatePublicMirrorListenAddresses_AllowsContainerBind(t *testing.T) {
	err := ValidatePublicMirrorListenAddresses("0.0.0.0:8081", "0.0.0.0:8082", true)
	assert.NoError(t, err)
	err = ValidatePublicMirrorListenAddresses("0.0.0.0:8081", "0.0.0.0:8082", false)
	assert.ErrorIs(t, err, constants.ErrPublicFeedListenAddress)
}

func mustFreePort(t *testing.T) string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	return port
}

func waitForBootstrap(t *testing.T, url string) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("bootstrap never became healthy at %s", url)
}

func waitForExplorerRuntime(t *testing.T, url, expectedMirrorOrigin string) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode == http.StatusOK && readErr == nil {
				var payload map[string]string
				if json.Unmarshal(body, &payload) == nil && payload["mirror_origin"] == expectedMirrorOrigin {
					return
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("explorer runtime never became healthy at %s", url)
}
