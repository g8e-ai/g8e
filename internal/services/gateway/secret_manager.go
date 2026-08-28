// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
)

var requiredBootstrapSecrets = []string{
	constants.SecretsFileSessionEncryptionKey,
	constants.SecretsFileActuatorSigningKey,
	constants.SecretsFileActuatorKeyID,
	constants.SecretsFileAuditorSigningKey,
	constants.SecretsFileAuditorKeyID,
	constants.SecretsFileAuditorHMACKey,
}

// SecretManager handles generation and validation of platform security secrets.
type SecretManager struct {
	db       *sqliteutil.DB
	logger   *slog.Logger
	fileSvc  fs.RuntimeFileService
	keystore *keystore.Keystore
}

func NewSecretManager(db *sqliteutil.DB, fileSvc fs.RuntimeFileService, logger *slog.Logger) (*SecretManager, error) {
	ks, err := keystore.NewWithFS(fileSvc, logger)
	if err != nil {
		return nil, fmt.Errorf("secret_manager: init: keystore: %w", err)
	}
	if err := ks.Initialize(); err != nil {
		return nil, fmt.Errorf("secret_manager: init: master key: %w", err)
	}
	if err := ks.EnforcePermissions(); err != nil {
		return nil, fmt.Errorf("secret_manager: init: permissions: %w", err)
	}
	return &SecretManager{
		db:       db,
		logger:   logger,
		fileSvc:  fileSvc,
		keystore: ks,
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
		return fmt.Errorf("secret_manager: init app settings: %w", err)
	}

	now := time.Now().UTC()

	if !exists {
		return m.createAppSettings(now)
	}

	if err := m.cleanupStaleAppSettings(); err != nil {
		m.logger.Warn("[SecretManager] Failed to cleanup stale platform settings", "error", err)
	}
	if err := m.migrateAuditorIdentity(); err != nil {
		return fmt.Errorf("secret_manager: init app settings: migrate auditor identity: %w", err)
	}

	return m.validateAppSettings()
}

func (m *SecretManager) cleanupStaleAppSettings() error {
	var dataJSON string
	err := m.db.QueryRowWithRetry(
		"SELECT data FROM documents WHERE collection = 'settings' AND id = 'platform_settings'",
	).Scan(&dataJSON)
	if err != nil {
		return fmt.Errorf("secret_manager: cleanup stale app settings: %w", err)
	}

	var doc models.SettingsDocument
	if err := json.Unmarshal([]byte(dataJSON), &doc); err != nil {
		return fmt.Errorf("secret_manager: cleanup stale app settings: unmarshal: %w", err)
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
		return fmt.Errorf("secret_manager: cleanup stale app settings: marshal: %w", err)
	}

	_, err = m.db.ExecWithRetry(
		"UPDATE documents SET data = ?, updated_at = ? WHERE collection = 'settings' AND id = 'platform_settings'",
		string(newData), timesvc.NowTimestamp(),
	)
	if err != nil {
		return fmt.Errorf("secret_manager: cleanup stale app settings: update: %w", err)
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
		return fmt.Errorf("secret_manager: recreate app settings: delete: %w", err)
	}

	// Delete existing secret files
	for _, name := range requiredBootstrapSecrets {
		relPath := filepath.Join(constants.SecretsDirname, name)
		if err := m.fileSvc.Remove(context.Background(), relPath); err != nil {
			m.logger.Warn("[SecretManager] Failed to delete secret file during recreation",
				"name", name, "error", err)
		}
	}

	// Delete bootstrap digest manifest if it exists
	manifestRelPath := filepath.Join(constants.SecretsDirname, constants.SecretsFileBootstrapDigest)
	if err := m.fileSvc.Remove(context.Background(), manifestRelPath); err != nil {
		m.logger.Warn("[SecretManager] Failed to delete digest manifest during recreation",
			"error", err)
	}

	// Recreate from scratch
	return m.createAppSettings(time.Now().UTC())
}

