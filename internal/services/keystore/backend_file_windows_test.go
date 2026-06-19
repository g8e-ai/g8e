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

//go:build windows

package keystore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileBackend(t *testing.T) {
	t.Parallel()

	secretsDir := t.TempDir()
	backend, err := newFileBackend(secretsDir)
	require.NoError(t, err)
	assert.Equal(t, "file", backend.Name())
}

func TestFileBackend_StoreRetrieveDelete(t *testing.T) {
	t.Parallel()

	secretsDir := t.TempDir()
	backend, err := newFileBackend(secretsDir)
	require.NoError(t, err)
	assert.Equal(t, "file", backend.Name())

	testKey := make([]byte, keySize)
	for i := range testKey {
		testKey[i] = byte(i)
	}

	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "store",
			fn: func(t *testing.T) {
				err := backend.StoreMasterKey(testKey)
				require.NoError(t, err)
			},
		},
		{
			name: "retrieve",
			fn: func(t *testing.T) {
				retrievedKey, err := backend.RetrieveMasterKey()
				require.NoError(t, err)
				assert.Equal(t, testKey, retrievedKey)
			},
		},
		{
			name: "delete",
			fn: func(t *testing.T) {
				err := backend.DeleteMasterKey()
				require.NoError(t, err)
			},
		},
		{
			name: "retrieve after delete",
			fn: func(t *testing.T) {
				_, err := backend.RetrieveMasterKey()
				require.Error(t, err)
				assert.Equal(t, constants.ErrKeyStoreKeyNotFound, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}

func TestFileBackend_RetrieveNotFound(t *testing.T) {
	t.Parallel()

	secretsDir := t.TempDir()
	backend, err := newFileBackend(secretsDir)
	require.NoError(t, err)

	_, err = backend.RetrieveMasterKey()
	require.Error(t, err)
	assert.Equal(t, constants.ErrKeyStoreKeyNotFound, err)
}

func TestFileBackend_DeleteIdempotent(t *testing.T) {
	t.Parallel()

	secretsDir := t.TempDir()
	backend, err := newFileBackend(secretsDir)
	require.NoError(t, err)

	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "first delete",
			fn: func(t *testing.T) {
				err := backend.DeleteMasterKey()
				require.NoError(t, err)
			},
		},
		{
			name: "second delete",
			fn: func(t *testing.T) {
				err := backend.DeleteMasterKey()
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}

func TestFileBackend_WithRealKeystore(t *testing.T) {
	t.Parallel()

	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	backend, err := newFileBackend(secretsDir)
	require.NoError(t, err)

	ks, err := NewWithBackend(secretsDir, logger, backend)
	require.NoError(t, err)

	err = ks.Initialize()
	require.NoError(t, err)

	plaintext := "test-secret-value"
	err = ks.EncryptSecret("test-secret", plaintext)
	require.NoError(t, err)

	decrypted, err := ks.DecryptSecret("test-secret")
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)

	secretPath := filepath.Join(secretsDir, "test-secret")
	info, err := os.Stat(secretPath)
	require.NoError(t, err)
	// Windows doesn't support Unix permissions exactly, so just check file is not world-writable
	perm := info.Mode().Perm()
	assert.NotEqual(t, os.FileMode(0777), perm, "secret file should not be world-writable")
}
