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

package keystoretest

import (
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
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
