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

//go:build integration

package gateway

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/models"
)

func TestConsensusStoreService_AddConsensus(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	consensusSvc := infra.Stores.ConsensusStore

	// Generate test signers
	pubKey1, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pubKey2, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pubKey3, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	signer1 := models.TrustedSigner{
		ID:        "member-1",
		PublicKey: hex.EncodeToString(pubKey1),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	signer2 := models.TrustedSigner{
		ID:        "member-2",
		PublicKey: hex.EncodeToString(pubKey2),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	disabledSigner := models.TrustedSigner{
		ID:        "disabled-member",
		PublicKey: hex.EncodeToString(pubKey3),
		AddedAt:   time.Now().UTC(),
		Enabled:   false,
	}

	// Register signers
	err = infra.Stores.SignerStore.AddTrustedSigner(signer1)
	require.NoError(t, err)
	t.Cleanup(func() { infra.Stores.SignerStore.DeleteTrustedSigner("member-1") })

	err = infra.Stores.SignerStore.AddTrustedSigner(signer2)
	require.NoError(t, err)
	t.Cleanup(func() { infra.Stores.SignerStore.DeleteTrustedSigner("member-2") })

	err = infra.Stores.SignerStore.AddTrustedSigner(disabledSigner)
	require.NoError(t, err)
	t.Cleanup(func() { infra.Stores.SignerStore.DeleteTrustedSigner("disabled-member") })

	tests := []struct {
		name        string
		policy      models.ConsensusPolicy
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid consensus",
			policy: models.ConsensusPolicy{
				ID:              "test-consensus",
				MemberAppIDs:    []string{"member-1", "member-2"},
				Quorum:          2,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: false,
		},
		{
			name: "empty ID",
			policy: models.ConsensusPolicy{
				ID:              "",
				MemberAppIDs:    []string{"member-1"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "consensus ID",
		},
		{
			name: "empty member IDs",
			policy: models.ConsensusPolicy{
				ID:              "test-consensus-2",
				MemberAppIDs:    []string{},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "member_app_ids",
		},
		{
			name: "quorum less than 1",
			policy: models.ConsensusPolicy{
				ID:              "test-consensus-3",
				MemberAppIDs:    []string{"member-1"},
				Quorum:          0,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "quorum must be >= 1",
		},
		{
			name: "quorum exceeds member count",
			policy: models.ConsensusPolicy{
				ID:              "test-consensus-4",
				MemberAppIDs:    []string{"member-1"},
				Quorum:          2,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "quorum cannot exceed member count",
		},
		{
			name: "duplicate member IDs",
			policy: models.ConsensusPolicy{
				ID:              "test-consensus-5",
				MemberAppIDs:    []string{"member-1", "member-1"},
				Quorum:          2,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "duplicate member_app_id",
		},
		{
			name: "unknown member ID",
			policy: models.ConsensusPolicy{
				ID:              "test-consensus-6",
				MemberAppIDs:    []string{"unknown-member"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "not an enabled trusted signer",
		},
		{
			name: "empty string in member IDs",
			policy: models.ConsensusPolicy{
				ID:              "test-consensus-7",
				MemberAppIDs:    []string{"member-1", ""},
				Quorum:          2,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "cannot contain empty strings",
		},
		{
			name: "invalid consensus ID with special characters",
			policy: models.ConsensusPolicy{
				ID:              "test@consensus",
				MemberAppIDs:    []string{"member-1"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "CONSENSUS_INVALID_ID",
		},
		{
			name: "invalid consensus ID with spaces",
			policy: models.ConsensusPolicy{
				ID:              "test consensus",
				MemberAppIDs:    []string{"member-1"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "CONSENSUS_INVALID_ID",
		},
		{
			name: "invalid consensus ID with path separator",
			policy: models.ConsensusPolicy{
				ID:              "test/consensus",
				MemberAppIDs:    []string{"member-1"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "CONSENSUS_INVALID_ID",
		},
		{
			name: "disabled new consensus rejected",
			policy: models.ConsensusPolicy{
				ID:              "test-consensus-disabled",
				MemberAppIDs:    []string{"member-1"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         false,
			},
			expectError: true,
			errorMsg:    "CONSENSUS_MUST_BE_ENABLED",
		},
		{
			name: "disabled signer as member rejected",
			policy: models.ConsensusPolicy{
				ID:              "test-consensus-disabled-signer",
				MemberAppIDs:    []string{"disabled-member"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "not an enabled trusted signer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := consensusSvc.AddConsensus(tt.policy)
			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
				// Cleanup
				t.Cleanup(func() { consensusSvc.DeleteConsensus(tt.policy.ID) })
			}
		})
	}
}

func TestConsensusStoreService_GetConsensus(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	consensusSvc := infra.Stores.ConsensusStore

	// Create a test signer
	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer := models.TrustedSigner{
		ID:        "test-member",
		PublicKey: hex.EncodeToString(pubKey),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	err = infra.Stores.SignerStore.AddTrustedSigner(signer)
	require.NoError(t, err)
	t.Cleanup(func() { infra.Stores.SignerStore.DeleteTrustedSigner("test-member") })

	// Create a test consensus
	policy := models.ConsensusPolicy{
		ID:              "get-test-consensus",
		MemberAppIDs:    []string{"test-member"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	err = consensusSvc.AddConsensus(policy)
	require.NoError(t, err)
	t.Cleanup(func() { consensusSvc.DeleteConsensus("get-test-consensus") })

	// Get the consensus
	retrieved, err := consensusSvc.GetConsensus("get-test-consensus")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, "get-test-consensus", retrieved.ID)
	assert.Equal(t, []string{"test-member"}, retrieved.MemberAppIDs)
	assert.Equal(t, 1, retrieved.Quorum)
	assert.True(t, retrieved.RequireDistinct)
	assert.True(t, retrieved.Enabled)

	// Get non-existent consensus
	retrieved, err = consensusSvc.GetConsensus("non-existent")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestConsensusStoreService_ListConsensus(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	consensusSvc := infra.Stores.ConsensusStore

	// Create test signers
	pubKey1, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pubKey2, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	signer1 := models.TrustedSigner{
		ID:        "list-member-1",
		PublicKey: hex.EncodeToString(pubKey1),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	signer2 := models.TrustedSigner{
		ID:        "list-member-2",
		PublicKey: hex.EncodeToString(pubKey2),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}

	err = infra.Stores.SignerStore.AddTrustedSigner(signer1)
	require.NoError(t, err)
	t.Cleanup(func() { infra.Stores.SignerStore.DeleteTrustedSigner("list-member-1") })

	err = infra.Stores.SignerStore.AddTrustedSigner(signer2)
	require.NoError(t, err)
	t.Cleanup(func() { infra.Stores.SignerStore.DeleteTrustedSigner("list-member-2") })

	// Create test consensus
	policy1 := models.ConsensusPolicy{
		ID:              "list-consensus-1",
		MemberAppIDs:    []string{"list-member-1"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	err = consensusSvc.AddConsensus(policy1)
	require.NoError(t, err)
	t.Cleanup(func() { consensusSvc.DeleteConsensus("list-consensus-1") })

	policy2 := models.ConsensusPolicy{
		ID:              "list-consensus-2",
		MemberAppIDs:    []string{"list-member-2"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	err = consensusSvc.AddConsensus(policy2)
	require.NoError(t, err)
	t.Cleanup(func() { consensusSvc.DeleteConsensus("list-consensus-2") })

	// List consensus
	consensus, err := consensusSvc.ListConsensus()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(consensus), 2)

	// Verify our consensus are in the list
	ids := make(map[string]bool)
	for _, t := range consensus {
		ids[t.ID] = true
	}
	assert.True(t, ids["list-consensus-1"])
	assert.True(t, ids["list-consensus-2"])
}

func TestConsensusStoreService_DeleteConsensus(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	consensusSvc := infra.Stores.ConsensusStore

	// Create a test signer
	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer := models.TrustedSigner{
		ID:        "delete-member",
		PublicKey: hex.EncodeToString(pubKey),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	err = infra.Stores.SignerStore.AddTrustedSigner(signer)
	require.NoError(t, err)
	t.Cleanup(func() { infra.Stores.SignerStore.DeleteTrustedSigner("delete-member") })

	// Create a test consensus
	policy := models.ConsensusPolicy{
		ID:              "delete-test-consensus",
		MemberAppIDs:    []string{"delete-member"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	err = consensusSvc.AddConsensus(policy)
	require.NoError(t, err)

	// Delete the consensus
	deleted, err := consensusSvc.DeleteConsensus("delete-test-consensus")
	require.NoError(t, err)
	assert.True(t, deleted)

	// Verify it's gone
	retrieved, err := consensusSvc.GetConsensus("delete-test-consensus")
	require.NoError(t, err)
	assert.Nil(t, retrieved)

	// Delete non-existent consensus
	deleted, err = consensusSvc.DeleteConsensus("non-existent")
	require.NoError(t, err)
	assert.False(t, deleted)
}

func TestConsensusStoreService_UpdateDisableConsensus(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	consensusSvc := infra.Stores.ConsensusStore

	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer := models.TrustedSigner{
		ID:        "update-member",
		PublicKey: hex.EncodeToString(pubKey),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	err = infra.Stores.SignerStore.AddTrustedSigner(signer)
	require.NoError(t, err)
	t.Cleanup(func() { infra.Stores.SignerStore.DeleteTrustedSigner("update-member") })

	// Create enabled consensus
	policy := models.ConsensusPolicy{
		ID:              "update-consensus",
		MemberAppIDs:    []string{"update-member"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	err = consensusSvc.AddConsensus(policy)
	require.NoError(t, err)
	t.Cleanup(func() { consensusSvc.DeleteConsensus("update-consensus") })

	// Update: disable the existing consensus (should succeed)
	policy.Enabled = false
	err = consensusSvc.AddConsensus(policy)
	require.NoError(t, err)

	// Verify it's stored as disabled
	retrieved, err := consensusSvc.GetConsensus("update-consensus")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.False(t, retrieved.Enabled)
}

func TestConsensusStoreService_AddConsensus_AlreadyExists(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	consensusSvc := infra.Stores.ConsensusStore

	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer := models.TrustedSigner{
		ID:        "exists-member",
		PublicKey: hex.EncodeToString(pubKey),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	err = infra.Stores.SignerStore.AddTrustedSigner(signer)
	require.NoError(t, err)
	t.Cleanup(func() { infra.Stores.SignerStore.DeleteTrustedSigner("exists-member") })

	policy := models.ConsensusPolicy{
		ID:              "exists-consensus",
		MemberAppIDs:    []string{"exists-member"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	err = consensusSvc.AddConsensus(policy)
	require.NoError(t, err)
	t.Cleanup(func() { consensusSvc.DeleteConsensus("exists-consensus") })

	// Attempt to create the same consensus again with Enabled=true
	err = consensusSvc.AddConsensus(policy)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestConsensusStoreService_GetConsensusPolicy(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	consensusSvc := infra.Stores.ConsensusStore

	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer := models.TrustedSigner{
		ID:        "consensus-member",
		PublicKey: hex.EncodeToString(pubKey),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	err = infra.Stores.SignerStore.AddTrustedSigner(signer)
	require.NoError(t, err)
	t.Cleanup(func() { infra.Stores.SignerStore.DeleteTrustedSigner("consensus-member") })

	policy := models.ConsensusPolicy{
		ID:              "consensus-test-consensus",
		MemberAppIDs:    []string{"consensus-member"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	err = consensusSvc.AddConsensus(policy)
	require.NoError(t, err)
	t.Cleanup(func() { consensusSvc.DeleteConsensus("consensus-test-consensus") })

	consensusPolicy, err := consensusSvc.GetConsensusPolicy("consensus-test-consensus")
	require.NoError(t, err)
	require.NotNil(t, consensusPolicy)
	assert.Equal(t, []string{"consensus-member"}, consensusPolicy.MemberKeyIDs)
	assert.Equal(t, 1, consensusPolicy.Quorum)
	assert.True(t, consensusPolicy.RequireDistinct)
	assert.True(t, consensusPolicy.Enabled)

	consensusPolicy, err = consensusSvc.GetConsensusPolicy("non-existent")
	require.NoError(t, err)
	assert.Nil(t, consensusPolicy)
}

func TestIsValidConsensusID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"valid-id", true},
		{"valid_id", true},
		{"ValidID123", true},
		{"a", true},
		{"", false},
		{"test@consensus", false},
		{"test consensus", false},
		{"test/consensus", false},
		{"test\\consensus", false},
		{"test:consensus", false},
		{"test.consensus", false},
		{"test\tconsensus", false},
		{"日本語", true}, // unicode letters are allowed
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := isValidConsensusID(tt.id)
			assert.Equal(t, tt.want, got)
		})
	}
}
