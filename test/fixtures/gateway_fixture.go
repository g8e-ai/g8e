// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration || e2e

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
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/consensus"
	"github.com/g8e-ai/g8e/internal/services/execution"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/services/gateway"
	govsvc "github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/services/network"
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
	DownstreamURL    string // URL of the downstream MCP/A2A server
	A2ADownstreamURL string // URL used for A2A (same as DownstreamURL if not overridden)
}

// GatewayFixtureOptions configures the gateway fixture.
type GatewayFixtureOptions struct {
	TestName          string
	Posture           config.GatewayPosture
	DownstreamURL     string // MCP downstream; creates mock server if empty
	A2ADownstreamURL  string // A2A downstream; if empty, reuses MCP downstream server
	AllowTestPortZero bool
	PublicBaseURL     string // Public base URL for approval links; defaults to localhost:HTTPS_port

	// Consensus configuration for consensus/notary postures. When zero-valued,
	// defaults to a single-member consensus (nMembers=1, quorum=1, nServiceMembers=1)
	// with consensusID "test-consensus".
	ConsensusID              string
	ConsensusNMembers        int
	ConsensusQuorum          int
	ConsensusNServiceMembers int
}

// repoTestResultsDir returns <repo>/test-results, computed from this source
// file's location so it is independent of the test's working directory. This
// file lives at <repo>/test/fixtures/gateway_fixture.go.
func repoTestResultsDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, constants.TestResultsDirname)
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
// Cleanup is registered internally via t.Cleanup() — callers do not need to
// defer anything.
func NewGatewayFixture(t *testing.T, opts GatewayFixtureOptions) *GatewayFixture {
	t.Helper()

	// Create test paths without mutating global constants.Paths. The ephemeral
	// scaffolding (secrets, runtime dir) lives under testutil.TempDir(t); only the
	// gateway data/vault is relocated to a persistent results directory below.
	testPaths := testutil.NewTestPathsFromTemp(t)

	// Integration runs leave their artifacts behind for inspection: each run
	// writes a fresh, uniquely-named directory under <repo>/test-results/ and
	// nothing is deleted between or within runs. os.MkdirTemp gives a unique
	// suffix so concurrent fixtures in the same test/second never collide, and
	// (unlike t.TempDir) the directory is NOT auto-removed when the test ends.
	resultsRoot := repoTestResultsDir()
	if err := os.MkdirAll(resultsRoot, 0755); err != nil {
		t.Fatalf("gateway_fixture: create test-results root: %v", err)
	}
	dataDir, err := os.MkdirTemp(resultsRoot, fmt.Sprintf("%s-%s-", time.Now().Format("20060102-150405"), opts.TestName))
	if err != nil {
		t.Fatalf("gateway_fixture: create test run directory: %v", err)
	}
	t.Logf("Test vault created at: %s", dataDir)

	secretsDir := testPaths.SecretsDir
	pkiDir := testPaths.PKIDir

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
					Result:  mustMarshal(mcp.ResourcesListResult{Resources: []mcp.Resource{{URI: "file:///test.txt", Name: "test.txt"}}}),
				}
				if err := json.NewEncoder(w).Encode(resp); err != nil {
					t.Logf("Failed to encode response: %v", err)
				}
			case "prompts/list":
				resp := mcp.JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      1,
					Result:  mustMarshal(mcp.PromptsListResult{Prompts: []mcp.Prompt{{Name: "test-prompt", Description: "A test prompt"}}}),
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
		VaultDir:          testPaths.VaultDir,
		VaultKeyPath:      testPaths.VaultKeyPath,
		PasskeyRpID:       "localhost",
		PasskeyRpName:     "g8e",
		AllowTestPortZero: opts.AllowTestPortZero,
		Posture:           posture,
	})
	require.NoError(t, err)
	cfg.Gateway.MCPDownstreamURL = downstreamURL

	if opts.PublicBaseURL != "" {
		cfg.Gateway.PublicBaseURL = opts.PublicBaseURL
	}

	a2aURL := opts.A2ADownstreamURL
	if a2aURL == "" {
		a2aURL = downstreamURL
	}
	cfg.Gateway.A2ADownstreamURL = a2aURL

	require.NoError(t, paths.InitWithBase(testPaths.BaseDir))
	fileSvc, err := fs.NewRuntimeFileService(testPaths.BaseDir, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

	ls, err := gateway.NewGatewayModeService(cfg, fileSvc, testutil.NewTestLogger())
	require.NoError(t, err)

	execSvc := execution.NewExecutionService(cfg, testutil.NewTestLogger())
	fileEditSvc := execution.NewFileEditService(cfg, testutil.NewTestLogger())
	govDeps := ls.GetGovernanceDeps()
	sm, err := ls.GetSecretManager()
	require.NoError(t, err)
	ActuatorPriv, ActuatorKeyID, err := sm.GetActuatorKey()
	require.NoError(t, err)

	// Add Actuator key to SignerStore so Implicit L2 signatures from the gateway are trusted
	ActuatorPub := ActuatorPriv.Public().(ed25519.PublicKey)
	err = ls.GetSignerStore().AddTrustedSigner(models.TrustedSigner{
		ID:        ActuatorKeyID,
		PublicKey: hex.EncodeToString(ActuatorPub),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	})
	require.NoError(t, err)

	mcpGateway := ls.GetMCPGateway()
	require.NotNil(t, mcpGateway)

	scrubbingSvc, err := scrubbing.NewScrubbingService(context.Background(), scrubbing.DefaultConfig(), testutil.NewTestLogger(), nil)
	require.NoError(t, err)

	// Auto-wire a Consensus for postures that require L2 signatures (consensus, notary).
	// Without L2 votes, the Warden rejects at L2 before L3 is ever checked.
	// This must happen before NewGatewayOperatorPubSubService so the deliberator
	// can be passed through GatewayCommandServiceConfig.L2ConsensusDeliberator
	// into the MCP gateway's RuntimeDependencies.
	var consensusSvc *consensus.ConsensusService
	var l2Deliberator *consensus.LocalDeliberator
	if posture == config.PostureConsensus || posture == config.PostureNotary {
		consensusID := opts.ConsensusID
		if consensusID == "" {
			consensusID = "test-consensus"
		}
		nMembers := opts.ConsensusNMembers
		if nMembers == 0 {
			nMembers = 1
		}
		quorum := opts.ConsensusQuorum
		if quorum == 0 {
			quorum = 1
		}
		nServiceMembers := opts.ConsensusNServiceMembers
		if nServiceMembers == 0 {
			nServiceMembers = 1
		}
		cs := SetupConsensus(t, &GatewayFixture{
			Config:        cfg,
			Service:       ls,
			MCPGateway:    mcpGateway,
			ActuatorPriv:  ActuatorPriv,
			ActuatorKeyID: ActuatorKeyID,
		}, consensusID, nMembers, quorum, nServiceMembers)
		consensusSvc = cs.Service
		l2Deliberator = cs.Deliberator
	}

	// Construct the in-process command service. The returned service is not
	// retained (cmdSvc) — the important side effect is wiring the
	// EnvProcAdapter and SessionValidatorAdapter targets so the HTTP handler's
	// GovernanceController can delegate envelope submission to this service.
	_, err = pubsub.NewGatewayOperatorPubSubService(pubsub.GatewayCommandServiceConfig{
		CommandServiceConfig: pubsub.CommandServiceConfig{
			Config:             cfg,
			Logger:             testutil.NewTestLogger(),
			Execution:          execSvc,
			FileEdit:           fileEditSvc,
			PubSubClient:       pubsub.NewInProcessPubSubClient(ls.GetGatewayWebSocketHandler()),
			Scrubbing:          scrubbingSvc,
			ActuatorSigningKey: ActuatorPriv,
			ActuatorKeyID:      ActuatorKeyID,
		},
		GovDeps: &pubsub.GovernanceDeps{
			ReplayStore:          govDeps.ReplayStore,
			StateRootProvider:    govDeps.StateRootProvider,
			TransactionAudit:     govDeps.TransactionAudit,
			GovernedDocStore:     govDeps.GovernedDocStore,
			SignerStore:          govDeps.SignerStore,
			ConsensusPolicyStore: govDeps.ConsensusPolicyStore,
			L3Notary:             RejectingL3Notary{},
			FieldReader:          govDeps.FieldReader,
			Doctrine:             govDeps.Doctrine,
		},
		MCPGateway:              mcpGateway,
		L2ConsensusDeliberator:  l2Deliberator,
		EnvProcAdapter:          ls.GetEnvProcAdapter(),
		SessionValidatorAdapter: ls.GetSessionValidatorAdapter(),
	})
	require.NoError(t, err)

	// The MCP gateway's runtime governance dependencies are wired by
	// NewGatewayOperatorPubSubService (mcpGateway was passed in through
	// GatewayCommandServiceConfig.MCPGateway above), so no extra wiring is needed.

	// Wire the consensus service into the already-constructed HTTP handler.
	// The handler was built during NewGatewayModeService with consensusSvc nil;
	// SetConsensusService updates the field that GovernanceController reads at
	// request time. Nil is valid (no consensus configured for this fixture).
	ls.SetConsensusService(consensusSvc)

	// Seed platform_settings required for health check
	err = ls.GetDocStore().DocSet(string(constants.CollectionSettings), "platform_settings", json.RawMessage(`{"session_encryption_key":"test-key"}`))
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

	// Register cleanup via t.Cleanup() — fires when the test completes
	t.Cleanup(func() {
		cancel()
		// Join the Start goroutine before the test ends. A graceful shutdown
		// surfaces http.ErrServerClosed, which is expected and not an error.
		if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Logf("gateway start error: %v", err)
		}
		if downstreamServer != nil {
			downstreamServer.Close()
		}
		// Stop the gateway service to close all databases and release file locks.
		// The data directory itself is intentionally left on disk: integration
		// runs accumulate results under <repo>/test-results/ for later inspection.
		if err := ls.Stop(context.Background()); err != nil {
			t.Logf("gateway stop error: %v", err)
		}
	})

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
		DownstreamURL:    downstreamURL,
		A2ADownstreamURL: a2aURL,
	}
}

