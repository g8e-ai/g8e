//go:build integration

package cmd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/gateway"
)

func TestPublicCmd_ExposesProductionSurface(t *testing.T) {
	cmd := publicCmd()
	expected := []string{"config", "init", "mirror", "publish", "push", "rotate-key", "status"}
	require.Len(t, cmd.Commands(), len(expected))
	for _, name := range expected {
		child, _, err := cmd.Find([]string{name})
		require.NoError(t, err)
		assert.Equal(t, name, child.Name())
	}
	mirror, _, err := cmd.Find([]string{"mirror"})
	require.NoError(t, err)
	require.NotNil(t, mirror.Commands())
	assert.Equal(t, "run", mirror.Commands()[0].Name())
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
		RecordBytes: `{"campaign_id":"campaign-a"}`,
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

func TestPublicMirrorRunCmd_RegistersSourceKeyAndStopsWithContext(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	initCmd := publicInitCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	initCmd.SetArgs([]string{"--source-id", "deployment-a", "--mirror-origin", "https://mirror.example"})
	require.NoError(t, initCmd.Execute())

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runCmd := publicMirrorRunCmdWithConfig(configLoaderFor(cfg), fileSvcFactoryFor(fileSvc))
	runCmd.SetContext(ctx)
	runCmd.SetArgs([]string{"--listen", "127.0.0.1:0"})
	errCh := make(chan error, 1)
	go func() { errCh <- runCmd.Execute() }()
	require.Eventually(t, func() bool {
		exists, err := fileSvc.FileExists(context.Background(), constants.PublicMirrorStatePath)
		return err == nil && exists
	}, time.Second, 10*time.Millisecond)
	cancel()
	require.NoError(t, <-errCh)

	state, err := gateway.NewRuntimePublicMirrorStore(fileSvc).Load(context.Background())
	require.NoError(t, err)
	assert.Len(t, state.KeyRegistry, 1)
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
