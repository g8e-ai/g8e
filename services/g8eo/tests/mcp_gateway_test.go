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
TestMCPGateway_EndToEnd exercises g8eo from the perspective of a standard MCP client
(e.g., Claude Code or a generic AI agent). It verifies the "Universal Protocol Translator"
logic which allows "dumb" clients to be governed by the g8e Gateway without needing
native signing or envelope construction logic.

Practical Coverage:
1. Protocol Translation: Maps JSON-RPC tools/list and tools/call to typed GovernanceEnvelopes.
2. 3-Layer Gauntlet: Forces tool calls through L1 (Hard Gates), L2 (Consensus), and L3 (Approval).
3. Suspension & OOB: Verifies that mutations are suspended, recorded, and only resumed
   after Out-of-Band (OOB) human approval via WebAuthn/Passkey.
4. Downstream Dispatch: Ensures verified payloads are correctly unwrapped and dispatched
   to the real downstream MCP server.
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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/services/g8eo/internal/config"
	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	"github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/execution"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/listen"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/pubsub"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/sentinel"
	"github.com/g8e-ai/g8e/services/g8eo/internal/testutil"
)

type gatewayAcceptingL3Notary struct{}

func (gatewayAcceptingL3Notary) VerifyL3Proof(_ string, _ string, _ string, _ *commonv1.L3Proof) (bool, error) {
	return true, nil
}

