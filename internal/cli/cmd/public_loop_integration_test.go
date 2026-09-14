//go:build integration

package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestVerifyPublicLoopPublicSurface_RequiresMutationRoutesAbsent(t *testing.T) {
	tests := []struct {
		name      string
		handler   http.Handler
		wantError bool
	}{
		{name: "read-only surface returns not found", handler: http.NotFoundHandler()},
		{name: "private surface authentication response is rejected", handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusUnauthorized) }), wantError: true},
		{name: "private surface authorization response is rejected", handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusForbidden) }), wantError: true},
		{name: "registered route method response is rejected", handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusMethodNotAllowed) }), wantError: true},
		{name: "successful mutation response is rejected", handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusOK) }), wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			t.Cleanup(server.Close)
			err := verifyPublicLoopMutationRoutesAbsent(context.Background(), server)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

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
			{Components: []string{"ensemble"}, ImageID: "sha256:" + strings.Repeat("5", 64)},
			{Components: []string{"dashboard"}, ImageID: "sha256:" + strings.Repeat("6", 64)},
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
	reproducedHash, err := canonicalContentHash(evidence)
	require.NoError(t, err)
	assert.Equal(t, reproducedHash, evidence.ContentHash)
}
