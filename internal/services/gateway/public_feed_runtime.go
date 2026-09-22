// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

const (
	defaultPublicSpectatorSourceID = "opendevops-local"
)

// ReadPublicExportConfig loads the durable public-feed export configuration.
func ReadPublicExportConfig(ctx context.Context, fileSvc fs.RuntimeFileService) (models.PublicExportConfig, error) {
	return readPublicExportConfigAt(ctx, fileSvc, constants.PublicFeedExportConfigPath)
}

func readPublicExportConfigAt(ctx context.Context, fileSvc fs.RuntimeFileService, relPath string) (models.PublicExportConfig, error) {
	data, err := fileSvc.ReadFile(ctx, relPath)
	if err != nil {
		return models.PublicExportConfig{}, fmt.Errorf("%w: %v", constants.ErrPublicFeedConfigRequired, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var exportConfig models.PublicExportConfig
	if err := decoder.Decode(&exportConfig); err != nil {
		return models.PublicExportConfig{}, fmt.Errorf("%w: decode: %v", constants.ErrPublicFeedConfigRequired, err)
	}
	if err := rejectTrailingPublicJSON(decoder); err != nil {
		return models.PublicExportConfig{}, err
	}
	if err := ValidatePublicExportConfig(exportConfig); err != nil {
		return models.PublicExportConfig{}, err
	}
	return exportConfig, nil
}

func ValidatePublicFeedSourceID(sourceID string) error {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return constants.ErrPublicFeedSourceIDRequired
	}
	if len(sourceID) > 128 || sourceID == "." || sourceID == ".." {
		return constants.ErrPublicFeedSourceIDInvalid
	}
	for _, char := range sourceID {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.' {
			continue
		}
		return constants.ErrPublicFeedSourceIDInvalid
	}
	return nil
}

// ValidatePublicExportConfig checks required public-feed publisher fields.
func ValidatePublicExportConfig(exportConfig models.PublicExportConfig) error {
	if err := ValidatePublicFeedSourceID(exportConfig.SourceID); err != nil {
		return err
	}
	if strings.TrimSpace(exportConfig.SigningKeyID) == "" {
		return constants.ErrPublicFeedSigningKeyIDRequired
	}
	if exportConfig.BatchMaxRecords <= 0 || exportConfig.BatchMaxRecords > constants.PublicFeedBatchMaxRecords || exportConfig.BatchMaxBytes <= 0 || exportConfig.BatchMaxBytes > constants.PublicFeedBatchMaxBytes || exportConfig.RetryMaxAttempts <= 0 || exportConfig.RetryInitialBackoffSecs < 0 || exportConfig.RetryMaxBackoffSecs < exportConfig.RetryInitialBackoffSecs || exportConfig.AckWindowSecs <= 0 {
		return constants.ErrPublicFeedConfigRequired
	}
	return ValidatePublicMirrorOrigin(exportConfig.MirrorOrigin)
}

// ValidatePublicMirrorOrigin ensures the mirror origin is a bare loopback or
// hosted origin without path or credentials.
func ValidatePublicMirrorOrigin(origin string) error {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
		return constants.ErrPublicFeedMirrorOriginRequired
	}
	return nil
}

// ValidatePublicMirrorListenAddresses enforces loopback-only private/public
// mirror listeners with distinct addresses. When allowContainerBind is true,
// 0.0.0.0 is also accepted so Docker port publishing can reach the listener.
func ValidatePublicMirrorListenAddresses(privateAddress, publicAddress string, allowContainerBind bool) error {
	if privateAddress == publicAddress {
		return fmt.Errorf("%w: duplicate address %q", constants.ErrPublicFeedListenAddress, privateAddress)
	}
	for _, address := range []string{privateAddress, publicAddress} {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("%w: %q: %v", constants.ErrPublicFeedListenAddress, address, err)
		}
		if !isAllowedPublicMirrorListenHost(host, allowContainerBind) {
			return fmt.Errorf("%w: %q", constants.ErrPublicFeedListenAddress, address)
		}
	}
	return nil
}

func isAllowedPublicMirrorListenHost(host string, allowContainerBind bool) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return true
	}
	return allowContainerBind && host == "0.0.0.0"
}

