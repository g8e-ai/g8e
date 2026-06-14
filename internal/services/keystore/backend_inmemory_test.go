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
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTestBackend(t *testing.T) {
	backend, err := NewTestBackend()
	require.NoError(t, err)
	require.NotNil(t, backend)
	assert.Equal(t, "test", backend.Name())
}

func TestTestBackend_RetrieveMasterKey_NotFound(t *testing.T) {
	backend, err := NewTestBackend()
	require.NoError(t, err)

	key, err := backend.RetrieveMasterKey()
	require.Error(t, err)
	assert.Equal(t, constants.ErrKeyStoreKeyNotFound, err)
	assert.Nil(t, key)
}

func TestTestBackend_StoreAndRetrieveMasterKey(t *testing.T) {
	backend, err := NewTestBackend()
	require.NoError(t, err)

	testKey := []byte("test-master-key-32-bytes-long-12345")
	err = backend.StoreMasterKey(testKey)
	require.NoError(t, err)

	retrievedKey, err := backend.RetrieveMasterKey()
	require.NoError(t, err)
	assert.Equal(t, testKey, retrievedKey)
}

func TestTestBackend_RetrieveMasterKey_ReturnsCopy(t *testing.T) {
	backend, err := NewTestBackend()
	require.NoError(t, err)

	testKey := []byte("test-master-key-32-bytes-long-12345")
	err = backend.StoreMasterKey(testKey)
	require.NoError(t, err)

	retrievedKey1, err := backend.RetrieveMasterKey()
	require.NoError(t, err)

	retrievedKey1[0] = 'X'

	retrievedKey2, err := backend.RetrieveMasterKey()
	require.NoError(t, err)
	assert.Equal(t, testKey, retrievedKey2, "modifying retrieved key should not affect stored key")
	assert.NotEqual(t, retrievedKey1, retrievedKey2)
}

func TestTestBackend_DeleteMasterKey(t *testing.T) {
	backend, err := NewTestBackend()
	require.NoError(t, err)

	testKey := []byte("test-master-key-32-bytes-long-12345")
	err = backend.StoreMasterKey(testKey)
	require.NoError(t, err)

	err = backend.DeleteMasterKey()
	require.NoError(t, err)

	key, err := backend.RetrieveMasterKey()
	require.Error(t, err)
	assert.Equal(t, constants.ErrKeyStoreKeyNotFound, err)
	assert.Nil(t, key)
}

func TestTestBackend_DeleteMasterKey_NotFound(t *testing.T) {
	backend, err := NewTestBackend()
	require.NoError(t, err)

	err = backend.DeleteMasterKey()
	require.NoError(t, err)
}

func TestTestBackend_OverwriteMasterKey(t *testing.T) {
	backend, err := NewTestBackend()
	require.NoError(t, err)

	key1 := []byte("first-master-key-32-bytes-long-123")
	err = backend.StoreMasterKey(key1)
	require.NoError(t, err)

	key2 := []byte("second-master-key-32-bytes-long-4")
	err = backend.StoreMasterKey(key2)
	require.NoError(t, err)

	retrievedKey, err := backend.RetrieveMasterKey()
	require.NoError(t, err)
	assert.Equal(t, key2, retrievedKey, "second key should overwrite first")
}
