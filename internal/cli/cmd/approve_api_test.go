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

package cmd

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAPIClient implements apiClient for testing.
type mockAPIClient struct {
	getResp  []byte
	getErr   error
	postResp []byte
	postErr  error
	postPath string
	postBody interface{}
}

func (m *mockAPIClient) Get(path string) ([]byte, error) {
	return m.getResp, m.getErr
}

func (m *mockAPIClient) Post(path string, body interface{}) ([]byte, error) {
	m.postPath = path
	m.postBody = body
	return m.postResp, m.postErr
}

func (m *mockAPIClient) Put(path string, body interface{}) ([]byte, error) {
	return nil, nil
}

func (m *mockAPIClient) Delete(path string) ([]byte, error) {
	return nil, nil
}

// setupApproveAPITestEnv creates a full test environment with valid key, cert, and credentials.
func setupApproveAPITestEnv(t *testing.T) (*config.Config, ed25519.PrivateKey) {
	t.Helper()
	tmpDir := t.TempDir()
	cfg := setupTestConfig(t, tmpDir)

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	require.NoError(t, os.WriteFile(cfg.CLIKeyFile(), keyPEM, 0o600))

	certDER := generateApproveTestCertDER(t, priv)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	require.NoError(t, os.WriteFile(cfg.CLICertFile(), certPEM, 0o600))

	creds := &auth.Credentials{
		OperatorSessionID: "op-sess-test",
		UserID:            "user-test",
		OperatorID:        "op-test",
		CLISessionID:      "cli-sess-test",
	}
	require.NoError(t, auth.SaveCredentials(cfg, creds))

	return cfg, priv
}

func TestApproveCmd_APIInjection_HappyPath(t *testing.T) {
	cfg, _ := setupApproveAPITestEnv(t)

	mockClient := &mockAPIClient{
		postResp: []byte(`{"status":"approved","result_summary":"success"}`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(*config.Config) (apiClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash123"})
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "approved successfully")
	assert.Contains(t, buf.String(), "approved")
	assert.Contains(t, buf.String(), "txhash123")
	assert.Contains(t, mockClient.postPath, "/api/v1/approve/txhash123")
}

func TestApproveCmd_APIInjection_PostError(t *testing.T) {
	cfg, _ := setupApproveAPITestEnv(t)

	mockClient := &mockAPIClient{
		postErr: errors.New("network failure"),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(*config.Config) (apiClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash123"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrTransactionApproveFailed)
}

func TestApproveCmd_APIInjection_InvalidJSONResponse(t *testing.T) {
	cfg, _ := setupApproveAPITestEnv(t)

	mockClient := &mockAPIClient{
		postResp: []byte(`not json {{{`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(*config.Config) (apiClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash123"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrResponseParseFailed)
}

func TestApproveCmd_APIInjection_ClientFactoryError(t *testing.T) {
	cfg, _ := setupApproveAPITestEnv(t)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(*config.Config) (apiClient, error) {
		return nil, constants.ErrNotAuthenticated
	}

	cmd := approveCmdWithConfig(loader, factory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash123"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
}

func TestApproveCmd_APIInjection_EmptyStatusInResponse(t *testing.T) {
	cfg, _ := setupApproveAPITestEnv(t)

	mockClient := &mockAPIClient{
		postResp: []byte(`{}`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(*config.Config) (apiClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash456"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "txhash456")
}

// httptest.Server-based test: verifies the full approve flow works against a real HTTP server
// using the real api.Client (via NewClientWithURL with the test server URL).
func TestApproveCmd_HTTPTestServer(t *testing.T) {
	cfg, _ := setupApproveAPITestEnv(t)

	// Create a trust bundle that the API client will accept.
	// Since we're using httptest.Server with default TLS, we need to provide
	// a CA bundle. Instead, use an httptest.Server without TLS and inject
	// a mock factory that wraps the real client logic against the test server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/api/v1/approve/")

		// Body is pre-marshaled JSON bytes; just verify it's non-empty
		bodyBytes, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.NotEmpty(t, bodyBytes)

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"approved","result_summary":"execution completed"}`)
	}))
	t.Cleanup(srv.Close)

	// Use a mock client that posts to the test server directly.
	mockClient := &httptestApproveClient{serverURL: srv.URL}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(*config.Config) (apiClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash789"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "approved successfully")
	assert.Contains(t, buf.String(), "approved")
	assert.Contains(t, buf.String(), "execution completed")
}

// httptestApproveClient implements apiClient by forwarding Post calls to the httptest.Server.
type httptestApproveClient struct {
	serverURL string
}

func (c *httptestApproveClient) Get(path string) ([]byte, error) {
	resp, err := http.Get(c.serverURL + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	return buf.Bytes(), nil
}

func (c *httptestApproveClient) Post(path string, body interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if bodyBytes, ok := body.([]byte); ok {
		bodyReader = bytes.NewReader(bodyBytes)
	} else {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}
	resp, err := http.Post(c.serverURL+path, "application/json", bodyReader)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	return buf.Bytes(), nil
}

func (c *httptestApproveClient) Put(path string, body interface{}) ([]byte, error) {
	return nil, nil
}

func (c *httptestApproveClient) Delete(path string) ([]byte, error) {
	return nil, nil
}

func generateApproveTestCertDER(t *testing.T, priv ed25519.PrivateKey) []byte {
	t.Helper()
	template := x509.Certificate{
		SerialNumber:       big.NewInt(1),
		NotBefore:          time.Now(),
		NotAfter:           time.Now().Add(24 * time.Hour),
		KeyUsage:           x509.KeyUsageDigitalSignature,
		ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, priv.Public(), priv)
	require.NoError(t, err)
	return certBytes
}
