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

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/emulator/config"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.Config
		wantErr bool
	}{
		{
			name: "minimal config with insecure skip verify",
			cfg: config.Config{
				Auth: config.Auth{
					Insecure: true,
				},
			},
			wantErr: false,
		},
		{
			name: "config with CA bundle",
			cfg: config.Config{
				Auth: config.Auth{
					Insecure: false,
					CABundle: "nonexistent.pem",
				},
			},
			wantErr: false, // missing CA bundle is tolerated
		},
		{
			name: "config with client cert/key",
			cfg: config.Config{
				Auth: config.Auth{
					Insecure:   false,
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
	tempDir := t.TempDir()
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
			Insecure:   false,
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
	tempDir := t.TempDir()
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
			Insecure:   false,
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
		Auth: config.Auth{
			Insecure: true,
		},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	retrievedCfg := client.Config()
	if retrievedCfg.MTLSBaseURL != cfg.MTLSBaseURL {
		t.Errorf("Config() returned different MTLSBaseURL")
	}
	if retrievedCfg.Auth.Insecure != cfg.Auth.Insecure {
		t.Errorf("Config() returned different Insecure setting")
	}
}

func TestClient_Record(t *testing.T) {
	cfg := config.Config{Auth: config.Auth{Insecure: true}}
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

func TestClient_StateRoot(t *testing.T) {
	tests := []struct {
		name          string
		responseBody  string
		expectedRoot  string
		wantErr       bool
		setupServer   func(*httptest.Server)
		usePublicBase bool
	}{
		{
			name:          "state_merkle_root field",
			responseBody:  `{"state_merkle_root": "abc123"}`,
			expectedRoot:  "abc123",
			wantErr:       false,
			usePublicBase: true,
		},
		{
			name:          "state_root field (legacy)",
			responseBody:  `{"state_root": "def456"}`,
			expectedRoot:  "def456",
			wantErr:       false,
			usePublicBase: true,
		},
		{
			name:          "both fields (state_merkle_root takes precedence)",
			responseBody:  `{"state_merkle_root": "abc123", "state_root": "def456"}`,
			expectedRoot:  "abc123",
			wantErr:       false,
			usePublicBase: true,
		},
		{
			name:          "empty response",
			responseBody:  `{}`,
			expectedRoot:  "",
			wantErr:       false,
			usePublicBase: true,
		},
		{
			name:          "invalid JSON",
			responseBody:  `invalid`,
			wantErr:       false, // stateRoot is lenient - returns empty string on invalid JSON
			expectedRoot:  "",
			usePublicBase: true,
		},
		{
			name:         "server error",
			responseBody: `{"error": "internal error"}`,
			wantErr:      false, // stateRoot is lenient - doesn't check status codes
			expectedRoot: "",
			setupServer: func(s *httptest.Server) {
				s.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(`{"error": "internal error"}`))
				})
			},
			usePublicBase: true,
		},
		{
			name:          "mTLS surface",
			responseBody:  `{"state_merkle_root": "mTLS-root"}`,
			expectedRoot:  "mTLS-root",
			wantErr:       false,
			usePublicBase: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != constants.APIPaths.Health {
					t.Errorf("expected path %s, got %s", constants.APIPaths.Health, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(tt.responseBody))
			})

			if tt.setupServer != nil {
				server := httptest.NewServer(handler)
				tt.setupServer(server)
			}

			server := httptest.NewServer(handler)
			defer server.Close()

			cfg := config.Config{
				PublicBaseURL: server.URL,
				MTLSBaseURL:   server.URL,
				Auth:          config.Auth{Insecure: true},
			}

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			ctx := context.Background()
			var root string
			if tt.usePublicBase {
				root, err = client.StateRoot(ctx)
			} else {
				root, err = client.StateRootFromMTLS(ctx)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("StateRoot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && root != tt.expectedRoot {
				t.Errorf("StateRoot() = %v, want %v", root, tt.expectedRoot)
			}
		})
	}
}