func TestMCPGateway_EndToEnd(t *testing.T) {
	dataDir := t.TempDir()
	secretsDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")

	// 1. Setup Mock Downstream MCP Server
	downstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		if req.Method == "tools/list" {
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"echo","description":"echoes input"}]}}`))
		} else if req.Method == "tools/call" {
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"mcp says hello"}]}}`))
		} else if req.Method == "resources/list" {
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"resources":[{"uri":"file:///test.txt","name":"test.txt"}]}}`))
		} else if req.Method == "prompts/list" {
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"prompts":[{"name":"test-prompt","description":"A test prompt"}]}}`))
		}
	}))
	defer downstreamServer.Close()

	// 2. Setup Operator with MCP configuration
	cfg, err := config.LoadListen(config.ListenOptions{
		DataDir:           dataDir,
		PKIDir:            pkiDir,
		SecretsDir:        secretsDir,
		PasskeyRpID:       "localhost",
		PasskeyRpName:     "g8e",
		AllowTestPortZero: true,
	})
	require.NoError(t, err)
	cfg.Listen.MCPDownstreamURL = downstreamServer.URL

	ls, err := listen.NewListenService(cfg, testutil.NewTestLogger())
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
		Sentinel:           sentinel.NewSentinel(services.ProductionSentinelConfig(), testutil.NewTestLogger()),
		ReplayStore:        govDeps.ReplayStore,
		StateRootProvider:  govDeps.StateRootProvider,
		TransactionAudit:   govDeps.TransactionAudit,
		SignerStore:        govDeps.SignerStore,
		L3Notary:           gatewayAcceptingL3Notary{},
		ActuatorSigningKey: ActuatorPriv,
		ActuatorKeyID:      ActuatorKeyID,
		MCPGateway:         mcpGateway,
	})
	require.NoError(t, err)
	ls.SetEnvelopeProcessor(cmdSvc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ls.Start(ctx)

	require.Eventually(t, func() bool { return ls.IsReady() }, 5*time.Second, 100*time.Millisecond)

	// 3. Setup client identity
	token := "dlk_mcp_test"
	userID := "mcp-user"
	linkData := models.DeviceLinkData{
		Token: token, UserID: userID, OrganizationID: "mcp-org", MaxUses: 1, Status: "active", ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	linkBytes, _ := json.Marshal(linkData)
	ls.GetDB().KVSet("g8e:device-link:"+token, string(linkBytes), 3600)

	// Create user with a dummy passkey so VerifyL3Proof passes
	user := models.User{
		ID:     userID,
		Email:  "mcp@test.com",
		Name:   "MCP User",
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

	// Create web session EARLIER so it's part of the state root when the transaction is initiated
	webSess, err := ls.GetHTTPHandler().GetPasskeyService().CreateWebSession(userID)
	require.NoError(t, err)
	cookie := &http.Cookie{Name: "g8e_session", Value: webSess.ID}

	// Register and get cert (bootstrap port serves plain HTTP for trust establishment)
	bootstrapURL := fmt.Sprintf("http://localhost:%d", ls.GetBootstrapPort())
	rootPEM, _ := os.ReadFile(filepath.Join(pkiDir, "root", "root_ca.crt"))
	hubPEM, _ := os.ReadFile(filepath.Join(pkiDir, "trust", "hub-bundle.pem"))
	rootPool := x509.NewCertPool()
	rootPool.AppendCertsFromPEM(rootPEM)
	rootPool.AppendCertsFromPEM(hubPEM)
	bootstrapClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: rootPool}}}

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	csrTmpl := &x509.CertificateRequest{Subject: pkix.Name{CommonName: "mcp-test-client"}}
	csrDER, _ := x509.CreateCertificateRequest(rand.Reader, csrTmpl, priv)
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	regReq := models.OperatorRegistrationRequest{
		CSR:               string(csrPEM),
		SystemFingerprint: "mcp-fingerprint",
		Hostname:          "mcp-host",
	}
	regBody, _ := json.Marshal(regReq)
	hReq, _ := http.NewRequest(http.MethodPost, bootstrapURL+"/api/auth/device-link/register", bytes.NewReader(regBody))
	hReq.Header.Set(constants.HeaderDeviceToken, token)
	hResp, err := bootstrapClient.Do(hReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, hResp.StatusCode)
	var regResp models.OperatorRegistrationResponse
	json.NewDecoder(hResp.Body).Decode(&regResp)
	hResp.Body.Close()

	// Create mTLS client
	privBytes, _ := x509.MarshalPKCS8PrivateKey(priv)
	cert, _ := tls.X509KeyPair([]byte(regResp.OperatorCert), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}))

	mtlsClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{cert},
			},
		},
	}
	mtlsURL := fmt.Sprintf("https://localhost:%d", ls.GetHTTPPort())
	publicURL := fmt.Sprintf("https://localhost:%d", ls.GetPublicPort())

	// 4. Test MCP tools/list
	t.Run("tools/list", func(t *testing.T) {
		resp, err := mtlsClient.Get(mtlsURL + "/api/mcp/v1/tools/list")
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var mcpResp struct {
			Result struct {
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&mcpResp)
		require.NoError(t, err)
		require.Len(t, mcpResp.Result.Tools, 1)
		require.Equal(t, "echo", mcpResp.Result.Tools[0].Name)
	})

	// 4.5 Test MCP resources/list
	t.Run("resources/list", func(t *testing.T) {
		resp, err := mtlsClient.Get(mtlsURL + "/api/mcp/v1/resources/list")
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var mcpResp struct {
			Result struct {
				Resources []struct {
					URI  string `json:"uri"`
					Name string `json:"name"`
				} `json:"resources"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&mcpResp)
		require.NoError(t, err)
		require.Len(t, mcpResp.Result.Resources, 1)
		require.Equal(t, "file:///test.txt", mcpResp.Result.Resources[0].URI)
	})

	// 4.6 Test MCP prompts/list
	t.Run("prompts/list", func(t *testing.T) {
		resp, err := mtlsClient.Get(mtlsURL + "/api/mcp/v1/prompts/list")
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var mcpResp struct {
			Result struct {
				Prompts []struct {
					Name        string `json:"name"`
					Description string `json:"description"`
				} `json:"prompts"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&mcpResp)
		require.NoError(t, err)
		require.Len(t, mcpResp.Result.Prompts, 1)
		require.Equal(t, "test-prompt", mcpResp.Result.Prompts[0].Name)
	})

	// 5. Test MCP tools/call (Direct, no L3 needed for benign echo)
	// Actually, MCP_CALL is classified as a mutation, so it needs L3 unless we bypass it.
	// In this test environment, acceptingL3Notary always returns true.
	t.Run("tools/call", func(t *testing.T) {
		callReq := struct {
			Jsonrpc string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			} `json:"params"`
			ID int `json:"id"`
		}{
			Jsonrpc: "2.0",
			Method:  "tools/call",
			ID:      1,
		}
		callReq.Params.Name = "echo"
		callReq.Params.Arguments = map[string]interface{}{"msg": "hello"}

		reqBody, _ := json.Marshal(callReq)
		resp, err := mtlsClient.Post(mtlsURL+"/api/mcp/v1/tools/call", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var mcpRes struct {
			Result struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		body, _ := io.ReadAll(resp.Body)
		err = json.Unmarshal(body, &mcpRes)
		require.NoError(t, err)

		// MCP tool call returns "Execution paused" because it triggers L3
		require.NotEmpty(t, mcpRes.Result.Content)
		require.Contains(t, mcpRes.Result.Content[0].Text, "Execution paused")

		// Extract txHash from the message
		text := mcpRes.Result.Content[0].Text
		parts := strings.Split(text, "/approve/")
		require.Len(t, parts, 2)
		txHash := strings.Split(parts[1], " ")[0]

		// 6. OOB Approval Flow
		proofReq := map[string]interface{}{
			"id":                "fake-id",
			"rawId":             "fake-id",
			"clientDataJSON":    "fake-data",
			"authenticatorData": "fake-auth",
			"signature":         "fake-sig",
		}
		proofBody, _ := json.Marshal(proofReq)
		hReq, _ := http.NewRequest(http.MethodPost, publicURL+"/api/approve/"+txHash+"/verify", bytes.NewReader(proofBody))
		hReq.Header.Set("Content-Type", "application/json")
		hReq.AddCookie(cookie)

		// Create a separate client for public port that doesn't use mTLS certs
		publicClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: rootPool,
				},
			},
		}
		resp, err = publicClient.Do(hReq)
		require.NoError(t, err)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("OOB approval failed with status %d: %s", resp.StatusCode, string(body))
		}

		var receipt struct {
			ResultSummary string `json:"result_summary"`
		}
		err = json.NewDecoder(resp.Body).Decode(&receipt)
		require.NoError(t, err)
		require.Equal(t, "mcp says hello\n", receipt.ResultSummary)
	})
}

