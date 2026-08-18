// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package consensus

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestFileKeyProvider_GetMemberKey_Success(t *testing.T) {
	t.Parallel()

	secretsDir := testutil.TempDir(t)
	consensusID := "test-consensus"
	memberAppID := "member-1"

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	err = SaveMemberKey(secretsDir, consensusID, memberAppID, priv)
	require.NoError(t, err)

	provider := NewFileKeyProvider(secretsDir, consensusID)
	loadedKey, err := provider.GetMemberKey(memberAppID)
	require.NoError(t, err)

	assert.Equal(t, priv, loadedKey, "loaded key should match saved key")
	assert.Equal(t, pub, loadedKey.Public().(ed25519.PublicKey), "public key should match")
}

func TestFileKeyProvider_GetMemberKey_NotFound(t *testing.T) {
	t.Parallel()

	secretsDir := testutil.TempDir(t)
	consensusID := "test-consensus"

	provider := NewFileKeyProvider(secretsDir, consensusID)
	_, err := provider.GetMemberKey("nonexistent-member")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key file not found")
}

func TestFileKeyProvider_GetMemberKey_InvalidSeedLength(t *testing.T) {
	t.Parallel()

	secretsDir := testutil.TempDir(t)
	consensusID := "test-consensus"
	memberAppID := "member-bad"

	filename := constants.SecretsFileConsensusMemberKeyPrefix + consensusID + "_" + memberAppID + ".key"
	keyPath := filepath.Join(secretsDir, filename)
	err := os.WriteFile(keyPath, []byte(hex.EncodeToString([]byte("too-short"))), constants.PermFilePrivate)
	require.NoError(t, err)

	provider := NewFileKeyProvider(secretsDir, consensusID)
	_, err = provider.GetMemberKey(memberAppID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid seed length")
}

func TestFileKeyProvider_GetMemberKey_InvalidHex(t *testing.T) {
	t.Parallel()

	secretsDir := testutil.TempDir(t)
	consensusID := "test-consensus"
	memberAppID := "member-bad-hex"

	filename := constants.SecretsFileConsensusMemberKeyPrefix + consensusID + "_" + memberAppID + ".key"
	keyPath := filepath.Join(secretsDir, filename)
	err := os.WriteFile(keyPath, []byte("not-valid-hex!!"), constants.PermFilePrivate)
	require.NoError(t, err)

	provider := NewFileKeyProvider(secretsDir, consensusID)
	_, err = provider.GetMemberKey(memberAppID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode seed")
}

func TestSaveMemberKey_CreatesDirectoryAndFile(t *testing.T) {
	t.Parallel()

	secretsDir := filepath.Join(testutil.TempDir(t), "nested", "secrets")
	consensusID := "test-consensus"
	memberAppID := "member-1"

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	err = SaveMemberKey(secretsDir, consensusID, memberAppID, priv)
	require.NoError(t, err)

	filename := constants.SecretsFileConsensusMemberKeyPrefix + consensusID + "_" + memberAppID + ".key"
	keyPath := filepath.Join(secretsDir, filename)

	info, err := os.Stat(keyPath)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(constants.PermFilePrivate), info.Mode().Perm(), "key file should have private permissions")
	}

	seedHex, err := os.ReadFile(keyPath)
	require.NoError(t, err)

	seed, err := hex.DecodeString(string(seedHex))
	require.NoError(t, err)
	assert.Len(t, seed, ed25519.SeedSize)

	loadedPriv := ed25519.NewKeyFromSeed(seed)
	assert.Equal(t, priv, loadedPriv, "loaded key should match saved key")
}

func TestFileKeyProvider_MultipleMembers(t *testing.T) {
	t.Parallel()

	secretsDir := testutil.TempDir(t)
	consensusID := "multi-consensus"

	members := []string{"member-0", "member-1", "member-2"}
	savedKeys := make(map[string]ed25519.PrivateKey)

	for _, appID := range members {
		_, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		err = SaveMemberKey(secretsDir, consensusID, appID, priv)
		require.NoError(t, err)
		savedKeys[appID] = priv
	}

	provider := NewFileKeyProvider(secretsDir, consensusID)
	for _, appID := range members {
		loadedKey, err := provider.GetMemberKey(appID)
		require.NoError(t, err)
		assert.Equal(t, savedKeys[appID], loadedKey, "key for %s should match", appID)
	}
}
