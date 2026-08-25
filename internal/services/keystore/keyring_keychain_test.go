// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build darwin

package keystore

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestNewKeychainKeyring(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("security command not available, skipping keychain keyring test")
	}

	keyring, err := newKeychainKeyring()
	require.NoError(t, err)
	assert.Equal(t, "keychain", keyring.Name())
}

func TestKeychainKeyring_StoreRetrieveDelete(t *testing.T) {
	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("security command not available, skipping keychain keyring test")
	}

	keyring, err := newKeychainKeyring()
	require.NoError(t, err)

	testKey := make([]byte, vault.KeySize)
	for i := range testKey {
		testKey[i] = byte(i)
	}

	err = keyring.StoreMasterKey(testKey)
	require.NoError(t, err)

	retrievedKey, err := keyring.RetrieveMasterKey()
	require.NoError(t, err)
	assert.Equal(t, testKey, retrievedKey)

	err = keyring.DeleteMasterKey()
	require.NoError(t, err)

	_, err = keyring.RetrieveMasterKey()
	require.Error(t, err)
	assert.Equal(t, constants.ErrKeyStoreKeyNotFound, err)
}

func TestKeychainKeyring_RetrieveNotFound(t *testing.T) {
	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("security command not available, skipping keychain keyring test")
	}

	keyring, err := newKeychainKeyring()
	require.NoError(t, err)

	err = keyring.DeleteMasterKey()
	require.NoError(t, err)

	_, err = keyring.RetrieveMasterKey()
	require.Error(t, err)
	assert.Equal(t, constants.ErrKeyStoreKeyNotFound, err)
}

func TestKeychainKeyring_DeleteIdempotent(t *testing.T) {
	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("security command not available, skipping keychain keyring test")
	}

	keyring, err := newKeychainKeyring()
	require.NoError(t, err)

	err = keyring.DeleteMasterKey()
	require.NoError(t, err)

	err = keyring.DeleteMasterKey()
	require.NoError(t, err)
}

func TestNewDarwin_WithKeychain(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("security command not available, skipping keychain keyring test")
	}

	secretsDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()

	ks, err := New(secretsDir, logger)
	require.NoError(t, err)
	assert.Equal(t, "keychain", ks.KeyringName())
}