func (m *SecretManager) migrateAuditorIdentity() error {
	manifest, err := m.readDigestManifest()
	if err != nil {
		return err
	}
	auditorSecrets := []string{constants.SecretsFileAuditorSigningKey, constants.SecretsFileAuditorKeyID}
	_, hasSigningKey := manifest.Secrets[constants.SecretsFileAuditorSigningKey]
	_, hasKeyID := manifest.Secrets[constants.SecretsFileAuditorKeyID]
	if hasSigningKey && hasKeyID {
		_, _, err := m.GetAuditorKey()
		return err
	}
	if hasSigningKey || hasKeyID {
		return constants.ErrValidationFailed
	}
	for _, name := range requiredBootstrapSecrets {
		if name == constants.SecretsFileAuditorSigningKey || name == constants.SecretsFileAuditorKeyID {
			continue
		}
		if err := m.validateManifestSecret(manifest, name); err != nil {
			return err
		}
	}
	existing := 0
	for _, name := range auditorSecrets {
		exists, err := m.fileSvc.FileExists(context.Background(), filepath.Join(constants.SecretsDirname, name))
		if err != nil {
			return fmt.Errorf("check %s: %w", name, err)
		}
		if exists {
			existing++
		}
	}
	if existing == 1 {
		return constants.ErrValidationFailed
	}
	if existing == 0 {
		seed, err := m.generateSecureTokenBytes(ed25519.SeedSize)
		if err != nil {
			return err
		}
		priv := ed25519.NewKeyFromSeed(seed)
		secrets := map[string]string{
			constants.SecretsFileAuditorSigningKey: hex.EncodeToString(seed),
			constants.SecretsFileAuditorKeyID:      hex.EncodeToString(priv.Public().(ed25519.PublicKey)),
		}
		for _, name := range auditorSecrets {
			if err := m.keystore.EncryptSecret(name, secrets[name]); err != nil {
				return fmt.Errorf("encrypt %s: %w", name, err)
			}
		}
	}
	if _, _, err := m.GetAuditorKey(); err != nil {
		return err
	}
	return m.writeDigestManifestFromEncryptedFiles(time.Now().UTC())
}

