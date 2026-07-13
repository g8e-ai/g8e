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
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAPIClient implements apiClient for testing.
type mockAPIClient struct {
	getResp   []byte
	getErr    error
	postResp  []byte
	postErr   error
	getCalls  []string
	postCalls []mockPostCall
}

type mockPostCall struct {
	path string
	body interface{}
}

func (m *mockAPIClient) Get(path string) ([]byte, error) {
	m.getCalls = append(m.getCalls, path)
	return m.getResp, m.getErr
}

func (m *mockAPIClient) Post(path string, body interface{}) ([]byte, error) {
	m.postCalls = append(m.postCalls, mockPostCall{path: path, body: body})
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
	tmpDir := testutil.TempDir(t)
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

// setupApproveSSETestEnv extends setupApproveAPITestEnv by writing a valid CA
// cert to the trust bundle path so auth.BuildMTLSClient can succeed.
func setupApproveSSETestEnv(t *testing.T) (*config.Config, ed25519.PrivateKey) {
	t.Helper()
	cfg, priv := setupApproveAPITestEnv(t)

	certPEM, err := os.ReadFile(cfg.CLICertFile())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfg.TrustBundlePath(), certPEM, 0o600))

	return cfg, priv
}

// sseApproveServer returns an httptest.Server that serves a single
// approval.completed SSE event with the given userID and txHash.
func sseApproveServer(t *testing.T, userID, txHash string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		eventPayload, err := json.Marshal(models.ApprovalCompletedEvent{
			Type:   constants.SSEEventTypeApprovalCompleted,
			UserID: userID,
			TxHash: txHash,
		})
		require.NoError(t, err)
		envelope := models.SSEPushPayload{
			UserID: userID,
			Event:  eventPayload,
		}
		envelopeJSON, err := json.Marshal(envelope)
		require.NoError(t, err)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", constants.SSEEventTypeApprovalCompleted, string(envelopeJSON))
	}))
}

// sseNoEventServer returns an httptest.Server that accepts SSE connections
// but never sends any events, causing the client to block until context cancel.
// The server registers its own cleanup to ensure the handler unblocks before
// Close is called, which is necessary on Windows where the TCP stack does not
// promptly notify the server of client-side connection closure.
func sseNoEventServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		select {
		case <-r.Context().Done():
		case <-ctx.Done():
		}
	}))
	t.Cleanup(func() {
		cancel()
		srv.Close()
	})
	return srv
}

// withEndpointOverride sets the config endpoint override to the given URL and
// resets it on cleanup.
func withEndpointOverride(t *testing.T, url string) {
	t.Helper()
	config.SetEndpointOverride(url)
	t.Cleanup(func() { config.SetEndpointOverride("") })
}

func TestApproveCmd_SSE_HappyPath(t *testing.T) {
	cfg, _ := setupApproveSSETestEnv(t)

	srv := sseApproveServer(t, "user-test", "txhash123")
	t.Cleanup(srv.Close)
	withEndpointOverride(t, srv.URL)

	mockClient := &mockAPIClient{
		getResp: []byte(`{"status":"approved","result_summary":"success"}`),
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
	assert.Contains(t, buf.String(), "txhash123")
}

func TestApproveCmd_SSE_Timeout(t *testing.T) {
	cfg, _ := setupApproveSSETestEnv(t)

	srv := sseNoEventServer(t)
	withEndpointOverride(t, srv.URL)

	mockClient := &mockAPIClient{
		getResp: []byte(`{"status":"approved"}`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(*config.Config) (apiClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, []string{"txhash123"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestApproveCmd_SSE_Success_GetError(t *testing.T) {
	cfg, _ := setupApproveSSETestEnv(t)

	srv := sseApproveServer(t, "user-test", "txhash123")
	t.Cleanup(srv.Close)
	withEndpointOverride(t, srv.URL)

	mockClient := &mockAPIClient{
		getErr: errors.New("network failure"),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(*config.Config) (apiClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash123"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify status")
}

func TestApproveCmd_SSE_Success_InvalidJSONStatus(t *testing.T) {
	cfg, _ := setupApproveSSETestEnv(t)

	srv := sseApproveServer(t, "user-test", "txhash123")
	t.Cleanup(srv.Close)
	withEndpointOverride(t, srv.URL)

	mockClient := &mockAPIClient{
		getResp: []byte(`not json {{{`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(*config.Config) (apiClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash123"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse status response")
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

func TestApproveCmd_SSE_Success_EmptyStatus(t *testing.T) {
	cfg, _ := setupApproveSSETestEnv(t)

	srv := sseApproveServer(t, "user-test", "txhash456")
	t.Cleanup(srv.Close)
	withEndpointOverride(t, srv.URL)

	mockClient := &mockAPIClient{
		getResp: []byte(`{}`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(*config.Config) (apiClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash456"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status")
	assert.Contains(t, buf.String(), "txhash456")
}

func TestApproveCmd_SSE_Success_StatusNotApproved(t *testing.T) {
	cfg, _ := setupApproveSSETestEnv(t)

	srv := sseApproveServer(t, "user-test", "txhash789")
	t.Cleanup(srv.Close)
	withEndpointOverride(t, srv.URL)

	mockClient := &mockAPIClient{
		getResp: []byte(`{"status":"pending"}`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(*config.Config) (apiClient, error) { return mockClient, nil }

	cmd := approveCmdWithConfig(loader, factory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash789"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status")
}

func TestApproveCmd_NoCredentials_Error(t *testing.T) {
	cfg, _ := setupApproveAPITestEnv(t)
	require.NoError(t, os.Remove(cfg.CredentialsFile()))

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(*config.Config) (apiClient, error) { return &mockAPIClient{}, nil }

	cmd := approveCmdWithConfig(loader, factory)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"txhash123"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
}

func generateApproveTestCertDER(t *testing.T, priv ed25519.PrivateKey) []byte {
	t.Helper()
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, priv.Public(), priv)
	require.NoError(t, err)
	return certBytes
}
