// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.MTLSBaseURL == "" {
		t.Error("MTLSBaseURL should not be empty")
	}
	if cfg.PublicBaseURL == "" {
		t.Error("PublicBaseURL should not be empty")
	}
	if cfg.EnvelopeTTL != 5*time.Minute {
		t.Errorf("EnvelopeTTL should be 5 minutes, got %v", cfg.EnvelopeTTL)
	}
	if cfg.OutDir != "./auditor-out" {
		t.Errorf("OutDir should be './auditor-out', got %s", cfg.OutDir)
	}
	if !cfg.UseCLIConfig {
		t.Error("UseCLIConfig should be true")
	}
}

func TestLoadFile(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	configPath := filepath.Join(tmpDir, "config.json")

	// Create a test config file
	testConfig := map[string]any{
		"mtls_base_url":   "https://example.com:8443",
		"public_base_url": "https://example.com:8080",
		"envelope_ttl":    600000000000, // 10 minutes in nanoseconds
		"out_dir":         "/tmp/test-out",
		"use_cli_config":  false,
		"auth": map[string]any{
			"client_cert": "/path/to/cert.pem",
			"client_key":  "/path/to/key.pem",
			"ca_bundle":   "/path/to/ca.pem",
			"api_key":     "test-api-key",
		},
	}

	data, err := json.Marshal(testConfig)
	if err != nil {
		t.Fatalf("Failed to marshal test config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load the config
	cfg := Default()
	err = cfg.LoadFile(configPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	// Verify fields were loaded
	if cfg.MTLSBaseURL != "https://example.com:8443" {
		t.Errorf("MTLSBaseURL not loaded correctly, got %s", cfg.MTLSBaseURL)
	}
	if cfg.PublicBaseURL != "https://example.com:8080" {
		t.Errorf("PublicBaseURL not loaded correctly, got %s", cfg.PublicBaseURL)
	}
	if cfg.OutDir != "/tmp/test-out" {
		t.Errorf("OutDir not loaded correctly, got %s", cfg.OutDir)
	}
	if cfg.UseCLIConfig {
		t.Error("UseCLIConfig should be false after loading")
	}
	if cfg.Auth.ClientCert != "/path/to/cert.pem" {
		t.Errorf("ClientCert not loaded correctly, got %s", cfg.Auth.ClientCert)
	}
	if cfg.Auth.APIKey != "test-api-key" {
		t.Errorf("APIKey not loaded correctly, got %s", cfg.Auth.APIKey)
	}
}

func TestLoadFile_NonExistent(t *testing.T) {
	cfg := Default()
	err := cfg.LoadFile("/nonexistent/path/config.json")
	if err == nil {
		t.Error("LoadFile should return error for non-existent file")
	}
}

func TestLoadFile_InvalidJSON(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	configPath := filepath.Join(tmpDir, "invalid.json")

	if err := os.WriteFile(configPath, []byte("invalid json"), 0o644); err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}

	cfg := Default()
	err := cfg.LoadFile(configPath)
	if err == nil {
		t.Error("LoadFile should return error for invalid JSON")
	}
}

func TestLoadFile_PartialOverlay(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	configPath := filepath.Join(tmpDir, "partial.json")

	// Create a partial config with only some fields
	partialConfig := map[string]any{
		"out_dir": "/tmp/partial-out",
	}

	data, err := json.Marshal(partialConfig)
	if err != nil {
		t.Fatalf("Failed to marshal partial config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("Failed to write partial config: %v", err)
	}

	cfg := Default()
	originalMTLSURL := cfg.MTLSBaseURL

	err = cfg.LoadFile(configPath)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	// Unchanged field should remain from Default()
	if cfg.MTLSBaseURL != originalMTLSURL {
		t.Error("Unchanged field should remain from Default()")
	}
	// Changed field should be updated
	if cfg.OutDir != "/tmp/partial-out" {
		t.Errorf("OutDir should be updated to /tmp/partial-out, got %s", cfg.OutDir)
	}
}