func (m *SecretManager) createAppSettings(now time.Time) error {
	if err := m.rejectPreexistingBootstrapState(); err != nil {
		return fmt.Errorf("secret_manager: create app settings: %w", err)
	}
	// Generate Actuator signing key and compute its KeyID once
	ActuatorSeedBytes, err := m.generateSecureTokenBytes(ed25519.SeedSize)
	if err != nil {
		return fmt.Errorf("secret_manager: create app settings: %w", err)
	}
	ActuatorSeed := hex.EncodeToString(ActuatorSeedBytes)
	ActuatorPriv := ed25519.NewKeyFromSeed(ActuatorSeedBytes)
	ActuatorPub := ActuatorPriv.Public().(ed25519.PublicKey)
	ActuatorKeyID := hex.EncodeToString(ActuatorPub)
	auditorSeedBytes, err := m.generateSecureTokenBytes(ed25519.SeedSize)
	if err != nil {
		return fmt.Errorf("secret_manager: create app settings: %w", err)
	}
	auditorSeed := hex.EncodeToString(auditorSeedBytes)
	auditorPriv := ed25519.NewKeyFromSeed(auditorSeedBytes)
	auditorKeyID := hex.EncodeToString(auditorPriv.Public().(ed25519.PublicKey))

	sessionEncryptionKey, err := m.generateSecureToken(32)
	if err != nil {
		return fmt.Errorf("secret_manager: create app settings: %w", err)
	}
	auditorHMACKey, err := m.generateSecureToken(32)
	if err != nil {
		return fmt.Errorf("secret_manager: create app settings: %w", err)
	}

	secrets := map[string]string{
		constants.SecretsFileSessionEncryptionKey: sessionEncryptionKey,
		constants.SecretsFileActuatorSigningKey:   ActuatorSeed, // Seed for ED25519
		constants.SecretsFileActuatorKeyID:        ActuatorKeyID,
		constants.SecretsFileAuditorSigningKey:    auditorSeed,
		constants.SecretsFileAuditorKeyID:         auditorKeyID,
		constants.SecretsFileAuditorHMACKey:       auditorHMACKey,
	}

	platformSettings := models.SettingsDocument{
		Settings: &models.PlatformSettings{
			ActuatorKeyID: secrets[constants.SecretsFileActuatorKeyID],
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	dataJSON, err := json.Marshal(platformSettings)
	if err != nil {
		return fmt.Errorf("secret_manager: create app settings: marshal: %w", err)
	}

	nowStr := timesvc.FormatTimestamp(now)
	_, err = m.db.ExecWithRetry(
		`INSERT INTO documents (collection, id, data, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"settings", "platform_settings", string(dataJSON), nowStr, nowStr,
	)
	if err != nil {
		return fmt.Errorf("secret_manager: create app settings: insert: %w", err)
	}
	m.logger.Info("[SecretManager] platform_settings document created with security secrets")

	m.warmAppSettingsCache(string(dataJSON), now)

	for _, name := range requiredBootstrapSecrets {
		if err := m.keystore.EncryptSecret(name, secrets[name]); err != nil {
			return fmt.Errorf("secret_manager: create app settings: encrypt secret %s: %w", name, err)
		}
	}

	if err := m.writeDigestManifestFromEncryptedFiles(now); err != nil {
		return fmt.Errorf("secret_manager: create app settings: manifest: %w", err)
	}

	return m.validateAppSettings()
}

func (m *SecretManager) validateAppSettings() error {
	info, err := m.fileSvc.Stat(context.Background(), constants.SecretsDirname)
	if err != nil {
		return fmt.Errorf("secret_manager: validate app settings: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("secret_manager: validate app settings: %w", constants.ErrNotADirectory)
	}

	manifest, err := m.readDigestManifest()
	if err != nil {
		// If bootstrap digest manifest is missing, treat this as corrupted state
		// and recreate secrets (e.g., when .g8e directory was wiped but DB persists)
		if errors.Is(err, constants.ErrNotFound) {
			m.logger.Warn("[SecretManager] Bootstrap digest manifest missing, recreating secrets")
			return m.recreateAppSettings()
		}
		return fmt.Errorf("secret_manager: validate app settings: %w", err)
	}

	for _, name := range requiredBootstrapSecrets {
		if err := m.validateManifestSecret(manifest, name); err != nil {
			return fmt.Errorf("secret_manager: validate app settings: %w", err)
		}
	}

	return nil
}

func (m *SecretManager) validateManifestSecret(manifest *bootstrapDigestManifest, name string) error {
	entry, ok := manifest.Secrets[name]
	if !ok || entry.SHA256 == "" {
		return fmt.Errorf("%s: %w", name, constants.ErrNotFound)
	}
	relPath := filepath.Join(constants.SecretsDirname, name)
	encryptedData, err := m.fileSvc.ReadFile(context.Background(), relPath)
	if err != nil {
		return fmt.Errorf("read secret %s: %w", name, err)
	}
	encryptedDigest := sha256.Sum256(encryptedData)
	if actual := hex.EncodeToString(encryptedDigest[:]); actual != entry.SHA256 {
		return fmt.Errorf("%s: %w", name, constants.ErrValidationFailed)
	}
	return nil
}

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
		UpdatedAt: timesvc.FormatTimestamp(now),
		Secrets:   make(map[string]bootstrapDigestRef, len(requiredBootstrapSecrets)),
	}

	for _, name := range requiredBootstrapSecrets {
		relPath := filepath.Join(constants.SecretsDirname, name)
		data, err := m.fileSvc.ReadFile(context.Background(), relPath)
		if err != nil {
			return fmt.Errorf("secret_manager: write digest manifest: read %s: %w", name, err)
		}
		sum := sha256.Sum256(data)
		manifest.Secrets[name] = bootstrapDigestRef{SHA256: hex.EncodeToString(sum[:])}
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("secret_manager: write digest manifest: marshal: %w", err)
	}

	manifestRelPath := filepath.Join(constants.SecretsDirname, constants.SecretsFileBootstrapDigest)
	if err := m.fileSvc.WriteFile(context.Background(), manifestRelPath, data, constants.PermFilePrivate); err != nil {
		m.logger.Error("[SecretManager] Failed to write bootstrap digest manifest",
			"error", err)
		return fmt.Errorf("secret_manager: write digest manifest: write: %w", err)
	}
	m.logger.Info("[SecretManager] Bootstrap digest manifest written from encrypted files",
		"secrets", len(manifest.Secrets))
	return nil
}

func (m *SecretManager) readDigestManifest() (*bootstrapDigestManifest, error) {
	manifestRelPath := filepath.Join(constants.SecretsDirname, constants.SecretsFileBootstrapDigest)
	data, err := m.fileSvc.ReadFile(context.Background(), manifestRelPath)
	if err != nil {
		if errors.Is(err, constants.ErrNotFound) {
			return nil, fmt.Errorf("secret_manager: read digest manifest: %w", constants.ErrNotFound)
		}
		return nil, fmt.Errorf("secret_manager: read digest manifest: %w", err)
	}

	var manifest bootstrapDigestManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("secret_manager: read digest manifest: unmarshal: %w", err)
	}
	if manifest.Version != 1 {
		return nil, fmt.Errorf("secret_manager: read digest manifest: version %d: %w", manifest.Version, constants.ErrValidationFailed)
	}
	if manifest.Secrets == nil {
		return nil, fmt.Errorf("secret_manager: read digest manifest: %w", constants.ErrMissingRequiredField)
	}
	return &manifest, nil
}

func (m *SecretManager) rejectPreexistingBootstrapState() error {
	for _, name := range requiredBootstrapSecrets {
		relPath := filepath.Join(constants.SecretsDirname, name)
		exists, err := m.fileSvc.FileExists(context.Background(), relPath)
		if err != nil {
			return fmt.Errorf("secret_manager: reject preexisting bootstrap state: %s: %w", name, err)
		}
		if exists {
			return fmt.Errorf("secret_manager: reject preexisting bootstrap state: %s: %w", name, constants.ErrAlreadyExists)
		}
	}
	manifestRelPath := filepath.Join(constants.SecretsDirname, constants.SecretsFileBootstrapDigest)
	exists, err := m.fileSvc.FileExists(context.Background(), manifestRelPath)
	if err != nil {
		return fmt.Errorf("secret_manager: reject preexisting bootstrap state: digest manifest: %w", err)
	}
	if exists {
		return fmt.Errorf("secret_manager: reject preexisting bootstrap state: digest manifest: %w", constants.ErrAlreadyExists)
	}
	return nil
}

func (m *SecretManager) warmAppSettingsCache(dataJSON string, now time.Time) {
	cacheKey := "g8e:cache:doc:settings:platform_settings"
	cacheTTL := 3600
	nowStr := timesvc.FormatTimestamp(now)
	_, err := m.db.ExecWithRetry(
		`INSERT INTO kv_store (key, value, created_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		cacheKey, dataJSON, nowStr, timesvc.FormatTimestamp(now.Add(time.Duration(cacheTTL)*time.Second)),
	)
	if err != nil {
		m.logger.Warn("[SecretManager] Failed to warm cache for platform_settings", "error", err)
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
		return nil, fmt.Errorf("secret_manager: generate secure token bytes: %w", constants.ErrValidationFailed)
	}
	tokenBytes := make([]byte, bytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("secret_manager: generate secure token bytes: %w", err)
	}
	return tokenBytes, nil
}

// GetActuatorKey retrieves the Actuator's ED25519 signing key and its KeyID.
// The signing key is decrypted from the keystore; the KeyID is read from platform_settings.
func (m *SecretManager) GetActuatorKey() (ed25519.PrivateKey, string, error) {
	seedHex, err := m.keystore.DecryptSecret(constants.SecretsFileActuatorSigningKey)
	if err != nil {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: decrypt: %w", err)
	}

	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: decode: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: %w", constants.ErrValidationFailed)
	}

	priv := ed25519.NewKeyFromSeed(seed)

	var dataJSON string
	if err := m.db.QueryRowWithRetry(
		"SELECT data FROM documents WHERE collection = 'settings' AND id = 'platform_settings'",
	).Scan(&dataJSON); err != nil {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: query: %w", err)
	}

	var settings models.SettingsDocument
	if err := json.Unmarshal([]byte(dataJSON), &settings); err != nil {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: unmarshal: %w", err)
	}
	if settings.Settings == nil {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: %w", constants.ErrMissingRequiredField)
	}

	keyID := strings.TrimSpace(settings.Settings.ActuatorKeyID)
	if keyID == "" {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: %w", constants.ErrMissingRequiredField)
	}

	expectedKeyID := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	if keyID != expectedKeyID {
		return nil, "", fmt.Errorf("secret_manager: get actuator key: %w", constants.ErrValidationFailed)
	}

	return priv, keyID, nil
}

// GetAuditorKey retrieves the Auditor's ED25519 signing key and key ID.
func (m *SecretManager) GetAuditorKey() (ed25519.PrivateKey, string, error) {
	seedHex, err := m.keystore.DecryptSecret(constants.SecretsFileAuditorSigningKey)
	if err != nil {
		return nil, "", fmt.Errorf("secret_manager: get auditor key: decrypt seed: %w", err)
	}
	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		return nil, "", fmt.Errorf("secret_manager: get auditor key: decode seed: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, "", fmt.Errorf("secret_manager: get auditor key: %w", constants.ErrValidationFailed)
	}
	keyID, err := m.keystore.DecryptSecret(constants.SecretsFileAuditorKeyID)
	if err != nil {
		return nil, "", fmt.Errorf("secret_manager: get auditor key: decrypt key ID: %w", err)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	if keyID == "" || keyID != hex.EncodeToString(priv.Public().(ed25519.PublicKey)) {
		return nil, "", fmt.Errorf("secret_manager: get auditor key: %w", constants.ErrValidationFailed)
	}
	return priv, keyID, nil
}

// GetSessionEncryptionKey retrieves the session encryption key from the keystore.
func (m *SecretManager) GetSessionEncryptionKey() (string, error) {
	return m.keystore.DecryptSecret(constants.SecretsFileSessionEncryptionKey)
}

// GetAuditorHMACKey retrieves the auditor HMAC key from the keystore.
func (m *SecretManager) GetAuditorHMACKey() (string, error) {
	return m.keystore.DecryptSecret(constants.SecretsFileAuditorHMACKey)
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

// StoreNotaryKey stores an L3 Notary signing key in the keystore.
func (m *SecretManager) StoreNotaryKey(seedHex string) error {
	return m.keystore.EncryptSecret(constants.SecretsFileNotarySigningKey, seedHex)
}

// GetNotaryKey retrieves an L3 Notary signing key from the keystore.
func (m *SecretManager) GetNotaryKey() (string, error) {
	return m.keystore.DecryptSecret(constants.SecretsFileNotarySigningKey)
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
	return m.keystore.EncryptSecret(constants.SecretsFileOperatorPrivateKey, seedHex)
}

// GetOperatorPrivateKey retrieves an Operator ed25519 private key from the keystore.
func (m *SecretManager) GetOperatorPrivateKey() (ed25519.PrivateKey, error) {
	seedHex, err := m.keystore.DecryptSecret(constants.SecretsFileOperatorPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("secret_manager: get operator private key: decrypt: %w", err)
	}
	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		return nil, fmt.Errorf("secret_manager: get operator private key: decode: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("secret_manager: get operator private key: %w", constants.ErrValidationFailed)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// StoreCLIPrivateKey stores a CLI ed25519 private key in the keystore.
// This is used for BYO-client enrollment where the CLI has its own identity.
func (m *SecretManager) StoreCLIPrivateKey(key ed25519.PrivateKey) error {
	seedHex := hex.EncodeToString(key.Seed())
	return m.keystore.EncryptSecret(constants.SecretsFileCLIPrivateKey, seedHex)
}

// GetCLIPrivateKey retrieves a CLI ed25519 private key from the keystore.
func (m *SecretManager) GetCLIPrivateKey() (ed25519.PrivateKey, error) {
	seedHex, err := m.keystore.DecryptSecret(constants.SecretsFileCLIPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("secret_manager: get CLI private key: decrypt: %w", err)
	}
	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		return nil, fmt.Errorf("secret_manager: get CLI private key: decode: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("secret_manager: get CLI private key: %w", constants.ErrValidationFailed)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// StoreSessionToken stores a session token with TTL in the keystore.
// The token is stored with a timestamp for TTL validation.
func (m *SecretManager) StoreSessionToken(token string, ttl time.Duration) error {
	expiresAt := timesvc.FormatTimestamp(time.Now().UTC().Add(ttl))
	tokenData := fmt.Sprintf("%s|%s", token, expiresAt)
	return m.keystore.EncryptSecret(constants.SecretsFileSessionToken, tokenData)
}

// GetSessionToken retrieves a session token from the keystore.
// Returns error if the token has expired.
func (m *SecretManager) GetSessionToken() (string, error) {
	tokenData, err := m.keystore.DecryptSecret(constants.SecretsFileSessionToken)
	if err != nil {
		return "", fmt.Errorf("secret_manager: get session token: decrypt: %w", err)
	}

	parts := strings.Split(tokenData, "|")
	if len(parts) != 2 {
		return "", fmt.Errorf("secret_manager: get session token: %w", constants.ErrValidationFailed)
	}

	token := parts[0]
	expiresAtStr := parts[1]
	expiresAt, err := timesvc.ParseTimestamp(expiresAtStr)
	if err != nil {
		return "", fmt.Errorf("secret_manager: get session token: parse expiry: %w", err)
	}

	if time.Now().UTC().After(expiresAt) {
		return "", fmt.Errorf("secret_manager: get session token: %w", constants.ErrExpired)
	}

	return token, nil
}
