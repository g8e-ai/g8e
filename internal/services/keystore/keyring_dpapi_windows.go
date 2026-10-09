// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows

package keystore

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
)

// dpapiEntropy binds the sealed blob to this use; DPAPI blobs sealed by other
// software for the same user cannot be substituted for the master key.
var dpapiEntropy = []byte(keyStoreName + "/" + masterKeyName)

// dpapiKeyring seals the master key with DPAPI under the current Windows user.
// The sealed blob lives in the secrets directory, but only that user on that
// machine can unseal it: a copied volume, backup, or snapshot is useless
// without the user's DPAPI master key.
type dpapiKeyring struct {
	fileSvc fs.RuntimeFileService
	path    string
}

func newDPAPIKeyring(fileSvc fs.RuntimeFileService) (Keyring, error) {
	if fileSvc == nil {
		return nil, fmt.Errorf("dpapi keyring: %w", constants.ErrMissingRequiredField)
	}
	return &dpapiKeyring{
		fileSvc: fileSvc,
		path:    filepath.Join(constants.SecretsDirname, constants.MasterKeyDPAPIFilename),
	}, nil
}

func (d *dpapiKeyring) Name() string {
	return "dpapi"
}

func (d *dpapiKeyring) RetrieveMasterKey() ([]byte, error) {
	sealed, err := d.fileSvc.ReadFile(context.Background(), d.path)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) {
			return nil, constants.ErrKeyStoreKeyNotFound
		}
		return nil, fmt.Errorf("dpapi: read sealed master key: %w", err)
	}
	if len(sealed) == 0 {
		return nil, constants.ErrKeyStoreKeyNotFound
	}
	key, err := dpapiUnprotect(sealed)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (d *dpapiKeyring) StoreMasterKey(key []byte) error {
	if len(key) != vault.KeySize {
		return fmt.Errorf("%w: got %d, expected %d", constants.ErrKeyStoreInvalidKeyLength, len(key), vault.KeySize)
	}
	sealed, err := dpapiProtect(key)
	if err != nil {
		return err
	}
	if err := d.fileSvc.WriteFile(context.Background(), d.path, sealed, constants.PermFilePrivate); err != nil {
		return fmt.Errorf("dpapi: write sealed master key: %w", err)
	}
	return nil
}

func (d *dpapiKeyring) DeleteMasterKey() error {
	if err := d.fileSvc.Remove(context.Background(), d.path); err != nil && !errors.Is(err, constants.ErrNotFound) {
		return fmt.Errorf("dpapi: delete sealed master key: %w", err)
	}
	return nil
}

func dpapiBlob(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

// takeDPAPIOutput copies a DPAPI-allocated output blob into Go memory, then
// zeroes and frees the original.
func takeDPAPIOutput(out *windows.DataBlob) []byte {
	if out.Data == nil {
		return nil
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) //nolint:errcheck // LocalFree failure leaks only memory
	src := unsafe.Slice(out.Data, out.Size)
	dst := make([]byte, len(src))
	copy(dst, src)
	vault.SecureZero(src)
	return dst
}

func dpapiProtect(plaintext []byte) ([]byte, error) {
	var out windows.DataBlob
	err := windows.CryptProtectData(dpapiBlob(plaintext), nil, dpapiBlob(dpapiEntropy), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, fmt.Errorf("%w: protect: %w", constants.ErrKeyStoreDPAPIFailed, err)
	}
	return takeDPAPIOutput(&out), nil
}

func dpapiUnprotect(sealed []byte) ([]byte, error) {
	var out windows.DataBlob
	err := windows.CryptUnprotectData(dpapiBlob(sealed), nil, dpapiBlob(dpapiEntropy), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	if err != nil {
		return nil, fmt.Errorf("%w: unprotect: %w", constants.ErrKeyStoreDPAPIFailed, err)
	}
	return takeDPAPIOutput(&out), nil
}
