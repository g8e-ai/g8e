// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
