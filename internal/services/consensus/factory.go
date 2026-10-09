// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package consensus

import (
	"crypto/ed25519"
	"fmt"
	"log/slog"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	govsvc "github.com/g8e-ai/g8e/v2/internal/services/governance"
)

// KeyProvider resolves Ed25519 private keys for consensus members by AppID.
// Implementations may load keys from disk, use an in-process actuator key,
// or source them from any secure backing store.
type KeyProvider interface {
	GetMemberKey(appID string) (ed25519.PrivateKey, error)
}

// KeyProviderFunc is a function adapter for KeyProvider.
type KeyProviderFunc func(appID string) (ed25519.PrivateKey, error)

func (f KeyProviderFunc) GetMemberKey(appID string) (ed25519.PrivateKey, error) {
	return f(appID)
}

// NewConsensusFromPolicy constructs a ConsensusService from a ConsensusPolicy and
// a KeyProvider. It resolves each member's private key via the provider and
// builds the member list. Members whose keys cannot be resolved are included
// without a private key (they can participate in policy but cannot sign votes).
//
// This is the shared factory used by both production bootstrap (ConsensusBootstrap
// in internal/cli/serve/gateway.go) and test fixtures (SetupConsensus in
// test/fixtures/gateway_fixture.go), eliminating the duplication identified in
// CS-12.
func NewConsensusFromPolicy(
	policy *models.ConsensusPolicy,
	keyProvider KeyProvider,
	doctrine *govsvc.L1Doctrine,
	logger *slog.Logger,
	responder *response.Writer,
) (*ConsensusService, error) {
	if policy == nil {
		return nil, constants.ErrConsensusFactoryNilPolicy
	}
	if keyProvider == nil {
		return nil, constants.ErrConsensusFactoryNilKeyProvider
	}

	members := make([]ConsensusMember, 0, len(policy.MemberAppIDs))
	for _, appID := range policy.MemberAppIDs {
		privKey, err := keyProvider.GetMemberKey(appID)
		if err != nil {
			logger.Warn("Consensus member key not available",
				"member_app_id", appID,
				"error", err)
		}
		members = append(members, ConsensusMember{
			AppID:      appID,
			PrivateKey: privKey,
		})
	}

	return NewConsensusService(policy.ID, members, doctrine, logger, responder), nil
}

// KeystoreKeyProvider loads per-member Ed25519 signing keys from runtime
// secrets. Each member's seed is encrypted under the keystore master key in a
// secret named {prefix}{consensusID}_{memberAppID}.key, so multi-member
// co-signing never shares a key and no seed is stored in the clear.
type KeystoreKeyProvider struct {
	ks          *keystore.Keystore
	consensusID string
}

// NewKeystoreKeyProvider creates a KeystoreKeyProvider for consensusID.
func NewKeystoreKeyProvider(ks *keystore.Keystore, consensusID string) (*KeystoreKeyProvider, error) {
	if ks == nil || consensusID == "" {
		return nil, fmt.Errorf("consensus key provider: %w", constants.ErrMissingRequiredField)
	}
	return &KeystoreKeyProvider{ks: ks, consensusID: consensusID}, nil
}

func memberKeySecretName(consensusID, appID string) string {
	return fmt.Sprintf("%s%s_%s.key", constants.SecretsFileConsensusMemberKeyPrefix, consensusID, appID)
}

// GetMemberKey decrypts the Ed25519 private key for the given member AppID.
// A member with no stored key returns an error wrapping constants.ErrNotFound.
func (p *KeystoreKeyProvider) GetMemberKey(appID string) (ed25519.PrivateKey, error) {
	if p == nil || appID == "" {
		return nil, fmt.Errorf("consensus key provider: %w", constants.ErrMissingRequiredField)
	}
	seed, err := p.ks.LoadKeyMaterial(memberKeySecretName(p.consensusID, appID), ed25519.SeedSize)
	if err != nil {
		return nil, fmt.Errorf("consensus key provider: load key for member %s: %w", appID, err)
	}
	defer vault.SecureZero(seed)
	return ed25519.NewKeyFromSeed(seed), nil
}

// SaveMemberKey encrypts a member's Ed25519 seed under the keystore master key.
func SaveMemberKey(ks *keystore.Keystore, consensusID, appID string, privKey ed25519.PrivateKey) error {
	if ks == nil || consensusID == "" || appID == "" || len(privKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("consensus: save member key: %w", constants.ErrMissingRequiredField)
	}
	seed := privKey.Seed()
	defer vault.SecureZero(seed)
	if err := ks.StoreKeyMaterial(memberKeySecretName(consensusID, appID), seed); err != nil {
		return fmt.Errorf("consensus: save member key: %w", err)
	}
	return nil
}
