// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// SignerStore defines the interface for loading trusted L2 signers.
type SignerStore interface {
	GetTrustedSigner(keyID string) (ed25519.PublicKey, error)
}

// FailClosedSignerStore implements SignerStore using a static map.
// Used as a production fallback (empty map = fail-closed) and in tests.
type FailClosedSignerStore struct {
	Signers map[string]ed25519.PublicKey
}

func (s *FailClosedSignerStore) GetTrustedSigner(keyID string) (ed25519.PublicKey, error) {
	if s.Signers == nil {
		return nil, nil
	}
	pubKey, ok := s.Signers[keyID]
	if !ok {
		return nil, nil
	}
	return pubKey, nil
}

// FilesystemSignerStore implements SignerStore by loading public keys from
// runtime .pub files. Each file's basename is the keyID, and the file content
// is a hex-encoded ED25519 public key.
type FilesystemSignerStore struct {
	signers map[string]ed25519.PublicKey
}

// NewFilesystemSignerStore loads all .pub files from a runtime-relative
// directory through RuntimeFileService. The directory must exist.
func NewFilesystemSignerStore(fileSvc fs.RuntimeFileService, relDir string, logger *slog.Logger) (*FilesystemSignerStore, error) {
	if fileSvc == nil || relDir == "" {
		return nil, fmt.Errorf("trusted signers: %w", constants.ErrMissingRequiredField)
	}
	entries, err := fileSvc.ReadDir(context.Background(), relDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read trusted signers directory %s: %w", relDir, err)
	}

	signers := make(map[string]ed25519.PublicKey)
	loadedCount := 0
	failureCount := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pub") {
			continue
		}
		keyID := strings.TrimSuffix(entry.Name(), ".pub")
		relPath := filepath.Join(relDir, entry.Name())
		data, err := fileSvc.ReadFile(context.Background(), relPath)
		if err != nil {
			if logger != nil {
				logger.Warn("Failed to read trusted signer file", "path", relPath, "error", err)
			}
			failureCount++
			continue
		}
		pubKeyBytes, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err != nil {
			if logger != nil {
				logger.Warn("Failed to decode hex public key", "path", relPath, "error", err)
			}
			failureCount++
			continue
		}
		if len(pubKeyBytes) != ed25519.PublicKeySize {
			if logger != nil {
				logger.Warn("Invalid public key size", "path", relPath, "size", len(pubKeyBytes), "expected", ed25519.PublicKeySize)
			}
			failureCount++
			continue
		}
		signers[keyID] = ed25519.PublicKey(pubKeyBytes)
		loadedCount++
	}
	if logger != nil {
		logger.Info("Loaded trusted signers from filesystem", "directory", relDir, "loaded", loadedCount, "failed", failureCount)
	}
	return &FilesystemSignerStore{signers: signers}, nil
}

func (s *FilesystemSignerStore) GetTrustedSigner(keyID string) (ed25519.PublicKey, error) {
	if s.signers == nil {
		return nil, nil
	}
	pubKey, ok := s.signers[keyID]
	if !ok {
		return nil, nil
	}
	return pubKey, nil
}