// WaitForReady polls the HTTP health endpoint until the server accepts connections.
// Uses HTTP port instead of HTTPS to avoid mTLS certificate requirements.
func (f *GatewayFixture) WaitForReady(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	require.Eventually(t, func() bool {
		httpURL := network.LocalhostHTTPURL(f.Service.GetHTTPPort())
		resp, err := client.Get(httpURL + constants.APIPaths.Health)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 10*time.Second, 100*time.Millisecond, "HTTP server did not become ready")
}

// RejectingL3Notary is a test implementation that always rejects L3 proofs.
type RejectingL3Notary struct{}

func (RejectingL3Notary) VerifyL3Proof(_ context.Context, _ string, _ string, _ string, _ *commonv1.L3Proof) (bool, error) {
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
	CLISessionID      string
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
	err = f.Service.GetDocStore().DocSet(string(constants.CollectionUsers), userID, userBytes)
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
	require.True(t, rootPool.AppendCertsFromPEM(rootPEM), "failed to parse root CA PEM")
	require.True(t, rootPool.AppendCertsFromPEM(operatorPEM), "failed to parse operator CA PEM")

	enrollClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{tempCert},
			},
		},
	}

	// Enroll via CSR endpoint
	mtlsURL := network.LocalhostHTTPSURL(f.Service.GetHTTPSPort())
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
		op, err := f.Service.GetDocStore().DocGet(string(constants.CollectionOperators), regResp.OperatorID)
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

	cliPrivBytes, err := x509.MarshalECPrivateKey(cliPriv)
	require.NoError(t, err)

	return &ClientIdentity{
		UserID:            userID,
		OrganizationID:    organizationID,
		PrivateKey:        privBytes,
		Certificate:       []byte(regResp.OperatorCert),
		CLIPrivateKey:     cliPrivBytes,
		CLICertificate:    regResp.CLICert,
		CLISessionID:      regResp.CLISessionID,
		OperatorID:        regResp.OperatorID,
		OperatorSessionID: regResp.OperatorSessionID,
	}
}

