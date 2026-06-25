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

func TestTribunalStoreService_AddTribunal(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	tribunalSvc := NewTribunalStoreService(infra.DB.GetDB(), infra.Logger, infra.DB.SignerStore)

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
	err = infra.DB.SignerStore.AddTrustedSigner(signer1)
	require.NoError(t, err)
	t.Cleanup(func() { infra.DB.SignerStore.DeleteTrustedSigner("member-1") })

	err = infra.DB.SignerStore.AddTrustedSigner(signer2)
	require.NoError(t, err)
	t.Cleanup(func() { infra.DB.SignerStore.DeleteTrustedSigner("member-2") })

	err = infra.DB.SignerStore.AddTrustedSigner(disabledSigner)
	require.NoError(t, err)
	t.Cleanup(func() { infra.DB.SignerStore.DeleteTrustedSigner("disabled-member") })

	tests := []struct {
		name        string
		policy      models.TribunalPolicy
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid tribunal",
			policy: models.TribunalPolicy{
				ID:              "test-tribunal",
				MemberAppIDs:    []string{"member-1", "member-2"},
				Quorum:          2,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: false,
		},
		{
			name: "empty ID",
			policy: models.TribunalPolicy{
				ID:              "",
				MemberAppIDs:    []string{"member-1"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "tribunal ID",
		},
		{
			name: "empty member IDs",
			policy: models.TribunalPolicy{
				ID:              "test-tribunal-2",
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
			policy: models.TribunalPolicy{
				ID:              "test-tribunal-3",
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
			policy: models.TribunalPolicy{
				ID:              "test-tribunal-4",
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
			policy: models.TribunalPolicy{
				ID:              "test-tribunal-5",
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
			policy: models.TribunalPolicy{
				ID:              "test-tribunal-6",
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
			policy: models.TribunalPolicy{
				ID:              "test-tribunal-7",
				MemberAppIDs:    []string{"member-1", ""},
				Quorum:          2,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "cannot contain empty strings",
		},
		{
			name: "invalid tribunal ID with special characters",
			policy: models.TribunalPolicy{
				ID:              "test@tribunal",
				MemberAppIDs:    []string{"member-1"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "TRIBUNAL_INVALID_ID",
		},
		{
			name: "invalid tribunal ID with spaces",
			policy: models.TribunalPolicy{
				ID:              "test tribunal",
				MemberAppIDs:    []string{"member-1"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "TRIBUNAL_INVALID_ID",
		},
		{
			name: "invalid tribunal ID with path separator",
			policy: models.TribunalPolicy{
				ID:              "test/tribunal",
				MemberAppIDs:    []string{"member-1"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         true,
			},
			expectError: true,
			errorMsg:    "TRIBUNAL_INVALID_ID",
		},
		{
			name: "disabled new tribunal rejected",
			policy: models.TribunalPolicy{
				ID:              "test-tribunal-disabled",
				MemberAppIDs:    []string{"member-1"},
				Quorum:          1,
				RequireDistinct: true,
				Enabled:         false,
			},
			expectError: true,
			errorMsg:    "TRIBUNAL_MUST_BE_ENABLED",
		},
		{
			name: "disabled signer as member rejected",
			policy: models.TribunalPolicy{
				ID:              "test-tribunal-disabled-signer",
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
			err := tribunalSvc.AddTribunal(tt.policy)
			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
				// Cleanup
				t.Cleanup(func() { tribunalSvc.DeleteTribunal(tt.policy.ID) })
			}
		})
	}
}

func TestTribunalStoreService_GetTribunal(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	tribunalSvc := NewTribunalStoreService(infra.DB.GetDB(), infra.Logger, infra.DB.SignerStore)

	// Create a test signer
	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer := models.TrustedSigner{
		ID:        "test-member",
		PublicKey: hex.EncodeToString(pubKey),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	err = infra.DB.SignerStore.AddTrustedSigner(signer)
	require.NoError(t, err)
	t.Cleanup(func() { infra.DB.SignerStore.DeleteTrustedSigner("test-member") })

	// Create a test tribunal
	policy := models.TribunalPolicy{
		ID:              "get-test-tribunal",
		MemberAppIDs:    []string{"test-member"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	err = tribunalSvc.AddTribunal(policy)
	require.NoError(t, err)
	t.Cleanup(func() { tribunalSvc.DeleteTribunal("get-test-tribunal") })

	// Get the tribunal
	retrieved, err := tribunalSvc.GetTribunal("get-test-tribunal")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, "get-test-tribunal", retrieved.ID)
	assert.Equal(t, []string{"test-member"}, retrieved.MemberAppIDs)
	assert.Equal(t, 1, retrieved.Quorum)
	assert.True(t, retrieved.RequireDistinct)
	assert.True(t, retrieved.Enabled)

	// Get non-existent tribunal
	retrieved, err = tribunalSvc.GetTribunal("non-existent")
	require.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestTribunalStoreService_ListTribunals(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	tribunalSvc := NewTribunalStoreService(infra.DB.GetDB(), infra.Logger, infra.DB.SignerStore)

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

	err = infra.DB.SignerStore.AddTrustedSigner(signer1)
	require.NoError(t, err)
	t.Cleanup(func() { infra.DB.SignerStore.DeleteTrustedSigner("list-member-1") })

	err = infra.DB.SignerStore.AddTrustedSigner(signer2)
	require.NoError(t, err)
	t.Cleanup(func() { infra.DB.SignerStore.DeleteTrustedSigner("list-member-2") })

	// Create test tribunals
	policy1 := models.TribunalPolicy{
		ID:              "list-tribunal-1",
		MemberAppIDs:    []string{"list-member-1"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	err = tribunalSvc.AddTribunal(policy1)
	require.NoError(t, err)
	t.Cleanup(func() { tribunalSvc.DeleteTribunal("list-tribunal-1") })

	policy2 := models.TribunalPolicy{
		ID:              "list-tribunal-2",
		MemberAppIDs:    []string{"list-member-2"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	err = tribunalSvc.AddTribunal(policy2)
	require.NoError(t, err)
	t.Cleanup(func() { tribunalSvc.DeleteTribunal("list-tribunal-2") })

	// List tribunals
	tribunals, err := tribunalSvc.ListTribunals()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(tribunals), 2)

	// Verify our tribunals are in the list
	ids := make(map[string]bool)
	for _, t := range tribunals {
		ids[t.ID] = true
	}
	assert.True(t, ids["list-tribunal-1"])
	assert.True(t, ids["list-tribunal-2"])
}

func TestTribunalStoreService_DeleteTribunal(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	tribunalSvc := NewTribunalStoreService(infra.DB.GetDB(), infra.Logger, infra.DB.SignerStore)

	// Create a test signer
	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer := models.TrustedSigner{
		ID:        "delete-member",
		PublicKey: hex.EncodeToString(pubKey),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	err = infra.DB.SignerStore.AddTrustedSigner(signer)
	require.NoError(t, err)
	t.Cleanup(func() { infra.DB.SignerStore.DeleteTrustedSigner("delete-member") })

	// Create a test tribunal
	policy := models.TribunalPolicy{
		ID:              "delete-test-tribunal",
		MemberAppIDs:    []string{"delete-member"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	err = tribunalSvc.AddTribunal(policy)
	require.NoError(t, err)

	// Delete the tribunal
	deleted, err := tribunalSvc.DeleteTribunal("delete-test-tribunal")
	require.NoError(t, err)
	assert.True(t, deleted)

	// Verify it's gone
	retrieved, err := tribunalSvc.GetTribunal("delete-test-tribunal")
	require.NoError(t, err)
	assert.Nil(t, retrieved)

	// Delete non-existent tribunal
	deleted, err = tribunalSvc.DeleteTribunal("non-existent")
	require.NoError(t, err)
	assert.False(t, deleted)
}

func TestTribunalStoreService_UpdateDisableTribunal(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	tribunalSvc := NewTribunalStoreService(infra.DB.GetDB(), infra.Logger, infra.DB.SignerStore)

	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer := models.TrustedSigner{
		ID:        "update-member",
		PublicKey: hex.EncodeToString(pubKey),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	err = infra.DB.SignerStore.AddTrustedSigner(signer)
	require.NoError(t, err)
	t.Cleanup(func() { infra.DB.SignerStore.DeleteTrustedSigner("update-member") })

	// Create enabled tribunal
	policy := models.TribunalPolicy{
		ID:              "update-tribunal",
		MemberAppIDs:    []string{"update-member"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	err = tribunalSvc.AddTribunal(policy)
	require.NoError(t, err)
	t.Cleanup(func() { tribunalSvc.DeleteTribunal("update-tribunal") })

	// Update: disable the existing tribunal (should succeed)
	policy.Enabled = false
	err = tribunalSvc.AddTribunal(policy)
	require.NoError(t, err)

	// Verify it's stored as disabled
	retrieved, err := tribunalSvc.GetTribunal("update-tribunal")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.False(t, retrieved.Enabled)
}

func TestTribunalStoreService_AddTribunal_AlreadyExists(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	tribunalSvc := NewTribunalStoreService(infra.DB.GetDB(), infra.Logger, infra.DB.SignerStore)

	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer := models.TrustedSigner{
		ID:        "exists-member",
		PublicKey: hex.EncodeToString(pubKey),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	err = infra.DB.SignerStore.AddTrustedSigner(signer)
	require.NoError(t, err)
	t.Cleanup(func() { infra.DB.SignerStore.DeleteTrustedSigner("exists-member") })

	policy := models.TribunalPolicy{
		ID:              "exists-tribunal",
		MemberAppIDs:    []string{"exists-member"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	err = tribunalSvc.AddTribunal(policy)
	require.NoError(t, err)
	t.Cleanup(func() { tribunalSvc.DeleteTribunal("exists-tribunal") })

	// Attempt to create the same tribunal again with Enabled=true
	err = tribunalSvc.AddTribunal(policy)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestIsValidTribunalID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"valid-id", true},
		{"valid_id", true},
		{"ValidID123", true},
		{"a", true},
		{"", false},
		{"test@tribunal", false},
		{"test tribunal", false},
		{"test/tribunal", false},
		{"test\\tribunal", false},
		{"test:tribunal", false},
		{"test.tribunal", false},
		{"test\ttribunal", false},
		{"日本語", true}, // unicode letters are allowed
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := isValidTribunalID(tt.id)
			assert.Equal(t, tt.want, got)
		})
	}
}