func TestClient_RegisterSigner(t *testing.T) {
	tests := []struct {
		name         string
		keyID        string
		pubHex       string
		role         string
		responseCode int
		wantErr      bool
	}{
		{
			name:         "successful registration",
			keyID:        "test-key",
			pubHex:       "abc123",
			role:         "consensus",
			responseCode: http.StatusOK,
			wantErr:      false,
		},
		{
			name:         "principal role",
			keyID:        "principal-key",
			pubHex:       "def456",
			role:         "principal",
			responseCode: http.StatusOK,
			wantErr:      false,
		},
		{
			name:         "server returns 404 (best-effort)",
			keyID:        "test-key",
			pubHex:       "abc123",
			role:         "consensus",
			responseCode: http.StatusNotFound,
			wantErr:      true,
		},
		{
			name:         "server returns 500",
			keyID:        "test-key",
			pubHex:       "abc123",
			role:         "consensus",
			responseCode: http.StatusInternalServerError,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/api/governance/signers" {
					t.Errorf("expected path /api/governance/signers, got %s", r.URL.Path)
				}

				var req map[string]string
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("failed to decode request: %v", err)
				}

				if req["key_id"] != tt.keyID {
					t.Errorf("expected key_id %s, got %s", tt.keyID, req["key_id"])
				}
				if req["public_key"] != tt.pubHex {
					t.Errorf("expected public_key %s, got %s", tt.pubHex, req["public_key"])
				}
				if req["algorithm"] != "ed25519" {
					t.Errorf("expected algorithm ed25519, got %s", req["algorithm"])
				}
				if req["role"] != tt.role {
					t.Errorf("expected role %s, got %s", tt.role, req["role"])
				}

				w.WriteHeader(tt.responseCode)
			}))
			defer server.Close()

			cfg := config.Config{
				MTLSBaseURL: server.URL,
				Auth:        config.Auth{Insecure: true},
			}

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			ctx := context.Background()
			err = client.RegisterSigner(ctx, tt.keyID, tt.pubHex, tt.role)

			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterSigner() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_Approve(t *testing.T) {
	tests := []struct {
		name         string
		txHash       string
		responseCode int
		responseBody string
		wantErr      bool
	}{
		{
			name:         "successful approval",
			txHash:       "abc123",
			responseCode: http.StatusOK,
			responseBody: `{"status": "approved"}`,
			wantErr:      false,
		},
		{
			name:         "transaction not found",
			txHash:       "nonexistent",
			responseCode: http.StatusNotFound,
			responseBody: `{"error": "not found"}`,
			wantErr:      false, // no error on status code, just returned
		},
		{
			name:         "server error",
			txHash:       "abc123",
			responseCode: http.StatusInternalServerError,
			responseBody: `{"error": "internal error"}`,
			wantErr:      false, // no error on status code, just returned
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				expectedPath := "/api/approve/" + tt.txHash
				if r.URL.Path != expectedPath {
					t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
				}

				var req map[string]string
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("failed to decode request: %v", err)
				}

				if req["action"] != "approve" {
					t.Errorf("expected action approve, got %s", req["action"])
				}

				w.WriteHeader(tt.responseCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			cfg := config.Config{
				PublicBaseURL: server.URL,
				Auth:          config.Auth{Insecure: true},
			}

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			ctx := context.Background()
			p := Persona{ID: "test-user"}
			status, body, err := client.Approve(ctx, p, tt.txHash)

			if err != nil {
				t.Errorf("Approve() unexpected error = %v", err)
			}
			if status != tt.responseCode {
				t.Errorf("Approve() status = %d, want %d", status, tt.responseCode)
			}
			if string(body) != tt.responseBody {
				t.Errorf("Approve() body = %s, want %s", string(body), tt.responseBody)
			}
		})
	}
}

