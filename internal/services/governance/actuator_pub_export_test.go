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
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/require"
)

// actuatorPublicKeyExportData represents the typed structure for actuator public key JSON export
type actuatorPublicKeyExportData struct {
	KeyID     string `json:"key_id"`
	PublicKey string `json:"public_key"`
	Algorithm string `json:"algorithm"`
}

func TestActuatorPublicKeyExport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		keyID string
	}{
		{
			name:  "standard export",
			keyID: "test-Actuator-key",
		},
		{
			name:  "export with different key ID",
			keyID: "alternate-key-id",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			pubKey, _, err := ed25519.GenerateKey(nil)
			require.NoError(t, err)

			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

			// Write public key using the same logic as main.go's exportActuatorPublicKey
			err = exportActuatorPublicKey(tmpDir, pubKey, tt.keyID, logger)
			require.NoError(t, err)

			// 1. Verify PEM file
			pemPath := filepath.Join(tmpDir, constants.ActuatorPubPEMFilename)
			pemData, err := os.ReadFile(pemPath)
			require.NoError(t, err)

			block, rest := pem.Decode(pemData)
			require.NotNil(t, block, "failed to decode PEM block")
			require.Empty(t, rest, "unexpected trailing data after PEM block")
			require.Equal(t, "PUBLIC KEY", block.Type)
			require.Equal(t, []byte(pubKey), block.Bytes)

			// 2. Verify JSON file
			jsonPath := filepath.Join(tmpDir, constants.ActuatorPubJSONFilename)
			jsonData, err := os.ReadFile(jsonPath)
			require.NoError(t, err)

			var parsed actuatorPublicKeyExportData
			err = json.Unmarshal(jsonData, &parsed)
			require.NoError(t, err)

			require.Equal(t, tt.keyID, parsed.KeyID)
			require.Equal(t, hex.EncodeToString(pubKey), parsed.PublicKey)
			require.Equal(t, "ed25519", parsed.Algorithm)
		})
	}
}

// exportActuatorPublicKey is a copy of the function in main.go to allow testing.
// In a real refactor, this should move to internal/services/governance/Actuator.go.
func exportActuatorPublicKey(pkiDir string, pubKey ed25519.PublicKey, keyID string, logger *slog.Logger) error {
	if err := os.MkdirAll(pkiDir, 0700); err != nil {
		return fmt.Errorf("exportActuatorPublicKey: failed to create PKI directory: %w", err)
	}

	// Write PEM format
	pemPath := filepath.Join(pkiDir, constants.ActuatorPubPEMFilename)
	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKey,
	})
	if err := os.WriteFile(pemPath, pemData, 0644); err != nil {
		return fmt.Errorf("exportActuatorPublicKey: failed to write PEM file: %w", err)
	}

	// Write JSON format
	jsonPath := filepath.Join(pkiDir, constants.ActuatorPubJSONFilename)
	jsonData := actuatorPublicKeyExportData{
		KeyID:     keyID,
		PublicKey: hex.EncodeToString(pubKey),
		Algorithm: "ed25519",
	}
	jsonBytes, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return fmt.Errorf("exportActuatorPublicKey: failed to marshal JSON: %w", err)
	}
	if err := os.WriteFile(jsonPath, jsonBytes, 0644); err != nil {
		return fmt.Errorf("exportActuatorPublicKey: failed to write JSON file: %w", err)
	}
	return nil
}