// ValidatePublicExplorerListenAddress enforces loopback-only explorer listeners
// with an address distinct from the private and public mirror listeners.
func ValidatePublicExplorerListenAddress(explorerAddress, privateAddress, publicAddress string, allowContainerBind bool) error {
	explorerAddress = strings.TrimSpace(explorerAddress)
	if explorerAddress == "" {
		return nil
	}
	if sameListenAddress(explorerAddress, privateAddress) || sameListenAddress(explorerAddress, publicAddress) {
		return fmt.Errorf("%w: duplicate address %q", constants.ErrPublicFeedListenAddress, explorerAddress)
	}
	host, _, err := net.SplitHostPort(explorerAddress)
	if err != nil {
		return fmt.Errorf("%w: %q: %v", constants.ErrPublicFeedListenAddress, explorerAddress, err)
	}
	if !isAllowedPublicMirrorListenHost(host, allowContainerBind) {
		return fmt.Errorf("%w: %q", constants.ErrPublicFeedListenAddress, explorerAddress)
	}
	return nil
}

func sameListenAddress(a, b string) bool {
	return normalizeListenAddress(a) == normalizeListenAddress(b)
}

func normalizeListenAddress(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	if host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// EnsureLocalPublicFeed initializes export config and signing material when
// absent so the gateway-owned mirror can accept publisher ingest.
func EnsureLocalPublicFeed(ctx context.Context, fileSvc fs.RuntimeFileService, sourceID, mirrorOrigin string) (models.PublicExportConfig, error) {
	exportConfig, err := ReadPublicExportConfig(ctx, fileSvc)
	if err == nil {
		return exportConfig, nil
	}
	if !errors.Is(err, constants.ErrPublicFeedConfigRequired) {
		return models.PublicExportConfig{}, err
	}
	if strings.TrimSpace(sourceID) == "" {
		sourceID = defaultPublicSpectatorSourceID
	}
	if strings.TrimSpace(mirrorOrigin) == "" {
		mirrorOrigin = fmt.Sprintf("http://127.0.0.1:%d", constants.PublicSpectatorPrivatePort)
	}
	if err := ValidatePublicMirrorOrigin(mirrorOrigin); err != nil {
		return models.PublicExportConfig{}, err
	}
	for _, relPath := range []string{constants.PublicFeedExportConfigPath, constants.PublicFeedSigningKeyPath, constants.PublicFeedIngestTokenPath} {
		exists, existsErr := fileSvc.FileExists(ctx, relPath)
		if existsErr != nil {
			return models.PublicExportConfig{}, fmt.Errorf("public-feed: inspect initialization path: %w", existsErr)
		}
		if exists {
			return models.PublicExportConfig{}, constants.ErrPublicFeedConfigExists
		}
	}
	if err := fileSvc.MkdirAll(ctx, constants.PublicFeedDirname, constants.PermDirPrivate); err != nil {
		return models.PublicExportConfig{}, fmt.Errorf("public-feed: create runtime directory: %w", err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return models.PublicExportConfig{}, fmt.Errorf("%w: %v", constants.ErrPublicFeedKeyGenFailed, err)
	}
	token := make([]byte, constants.PublicFeedIngestTokenBytes)
	if _, err := rand.Read(token); err != nil {
		return models.PublicExportConfig{}, fmt.Errorf("public-feed: generate ingest token: %w", err)
	}
	keyDigest := sha256.Sum256(publicKey)
	exportConfig = models.DefaultPublicExportConfig()
	exportConfig.Enabled = true
	exportConfig.SourceID = sourceID
	exportConfig.MirrorOrigin = mirrorOrigin
	exportConfig.SigningKeyID = hex.EncodeToString(keyDigest[:])
	if err := fileSvc.WriteFile(ctx, constants.PublicFeedSigningKeyPath, []byte(hex.EncodeToString(privateKey)), constants.PermFilePrivate); err != nil {
		return models.PublicExportConfig{}, fmt.Errorf("public-feed: write signing key: %w", err)
	}
	if err := fileSvc.WriteFile(ctx, constants.PublicFeedIngestTokenPath, []byte(hex.EncodeToString(token)), constants.PermFilePrivate); err != nil {
		return models.PublicExportConfig{}, fmt.Errorf("public-feed: write ingest token: %w", err)
	}
	if err := writePublicExportConfig(ctx, fileSvc, exportConfig); err != nil {
		return models.PublicExportConfig{}, err
	}
	return exportConfig, nil
}

type publicFeedArchivePath struct {
	active  string
	archive string
}

type PublicFeedArchivePathSet struct {
	Root          string
	ExportConfig  string
	SigningKey    string
	IngestToken   string
	Outbox        string
	Snapshot      string
	KeyRotation   string
	Proofs        string
	ProofCatalog  string
	ProofManifest string
}

func PublicFeedArchivePathsFor(sourceID string) (PublicFeedArchivePathSet, error) {
	if err := ValidatePublicFeedSourceID(sourceID); err != nil {
		return PublicFeedArchivePathSet{}, err
	}
	root := path.Join(constants.PublicFeedArchiveGenerationsDirname, strings.TrimSpace(sourceID))
	return PublicFeedArchivePathSet{
		Root:          root,
		ExportConfig:  path.Join(root, constants.PublicFeedExportConfigFilename),
		SigningKey:    path.Join(root, constants.PublicFeedSigningKeyFilename),
		IngestToken:   path.Join(root, constants.PublicFeedIngestTokenFilename),
		Outbox:        path.Join(root, constants.PublicFeedOutboxFilename),
		Snapshot:      path.Join(root, constants.PublicFeedSnapshotFilename),
		KeyRotation:   path.Join(root, constants.PublicFeedKeyRotationFilename),
		Proofs:        path.Join(root, constants.PublicProofsDirname),
		ProofCatalog:  path.Join(root, constants.PublicProofCatalogFilename),
		ProofManifest: path.Join(root, constants.PublicProofManifestFilename),
	}, nil
}

func legacyPublicFeedArchivePaths() PublicFeedArchivePathSet {
	return PublicFeedArchivePathSet{
		Root:          constants.PublicFeedArchiveDirname,
		ExportConfig:  constants.PublicFeedLegacyArchiveExportConfigPath,
		SigningKey:    constants.PublicFeedLegacyArchiveSigningKeyPath,
		IngestToken:   constants.PublicFeedLegacyArchiveIngestTokenPath,
		Outbox:        constants.PublicFeedLegacyArchiveOutboxPath,
		Snapshot:      constants.PublicFeedLegacyArchiveSnapshotPath,
		KeyRotation:   constants.PublicFeedLegacyArchiveKeyRotationPath,
		Proofs:        constants.PublicFeedLegacyArchiveProofsPath,
		ProofCatalog:  constants.PublicFeedLegacyArchiveProofCatalogPath,
		ProofManifest: constants.PublicFeedLegacyArchiveProofManifestPath,
	}
}

func archivePathPairs(archive PublicFeedArchivePathSet) []publicFeedArchivePath {
	return []publicFeedArchivePath{
		{active: constants.PublicFeedExportConfigPath, archive: archive.ExportConfig},
		{active: constants.PublicFeedSigningKeyPath, archive: archive.SigningKey},
		{active: constants.PublicFeedIngestTokenPath, archive: archive.IngestToken},
		{active: constants.PublicFeedOutboxPath, archive: archive.Outbox},
		{active: constants.PublicFeedSnapshotPath, archive: archive.Snapshot},
		{active: constants.PublicFeedKeyRotationPath, archive: archive.KeyRotation},
		{active: constants.PublicProofsDirname, archive: archive.Proofs},
		{active: constants.PublicProofCatalogFilename, archive: archive.ProofCatalog},
		{active: constants.PublicProofManifestFilename, archive: archive.ProofManifest},
	}
}

func migrateLegacyPublicFeedArchive(ctx context.Context, fileSvc fs.RuntimeFileService, requestedSourceID string) ([]publicFeedArchivePath, PublicFeedArchivePathSet, error) {
	legacy := legacyPublicFeedArchivePaths()
	configExists, err := fileSvc.FileExists(ctx, legacy.ExportConfig)
	if err != nil {
		return nil, PublicFeedArchivePathSet{}, fmt.Errorf("public-feed: inspect legacy archive: %w", err)
	}
	if !configExists {
		for _, archivePath := range []string{legacy.SigningKey, legacy.IngestToken, legacy.Outbox, legacy.Snapshot, legacy.KeyRotation, legacy.Proofs, legacy.ProofCatalog, legacy.ProofManifest} {
			exists, existsErr := fileSvc.FileExists(ctx, archivePath)
			if existsErr != nil {
				return nil, PublicFeedArchivePathSet{}, fmt.Errorf("public-feed: inspect legacy archive path %s: %w", archivePath, existsErr)
			}
			if exists {
				return nil, PublicFeedArchivePathSet{}, fmt.Errorf("%w: configuration is missing for %s", constants.ErrPublicFeedArchiveCorrupt, archivePath)
			}
		}
		return nil, PublicFeedArchivePathSet{}, nil
	}
	legacyConfig, err := readPublicExportConfigAt(ctx, fileSvc, legacy.ExportConfig)
	if err != nil {
		return nil, PublicFeedArchivePathSet{}, fmt.Errorf("%w: legacy configuration: %w", constants.ErrPublicFeedArchiveCorrupt, err)
	}
	generation, err := PublicFeedArchivePathsFor(legacyConfig.SourceID)
	if err != nil {
		return nil, PublicFeedArchivePathSet{}, fmt.Errorf("%w: legacy source identity: %w", constants.ErrPublicFeedArchiveCorrupt, err)
	}
	if requestedSourceID == legacyConfig.SourceID {
		return nil, PublicFeedArchivePathSet{}, constants.ErrPublicFeedArchiveGenerationExists
	}
	generationExists, err := fileSvc.FileExists(ctx, generation.Root)
	if err != nil {
		return nil, PublicFeedArchivePathSet{}, fmt.Errorf("public-feed: inspect legacy generation: %w", err)
	}
	if generationExists {
		return nil, PublicFeedArchivePathSet{}, constants.ErrPublicFeedArchiveGenerationExists
	}
	if err := fileSvc.MkdirAll(ctx, generation.Root, constants.PermDirPrivate); err != nil {
		return nil, PublicFeedArchivePathSet{}, fmt.Errorf("public-feed: create legacy generation: %w", err)
	}
	moved := make([]publicFeedArchivePath, 0)
	rollback := func(cause error) ([]publicFeedArchivePath, PublicFeedArchivePathSet, error) {
		result := cause
		for index := len(moved) - 1; index >= 0; index-- {
			if renameErr := fileSvc.Rename(ctx, moved[index].archive, moved[index].active); renameErr != nil {
				result = errors.Join(result, fmt.Errorf("public-feed: restore legacy archive path %s: %w", moved[index].active, renameErr))
			}
		}
		if removeErr := fileSvc.RemoveAll(ctx, generation.Root); removeErr != nil {
			result = errors.Join(result, fmt.Errorf("public-feed: remove legacy generation: %w", removeErr))
		}
		return nil, PublicFeedArchivePathSet{}, result
	}
	for _, archivePath := range []string{legacy.ExportConfig, legacy.SigningKey, legacy.IngestToken, legacy.Outbox, legacy.Snapshot, legacy.KeyRotation, legacy.Proofs, legacy.ProofCatalog, legacy.ProofManifest} {
		exists, existsErr := fileSvc.FileExists(ctx, archivePath)
		if existsErr != nil {
			return rollback(fmt.Errorf("public-feed: inspect legacy archive path %s: %w", archivePath, existsErr))
		}
		if !exists {
			continue
		}
		var destination string
		switch archivePath {
		case legacy.ExportConfig:
			destination = generation.ExportConfig
		case legacy.SigningKey:
			destination = generation.SigningKey
		case legacy.IngestToken:
			destination = generation.IngestToken
		case legacy.Outbox:
			destination = generation.Outbox
		case legacy.Snapshot:
			destination = generation.Snapshot
		case legacy.KeyRotation:
			destination = generation.KeyRotation
		case legacy.Proofs:
			destination = generation.Proofs
		case legacy.ProofCatalog:
			destination = generation.ProofCatalog
		case legacy.ProofManifest:
			destination = generation.ProofManifest
		}
		if err := fileSvc.Rename(ctx, archivePath, destination); err != nil {
			return rollback(fmt.Errorf("public-feed: migrate legacy archive path %s: %w", archivePath, err))
		}
		moved = append(moved, publicFeedArchivePath{active: archivePath, archive: destination})
	}
	return moved, generation, nil
}

func TransitionLocalPublicFeed(ctx context.Context, fileSvc fs.RuntimeFileService, sourceID string) (models.PublicExportConfig, error) {
	sourceID = strings.TrimSpace(sourceID)
	if err := ValidatePublicFeedSourceID(sourceID); err != nil {
		return models.PublicExportConfig{}, err
	}
	oldConfig, err := ReadPublicExportConfig(ctx, fileSvc)
	if err != nil {
		return models.PublicExportConfig{}, err
	}
	if sourceID == oldConfig.SourceID {
		return models.PublicExportConfig{}, fmt.Errorf("%w: source is unchanged", constants.ErrValidationFailed)
	}
	requestedGeneration, err := PublicFeedArchivePathsFor(sourceID)
	if err != nil {
		return models.PublicExportConfig{}, err
	}
	requestedGenerationExists, err := fileSvc.FileExists(ctx, requestedGeneration.Root)
	if err != nil {
		return models.PublicExportConfig{}, fmt.Errorf("public-feed: inspect requested archive generation: %w", err)
	}
	if requestedGenerationExists {
		return models.PublicExportConfig{}, constants.ErrPublicFeedArchiveGenerationExists
	}
	legacyMoved, legacyGeneration, err := migrateLegacyPublicFeedArchive(ctx, fileSvc, sourceID)
	if err != nil {
		return models.PublicExportConfig{}, err
	}
	restoreLegacy := func(cause error) error {
		result := cause
		for index := len(legacyMoved) - 1; index >= 0; index-- {
			if renameErr := fileSvc.Rename(ctx, legacyMoved[index].archive, legacyMoved[index].active); renameErr != nil {
				result = errors.Join(result, fmt.Errorf("public-feed: restore migrated archive path %s: %w", legacyMoved[index].active, renameErr))
			}
		}
		if len(legacyMoved) > 0 {
			if removeErr := fileSvc.RemoveAll(ctx, legacyGeneration.Root); removeErr != nil {
				result = errors.Join(result, fmt.Errorf("public-feed: remove migrated generation: %w", removeErr))
			}
		}
		return result
	}
	generation, err := PublicFeedArchivePathsFor(oldConfig.SourceID)
	if err != nil {
		return models.PublicExportConfig{}, restoreLegacy(err)
	}
	generationExists, err := fileSvc.FileExists(ctx, generation.Root)
	if err != nil {
		return models.PublicExportConfig{}, restoreLegacy(fmt.Errorf("public-feed: inspect archive generation: %w", err))
	}
	if generationExists {
		return models.PublicExportConfig{}, restoreLegacy(constants.ErrPublicFeedArchiveGenerationExists)
	}
	if _, err := readPublicSecret(ctx, fileSvc, constants.PublicFeedSigningKeyPath, ed25519.PrivateKeySize, constants.ErrPublicFeedSigningKeyRequired); err != nil {
		return models.PublicExportConfig{}, restoreLegacy(err)
	}
	if _, err := readPublicSecret(ctx, fileSvc, constants.PublicFeedIngestTokenPath, constants.PublicFeedIngestTokenBytes, constants.ErrPublicFeedIngestTokenRequired); err != nil {
		return models.PublicExportConfig{}, restoreLegacy(err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return models.PublicExportConfig{}, restoreLegacy(fmt.Errorf("%w: %v", constants.ErrPublicFeedKeyGenFailed, err))
	}
	token := make([]byte, constants.PublicFeedIngestTokenBytes)
	if _, err := rand.Read(token); err != nil {
		return models.PublicExportConfig{}, restoreLegacy(fmt.Errorf("public-feed: generate ingest token: %w", err))
	}
	keyDigest := sha256.Sum256(publicKey)
	newConfig := oldConfig
	newConfig.SourceID = sourceID
	newConfig.SigningKeyID = hex.EncodeToString(keyDigest[:])

	if err := fileSvc.MkdirAll(ctx, generation.Root, constants.PermDirPrivate); err != nil {
		return models.PublicExportConfig{}, restoreLegacy(fmt.Errorf("public-feed: create archive generation: %w", err))
	}
	paths := archivePathPairs(generation)
	moved := make([]publicFeedArchivePath, 0, len(paths))
	rollback := func(cause error) error {
		result := cause
		for _, relPath := range []string{constants.PublicFeedExportConfigPath, constants.PublicFeedSigningKeyPath, constants.PublicFeedIngestTokenPath} {
			if removeErr := fileSvc.Remove(ctx, relPath); removeErr != nil {
				result = errors.Join(result, fmt.Errorf("public-feed: roll back new source path %s: %w", relPath, removeErr))
			}
		}
		for index := len(moved) - 1; index >= 0; index-- {
			if renameErr := fileSvc.Rename(ctx, moved[index].archive, moved[index].active); renameErr != nil {
				result = errors.Join(result, fmt.Errorf("public-feed: restore archived path %s: %w", moved[index].active, renameErr))
			}
		}
		if removeErr := fileSvc.RemoveAll(ctx, generation.Root); removeErr != nil {
			result = errors.Join(result, fmt.Errorf("public-feed: remove rolled back generation: %w", removeErr))
		}
		return restoreLegacy(result)
	}
	for _, archivePath := range paths {
		exists, existsErr := fileSvc.FileExists(ctx, archivePath.active)
		if existsErr != nil {
			return models.PublicExportConfig{}, rollback(fmt.Errorf("public-feed: inspect source path %s: %w", archivePath.active, existsErr))
		}
		if !exists {
			continue
		}
		if err := fileSvc.Rename(ctx, archivePath.active, archivePath.archive); err != nil {
			return models.PublicExportConfig{}, rollback(fmt.Errorf("public-feed: archive path %s: %w", archivePath.active, err))
		}
		moved = append(moved, archivePath)
	}
	if err := fileSvc.WriteFile(ctx, constants.PublicFeedSigningKeyPath, []byte(hex.EncodeToString(privateKey)), constants.PermFilePrivate); err != nil {
		return models.PublicExportConfig{}, rollback(fmt.Errorf("public-feed: write new signing key: %w", err))
	}
	if err := fileSvc.WriteFile(ctx, constants.PublicFeedIngestTokenPath, []byte(hex.EncodeToString(token)), constants.PermFilePrivate); err != nil {
		return models.PublicExportConfig{}, rollback(fmt.Errorf("public-feed: write new ingest token: %w", err))
	}
	if err := writePublicExportConfig(ctx, fileSvc, newConfig); err != nil {
		return models.PublicExportConfig{}, rollback(err)
	}
	return newConfig, nil
}

func writePublicExportConfig(ctx context.Context, fileSvc fs.RuntimeFileService, exportConfig models.PublicExportConfig) error {
	if err := ValidatePublicExportConfig(exportConfig); err != nil {
		return err
	}
	data, err := json.MarshalIndent(exportConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("public-feed: encode export config: %w", err)
	}
	data = append(data, '\n')
	if err := fileSvc.WriteFile(ctx, constants.PublicFeedExportConfigPath, data, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("public-feed: write export config: %w", err)
	}
	return nil
}

func readPublicSecret(ctx context.Context, fileSvc fs.RuntimeFileService, relPath string, expectedBytes int, missingErr error) ([]byte, error) {
	data, err := fileSvc.ReadFile(ctx, relPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", missingErr, err)
	}
	decoded, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(decoded) != expectedBytes {
		return nil, missingErr
	}
	return decoded, nil
}

func rejectTrailingPublicJSON(decoder *json.Decoder) error {
	if decoder.More() {
		return fmt.Errorf("%w: trailing JSON", constants.ErrPublicFeedConfigRequired)
	}
	return nil
}

func newPublicMirrorHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
