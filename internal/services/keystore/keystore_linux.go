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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// NewWithFS creates a new Keystore instance with the libsecret keyring
// and the provided RuntimeFileService for file I/O.
// Falls back to file-based storage if libsecret is not available.
func NewWithFS(fileSvc fs.RuntimeFileService, logger *slog.Logger) (*Keystore, error) {
	secretsDir := fileSvc.Resolve(constants.SecretsDirname)

	keyring, err := newLibsecretKeyring()
	if err != nil {
		keyring, err = newFileKeyring(secretsDir)
		if err != nil {
			return nil, fmt.Errorf("keystore: initialize file keyring: %w", err)
		}
		logger.Info("[Keystore] Using file-based storage (libsecret unavailable)", "keyring", keyring.Name())
	}

	return &Keystore{
		logger:  logger,
		keyring: keyring,
		fileSvc: fileSvc,
	}, nil
}