// CreateNoCertClient creates an HTTP client that trusts the gateway's CA bundle
// but does not present a client certificate. This is useful for testing endpoints
// that should reject unauthenticated connections (e.g., API key rejection).
func CreateNoCertClient(t *testing.T, f *GatewayFixture) *http.Client {
	t.Helper()

	rootPEM := testutil.ReadRootCA(t, f.PKIDir)
	operatorPEM := testutil.ReadOperatorCA(t, f.PKIDir)
	rootPool := x509.NewCertPool()
	require.True(t, rootPool.AppendCertsFromPEM(rootPEM), "failed to parse root CA PEM")
	require.True(t, rootPool.AppendCertsFromPEM(operatorPEM), "failed to parse operator CA PEM")

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: rootPool,
			},
		},
	}
}

// CreateMTLSClient creates an HTTP client configured for mTLS using the enrolled identity.
func CreateMTLSClient(t *testing.T, f *GatewayFixture, identity *ClientIdentity) *http.Client {
	t.Helper()

	rootPEM := testutil.ReadRootCA(t, f.PKIDir)
	operatorPEM := testutil.ReadOperatorCA(t, f.PKIDir)
	rootPool := x509.NewCertPool()
	require.True(t, rootPool.AppendCertsFromPEM(rootPEM), "failed to parse root CA PEM")
	require.True(t, rootPool.AppendCertsFromPEM(operatorPEM), "failed to parse operator CA PEM")

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

// CreateCLIMTLSClient creates an HTTP client configured for mTLS using the
// enrolled CLI identity. The client presents the CLI certificate (which carries
// the CLI SPIFFE URI SAN) and sets the X-G8E-CLI-Session-ID header on every
// outbound request via a wrapping RoundTripper.
func CreateCLIMTLSClient(t *testing.T, f *GatewayFixture, identity *ClientIdentity) *http.Client {
	t.Helper()

	rootPEM := testutil.ReadRootCA(t, f.PKIDir)
	operatorPEM := testutil.ReadOperatorCA(t, f.PKIDir)
	rootPool := x509.NewCertPool()
	require.True(t, rootPool.AppendCertsFromPEM(rootPEM), "failed to parse root CA PEM")
	require.True(t, rootPool.AppendCertsFromPEM(operatorPEM), "failed to parse operator CA PEM")

	cliCert, err := tls.X509KeyPair([]byte(identity.CLICertificate), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: identity.CLIPrivateKey}))
	require.NoError(t, err)

	base := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      rootPool,
				Certificates: []tls.Certificate{cliCert},
			},
		},
	}

	return &http.Client{
		Transport: &cliSessionRoundTripper{
			base:      base.Transport,
			sessionID: identity.CLISessionID,
		},
	}
}

