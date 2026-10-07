// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.0.

//go:build integration

package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/gwremote"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

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

	originalURL := gwremote.PublicMirrorBootstrapURL
	gwremote.PublicMirrorBootstrapURL = server.URL + "/bootstrap"
	defer func() { gwremote.PublicMirrorBootstrapURL = originalURL }()

	require.NoError(t, reportDockerPublicSpectatorReady(cmd))
	assert.Contains(t, buf.String(), "high_water_sequence=42")
}
