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

package tribunal

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

// KeyProvider resolves Ed25519 private keys for tribunal members by AppID.
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

// NewTribunalFromPolicy constructs a TribunalService from a TribunalPolicy and
// a KeyProvider. It resolves each member's private key via the provider and
// builds the member list. Members whose keys cannot be resolved are included
// without a private key (they can participate in policy but cannot sign votes).
//
// This is the shared factory used by both production bootstrap (BootstrapTribunal
// in internal/cli/serve/gateway.go) and test fixtures (SetupTribunal in
// test/fixtures/gateway_fixture.go), eliminating the duplication identified in
// CS-12.
func NewTribunalFromPolicy(
	policy *models.TribunalPolicy,
	keyProvider KeyProvider,
	doctrine *govsvc.L1Doctrine,
	logger *slog.Logger,
	responder *response.Writer,
) (*TribunalService, error) {
	if policy == nil {
		return nil, constants.ErrTribunalFactoryNilPolicy
	}
	if keyProvider == nil {
		return nil, constants.ErrTribunalFactoryNilKeyProvider
	}

	members := make([]TribunalMember, 0, len(policy.MemberAppIDs))
	for _, appID := range policy.MemberAppIDs {
		privKey, err := keyProvider.GetMemberKey(appID)
		if err != nil {
			logger.Warn("Tribunal member key not available",
				"member_app_id", appID,
				"error", err)
		}
		members = append(members, TribunalMember{
			AppID:      appID,
			PrivateKey: privKey,
		})
	}

	return NewTribunalService(policy.ID, members, doctrine, logger, responder), nil
}

// FileKeyProvider loads Ed25519 private keys from disk-based files.
// Each member's key is stored as a hex-encoded Ed25519 seed in a file named
// {prefix}{tribunalID}_{memberAppID}.key within the secrets directory.
// This enables multi-member tribunal co-signing without sharing a single key.
type FileKeyProvider struct {
	secretsDir string
	tribunalID string
	keyPrefix  string
}

// NewFileKeyProvider creates a FileKeyProvider that looks for member keys in
// the given secrets directory using the standard naming convention.
func NewFileKeyProvider(secretsDir, tribunalID string) *FileKeyProvider {
	return &FileKeyProvider{
		secretsDir: secretsDir,
		tribunalID: tribunalID,
		keyPrefix:  constants.SecretsFileTribunalMemberKeyPrefix,
	}
}

// GetMemberKey loads the Ed25519 private key for the given member AppID from disk.
// Returns an error if the key file does not exist or contains invalid data.
func (p *FileKeyProvider) GetMemberKey(appID string) (ed25519.PrivateKey, error) {
	filename := fmt.Sprintf("%s%s_%s.key", p.keyPrefix, p.tribunalID, appID)
	keyPath := filepath.Join(p.secretsDir, filename)

	seedHex, err := os.ReadFile(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("tribunal file key provider: key file not found for member %s: %s", appID, keyPath)
		}
		return nil, fmt.Errorf("tribunal file key provider: read key for member %s: %w", appID, err)
	}

	seedHexStr := strings.TrimSpace(string(seedHex))
	seed, err := hex.DecodeString(seedHexStr)
	if err != nil {
		return nil, fmt.Errorf("tribunal file key provider: decode seed for member %s: %w", appID, err)
	}

	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("tribunal file key provider: %w for member %s: got %d, expected %d", constants.ErrInvalidSeedLength, appID, len(seed), ed25519.SeedSize)
	}

	return ed25519.NewKeyFromSeed(seed), nil
}

// SaveMemberKey writes an Ed25519 private key seed to disk for the given member.
// This is used during tribunal member provisioning to persist generated keys.
func SaveMemberKey(secretsDir, tribunalID, appID string, privKey ed25519.PrivateKey) error {
	if err := os.MkdirAll(secretsDir, constants.PermDirPrivate); err != nil {
		return fmt.Errorf("tribunal: save member key: create secrets dir: %w", err)
	}

	seed := privKey.Seed()
	seedHex := hex.EncodeToString(seed)

	filename := fmt.Sprintf("%s%s_%s.key", constants.SecretsFileTribunalMemberKeyPrefix, tribunalID, appID)
	keyPath := filepath.Join(secretsDir, filename)

	if err := os.WriteFile(keyPath, []byte(seedHex), constants.PermFilePrivate); err != nil {
		return fmt.Errorf("tribunal: save member key: write: %w", err)
	}

	return nil
}
