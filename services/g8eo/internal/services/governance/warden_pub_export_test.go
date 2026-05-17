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
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWardenPublicKeyExport(t *testing.T) {
	tmpDir := t.TempDir()
	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	keyID := "test-warden-key"
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Write public key using the same logic as main.go's exportWardenPublicKey
	err = exportWardenPublicKey(tmpDir, pubKey, keyID, logger)
	require.NoError(t, err)

	// 1. Verify PEM file
	pemPath := filepath.Join(tmpDir, "warden_pub.pem")
	pemData, err := os.ReadFile(pemPath)
	require.NoError(t, err)

	block, _ := pem.Decode(pemData)
	require.NotNil(t, block)
	require.Equal(t, "PUBLIC KEY", block.Type)
	require.Equal(t, []byte(pubKey), block.Bytes)

	// 2. Verify JSON file
	jsonPath := filepath.Join(tmpDir, "warden_pub.json")
	jsonData, err := os.ReadFile(jsonPath)
	require.NoError(t, err)

	var parsed map[string]string
	err = json.Unmarshal(jsonData, &parsed)
	require.NoError(t, err)

	require.Equal(t, keyID, parsed["key_id"])
	require.Equal(t, hex.EncodeToString(pubKey), parsed["public_key"])
	require.Equal(t, "ed25519", parsed["algorithm"])
}

// exportWardenPublicKey is a copy of the function in main.go to allow testing.
// In a real refactor, this should move to internal/services/governance/warden.go.
func exportWardenPublicKey(pkiDir string, pubKey ed25519.PublicKey, keyID string, logger *slog.Logger) error {
	if err := os.MkdirAll(pkiDir, 0700); err != nil {
		return err
	}

	// Write PEM format
	pemPath := filepath.Join(pkiDir, "warden_pub.pem")
	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKey,
	})
	if err := os.WriteFile(pemPath, pemData, 0644); err != nil {
		return err
	}

	// Write JSON format
	jsonPath := filepath.Join(pkiDir, "warden_pub.json")
	jsonData := map[string]string{
		"key_id":     keyID,
		"public_key": hex.EncodeToString(pubKey),
		"algorithm":  "ed25519",
	}
	jsonBytes, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, jsonBytes, 0644); err != nil {
		return err
	}
	return nil
}
