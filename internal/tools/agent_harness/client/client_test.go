// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Config
		wantErr bool
	}{
		{
			name: "minimal config",
			cfg: config.Config{
				Auth: config.Auth{},
			},
			wantErr: false,
		},
		{
			name: "config with CA bundle",
			cfg: config.Config{
				Auth: config.Auth{
					CABundle: "nonexistent.pem",
				},
			},
			wantErr: false, // missing CA bundle is tolerated
		},
		{
			name: "config with client cert/key",
			cfg: config.Config{
				Auth: config.Auth{
					ClientCert: "nonexistent.crt",
					ClientKey:  "nonexistent.key",
				},
			},
			wantErr: false, // missing cert/key is tolerated
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := New(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("New() returned nil client")
			}
		})
	}
}

func TestNew_WithValidCerts(t *testing.T) {
	tempDir := testutil.TempDir(t)
	caPath := filepath.Join(tempDir, "ca.pem")
	certPath := filepath.Join(tempDir, "client.crt")
	keyPath := filepath.Join(tempDir, "client.key")

	// Write a minimal valid CA bundle
	caCert := `-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAK3QW7B4Ls7ZMA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBnRl
c3RjYTAeFw0yNDAxMDEwMDAwMDBaFw0yNTAxMDEwMDAwMDBaMBExDzANBgNVBAMM
BnRlc3RjYTCBnzANBgkqhkiG9w0BAQEFAAOBjQAwgYkCgYEAwT8kQCE6x5V8U2v7
-----END CERTIFICATE-----`
	if err := os.WriteFile(caPath, []byte(caCert), 0600); err != nil {
		t.Fatalf("failed to write CA: %v", err)
	}

	cfg := config.Config{
		Auth: config.Auth{
			CABundle:   caPath,
			ClientCert: certPath, // doesn't exist, should be tolerated
			ClientKey:  keyPath,  // doesn't exist, should be tolerated
		},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if client == nil {
		t.Fatal("New() returned nil client")
	}
}

func TestNew_InvalidKeyPair(t *testing.T) {
	tempDir := testutil.TempDir(t)
	certPath := filepath.Join(tempDir, "client.crt")
	keyPath := filepath.Join(tempDir, "client.key")

	// Write invalid cert/key pair
	if err := os.WriteFile(certPath, []byte("invalid cert"), 0600); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte("invalid key"), 0600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}

	cfg := config.Config{
		Auth: config.Auth{
			ClientCert: certPath,
			ClientKey:  keyPath,
		},
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("New() should fail with invalid cert/key pair")
	}
}

func TestClient_Config(t *testing.T) {
	cfg := config.Config{
		MTLSBaseURL: "https://example.com",
		Auth:        config.Auth{},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	retrievedCfg := client.Config()
	if retrievedCfg.MTLSBaseURL != cfg.MTLSBaseURL {
		t.Errorf("Config() returned different MTLSBaseURL")
	}
}

func TestClient_Record(t *testing.T) {
	cfg := config.Config{Auth: config.Auth{}}
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	var exchanges []Exchange
	client.Record(&exchanges)

	if client.rec == nil {
		t.Error("Record() did not set rec")
	}
	if client.rec != &exchanges {
		t.Error("Record() did not set rec to the provided sink")
	}
}

func TestAttachBody(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		wantJSON bool
		wantRaw  bool
	}{
		{
			name:     "nil input",
			input:    nil,
			wantJSON: false,
			wantRaw:  false,
		},
		{
			name:     "empty input",
			input:    []byte{},
			wantJSON: false,
			wantRaw:  false,
		},
		{
			name:     "valid JSON",
			input:    []byte(`{"test": "value"}`),
			wantJSON: true,
			wantRaw:  false,
		},
		{
			name:     "invalid JSON",
			input:    []byte(`not json`),
			wantJSON: false,
			wantRaw:  true,
		},
		{
			name:     "plain text",
			input:    []byte(`plain text response`),
			wantJSON: false,
			wantRaw:  true,
		},
		{
			name:     "JSON array",
			input:    []byte(`[1, 2, 3]`),
			wantJSON: true,
			wantRaw:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var j json.RawMessage
			var raw string

			attachBody(&j, &raw, tt.input)

			hasJSON := len(j) > 0
			hasRaw := raw != ""

			if hasJSON != tt.wantJSON {
				t.Errorf("attachBody() JSON = %v, want %v", hasJSON, tt.wantJSON)
			}
			if hasRaw != tt.wantRaw {
				t.Errorf("attachBody() raw = %v, want %v", hasRaw, tt.wantRaw)
			}
		})
	}
}

