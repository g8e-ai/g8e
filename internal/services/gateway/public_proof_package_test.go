// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// newProofBuilderTestEnv builds a PublicPublisherService for proof package
// builder tests.
func newProofBuilderTestEnv(t *testing.T) *PublicPublisherService {
	t.Helper()
	logger := testutil.NewTestLogger()
	cfg := sqliteutil.DefaultDBConfig(":memory:")
	db, err := sqliteutil.OpenDB(cfg, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(gatewaySchema)
	require.NoError(t, err)

	docStore := NewDocumentStoreService(db, logger)
	fileSvc := newProducerFileSvc(t)

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	exportCfg := models.DefaultPublicExportConfig()
	exportCfg.Enabled = true
	exportCfg.SourceID = "test-source-1"
	exportCfg.SigningKeyID = "test-key-1"

	return NewPublicPublisherService(docStore, fileSvc, logger, exportCfg, priv, "test-key-1")
}

// TestBuildProofPackage_CreatesCompletePackage verifies that BuildProofPackage
// creates a complete proof package directory with a root manifest, catalog
// entries, and all artifacts from a passing verification report.
func TestBuildProofPackage_CreatesCompletePackage(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	artifacts := []ProofArtifactInput{
		{
			Filename:   "model-campaign.json",
			MediaType:  "application/json",
			Content:    []byte(`{"campaign_id":"c1","verification_ok":true}`),
			CampaignID: "c1",
		},
		{
			Filename:    "campaign-projections.jsonl",
			MediaType:   "application/json",
			Content:     []byte(`{"campaign_id":"c1","variant_id":"v1"}\n`),
			CampaignID:  "c1",
			SourceRunID: "run-1",
		},
	}

	manifest, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "abc123hash", true, artifacts)
	require.NoError(t, err)
	require.NotNil(t, manifest)

	assert.Equal(t, constants.PublicProofManifestSchemaVersion, manifest.SchemaVersion)
	assert.Equal(t, "c1", manifest.CampaignID)
	assert.Equal(t, "rev-1", manifest.CampaignRevision)
	assert.Equal(t, "abc123hash", manifest.VerifiedIndexGenerationHash)
	assert.True(t, manifest.VerificationOK)
	assert.Equal(t, 2, manifest.ArtifactCount)
	assert.Len(t, manifest.Artifacts, 2)
	assert.NotEmpty(t, manifest.ProofRootSHA256)
	assert.NotEmpty(t, manifest.Signature)
	assert.NotEmpty(t, manifest.SigningKeyID)

	// Each artifact has a content-addressed ID and immutable URL.
	for _, entry := range manifest.Artifacts {
		assert.NotEmpty(t, entry.ArtifactID)
		assert.NotEmpty(t, entry.ImmutableURL)
		assert.Equal(t, models.PublicFeedProofClassificationPublicSafe, entry.Classification)
		assert.NotEmpty(t, entry.SHA256)
		assert.Greater(t, entry.ByteSize, int64(0))
		assert.NotEmpty(t, entry.VerificationCommand)
	}
}

// TestBuildProofPackage_RejectsFailingVerification verifies that
// BuildProofPackage rejects a failing verification report.
func TestBuildProofPackage_RejectsFailingVerification(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	artifacts := []ProofArtifactInput{
		{
			Filename:   "model-campaign.json",
			MediaType:  "application/json",
			Content:    []byte(`{}`),
			CampaignID: "c1",
		},
	}

	_, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "hash", false, artifacts)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedProofNotVerified)
}

// TestBuildProofPackage_RejectsEmptyArtifacts verifies that an empty artifact
// list is rejected.
func TestBuildProofPackage_RejectsEmptyArtifacts(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	_, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "hash", true, nil)
	require.Error(t, err)
}

// TestBuildProofPackage_RejectsOversizedArtifact verifies that an artifact
// exceeding the max bytes limit is rejected.
func TestBuildProofPackage_RejectsOversizedArtifact(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	// Create an artifact that exceeds the max bytes.
	hugeContent := make([]byte, constants.PublicFeedMaxArtifactBytes+1)
	artifacts := []ProofArtifactInput{
		{
			Filename:   "huge.json",
			MediaType:  "application/json",
			Content:    hugeContent,
			CampaignID: "c1",
		},
	}

	_, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "hash", true, artifacts)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedProofOversized)
}

