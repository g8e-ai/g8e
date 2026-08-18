// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
// .pub files in a directory. Each file's basename is the keyID, and the file
// content is a hex-encoded ED25519 public key.
type FilesystemSignerStore struct {
	signers map[string]ed25519.PublicKey
}

// NewFilesystemSignerStore loads all .pub files from the specified directory.
// The directory must exist and contain .pub files with hex-encoded public keys.
// Returns an error if the directory does not exist or if any file is malformed.
func NewFilesystemSignerStore(dir string, logger *slog.Logger) (*FilesystemSignerStore, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read trusted signers directory %s: %w", dir, err)
	}

	signers := make(map[string]ed25519.PublicKey)
	loadedCount := 0
	failureCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".pub") {
			continue
		}

		keyID := strings.TrimSuffix(entry.Name(), ".pub")
		filePath := filepath.Join(dir, entry.Name())

		data, err := os.ReadFile(filePath)
		if err != nil {
			if logger != nil {
				logger.Warn("Failed to read trusted signer file", "path", filePath, "error", err)
			}
			failureCount++
			continue
		}

		hexKey := strings.TrimSpace(string(data))
		pubKeyBytes, err := hex.DecodeString(hexKey)
		if err != nil {
			if logger != nil {
				logger.Warn("Failed to decode hex public key", "path", filePath, "error", err)
			}
			failureCount++
			continue
		}

		if len(pubKeyBytes) != ed25519.PublicKeySize {
			if logger != nil {
				logger.Warn("Invalid public key size", "path", filePath, "size", len(pubKeyBytes), "expected", ed25519.PublicKeySize)
			}
			failureCount++
			continue
		}

		signers[keyID] = ed25519.PublicKey(pubKeyBytes)
		loadedCount++
	}

	if logger != nil {
		logger.Info("Loaded trusted signers from filesystem",
			"directory", dir,
			"loaded", loadedCount,
			"failed", failureCount)
	}

	return &FilesystemSignerStore{
		signers: signers,
	}, nil
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
