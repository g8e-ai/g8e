// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/governance/governancetest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func newSignerTestFileService(t *testing.T) (fs.RuntimeFileService, string) {
	t.Helper()
	fileSvc, err := fs.NewRuntimeFileService(testutil.TempDir(t), testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))
	return fileSvc, filepath.Join(constants.PkiDirname, constants.PkiSubdirTrustedSigners)
}

func TestFailClosedSignerStore_NilMap(t *testing.T) {
	t.Parallel()
	s := &FailClosedSignerStore{}
	pubKey, err := s.GetTrustedSigner("key1")
	require.NoError(t, err)
	assert.Nil(t, pubKey)
}

func TestFailClosedSignerStore_NotFound(t *testing.T) {
	t.Parallel()
	s := &FailClosedSignerStore{Signers: map[string]ed25519.PublicKey{}}
	pubKey, err := s.GetTrustedSigner("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, pubKey)
}

func TestFailClosedSignerStore_Found(t *testing.T) {
	t.Parallel()
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	s := &FailClosedSignerStore{Signers: map[string]ed25519.PublicKey{"key1": pub}}
	result, err := s.GetTrustedSigner("key1")
	require.NoError(t, err)
	assert.Equal(t, pub, result)
}

func TestFilesystemSignerStore_NonexistentDir(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	fileSvc, relDir := newSignerTestFileService(t)
	_, err := NewFilesystemSignerStore(fileSvc, filepath.Join(relDir, "missing"), logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read trusted signers directory")
}

func TestFilesystemSignerStore_EmptyDir(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	fileSvc, dir := newSignerTestFileService(t)
	store, err := NewFilesystemSignerStore(fileSvc, dir, logger)
	require.NoError(t, err)
	assert.NotNil(t, store)

	pubKey, err := store.GetTrustedSigner("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, pubKey)
}

func TestFilesystemSignerStore_WithValidKey(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	fileSvc, dir := newSignerTestFileService(t)

	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	hexKey := hex.EncodeToString(pub)
	keyPath := filepath.Join(dir, "signer1.pub")
	require.NoError(t, fileSvc.WriteFile(context.Background(), keyPath, []byte(hexKey), constants.PermFilePublic))

	store, err := NewFilesystemSignerStore(fileSvc, dir, logger)
	require.NoError(t, err)

	result, err := store.GetTrustedSigner("signer1")
	require.NoError(t, err)
	assert.Equal(t, pub, result)
}

func TestFilesystemSignerStore_WithInvalidHex(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	fileSvc, dir := newSignerTestFileService(t)

	require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(dir, "bad.pub"), []byte("not-hex!"), constants.PermFilePublic))

	store, err := NewFilesystemSignerStore(fileSvc, dir, logger)
	require.NoError(t, err)

	pubKey, err := store.GetTrustedSigner("bad")
	require.NoError(t, err)
	assert.Nil(t, pubKey, "malformed key should not be loaded")
}

func TestFilesystemSignerStore_WithWrongKeySize(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	fileSvc, dir := newSignerTestFileService(t)

	shortKey := hex.EncodeToString([]byte("short"))
	require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(dir, "short.pub"), []byte(shortKey), constants.PermFilePublic))

	store, err := NewFilesystemSignerStore(fileSvc, dir, logger)
	require.NoError(t, err)

	pubKey, err := store.GetTrustedSigner("short")
	require.NoError(t, err)
	assert.Nil(t, pubKey, "wrong-size key should not be loaded")
}

func TestFilesystemSignerStore_SkipsNonPubFiles(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	fileSvc, dir := newSignerTestFileService(t)

	require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(dir, "readme.txt"), []byte("ignore me"), constants.PermFilePublic))
	require.NoError(t, fileSvc.MkdirAll(context.Background(), filepath.Join(dir, "subdir"), constants.PermDirStandard))

	store, err := NewFilesystemSignerStore(fileSvc, dir, logger)
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestFilesystemSignerStore_GetTrustedSigner_NilMap(t *testing.T) {
	t.Parallel()
	store := &FilesystemSignerStore{signers: nil}
	pubKey, err := store.GetTrustedSigner("key1")
	require.NoError(t, err)
	assert.Nil(t, pubKey)
}

func TestPostureDescription_AllPostures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		posture  GovernancePosture
		expected string
	}{
		{"doctrine", &DoctrinePosture{}, "doctrine (L1 enforced, L2/L3 audited)"},
		{"consensus", &ConsensusPosture{}, "consensus (L1/L2 enforced, L3 audited)"},
		{"notary", &NotaryPosture{}, "notary (L1/L2/L3 strictly enforced)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.name, tt.posture.Name())
			assert.Equal(t, tt.expected, tt.posture.Description())
		})
	}
}

func TestParseGovernancePosture_Invalid(t *testing.T) {
	t.Parallel()
	_, err := ParseGovernancePosture("invalid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid governance posture")
}

func TestL4Warden_Doctrine(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	warden := NewL4Warden(
		logger,
		nil,
		&governancetest.SimpleStateRootProvider{Root: "root"},
		&FailClosedSignerStore{},
		&consensusStoreTestAdapter{Inner: &governancetest.SimpleConsensusStore{}},
		nil,
		NewL1Doctrine(),
		[]constants.ActionType{constants.ActionTypeFileEdit},
		nil,
	)

	assert.NotNil(t, warden.Doctrine())
}

func TestL5Actuator_Wait_NoGoroutines(t *testing.T) {
	t.Parallel()
	actuator := &L5Actuator{}
	actuator.Wait()
}
