// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package cmd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/gateway"
)

type publicFailOnceFileSvc struct {
	fs.RuntimeFileService
	path      string
	remaining int
}

func (s *publicFailOnceFileSvc) WriteFile(ctx context.Context, relPath string, data []byte, mode os.FileMode) error {
	if relPath == s.path && s.remaining > 0 {
		s.remaining--
		return fmt.Errorf("injected write failure")
	}
	return s.RuntimeFileService.WriteFile(ctx, relPath, data, mode)
}

func validPublicProjectionBytes(campaignID string) string {
	return fmt.Sprintf(`{"schema_version":"1.3.0","kind":"catalog_snapshot","dataset_id":"%s","quality_state":"live_in_progress","observed_at":"2026-09-21T00:00:00Z"}`, campaignID)
}

func TestPublicCmd_ExposesProductionSurface(t *testing.T) {
	cmd := publicCmd()
	expected := []string{"config", "init", "publish", "push", "repair-outbox", "rotate-key", "source", "status"}
	require.Len(t, cmd.Commands(), len(expected))
	for _, name := range expected {
		child, _, err := cmd.Find([]string{name})
		require.NoError(t, err)
		assert.Equal(t, name, child.Name())
	}
}

func TestPublicInitCmd_PersistsPrivateConfigurationAndSecrets(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	cmd := publicInitCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	cmd.SetArgs([]string{"--source-id", "deployment-a", "--mirror-origin", "https://mirror.example"})

	require.NoError(t, cmd.Execute())

	configBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedExportConfigPath)
	require.NoError(t, err)
	var exportConfig models.PublicExportConfig
	require.NoError(t, json.Unmarshal(configBytes, &exportConfig))
	assert.True(t, exportConfig.Enabled)
	assert.Equal(t, "deployment-a", exportConfig.SourceID)
	assert.Equal(t, "https://mirror.example", exportConfig.MirrorOrigin)
	assert.NotEmpty(t, exportConfig.SigningKeyID)

	keyBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedSigningKeyPath)
	require.NoError(t, err)
	decodedKey, err := hex.DecodeString(string(keyBytes))
	require.NoError(t, err)
	assert.Len(t, decodedKey, ed25519.PrivateKeySize)

	tokenBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedIngestTokenPath)
	require.NoError(t, err)
	decodedToken, err := hex.DecodeString(string(tokenBytes))
	require.NoError(t, err)
	assert.Len(t, decodedToken, constants.PublicFeedIngestTokenBytes)

	for _, relPath := range []string{constants.PublicFeedExportConfigPath, constants.PublicFeedSigningKeyPath, constants.PublicFeedIngestTokenPath} {
		info, statErr := fileSvc.Stat(context.Background(), relPath)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(constants.PermFilePrivate), info.Mode().Perm())
	}
}

