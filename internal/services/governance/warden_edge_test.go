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

package governance

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleAppPolicyStore_NilMap(t *testing.T) {
	t.Parallel()
	s := &SimpleAppPolicyStore{}
	policy, err := s.GetAppPolicy("app1")
	require.NoError(t, err)
	assert.Nil(t, policy)
}

func TestSimpleAppPolicyStore_NotFound(t *testing.T) {
	t.Parallel()
	s := &SimpleAppPolicyStore{Policies: map[string]*models.AppPolicy{}}
	policy, err := s.GetAppPolicy("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, policy)
}

func TestSimpleAppPolicyStore_Found(t *testing.T) {
	t.Parallel()
	expected := &models.AppPolicy{AppID: "app1"}
	s := &SimpleAppPolicyStore{Policies: map[string]*models.AppPolicy{"app1": expected}}
	policy, err := s.GetAppPolicy("app1")
	require.NoError(t, err)
	assert.Equal(t, expected, policy)
}

func TestSimpleSignerStore_NilMap(t *testing.T) {
	t.Parallel()
	s := &SimpleSignerStore{}
	pubKey, err := s.GetTrustedSigner("key1")
	require.NoError(t, err)
	assert.Nil(t, pubKey)
}

func TestSimpleSignerStore_NotFound(t *testing.T) {
	t.Parallel()
	s := &SimpleSignerStore{Signers: map[string]ed25519.PublicKey{}}
	pubKey, err := s.GetTrustedSigner("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, pubKey)
}

func TestSimpleSignerStore_Found(t *testing.T) {
	t.Parallel()
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	s := &SimpleSignerStore{Signers: map[string]ed25519.PublicKey{"key1": pub}}
	result, err := s.GetTrustedSigner("key1")
	require.NoError(t, err)
	assert.Equal(t, pub, result)
}

func TestSimpleStateRootProvider_EmptyRoot(t *testing.T) {
	t.Parallel()
	s := &SimpleStateRootProvider{Root: ""}
	_, err := s.GetCurrentStateRoot()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrTxProviderMisconfigured)
}

func TestSimpleStateRootProvider_ValidRoot(t *testing.T) {
	t.Parallel()
	s := &SimpleStateRootProvider{Root: "abc123"}
	root, err := s.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, "abc123", root)
}

func TestSimpleTribunalStore_NilMap(t *testing.T) {
	t.Parallel()
	s := &SimpleTribunalStore{}
	tribunal, err := s.GetTribunal("trib1")
	require.NoError(t, err)
	assert.Nil(t, tribunal)
}

func TestSimpleTribunalStore_NotFound(t *testing.T) {
	t.Parallel()
	s := &SimpleTribunalStore{Tribunals: map[string]*models.TribunalPolicy{}}
	tribunal, err := s.GetTribunal("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, tribunal)
}

func TestFilesystemSignerStore_NonexistentDir(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	_, err := NewFilesystemSignerStore("/nonexistent/path/that/does/not/exist", logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read trusted signers directory")
}

func TestFilesystemSignerStore_EmptyDir(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dir := testutil.TempDir(t)
	store, err := NewFilesystemSignerStore(dir, logger)
	require.NoError(t, err)
	assert.NotNil(t, store)

	pubKey, err := store.GetTrustedSigner("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, pubKey)
}

func TestFilesystemSignerStore_WithValidKey(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dir := testutil.TempDir(t)

	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	hexKey := hex.EncodeToString(pub)
	keyPath := filepath.Join(dir, "signer1.pub")
	require.NoError(t, os.WriteFile(keyPath, []byte(hexKey), 0644))

	store, err := NewFilesystemSignerStore(dir, logger)
	require.NoError(t, err)

	result, err := store.GetTrustedSigner("signer1")
	require.NoError(t, err)
	assert.Equal(t, pub, result)
}

func TestFilesystemSignerStore_WithInvalidHex(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dir := testutil.TempDir(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.pub"), []byte("not-hex!"), 0644))

	store, err := NewFilesystemSignerStore(dir, logger)
	require.NoError(t, err)

	pubKey, err := store.GetTrustedSigner("bad")
	require.NoError(t, err)
	assert.Nil(t, pubKey, "malformed key should not be loaded")
}

func TestFilesystemSignerStore_WithWrongKeySize(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dir := testutil.TempDir(t)

	shortKey := hex.EncodeToString([]byte("short"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "short.pub"), []byte(shortKey), 0644))

	store, err := NewFilesystemSignerStore(dir, logger)
	require.NoError(t, err)

	pubKey, err := store.GetTrustedSigner("short")
	require.NoError(t, err)
	assert.Nil(t, pubKey, "wrong-size key should not be loaded")
}

func TestFilesystemSignerStore_SkipsNonPubFiles(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dir := testutil.TempDir(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "subdir"), []byte("dir"), 0644))

	store, err := NewFilesystemSignerStore(dir, logger)
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestFilesystemSignerStore_GetTrustedSigner_NilMap(t *testing.T) {
	t.Parallel()
	store := &FilesystemSignerStore{signers: nil, logger: testutil.NewTestLogger()}
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

func TestL4Warden_Posture_And_Doctrine(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	warden := NewL4Warden(
		logger,
		nil,
		&SimpleStateRootProvider{Root: "root"},
		&SimpleSignerStore{},
		&SimpleTribunalStore{},
		&SimpleAppPolicyStore{},
		nil,
		NewL1Doctrine(),
		[]constants.ActionType{constants.ActionTypeFileEdit},
		"consensus",
		nil,
	)

	assert.Equal(t, "consensus", warden.Posture().Name())
	assert.NotNil(t, warden.Doctrine())
}

func TestL5Actuator_Wait_NoGoroutines(t *testing.T) {
	t.Parallel()
	actuator := &L5Actuator{}
	actuator.Wait()
}
