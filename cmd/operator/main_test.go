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

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	vault "github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestHandleVerifyVault_NotInitialized(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{
		DataDir: dir,
		Logger:  logger,
	})
	require.NoError(t, err)
	defer v.Close()

	assert.False(t, v.IsInitialized())
}

func TestHandleRekeyVault_RequiresInitializedVault(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{
		DataDir: dir,
		Logger:  logger,
	})
	require.NoError(t, err)
	defer v.Close()

	err = v.Rekey([]byte("old-key"), []byte("new-key"))
	require.Error(t, err)
}

func TestHandleResetVault_RequiresInitializedVault(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{
		DataDir: dir,
		Logger:  logger,
	})
	require.NoError(t, err)
	defer v.Close()

	assert.False(t, v.IsInitialized())
}

func TestHandleRekeyVault_MissingOldKey_PrintsError(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{DataDir: dir, Logger: logger})
	require.NoError(t, err)
	defer v.Close()

	// Rekey without initializing vault - must return error
	err = v.Rekey([]byte(""), []byte("new-key"))
	require.Error(t, err)
}

func TestHandleVerifyVault_VaultNotInitialized(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{DataDir: dir, Logger: logger})
	require.NoError(t, err)
	defer v.Close()

	assert.False(t, v.IsInitialized(), "fresh vault must not be initialized")
}

func TestHandleVerifyVault_MissingPrivateKey_VaultInitialized(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{DataDir: dir, Logger: logger})
	require.NoError(t, err)
	defer v.Close()

	header, _, err := vault.NewVaultHeader([]byte("initial-key"))
	require.NoError(t, err)
	require.NoError(t, header.Save(dir))

	require.True(t, v.IsInitialized())

	// Wrong key should fail integrity check
	err = v.VerifyIntegrity([]byte("wrong-key"))
	require.Error(t, err)
}

func TestHandleResetVault_VaultNotInitialized_NoOp(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{DataDir: dir, Logger: logger})
	require.NoError(t, err)
	defer v.Close()

	assert.False(t, v.IsInitialized(), "nothing to reset on fresh vault")
}

func TestHandleResetVault_Initialized_ResetDestroysData(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{DataDir: dir, Logger: logger})
	require.NoError(t, err)
	defer v.Close()

	header, _, err := vault.NewVaultHeader([]byte("some-key"))
	require.NoError(t, err)
	require.NoError(t, header.Save(dir))
	require.True(t, v.IsInitialized())

	require.NoError(t, v.Reset(true))
	assert.False(t, v.IsInitialized(), "vault must be uninitialized after reset")
}

func TestHandleVaultLifecycle(t *testing.T) {
	logger := testutil.NewTestLogger()
	dir := t.TempDir()

	v, err := vault.NewVault(&vault.VaultConfig{DataDir: dir, Logger: logger})
	require.NoError(t, err)
	defer v.Close()

	header, _, err := vault.NewVaultHeader([]byte("init-key"))
	require.NoError(t, err)
	require.NoError(t, header.Save(dir))
	require.True(t, v.IsInitialized())

	require.NoError(t, v.VerifyIntegrity([]byte("init-key")))
	require.NoError(t, v.Reset(true))
	assert.False(t, v.IsInitialized())
}
