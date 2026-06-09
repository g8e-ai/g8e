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

package gateway

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/keystore"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
)

var requiredBootstrapSecrets = []string{
	"session_encryption_key",
	"actuator_signing_key",
	"actuator_key_id",
	"auditor_hmac_key",
	"consensus_signing_key",
}

// SecretManager handles generation and validation of platform security secrets.
type SecretManager struct {
	db         *sqliteutil.DB
	logger     *slog.Logger
	secretsDir string
	keystore   *keystore.Keystore
}

func NewSecretManager(db *sqliteutil.DB, secretsDir string, logger *slog.Logger) (*SecretManager, error) {
	ks, err := keystore.New(secretsDir, logger)
	if err != nil {
		return nil, fmt.Errorf("initialize keystore: %w", err)
	}
	if err := ks.Initialize(); err != nil {
		return nil, fmt.Errorf("initialize master key: %w", err)
	}
	if err := ks.EnsurePermissions(); err != nil {
		return nil, fmt.Errorf("enforce keystore permissions: %w", err)
	}
	return &SecretManager{
		db:         db,
		secretsDir: secretsDir,
		logger:     logger,
		keystore:   ks,
	}, nil
}

// GetKeystore returns the underlying Keystore instance.
func (m *SecretManager) GetKeystore() *keystore.Keystore {
	return m.keystore
}

// InitAppSettings creates secrets on first boot and validates them on later boots.
func (m *SecretManager) InitAppSettings() error {
	var exists bool
	err := m.db.QueryRowWithRetry(
		"SELECT EXISTS(SELECT 1 FROM documents WHERE collection = 'settings' AND id = 'platform_settings')",
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("secret_manager: init app settings: check platform_settings existence: %w", err)
	}

	now := time.Now().UTC()

	if !exists {
		return m.createAppSettings(now)
	}

	if err := m.cleanupStaleAppSettings(); err != nil {
		m.logger.Warn("[SecretManager] Failed to cleanup stale platform settings", string(constants.ConnectionStateError), err)
	}

	return m.validateAppSettings()
}

func (m *SecretManager) cleanupStaleAppSettings() error {
	var dataJSON string
	err := m.db.QueryRowWithRetry(
		"SELECT data FROM documents WHERE collection = 'settings' AND id = 'platform_settings'",
	).Scan(&dataJSON)
	if err != nil {
		return fmt.Errorf("secret_manager: cleanup stale app settings: query document: %w", err)
	}

	var doc models.SettingsDocument
	if err := json.Unmarshal([]byte(dataJSON), &doc); err != nil {
		return fmt.Errorf("secret_manager: cleanup stale app settings: unmarshal document: %w", err)
	}

	if doc.Settings == nil {
		return nil
	}

	changed := false
	if doc.Settings.PasskeyRPID != "" {
		doc.Settings.PasskeyRPID = ""
		changed = true
	}
	if doc.Settings.PasskeyOrigin != "" {
		doc.Settings.PasskeyOrigin = ""
		changed = true
	}

	if !changed {
		return nil
	}

	m.logger.Info("[SecretManager] Cleaning up stale fields from platform_settings document", "fields", []string{"passkey_rp_id", "passkey_origin"})

	newData, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("secret_manager: cleanup stale app settings: marshal document: %w", err)
	}

	_, err = m.db.ExecWithRetry(
		"UPDATE documents SET data = ?, updated_at = ? WHERE collection = 'settings' AND id = 'platform_settings'",
		string(newData), sqliteutil.NowTimestamp(),
	)
	if err != nil {
		return fmt.Errorf("secret_manager: cleanup stale app settings: update document: %w", err)
	}
	return nil
}

