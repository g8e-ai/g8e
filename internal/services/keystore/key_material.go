// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package keystore

import (
	"encoding/hex"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// StoreKeyMaterial encrypts raw key bytes under the master key and writes them
// to the secrets directory as name.
func (k *Keystore) StoreKeyMaterial(name string, key []byte) error {
	if len(key) == 0 {
		return fmt.Errorf("keystore: store key material %s: %w", name, constants.ErrMissingRequiredField)
	}
	return k.EncryptSecret(name, hex.EncodeToString(key))
}

// LoadKeyMaterial decrypts key bytes written by StoreKeyMaterial and checks
// they are exactly size bytes long.
func (k *Keystore) LoadKeyMaterial(name string, size int) ([]byte, error) {
	plaintext, err := k.DecryptSecret(name)
	if err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(plaintext)
	if err != nil {
		return nil, fmt.Errorf("keystore: load key material %s: %w: %w", name, constants.ErrKeyStoreDecodeFailed, err)
	}
	if len(key) != size {
		return nil, fmt.Errorf("keystore: load key material %s: %w: got %d, expected %d", name, constants.ErrKeyStoreInvalidKeyLength, len(key), size)
	}
	return key, nil
}
