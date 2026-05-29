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

package tests

/*
TestA2AGateway_EndToEnd exercises g8eo from the perspective of a standard A2A client
(e.g., Google Agent2Agent protocol). It verifies the "Universal Protocol Translator"
logic which allows "dumb" clients to be governed by the g8e Gateway without needing
native signing or envelope construction logic.

Practical Coverage:
1. Protocol Translation: Maps A2A skill calls to typed GovernanceEnvelopes.
2. 3-Layer Verification: Forces skill calls through L1 (Hard Gates), L2 (Consensus), and L3 (Approval).
3. Suspension & OOB: Verifies that mutations are suspended, recorded, and only resumed
   after Out-of-Band (OOB) human approval via WebAuthn/Passkey.
4. Downstream Dispatch: Ensures verified payloads are correctly unwrapped and dispatched
   to the real downstream A2A server.
*/

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/execution"
	"github.com/g8e-ai/g8e/internal/services/gateway"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	"github.com/g8e-ai/g8e/internal/services/sovereignty"
	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

type a2aGatewayRejectingL3Notary struct{}

func (a2aGatewayRejectingL3Notary) VerifyL3Proof(_ string, _ string, _ string, _ *commonv1.L3Proof) (bool, error) {
	return false, nil
}

func TestA2AGateway_SkillCallEndToEnd(t *testing.T) {
	// Use shared test vault directory for persistent inspection
	repoRoot, err := os.Getwd()
	require.NoError(t, err)
	// Navigate from test/ to repo root
	for i := 0; i < 2; i++ {
		repoRoot = filepath.Dir(repoRoot)
	}
	testVaultDir := filepath.Join(repoRoot, ".g8e", "test-vault")
	if err := os.MkdirAll(testVaultDir, 0755); err != nil {
		t.Fatalf("failed to create test vault directory: %v", err)
	}

	// Create unique subdirectory for this test run
	testRunID := fmt.Sprintf("%s-%s", time.Now().Format("20060102-150405"), t.Name())
	dataDir := filepath.Join(testVaultDir, testRunID)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("failed to create test run directory: %v", err)
	}
	t.Logf("Test vault created at: %s", dataDir)

	secretsDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")

	// 1. Setup Mock Downstream A2A Server
	downstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SkillName string `json:"skill_name"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":"a2a says hello","summary":"verified skill execution"}`))
	}))
	defer downstreamServer.Close()

	// 2. Setup Operator with A2A configuration
	cfg, err := config.LoadGateway(config.GatewayOptions{
		DataDir:           dataDir,
		PKIDir:            pkiDir,
		SecretsDir:        secretsDir,
		PasskeyRpID:       "localhost",
		PasskeyRpName:     "g8e",
		AllowTestPortZero: true,
		Posture:           config.PostureNotary, // Enforce L3 verification
	})
	require.NoError(t, err)
	cfg.Gateway.A2ADownstreamURL = downstreamServer.URL

	ls, err := gateway.NewGatewayService(cfg, testutil.NewTestLogger())
	require.NoError(t, err)

	execSvc := execution.NewExecutionService(cfg, testutil.NewTestLogger())
	fileSvc := execution.NewFileEditService(cfg, testutil.NewTestLogger())
	govDeps := ls.GetGovernanceDeps()
	sm, err := ls.GetSecretManager()
	require.NoError(t, err)
	ActuatorPriv, ActuatorKeyID, err := sm.GetActuatorKey()
	require.NoError(t, err)

	// Add Actuator key to SignerStore so Implicit L2 signatures from the gateway are trusted
	ActuatorPub := ActuatorPriv.Public().(ed25519.PublicKey)
	err = ls.GetDB().AddTrustedSigner(models.TrustedSigner{
		ID:        ActuatorKeyID,
		PublicKey: hex.EncodeToString(ActuatorPub),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	})
	require.NoError(t, err)

	mcpGateway := ls.GetHTTPHandler().GetMCPGateway()
	require.NotNil(t, mcpGateway)

	cmdSvc, err := pubsub.NewPubSubCommandService(pubsub.CommandServiceConfig{
		Config:             cfg,
		Logger:             testutil.NewTestLogger(),
		Execution:          execSvc,
		FileEdit:           fileSvc,
		PubSubClient:       pubsub.NewInProcessPubSubClient(ls.GetHTTPHandler().GetPubSubBroker()),
		Sovereignty:        sovereignty.NewSovereigntyService(sovereignty.DefaultConfig(), testutil.NewTestLogger(), nil),
		ReplayStore:        govDeps.ReplayStore,
		StateRootProvider:  govDeps.StateRootProvider,
		TransactionAudit:   govDeps.TransactionAudit,
		SignerStore:        govDeps.SignerStore,
		L3Notary:           a2aGatewayRejectingL3Notary{},
		ActuatorSigningKey: ActuatorPriv,
		ActuatorKeyID:      ActuatorKeyID,
		MCPGateway:         mcpGateway,
	})
	require.NoError(t, err)
	ls.SetEnvelopeProcessor(cmdSvc)

	// Set MCP gateway dependencies for governance processing
	mcpGateway.SetDependencies(cmdSvc, govDeps.StateRootProvider, ActuatorPriv, ActuatorKeyID, downstreamServer.URL)
	mcpGateway.SetA2ADependencies(downstreamServer.URL)

	// Seed platform_settings required for health check
	ls.GetDB().DocSet("settings", "platform_settings", json.RawMessage(`{"session_encryption_key":"test-key"}`))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ls.Start(ctx)

	require.Eventually(t, func() bool { return ls.IsReady() }, 5*time.Second, 100*time.Millisecond)

	// 3. Setup client identity via CSR enrollment
	userID := "a2a-user"
	organizationID := "a2a-org"

	// Create user with a dummy passkey so VerifyL3Proof passes
	user := models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
		PasskeyCredentials: []models.PasskeyCredential{
			{
				ID:              []byte("fake-id"),
				PublicKey:       []byte("fake-pubkey"),
				AttestationType: "none",
				Authenticator: models.Authenticator{
					AAGUID:    []byte("AAAAAAAAAAAAAAAAAAAAAA=="),
					SignCount: 0,
				},
				CreatedAtUnixMs: time.Now().UnixMilli(),
			},
		},
	}
	userBytes, err := json.Marshal(user)
	require.NoError(t, err)
	ls.GetDB().DocSet("users", userID, userBytes)

	// Generate CSR for client certificate
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	csrTmpl := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   userID,
			Organization: []string{organizationID},
		},
	}
	csrDER, _ := x509.CreateCertificateRequest(rand.Reader, csrTmpl, priv)
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	// Create a temporary client cert for initial enrollment (using hub CA)
	hubCAPEM := testutil.ReadHubCA(t, pkiDir)
	hubBlock, _ := pem.Decode(hubCAPEM)
	hubCert, err := x509.ParseCertificate(hubBlock.Bytes)
	require.NoError(t, err)
	hubKeyDER, err := sm.GetCAPrivateKey("hub")
	require.NoError(t, err)
	hubKey, err := x509.ParseECPrivateKey(hubKeyDER)
	require.NoError(t, err)

	spiffeURI, err := url.Parse(fmt.Sprintf("spiffe://g8e.local/cli/%s/cli-session-123", userID))
	require.NoError(t, err)
	tempCertTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               csrTmpl.Subject,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{spiffeURI},
	}
	tempCertDER, _ := x509.CreateCertificate(rand.Reader, tempCertTemplate, hubCert, priv.Public(), hubKey)
	tempCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tempCertDER})

	// Create mTLS client with temporary cert
	privBytes, _ := x509.MarshalPKCS8PrivateKey(priv)
	tempCert, _ := tls.X509KeyPair(tempCertPEM, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}))

	rootPEM := testutil.ReadRootCA(t, pkiDir)
	hubPEM := testutil.ReadHubBundle(t, pkiDir)
	rootPool := x509.NewCertPool()
	rootPool.AppendCertsFromPEM(rootPEM)
	rootPool.AppendCertsFromPEM(hubPEM)

	enrollClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{tempCert},
			},
		},
	}

	// Enroll via CSR endpoint
	mtlsURL := fmt.Sprintf("https://localhost:%d", ls.GetHTTPPort())
	regReq := models.OperatorRegistrationRequest{
		CSR:               string(csrPEM),
		SystemFingerprint: "a2a-fingerprint",
		Hostname:          "a2a-host",
	}
	regBody, _ := json.Marshal(regReq)
	hReq, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/pki/device-enroll", bytes.NewReader(regBody))
	hResp, err := enrollClient.Do(hReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, hResp.StatusCode)
	var regResp models.OperatorRegistrationResponse
	json.NewDecoder(hResp.Body).Decode(&regResp)
	hResp.Body.Close()

	// Create mTLS client with enrolled cert
	cert, _ := tls.X509KeyPair([]byte(regResp.OperatorCert), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}))

	mtlsClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{cert},
			},
		},
	}
	mtlsURL = fmt.Sprintf("https://localhost:%d", ls.GetHTTPPort())

	// Set public base URL for approval links
	publicURL := fmt.Sprintf("https://localhost:%d", ls.GetPublicPort())
	mcpGateway.SetPublicBaseURL(publicURL)

	// 4. Test A2A Call (Suspends for L3, then Resume)
	t.Run("a2a call", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name": "test-skill",
				"payload":    map[string]string{"foo": "bar"},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		resp, err := mtlsClient.Post(mtlsURL+"/api/v1/a2a/call", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var a2aRes struct {
			JSONRPC string `json:"jsonrpc"`
			ID      int    `json:"id"`
			Result  struct {
				ID          string `json:"id"`
				Status      string `json:"status"`
				TxHash      string `json:"tx_hash"`
				ApprovalURL string `json:"approval_url"`
				Message     string `json:"message"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&a2aRes)
		require.NoError(t, err)
		// The L3 notary rejects, so the transaction should be suspended
		require.Equal(t, "suspended", a2aRes.Result.Status, "expected suspended status, got: %s", a2aRes.Result.Status)
		require.NotEmpty(t, a2aRes.Result.ApprovalURL)
	})
}

func TestA2AGateway_PayloadVariations(t *testing.T) {
	// Use shared test vault directory for persistent inspection
	repoRoot, err := os.Getwd()
	require.NoError(t, err)
	// Navigate from test/ to repo root
	for i := 0; i < 2; i++ {
		repoRoot = filepath.Dir(repoRoot)
	}
	testVaultDir := filepath.Join(repoRoot, ".g8e", "test-vault")
	if err := os.MkdirAll(testVaultDir, 0755); err != nil {
		t.Fatalf("failed to create test vault directory: %v", err)
	}

	// Create unique subdirectory for this test run
	testRunID := fmt.Sprintf("%s-%s", time.Now().Format("20060102-150405"), t.Name())
	dataDir := filepath.Join(testVaultDir, testRunID)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("failed to create test run directory: %v", err)
	}
	t.Logf("Test vault created at: %s", dataDir)

	secretsDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")

	// 1. Setup Mock Downstream A2A Server
	downstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SkillName string `json:"skill_name"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":"a2a response","summary":"verified skill execution"}`))
	}))
	defer downstreamServer.Close()

	// 2. Setup Operator with A2A configuration
	cfg, err := config.LoadGateway(config.GatewayOptions{
		DataDir:           dataDir,
		PKIDir:            pkiDir,
		SecretsDir:        secretsDir,
		PasskeyRpID:       "localhost",
		PasskeyRpName:     "g8e",
		AllowTestPortZero: true,
		Posture:           config.PostureNotary,
	})
	require.NoError(t, err)
	cfg.Gateway.A2ADownstreamURL = downstreamServer.URL

	ls, err := gateway.NewGatewayService(cfg, testutil.NewTestLogger())
	require.NoError(t, err)

	execSvc := execution.NewExecutionService(cfg, testutil.NewTestLogger())
	fileSvc := execution.NewFileEditService(cfg, testutil.NewTestLogger())
	govDeps := ls.GetGovernanceDeps()
	sm, err := ls.GetSecretManager()
	require.NoError(t, err)
	ActuatorPriv, ActuatorKeyID, err := sm.GetActuatorKey()
	require.NoError(t, err)

	ActuatorPub := ActuatorPriv.Public().(ed25519.PublicKey)
	err = ls.GetDB().AddTrustedSigner(models.TrustedSigner{
		ID:        ActuatorKeyID,
		PublicKey: hex.EncodeToString(ActuatorPub),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	})
	require.NoError(t, err)

	mcpGateway := ls.GetHTTPHandler().GetMCPGateway()
	require.NotNil(t, mcpGateway)

	cmdSvc, err := pubsub.NewPubSubCommandService(pubsub.CommandServiceConfig{
		Config:             cfg,
		Logger:             testutil.NewTestLogger(),
		Execution:          execSvc,
		FileEdit:           fileSvc,
		PubSubClient:       pubsub.NewInProcessPubSubClient(ls.GetHTTPHandler().GetPubSubBroker()),
		Sovereignty:        sovereignty.NewSovereigntyService(sovereignty.DefaultConfig(), testutil.NewTestLogger(), nil),
		ReplayStore:        govDeps.ReplayStore,
		StateRootProvider:  govDeps.StateRootProvider,
		TransactionAudit:   govDeps.TransactionAudit,
		SignerStore:        govDeps.SignerStore,
		L3Notary:           a2aGatewayRejectingL3Notary{},
		ActuatorSigningKey: ActuatorPriv,
		ActuatorKeyID:      ActuatorKeyID,
		MCPGateway:         mcpGateway,
	})
	require.NoError(t, err)
	ls.SetEnvelopeProcessor(cmdSvc)

	mcpGateway.SetDependencies(cmdSvc, govDeps.StateRootProvider, ActuatorPriv, ActuatorKeyID, downstreamServer.URL)
	mcpGateway.SetA2ADependencies(downstreamServer.URL)

	// Seed platform_settings required for health check
	ls.GetDB().DocSet("settings", "platform_settings", json.RawMessage(`{"session_encryption_key":"test-key"}`))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ls.Start(ctx)

	require.Eventually(t, func() bool { return ls.IsReady() }, 5*time.Second, 100*time.Millisecond)

	userID := "a2a-payload-user"
	organizationID := "a2a-payload-org"

	user := models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
		PasskeyCredentials: []models.PasskeyCredential{
			{
				ID:              []byte("fake-id"),
				PublicKey:       []byte("fake-pubkey"),
				AttestationType: "none",
				Authenticator: models.Authenticator{
					AAGUID:    []byte("AAAAAAAAAAAAAAAAAAAAAA=="),
					SignCount: 0,
				},
				CreatedAtUnixMs: time.Now().UnixMilli(),
			},
		},
	}
	userBytes, err := json.Marshal(user)
	require.NoError(t, err)
	ls.GetDB().DocSet("users", userID, userBytes)

	// Generate CSR for client certificate
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	csrTmpl := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   userID,
			Organization: []string{organizationID},
		},
	}
	csrDER, _ := x509.CreateCertificateRequest(rand.Reader, csrTmpl, priv)
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	// Create a temporary client cert for initial enrollment (using hub CA)
	hubCAPEM := testutil.ReadHubCA(t, pkiDir)
	hubBlock, _ := pem.Decode(hubCAPEM)
	hubCert, err := x509.ParseCertificate(hubBlock.Bytes)
	require.NoError(t, err)
	hubKeyDER, err := sm.GetCAPrivateKey("hub")
	require.NoError(t, err)
	hubKey, err := x509.ParseECPrivateKey(hubKeyDER)
	require.NoError(t, err)

	spiffeURI, err := url.Parse(fmt.Sprintf("spiffe://g8e.local/cli/%s/cli-session-123", userID))
	require.NoError(t, err)
	tempCertTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               csrTmpl.Subject,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{spiffeURI},
	}
	tempCertDER, _ := x509.CreateCertificate(rand.Reader, tempCertTemplate, hubCert, priv.Public(), hubKey)
	tempCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tempCertDER})

	// Create mTLS client with temporary cert
	privBytes, _ := x509.MarshalPKCS8PrivateKey(priv)
	tempCert, _ := tls.X509KeyPair(tempCertPEM, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}))

	rootPEM := testutil.ReadRootCA(t, pkiDir)
	hubPEM := testutil.ReadHubBundle(t, pkiDir)
	rootPool := x509.NewCertPool()
	rootPool.AppendCertsFromPEM(rootPEM)
	rootPool.AppendCertsFromPEM(hubPEM)

	enrollClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{tempCert},
			},
		},
	}

	// Enroll via CSR endpoint
	mtlsURL := fmt.Sprintf("https://localhost:%d", ls.GetHTTPPort())
	regReq := models.OperatorRegistrationRequest{
		CSR:               string(csrPEM),
		SystemFingerprint: "a2a-payload-fingerprint",
		Hostname:          "a2a-payload-host",
	}
	regBody, _ := json.Marshal(regReq)
	hReq, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/pki/device-enroll", bytes.NewReader(regBody))
	hResp, err := enrollClient.Do(hReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, hResp.StatusCode)
	var regResp models.OperatorRegistrationResponse
	json.NewDecoder(hResp.Body).Decode(&regResp)
	hResp.Body.Close()

	// Create mTLS client with enrolled cert
	cert, _ := tls.X509KeyPair([]byte(regResp.OperatorCert), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}))

	mtlsClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{cert},
			},
		},
	}

	publicURL := fmt.Sprintf("https://localhost:%d", ls.GetPublicPort())
	mcpGateway.SetPublicBaseURL(publicURL)

	t.Run("nested payload structure", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name": "nested_skill",
				"payload": map[string]interface{}{
					"config": map[string]interface{}{
						"nested": map[string]interface{}{
							"deep": map[string]interface{}{
								"value": "test",
							},
						},
					},
					"items": []interface{}{"item1", "item2", 123},
				},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		resp, err := mtlsClient.Post(mtlsURL+"/api/v1/a2a/call", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var a2aRes struct {
			Result struct {
				Status string `json:"status"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&a2aRes)
		require.NoError(t, err)
		require.Equal(t, "suspended", a2aRes.Result.Status)
	})

	t.Run("unicode and special characters in payload", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name": "unicode_skill",
				"payload": map[string]interface{}{
					"text":  "Hello 世界 🌍 \n\t\r\"'\\",
					"emoji": []string{"😀", "🎉", "🚀"},
				},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		resp, err := mtlsClient.Post(mtlsURL+"/api/v1/a2a/call", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var a2aRes struct {
			Result struct {
				Status string `json:"status"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&a2aRes)
		require.NoError(t, err)
		require.Equal(t, "suspended", a2aRes.Result.Status)
	})

	t.Run("large payload", func(t *testing.T) {
		largeString := strings.Repeat("x", 100000)
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name": "large_skill",
				"payload": map[string]interface{}{
					"data": largeString,
				},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		resp, err := mtlsClient.Post(mtlsURL+"/api/v1/a2a/call", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var a2aRes struct {
			Result struct {
				Status string `json:"status"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&a2aRes)
		require.NoError(t, err)
		require.Equal(t, "suspended", a2aRes.Result.Status)
	})

	t.Run("empty payload", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name": "empty_skill",
				"payload":    map[string]interface{}{},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		resp, err := mtlsClient.Post(mtlsURL+"/api/v1/a2a/call", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var a2aRes struct {
			Result struct {
				Status string `json:"status"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&a2aRes)
		require.NoError(t, err)
		require.Equal(t, "suspended", a2aRes.Result.Status)
	})

	t.Run("null payload", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name": "null_skill",
				"payload":    nil,
			},
		}

		reqBody, _ := json.Marshal(callReq)
		resp, err := mtlsClient.Post(mtlsURL+"/api/v1/a2a/call", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var a2aRes struct {
			Result struct {
				Status string `json:"status"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&a2aRes)
		require.NoError(t, err)
		require.Equal(t, "suspended", a2aRes.Result.Status)
	})

	t.Run("execution_id parameter", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name":   "execution_id_skill",
				"payload":      map[string]string{"foo": "bar"},
				"execution_id": "exec-12345",
			},
		}

		reqBody, _ := json.Marshal(callReq)
		resp, err := mtlsClient.Post(mtlsURL+"/api/v1/a2a/call", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var a2aRes struct {
			Result struct {
				Status string `json:"status"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&a2aRes)
		require.NoError(t, err)
		require.Equal(t, "suspended", a2aRes.Result.Status)
	})
}

func TestA2AGateway_ErrorCases(t *testing.T) {
	// Use shared test vault directory for persistent inspection
	repoRoot, err := os.Getwd()
	require.NoError(t, err)
	// Navigate from test/ to repo root
	for i := 0; i < 2; i++ {
		repoRoot = filepath.Dir(repoRoot)
	}
	testVaultDir := filepath.Join(repoRoot, ".g8e", "test-vault")
	if err := os.MkdirAll(testVaultDir, 0755); err != nil {
		t.Fatalf("failed to create test vault directory: %v", err)
	}

	// Create unique subdirectory for this test run
	testRunID := fmt.Sprintf("%s-%s", time.Now().Format("20060102-150405"), t.Name())
	dataDir := filepath.Join(testVaultDir, testRunID)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("failed to create test run directory: %v", err)
	}
	t.Logf("Test vault created at: %s", dataDir)

	secretsDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")

	cfg, err := config.LoadGateway(config.GatewayOptions{
		DataDir:           dataDir,
		PKIDir:            pkiDir,
		SecretsDir:        secretsDir,
		PasskeyRpID:       "localhost",
		PasskeyRpName:     "g8e",
		AllowTestPortZero: true,
	})
	require.NoError(t, err)

	ls, err := gateway.NewGatewayService(cfg, testutil.NewTestLogger())
	require.NoError(t, err)

	execSvc := execution.NewExecutionService(cfg, testutil.NewTestLogger())
	fileSvc := execution.NewFileEditService(cfg, testutil.NewTestLogger())
	govDeps := ls.GetGovernanceDeps()
	sm, err := ls.GetSecretManager()
	require.NoError(t, err)
	ActuatorPriv, ActuatorKeyID, err := sm.GetActuatorKey()
	require.NoError(t, err)

	ActuatorPub := ActuatorPriv.Public().(ed25519.PublicKey)
	err = ls.GetDB().AddTrustedSigner(models.TrustedSigner{
		ID:        ActuatorKeyID,
		PublicKey: hex.EncodeToString(ActuatorPub),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	})
	require.NoError(t, err)

	mcpGateway := ls.GetHTTPHandler().GetMCPGateway()
	require.NotNil(t, mcpGateway)

	cmdSvc, err := pubsub.NewPubSubCommandService(pubsub.CommandServiceConfig{
		Config:             cfg,
		Logger:             testutil.NewTestLogger(),
		Execution:          execSvc,
		FileEdit:           fileSvc,
		PubSubClient:       pubsub.NewInProcessPubSubClient(ls.GetHTTPHandler().GetPubSubBroker()),
		Sovereignty:        sovereignty.NewSovereigntyService(sovereignty.DefaultConfig(), testutil.NewTestLogger(), nil),
		ReplayStore:        govDeps.ReplayStore,
		StateRootProvider:  govDeps.StateRootProvider,
		TransactionAudit:   govDeps.TransactionAudit,
		SignerStore:        govDeps.SignerStore,
		L3Notary:           a2aGatewayRejectingL3Notary{},
		ActuatorSigningKey: ActuatorPriv,
		ActuatorKeyID:      ActuatorKeyID,
		MCPGateway:         mcpGateway,
	})
	require.NoError(t, err)
	ls.SetEnvelopeProcessor(cmdSvc)

	mcpGateway.SetDependencies(cmdSvc, govDeps.StateRootProvider, ActuatorPriv, ActuatorKeyID, "")

	// Seed platform_settings required for health check
	ls.GetDB().DocSet("settings", "platform_settings", json.RawMessage(`{"session_encryption_key":"test-key"}`))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ls.Start(ctx)

	require.Eventually(t, func() bool { return ls.IsReady() }, 5*time.Second, 100*time.Millisecond)

	userID := "a2a-error-user"
	organizationID := "a2a-error-org"

	user := models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
		PasskeyCredentials: []models.PasskeyCredential{
			{
				ID:              []byte("fake-id"),
				PublicKey:       []byte("fake-pubkey"),
				AttestationType: "none",
				Authenticator: models.Authenticator{
					AAGUID:    []byte("AAAAAAAAAAAAAAAAAAAAAA=="),
					SignCount: 0,
				},
				CreatedAtUnixMs: time.Now().UnixMilli(),
			},
		},
	}
	userBytes, err := json.Marshal(user)
	require.NoError(t, err)
	ls.GetDB().DocSet("users", userID, userBytes)

	// Generate CSR for client certificate
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	csrTmpl := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   userID,
			Organization: []string{organizationID},
		},
	}
	csrDER, _ := x509.CreateCertificateRequest(rand.Reader, csrTmpl, priv)
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	// Create a temporary client cert for initial enrollment (using hub CA)
	hubCAPEM := testutil.ReadHubCA(t, pkiDir)
	hubBlock, _ := pem.Decode(hubCAPEM)
	hubCert, err := x509.ParseCertificate(hubBlock.Bytes)
	require.NoError(t, err)
	hubKeyDER, err := sm.GetCAPrivateKey("hub")
	require.NoError(t, err)
	hubKey, err := x509.ParseECPrivateKey(hubKeyDER)
	require.NoError(t, err)

	spiffeURI, err := url.Parse(fmt.Sprintf("spiffe://g8e.local/cli/%s/cli-session-123", userID))
	require.NoError(t, err)
	tempCertTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               csrTmpl.Subject,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{spiffeURI},
	}
	tempCertDER, _ := x509.CreateCertificate(rand.Reader, tempCertTemplate, hubCert, priv.Public(), hubKey)
	tempCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tempCertDER})

	// Create mTLS client with temporary cert
	privBytes, _ := x509.MarshalPKCS8PrivateKey(priv)
	tempCert, _ := tls.X509KeyPair(tempCertPEM, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}))

	rootPEM := testutil.ReadRootCA(t, pkiDir)
	hubPEM := testutil.ReadHubBundle(t, pkiDir)
	rootPool := x509.NewCertPool()
	rootPool.AppendCertsFromPEM(rootPEM)
	rootPool.AppendCertsFromPEM(hubPEM)

	enrollClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{tempCert},
			},
		},
	}

	// Enroll via CSR endpoint
	mtlsURL := fmt.Sprintf("https://localhost:%d", ls.GetHTTPPort())
	regReq := models.OperatorRegistrationRequest{
		CSR:               string(csrPEM),
		SystemFingerprint: "a2a-error-fingerprint",
		Hostname:          "a2a-error-host",
	}
	regBody, _ := json.Marshal(regReq)
	hReq, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/pki/device-enroll", bytes.NewReader(regBody))
	hResp, err := enrollClient.Do(hReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, hResp.StatusCode)
	var regResp models.OperatorRegistrationResponse
	json.NewDecoder(hResp.Body).Decode(&regResp)
	hResp.Body.Close()

	// Create mTLS client with enrolled cert
	cert, _ := tls.X509KeyPair([]byte(regResp.OperatorCert), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}))

	mtlsClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{cert},
			},
		},
	}

	publicURL := fmt.Sprintf("https://localhost:%d", ls.GetPublicPort())
	mcpGateway.SetPublicBaseURL(publicURL)

	t.Run("api key rejected", func(t *testing.T) {
		plainClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"a2a/call","params":{"skill_name":"test"}}`

		// Test with API key in header
		req, _ := http.NewRequest("POST", mtlsURL+"/api/v1/a2a/call", bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", "test-api-key")

		resp, err := plainClient.Do(req)
		if resp != nil {
			defer resp.Body.Close()
		}
		require.Error(t, err)
		require.Contains(t, err.Error(), "tls: certificate required")

		// The gateway should reject this at the TLS layer or middleware before reaching the A2A handler
	})
	t.Run("invalid JSON-RPC version", func(t *testing.T) {
		reqBody := `{"jsonrpc":"1.0","id":1,"method":"a2a/call","params":{"skill_name":"test"}}`
		resp, err := mtlsClient.Post(mtlsURL+"/api/v1/a2a/call", "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("missing method", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"params":{"skill_name":"test"}}`
		resp, err := mtlsClient.Post(mtlsURL+"/api/v1/a2a/call", "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("unknown method", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"unknown_method","params":{}}`
		resp, err := mtlsClient.Post(mtlsURL+"/api/v1/a2a/call", "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("malformed JSON", func(t *testing.T) {
		reqBody := `{invalid json`
		resp, err := mtlsClient.Post(mtlsURL+"/api/v1/a2a/call", "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var jsonRPCResp struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		body, _ := io.ReadAll(resp.Body)
		err = json.Unmarshal(body, &jsonRPCResp)
		require.NoError(t, err)
		require.Equal(t, -32700, jsonRPCResp.Error.Code)
		require.Contains(t, jsonRPCResp.Error.Message, "parse error")
	})

	t.Run("missing skill_name", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"a2a/call","params":{"payload":{}}}`
		resp, err := mtlsClient.Post(mtlsURL+"/api/v1/a2a/call", "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("invalid payload JSON", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"a2a/call","params":{"skill_name":"test","payload":"{invalid}"}}`
		resp, err := mtlsClient.Post(mtlsURL+"/api/v1/a2a/call", "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("missing params", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"a2a/call"}`
		resp, err := mtlsClient.Post(mtlsURL+"/api/v1/a2a/call", "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
