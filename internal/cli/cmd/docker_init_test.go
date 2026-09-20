// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.0.

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestDockerInit_MissingComposeFileReturnsError(t *testing.T) {
	chdirTemp(t)

	cmd := dockerInitCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.True(t, isNotFoundErr(err))
}

func TestReadDotEnvFile_ParsesValues(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".env")
	content := `# comment
G8E_OLLAMA_ENDPOINT=http://192.168.1.2:11434
G8E_INFERENCE_CAMPAIGN_ID=eval-smoke-mini
G8E_INFERENCE_MODEL_REGISTRY_DIGEST=abc123
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	values, err := readDotEnvFile(path)
	require.NoError(t, err)
	assert.Equal(t, "http://192.168.1.2:11434", values["G8E_OLLAMA_ENDPOINT"])
	assert.Equal(t, "eval-smoke-mini", values["G8E_INFERENCE_CAMPAIGN_ID"])
	assert.Equal(t, "abc123", values["G8E_INFERENCE_MODEL_REGISTRY_DIGEST"])
}

func TestCheckDockerInitEnv_MissingFile(t *testing.T) {
	chdirTemp(t)

	err := checkDockerInitEnv()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrDockerInitEnvRequired)
}

func TestCheckDockerInitEnv_MissingRequiredKeys(t *testing.T) {
	tmpDir := chdirTemp(t)
	content := "G8E_INFERENCE_CAMPAIGN_ID=eval-smoke-mini\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(content), 0o644))

	err := checkDockerInitEnv()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrDockerInitEnvRequired)
	assert.Contains(t, err.Error(), "G8E_OLLAMA_ENDPOINT")
}

func TestCheckDockerInitEnv_Succeeds(t *testing.T) {
	tmpDir := chdirTemp(t)
	content := "G8E_OLLAMA_ENDPOINT=http://192.168.1.2:11434\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(content), 0o644))

	err := checkDockerInitEnv()
	assert.NoError(t, err)
}

func TestIsInferenceOperatorPendingRequest(t *testing.T) {
	dataReq := models.PlatformEnrollmentPendingRequest{
		ComponentKind: models.PlatformComponentOperator,
		InstanceID:    "operator-a1b2c3d4",
		Hostname:      "a1b2c3d4",
	}
	inferenceReq := models.PlatformEnrollmentPendingRequest{
		ComponentKind: models.PlatformComponentOperator,
		InstanceID:    "operator-inference-operator",
		Hostname:      "inference-operator",
	}

	assert.False(t, isInferenceOperatorPendingRequest(&dataReq))
	assert.True(t, isInferenceOperatorPendingRequest(&inferenceReq))
}

func TestNextDockerInitApprovalSlot(t *testing.T) {
	assert.Equal(t, 1, nextDockerInitApprovalSlot(nil))
	assert.Equal(t, 2, nextDockerInitApprovalSlot(map[int]struct{}{1: {}}))
	assert.Equal(t, 0, nextDockerInitApprovalSlot(map[int]struct{}{1: {}, 2: {}, 3: {}, 4: {}}))
}

func TestSelectDockerInitApprovalCandidate(t *testing.T) {
	inference := models.PlatformEnrollmentPendingRequest{
		RequestID:     "inf-1",
		ComponentKind: models.PlatformComponentOperator,
		Hostname:      "inference-operator",
	}
	dataOp := models.PlatformEnrollmentPendingRequest{
		RequestID:     "op-1",
		ComponentKind: models.PlatformComponentOperator,
		Hostname:      "g8e-operator",
	}
	pending := []models.PlatformEnrollmentPendingRequest{inference, dataOp}

	assert.Nil(t, selectDockerInitApprovalCandidate(pending, 2))
	assert.Equal(t, "op-1", selectDockerInitApprovalCandidate(pending, 1).RequestID)
	assert.Equal(t, "inf-1", selectDockerInitApprovalCandidate(pending, 4).RequestID)
}

func TestPlatformEnrollmentApprovalRank(t *testing.T) {
	tests := []struct {
		name string
		req  models.PlatformEnrollmentPendingRequest
		want int
	}{
		{
			name: "data operator first",
			req: models.PlatformEnrollmentPendingRequest{
				ComponentKind: models.PlatformComponentOperator,
				InstanceID:    "operator-host",
			},
			want: 1,
		},
		{
			name: "dashboard second",
			req: models.PlatformEnrollmentPendingRequest{
				ComponentKind: models.PlatformComponentDashboard,
			},
			want: 2,
		},
		{
			name: "ensemble third",
			req: models.PlatformEnrollmentPendingRequest{
				ComponentKind: models.PlatformComponentEnsemble,
			},
			want: 3,
		},
		{
			name: "inference operator last",
			req: models.PlatformEnrollmentPendingRequest{
				ComponentKind: models.PlatformComponentOperator,
				Hostname:      "inference-operator",
			},
			want: 4,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, platformEnrollmentApprovalRank(tc.req))
		})
	}
}

func TestDockerOwnerEnrollmentOptions(t *testing.T) {
	t.Run("default uses passkey ceremony", func(t *testing.T) {
		opts := dockerOwnerEnrollmentOptions(false)
		assert.False(t, opts.Headless)
		assert.False(t, opts.NoSystemTrust)
	})
	t.Run("headless skips passkey and OS trust", func(t *testing.T) {
		opts := dockerOwnerEnrollmentOptions(true)
		assert.True(t, opts.Headless)
		assert.True(t, opts.NoSystemTrust)
	})
}

func TestDockerFullStackProfiles(t *testing.T) {
	assert.Equal(t, []string{
		constants.DockerBootstrappedProfile,
		constants.DockerEvaluationProfile,
	}, dockerFullStackProfiles())
}

func TestReportDockerPublicSpectatorReady_PrintsBootstrapSequence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/bootstrap", r.URL.Path)
		_ = json.NewEncoder(w).Encode(models.PublicFeedBootstrap{
			Snapshot: models.PublicFeedSnapshot{HighWaterSequence: 42},
		})
	}))
	defer server.Close()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	originalURL := publicMirrorBootstrapURL
	publicMirrorBootstrapURL = server.URL + "/bootstrap"
	defer func() { publicMirrorBootstrapURL = originalURL }()

	require.NoError(t, reportDockerPublicSpectatorReady(cmd))
	assert.Contains(t, buf.String(), "high_water_sequence=42")
}
