// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build linux || windows

package keystore

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
)

// fileKeyring stores the master key in a file within the secrets directory.
// It is the fallback when no OS-native keyring is available: the libsecret
// fallback on Linux and the default keyring on Windows.
type fileKeyring struct {
	masterKeyPath string
}

func newFileKeyring(secretsDir string) (Keyring, error) {
	return &fileKeyring{masterKeyPath: filepath.Join(secretsDir, constants.MasterKeyFilename)}, nil
}

func (f *fileKeyring) Name() string {
	return "file"
}

func (f *fileKeyring) RetrieveMasterKey() ([]byte, error) {
	data, err := os.ReadFile(f.masterKeyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, constants.ErrKeyStoreKeyNotFound
		}
		return nil, fmt.Errorf("read master key file: %w", err)
	}

	// Decode base64
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, fmt.Errorf("decode base64 master key: %w", err)
	}

	if len(key) == 0 {
		return nil, constants.ErrKeyStoreKeyNotFound
	}

	return key, nil
}

func (f *fileKeyring) StoreMasterKey(key []byte) error {
	// Validate key length (AES-256 requires 32 bytes)
	if len(key) != vault.KeySize {
		return fmt.Errorf("invalid master key length %d, expected %d", len(key), vault.KeySize)
	}

	// Encode as base64 for safe storage
	encoded := base64.StdEncoding.EncodeToString(key)
	tmpPath := f.masterKeyPath + constants.TmpFileSuffix

	if err := os.WriteFile(tmpPath, []byte(encoded), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("write master key file: %w", err)
	}

	if err := os.Rename(tmpPath, f.masterKeyPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic rename master key file: %w", err)
	}

	return nil
}

func (f *fileKeyring) DeleteMasterKey() error {
	if err := os.Remove(f.masterKeyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete master key file: %w", err)
	}
	return nil
}
