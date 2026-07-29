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

package consensus

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	govsvc "github.com/g8e-ai/g8e/internal/services/governance"
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

// FileKeyProvider loads Ed25519 private keys from disk-based files.
// Each member's key is stored as a hex-encoded Ed25519 seed in a file named
// {prefix}{consensusID}_{memberAppID}.key within the secrets directory.
// This enables multi-member consensus co-signing without sharing a single key.
type FileKeyProvider struct {
	secretsDir  string
	consensusID string
	keyPrefix   string
}

// NewFileKeyProvider creates a FileKeyProvider that looks for member keys in
// the given secrets directory using the standard naming convention.
func NewFileKeyProvider(secretsDir, consensusID string) *FileKeyProvider {
	return &FileKeyProvider{
		secretsDir:  secretsDir,
		consensusID: consensusID,
		keyPrefix:   constants.SecretsFileConsensusMemberKeyPrefix,
	}
}

// GetMemberKey loads the Ed25519 private key for the given member AppID from disk.
// Returns an error if the key file does not exist or contains invalid data.
func (p *FileKeyProvider) GetMemberKey(appID string) (ed25519.PrivateKey, error) {
	filename := fmt.Sprintf("%s%s_%s.key", p.keyPrefix, p.consensusID, appID)
	keyPath := filepath.Join(p.secretsDir, filename)

	seedHex, err := os.ReadFile(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("consensus file key provider: key file not found for member %s: %s", appID, keyPath)
		}
		return nil, fmt.Errorf("consensus file key provider: read key for member %s: %w", appID, err)
	}

	seedHexStr := strings.TrimSpace(string(seedHex))
	seed, err := hex.DecodeString(seedHexStr)
	if err != nil {
		return nil, fmt.Errorf("consensus file key provider: decode seed for member %s: %w", appID, err)
	}

	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("consensus file key provider: %w for member %s: got %d, expected %d", constants.ErrInvalidSeedLength, appID, len(seed), ed25519.SeedSize)
	}

	return ed25519.NewKeyFromSeed(seed), nil
}

// SaveMemberKey writes an Ed25519 private key seed to disk for the given member.
// This is used during consensus member provisioning to persist generated keys.
func SaveMemberKey(secretsDir, consensusID, appID string, privKey ed25519.PrivateKey) error {
	if err := os.MkdirAll(secretsDir, constants.PermDirPrivate); err != nil {
		return fmt.Errorf("consensus: save member key: create secrets dir: %w", err)
	}

	seed := privKey.Seed()
	seedHex := hex.EncodeToString(seed)

	filename := fmt.Sprintf("%s%s_%s.key", constants.SecretsFileConsensusMemberKeyPrefix, consensusID, appID)
	keyPath := filepath.Join(secretsDir, filename)

	if err := os.WriteFile(keyPath, []byte(seedHex), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("consensus: save member key: write: %w", err)
	}

	return nil
}
