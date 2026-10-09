// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package serve

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/paths"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/gateway"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore/keystoretest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

type bootstrapFixture struct {
	consensus *gateway.ConsensusStoreService
	signers   *gateway.SignerStoreService
	fileSvc   fs.RuntimeFileService
}

// newBootstrapFixture opens the real canonical gateway database (SQLite,
// encryption vault, document store) in an isolated runtime tree and returns
// the consensus and signer stores the bootstrap writes to.
func newBootstrapFixture(t *testing.T) bootstrapFixture {
	t.Helper()
	base := testutil.TempDir(t)
	require.NoError(t, paths.InitWithBase(base))
	fileSvc, err := fs.NewRuntimeFileService(base, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(t.Context()))

	ks, err := keystore.NewWithKeyringAndFS(testutil.NewTestLogger(), keystoretest.NewMemoryKeyring(), fileSvc)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.EnforcePermissions())

	db, err := gateway.OpenCanonicalDBService(testutil.NewTestLogger(), "", ks, fileSvc)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return bootstrapFixture{consensus: db.GetConsensusStore(), signers: db.GetSignerStore(), fileSvc: fileSvc}
}

func writeConsensusBootstrapFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(testutil.TempDir(t), constants.ConsensusBootstrapConfigFilename)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func seedHexOf(fill byte) string {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = fill
	}
	return hex.EncodeToString(seed)
}

func publicHexOfSeed(t *testing.T, seedHex string) string {
	t.Helper()
	seed, err := hex.DecodeString(seedHex)
	require.NoError(t, err)
	return hex.EncodeToString(ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey))
}

func (f bootstrapFixture) memberKeySeed(t *testing.T, consensusID, appID string) string {
	t.Helper()
	rel := filepath.Join(constants.SecretsDirname, fmt.Sprintf("%s%s_%s.key", constants.SecretsFileConsensusMemberKeyPrefix, consensusID, appID))
	data, err := f.fileSvc.ReadFile(t.Context(), rel)
	require.NoError(t, err, "member key for %s must be saved for the in-process deliberator", appID)
	info, err := f.fileSvc.Stat(t.Context(), rel)
	require.NoError(t, err)
	assert.Equal(t, testutil.FileMode(constants.PermFilePrivate, info.IsDir()), info.Mode().Perm(), "member signing keys must be 0600")
	return string(data)
}

func (f bootstrapFixture) signerPublicKeys(t *testing.T) map[string]string {
	t.Helper()
	signers, err := f.signers.ListTrustedSigners(t.Context())
	require.NoError(t, err)
	out := make(map[string]string, len(signers))
	for _, s := range signers {
		assert.True(t, s.Enabled, "bootstrap registers enabled signers only")
		out[s.ID] = s.PublicKey
	}
	return out
}

func TestConsensusPolicyBootstrap_SharedSeedRegistersEveryMemberWithTheSameKey(t *testing.T) {
	f := newBootstrapFixture(t)
	seed := seedHexOf(0x11)
	path := writeConsensusBootstrapFile(t, fmt.Sprintf(`{"consensus_id":"tribunal","member_app_ids":["alpha","beta"],"quorum":2,"seed_hex":%q}`, seed))

	require.NoError(t, consensusPolicyBootstrap(t.Context(), f.consensus,f.signers, path, f.fileSvc, testutil.NewTestLogger()))

	wantPub := publicHexOfSeed(t, seed)
	assert.Equal(t, map[string]string{"alpha": wantPub, "beta": wantPub}, f.signerPublicKeys(t))
	assert.Equal(t, seed, f.memberKeySeed(t, "tribunal", "alpha"))
	assert.Equal(t, seed, f.memberKeySeed(t, "tribunal", "beta"))

	policy, err := f.consensus.GetConsensus(t.Context(), "tribunal")
	require.NoError(t, err)
	require.NotNil(t, policy)
	assert.Equal(t, []string{"alpha", "beta"}, policy.MemberAppIDs)
	assert.Equal(t, 2, policy.Quorum)
	assert.True(t, policy.RequireDistinct)
	assert.True(t, policy.Enabled)
}

