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

package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExportWardenPublicKey(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Generate a test Ed25519 key pair
	pubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	keyID := hex.EncodeToString(pubKey)

	// Call exportWardenPublicKey with nil logger (logging is optional)
	err = exportWardenPublicKey(tmpDir, pubKey, keyID, nil)
	if err != nil {
		t.Fatalf("exportWardenPublicKey failed: %v", err)
	}

	// Verify PEM file exists and contains the correct key
	pemPath := filepath.Join(tmpDir, "warden_pub.pem")
	pemData, err := os.ReadFile(pemPath)
	if err != nil {
		t.Fatalf("Failed to read PEM file: %v", err)
	}

	if len(pemData) == 0 {
		t.Error("PEM file is empty")
	}

	// Verify the PEM data starts with the correct header
	pemString := string(pemData)
	if len(pemString) < 26 || pemString[:26] != "-----BEGIN PUBLIC KEY-----" {
		t.Errorf("PEM file does not have correct header, got: %s", pemString[:min(50, len(pemString))])
	}

	// Verify JSON file exists and contains the correct data
	jsonPath := filepath.Join(tmpDir, "warden_pub.json")
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("Failed to read JSON file: %v", err)
	}

	var jsonKey struct {
		KeyID     string `json:"key_id"`
		PublicKey string `json:"public_key"`
		Algorithm string `json:"algorithm"`
	}

	if err := json.Unmarshal(jsonData, &jsonKey); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if jsonKey.KeyID != keyID {
		t.Errorf("Expected key_id %s, got %s", keyID, jsonKey.KeyID)
	}

	if jsonKey.PublicKey != hex.EncodeToString(pubKey) {
		t.Errorf("Expected public_key %s, got %s", hex.EncodeToString(pubKey), jsonKey.PublicKey)
	}

	if jsonKey.Algorithm != "ed25519" {
		t.Errorf("Expected algorithm ed25519, got %s", jsonKey.Algorithm)
	}
}

func TestExportWardenPublicKeyCreatesDirectory(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create a subdirectory that doesn't exist yet
	subDir := filepath.Join(tmpDir, "pki", "nested")

	// Generate a test key
	pubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	keyID := hex.EncodeToString(pubKey)

	// Call exportWardenPublicKey with a non-existent directory
	err = exportWardenPublicKey(subDir, pubKey, keyID, nil)
	if err != nil {
		t.Fatalf("exportWardenPublicKey failed: %v", err)
	}

	// Verify the directory was created
	info, err := os.Stat(subDir)
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}

	if !info.IsDir() {
		t.Error("Path is not a directory")
	}

	// Verify files were created in the new directory
	pemPath := filepath.Join(subDir, "warden_pub.pem")
	if _, err := os.Stat(pemPath); err != nil {
		t.Errorf("PEM file not created in new directory: %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
