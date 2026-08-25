// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build darwin

package keystore

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// keychainKeyring uses macOS Keychain for key storage.
type keychainKeyring struct{}

func newKeychainKeyring() (Keyring, error) {
	// Check if security command is available
	if _, err := exec.LookPath("security"); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrKeyStoreSecurityNotFound, err)
	}
	return &keychainKeyring{}, nil
}

func (c *keychainKeyring) Name() string {
	return "keychain"
}

func (c *keychainKeyring) RetrieveMasterKey() ([]byte, error) {
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
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// macOS security command returns exit code 44 when item not found
			if exitErr.ExitCode() == 44 {
				return nil, constants.ErrKeyStoreKeyNotFound
			}
		}
		return nil, fmt.Errorf("%w: %w", constants.ErrKeyStoreRetrieveFailed, err)
	}

	// Keychain returns base64-encoded value
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(output)))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrKeyStoreDecodeFailed, err)
	}

	if len(key) == 0 {
		return nil, constants.ErrKeyStoreKeyNotFound
	}

	return key, nil
}

func (c *keychainKeyring) StoreMasterKey(key []byte) error {
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
		return fmt.Errorf("%w: %w", constants.ErrKeyStoreStoreFailed, err)
	}

	return nil
}

func (c *keychainKeyring) DeleteMasterKey() error {
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
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// macOS security command returns exit code 44 when item not found
			if exitErr.ExitCode() == 44 {
				return nil
			}
		}
		return fmt.Errorf("%w: %w", constants.ErrKeyStoreDeleteFailed, err)
	}

	return nil
}
