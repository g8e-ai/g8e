//go:build integration

package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestRunPublicLoop_EmitsCandidateBoundZeroInferenceEvidence(t *testing.T) {
	root := testutil.TempDir(t)
	candidatePath := filepath.Join(root, constants.TestQualificationCandidateFilename)
	outputPath := filepath.Join(root, constants.TestPublicLoopEvidenceFilename)
	candidate := publicLoopCandidate{
		SourceTreeHash:              strings.Repeat("1", 64),
		ExecutionSourceManifestHash: strings.Repeat("2", 64),
		BinarySHA256:                strings.Repeat("3", 64),
		Images: []publicLoopCandidateImage{
			{Components: []string{"gateway", "operator"}, ImageID: "sha256:" + strings.Repeat("4", 64)},
		},
	}
	var err error
	candidate.ContentHash, err = canonicalContentHash(candidate)
	require.NoError(t, err)
	candidateBytes, err := json.Marshal(candidate)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(candidatePath, candidateBytes, constants.PermFilePrivate))

	require.NoError(t, runPublicLoop(context.Background(), candidatePath, outputPath))

	evidenceBytes, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	var evidence publicLoopEvidence
	require.NoError(t, json.Unmarshal(evidenceBytes, &evidence))
	assert.Equal(t, candidate.ContentHash, evidence.CandidateContentHash)
	assert.Equal(t, 0, evidence.InferenceInvocations)
	assert.Equal(t, int64(4), evidence.HighWaterSequence)
	assert.True(t, evidence.OldKeyRevoked)
	assert.True(t, evidence.ReplacementKeyAcceptedAfterRestart)
	assert.True(t, evidence.MirrorRestartRecovered)
	assert.Equal(t, 1, evidence.RetryCount)
	assert.NotEmpty(t, evidence.ContentHash)
}