func TestConsensusPolicyBootstrap_PerMemberSeedsGiveEachMemberADistinctKeyAndWinOverSharedSeed(t *testing.T) {
	f := newBootstrapFixture(t)
	alphaSeed, betaSeed, gammaSeed := seedHexOf(0x21), seedHexOf(0x22), seedHexOf(0x23)
	path := writeConsensusBootstrapFile(t, fmt.Sprintf(
		`{"consensus_id":"tribunal","member_app_ids":["alpha","beta","gamma"],"quorum":2,`+
			`"seed_hex":%q,"member_seeds":{"alpha":%q,"beta":%q,"gamma":%q}}`,
		seedHexOf(0xEE), alphaSeed, betaSeed, gammaSeed))

	require.NoError(t, consensusPolicyBootstrap(t.Context(), f.consensus,f.signers, path, f.fileSvc, testutil.NewTestLogger()))

	keys := f.signerPublicKeys(t)
	assert.Equal(t, publicHexOfSeed(t, alphaSeed), keys["alpha"])
	assert.Equal(t, publicHexOfSeed(t, betaSeed), keys["beta"])
	assert.Equal(t, publicHexOfSeed(t, gammaSeed), keys["gamma"])
	assert.NotEqual(t, keys["alpha"], keys["beta"], "per-member keys make RequireDistinct cryptographically meaningful")
	assert.NotContains(t, keys, "", "no signer without a key")
	for appID, seed := range map[string]string{"alpha": alphaSeed, "beta": betaSeed, "gamma": gammaSeed} {
		assert.Equal(t, seed, f.memberKeySeed(t, "tribunal", appID))
		assert.NotEqual(t, publicHexOfSeed(t, seedHexOf(0xEE)), keys[appID], "the shared seed must be ignored once member_seeds is present")
	}
}

func TestConsensusPolicyBootstrap_GeneratesOneSharedKeyWhenNoSeedIsConfigured(t *testing.T) {
	f := newBootstrapFixture(t)
	path := writeConsensusBootstrapFile(t, `{"consensus_id":"ephemeral","member_app_ids":["solo-a","solo-b"],"quorum":1}`)

	require.NoError(t, consensusPolicyBootstrap(t.Context(), f.consensus,f.signers, path, f.fileSvc, testutil.NewTestLogger()))

	keys := f.signerPublicKeys(t)
	require.Len(t, keys, 2)
	assert.Equal(t, keys["solo-a"], keys["solo-b"], "the single-key ensemble pattern shares one generated key")
	seedA := f.memberKeySeed(t, "ephemeral", "solo-a")
	assert.Equal(t, seedA, f.memberKeySeed(t, "ephemeral", "solo-b"))
	assert.Equal(t, keys["solo-a"], publicHexOfSeed(t, seedA), "the saved private key must match the registered public key")
}

func TestConsensusPolicyBootstrap_IsIdempotentAndNeverRotatesExistingKeys(t *testing.T) {
	f := newBootstrapFixture(t)
	firstSeed := seedHexOf(0x31)
	first := writeConsensusBootstrapFile(t, fmt.Sprintf(`{"consensus_id":"tribunal","member_app_ids":["alpha"],"quorum":1,"seed_hex":%q}`, firstSeed))
	require.NoError(t, consensusPolicyBootstrap(t.Context(), f.consensus,f.signers, first, f.fileSvc, testutil.NewTestLogger()))
	keysBefore := f.signerPublicKeys(t)

	rotated := writeConsensusBootstrapFile(t, fmt.Sprintf(`{"consensus_id":"tribunal","member_app_ids":["alpha"],"quorum":1,"seed_hex":%q}`, seedHexOf(0x32)))
	require.NoError(t, consensusPolicyBootstrap(t.Context(), f.consensus,f.signers, rotated, f.fileSvc, testutil.NewTestLogger()))

	assert.Equal(t, keysBefore, f.signerPublicKeys(t), "an existing consensus must not have its signers replaced")
	assert.Equal(t, firstSeed, f.memberKeySeed(t, "tribunal", "alpha"), "an existing consensus must not have its member key overwritten")
}

