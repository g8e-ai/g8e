// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package model_provenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	defaultOllamaManifestHost = "registry.ollama.ai"
	defaultOllamaManifestNS   = "library"
)

// Attestor independently attests model weight blobs at the storage site.
type Attestor interface {
	Attest(ctx context.Context, servedModelTag, expectedModelDigest string, attestedAt time.Time) (*evalv1.ModelProvenanceAttestationWindow, error)
}

// OllamaStorageAttestor hashes Ollama manifest and content-addressed blobs
// beneath modelStorageRoot and compares the manifest digest to the expected
// campaign model digest.
type OllamaStorageAttestor struct {
	modelStorageRoot string
	operatorID       string
}

// NewOllamaStorageAttestor constructs a storage-side attestor for one model
// root directory (for example ~/.ollama/models).
func NewOllamaStorageAttestor(modelStorageRoot, operatorID string) (*OllamaStorageAttestor, error) {
	root := strings.TrimSpace(modelStorageRoot)
	if root == "" {
		return nil, fmt.Errorf("model provenance attestor: %w", constants.ErrMissingRequiredField)
	}
	if operatorID == "" {
		operatorID = "g8e-model-provenance-operator"
	}
	return &OllamaStorageAttestor{modelStorageRoot: root, operatorID: operatorID}, nil
}

type ollamaManifest struct {
	Config struct {
		Digest    string `json:"digest"`
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
	} `json:"config"`
	Layers []struct {
		Digest    string `json:"digest"`
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
	} `json:"layers"`
}

// Attest reads the Ollama manifest for servedModelTag, verifies referenced
// blobs, and returns a provenance window with digest_match set.
func (a *OllamaStorageAttestor) Attest(ctx context.Context, servedModelTag, expectedModelDigest string, attestedAt time.Time) (*evalv1.ModelProvenanceAttestationWindow, error) {
	if servedModelTag == "" || expectedModelDigest == "" {
		return nil, fmt.Errorf("model provenance attestor: %w", constants.ErrMissingRequiredField)
	}
	if !models.IsSHA256Hex(expectedModelDigest) {
		return nil, fmt.Errorf("model provenance attestor: invalid expected digest")
	}

	manifestPath, err := resolveOllamaManifestPath(a.modelStorageRoot, servedModelTag)
	if err != nil {
		return nil, err
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("model provenance attestor: read manifest: %w", err)
	}

	manifestDigest := models.SHA256Hex(manifestBytes)
	observedDigest := normalizeOllamaDigest(manifestDigest)

	var manifest ollamaManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("model provenance attestor: parse manifest: %w", err)
	}

	weightAttestations := make([]*evalv1.ModelWeightAttestation, 0, len(manifest.Layers)+1)
	if manifest.Config.Digest != "" {
		attestation, err := a.attestBlob(ctx, manifest.Config.Digest, manifest.Config.MediaType, manifest.Config.Size)
		if err != nil {
			return nil, err
		}
		weightAttestations = append(weightAttestations, attestation)
	}
	for _, layer := range manifest.Layers {
		attestation, err := a.attestBlob(ctx, layer.Digest, layer.MediaType, layer.Size)
		if err != nil {
			return nil, err
		}
		weightAttestations = append(weightAttestations, attestation)
	}

	digestMatch := observedDigest == normalizeOllamaDigest(expectedModelDigest)
	return &evalv1.ModelProvenanceAttestationWindow{
		SchemaVersion:              SchemaVersion,
		ProvenanceOperatorId:       a.operatorID,
		ServedModelTag:             servedModelTag,
		ExpectedModelDigest:        expectedModelDigest,
		ObservedModelDigest:        observedDigest,
		ManifestDigest:             manifestDigest,
		ManifestVerificationStatus: evalv1.ModelManifestVerificationStatus_MODEL_MANIFEST_VERIFICATION_STATUS_UNSIGNED,
		WeightAttestations:         weightAttestations,
		AttestedAtUnixMs:           attestedAt.UTC().UnixMilli(),
		DigestMatch:                digestMatch,
	}, nil
}

func (a *OllamaStorageAttestor) attestBlob(ctx context.Context, digestRef, mediaType string, declaredSize int64) (*evalv1.ModelWeightAttestation, error) {
	digestHex, err := digestRefToHex(digestRef)
	if err != nil {
		return nil, err
	}
	blobPath := filepath.Join(a.modelStorageRoot, "blobs", "sha256-"+digestHex)
	file, err := os.Open(blobPath)
	if err != nil {
		return nil, fmt.Errorf("model provenance attestor: open blob %s: %w", digestHex, err)
	}
	defer file.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return nil, fmt.Errorf("model provenance attestor: hash blob %s: %w", digestHex, err)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	observed := hex.EncodeToString(hasher.Sum(nil))
	if observed != digestHex {
		return nil, fmt.Errorf("model provenance attestor: blob digest mismatch for %s", digestHex)
	}
	if declaredSize > 0 && size != declaredSize {
		return nil, fmt.Errorf("model provenance attestor: blob size mismatch for %s", digestHex)
	}
	return &evalv1.ModelWeightAttestation{
		BlobDigest: digestHex,
		SizeBytes:  uint64(size),
		MediaType:  mediaType,
	}, nil
}

func resolveOllamaManifestPath(storageRoot, servedModelTag string) (string, error) {
	modelRef, tag, ok := strings.Cut(servedModelTag, ":")
	if !ok || modelRef == "" || tag == "" {
		return "", fmt.Errorf("model provenance attestor: invalid served model tag %q", servedModelTag)
	}
	namespace := defaultOllamaManifestNS
	modelName := modelRef
	if ns, name, ok := strings.Cut(modelRef, "/"); ok && ns != "" && name != "" {
		namespace = ns
		modelName = name
	}
	manifestPath := filepath.Join(storageRoot, "manifests", defaultOllamaManifestHost, namespace, modelName, tag)
	if _, err := os.Stat(manifestPath); err != nil {
		return "", fmt.Errorf("model provenance attestor: manifest not found for %q: %w", servedModelTag, err)
	}
	return manifestPath, nil
}

func digestRefToHex(digestRef string) (string, error) {
	digestRef = strings.TrimSpace(digestRef)
	digestRef = strings.TrimPrefix(digestRef, "sha256:")
	if !models.IsSHA256Hex(digestRef) {
		return "", fmt.Errorf("model provenance attestor: invalid digest ref %q", digestRef)
	}
	return digestRef, nil
}

func normalizeOllamaDigest(digest string) string {
	return strings.TrimPrefix(strings.TrimSpace(digest), "sha256:")
}
