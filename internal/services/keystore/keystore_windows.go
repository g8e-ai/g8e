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

package keystore

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/g8e-ai/g8e/internal/constants"
)

// New creates a new Keystore instance with file-based storage on Windows.
// Windows Credential Manager integration could be added in the future.
// Production callers should pass paths.Infra.SecretsDir for secretsDir.
func New(secretsDir string, logger *slog.Logger) (*Keystore, error) {
	if err := os.MkdirAll(secretsDir, constants.PermDirPrivate); err != nil {
		return nil, fmt.Errorf("keystore: create secrets directory: %w", err)
	}

	keyring, err := newFileKeyring(secretsDir)
	if err != nil {
		return nil, fmt.Errorf("keystore: initialize file keyring: %w", err)
	}

	logger.Info("[Keystore] Using file-based storage (Windows)", "keyring", keyring.Name())

	return &Keystore{
		logger:     logger,
		secretsDir: secretsDir,
		keyring:    keyring,
	}, nil
}
