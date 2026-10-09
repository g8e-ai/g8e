// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows

package keystore

import (
	"bytes"
	"context"
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestDPAPIKeyring_New_RequiresFileService(t *testing.T) {
	t.Parallel()
	_, err := newDPAPIKeyring(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestDPAPIKeyring_StoreRetrieveDelete_RoundTrip(t *testing.T) {
	t.Parallel()
	fileSvc, secretsDir := setupTestFileService(t)

	kr, err := newDPAPIKeyring(fileSvc)
	require.NoError(t, err)
	assert.Equal(t, "dpapi", kr.Name())

	// Retrieve before store returns ErrKeyStoreKeyNotFound
	_, err = kr.RetrieveMasterKey()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreKeyNotFound)

	// Invalid key lengths rejected
	err = kr.StoreMasterKey([]byte("too-short"))
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreInvalidKeyLength)

	tooLongKey := make([]byte, 64)
	err = kr.StoreMasterKey(tooLongKey)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreInvalidKeyLength)

	// Valid key store
	testKey := make([]byte, vault.KeySize)
	_, err = rand.Read(testKey)
	require.NoError(t, err)

	err = kr.StoreMasterKey(testKey)
	require.NoError(t, err)

	// Verify file exists on disk and is DPAPI protected (not raw bytes)
	dpapiFileRel := filepath.Join(constants.SecretsDirname, constants.MasterKeyDPAPIFilename)
	blob, err := fileSvc.ReadFile(context.Background(), dpapiFileRel)
	require.NoError(t, err)
	assert.NotEmpty(t, blob)
	assert.False(t, bytes.Equal(blob, testKey), "DPAPI file should contain ciphertext, not plaintext key")

	// Retrieve master key
	retrieved, err := kr.RetrieveMasterKey()
	require.NoError(t, err)
	assert.Equal(t, testKey, retrieved)

	// Delete
	err = kr.DeleteMasterKey()
	require.NoError(t, err)

	// Retrieve after delete returns ErrKeyStoreKeyNotFound
	_, err = kr.RetrieveMasterKey()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreKeyNotFound)

	// Delete again is idempotent
	assert.NoError(t, kr.DeleteMasterKey())
	_ = secretsDir
}

func TestDPAPIKeyring_KeystoreIntegration(t *testing.T) {
	t.Parallel()
	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()

	// Open uses platformKeyring (which on Windows is DPAPI)
	ks, err := Open(fileSvc, logger, Options{})
	require.NoError(t, err)
	assert.Equal(t, "dpapi", ks.KeyringName())

	// Verify encrypt and decrypt work
	secretName := "dpapi-test-secret"
	secretVal := "super-secure-windows-data"
	require.NoError(t, ks.EncryptSecret(secretName, secretVal))

	decrypted, err := ks.DecryptSecret(secretName)
	require.NoError(t, err)
	assert.Equal(t, secretVal, decrypted)

	// Reopen keystore to ensure retrieved key matches
	ks2, err := NewWithFS(fileSvc, logger, Options{})
	require.NoError(t, err)
	require.NoError(t, ks2.Initialize())

	decrypted2, err := ks2.DecryptSecret(secretName)
	require.NoError(t, err)
	assert.Equal(t, secretVal, decrypted2)
}
