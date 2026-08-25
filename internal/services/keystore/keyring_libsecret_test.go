// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build linux

package keystore

import (
	"os"
	"os/exec"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLibsecretKeyring(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("secret-tool"); err != nil {
		t.Skip("secret-tool not available, skipping libsecret keyring test")
	}

	keyring, err := newLibsecretKeyring()
	require.NoError(t, err)
	assert.Equal(t, "libsecret", keyring.Name())
}

func TestLibsecretKeyring_StoreRetrieveDelete(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("secret-tool"); err != nil {
		t.Skip("secret-tool not available, skipping libsecret keyring test")
	}

	keyring, err := newLibsecretKeyring()
	require.NoError(t, err)

	testKey := make([]byte, vault.KeySize)
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
				err := keyring.StoreMasterKey(testKey)
				require.NoError(t, err)
			},
		},
		{
			name: "retrieve",
			fn: func(t *testing.T) {
				retrievedKey, err := keyring.RetrieveMasterKey()
				require.NoError(t, err)
				assert.Equal(t, testKey, retrievedKey)
			},
		},
		{
			name: "delete",
			fn: func(t *testing.T) {
				err := keyring.DeleteMasterKey()
				require.NoError(t, err)
			},
		},
		{
			name: "retrieve after delete",
			fn: func(t *testing.T) {
				_, err := keyring.RetrieveMasterKey()
				require.Error(t, err)
				assert.Equal(t, constants.ErrKeyStoreKeyNotFound, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}

func TestLibsecretKeyring_RetrieveNotFound(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("secret-tool"); err != nil {
		t.Skip("secret-tool not available, skipping libsecret keyring test")
	}

	keyring, err := newLibsecretKeyring()
	require.NoError(t, err)

	// Ensure key doesn't exist
	err = keyring.DeleteMasterKey()
	require.NoError(t, err)

	_, err = keyring.RetrieveMasterKey()
	require.Error(t, err)
	assert.Equal(t, constants.ErrKeyStoreKeyNotFound, err)
}

func TestNewLinux_FallbackToFileKeyring(t *testing.T) {
	t.Parallel()

	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()

	backupPath := os.Getenv("PATH")
	defer func() { os.Setenv("PATH", backupPath) }()

	os.Setenv("PATH", "")

	ks, err := NewWithFS(fileSvc, logger)
	require.NoError(t, err)
	assert.Equal(t, "file", ks.KeyringName())
}

func TestNewLinux_WithLibsecret(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("secret-tool"); err != nil {
		t.Skip("secret-tool not available, skipping libsecret keyring test")
	}

	fileSvc, _ := setupTestFileService(t)
	logger := testutil.NewTestLogger()

	ks, err := NewWithFS(fileSvc, logger)
	require.NoError(t, err)
	assert.Equal(t, "libsecret", ks.KeyringName())
}
