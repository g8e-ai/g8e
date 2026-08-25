// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build linux

package keystore

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// libsecretKeyring uses the libsecret/GNOME Keyring for key storage on Linux.
type libsecretKeyring struct{}

func newLibsecretKeyring() (Keyring, error) {
	// Check if secret-tool is available
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return nil, fmt.Errorf("libsecret: check secret-tool availability: %w (install libsecret-tools)", err)
	}
	return &libsecretKeyring{}, nil
}

func (l *libsecretKeyring) Name() string {
	return "libsecret"
}

func (l *libsecretKeyring) RetrieveMasterKey() ([]byte, error) {
	args := []string{
		"lookup",
		keyStoreName,
		masterKeyName,
	}

	cmd := exec.Command("secret-tool", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// secret-tool returns exit code 1 when item not found
			return nil, constants.ErrKeyStoreKeyNotFound
		}
		return nil, fmt.Errorf("libsecret: lookup master key: %w", err)
	}

	// secret-tool returns base64-encoded value
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(output)))
	if err != nil {
		return nil, fmt.Errorf("libsecret: decode base64 key: %w", err)
	}

	if len(key) == 0 {
		return nil, constants.ErrKeyStoreKeyNotFound
	}

	return key, nil
}

func (l *libsecretKeyring) StoreMasterKey(key []byte) error {
	// Encode key as base64 for safe storage
	encoded := base64.StdEncoding.EncodeToString(key)

	args := []string{
		"store",
		"--label=" + keyStoreName,
		keyStoreName,
		masterKeyName,
		encoded,
	}

	cmd := exec.Command("secret-tool", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("libsecret: store master key: %w", err)
	}

	return nil
}

func (l *libsecretKeyring) DeleteMasterKey() error {
	args := []string{
		"clear",
		keyStoreName,
		masterKeyName,
	}

	cmd := exec.Command("secret-tool", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Don't error if key doesn't exist (already deleted)
		if strings.Contains(stderr.String(), "not found") {
			return nil
		}
		return fmt.Errorf("libsecret: clear master key: %w", err)
	}

	return nil
}
