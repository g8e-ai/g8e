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

//go:build darwin

package keystore

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
)

// keychainBackend uses macOS Keychain for key storage.
type keychainBackend struct{}

func newKeychainBackend() (Backend, error) {
	// Check if security command is available
	if _, err := exec.LookPath("security"); err != nil {
		return nil, fmt.Errorf("security command not found: %w", err)
	}
	return &keychainBackend{}, nil
}

func (b *keychainBackend) Name() string {
	return "keychain"
}

func (b *keychainBackend) RetrieveMasterKey() ([]byte, error) {
	account := fmt.Sprintf("%s/%s", keyStoreName, masterKeyName)

	args := []string{
		"find-generic-password",
		"-a", account,
		"-s", keyStoreName,
		"-w",
	}

	cmd := exec.Command("security", args...)
	output, err := cmd.Output()
	if err != nil {
		if strings.Contains(string(err.(*exec.ExitError).Stderr), "could not be found") {
			return nil, ErrKeyNotFound
		}
		return nil, fmt.Errorf("security find-generic-password: %w", err)
	}

	// Keychain returns base64-encoded value
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(output)))
	if err != nil {
		return nil, fmt.Errorf("decode base64 key: %w", err)
	}

	if len(key) == 0 {
		return nil, ErrKeyNotFound
	}

	return key, nil
}

func (b *keychainBackend) StoreMasterKey(key []byte) error {
	account := fmt.Sprintf("%s/%s", keyStoreName, masterKeyName)
	encoded := base64.StdEncoding.EncodeToString(key)

	args := []string{
		"add-generic-password",
		"-a", account,
		"-s", keyStoreName,
		"-w", encoded,
		"-U", // Update if exists
	}

	cmd := exec.Command("security", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("security add-generic-password: %w", err)
	}

	return nil
}

func (b *keychainBackend) DeleteMasterKey() error {
	account := fmt.Sprintf("%s/%s", keyStoreName, masterKeyName)

	args := []string{
		"delete-generic-password",
		"-a", account,
		"-s", keyStoreName,
	}

	cmd := exec.Command("security", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Don't error if key doesn't exist (already deleted)
		if strings.Contains(stderr.String(), "could not be found") {
			return nil
		}
		return fmt.Errorf("security delete-generic-password: %w", err)
	}

	return nil
}
