// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package model_provenance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaStorageAttestor_AttestMatchesManifestDigest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modelTag := "probe-model:7b"
	layerBytes := []byte("synthetic gguf payload")
	layerDigest := models.SHA256Hex(layerBytes)

	require.NoError(t, os.MkdirAll(filepath.Join(root, "blobs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "blobs", "sha256-"+layerDigest), layerBytes, 0o600))

	manifest := map[string]interface{}{
		"schemaVersion": 2,
		"layers": []map[string]interface{}{
			{
				"digest":    "sha256:" + layerDigest,
				"mediaType": "application/vnd.ollama.image.model",
				"size":      len(layerBytes),
			},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestPath := filepath.Join(root, "manifests", "registry.ollama.ai", "library", "probe-model", "7b")
	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.WriteFile(manifestPath, manifestBytes, 0o600))

	expectedDigest := models.SHA256Hex(manifestBytes)
	attestor, err := NewOllamaStorageAttestor(root, "test-provenance-operator")
	require.NoError(t, err)

	window, err := attestor.Attest(context.Background(), modelTag, expectedDigest, time.UnixMilli(1_700_000_000_000))
	require.NoError(t, err)
	assert.True(t, window.GetDigestMatch())
	assert.Equal(t, expectedDigest, window.GetObservedModelDigest())
	assert.Len(t, window.GetWeightAttestations(), 1)
	assert.Equal(t, layerDigest, window.GetWeightAttestations()[0].GetBlobDigest())
}
