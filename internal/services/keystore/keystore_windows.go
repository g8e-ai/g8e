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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

// NewWithFS creates a new Keystore instance with file-based storage on Windows
// and the provided RuntimeFileService for file I/O.
// Windows Credential Manager integration could be added in the future.
func NewWithFS(fileSvc fs.RuntimeFileService, logger *slog.Logger) (*Keystore, error) {
	secretsDir := fileSvc.Resolve(constants.SecretsDirname)

	keyring, err := newFileKeyring(secretsDir)
	if err != nil {
		return nil, fmt.Errorf("keystore: initialize file keyring: %w", err)
	}

	logger.Info("[Keystore] Using file-based storage (Windows)", "keyring", keyring.Name())

	return &Keystore{
		logger:  logger,
		keyring: keyring,
		fileSvc: fileSvc,
	}, nil
}