func TestPublicPublishAndRotateKeyCommands_DeriveDurableSequenceAndAuthenticateMirror(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	var received []models.PublicFeedBatch
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedIngestTokenPath)
		assert.NoError(t, err)
		assert.Equal(t, "Bearer "+string(token), r.Header.Get("Authorization"))
		if r.URL.Path == "/keys/register" {
			assert.NoError(t, json.NewEncoder(w).Encode(models.PublicKeyRegistrationResponse{Accepted: true}))
			return
		}
		var request models.PublicIngestRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		received = append(received, request.Batch)
		assert.NoError(t, json.NewEncoder(w).Encode(models.PublicIngestResponse{
			Accepted:          true,
			HighWaterSequence: request.Batch.LastSequence,
			FeedChainHash:     request.Batch.ContentHash,
		}))
	}))
	t.Cleanup(mirror.Close)

	initCmd := publicInitCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	initCmd.SetArgs([]string{"--source-id", "deployment-a", "--mirror-origin", mirror.URL})
	require.NoError(t, initCmd.Execute())

	input := models.PublicFeedRecordInput{
		RecordType:  models.PublicFeedRecordTypeProjection,
		RecordBytes: validPublicProjectionBytes("campaign-a"),
	}
	inputBytes, err := json.Marshal(input)
	require.NoError(t, err)
	inputBytes = append(inputBytes, '\n')
	inputPath := filepath.Join(cfg.ProjectRoot, constants.TestPublicFeedRecordsFilename)
	require.NoError(t, os.WriteFile(inputPath, inputBytes, constants.PermFilePrivate))

	publishCmd := publicPublishCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	publishCmd.SetArgs([]string{inputPath})
	require.NoError(t, publishCmd.Execute())
	require.Len(t, received, 1)
	assert.Equal(t, int64(1), received[0].FirstSequence)
	assert.Equal(t, int64(1), received[0].LastSequence)
	require.Len(t, received[0].Records, 1)
	assert.Equal(t, int64(1), received[0].Records[0].Sequence)

	keyBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedSigningKeyPath)
	require.NoError(t, err)
	tokenBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedIngestTokenPath)
	require.NoError(t, err)
	var output bytes.Buffer
	statusCmd := publicStatusCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	statusCmd.SetOut(&output)
	require.NoError(t, statusCmd.Execute())
	assert.NotContains(t, output.String(), string(keyBytes))
	assert.NotContains(t, output.String(), string(tokenBytes))
	assert.Contains(t, output.String(), `"high_water_sequence":1`)

	configBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedExportConfigPath)
	require.NoError(t, err)
	var beforeRotation models.PublicExportConfig
	require.NoError(t, json.Unmarshal(configBytes, &beforeRotation))
	rotateCmd := publicRotateKeyCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	require.NoError(t, rotateCmd.Execute())
	require.Len(t, received, 2)
	assert.Equal(t, int64(2), received[1].FirstSequence)
	assert.Equal(t, models.PublicFeedRecordTypeKeyRevocation, received[1].Records[0].RecordType)

	configBytes, err = fileSvc.ReadFile(context.Background(), constants.PublicFeedExportConfigPath)
	require.NoError(t, err)
	var afterRotation models.PublicExportConfig
	require.NoError(t, json.Unmarshal(configBytes, &afterRotation))
	assert.NotEqual(t, beforeRotation.SigningKeyID, afterRotation.SigningKeyID)
	rotatedKeyBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedSigningKeyPath)
	require.NoError(t, err)
	assert.NotEqual(t, keyBytes, rotatedKeyBytes)
}

func TestPublicSourceTransitionCmd_RequiresConfirmationAndArchivesOldSource(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	initCmd := publicInitCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	initCmd.SetArgs([]string{"--source-id", "deployment-old", "--mirror-origin", "https://mirror.example"})
	require.NoError(t, initCmd.Execute())
	require.NoError(t, fileSvc.WriteFile(context.Background(), constants.PublicFeedSnapshotPath, []byte("old-snapshot"), constants.PermFilePrivate))

	transitionCmd := publicSourceTransitionCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	transitionCmd.SetArgs([]string{"--source-id", "deployment-new"})
	err := transitionCmd.Execute()
	require.ErrorIs(t, err, constants.ErrPublicFeedSourceTransitionConfirmation)

	var output bytes.Buffer
	transitionCmd = publicSourceTransitionCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	transitionCmd.SetOut(&output)
	transitionCmd.SetArgs([]string{"--source-id", "deployment-new", "--yes"})
	require.NoError(t, transitionCmd.Execute())
	assert.Contains(t, output.String(), "deployment-new")

	activeConfig, err := readPublicExportConfig(context.Background(), fileSvc)
	require.NoError(t, err)
	assert.Equal(t, "deployment-new", activeConfig.SourceID)
	archivedPaths, err := gateway.PublicFeedArchivePathsFor("deployment-old")
	require.NoError(t, err)
	archivedConfigBytes, err := fileSvc.ReadFile(context.Background(), archivedPaths.ExportConfig)
	require.NoError(t, err)
	var archivedConfig models.PublicExportConfig
	require.NoError(t, json.Unmarshal(archivedConfigBytes, &archivedConfig))
	assert.Equal(t, "deployment-old", archivedConfig.SourceID)
	archivedSnapshot, err := fileSvc.ReadFile(context.Background(), archivedPaths.Snapshot)
	require.NoError(t, err)
	assert.Equal(t, "old-snapshot", string(archivedSnapshot))
}

