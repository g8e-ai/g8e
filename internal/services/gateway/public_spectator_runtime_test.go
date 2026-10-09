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
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestValidatePublicFeedSourceID_RejectsPathComponents(t *testing.T) {
	for _, sourceID := range []string{"../source", "source/id", ".", "source id"} {
		t.Run(sourceID, func(t *testing.T) {
			assert.ErrorIs(t, ValidatePublicFeedSourceID(sourceID), constants.ErrPublicFeedSourceIDInvalid)
		})
	}
}

func TestTransitionLocalPublicFeed_ArchivesPublisherStateAndStartsFreshSource(t *testing.T) {
	fileSvc := newProducerFileSvc(t)
	ks := newTestKeystore(t, fileSvc, testutil.NewTestLogger())
	ctx := context.Background()
	oldConfig, err := EnsureLocalPublicFeed(ctx, fileSvc, ks, "source-old", "http://127.0.0.1:8081")
	require.NoError(t, err)
	oldKey, err := fileSvc.ReadFile(ctx, constants.PublicFeedSigningKeyPath)
	require.NoError(t, err)
	oldToken, err := fileSvc.ReadFile(ctx, constants.PublicFeedIngestTokenPath)
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(ctx, constants.PublicFeedOutboxPath, []byte("old-outbox\n"), constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(ctx, constants.PublicFeedSnapshotPath, []byte("old-snapshot\n"), constants.PermFilePrivate))

	newConfig, err := TransitionLocalPublicFeed(ctx, fileSvc, ks, "source-new")
	require.NoError(t, err)
	assert.Equal(t, "source-new", newConfig.SourceID)
	assert.Equal(t, oldConfig.MirrorOrigin, newConfig.MirrorOrigin)
	assert.NotEqual(t, oldConfig.SigningKeyID, newConfig.SigningKeyID)
	archivedPaths, err := PublicFeedArchivePathsFor(oldConfig.SourceID)
	require.NoError(t, err)

	archivedConfigBytes, err := fileSvc.ReadFile(ctx, archivedPaths.ExportConfig)
	require.NoError(t, err)
	var archivedConfig models.PublicExportConfig
	require.NoError(t, json.Unmarshal(archivedConfigBytes, &archivedConfig))
	assert.Equal(t, oldConfig, archivedConfig)
	archivedKey, err := fileSvc.ReadFile(ctx, archivedPaths.SigningKey)
	require.NoError(t, err)
	assert.Equal(t, oldKey, archivedKey)
	archivedToken, err := fileSvc.ReadFile(ctx, archivedPaths.IngestToken)
	require.NoError(t, err)
	assert.Equal(t, oldToken, archivedToken)
	archivedOutbox, err := fileSvc.ReadFile(ctx, archivedPaths.Outbox)
	require.NoError(t, err)
	assert.Equal(t, "old-outbox\n", string(archivedOutbox))
	archivedSnapshot, err := fileSvc.ReadFile(ctx, archivedPaths.Snapshot)
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

func TestTransitionLocalPublicFeed_MigratesLegacyArchiveWithoutDiscardingIt(t *testing.T) {
	fileSvc := newProducerFileSvc(t)
	ks := newTestKeystore(t, fileSvc, testutil.NewTestLogger())
	ctx := context.Background()
	activeConfig, err := EnsureLocalPublicFeed(ctx, fileSvc, ks, "source-current", "http://127.0.0.1:8081")
	require.NoError(t, err)
	legacyConfig := activeConfig
	legacyConfig.SourceID = "source-legacy"
	legacyConfigBytes, err := json.Marshal(legacyConfig)
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(ctx, constants.PublicFeedLegacyArchiveExportConfigPath, legacyConfigBytes, constants.PermFilePrivate))
	activeKey, err := fileSvc.ReadFile(ctx, constants.PublicFeedSigningKeyPath)
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(ctx, constants.PublicFeedLegacyArchiveSigningKeyPath, activeKey, constants.PermFilePrivate))
	activeToken, err := fileSvc.ReadFile(ctx, constants.PublicFeedIngestTokenPath)
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(ctx, constants.PublicFeedLegacyArchiveIngestTokenPath, activeToken, constants.PermFilePrivate))

	_, err = TransitionLocalPublicFeed(ctx, fileSvc, ks, "source-next")
	require.NoError(t, err)
	legacyPaths, err := PublicFeedArchivePathsFor("source-legacy")
	require.NoError(t, err)
	legacyBody, err := fileSvc.ReadFile(ctx, legacyPaths.ExportConfig)
	require.NoError(t, err)
	assert.Equal(t, string(legacyConfigBytes), string(legacyBody))
	currentPaths, err := PublicFeedArchivePathsFor("source-current")
	require.NoError(t, err)
	_, err = fileSvc.ReadFile(ctx, currentPaths.ExportConfig)
	assert.NoError(t, err)
}

func TestTransitionLocalPublicFeed_PreservesEveryImmutableGeneration(t *testing.T) {
	fileSvc := newProducerFileSvc(t)
	ks := newTestKeystore(t, fileSvc, testutil.NewTestLogger())
	ctx := context.Background()
	_, err := EnsureLocalPublicFeed(ctx, fileSvc, ks, "source-one", "http://127.0.0.1:8081")
	require.NoError(t, err)

	_, err = TransitionLocalPublicFeed(ctx, fileSvc, ks, "source-two")
	require.NoError(t, err)
	firstGeneration, err := PublicFeedArchivePathsFor("source-one")
	require.NoError(t, err)
	firstConfig, err := fileSvc.ReadFile(ctx, firstGeneration.ExportConfig)
	require.NoError(t, err)

	_, err = TransitionLocalPublicFeed(ctx, fileSvc, ks, "source-three")
	require.NoError(t, err)
	secondGeneration, err := PublicFeedArchivePathsFor("source-two")
	require.NoError(t, err)
	secondConfig, err := fileSvc.ReadFile(ctx, secondGeneration.ExportConfig)
	require.NoError(t, err)

	assert.NotEmpty(t, firstConfig)
	assert.NotEmpty(t, secondConfig)
	assert.NotEqual(t, firstGeneration.Root, secondGeneration.Root)
	_, err = TransitionLocalPublicFeed(ctx, fileSvc, ks, "source-one")
	assert.ErrorIs(t, err, constants.ErrPublicFeedArchiveGenerationExists)
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

// getSpectatorBody issues one GET against a listener PublicSpectatorRuntime.Start
// has already bound. Start returning is the readiness signal, so there is
// nothing to poll: the connection queues on the socket until it is served.
func getSpectatorBody(t *testing.T, url string) (int, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, body
}

func requireBootstrapHealthy(t *testing.T, url string) {
	t.Helper()
	status, body := getSpectatorBody(t, url)
	require.Equal(t, http.StatusOK, status, "bootstrap at %s: %s", url, body)
}

func requireExplorerRuntime(t *testing.T, url, expectedMirrorOrigin string) {
	t.Helper()
	status, body := getSpectatorBody(t, url)
	require.Equal(t, http.StatusOK, status, "explorer runtime at %s: %s", url, body)
	var payload map[string]string
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, expectedMirrorOrigin, payload["mirror_origin"])
}