func TestA2AGateway_EndToEnd(t *testing.T) {
	dataDir := t.TempDir()
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
	cfg, err := config.LoadListen(config.ListenOptions{
		DataDir:           dataDir,
		PKIDir:            pkiDir,
		SecretsDir:        secretsDir,
		PasskeyRpID:       "localhost",
		PasskeyRpName:     "g8e",
		AllowTestPortZero: true,
	})
	require.NoError(t, err)
	cfg.Listen.A2ADownstreamURL = downstreamServer.URL

	ls, err := listen.NewListenService(cfg, testutil.NewTestLogger())
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
		Sentinel:           sentinel.NewSentinel(services.ProductionSentinelConfig(), testutil.NewTestLogger()),
		ReplayStore:        govDeps.ReplayStore,
		StateRootProvider:  govDeps.StateRootProvider,
		TransactionAudit:   govDeps.TransactionAudit,
		SignerStore:        govDeps.SignerStore,
		L3Notary:           gatewayAcceptingL3Notary{},
		ActuatorSigningKey: ActuatorPriv,
		ActuatorKeyID:      ActuatorKeyID,
		MCPGateway:         mcpGateway,
	})
	require.NoError(t, err)
	ls.SetEnvelopeProcessor(cmdSvc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ls.Start(ctx)

	require.Eventually(t, func() bool { return ls.IsReady() }, 5*time.Second, 100*time.Millisecond)

	// 3. Setup client identity
	token := "dlk_a2a_test"
	userID := "a2a-user"
	linkData := models.DeviceLinkData{
		Token: token, UserID: userID, OrganizationID: "a2a-org", MaxUses: 1, Status: "active", ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	linkBytes, _ := json.Marshal(linkData)
	ls.GetDB().KVSet("g8e:device-link:"+token, string(linkBytes), 3600)

	// Create user with a dummy passkey so VerifyL3Proof passes
	user := models.User{
		ID:     userID,
		Email:  "a2a@test.com",
		Name:   "A2A User",
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

	// Create web session EARLIER so it's part of the state root when the transaction is initiated
	webSess, err := ls.GetHTTPHandler().GetPasskeyService().CreateWebSession(userID)
	require.NoError(t, err)
	cookie := &http.Cookie{Name: "g8e_session", Value: webSess.ID}

	// Register and get cert (bootstrap port serves plain HTTP for trust establishment)
	bootstrapURL := fmt.Sprintf("http://localhost:%d", ls.GetBootstrapPort())
	rootPEM, _ := os.ReadFile(filepath.Join(pkiDir, "root", "root_ca.crt"))
	hubPEM, _ := os.ReadFile(filepath.Join(pkiDir, "trust", "hub-bundle.pem"))
	rootPool := x509.NewCertPool()
	rootPool.AppendCertsFromPEM(rootPEM)
	rootPool.AppendCertsFromPEM(hubPEM)
	bootstrapClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: rootPool}}}

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	csrTmpl := &x509.CertificateRequest{Subject: pkix.Name{CommonName: "a2a-test-client"}}
	csrDER, _ := x509.CreateCertificateRequest(rand.Reader, csrTmpl, priv)
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	regReq := models.OperatorRegistrationRequest{
		CSR:               string(csrPEM),
		SystemFingerprint: "a2a-fingerprint",
		Hostname:          "a2a-host",
	}
	regBody, _ := json.Marshal(regReq)
	hReq, _ := http.NewRequest(http.MethodPost, bootstrapURL+"/api/auth/device-link/register", bytes.NewReader(regBody))
	hReq.Header.Set(constants.HeaderDeviceToken, token)
	hResp, err := bootstrapClient.Do(hReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, hResp.StatusCode)
	var regResp models.OperatorRegistrationResponse
	json.NewDecoder(hResp.Body).Decode(&regResp)
	hResp.Body.Close()

	// Create mTLS client
	privBytes, _ := x509.MarshalPKCS8PrivateKey(priv)
	cert, _ := tls.X509KeyPair([]byte(regResp.OperatorCert), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}))

	mtlsClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{cert},
			},
		},
	}
	mtlsURL := fmt.Sprintf("https://localhost:%d", ls.GetHTTPPort())
	publicURL := fmt.Sprintf("https://localhost:%d", ls.GetPublicPort())

	// 4. Test A2A Call (Suspends for L3, then Resume)
	t.Run("a2a call", func(t *testing.T) {
		callReq := map[string]interface{}{
			"skill_name": "test-skill",
			"payload":    map[string]string{"foo": "bar"},
		}

		reqBody, _ := json.Marshal(callReq)
		resp, err := mtlsClient.Post(mtlsURL+"/api/a2a/v1/call", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var mcpRes struct {
			Result struct {
				Status      string `json:"status"`
				TxHash      string `json:"tx_hash"`
				ApprovalURL string `json:"approval_url"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&mcpRes)
		require.NoError(t, err)
		require.Equal(t, "suspended", mcpRes.Result.Status)
		txHash := mcpRes.Result.TxHash

		// 5. OOB Approval Flow
		proofReq := map[string]interface{}{
			"id":                "fake-id",
			"rawId":             "fake-id",
			"clientDataJSON":    "fake-data",
			"authenticatorData": "fake-auth",
			"signature":         "fake-sig",
		}
		proofBody, _ := json.Marshal(proofReq)
		req, _ := http.NewRequest(http.MethodPost, publicURL+"/api/approve/"+txHash+"/verify", bytes.NewReader(proofBody))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)

		// Create a separate client for public port that doesn't use mTLS certs
		publicClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: rootPool,
				},
			},
		}
		resp, err = publicClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("OOB approval failed with status %d: %s", resp.StatusCode, string(body))
		}

		var receipt struct {
			ResultSummary string `json:"result_summary"`
		}
		err = json.NewDecoder(resp.Body).Decode(&receipt)
		require.NoError(t, err)
		require.Equal(t, "verified skill execution", receipt.ResultSummary)
	})
}

func TestMCPGateway_PayloadVariations(t *testing.T) {
	dataDir := t.TempDir()
	secretsDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")

	cfg, err := config.LoadListen(config.ListenOptions{
		DataDir:           dataDir,
		PKIDir:            pkiDir,
		SecretsDir:        secretsDir,
		PasskeyRpID:       "localhost",
		PasskeyRpName:     "g8e",
		AllowTestPortZero: true,
	})
	require.NoError(t, err)

	ls, err := listen.NewListenService(cfg, testutil.NewTestLogger())
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
		Sentinel:           sentinel.NewSentinel(services.ProductionSentinelConfig(), testutil.NewTestLogger()),
		ReplayStore:        govDeps.ReplayStore,
		StateRootProvider:  govDeps.StateRootProvider,
		TransactionAudit:   govDeps.TransactionAudit,
		SignerStore:        govDeps.SignerStore,
		L3Notary:           gatewayAcceptingL3Notary{},
		ActuatorSigningKey: ActuatorPriv,
		ActuatorKeyID:      ActuatorKeyID,
		MCPGateway:         mcpGateway,
	})
	require.NoError(t, err)
	ls.SetEnvelopeProcessor(cmdSvc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ls.Start(ctx)

	require.Eventually(t, func() bool { return ls.IsReady() }, 5*time.Second, 100*time.Millisecond)

	token := "dlk_payload_test"
	userID := "payload-user"
	linkData := models.DeviceLinkData{
		Token: token, UserID: userID, OrganizationID: "payload-org", MaxUses: 1, Status: "active", ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	linkBytes, _ := json.Marshal(linkData)
	ls.GetDB().KVSet("g8e:device-link:"+token, string(linkBytes), 3600)

	user := models.User{
		ID:     userID,
		Email:  "payload@test.com",
		Name:   "Payload User",
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

	bootstrapURL := fmt.Sprintf("http://localhost:%d", ls.GetBootstrapPort())
	rootPEM, _ := os.ReadFile(filepath.Join(pkiDir, "root", "root_ca.crt"))
	hubPEM, _ := os.ReadFile(filepath.Join(pkiDir, "trust", "hub-bundle.pem"))
	rootPool := x509.NewCertPool()
	rootPool.AppendCertsFromPEM(rootPEM)
	rootPool.AppendCertsFromPEM(hubPEM)
	bootstrapClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: rootPool}}}

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	csrTmpl := &x509.CertificateRequest{Subject: pkix.Name{CommonName: "payload-test-client"}}
	csrDER, _ := x509.CreateCertificateRequest(rand.Reader, csrTmpl, priv)
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	regReq := models.OperatorRegistrationRequest{
		CSR:               string(csrPEM),
		SystemFingerprint: "payload-fingerprint",
		Hostname:          "payload-host",
	}
	regBody, _ := json.Marshal(regReq)
	hReq, _ := http.NewRequest(http.MethodPost, bootstrapURL+"/api/auth/device-link/register", bytes.NewReader(regBody))
	hReq.Header.Set(constants.HeaderDeviceToken, token)
	hResp, err := bootstrapClient.Do(hReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, hResp.StatusCode)
	var regResp models.OperatorRegistrationResponse
	json.NewDecoder(hResp.Body).Decode(&regResp)
	hResp.Body.Close()

	privBytes, _ := x509.MarshalPKCS8PrivateKey(priv)
	cert, _ := tls.X509KeyPair([]byte(regResp.OperatorCert), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}))

	mtlsClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{cert},
			},
		},
	}
	mtlsURL := fmt.Sprintf("https://localhost:%d", ls.GetHTTPPort())

	t.Run("nested object arguments", func(t *testing.T) {
		callReq := struct {
			Jsonrpc string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			} `json:"params"`
			ID int `json:"id"`
		}{
			Jsonrpc: "2.0",
			Method:  "tools/call",
			ID:      1,
		}
		callReq.Params.Name = "nested_tool"
		callReq.Params.Arguments = map[string]interface{}{
			"config": map[string]interface{}{
				"nested": map[string]interface{}{
					"deep": map[string]interface{}{
						"value": "test",
					},
				},
			},
			"items": []interface{}{"item1", "item2", 123},
		}

		reqBody, _ := json.Marshal(callReq)
		resp, err := mtlsClient.Post(mtlsURL+"/api/mcp/v1/tools/call", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var mcpRes struct {
			Result struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		body, _ := io.ReadAll(resp.Body)
		err = json.Unmarshal(body, &mcpRes)
		require.NoError(t, err)
		require.NotEmpty(t, mcpRes.Result.Content)
	})

	t.Run("unicode and special characters", func(t *testing.T) {
		callReq := struct {
			Jsonrpc string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			} `json:"params"`
			ID int `json:"id"`
		}{
			Jsonrpc: "2.0",
			Method:  "tools/call",
			ID:      1,
		}
		callReq.Params.Name = "unicode_tool"
		callReq.Params.Arguments = map[string]interface{}{
			"text":  "Hello 世界 🌍 \n\t\r\"'\\",
			"emoji": []string{"😀", "🎉", "🚀"},
		}

		reqBody, _ := json.Marshal(callReq)
		resp, err := mtlsClient.Post(mtlsURL+"/api/mcp/v1/tools/call", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("large payload", func(t *testing.T) {
		largeString := strings.Repeat("x", 100000)
		callReq := struct {
			Jsonrpc string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			} `json:"params"`
			ID int `json:"id"`
		}{
			Jsonrpc: "2.0",
			Method:  "tools/call",
			ID:      1,
		}
		callReq.Params.Name = "large_tool"
		callReq.Params.Arguments = map[string]interface{}{
			"data": largeString,
		}

		reqBody, _ := json.Marshal(callReq)
		resp, err := mtlsClient.Post(mtlsURL+"/api/mcp/v1/tools/call", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("empty arguments", func(t *testing.T) {
		callReq := struct {
			Jsonrpc string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"params"`
			ID int `json:"id"`
		}{
			Jsonrpc: "2.0",
			Method:  "tools/call",
			ID:      1,
		}
		callReq.Params.Name = "empty_tool"
		callReq.Params.Arguments = json.RawMessage("{}")

		reqBody, _ := json.Marshal(callReq)
		resp, err := mtlsClient.Post(mtlsURL+"/api/mcp/v1/tools/call", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("null arguments", func(t *testing.T) {
		callReq := struct {
			Jsonrpc string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"params"`
			ID int `json:"id"`
		}{
			Jsonrpc: "2.0",
			Method:  "tools/call",
			ID:      1,
		}
		callReq.Params.Name = "null_tool"
		callReq.Params.Arguments = json.RawMessage("null")

		reqBody, _ := json.Marshal(callReq)
		resp, err := mtlsClient.Post(mtlsURL+"/api/mcp/v1/tools/call", "application/json", bytes.NewReader(reqBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestMCPGateway_ErrorCases(t *testing.T) {
	dataDir := t.TempDir()
	secretsDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")

	cfg, err := config.LoadListen(config.ListenOptions{
		DataDir:           dataDir,
		PKIDir:            pkiDir,
		SecretsDir:        secretsDir,
		PasskeyRpID:       "localhost",
		PasskeyRpName:     "g8e",
		AllowTestPortZero: true,
	})
	require.NoError(t, err)

	ls, err := listen.NewListenService(cfg, testutil.NewTestLogger())
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
		Sentinel:           sentinel.NewSentinel(services.ProductionSentinelConfig(), testutil.NewTestLogger()),
		ReplayStore:        govDeps.ReplayStore,
		StateRootProvider:  govDeps.StateRootProvider,
		TransactionAudit:   govDeps.TransactionAudit,
		SignerStore:        govDeps.SignerStore,
		L3Notary:           gatewayAcceptingL3Notary{},
		ActuatorSigningKey: ActuatorPriv,
		ActuatorKeyID:      ActuatorKeyID,
		MCPGateway:         mcpGateway,
	})
	require.NoError(t, err)
	ls.SetEnvelopeProcessor(cmdSvc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ls.Start(ctx)

	require.Eventually(t, func() bool { return ls.IsReady() }, 5*time.Second, 100*time.Millisecond)

	token := "dlk_error_test"
	userID := "error-user"
	linkData := models.DeviceLinkData{
		Token: token, UserID: userID, OrganizationID: "error-org", MaxUses: 1, Status: "active", ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	linkBytes, _ := json.Marshal(linkData)
	ls.GetDB().KVSet("g8e:device-link:"+token, string(linkBytes), 3600)

	user := models.User{
		ID:     userID,
		Email:  "error@test.com",
		Name:   "Error User",
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

	bootstrapURL := fmt.Sprintf("http://localhost:%d", ls.GetBootstrapPort())
	rootPEM, _ := os.ReadFile(filepath.Join(pkiDir, "root", "root_ca.crt"))
	hubPEM, _ := os.ReadFile(filepath.Join(pkiDir, "trust", "hub-bundle.pem"))
	rootPool := x509.NewCertPool()
	rootPool.AppendCertsFromPEM(rootPEM)
	rootPool.AppendCertsFromPEM(hubPEM)
	bootstrapClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: rootPool}}}

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	csrTmpl := &x509.CertificateRequest{Subject: pkix.Name{CommonName: "error-test-client"}}
	csrDER, _ := x509.CreateCertificateRequest(rand.Reader, csrTmpl, priv)
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	regReq := models.OperatorRegistrationRequest{
		CSR:               string(csrPEM),
		SystemFingerprint: "error-fingerprint",
		Hostname:          "error-host",
	}
	regBody, _ := json.Marshal(regReq)
	hReq, _ := http.NewRequest(http.MethodPost, bootstrapURL+"/api/auth/device-link/register", bytes.NewReader(regBody))
	hReq.Header.Set(constants.HeaderDeviceToken, token)
	hResp, err := bootstrapClient.Do(hReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, hResp.StatusCode)
	var regResp models.OperatorRegistrationResponse
	json.NewDecoder(hResp.Body).Decode(&regResp)
	hResp.Body.Close()

	privBytes, _ := x509.MarshalPKCS8PrivateKey(priv)
	cert, _ := tls.X509KeyPair([]byte(regResp.OperatorCert), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}))

	mtlsClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{cert},
			},
		},
	}
	mtlsURL := fmt.Sprintf("https://localhost:%d", ls.GetHTTPPort())

	t.Run("invalid JSON-RPC version", func(t *testing.T) {
		reqBody := `{"jsonrpc":"1.0","id":1,"method":"tools/call","params":{"name":"test","arguments":{}}}`
		resp, err := mtlsClient.Post(mtlsURL+"/api/mcp/v1/tools/call", "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("missing method", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"params":{"name":"test","arguments":{}}}`
		resp, err := mtlsClient.Post(mtlsURL+"/api/mcp/v1/tools/call", "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("unknown method", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"unknown_method","params":{}}`
		resp, err := mtlsClient.Post(mtlsURL+"/api/mcp/v1/tools/call", "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("malformed JSON", func(t *testing.T) {
		reqBody := `{invalid json`
		resp, err := mtlsClient.Post(mtlsURL+"/api/mcp/v1/tools/call", "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("missing tool name", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"arguments":{}}}`
		resp, err := mtlsClient.Post(mtlsURL+"/api/mcp/v1/tools/call", "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("invalid arguments JSON", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test","arguments":"{invalid}"}}`
		resp, err := mtlsClient.Post(mtlsURL+"/api/mcp/v1/tools/call", "application/json", bytes.NewReader([]byte(reqBody)))
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