func TestPublicRotateKeyCmd_PreRegistersNewKeyAndDurablyRevokesOldKey(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	initCmd := publicInitCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	initCmd.SetArgs([]string{"--source-id", "deployment-a", "--mirror-origin", "https://mirror.example"})
	require.NoError(t, initCmd.Execute())

	configBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedExportConfigPath)
	require.NoError(t, err)
	var oldConfig models.PublicExportConfig
	require.NoError(t, json.Unmarshal(configBytes, &oldConfig))
	oldKeyBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedSigningKeyPath)
	require.NoError(t, err)
	oldPrivateKeyBytes, err := hex.DecodeString(string(oldKeyBytes))
	require.NoError(t, err)
	tokenBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedIngestTokenPath)
	require.NoError(t, err)

	mirror, err := gateway.NewPublicMirrorServer(slog.Default(), gateway.NewRuntimePublicMirrorStore(fileSvc), gateway.PublicMirrorConfig{})
	require.NoError(t, err)
	mirror.SetIngestAuthToken(string(tokenBytes))
	require.NoError(t, mirror.RegisterSourceKey(context.Background(), oldConfig.SourceID, oldConfig.SigningKeyID, ed25519.PrivateKey(oldPrivateKeyBytes).Public().(ed25519.PublicKey)))
	server := httptest.NewServer(mirror.Handler())

	setCmd := publicConfigSetCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	setCmd.SetArgs([]string{"--mirror-origin", server.URL})
	require.NoError(t, setCmd.Execute())
	inputPath := filepath.Join(cfg.ProjectRoot, constants.TestPublicFeedRecordsFilename)
	input := models.PublicFeedRecordInput{RecordType: models.PublicFeedRecordTypeProjection, RecordBytes: validPublicProjectionBytes("campaign-a")}
	inputBytes, err := json.Marshal(input)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(inputPath, append(inputBytes, '\n'), constants.PermFilePrivate))
	publishCmd := publicPublishCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	publishCmd.SetArgs([]string{inputPath})
	require.NoError(t, publishCmd.Execute())

	rotateCmd := publicRotateKeyCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	require.NoError(t, rotateCmd.Execute())
	server.Close()

	state, err := gateway.NewRuntimePublicMirrorStore(fileSvc).Load(context.Background())
	require.NoError(t, err)
	require.Len(t, state.KeyRegistry, 2)
	_, revoked := state.RevokedKeys[oldConfig.SourceID+":"+oldConfig.SigningKeyID]
	assert.True(t, revoked)

	restartedMirror, err := gateway.NewPublicMirrorServer(slog.Default(), gateway.NewRuntimePublicMirrorStore(fileSvc), gateway.PublicMirrorConfig{})
	require.NoError(t, err)
	restartedMirror.SetIngestAuthToken(string(tokenBytes))
	restartedServer := httptest.NewServer(restartedMirror.Handler())
	t.Cleanup(restartedServer.Close)
	setCmd = publicConfigSetCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	setCmd.SetArgs([]string{"--mirror-origin", restartedServer.URL})
	require.NoError(t, setCmd.Execute())
	input.RecordBytes = validPublicProjectionBytes("campaign-b")
	inputBytes, err = json.Marshal(input)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(inputPath, append(inputBytes, '\n'), constants.PermFilePrivate))
	publishCmd = publicPublishCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	publishCmd.SetArgs([]string{inputPath})
	require.NoError(t, publishCmd.Execute())

	oldFileSvc, _ := newCmdTestEnv(t)
	oldConfig.MirrorOrigin = restartedServer.URL
	oldPublisher := gateway.NewPublicPublisherService(nil, oldFileSvc, slog.Default(), oldConfig, ed25519.PrivateKey(oldPrivateKeyBytes), oldConfig.SigningKeyID)
	oldPublisher.SetIngestAuthToken(string(tokenBytes))
	oldRecordBytes := validPublicProjectionBytes("old-key")
	oldRecordHash := sha256.Sum256([]byte(oldRecordBytes))
	err = oldPublisher.ExportBatch(context.Background(), []models.PublicFeedRecord{{
		Sequence:    1,
		RecordType:  models.PublicFeedRecordTypeProjection,
		RecordHash:  hex.EncodeToString(oldRecordHash[:]),
		RecordBytes: oldRecordBytes,
	}})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedMirrorRejected)
}

