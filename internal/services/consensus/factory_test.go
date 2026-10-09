// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package consensus

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore/keystoretest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func newConsensusTestFileService(t *testing.T) fs.RuntimeFileService {
	t.Helper()
	root := testutil.TempDir(t)
	fileSvc, err := fs.NewRuntimeFileService(root, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	return fileSvc
}

func newConsensusTestKeystore(t *testing.T, fileSvc fs.RuntimeFileService) *keystore.Keystore {
	t.Helper()
	ks, err := keystore.NewWithKeyringAndFS(testutil.NewTestLogger(), keystoretest.NewMemoryKeyring(), fileSvc)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	return ks
}

func TestKeystoreKeyProvider_GetMemberKey_Success(t *testing.T) {
	t.Parallel()

	fileSvc := newConsensusTestFileService(t)
	ks := newConsensusTestKeystore(t, fileSvc)
	consensusID := "test-consensus"
	memberAppID := "member-1"

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	err = SaveMemberKey(ks, consensusID, memberAppID, priv)
	require.NoError(t, err)

	provider, err := NewKeystoreKeyProvider(ks, consensusID)
	require.NoError(t, err)
	loadedKey, err := provider.GetMemberKey(memberAppID)
	require.NoError(t, err)

	assert.Equal(t, priv, loadedKey, "loaded key should match saved key")
	assert.Equal(t, pub, loadedKey.Public().(ed25519.PublicKey), "public key should match")
}

func TestKeystoreKeyProvider_GetMemberKey_NotFound(t *testing.T) {
	t.Parallel()

	fileSvc := newConsensusTestFileService(t)
	ks := newConsensusTestKeystore(t, fileSvc)
	consensusID := "test-consensus"

	provider, err := NewKeystoreKeyProvider(ks, consensusID)
	require.NoError(t, err)
	_, err = provider.GetMemberKey("nonexistent-member")
	require.Error(t, err)
}

func TestKeystoreKeyProvider_GetMemberKey_WrongSeedLength(t *testing.T) {
	t.Parallel()

	fileSvc := newConsensusTestFileService(t)
	ks := newConsensusTestKeystore(t, fileSvc)
	consensusID := "test-consensus"
	memberAppID := "member-bad"

	secretName := fmt.Sprintf("%s%s_%s.key", constants.SecretsFileConsensusMemberKeyPrefix, consensusID, memberAppID)
	err := ks.StoreKeyMaterial(secretName, []byte("too-short"))
	require.NoError(t, err)

	provider, err := NewKeystoreKeyProvider(ks, consensusID)
	require.NoError(t, err)
	_, err = provider.GetMemberKey(memberAppID)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreInvalidKeyLength)
}

func TestKeystoreKeyProvider_GetMemberKey_InvalidHex(t *testing.T) {
	t.Parallel()

	fileSvc := newConsensusTestFileService(t)
	ks := newConsensusTestKeystore(t, fileSvc)
	consensusID := "test-consensus"
	memberAppID := "member-bad-hex"

	secretName := fmt.Sprintf("%s%s_%s.key", constants.SecretsFileConsensusMemberKeyPrefix, consensusID, memberAppID)
	err := ks.EncryptSecret(secretName, "not-valid-hex!!")
	require.NoError(t, err)

	provider, err := NewKeystoreKeyProvider(ks, consensusID)
	require.NoError(t, err)
	_, err = provider.GetMemberKey(memberAppID)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrKeyStoreDecodeFailed)
}

func TestKeystoreKeyProvider_MultipleMembers(t *testing.T) {
	t.Parallel()

	fileSvc := newConsensusTestFileService(t)
	ks := newConsensusTestKeystore(t, fileSvc)
	consensusID := "multi-consensus"

	members := []string{"member-0", "member-1", "member-2"}
	savedKeys := make(map[string]ed25519.PrivateKey)

	for _, appID := range members {
		_, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		err = SaveMemberKey(ks, consensusID, appID, priv)
		require.NoError(t, err)
		savedKeys[appID] = priv
	}

	provider, err := NewKeystoreKeyProvider(ks, consensusID)
	require.NoError(t, err)
	for _, appID := range members {
		loadedKey, err := provider.GetMemberKey(appID)
		require.NoError(t, err)
		assert.Equal(t, savedKeys[appID], loadedKey, "key for %s should match", appID)
	}
}

func TestSaveMemberKey_NoPlaintextOnDisk(t *testing.T) {
	t.Parallel()

	fileSvc := newConsensusTestFileService(t)
	ks := newConsensusTestKeystore(t, fileSvc)
	consensusID := "test-consensus"
	memberAppID := "member-secret"

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	err = SaveMemberKey(ks, consensusID, memberAppID, priv)
	require.NoError(t, err)

	seed := priv.Seed()
	seedHex := hex.EncodeToString(seed)

	entries, err := fileSvc.ReadDir(context.Background(), constants.SecretsDirname)
	require.NoError(t, err)
	require.NotEmpty(t, entries, "secrets directory should have entries")

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := fileSvc.ReadFile(context.Background(), constants.SecretsDirname+"/"+entry.Name())
		require.NoError(t, err)
		assert.False(t, bytes.Contains(content, []byte(seedHex)),
			"file %s contains plaintext hex seed", entry.Name())
		assert.False(t, bytes.Contains(content, seed),
			"file %s contains raw seed bytes", entry.Name())
	}
}