func TestConsensusPolicyBootstrap_FailsClosedWithoutCreatingAPolicy(t *testing.T) {
	validSeed := seedHexOf(0x41)
	tests := []struct {
		name     string
		body     string
		wantErr  error
		wantText string
	}{
		{
			name:     "member missing from member_seeds",
			body:     fmt.Sprintf(`{"consensus_id":"c1","member_app_ids":["alpha","beta"],"quorum":1,"member_seeds":{"alpha":%q}}`, validSeed),
			wantErr:  constants.ErrConsensusBootstrapMissingFields,
			wantText: "member beta has no seed",
		},
		{
			name:    "shared seed is not hex",
			body:    `{"consensus_id":"c1","member_app_ids":["alpha"],"quorum":1,"seed_hex":"zz"}`,
			wantErr: constants.ErrConsensusBootstrapDecodeSeed,
		},
		{
			name:    "shared seed has the wrong length",
			body:    `{"consensus_id":"c1","member_app_ids":["alpha"],"quorum":1,"seed_hex":"abcd"}`,
			wantErr: constants.ErrInvalidSeedLength,
		},
		{
			name:    "member seed is not hex",
			body:    `{"consensus_id":"c1","member_app_ids":["alpha"],"quorum":1,"member_seeds":{"alpha":"zz"}}`,
			wantErr: constants.ErrConsensusBootstrapDecodeSeed,
		},
		{
			name:    "member seed has the wrong length",
			body:    `{"consensus_id":"c1","member_app_ids":["alpha"],"quorum":1,"member_seeds":{"alpha":"abcd"}}`,
			wantErr: constants.ErrInvalidSeedLength,
		},
		{
			name:     "quorum exceeds the member count",
			body:     fmt.Sprintf(`{"consensus_id":"c1","member_app_ids":["alpha"],"quorum":2,"seed_hex":%q}`, validSeed),
			wantErr:  constants.ErrConstraintViolation,
			wantText: "quorum cannot exceed member count",
		},
		{
			name:     "duplicate members",
			body:     fmt.Sprintf(`{"consensus_id":"c1","member_app_ids":["alpha","alpha"],"quorum":1,"seed_hex":%q}`, validSeed),
			wantErr:  constants.ErrConstraintViolation,
			wantText: "duplicate member_app_id",
		},
		{
			name:    "consensus id with illegal characters",
			body:    fmt.Sprintf(`{"consensus_id":"bad id!","member_app_ids":["alpha"],"quorum":1,"seed_hex":%q}`, validSeed),
			wantErr: constants.ErrConsensusInvalidID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newBootstrapFixture(t)
			path := writeConsensusBootstrapFile(t, tt.body)

			err := consensusPolicyBootstrap(t.Context(), f.consensus,f.signers, path, f.fileSvc, testutil.NewTestLogger())

			require.ErrorIs(t, err, tt.wantErr)
			if tt.wantText != "" {
				assert.True(t, strings.Contains(err.Error(), tt.wantText), "error %q should mention %q", err.Error(), tt.wantText)
			}
			policies, listErr := f.consensus.ListConsensus(t.Context())
			require.NoError(t, listErr)
			assert.Empty(t, policies, "a failed bootstrap must not leave a consensus policy that would be skipped as already-bootstrapped on retry")
		})
	}
}

func TestConsensusPolicyBootstrap_RetryAfterFailureCompletesTheBootstrap(t *testing.T) {
	f := newBootstrapFixture(t)
	seed := seedHexOf(0x51)
	bad := writeConsensusBootstrapFile(t, fmt.Sprintf(`{"consensus_id":"tribunal","member_app_ids":["alpha","beta"],"quorum":3,"seed_hex":%q}`, seed))
	require.Error(t, consensusPolicyBootstrap(t.Context(), f.consensus,f.signers, bad, f.fileSvc, testutil.NewTestLogger()))

	good := writeConsensusBootstrapFile(t, fmt.Sprintf(`{"consensus_id":"tribunal","member_app_ids":["alpha","beta"],"quorum":2,"seed_hex":%q}`, seed))
	require.NoError(t, consensusPolicyBootstrap(t.Context(), f.consensus,f.signers, good, f.fileSvc, testutil.NewTestLogger()))

	policy, err := f.consensus.GetConsensus(t.Context(), "tribunal")
	require.NoError(t, err)
	require.NotNil(t, policy)
	assert.Equal(t, 2, policy.Quorum)
	assert.True(t, policy.RequireDistinct)
	assert.True(t, policy.Enabled)
}