func TestPublicInitCmd_RollsBackPartialSecretStateWhenConfigurationWriteFails(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	failingFileSvc := &publicFailOnceFileSvc{RuntimeFileService: fileSvc, path: constants.PublicFeedExportConfigPath, remaining: 1}
	cmd := publicInitCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(failingFileSvc))
	cmd.SetArgs([]string{"--source-id", "deployment-a", "--mirror-origin", "https://mirror.example"})

	err := cmd.Execute()
	require.Error(t, err)
	for _, relPath := range []string{constants.PublicFeedExportConfigPath, constants.PublicFeedSigningKeyPath, constants.PublicFeedIngestTokenPath} {
		exists, existsErr := fileSvc.FileExists(context.Background(), relPath)
		require.NoError(t, existsErr)
		assert.False(t, exists)
	}
}

func TestPublicRotateKeyCmd_RecoversAfterLocalConfigurationPersistenceFailure(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	initCmd := publicInitCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	initCmd.SetArgs([]string{"--source-id", "deployment-a", "--mirror-origin", "https://mirror.example"})
	require.NoError(t, initCmd.Execute())
	configBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedExportConfigPath)
	require.NoError(t, err)
	var oldConfig models.PublicExportConfig
	require.NoError(t, json.Unmarshal(configBytes, &oldConfig))
	keyBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedSigningKeyPath)
	require.NoError(t, err)
	privateKeyBytes, err := hex.DecodeString(string(keyBytes))
	require.NoError(t, err)
	tokenBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedIngestTokenPath)
	require.NoError(t, err)
	mirror, err := gateway.NewPublicMirrorServer(slog.Default(), gateway.NewRuntimePublicMirrorStore(fileSvc), gateway.PublicMirrorConfig{})
	require.NoError(t, err)
	mirror.SetIngestAuthToken(string(tokenBytes))
	require.NoError(t, mirror.RegisterSourceKey(context.Background(), oldConfig.SourceID, oldConfig.SigningKeyID, ed25519.PrivateKey(privateKeyBytes).Public().(ed25519.PublicKey)))
	server := httptest.NewServer(mirror.Handler())
	t.Cleanup(server.Close)
	setCmd := publicConfigSetCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	setCmd.SetArgs([]string{"--mirror-origin", server.URL})
	require.NoError(t, setCmd.Execute())

	failingFileSvc := &publicFailOnceFileSvc{RuntimeFileService: fileSvc, path: constants.PublicFeedExportConfigPath, remaining: 1}
	rotateCmd := publicRotateKeyCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(failingFileSvc))
	err = rotateCmd.Execute()
	require.Error(t, err)
	exists, err := fileSvc.FileExists(context.Background(), constants.PublicFeedKeyRotationPath)
	require.NoError(t, err)
	assert.True(t, exists)
	state, err := gateway.NewRuntimePublicMirrorStore(fileSvc).Load(context.Background())
	require.NoError(t, err)
	require.Len(t, state.Sources[oldConfig.SourceID].Batches, 1)
	_, revoked := state.RevokedKeys[oldConfig.SourceID+":"+oldConfig.SigningKeyID]
	assert.True(t, revoked)

	rotateCmd = publicRotateKeyCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	require.NoError(t, rotateCmd.Execute())
	exists, err = fileSvc.FileExists(context.Background(), constants.PublicFeedKeyRotationPath)
	require.NoError(t, err)
	assert.False(t, exists)
	state, err = gateway.NewRuntimePublicMirrorStore(fileSvc).Load(context.Background())
	require.NoError(t, err)
	assert.Len(t, state.Sources[oldConfig.SourceID].Batches, 1)
}