func TestBuildProofPackage_RejectsUnsafeArtifactMetadataAndContent(t *testing.T) {
	tests := []struct {
		name     string
		artifact ProofArtifactInput
		err      error
	}{
		{
			name: "path traversal filename",
			artifact: ProofArtifactInput{
				Filename:   "../private.json",
				MediaType:  "application/json",
				Content:    []byte(`{"campaign_id":"c1"}`),
				CampaignID: "c1",
			},
			err: constants.ErrPublicFeedProofPathTraversal,
		},
		{
			name: "nested restricted JSON field",
			artifact: ProofArtifactInput{
				Filename:   "public.json",
				MediaType:  "application/json",
				Content:    []byte(`{"campaign":{"credentials":{"token":"restricted"}}}`),
				CampaignID: "c1",
			},
			err: constants.ErrPublicFeedProofRestricted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publisher := newProofBuilderTestEnv(t)
			_, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "hash", true, []ProofArtifactInput{tt.artifact})
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.err)
		})
	}
}

// TestBuildProofPackage_ArtifactsContentAddressed verifies that artifact IDs
// are derived from the SHA-256 of the content.
func TestBuildProofPackage_ArtifactsContentAddressed(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	content := []byte(`{"campaign_id":"c1"}`)
	artifacts := []ProofArtifactInput{
		{
			Filename:   "test.json",
			MediaType:  "application/json",
			Content:    content,
			CampaignID: "c1",
		},
	}

	manifest, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "hash", true, artifacts)
	require.NoError(t, err)

	expectedHash := sha256.Sum256(content)
	expectedID := hex.EncodeToString(expectedHash[:])
	assert.Equal(t, expectedID, manifest.Artifacts[0].ArtifactID)
	assert.Equal(t, expectedID, manifest.Artifacts[0].SHA256)
}

// TestBuildProofPackage_ProofRootHashVerifiable verifies that the proof root
// SHA-256 can be recomputed from the artifact hashes and manifest metadata.
func TestBuildProofPackage_ProofRootHashVerifiable(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	artifacts := []ProofArtifactInput{
		{
			Filename:   "a.json",
			MediaType:  "application/json",
			Content:    []byte(`{"a":1}`),
			CampaignID: "c1",
		},
		{
			Filename:   "b.json",
			MediaType:  "application/json",
			Content:    []byte(`{"b":2}`),
			CampaignID: "c1",
		},
	}

	manifest, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "hash", true, artifacts)
	require.NoError(t, err)

	// Recompute the proof root hash.
	recomputed, err := publisher.ComputeProofRootHash(manifest)
	require.NoError(t, err)
	assert.Equal(t, manifest.ProofRootSHA256, recomputed)
}

// TestBuildProofPackage_SignatureValid verifies that the proof manifest
// signature is a valid Ed25519 signature over the proof root hash.
func TestBuildProofPackage_SignatureValid(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	artifacts := []ProofArtifactInput{
		{
			Filename:   "test.json",
			MediaType:  "application/json",
			Content:    []byte(`{"test":true}`),
			CampaignID: "c1",
		},
	}

	manifest, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "hash", true, artifacts)
	require.NoError(t, err)

	// Verify the signature.
	rootHashBytes, err := hex.DecodeString(manifest.ProofRootSHA256)
	require.NoError(t, err)
	sigBytes, err := hex.DecodeString(manifest.Signature)
	require.NoError(t, err)
	assert.True(t, ed25519.Verify(publisher.signingPubKey, rootHashBytes, sigBytes), "proof manifest signature must be valid")
}

// TestBuildProofPackage_WritesArtifactsToDisk verifies that the proof
// artifacts are written to the runtime tree under the public-proofs
// directory.
func TestBuildProofPackage_WritesArtifactsToDisk(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	content := []byte(`{"campaign_id":"c1"}`)
	artifacts := []ProofArtifactInput{
		{
			Filename:   "test.json",
			MediaType:  "application/json",
			Content:    content,
			CampaignID: "c1",
		},
	}

	manifest, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "hash", true, artifacts)
	require.NoError(t, err)

	// The artifact file exists on disk.
	relPath := filepath.Join(constants.PublicProofsDirname, manifest.Artifacts[0].ArtifactID)
	absPath := publisher.fileSvc.Resolve(relPath)
	diskContent, err := os.ReadFile(absPath)
	require.NoError(t, err)
	assert.Equal(t, content, diskContent)
}

