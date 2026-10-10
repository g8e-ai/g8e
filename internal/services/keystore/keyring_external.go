// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package keystore

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	runtimefs "github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
)

// externalFileKeyring reads an operator-provisioned master key, such as a
// Docker or Kubernetes secret mount. The file is external input, not runtime
// state: it must sit outside the runtime directory to separate key material
// from runtime backups, and g8e never writes or deletes it.
type externalFileKeyring struct {
	path string
	root string
}

func newExternalFileKeyring(fileSvc runtimefs.RuntimeFileService, path string) (Keyring, error) {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("%w: %q is not absolute", constants.ErrKeyStoreExternalKeyPath, path)
	}
	path = filepath.Clean(path)
	if pathWithin(fileSvc.Resolve(""), path) {
		return nil, fmt.Errorf("%w: %s is inside the runtime directory", constants.ErrKeyStoreExternalKeyPath, path)
	}
	return &externalFileKeyring{path: path, root: fileSvc.Resolve("")}, nil
}

func (e *externalFileKeyring) Name() string {
	return "external-file"
}

// pathWithin compares path components, including children whose names start with "..".
func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (e *externalFileKeyring) RetrieveMasterKey() ([]byte, error) {
	// Recheck every read: secret mounts may rotate symlinks or permissions.
	root, err := filepath.EvalSymlinks(e.root)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve runtime root: %w", constants.ErrKeyStoreExternalKeyPath, err)
	}
	path, err := filepath.EvalSymlinks(e.path)
	if err != nil {
		return nil, fmt.Errorf("external master key: resolve provisioned file: %w (%s)", err, masterKeyProvisioning)
	}
	if pathWithin(root, path) {
		return nil, fmt.Errorf("%w: resolved file is inside runtime directory", constants.ErrKeyStoreExternalKeyPath)
	}
	f, err := openExternalKey(path)
	if err != nil {
		return nil, fmt.Errorf("external master key: open: %w (%s)", err, masterKeyProvisioning)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("external master key: stat: %w", err)
	}
	const maxKeyFileSize = 4096
	if !info.Mode().IsRegular() || info.Size() > maxKeyFileSize {
		return nil, constants.ErrKeyStoreExternalFileInvalid
	}
	if err := validateExternalKeyMetadata(info); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxKeyFileSize+1))
	defer vault.SecureZero(data)
	if err != nil {
		return nil, fmt.Errorf("external master key: read: %w", err)
	}
	if len(data) > maxKeyFileSize {
		return nil, constants.ErrKeyStoreExternalFileInvalid
	}
	encoded := bytes.TrimSpace(data)
	key := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
	n, err := base64.StdEncoding.Decode(key, encoded)
	if err != nil {
		vault.SecureZero(key)
		return nil, fmt.Errorf("%w: external master key: %w", constants.ErrKeyStoreDecodeFailed, err)
	}
	if n != vault.KeySize {
		vault.SecureZero(key)
		return nil, fmt.Errorf("%w: got %d, expected %d", constants.ErrKeyStoreInvalidKeyLength, n, vault.KeySize)
	}
	return key[:n], nil
}

func (e *externalFileKeyring) StoreMasterKey([]byte) error {
	return fmt.Errorf("%w: provision %s with 32 random bytes encoded as base64 (for example: openssl rand -base64 32)", constants.ErrKeyStoreExternalReadOnly, e.path)
}

// DeleteMasterKey leaves the operator-provisioned file in place; its lifecycle
// belongs to whoever provisioned it.
func (e *externalFileKeyring) DeleteMasterKey() error {
	return nil
}