func (m *SecretManager) recreateAppSettings() error {
	m.logger.Info("[SecretManager] Recreating app settings due to corrupted state")

	// Delete existing platform_settings document from database
	_, err := m.db.ExecWithRetry(
		"DELETE FROM documents WHERE collection = 'settings' AND id = 'platform_settings'",
	)
	if err != nil {
		return fmt.Errorf("secret_manager: recreate app settings: delete platform_settings: %w", err)
	}

	// Delete existing secret files
	for _, name := range requiredBootstrapSecrets {
		filePath := filepath.Join(m.secretsDir, name)
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			m.logger.Warn("[SecretManager] Failed to delete secret file during recreation",
				"path", filePath, string(constants.ConnectionStateError), err)
		}
	}

	// Delete bootstrap digest manifest if it exists
	manifestPath := filepath.Join(m.secretsDir, BootstrapDigestManifestFile)
	if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
		m.logger.Warn("[SecretManager] Failed to delete digest manifest during recreation",
			"path", manifestPath, string(constants.ConnectionStateError), err)
	}

	// Recreate from scratch
	return m.createAppSettings(time.Now().UTC())
}

func (m *SecretManager) createAppSettings(now time.Time) error {
	if err := m.rejectPreexistingBootstrapState(); err != nil {
		return fmt.Errorf("secret_manager: create app settings: reject preexisting state: %w", err)
	}
	if err := os.MkdirAll(m.secretsDir, 0700); err != nil {
		return fmt.Errorf("secret_manager: create app settings: create directory %s: %w", m.secretsDir, err)
	}

	// Generate Actuator signing key and compute its KeyID once
	ActuatorSeedBytes, err := m.generateSecureTokenBytes(ed25519.SeedSize)
	if err != nil {
		return fmt.Errorf("secret_manager: create app settings: generate actuator seed: %w", err)
	}
	ActuatorSeed := hex.EncodeToString(ActuatorSeedBytes)
	ActuatorPriv := ed25519.NewKeyFromSeed(ActuatorSeedBytes)
	ActuatorPub := ActuatorPriv.Public().(ed25519.PublicKey)
	ActuatorKeyID := hex.EncodeToString(ActuatorPub)

	// Generate consensus signing key for L2 consensus
	ConsensusSeedBytes, err := m.generateSecureTokenBytes(ed25519.SeedSize)
	if err != nil {
		return fmt.Errorf("secret_manager: create app settings: generate consensus seed: %w", err)
	}
	ConsensusSeed := hex.EncodeToString(ConsensusSeedBytes)

	sessionEncryptionKey, err := m.generateSecureToken(32)
	if err != nil {
		return fmt.Errorf("secret_manager: create app settings: generate session encryption key: %w", err)
	}
	auditorHMACKey, err := m.generateSecureToken(32)
	if err != nil {
		return fmt.Errorf("secret_manager: create app settings: generate auditor HMAC key: %w", err)
	}

	secrets := map[string]string{
		"session_encryption_key": sessionEncryptionKey,
		"actuator_signing_key":   ActuatorSeed, // Seed for ED25519
		"actuator_key_id":        ActuatorKeyID,
		"auditor_hmac_key":       auditorHMACKey,
		"consensus_signing_key":  ConsensusSeed, // Seed for ED25519
	}

	platformSettings := models.SettingsDocument{
		Settings: &models.PlatformSettings{
			ActuatorKeyID: secrets["actuator_key_id"],
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	dataJSON, err := json.Marshal(platformSettings)
	if err != nil {
		return fmt.Errorf("secret_manager: create app settings: marshal platform_settings: %w", err)
	}

	nowStr := sqliteutil.FormatTimestamp(now)
	_, err = m.db.ExecWithRetry(
		`INSERT INTO documents (collection, id, data, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"settings", "platform_settings", string(dataJSON), nowStr, nowStr,
	)
	if err != nil {
		return fmt.Errorf("secret_manager: create app settings: insert platform_settings document: %w", err)
	}
	m.logger.Info("[SecretManager] platform_settings document created with security secrets")

	m.warmAppSettingsCache(string(dataJSON), now)

	for _, name := range requiredBootstrapSecrets {
		if err := m.keystore.EncryptSecret(name, secrets[name]); err != nil {
			return fmt.Errorf("secret_manager: create app settings: encrypt secret %s: %w", name, err)
		}
	}

	if err := m.writeDigestManifestFromEncryptedFiles(now); err != nil {
		return fmt.Errorf("secret_manager: create app settings: write digest manifest: %w", err)
	}

	return m.validateAppSettings()
}

func (m *SecretManager) validateAppSettings() error {
	if info, err := os.Stat(m.secretsDir); err != nil {
		return fmt.Errorf("secret_manager: validate app settings: secrets directory required: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("secret_manager: validate app settings: secrets path is not a directory")
	}

	manifest, err := m.readDigestManifest()
	if err != nil {
		// If bootstrap digest manifest is missing, treat this as corrupted state
		// and recreate secrets (e.g., when .g8e directory was wiped but DB persists)
		if errors.Is(err, os.ErrNotExist) {
			m.logger.Warn("[SecretManager] Bootstrap digest manifest missing, recreating secrets",
				"path", filepath.Join(m.secretsDir, BootstrapDigestManifestFile))
			return m.recreateAppSettings()
		}
		return fmt.Errorf("secret_manager: validate app settings: read digest manifest: %w", err)
	}

	for _, name := range requiredBootstrapSecrets {
		// Verify encrypted file digest matches manifest (what g8e-compatible agentic ensembles will check)
		entry, ok := manifest.Secrets[name]
		if !ok || entry.SHA256 == "" {
			return fmt.Errorf("secret_manager: validate app settings: manifest missing entry %s", name)
		}
		filePath := filepath.Join(m.secretsDir, name)
		encryptedData, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("secret_manager: validate app settings: read encrypted secret file %s: %w", filePath, err)
		}
		encryptedDigest := sha256.Sum256(encryptedData)
		if actual := hex.EncodeToString(encryptedDigest[:]); actual != entry.SHA256 {
			return fmt.Errorf("secret_manager: validate app settings: secret %s digest mismatch", name)
		}
	}

	return nil
}

// BootstrapDigestManifestFile is the filename of the tamper-evidence manifest
// written alongside bootstrap secrets.
const BootstrapDigestManifestFile = "bootstrap_digest.json"

// bootstrapDigestManifest is the on-disk schema for bootstrap_digest.json.
// Consumers on startup compute SHA-256 of each secret they load from the
// volume and compare to the digest recorded here. Divergence means the
// volume file has drifted from the DB-authoritative value SecretManager
// wrote, which must abort startup rather than authenticate with a silently
// incorrect secret.
type bootstrapDigestManifest struct {
	Version   int                           `json:"version"`
	UpdatedAt string                        `json:"updated_at"`
	Secrets   map[string]bootstrapDigestRef `json:"secrets"`
}

type bootstrapDigestRef struct {
	SHA256 string `json:"sha256"`
}

// writeDigestManifestFromEncryptedFiles writes the bootstrap digest manifest by reading
// the actual encrypted files on disk and computing their SHA-256 digests. This ensures
// the manifest matches what consumers (g8e-compatible agentic ensembles) will verify: they read the encrypted file
// content and hash it, so the manifest must contain digests of the encrypted content,
// not the plaintext secrets.
func (m *SecretManager) writeDigestManifestFromEncryptedFiles(now time.Time) error {
	manifest := bootstrapDigestManifest{
		Version:   1,
		UpdatedAt: now.UTC().Format(time.RFC3339Nano),
		Secrets:   make(map[string]bootstrapDigestRef, len(requiredBootstrapSecrets)),
	}

	for _, name := range requiredBootstrapSecrets {
		filePath := filepath.Join(m.secretsDir, name)
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("secret_manager: write digest manifest: read encrypted secret file %s: %w", filePath, err)
		}
		sum := sha256.Sum256(data)
		manifest.Secrets[name] = bootstrapDigestRef{SHA256: hex.EncodeToString(sum[:])}
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("secret_manager: write digest manifest: marshal manifest: %w", err)
	}

	finalPath := filepath.Join(m.secretsDir, BootstrapDigestManifestFile)
	tmpPath := finalPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		m.logger.Error("[SecretManager] Failed to write bootstrap digest manifest",
			"path", tmpPath, string(constants.ConnectionStateError), err)
		return fmt.Errorf("secret_manager: write digest manifest: write file %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		m.logger.Error("[SecretManager] Failed to rename bootstrap digest manifest",
			"from", tmpPath, "to", finalPath, string(constants.ConnectionStateError), err)
		return fmt.Errorf("secret_manager: write digest manifest: rename to %s: %w", finalPath, err)
	}
	m.logger.Info("[SecretManager] Bootstrap digest manifest written from encrypted files",
		"path", finalPath, "secrets", len(manifest.Secrets))
	return nil
}

func (m *SecretManager) readDigestManifest() (*bootstrapDigestManifest, error) {
	manifestPath := filepath.Join(m.secretsDir, BootstrapDigestManifestFile)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("secret_manager: read digest manifest: bootstrap digest manifest file %s is required but does not exist: %w", manifestPath, err)
		}
		return nil, fmt.Errorf("secret_manager: read digest manifest: read file %s: %w", manifestPath, err)
	}

	var manifest bootstrapDigestManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("secret_manager: read digest manifest: unmarshal %s: %w", manifestPath, err)
	}
	if manifest.Version != 1 {
		return nil, fmt.Errorf("secret_manager: read digest manifest: unsupported version %d", manifest.Version)
	}
	if manifest.Secrets == nil {
		return nil, fmt.Errorf("secret_manager: read digest manifest: missing secrets map")
	}
	return &manifest, nil
}

func (m *SecretManager) rejectPreexistingBootstrapState() error {
	for _, name := range requiredBootstrapSecrets {
		if _, err := os.Stat(filepath.Join(m.secretsDir, name)); err == nil {
			return fmt.Errorf("secret_manager: reject preexisting bootstrap state: found preexisting secret %s", name)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("secret_manager: reject preexisting bootstrap state: inspect secret %s: %w", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(m.secretsDir, BootstrapDigestManifestFile)); err == nil {
		return fmt.Errorf("secret_manager: reject preexisting bootstrap state: found preexisting digest manifest")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("secret_manager: reject preexisting bootstrap state: inspect digest manifest: %w", err)
	}
	return nil
}

func (m *SecretManager) warmAppSettingsCache(dataJSON string, now time.Time) {
	cacheKey := "g8e:cache:doc:settings:platform_settings"
	cacheTTL := 3600
	nowStr := sqliteutil.FormatTimestamp(now)
	_, err := m.db.ExecWithRetry(
		`INSERT INTO kv_store (key, value, created_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		cacheKey, dataJSON, nowStr, sqliteutil.FormatTimestamp(now.Add(time.Duration(cacheTTL)*time.Second)),
	)
	if err != nil {
		m.logger.Warn("[SecretManager] Failed to warm cache for platform_settings", string(constants.ConnectionStateError), err)
	} else {
		m.logger.Info("[SecretManager] platform_settings cache warmed", "key", cacheKey, "ttl", cacheTTL)
	}
}

func (m *SecretManager) generateSecureToken(bytes int) (string, error) {
	tokenBytes, err := m.generateSecureTokenBytes(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(tokenBytes), nil
}

func (m *SecretManager) generateSecureTokenBytes(bytes int) ([]byte, error) {
	if bytes <= 0 {
		return nil, fmt.Errorf("secure token byte length must be positive")
	}
	tokenBytes := make([]byte, bytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate secure random token: %w", err)
	}
	return tokenBytes, nil
}

// GetActuatorKey retrieves the Actuator's ED25519 signing key and its KeyID.
// The signing key is decrypted from the keystore; the KeyID is read from platform_settings.
func (m *SecretManager) GetActuatorKey() (ed25519.PrivateKey, string, error) {
	seedHex, err := m.keystore.DecryptSecret("actuator_signing_key")
	if err != nil {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: decrypt secret: %w", err)
	}

	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: decode seed: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: invalid seed length %d; expected %d", len(seed), ed25519.SeedSize)
	}

	priv := ed25519.NewKeyFromSeed(seed)

	var dataJSON string
	if err := m.db.QueryRowWithRetry(
		"SELECT data FROM documents WHERE collection = 'settings' AND id = 'platform_settings'",
	).Scan(&dataJSON); err != nil {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: query platform_settings: %w", err)
	}

	var settings models.SettingsDocument
	if err := json.Unmarshal([]byte(dataJSON), &settings); err != nil {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: unmarshal platform_settings: %w", err)
	}
	if settings.Settings == nil {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: platform_settings missing settings; delete and recreate runtime state")
	}

	keyID := strings.TrimSpace(settings.Settings.ActuatorKeyID)
	if keyID == "" {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: platform_settings missing actuator_key_id; delete and recreate runtime state")
	}

	expectedKeyID := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	if keyID != expectedKeyID {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: key_id mismatch; delete and recreate runtime state")
	}

	return priv, keyID, nil
}

// GetSessionEncryptionKey retrieves the session encryption key from the keystore.
func (m *SecretManager) GetSessionEncryptionKey() (string, error) {
	return m.keystore.DecryptSecret("session_encryption_key")
}

// GetAuditorHMACKey retrieves the auditor HMAC key from the keystore.
func (m *SecretManager) GetAuditorHMACKey() (string, error) {
	return m.keystore.DecryptSecret("auditor_hmac_key")
}

// StoreCAPrivateKey stores a CA private key in the keystore.
// caType should be "root", "hub", "operator", or "bootstrap".
func (m *SecretManager) StoreCAPrivateKey(caType string, keyDER []byte) error {
	name := fmt.Sprintf("ca_%s_key", caType)
	plaintext := hex.EncodeToString(keyDER)
	return m.keystore.EncryptSecret(name, plaintext)
}

// GetCAPrivateKey retrieves a CA private key from the keystore.
// caType should be "root", "hub", "operator", or "bootstrap".
func (m *SecretManager) GetCAPrivateKey(caType string) ([]byte, error) {
	name := fmt.Sprintf("ca_%s_key", caType)
	plaintext, err := m.keystore.DecryptSecret(name)
	if err != nil {
		return nil, err
	}
	return hex.DecodeString(plaintext)
}

// StoreConsensusKey stores a consensus signing key in the keystore.
func (m *SecretManager) StoreConsensusKey(seedHex string) error {
	return m.keystore.EncryptSecret("consensus_signing_key", seedHex)
}

// GetConsensusKey retrieves the consensus ED25519 signing key from the keystore.
func (m *SecretManager) GetConsensusKey() (ed25519.PrivateKey, error) {
	seedHex, err := m.keystore.DecryptSecret("consensus_signing_key")
	if err != nil {
		return nil, fmt.Errorf("secret_manager: get consensus key: decrypt secret: %w", err)
	}

	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		return nil, fmt.Errorf("secret_manager: get consensus key: decode seed: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("secret_manager: get consensus key: invalid seed length %d; expected %d", len(seed), ed25519.SeedSize)
	}

	return ed25519.NewKeyFromSeed(seed), nil
}

// StoreNotaryKey stores an L3 Notary signing key in the keystore.
func (m *SecretManager) StoreNotaryKey(seedHex string) error {
	return m.keystore.EncryptSecret("notary_signing_key", seedHex)
}

// GetNotaryKey retrieves an L3 Notary signing key from the keystore.
func (m *SecretManager) GetNotaryKey() (string, error) {
	return m.keystore.DecryptSecret("notary_signing_key")
}

// StoreServicePrivateKey stores a service or app certificate private key in the keystore.
// name should be the service/app identifier (e.g., "operator-gateway", "g8e-compatible-ensemble").
func (m *SecretManager) StoreServicePrivateKey(name string, keyDER []byte) error {
	// Use SHA-256 hash for keystore key names to guarantee filesystem safety
	// and avoid issues with special characters in SPIFFE IDs.
	hash := sha256.Sum256([]byte(name))
	keystoreName := fmt.Sprintf("service_%x_key", hash)
	plaintext := hex.EncodeToString(keyDER)
	return m.keystore.EncryptSecret(keystoreName, plaintext)
}

// GetServicePrivateKey retrieves a service or app certificate private key from the keystore.
// name should be the service/app identifier (e.g., "operator-gateway", "agent").
func (m *SecretManager) GetServicePrivateKey(name string) ([]byte, error) {
	hash := sha256.Sum256([]byte(name))
	keystoreName := fmt.Sprintf("service_%x_key", hash)
	plaintext, err := m.keystore.DecryptSecret(keystoreName)
	if err != nil {
		return nil, err
	}
	return hex.DecodeString(plaintext)
}

// DeleteServicePrivateKey deletes a service or app certificate private key from the keystore.
// name should be the service/app identifier (e.g., "operator-gateway", "agent").
func (m *SecretManager) DeleteServicePrivateKey(name string) error {
	hash := sha256.Sum256([]byte(name))
	keystoreName := fmt.Sprintf("service_%x_key", hash)
	return m.keystore.DeleteSecret(keystoreName)
}

// StoreAPIKey stores an API key for external service integration in the keystore.
// service identifies the external service (e.g., "openai", "anthropic").
func (m *SecretManager) StoreAPIKey(service string, apiKey string) error {
	keystoreName := fmt.Sprintf("api_key_%s", service)
	return m.keystore.EncryptSecret(keystoreName, apiKey)
}

// GetAPIKey retrieves an API key for external service integration from the keystore.
// service identifies the external service (e.g., "openai", "anthropic").
func (m *SecretManager) GetAPIKey(service string) (string, error) {
	keystoreName := fmt.Sprintf("api_key_%s", service)
	return m.keystore.DecryptSecret(keystoreName)
}

// StoreOperatorPrivateKey stores an Operator ed25519 private key in the keystore.
// This is used for BYO-client enrollment where the Operator has its own identity.
func (m *SecretManager) StoreOperatorPrivateKey(key ed25519.PrivateKey) error {
	seedHex := hex.EncodeToString(key.Seed())
	return m.keystore.EncryptSecret("operator_private_key", seedHex)
}

// GetOperatorPrivateKey retrieves an Operator ed25519 private key from the keystore.
func (m *SecretManager) GetOperatorPrivateKey() (ed25519.PrivateKey, error) {
	seedHex, err := m.keystore.DecryptSecret("operator_private_key")
	if err != nil {
		return nil, fmt.Errorf("secret_manager: get operator private key: decrypt secret: %w", err)
	}
	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		return nil, fmt.Errorf("secret_manager: get operator private key: decode seed: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("secret_manager: get operator private key: invalid seed length %d; expected %d", len(seed), ed25519.SeedSize)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// StoreCLIPrivateKey stores a CLI ed25519 private key in the keystore.
// This is used for BYO-client enrollment where the CLI has its own identity.
func (m *SecretManager) StoreCLIPrivateKey(key ed25519.PrivateKey) error {
	seedHex := hex.EncodeToString(key.Seed())
	return m.keystore.EncryptSecret("cli_private_key", seedHex)
}

// GetCLIPrivateKey retrieves a CLI ed25519 private key from the keystore.
func (m *SecretManager) GetCLIPrivateKey() (ed25519.PrivateKey, error) {
	seedHex, err := m.keystore.DecryptSecret("cli_private_key")
	if err != nil {
		return nil, fmt.Errorf("secret_manager: get CLI private key: decrypt secret: %w", err)
	}
	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		return nil, fmt.Errorf("secret_manager: get CLI private key: decode seed: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("secret_manager: get CLI private key: invalid seed length %d; expected %d", len(seed), ed25519.SeedSize)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// StoreSessionToken stores a session token with TTL in the keystore.
// The token is stored with a timestamp for TTL validation.
func (m *SecretManager) StoreSessionToken(token string, ttl time.Duration) error {
	expiresAt := time.Now().UTC().Add(ttl).Format(time.RFC3339Nano)
	tokenData := fmt.Sprintf("%s|%s", token, expiresAt)
	return m.keystore.EncryptSecret("session_token", tokenData)
}

// GetSessionToken retrieves a session token from the keystore.
// Returns error if the token has expired.
func (m *SecretManager) GetSessionToken() (string, error) {
	tokenData, err := m.keystore.DecryptSecret("session_token")
	if err != nil {
		return "", fmt.Errorf("secret_manager: get session token: decrypt secret: %w", err)
	}

	parts := strings.Split(tokenData, "|")
	if len(parts) != 2 {
		return "", fmt.Errorf("secret_manager: get session token: invalid format")
	}

	token := parts[0]
	expiresAtStr := parts[1]
	expiresAt, err := time.Parse(time.RFC3339Nano, expiresAtStr)
	if err != nil {
		return "", fmt.Errorf("secret_manager: get session token: parse expiry: %w", err)
	}

	if time.Now().UTC().After(expiresAt) {
		return "", fmt.Errorf("%w: token expired at %s", constants.ErrExpired, expiresAtStr)
	}

	return token, nil
}
