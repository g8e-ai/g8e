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

//go:build linux

package keystore

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// fileBackend stores the master key in a file within the secrets directory.
// This is a fallback for Linux systems without libsecret installed.
type fileBackend struct {
	secretsDir string
}

func newFileBackend(secretsDir string) (Backend, error) {
	return &fileBackend{secretsDir: secretsDir}, nil
}

func (b *fileBackend) Name() string {
	return "file"
}

func (b *fileBackend) RetrieveMasterKey() ([]byte, error) {
	path := filepath.Join(b.secretsDir, ".master_key")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrKeyNotFound
		}
		return nil, fmt.Errorf("read master key file: %w", err)
	}

	// Decode base64
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, fmt.Errorf("decode base64 master key: %w", err)
	}

	if len(key) == 0 {
		return nil, ErrKeyNotFound
	}

	return key, nil
}

func (b *fileBackend) StoreMasterKey(key []byte) error {
	// Encode as base64 for safe storage
	encoded := base64.StdEncoding.EncodeToString(key)
	path := filepath.Join(b.secretsDir, ".master_key")
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, []byte(encoded), 0600); err != nil {
		return fmt.Errorf("write master key file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic rename master key file: %w", err)
	}

	return nil
}

func (b *fileBackend) DeleteMasterKey() error {
	path := filepath.Join(b.secretsDir, ".master_key")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete master key file: %w", err)
	}
	return nil
}
