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

package vault

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// VaultHeader contains the metadata and wrapped DEK for an encrypted vault.
// The header is stored at .g8e/data/vault.header as JSON.
// The DEK is wrapped (encrypted) with the KEK derived from the API key.
type VaultHeader struct {
	Version        int        `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	LastRekeyedAt  *time.Time `json:"last_rekeyed_at,omitempty"`
	KDF            KDFParams  `json:"kdf"`
	KEK            KEKParams  `json:"kek"`
	DEK            DEKParams  `json:"dek"`
	KeyFingerprint string     `json:"key_fingerprint"`
}

// KDFParams contains key derivation function configuration
type KDFParams struct {
	Algorithm string `json:"algorithm"`
	Info      string `json:"info"`
}

// KEKParams contains key encryption key configuration
type KEKParams struct {
	Algorithm string `json:"algorithm"`
}

// DEKParams contains the wrapped data encryption key
type DEKParams struct {
	Algorithm string `json:"algorithm"`
	Wrapped   string `json:"wrapped"`
}

const (
	VaultHeaderVersion = 1
	VaultHeaderFile    = "vault.header"

	KDFAlgorithm = "hkdf-sha256"
	KEKAlgorithm = "aes-256-kw"
	DEKAlgorithm = "aes-256-gcm"
)

var (
	ErrHeaderNotFound      = errors.New("vault header not found")
	ErrHeaderCorrupted     = errors.New("vault header is corrupted")
	ErrHeaderVersionUnsup  = errors.New("unsupported vault header version")
	ErrKeyFingerprintMatch = errors.New("API key fingerprint mismatch")
	ErrVaultAlreadyExists  = errors.New("vault already exists")
)

// NewVaultHeader creates a new vault header with a freshly generated DEK
// wrapped with a KEK derived from the provided API key.
func NewVaultHeader(apiKey string) (*VaultHeader, []byte, error) {
	kek, err := DeriveKEK(apiKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to derive KEK: %w", err)
	}
	defer SecureZero(kek)

	dek, err := GenerateDEK()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate DEK: %w", err)
	}

	wrappedDEK, err := AESKeyWrap(kek, dek)
	if err != nil {
		SecureZero(dek)
		return nil, nil, fmt.Errorf("failed to wrap DEK: %w", err)
	}

	header := &VaultHeader{
		Version:   VaultHeaderVersion,
		CreatedAt: time.Now().UTC(),
		KDF: KDFParams{
			Algorithm: KDFAlgorithm,
			Info:      HKDFInfo,
		},
		KEK: KEKParams{
			Algorithm: KEKAlgorithm,
		},
		DEK: DEKParams{
			Algorithm: DEKAlgorithm,
			Wrapped:   base64.StdEncoding.EncodeToString(wrappedDEK),
		},
		KeyFingerprint: hex.EncodeToString(APIKeyFingerprint(apiKey)),
	}

	return header, dek, nil
}

// UnwrapDEK unwraps the DEK using the provided API key.
func (h *VaultHeader) UnwrapDEK(apiKey string) ([]byte, error) {
	expectedFingerprint := hex.EncodeToString(APIKeyFingerprint(apiKey))
	if h.KeyFingerprint != expectedFingerprint {
		return nil, ErrKeyFingerprintMatch
	}

	kek, err := DeriveKEK(apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to derive KEK: %w", err)
	}
	defer SecureZero(kek)

	wrappedDEK, err := base64.StdEncoding.DecodeString(h.DEK.Wrapped)
	if err != nil {
		return nil, fmt.Errorf("failed to decode wrapped DEK: %w", err)
	}

	dek, err := AESKeyUnwrap(kek, wrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap DEK: %w", err)
	}

	return dek, nil
}

// Rekey re-wraps the DEK with a new API key.
func (h *VaultHeader) Rekey(oldAPIKey, newAPIKey string) error {
	dek, err := h.UnwrapDEK(oldAPIKey)
	if err != nil {
		return fmt.Errorf("failed to unwrap DEK with old key: %w", err)
	}
	defer SecureZero(dek)

	newKEK, err := DeriveKEK(newAPIKey)
	if err != nil {
		return fmt.Errorf("failed to derive new KEK: %w", err)
	}
	defer SecureZero(newKEK)

	wrappedDEK, err := AESKeyWrap(newKEK, dek)
	if err != nil {
		return fmt.Errorf("failed to wrap DEK with new key: %w", err)
	}

	now := time.Now().UTC()
	h.LastRekeyedAt = &now
	h.DEK.Wrapped = base64.StdEncoding.EncodeToString(wrappedDEK)
	h.KeyFingerprint = hex.EncodeToString(APIKeyFingerprint(newAPIKey))

	return nil
}

// Save writes the header to disk at the specified data directory.
func (h *VaultHeader) Save(dataDir string) error {
	headerPath := filepath.Join(dataDir, VaultHeaderFile)

	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal header: %w", err)
	}

	tempPath := headerPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	if err := os.Rename(tempPath, headerPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename header: %w", err)
	}

	return nil
}

// LoadVaultHeader loads a vault header from the specified data directory.
func LoadVaultHeader(dataDir string) (*VaultHeader, error) {
	headerPath := filepath.Join(dataDir, VaultHeaderFile)

	data, err := os.ReadFile(headerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrHeaderNotFound
		}
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	var header VaultHeader
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, ErrHeaderCorrupted
	}

	if header.Version > VaultHeaderVersion {
		return nil, ErrHeaderVersionUnsup
	}

	return &header, nil
}

// VaultHeaderExists checks if a vault header exists at the specified data directory.
func VaultHeaderExists(dataDir string) bool {
	headerPath := filepath.Join(dataDir, VaultHeaderFile)
	_, err := os.Stat(headerPath)
	return err == nil
}

// DeleteVaultHeader removes the vault header file. This makes the vault unrecoverable.
func DeleteVaultHeader(dataDir string) error {
	headerPath := filepath.Join(dataDir, VaultHeaderFile)
	if err := os.Remove(headerPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete header: %w", err)
	}
	return nil
}

// ValidateAPIKey checks if the provided API key matches the vault's key fingerprint.
func (h *VaultHeader) ValidateAPIKey(apiKey string) bool {
	expectedFingerprint := hex.EncodeToString(APIKeyFingerprint(apiKey))
	return h.KeyFingerprint == expectedFingerprint
}