func TestExchange_Marshal(t *testing.T) {
	ex := Exchange{
		Persona:   "test-client",
		Method:    "GET",
		URL:       "http://example.com/test",
		ReqBody:   json.RawMessage(`{"test": "request"}`),
		Status:    200,
		RespBody:  json.RawMessage(`{"result": "ok"}`),
		LatencyMS: 100,
		At:        time.Now(),
	}

	data, err := json.Marshal(ex)
	if err != nil {
		t.Fatalf("failed to marshal Exchange: %v", err)
	}

	var decoded Exchange
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Exchange: %v", err)
	}

	if decoded.Persona != ex.Persona {
		t.Errorf("Persona mismatch: got %s, want %s", decoded.Persona, ex.Persona)
	}
	if decoded.Method != ex.Method {
		t.Errorf("Method mismatch: got %s, want %s", decoded.Method, ex.Method)
	}
	if decoded.Status != ex.Status {
		t.Errorf("Status mismatch: got %d, want %d", decoded.Status, ex.Status)
	}
}

func TestPersona(t *testing.T) {
	p := Persona{
		ID:                "test-id",
		UserAgent:         "TestAgent/1.0",
		OperatorSessionID: "session-123",
		CLISessionID:      "cli-session-123",
		UserID:            "user-123",
		OperatorID:        "operator-123",
	}

	if p.ID != "test-id" {
		t.Errorf("ID mismatch")
	}
	if p.UserAgent != "TestAgent/1.0" {
		t.Errorf("UserAgent mismatch")
	}
	if p.OperatorSessionID != "session-123" {
		t.Errorf("OperatorSessionID mismatch")
	}
	if p.CLISessionID != "cli-session-123" {
		t.Errorf("CLISessionID mismatch")
	}
	if p.UserID != "user-123" {
		t.Errorf("UserID mismatch")
	}
	if p.OperatorID != "operator-123" {
		t.Errorf("OperatorID mismatch")
	}
}

func TestNew_CLIAuthBuildsSecondClient(t *testing.T) {
	t.Run("cliHTTP is nil when CLIAuth equals Auth", func(t *testing.T) {
		cfg := config.Config{Auth: config.Auth{}}
		c, err := New(cfg)
		require.NoError(t, err)
		assert.Nil(t, c.cliHTTP, "cliHTTP should be nil when CLIAuth is not set")
		assert.Nil(t, c.cliTLS, "cliTLS should be nil when CLIAuth is not set")
	})

	t.Run("cliHTTP is nil when CLIAuth matches Auth", func(t *testing.T) {
		cfg := config.Config{
			Auth: config.Auth{
				ClientCert: "/same/cert",
				ClientKey:  "/same/key",
			},
			CLIAuth: config.Auth{
				ClientCert: "/same/cert",
				ClientKey:  "/same/key",
			},
		}
		c, err := New(cfg)
		require.NoError(t, err)
		assert.Nil(t, c.cliHTTP, "cliHTTP should be nil when CLIAuth matches Auth")
	})

	t.Run("cliHTTP is non-nil when CLIAuth differs from Auth (paths missing, tolerated)", func(t *testing.T) {
		cfg := config.Config{
			Auth: config.Auth{
				ClientCert: "/operator/cert",
				ClientKey:  "/operator/key",
			},
			CLIAuth: config.Auth{
				ClientCert: "/cli/cert",
				ClientKey:  "/cli/key",
			},
		}
		c, err := New(cfg)
		require.NoError(t, err)
		assert.NotNil(t, c.cliHTTP, "cliHTTP should be built when CLIAuth differs from Auth")
		assert.NotNil(t, c.cliTLS, "cliTLS should be built when CLIAuth differs from Auth")
	})
}