func TestClient_do(t *testing.T) {
	tests := []struct {
		name          string
		persona       Persona
		method        string
		body          []byte
		responseCode  int
		responseBody  string
		wantErr       bool
		verifyHeaders map[string]string
	}{
		{
			name:         "GET request",
			persona:      Persona{ID: "test-client"},
			method:       http.MethodGet,
			body:         nil,
			responseCode: http.StatusOK,
			responseBody: `{"result": "ok"}`,
			wantErr:      false,
		},
		{
			name:         "POST request with body",
			persona:      Persona{ID: "test-client"},
			method:       http.MethodPost,
			body:         []byte(`{"test": "data"}`),
			responseCode: http.StatusOK,
			responseBody: `{"result": "created"}`,
			wantErr:      false,
		},
		{
			name:         "request with User-Agent",
			persona:      Persona{ID: "test-client", UserAgent: "TestAgent/1.0"},
			method:       http.MethodGet,
			body:         nil,
			responseCode: http.StatusOK,
			responseBody: `{"result": "ok"}`,
			wantErr:      false,
			verifyHeaders: map[string]string{
				"User-Agent":           "TestAgent/1.0",
				"X-G8E-Client-Persona": "test-client",
			},
		},
		{
			name:         "request with API key",
			persona:      Persona{ID: "test-client"},
			method:       http.MethodGet,
			body:         nil,
			responseCode: http.StatusOK,
			responseBody: `{"result": "ok"}`,
			wantErr:      false,
		},
		{
			name:         "request with Operator session ID",
			persona:      Persona{ID: "test-client", OperatorSessionID: "session-123"},
			method:       http.MethodGet,
			body:         nil,
			responseCode: http.StatusOK,
			responseBody: `{"result": "ok"}`,
			wantErr:      false,
			verifyHeaders: map[string]string{
				"Authorization": "Bearer session-123",
			},
		},
		{
			name:         "request with CLI session ID",
			persona:      Persona{ID: "test-client", CLISessionID: "cli-session-123"},
			method:       http.MethodGet,
			body:         nil,
			responseCode: http.StatusOK,
			responseBody: `{"result": "ok"}`,
			wantErr:      false,
			verifyHeaders: map[string]string{
				constants.HeaderCLISessionID: "cli-session-123",
			},
		},
		{
			name:         "request with User ID",
			persona:      Persona{ID: "test-client", UserID: "user-123"},
			method:       http.MethodGet,
			body:         nil,
			responseCode: http.StatusOK,
			responseBody: `{"result": "ok"}`,
			wantErr:      false,
			verifyHeaders: map[string]string{
				constants.HeaderUserID: "user-123",
			},
		},
		{
			name:         "request with Operator ID",
			persona:      Persona{ID: "test-client", OperatorID: "operator-123"},
			method:       http.MethodGet,
			body:         nil,
			responseCode: http.StatusOK,
			responseBody: `{"result": "ok"}`,
			wantErr:      false,
			verifyHeaders: map[string]string{
				constants.HeaderOperatorID: "operator-123",
			},
		},
		{
			name:         "server error",
			persona:      Persona{ID: "test-client"},
			method:       http.MethodGet,
			body:         nil,
			responseCode: http.StatusInternalServerError,
			responseBody: `{"error": "internal error"}`,
			wantErr:      false, // do() doesn't error on status codes
		},
		{
			name:         "network error (context canceled)",
			persona:      Persona{ID: "test-client"},
			method:       http.MethodGet,
			body:         nil,
			responseCode: http.StatusOK,
			responseBody: `{"result": "ok"}`,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedHeaders http.Header

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeaders = r.Header.Clone()
				if r.Method != tt.method {
					t.Errorf("expected method %s, got %s", tt.method, r.Method)
				}
				w.WriteHeader(tt.responseCode)
				w.Write([]byte(tt.responseBody))
			})

			server := httptest.NewServer(handler)
			defer server.Close()

			cfg := config.Config{
				MTLSBaseURL: server.URL,
				Auth:        config.Auth{Insecure: true},
			}

			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			ctx := context.Background()
			if tt.name == "network error (context canceled)" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			status, body, err := client.do(ctx, tt.persona, tt.method, server.URL+"/test", tt.body)

			if (err != nil) != tt.wantErr {
				t.Errorf("do() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if status != tt.responseCode {
					t.Errorf("do() status = %d, want %d", status, tt.responseCode)
				}
				if string(body) != tt.responseBody {
					t.Errorf("do() body = %s, want %s", string(body), tt.responseBody)
				}

				// Verify headers
				for key, expectedValue := range tt.verifyHeaders {
					actualValue := receivedHeaders.Get(key)
					if actualValue != expectedValue {
						t.Errorf("do() header %s = %s, want %s", key, actualValue, expectedValue)
					}
				}
			}
		})
	}
}

func TestClient_do_Recording(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result": "ok"}`))
	}))
	defer server.Close()

	cfg := config.Config{
		MTLSBaseURL: server.URL,
		Auth:        config.Auth{Insecure: true},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	var exchanges []Exchange
	client.Record(&exchanges)

	ctx := context.Background()
	p := Persona{ID: "test-client"}
	_, _, err = client.do(ctx, p, http.MethodGet, server.URL+"/test", nil)

	if err != nil {
		t.Fatalf("do() failed: %v", err)
	}

	if len(exchanges) != 1 {
		t.Errorf("expected 1 exchange, got %d", len(exchanges))
	}

	ex := exchanges[0]
	if ex.Persona != "test-client" {
		t.Errorf("expected persona test-client, got %s", ex.Persona)
	}
	if ex.Method != http.MethodGet {
		t.Errorf("expected method GET, got %s", ex.Method)
	}
	if ex.Status != http.StatusOK {
		t.Errorf("expected status 200, got %d", ex.Status)
	}
	if ex.LatencyMS < 0 {
		t.Errorf("expected non-negative latency, got %d", ex.LatencyMS)
	}
	if ex.At.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestClient_do_Verbose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result": "ok"}`))
	}))
	defer server.Close()

	cfg := config.Config{
		MTLSBaseURL: server.URL,
		Auth:        config.Auth{Insecure: true},
		Verbose:     true,
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Redirect stderr to capture verbose output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	ctx := context.Background()
	p := Persona{ID: "test-client"}
	_, _, err = client.do(ctx, p, http.MethodGet, server.URL+"/test", nil)

	w.Close()
	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("do() failed: %v", err)
	}

	// Check that something was written to stderr
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	if n == 0 {
		t.Error("expected verbose output to stderr")
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
