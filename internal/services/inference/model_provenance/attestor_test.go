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

func TestOllamaStorageAttestor_AttestNamespacedModelTag(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modelTag := "Impulse2000/smollm3:3b-q4_k_m"
	layerBytes := []byte("namespaced gguf payload")
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
	manifestPath := filepath.Join(root, "manifests", "registry.ollama.ai", "Impulse2000", "smollm3", "3b-q4_k_m")
	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.WriteFile(manifestPath, manifestBytes, 0o600))

	expectedDigest := models.SHA256Hex(manifestBytes)
	attestor, err := NewOllamaStorageAttestor(root, "test-provenance-operator")
	require.NoError(t, err)

	window, err := attestor.Attest(context.Background(), modelTag, expectedDigest, time.UnixMilli(1_700_000_000_000))
	require.NoError(t, err)
	assert.True(t, window.GetDigestMatch())
	assert.Equal(t, modelTag, window.GetServedModelTag())
}

func TestResolveOllamaManifestPath_NamespacedVsLibrary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	libraryPath := filepath.Join(root, "manifests", "registry.ollama.ai", "library", "probe-model", "7b")
	require.NoError(t, os.MkdirAll(filepath.Dir(libraryPath), 0o755))
	require.NoError(t, os.WriteFile(libraryPath, []byte("{}"), 0o600))

	libraryResolved, err := resolveOllamaManifestPath(root, "probe-model:7b")
	require.NoError(t, err)
	assert.Equal(t, libraryPath, libraryResolved)

	namespacedPath := filepath.Join(root, "manifests", "registry.ollama.ai", "Impulse2000", "smollm3", "3b-q4_k_m")
	require.NoError(t, os.MkdirAll(filepath.Dir(namespacedPath), 0o755))
	require.NoError(t, os.WriteFile(namespacedPath, []byte("{}"), 0o600))

	namespacedResolved, err := resolveOllamaManifestPath(root, "Impulse2000/smollm3:3b-q4_k_m")
	require.NoError(t, err)
	assert.Equal(t, namespacedPath, namespacedResolved)
}

func TestResolveOllamaManifestPath_HuggingFaceHostPull(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifests", "huggingface.co", "unsloth", "Qwen3.8-27B-GGUF", "UD-Q4_K_M")
	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.WriteFile(manifestPath, []byte("{}"), 0o600))

	resolved, err := resolveOllamaManifestPath(root, "huggingface.co/unsloth/Qwen3.8-27B-GGUF:UD-Q4_K_M")
	require.NoError(t, err)
	assert.Equal(t, manifestPath, resolved)
}

func TestResolveOllamaManifestPath_UntaggedModelDefaultsToLatest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifests", "registry.ollama.ai", "library", "glm-5.3-flash", "latest")
	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.WriteFile(manifestPath, []byte("{}"), 0o600))

	resolved, err := resolveOllamaManifestPath(root, "glm-5.3-flash")
	require.NoError(t, err)
	assert.Equal(t, manifestPath, resolved)
}

func TestOllamaStorageAttestor_AttestUntaggedModelTag(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	modelTag := "glm-5.3-flash"
	layerBytes := []byte("glm flash payload")
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
	manifestPath := filepath.Join(root, "manifests", "registry.ollama.ai", "library", "glm-5.3-flash", "latest")
	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.WriteFile(manifestPath, manifestBytes, 0o600))

	expectedDigest := models.SHA256Hex(manifestBytes)
	attestor, err := NewOllamaStorageAttestor(root, "test-provenance-operator")
	require.NoError(t, err)

	window, err := attestor.Attest(context.Background(), modelTag, expectedDigest, time.UnixMilli(1_700_000_000_000))
	require.NoError(t, err)
	assert.True(t, window.GetDigestMatch())
	assert.Equal(t, modelTag, window.GetServedModelTag())
}
