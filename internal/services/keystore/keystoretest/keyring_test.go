// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package keystoretest

import (
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMemoryKeyring(t *testing.T) {
	keyring := NewMemoryKeyring()
	require.NotNil(t, keyring)
	assert.Equal(t, "memory", keyring.Name())
}

func TestMemoryKeyring_RetrieveMasterKey_NotFound(t *testing.T) {
	keyring := NewMemoryKeyring()

	key, err := keyring.RetrieveMasterKey()
	require.Error(t, err)
	assert.Equal(t, constants.ErrKeyStoreKeyNotFound, err)
	assert.Nil(t, key)
}

func TestMemoryKeyring_StoreAndRetrieveMasterKey(t *testing.T) {
	keyring := NewMemoryKeyring()

	testKey := []byte("test-master-key-32-bytes-long-12345")
	err := keyring.StoreMasterKey(testKey)
	require.NoError(t, err)

	retrievedKey, err := keyring.RetrieveMasterKey()
	require.NoError(t, err)
	assert.Equal(t, testKey, retrievedKey)
}

func TestMemoryKeyring_RetrieveMasterKey_ReturnsCopy(t *testing.T) {
	keyring := NewMemoryKeyring()

	testKey := []byte("test-master-key-32-bytes-long-12345")
	err := keyring.StoreMasterKey(testKey)
	require.NoError(t, err)

	retrievedKey1, err := keyring.RetrieveMasterKey()
	require.NoError(t, err)

	retrievedKey1[0] = 'X'

	retrievedKey2, err := keyring.RetrieveMasterKey()
	require.NoError(t, err)
	assert.Equal(t, testKey, retrievedKey2, "modifying retrieved key should not affect stored key")
	assert.NotEqual(t, retrievedKey1, retrievedKey2)
}

func TestMemoryKeyring_DeleteMasterKey(t *testing.T) {
	keyring := NewMemoryKeyring()

	testKey := []byte("test-master-key-32-bytes-long-12345")
	err := keyring.StoreMasterKey(testKey)
	require.NoError(t, err)

	err = keyring.DeleteMasterKey()
	require.NoError(t, err)

	key, err := keyring.RetrieveMasterKey()
	require.Error(t, err)
	assert.Equal(t, constants.ErrKeyStoreKeyNotFound, err)
	assert.Nil(t, key)
}

func TestMemoryKeyring_DeleteMasterKey_NotFound(t *testing.T) {
	keyring := NewMemoryKeyring()

	err := keyring.DeleteMasterKey()
	require.NoError(t, err)
}

func TestMemoryKeyring_OverwriteMasterKey(t *testing.T) {
	keyring := NewMemoryKeyring()

	key1 := []byte("first-master-key-32-bytes-long-123")
	err := keyring.StoreMasterKey(key1)
	require.NoError(t, err)

	key2 := []byte("second-master-key-32-bytes-long-4")
	err = keyring.StoreMasterKey(key2)
	require.NoError(t, err)

	retrievedKey, err := keyring.RetrieveMasterKey()
	require.NoError(t, err)
	assert.Equal(t, key2, retrievedKey, "second key should overwrite first")
}
