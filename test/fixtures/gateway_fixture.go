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

//go:build integration

package fixtures

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/execution"
	"github.com/g8e-ai/g8e/internal/services/gateway"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

// GatewayFixture provides a reusable gateway setup for integration tests.
// It encapsulates the common pattern of creating a gateway, downstream server,
// and all required dependencies.
type GatewayFixture struct {
	Config           *config.Config
	Service          *gateway.GatewayModeService
	DownstreamServer *httptest.Server
	DataDir          string
	SecretsDir       string
	PKIDir           string
	MCPGateway       *mcp.GatewayService
	ActuatorPriv     ed25519.PrivateKey
	ActuatorKeyID    string
	Cleanup          func()
}

// GatewayFixtureOptions configures the gateway fixture.
type GatewayFixtureOptions struct {
	TestName          string
	Posture           config.GatewayPosture
	DownstreamURL     string // If empty, creates a mock server
	AllowTestPortZero bool
}

// NewGatewayFixture creates a fully configured gateway fixture for testing.
// It handles:
// - Path initialization and test data directory creation
// - Mock downstream MCP server (if no URL provided)
// - Gateway configuration and service initialization
// - Execution and file edit services
// - Governance dependencies and actuator key setup
// - MCP gateway dependency wiring
//
// The returned fixture includes a Cleanup function that should be called
// in a defer statement to clean up resources.
func NewGatewayFixture(t *testing.T, opts GatewayFixtureOptions) *GatewayFixture {
	t.Helper()

	// Create test paths without mutating global constants.Paths
	testPaths := testutil.NewTestPathsFromTemp(t)

	// Create unique subdirectory for this test run
	testRunID := fmt.Sprintf("%s-%s", time.Now().Format("20060102-150405"), opts.TestName)
	dataDir := filepath.Join(testPaths.TestVaultDir, testRunID)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("gateway_fixture: create test run directory: %v", err)
	}
	t.Logf("Test vault created at: %s", dataDir)

	secretsDir := testPaths.SecretsDir
	pkiDir := filepath.Join(dataDir, constants.PkiDirname)

	var downstreamServer *httptest.Server
	var downstreamURL string

	if opts.DownstreamURL == "" {
		// Create default mock downstream MCP server
		downstreamServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Method string `json:"method"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			switch req.Method {
			case "tools/list":
				resp := mcp.JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      1,
					Result:  mustMarshal(mcp.ToolsListResult{Tools: []mcp.Tool{{Name: "echo", Description: "echoes input"}}}),
				}
				if err := json.NewEncoder(w).Encode(resp); err != nil {
					t.Logf("Failed to encode response: %v", err)
				}
			case "tools/call":
				resp := mcp.JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      1,
					Result:  mustMarshal(mcp.CallToolResult{Content: []mcp.TextContent{{Type: "text", Text: "mcp says hello"}}}),
				}
				if err := json.NewEncoder(w).Encode(resp); err != nil {
					t.Logf("Failed to encode response: %v", err)
				}
			case "resources/list":
				resp := mcp.JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      1,
					Result:  mustMarshal(mcp.ListResourcesResult{Resources: []mcp.Resource{{URI: "file:///test.txt", Name: "test.txt"}}}),
				}
				if err := json.NewEncoder(w).Encode(resp); err != nil {
					t.Logf("Failed to encode response: %v", err)
				}
			case "prompts/list":
				resp := mcp.JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      1,
					Result:  mustMarshal(mcp.ListPromptsResult{Prompts: []mcp.Prompt{{Name: "test-prompt", Description: "A test prompt"}}}),
				}
				if err := json.NewEncoder(w).Encode(resp); err != nil {
					t.Logf("Failed to encode response: %v", err)
				}
			}
		}))
		downstreamURL = downstreamServer.URL
	} else {
		downstreamURL = opts.DownstreamURL
	}

	// Setup gateway configuration
	posture := opts.Posture
	if posture == "" {
		posture = config.PostureNotary
	}

	cfg, err := config.LoadGateway(config.GatewayOptions{
		DataDir:           dataDir,
		PKIDir:            pkiDir,
		SecretsDir:        secretsDir,
		PasskeyRpID:       "localhost",
		PasskeyRpName:     "g8e",
		AllowTestPortZero: opts.AllowTestPortZero,
		Posture:           posture,
	})
	require.NoError(t, err)
	cfg.Gateway.MCPDownstreamURL = downstreamURL

	ls, err := gateway.NewGatewayModeService(cfg, testutil.NewTestLogger())
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
	err = ls.GetDB().SignerStore.AddTrustedSigner(models.TrustedSigner{
		ID:        ActuatorKeyID,
		PublicKey: hex.EncodeToString(ActuatorPub),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	})
	require.NoError(t, err)

	mcpGateway := ls.GetHTTPHandler().GetMCPGateway()
	require.NotNil(t, mcpGateway)

	cmdSvc, err := pubsub.NewOperatorPubSubService(pubsub.CommandServiceConfig{
		Config:             cfg,
		Logger:             testutil.NewTestLogger(),
		Execution:          execSvc,
		FileEdit:           fileSvc,
		PubSubClient:       pubsub.NewInProcessPubSubClient(ls.GetHTTPHandler().GetGatewayWebSocketHandler()),
		Scrubbing:          scrubbing.NewScrubbingService(scrubbing.DefaultConfig(), testutil.NewTestLogger(), nil),
		ReplayStore:        govDeps.ReplayStore,
		StateRootProvider:  govDeps.StateRootProvider,
		TransactionAudit:   govDeps.TransactionAudit,
		FieldReader:        govDeps.FieldReader,
		SignerStore:        govDeps.SignerStore,
		L3Notary:           gatewayRejectingL3Notary{},
		ActuatorSigningKey: ActuatorPriv,
		ActuatorKeyID:      ActuatorKeyID,
		MCPGateway:         mcpGateway,
	})
	require.NoError(t, err)
	ls.SetEnvelopeProcessor(cmdSvc)

	// The MCP gateway's runtime governance dependencies are wired by
	// NewOperatorPubSubService via initializeGovernance (mcpGateway was passed in
	// through CommandServiceConfig.MCPGateway above), so no extra wiring is needed.

	// Seed platform_settings required for health check
	err = ls.GetDB().DocStore.DocSet(string(constants.CollectionSettings), "platform_settings", json.RawMessage(`{"session_encryption_key":"test-key"}`))
	require.NoError(t, err)

	// Start the gateway service. The Start error is delivered on a buffered
	// channel and drained by Cleanup, so the goroutine never logs after the
	// test has completed (which would panic the test runtime under -race).
	ctx, cancel := context.WithCancel(context.Background())
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- ls.Start(ctx)
	}()

	// Wait for the gateway service to be ready
	require.Eventually(t, func() bool { return ls.IsReady() }, 10*time.Second, 100*time.Millisecond)

	// Create cleanup function
	cleanup := func() {
		cancel()
		// Join the Start goroutine before the test ends. A graceful shutdown
		// surfaces http.ErrServerClosed, which is expected and not an error.
		if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Logf("gateway start error: %v", err)
		}
		if downstreamServer != nil {
			downstreamServer.Close()
		}
		// Clean up test data directory
		os.RemoveAll(dataDir)
	}

	return &GatewayFixture{
		Config:           cfg,
		Service:          ls,
		DownstreamServer: downstreamServer,
		DataDir:          dataDir,
		SecretsDir:       secretsDir,
		PKIDir:           pkiDir,
		MCPGateway:       mcpGateway,
		ActuatorPriv:     ActuatorPriv,
		ActuatorKeyID:    ActuatorKeyID,
		Cleanup:          cleanup,
	}
}

// WaitForReady polls the HTTP health endpoint until the server accepts connections.
// Uses HTTP port instead of HTTPS to avoid mTLS certificate requirements.
func (f *GatewayFixture) WaitForReady(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	require.Eventually(t, func() bool {
		httpURL := constants.LocalhostHTTPURL(f.Service.GetHTTPPort())
		resp, err := client.Get(httpURL + constants.APIPaths.Health)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 10*time.Second, 100*time.Millisecond, "HTTP server did not become ready")
}

// SetPublicBaseURL sets the public base URL for the MCP gateway (used for approval links).
func (f *GatewayFixture) SetPublicBaseURL(baseURL string) {
	f.MCPGateway.SetPublicBaseURL(baseURL)
}

// gatewayRejectingL3Notary is a test implementation that always rejects L3 proofs.
type gatewayRejectingL3Notary struct{}

func (gatewayRejectingL3Notary) VerifyL3Proof(_ context.Context, _ string, _ string, _ string, _ *commonv1.L3Proof) (bool, error) {
	return false, nil
}

func mustMarshal(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal: %v", err))
	}
	return b
}

// ClientIdentity represents an enrolled client with certificates.
type ClientIdentity struct {
	UserID            string
	OrganizationID    string
	PrivateKey        []byte
	Certificate       []byte
	CLIPrivateKey     []byte
	CLICertificate    string
	OperatorID        string
	OperatorSessionID string
}

// EnrollClientIdentity performs CSR enrollment for a test client.
// It generates CSRs, creates a temporary certificate for enrollment,
// calls the enrollment endpoint, and returns the enrolled identity.
func EnrollClientIdentity(t *testing.T, f *GatewayFixture, userID, organizationID, systemFingerprint, hostname string) *ClientIdentity {
	t.Helper()

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
	err = f.Service.GetDB().DocStore.DocSet(string(constants.CollectionUsers), userID, userBytes)
	require.NoError(t, err)

	// Generate CSR for client certificate using P-256 (required by PKI curve enforcement)
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	csrTmpl := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   userID,
			Organization: []string{organizationID},
		},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTmpl, priv)
	require.NoError(t, err)
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	// Generate CLI CSR for distinct SPIFFE identity (required by PKI cleanup Phase 3)
	cliPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	cliCSRTmpl := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   userID + "-cli",
			Organization: []string{organizationID},
		},
	}
	cliCSRDER, err := x509.CreateCertificateRequest(rand.Reader, cliCSRTmpl, cliPriv)
	require.NoError(t, err)
	cliCSRPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: cliCSRDER})

	// Create a temporary client cert for initial enrollment (using Operator CA)
	operatorCAPEM := testutil.ReadOperatorCA(t, f.PKIDir)
	operatorBlock, _ := pem.Decode(operatorCAPEM)
	operatorCert, err := x509.ParseCertificate(operatorBlock.Bytes)
	require.NoError(t, err)
	sm, err := f.Service.GetSecretManager()
	require.NoError(t, err)
	operatorKeyDER, err := sm.GetCAPrivateKey("operator")
	require.NoError(t, err)
	operatorKey, err := x509.ParseECPrivateKey(operatorKeyDER)
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
	tempCertDER, err := x509.CreateCertificate(rand.Reader, tempCertTemplate, operatorCert, priv.Public(), operatorKey)
	require.NoError(t, err)
	tempCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tempCertDER})

	// Create mTLS client with temporary cert
	privBytes, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)
	tempCert, err := tls.X509KeyPair(tempCertPEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes}))
	require.NoError(t, err)

	rootPEM := testutil.ReadRootCA(t, f.PKIDir)
	operatorPEM := testutil.ReadOperatorCA(t, f.PKIDir)
	rootPool := x509.NewCertPool()
	rootPool.AppendCertsFromPEM(rootPEM)
	rootPool.AppendCertsFromPEM(operatorPEM)

	enrollClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{tempCert},
			},
		},
	}

	// Enroll via CSR endpoint
	mtlsURL := constants.LocalhostHTTPSURL(f.Service.GetHTTPSPort())
	regReq := models.OperatorRegistrationRequest{
		CSR:               string(csrPEM),
		CLICSR:            string(cliCSRPEM),
		SystemFingerprint: systemFingerprint,
		Hostname:          hostname,
	}
	regBody, _ := json.Marshal(regReq)
	hReq, _ := http.NewRequest(http.MethodPost, mtlsURL+constants.APIPaths.PKIDevicesEnroll, bytes.NewReader(regBody))
	hResp, err := enrollClient.Do(hReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, hResp.StatusCode)
	var regResp models.OperatorRegistrationResponse
	if err := json.NewDecoder(hResp.Body).Decode(&regResp); err != nil {
		t.Fatalf("gateway_fixture: decode registration response: %v", err)
	}
	hResp.Body.Close()

	// Wait for operator session to be persisted in database
	require.Eventually(t, func() bool {
		op, err := f.Service.GetDB().DocStore.DocGet(string(constants.CollectionOperators), regResp.OperatorID)
		if err != nil || op == nil {
			return false
		}
		var opDoc models.OperatorDocumentGo
		opBytes, err := json.Marshal(op.ForWire())
		if err != nil {
			return false
		}
		if err := json.Unmarshal(opBytes, &opDoc); err != nil {
			return false
		}
		t.Logf("Operator doc: ID=%s, SessionID=%s, Status=%s, OrgID=%s", opDoc.ID, opDoc.OperatorSessionID, opDoc.Status, opDoc.OrganizationID)
		return opDoc.OperatorSessionID == regResp.OperatorSessionID && opDoc.Status == constants.OperatorStatusActive
	}, 5*time.Second, 100*time.Millisecond, "Operator session not persisted")

	return &ClientIdentity{
		UserID:            userID,
		OrganizationID:    organizationID,
		PrivateKey:        privBytes,
		Certificate:       []byte(regResp.OperatorCert),
		CLIPrivateKey:     privBytes,
		CLICertificate:    regResp.OperatorCert,
		OperatorID:        regResp.OperatorID,
		OperatorSessionID: regResp.OperatorSessionID,
	}
}

// CreateMTLSClient creates an HTTP client configured for mTLS using the enrolled identity.
func CreateMTLSClient(t *testing.T, f *GatewayFixture, identity *ClientIdentity) *http.Client {
	t.Helper()

	rootPEM := testutil.ReadRootCA(t, f.PKIDir)
	operatorPEM := testutil.ReadOperatorCA(t, f.PKIDir)
	rootPool := x509.NewCertPool()
	rootPool.AppendCertsFromPEM(rootPEM)
	rootPool.AppendCertsFromPEM(operatorPEM)

	enrolledCert, err := tls.X509KeyPair(identity.Certificate, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: identity.PrivateKey}))
	require.NoError(t, err)

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{enrolledCert},
			},
		},
	}
}