func TestPublicCommands_RejectMalformedTrailingUnknownAndInvalidConfiguration(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	valid := models.DefaultPublicExportConfig()
	valid.Enabled = true
	valid.SourceID = "deployment-a"
	valid.MirrorOrigin = "https://mirror.example"
	valid.SigningKeyID = "key-a"
	validBytes, err := json.Marshal(valid)
	require.NoError(t, err)
	unknown := append([]byte(nil), validBytes[:len(validBytes)-1]...)
	unknown = append(unknown, []byte(`,"unexpected":true}`)...)
	invalid := valid
	invalid.BatchMaxRecords = 0
	invalidBytes, err := json.Marshal(invalid)
	require.NoError(t, err)
	tests := []struct {
		name string
		body []byte
	}{
		{name: "malformed JSON", body: []byte(`{`)},
		{name: "trailing JSON", body: append(append([]byte(nil), validBytes...), []byte(`{}`)...)},
		{name: "unknown field", body: unknown},
		{name: "invalid limits", body: invalidBytes},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			require.NoError(t, fileSvc.WriteFile(context.Background(), constants.PublicFeedExportConfigPath, testCase.body, constants.PermFilePrivate))
			cmd := publicStatusCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
			err := cmd.Execute()
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrPublicFeedConfigRequired)
		})
	}
}

func TestPublicPushCmd_RetriesDurableOutboxAfterCommandRestart(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	failedMirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	initCmd := publicInitCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	initCmd.SetArgs([]string{"--source-id", "deployment-a", "--mirror-origin", failedMirror.URL})
	require.NoError(t, initCmd.Execute())
	configBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedExportConfigPath)
	require.NoError(t, err)
	var exportConfig models.PublicExportConfig
	require.NoError(t, json.Unmarshal(configBytes, &exportConfig))
	exportConfig.RetryMaxAttempts = 1
	require.NoError(t, writePublicExportConfig(context.Background(), fileSvc, exportConfig))
	input := models.PublicFeedRecordInput{RecordType: models.PublicFeedRecordTypeProjection, RecordBytes: validPublicProjectionBytes("campaign-a")}
	inputBytes, err := json.Marshal(input)
	require.NoError(t, err)
	inputPath := filepath.Join(cfg.ProjectRoot, constants.TestPublicFeedRecordsFilename)
	require.NoError(t, os.WriteFile(inputPath, append(inputBytes, '\n'), constants.PermFilePrivate))
	publishCmd := publicPublishCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	publishCmd.SetArgs([]string{inputPath})
	require.Error(t, publishCmd.Execute())
	failedMirror.Close()

	keyBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedSigningKeyPath)
	require.NoError(t, err)
	privateKeyBytes, err := hex.DecodeString(string(keyBytes))
	require.NoError(t, err)
	tokenBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedIngestTokenPath)
	require.NoError(t, err)
	mirror, err := gateway.NewPublicMirrorServer(slog.Default(), gateway.NewRuntimePublicMirrorStore(fileSvc), gateway.PublicMirrorConfig{})
	require.NoError(t, err)
	mirror.SetIngestAuthToken(string(tokenBytes))
	require.NoError(t, mirror.RegisterSourceKey(context.Background(), exportConfig.SourceID, exportConfig.SigningKeyID, ed25519.PrivateKey(privateKeyBytes).Public().(ed25519.PublicKey)))
	server := httptest.NewServer(mirror.Handler())
	t.Cleanup(server.Close)
	setCmd := publicConfigSetCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	setCmd.SetArgs([]string{"--mirror-origin", server.URL})
	require.NoError(t, setCmd.Execute())
	pushCmd := publicPushCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	require.NoError(t, pushCmd.Execute())
	state, err := gateway.NewRuntimePublicMirrorStore(fileSvc).Load(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), state.Sources[exportConfig.SourceID].HighWaterSequence)
	assert.Len(t, state.Sources[exportConfig.SourceID].Batches, 1)
}

