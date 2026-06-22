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

//go:build linux

package keystore

import (
	"os"
	"os/exec"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
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
	assert.Equal(t, constants.ErrKeyNotFound, err)
}

func TestNewLinux_FallbackToFileKeyring(t *testing.T) {
	t.Parallel()

	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()

	backupPath := os.Getenv("PATH")
	defer func() { os.Setenv("PATH", backupPath) }()

	os.Setenv("PATH", "")

	ks, err := New(secretsDir, logger)
	require.NoError(t, err)
	assert.Equal(t, "file", ks.KeyringName())
}

func TestNewLinux_WithLibsecret(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("secret-tool"); err != nil {
		t.Skip("secret-tool not available, skipping libsecret keyring test")
	}

	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()

	ks, err := New(secretsDir, logger)
	require.NoError(t, err)
	assert.Equal(t, "libsecret", ks.KeyringName())
}
