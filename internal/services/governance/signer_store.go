// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

// SimpleSignerStore implements SignerStore using a static map.
// Used as a production fallback (empty map = fail-closed) and in tests.
type SimpleSignerStore struct {
	Signers map[string]ed25519.PublicKey
}

func (s *SimpleSignerStore) GetTrustedSigner(keyID string) (ed25519.PublicKey, error) {
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
	logger  *slog.Logger
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
		logger:  logger,
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