func TestPublicPushCmd_AuthenticatesAndPublishesProofPackage(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	initCmd := publicInitCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	initCmd.SetArgs([]string{"--source-id", "deployment-a", "--mirror-origin", "https://mirror.example"})
	require.NoError(t, initCmd.Execute())
	exportConfig, err := readPublicExportConfig(context.Background(), fileSvc)
	require.NoError(t, err)
	keyBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedSigningKeyPath)
	require.NoError(t, err)
	privateKeyBytes, err := hex.DecodeString(string(keyBytes))
	require.NoError(t, err)
	tokenBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedIngestTokenPath)
	require.NoError(t, err)
	mirror, err := gateway.NewPublicMirrorServer(slog.Default(), gateway.NewRuntimePublicMirrorStore(fileSvc), gateway.PublicMirrorConfig{})
	require.NoError(t, err)
	mirror.SetIngestAuthToken(string(tokenBytes))
	require.NoError(t, mirror.RegisterSourceKey(context.Background(), exportConfig.SourceID, exportConfig.SigningKeyID, ed25519.PrivateKey(privateKeyBytes).Public().(ed25519.PublicKey)))
	server := httptest.NewServer(mirror.Handler())
	t.Cleanup(server.Close)
	exportConfig.MirrorOrigin = server.URL
	require.NoError(t, writePublicExportConfig(context.Background(), fileSvc, exportConfig))
	publisher, err := newPublicPublisherForCommand(context.Background(), fileSvc, exportConfig)
	require.NoError(t, err)
	_, err = publisher.BuildProofPackage(context.Background(), "campaign-a", "revision-a", "verified-index-hash-a", true, []gateway.ProofArtifactInput{{
		Filename:   "proof-a.json",
		MediaType:  "application/json",
		Content:    []byte(`{"campaign_id":"campaign-a"}`),
		CampaignID: "campaign-a",
	}})
	require.NoError(t, err)
	manifest, err := publisher.BuildProofPackage(context.Background(), "campaign-b", "revision-b", "verified-index-hash-b", true, []gateway.ProofArtifactInput{{
		Filename:   "proof-b.json",
		MediaType:  "application/json",
		Content:    []byte(`{"campaign_id":"campaign-b"}`),
		CampaignID: "campaign-b",
	}})
	require.NoError(t, err)
	pushCmd := publicPushCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	require.NoError(t, pushCmd.Execute())
	state, err := gateway.NewRuntimePublicMirrorStore(fileSvc).Load(context.Background())
	require.NoError(t, err)
	assert.Equal(t, manifest.ProofRootSHA256, state.ProofManifests[exportConfig.SourceID].ProofRootSHA256)
	assert.Len(t, state.ProofArtifacts, 2)
}

func TestPublicConfigSetCmd_UpdatesOnlyExplicitFields(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	initial := models.DefaultPublicExportConfig()
	initial.Enabled = true
	initial.SourceID = "deployment-a"
	initial.MirrorOrigin = "https://old.example"
	initial.SigningKeyID = "key-a"
	body, err := json.Marshal(initial)
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), constants.PublicFeedExportConfigPath, body, constants.PermFilePrivate))

	cmd := publicConfigSetCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	cmd.SetArgs([]string{"--mirror-origin", "https://new.example"})
	require.NoError(t, cmd.Execute())

	updatedBytes, err := fileSvc.ReadFile(context.Background(), constants.PublicFeedExportConfigPath)
	require.NoError(t, err)
	var updated models.PublicExportConfig
	require.NoError(t, json.Unmarshal(updatedBytes, &updated))
	assert.Equal(t, "deployment-a", updated.SourceID)
	assert.Equal(t, "https://new.example", updated.MirrorOrigin)
	assert.Equal(t, "key-a", updated.SigningKeyID)
}