// TestBuildProofPackage_RejectsSymlinkArtifact verifies that a symlink in
// the proof directory is rejected.
func TestBuildProofPackage_RejectsSymlinkArtifact(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	// Create a symlink in the public-proofs directory before building.
	proofsDir := publisher.fileSvc.Resolve(constants.PublicProofsDirname)
	require.NoError(t, os.MkdirAll(proofsDir, constants.PermDirPrivate))
	target := filepath.Join(proofsDir, "target.json")
	require.NoError(t, os.WriteFile(target, []byte("target"), constants.PermFilePrivate))
	linkPath := filepath.Join(proofsDir, "link.json")
	require.NoError(t, os.Symlink(target, linkPath))

	artifacts := []ProofArtifactInput{
		{
			Filename:   "test.json",
			MediaType:  "application/json",
			Content:    []byte(`{"test":true}`),
			CampaignID: "c1",
		},
	}

	_, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "hash", true, artifacts)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedProofSymlinkRejected)
}

// TestGetProofCatalog_ReturnsAllArtifacts verifies that GetProofCatalog
// returns all proof artifacts in the catalog.
func TestGetProofCatalog_ReturnsAllArtifacts(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	artifacts1 := []ProofArtifactInput{
		{
			Filename:   "a.json",
			MediaType:  "application/json",
			Content:    []byte(`{"a":1}`),
			CampaignID: "c1",
		},
	}
	_, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "hash1", true, artifacts1)
	require.NoError(t, err)

	artifacts2 := []ProofArtifactInput{
		{
			Filename:   "b.json",
			MediaType:  "application/json",
			Content:    []byte(`{"b":2}`),
			CampaignID: "c2",
		},
	}
	_, err = publisher.BuildProofPackage(context.Background(), "c2", "rev-2", "hash2", true, artifacts2)
	require.NoError(t, err)

	catalog, err := publisher.GetProofCatalog(context.Background())
	require.NoError(t, err)
	assert.Len(t, catalog.Entries, 2)
	assert.Equal(t, constants.PublicProofCatalogSchemaVersion, catalog.SchemaVersion)
}

// TestGetProofCatalog_EmptyWhenNoProofs verifies that GetProofCatalog returns
// an empty catalog when no proofs have been built.
func TestGetProofCatalog_EmptyWhenNoProofs(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	catalog, err := publisher.GetProofCatalog(context.Background())
	require.NoError(t, err)
	assert.Empty(t, catalog.Entries)
}

// TestStreamProof_ServesContentWithSafeHeaders verifies that StreamProof
// serves the artifact bytes with fixed safe headers (Cache-Control: immutable,
// correct Content-Type).
func TestStreamProof_ServesContentWithSafeHeaders(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	content := []byte(`{"campaign_id":"c1","result":"ok"}`)
	artifacts := []ProofArtifactInput{
		{
			Filename:   "result.json",
			MediaType:  "application/json",
			Content:    content,
			CampaignID: "c1",
		},
	}

	manifest, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "hash", true, artifacts)
	require.NoError(t, err)

	artifactID := manifest.Artifacts[0].ArtifactID
	w := httptestResponseWriter()
	err = publisher.StreamProof(context.Background(), artifactID, w)
	require.NoError(t, err)

	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Equal(t, "public, max-age=31536000, immutable", w.Header().Get("Cache-Control"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "result.json")
	assert.Equal(t, string(content), w.Body.String())
}

// TestStreamProof_RejectsNotFound verifies that StreamProof returns
// ErrPublicFeedProofNotFound for an unknown artifact ID.
func TestStreamProof_RejectsNotFound(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	w := httptestResponseWriter()
	err := publisher.StreamProof(context.Background(), "nonexistent-id", w)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedProofNotFound)
}

