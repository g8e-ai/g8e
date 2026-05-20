package tests

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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/services/g8eo/internal/config"
	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/execution"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/listen"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/pubsub"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/sentinel"
	"github.com/g8e-ai/g8e/services/g8eo/internal/testutil"
)

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
		}
	}))
	defer downstreamServer.Close()

	// 2. Setup Operator with MCP configuration
	cfg, err := config.LoadListen(0, 0, 0, 0, dataDir, pkiDir, secretsDir, "localhost", "g8e", "", "", true)
	require.NoError(t, err)
	cfg.Listen.MCPDownstreamURL = downstreamServer.URL

	ls, err := listen.NewListenService(cfg, testutil.NewTestLogger())
	require.NoError(t, err)

	execSvc := execution.NewExecutionService(cfg, testutil.NewTestLogger())
	fileSvc := execution.NewFileEditService(cfg, testutil.NewTestLogger())
	govDeps := ls.GetGovernanceDeps()
	wardenPriv, wardenKeyID, err := ls.GetSecretManager().GetWardenKey()
	require.NoError(t, err)

	// Add Warden key to SignerStore so Implicit L2 signatures from the gateway are trusted
	wardenPub := wardenPriv.Public().(ed25519.PublicKey)
	err = ls.GetDB().AddTrustedSigner(models.TrustedSigner{
		ID:        wardenKeyID,
		PublicKey: hex.EncodeToString(wardenPub),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	})
	require.NoError(t, err)

	mcpGateway := ls.GetHTTPHandler().GetMCPGateway()
	require.NotNil(t, mcpGateway)

	cmdSvc, err := pubsub.NewPubSubCommandService(pubsub.CommandServiceConfig{
		Config:            cfg,
		Logger:            testutil.NewTestLogger(),
		Execution:         execSvc,
		FileEdit:          fileSvc,
		PubSubClient:      pubsub.NewInProcessPubSubClient(ls.GetHTTPHandler().GetPubSubBroker()),
		Sentinel:          sentinel.NewSentinel(services.ProductionSentinelConfig(), testutil.NewTestLogger()),
		ReplayStore:       govDeps.ReplayStore,
		StateRootProvider: govDeps.StateRootProvider,
		TransactionAudit:  govDeps.TransactionAudit,
		SignerStore:       govDeps.SignerStore,
		L3Verifier:        acceptingL3Verifier{},
		WardenSigningKey:  wardenPriv,
		WardenKeyID:       wardenKeyID,
		MCPGateway:        mcpGateway,
	})
	require.NoError(t, err)
	ls.SetEnvelopeProcessor(cmdSvc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ls.Start(ctx)

	require.Eventually(t, func() bool { return ls.IsReady() }, 5*time.Second, 100*time.Millisecond)

	// 3. Setup client identity
	token := "dlk_mcp_test"
	linkData := models.DeviceLinkData{
		Token: token, UserID: "mcp-user", OrganizationID: "mcp-org", MaxUses: 1, Status: "active", ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	linkBytes, _ := json.Marshal(linkData)
	ls.GetDB().KVSet("g8e:device-link:"+token, string(linkBytes), 3600)

	// Register and get cert
	bootstrapURL := fmt.Sprintf("http://localhost:%d", ls.GetBootstrapPort())
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
	hResp, err := http.DefaultClient.Do(hReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, hResp.StatusCode)
	var regResp models.OperatorRegistrationResponse
	json.NewDecoder(hResp.Body).Decode(&regResp)
	hResp.Body.Close()

	// Create mTLS client
	rootPEM, _ := os.ReadFile(filepath.Join(pkiDir, "root", "root_ca.crt"))
	hubPEM, _ := os.ReadFile(filepath.Join(pkiDir, "trust", "hub-bundle.pem"))
	rootPool := x509.NewCertPool()
	rootPool.AppendCertsFromPEM(rootPEM)
	rootPool.AppendCertsFromPEM(hubPEM)
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

	// 5. Test MCP tools/call (Direct, no L3 needed for benign echo)
	// Actually, MCP_CALL is classified as a mutation, so it needs L3 unless we bypass it.
	// In this test environment, acceptingL3Verifier always returns true.
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
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		body, _ := io.ReadAll(resp.Body)
		err = json.Unmarshal(body, &mcpRes)
		require.NoError(t, err)
		if len(mcpRes.Result.Content) == 0 {
			t.Fatalf("empty content in MCP response. Raw body: %s", string(body))
		}
		require.Equal(t, "mcp says hello\n", mcpRes.Result.Content[0].Text)
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
	cfg, err := config.LoadListen(0, 0, 0, 0, dataDir, pkiDir, secretsDir, "localhost", "g8e", "", "", true)
	require.NoError(t, err)
	cfg.Listen.A2ADownstreamURL = downstreamServer.URL

	ls, err := listen.NewListenService(cfg, testutil.NewTestLogger())
	require.NoError(t, err)

	execSvc := execution.NewExecutionService(cfg, testutil.NewTestLogger())
	fileSvc := execution.NewFileEditService(cfg, testutil.NewTestLogger())
	govDeps := ls.GetGovernanceDeps()
	wardenPriv, wardenKeyID, err := ls.GetSecretManager().GetWardenKey()
	require.NoError(t, err)

	// Add Warden key to SignerStore so Implicit L2 signatures from the gateway are trusted
	wardenPub := wardenPriv.Public().(ed25519.PublicKey)
	err = ls.GetDB().AddTrustedSigner(models.TrustedSigner{
		ID:        wardenKeyID,
		PublicKey: hex.EncodeToString(wardenPub),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	})
	require.NoError(t, err)

	mcpGateway := ls.GetHTTPHandler().GetMCPGateway()
	require.NotNil(t, mcpGateway)

	cmdSvc, err := pubsub.NewPubSubCommandService(pubsub.CommandServiceConfig{
		Config:            cfg,
		Logger:            testutil.NewTestLogger(),
		Execution:         execSvc,
		FileEdit:          fileSvc,
		PubSubClient:      pubsub.NewInProcessPubSubClient(ls.GetHTTPHandler().GetPubSubBroker()),
		Sentinel:          sentinel.NewSentinel(services.ProductionSentinelConfig(), testutil.NewTestLogger()),
		ReplayStore:       govDeps.ReplayStore,
		StateRootProvider: govDeps.StateRootProvider,
		TransactionAudit:  govDeps.TransactionAudit,
		SignerStore:       govDeps.SignerStore,
		L3Verifier:        acceptingL3Verifier{},
		WardenSigningKey:  wardenPriv,
		WardenKeyID:       wardenKeyID,
		MCPGateway:        mcpGateway,
	})
	require.NoError(t, err)
	ls.SetEnvelopeProcessor(cmdSvc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ls.Start(ctx)

	require.Eventually(t, func() bool { return ls.IsReady() }, 5*time.Second, 100*time.Millisecond)

	// 3. Setup client identity
	token := "dlk_a2a_test"
	linkData := models.DeviceLinkData{
		Token: token, UserID: "a2a-user", OrganizationID: "a2a-org", MaxUses: 1, Status: "active", ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	linkBytes, _ := json.Marshal(linkData)
	ls.GetDB().KVSet("g8e:device-link:"+token, string(linkBytes), 3600)

	// Register and get cert
	bootstrapURL := fmt.Sprintf("http://localhost:%d", ls.GetBootstrapPort())
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
	hResp, err := http.DefaultClient.Do(hReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, hResp.StatusCode)
	var regResp models.OperatorRegistrationResponse
	json.NewDecoder(hResp.Body).Decode(&regResp)
	hResp.Body.Close()

	// Create mTLS client
	rootPEM, _ := os.ReadFile(filepath.Join(pkiDir, "root", "root_ca.crt"))
	hubPEM, _ := os.ReadFile(filepath.Join(pkiDir, "trust", "hub-bundle.pem"))
	rootPool := x509.NewCertPool()
	rootPool.AppendCertsFromPEM(rootPEM)
	rootPool.AppendCertsFromPEM(hubPEM)
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
		webSess, err := ls.GetHTTPHandler().GetPasskeyService().CreateWebSession("a2a-user")
		require.NoError(t, err)
		cookie := &http.Cookie{Name: "g8e_session", Value: webSess.ID}

		proofReq := map[string]interface{}{
			"id":                "fake-id",
			"rawId":             "fake-id",
			"clientDataJSON":    "fake-data",
			"authenticatorData": "fake-auth",
			"signature":         "fake-sig",
		}
		proofBody, _ := json.Marshal(proofReq)
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/approve/"+txHash+"/verify", bytes.NewReader(proofBody))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		resp, err = mtlsClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var receipt struct {
			ResultSummary string `json:"result_summary"`
		}
		err = json.NewDecoder(resp.Body).Decode(&receipt)
		require.NoError(t, err)
		require.Equal(t, "verified skill execution", receipt.ResultSummary)
	})
}
