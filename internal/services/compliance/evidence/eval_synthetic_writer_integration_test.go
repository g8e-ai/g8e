//go:build integration

package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

type evalFixtureDirectoryReader struct {
	root fs.FS
}

func (r *evalFixtureDirectoryReader) ReadFile(_ context.Context, path string) ([]byte, error) {
	body, err := fs.ReadFile(r.root, filepath.ToSlash(path))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, constants.ErrNotFound
	}
	return body, err
}

func (r *evalFixtureDirectoryReader) ReadDir(_ context.Context, path string) ([]os.DirEntry, error) {
	entries, err := fs.ReadDir(r.root, filepath.ToSlash(path))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, constants.ErrNotFound
	}
	return entries, err
}

func TestEvalBundleImporter_Import_LoadsRealSyntheticWriterBundle(t *testing.T) {
	ctx := context.Background()
	reader := &evalFixtureDirectoryReader{root: os.DirFS(constants.TestDataDirname)}
	manifestBody, err := reader.ReadFile(ctx, filepath.Join(constants.TestEvalSyntheticGovernanceBundleDirname, constants.EvalRunManifestFilename))
	require.NoError(t, err)
	var manifest evalRunManifest
	require.NoError(t, json.Unmarshal(manifestBody, &manifest))

	nodes, err := NewEvalBundleImporter(reader, manifest.RunID, constants.TestEvalSyntheticGovernanceBundleDirname).Import(ctx)
	require.NoError(t, err)
	assert.Len(t, nodesByType(nodes, ArtifactTypeEvalManifest), 1)
	assert.Len(t, nodesByType(nodes, ArtifactTypeEvalTask), 1)
	assert.Len(t, nodesByType(nodes, ArtifactTypeEvalAttempt), 1)
	receiptNodes := nodesByType(nodes, ArtifactTypeEvalReceipt)
	require.Len(t, receiptNodes, 1)
	assert.Equal(t, VerificationStatusFailed, receiptNodes[0].VerificationStatus)
	assert.Len(t, nodesByType(nodes, ArtifactTypeEvalStage), 0)
	assert.Len(t, nodesByType(nodes, ArtifactTypeEvalMetric), 1)
	assert.Len(t, nodesByType(nodes, ArtifactTypeEvalObservation), 2)
}