// TestStreamProof_VerifiesBytesBeforeServing verifies that StreamProof
// checks the on-disk file hash against the catalog hash and rejects
// tampered files.
func TestStreamProof_VerifiesBytesBeforeServing(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	content := []byte(`{"campaign_id":"c1"}`)
	artifacts := []ProofArtifactInput{
		{
			Filename:   "test.json",
			MediaType:  "application/json",
			Content:    content,
			CampaignID: "c1",
		},
	}

	manifest, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "hash", true, artifacts)
	require.NoError(t, err)

	artifactID := manifest.Artifacts[0].ArtifactID

	// Tamper with the file on disk.
	relPath := filepath.Join(constants.PublicProofsDirname, artifactID)
	absPath := publisher.fileSvc.Resolve(relPath)
	require.NoError(t, os.WriteFile(absPath, []byte("tampered"), constants.PermFilePrivate))

	w := httptestResponseWriter()
	err = publisher.StreamProof(context.Background(), artifactID, w)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedProofHashMismatch)
}

// TestStreamProof_RejectsSymlink verifies that StreamProof rejects a symlinked
// artifact file.
func TestStreamProof_RejectsSymlink(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	content := []byte(`{"campaign_id":"c1"}`)
	artifacts := []ProofArtifactInput{
		{
			Filename:   "test.json",
			MediaType:  "application/json",
			Content:    content,
			CampaignID: "c1",
		},
	}

	manifest, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "hash", true, artifacts)
	require.NoError(t, err)

	artifactID := manifest.Artifacts[0].ArtifactID

	// Replace the file with a symlink.
	relPath := filepath.Join(constants.PublicProofsDirname, artifactID)
	absPath := publisher.fileSvc.Resolve(relPath)
	require.NoError(t, os.Remove(absPath))
	target := absPath + ".target"
	require.NoError(t, os.WriteFile(target, []byte("target"), constants.PermFilePrivate))
	require.NoError(t, os.Symlink(target, absPath))

	w := httptestResponseWriter()
	err = publisher.StreamProof(context.Background(), artifactID, w)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPublicFeedProofSymlinkRejected)
}

// TestBuildProofPackage_ExcludesRestrictedEvidence verifies that the proof
// package builder only accepts public_safe artifacts.
func TestBuildProofPackage_ExcludesRestrictedEvidence(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	// The ProofArtifactInput does not carry a privacy classification — all
	// artifacts in the proof package are public_safe by construction. The
	// builder does not accept restricted artifacts.
	artifacts := []ProofArtifactInput{
		{
			Filename:   "public.json",
			MediaType:  "application/json",
			Content:    []byte(`{"public":true}`),
			CampaignID: "c1",
		},
	}

	manifest, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "hash", true, artifacts)
	require.NoError(t, err)

	for _, entry := range manifest.Artifacts {
		assert.Equal(t, models.PublicFeedProofClassificationPublicSafe, entry.Classification,
			"all proof artifacts must be public_safe")
	}
}

// TestBuildProofPackage_ManifestRoundTrip verifies that the proof manifest
// can be serialized and deserialized without loss.
func TestBuildProofPackage_ManifestRoundTrip(t *testing.T) {
	publisher := newProofBuilderTestEnv(t)

	artifacts := []ProofArtifactInput{
		{
			Filename:   "test.json",
			MediaType:  "application/json",
			Content:    []byte(`{"test":true}`),
			CampaignID: "c1",
		},
	}

	manifest, err := publisher.BuildProofPackage(context.Background(), "c1", "rev-1", "hash", true, artifacts)
	require.NoError(t, err)

	manifestBytes, err := json.Marshal(manifest)
	require.NoError(t, err)

	var roundTripped models.PublicProofManifest
	err = json.Unmarshal(manifestBytes, &roundTripped)
	require.NoError(t, err)
	assert.Equal(t, manifest.ProofRootSHA256, roundTripped.ProofRootSHA256)
	assert.Equal(t, manifest.CampaignID, roundTripped.CampaignID)
	assert.Equal(t, manifest.ArtifactCount, roundTripped.ArtifactCount)
	assert.Len(t, roundTripped.Artifacts, len(manifest.Artifacts))
}
