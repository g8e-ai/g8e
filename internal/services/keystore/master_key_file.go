// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package keystore

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// ProvisionMasterKeyFile writes a fresh base64-encoded 32-byte master key to a
// private file in dir and returns its path, in the format the external-file
// keyring reads. The file is owned by the caller, who removes dir when done.
func ProvisionMasterKeyFile(dir string) (string, error) {
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", fmt.Errorf("generate master key: %w", err)
	}
	path := filepath.Join(dir, "master.key")
	encoded := base64.StdEncoding.EncodeToString(keyBytes) + "\n"
	if err := os.WriteFile(path, []byte(encoded), constants.PermFilePrivate); err != nil {
		return "", fmt.Errorf("write master key: %w", err)
	}
	return path, nil
}
