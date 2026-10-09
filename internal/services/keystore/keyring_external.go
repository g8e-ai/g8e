// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package keystore

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	runtimefs "github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// externalFileKeyring reads an operator-provisioned master key, such as a
// Docker or Kubernetes secret mount. The file is external input, not runtime
// state: it must sit outside the runtime directory so a copy of the data volume
// never carries the key that decrypts it, and g8e never writes or deletes it.
type externalFileKeyring struct {
	path string
}

func newExternalFileKeyring(fileSvc runtimefs.RuntimeFileService, path string) (Keyring, error) {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("%w: %q is not absolute", constants.ErrKeyStoreExternalKeyPath, path)
	}
	path = filepath.Clean(path)
	if _, err := fileSvc.Rel(path); err == nil {
		return nil, fmt.Errorf("%w: %s is inside the runtime directory", constants.ErrKeyStoreExternalKeyPath, path)
	}
	return &externalFileKeyring{path: path}, nil
}

func (e *externalFileKeyring) Name() string {
	return "external-file"
}

func (e *externalFileKeyring) RetrieveMasterKey() ([]byte, error) {
	data, err := os.ReadFile(e.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, constants.ErrKeyStoreKeyNotFound
		}
		return nil, fmt.Errorf("external master key: read %s: %w", e.path, err)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: external master key %s: %w", constants.ErrKeyStoreDecodeFailed, e.path, err)
	}
	if len(key) == 0 {
		return nil, constants.ErrKeyStoreKeyNotFound
	}
	return key, nil
}

func (e *externalFileKeyring) StoreMasterKey([]byte) error {
	return fmt.Errorf("%w: provision %s with 32 random bytes encoded as base64 (for example: openssl rand -base64 32)", constants.ErrKeyStoreExternalReadOnly, e.path)
}

// DeleteMasterKey leaves the operator-provisioned file in place; its lifecycle
// belongs to whoever provisioned it.
func (e *externalFileKeyring) DeleteMasterKey() error {
	return nil
}
