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

//go:build linux || windows

package keystore

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
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
	if len(key) != keySize {
		return fmt.Errorf("invalid master key length %d, expected %d", len(key), keySize)
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
