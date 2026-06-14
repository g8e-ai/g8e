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

//go:build darwin

package keystore

import (
	"os/exec"
	"testing"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewKeychainBackend(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("security command not available, skipping keychain backend test")
	}

	backend, err := newKeychainBackend()
	require.NoError(t, err)
	assert.Equal(t, "keychain", backend.Name())
}

func TestKeychainBackend_StoreRetrieveDelete(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("security command not available, skipping keychain backend test")
	}

	backend, err := newKeychainBackend()
	require.NoError(t, err)

	testKey := make([]byte, keySize)
	for i := range testKey {
		testKey[i] = byte(i)
	}

	err = backend.StoreMasterKey(testKey)
	require.NoError(t, err)

	retrievedKey, err := backend.RetrieveMasterKey()
	require.NoError(t, err)
	assert.Equal(t, testKey, retrievedKey)

	err = backend.DeleteMasterKey()
	require.NoError(t, err)

	_, err = backend.RetrieveMasterKey()
	require.Error(t, err)
	assert.Equal(t, constants.ErrKeyNotFound, err)
}

func TestKeychainBackend_RetrieveNotFound(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("security command not available, skipping keychain backend test")
	}

	backend, err := newKeychainBackend()
	require.NoError(t, err)

	err = backend.DeleteMasterKey()
	require.NoError(t, err)

	_, err = backend.RetrieveMasterKey()
	require.Error(t, err)
	assert.Equal(t, constants.ErrKeyNotFound, err)
}

func TestKeychainBackend_DeleteIdempotent(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("security command not available, skipping keychain backend test")
	}

	backend, err := newKeychainBackend()
	require.NoError(t, err)

	err = backend.DeleteMasterKey()
	require.NoError(t, err)

	err = backend.DeleteMasterKey()
	require.NoError(t, err)
}

func TestNewDarwin_WithKeychain(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("security command not available, skipping keychain backend test")
	}

	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()

	ks, err := New(secretsDir, logger)
	require.NoError(t, err)
	assert.Equal(t, "keychain", ks.BackendName())
}