// cliSessionRoundTripper wraps an http.RoundTripper and injects the
// X-G8E-CLI-Session-ID header on every outbound request.
type cliSessionRoundTripper struct {
	base      http.RoundTripper
	sessionID string
}

func (rt *cliSessionRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set(constants.HeaderCLISessionID, rt.sessionID)
	return rt.base.RoundTrip(clone)
}

// ConsensusSetup holds the result of wiring a real ConsensusService into a gateway fixture.
type ConsensusSetup struct {
	ConsensusID string
	Members     []consensus.ConsensusMember
	Service     *consensus.ConsensusService
	Deliberator *consensus.LocalDeliberator
}

// SetupConsensus wires a real ConsensusService into the gateway fixture for consensus
// posture integration tests. It generates nMembers Ed25519 key pairs, registers each
// member's public key as a TrustedSigner, creates a ConsensusPolicy in the ConsensusStore,
// constructs a ConsensusService via the shared consensus.NewConsensusFromPolicy factory,
// and returns a LocalDeliberator via ConsensusSetup.Deliberator. The caller
// passes the deliberator through GatewayCommandServiceConfig.L2ConsensusDeliberator
// so it is wired into the MCP gateway's RuntimeDependencies. The returned
// ConsensusSetup.Service is wired into the gateway via SetConsensusService by
// the caller.
//
// If nServiceMembers < nMembers, only the first nServiceMembers are given private keys
// — the remaining policy members exist in the store but cannot vote (their keys resolve
// to nil via the KeyProvider, and Deliberate skips nil-key members). This lets tests
// simulate quorum-not-reached by producing fewer votes than the quorum threshold requires.
//
// This uses the same consensus.NewConsensusFromPolicy factory as production ConsensusBootstrap
// in internal/cli/serve/gateway.go, eliminating the duplication identified in CS-12.
func SetupConsensus(t *testing.T, f *GatewayFixture, consensusID string, nMembers, quorum, nServiceMembers int) *ConsensusSetup {
	t.Helper()

	memberAppIDs := make([]string, nMembers)
	memberKeys := make(map[string]ed25519.PrivateKey, nServiceMembers)
	signingMembers := make([]consensus.ConsensusMember, 0, nServiceMembers)

	for i := 0; i < nMembers; i++ {
		appID := fmt.Sprintf("%s-member-%d", consensusID, i)
		memberAppIDs[i] = appID

		pub, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		err = f.Service.GetSignerStore().AddTrustedSigner(models.TrustedSigner{
			ID:        appID,
			PublicKey: hex.EncodeToString(pub),
			AddedAt:   time.Now().UTC(),
			Enabled:   true,
		})
		require.NoError(t, err)

		if i < nServiceMembers {
			memberKeys[appID] = priv
			signingMembers = append(signingMembers, consensus.ConsensusMember{
				AppID:      appID,
				PrivateKey: priv,
			})
		}
	}

	policy := models.ConsensusPolicy{
		ID:              consensusID,
		MemberAppIDs:    memberAppIDs,
		Quorum:          quorum,
		RequireDistinct: true,
		Enabled:         true,
	}
	err := f.Service.GetConsensusStore().AddConsensus(policy)
	require.NoError(t, err)

	keyProvider := consensus.KeyProviderFunc(func(appID string) (ed25519.PrivateKey, error) {
		if key, ok := memberKeys[appID]; ok {
			return key, nil
		}
		return nil, fmt.Errorf("no private key for member %s (quorum-not-reached simulation)", appID)
	})

	doctrine := govsvc.NewL1Doctrine()
	responder := response.NewWriter(testutil.NewTestLogger())
	consensusSvc, err := consensus.NewConsensusFromPolicy(&policy, keyProvider, doctrine, testutil.NewTestLogger(), responder)
	require.NoError(t, err)

	deliberator := consensus.NewLocalDeliberator(consensusSvc)

	return &ConsensusSetup{
		ConsensusID: consensusID,
		Members:     signingMembers,
		Service:     consensusSvc,
		Deliberator: deliberator,
	}
}
