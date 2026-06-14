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

package gateway

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/keystore"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

type mockEnvelopeProcessor struct {
	Captured []byte
}

func (m *mockEnvelopeProcessor) ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error) {
	m.Captured = payload
	return &operatorv1.ActionReceipt{TransactionHash: "mocked-hash"}, nil
}

func setupTestIdP(t *testing.T) (*rsa.PrivateKey, *httptest.Server) {
	t.Helper()
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		jwks := JWKS{
			Keys: []JWK{
				{
					Kty: "RSA",
					Kid: "test-key-1",
					Use: "sig",
					N:   base64.RawURLEncoding.EncodeToString(privKey.N.Bytes()),
					E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privKey.E)).Bytes()),
				},
			},
		}
		json.NewEncoder(w).Encode(jwks)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(func() { server.Close() })
	return privKey, server
}

func generateSignedJWT(t *testing.T, privKey *rsa.PrivateKey, claims map[string]interface{}) string {
	t.Helper()
	header := map[string]string{
		"alg": "RS256",
		"kid": "test-key-1",
		"typ": "JWT",
	}

	headerBytes, _ := json.Marshal(header)
	claimsBytes, _ := json.Marshal(claims)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerBytes)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsBytes)

	signingString := headerB64 + "." + claimsB64
	hasher := sha256.New()
	hasher.Write([]byte(signingString))
	hashed := hasher.Sum(nil)

	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, privKey, crypto.SHA256, hashed)
	require.NoError(t, err)

	sigB64 := base64.RawURLEncoding.EncodeToString(sigBytes)
	return signingString + "." + sigB64
}

func setupSuspendedTxService(t *testing.T, dbDir string) *storage.SuspendedTransactionService {
	t.Helper()
	suspendedTxConfig := &storage.SuspendedTransactionConfig{
		DBPath:               filepath.Join(dbDir, constants.SuspendedTxFilename),
		MaxDBSizeMB:          256,
		RetentionDays:        7,
		PruneIntervalMinutes: 30,
	}
	suspendedTxService, err := storage.NewSuspendedTransactionService(suspendedTxConfig, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { suspendedTxService.Close() })
	return suspendedTxService
}

func TestGateway_JWTIntegration(t *testing.T) {
	privKey, idpServer := setupTestIdP(t)

	cfg := testutil.NewTestConfig(t)
	cfg.Gateway.JWKSURL = idpServer.URL + "/.well-known/jwks.json"
	cfg.Gateway.JWTRoleClaim = "roles"

	logger := testutil.NewTestLogger()

	dbDir := tempDir(t)
	pkiDir := tempDir(t)
	secretsDir := tempDir(t)
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, constants.VaultDirname), logger, true, "", false, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	os.RemoveAll(secretsDir)
	os.MkdirAll(secretsDir, 0755)

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	backend, err := keystore.NewTestBackend()
	require.NoError(t, err)
	ks, err := keystore.NewWithBackend(tempDir(t), logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.EnforcePermissions())
	sm := &SecretManager{
		db:         db.db,
		secretsDir: secretsDir,
		logger:     logger,
		keystore:   ks,
	}

	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	err = pki.InitializePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	// Initialize default personas
	for _, persona := range DefaultPersonaDefinitions() {
		existing, err := personaSvc.GetByID(persona.ID)
		require.NoError(t, err)
		if existing == nil {
			require.NoError(t, personaSvc.CreatePersona(&persona))
		}
	}

	// Create an invitation for JIT provisioning
	_, err = userSvc.CreateInvitation("tenant-abc", "user-1234", "bootstrap", []string{"admin"}, 24*time.Hour)
	require.NoError(t, err)
	resp := response.NewWriter(logger)
	auth := NewAuthService(db, pki, logger, userSvc, personaSvc, resp, secretsDir, NewJWKSProvider(cfg.Gateway.JWKSURL), cfg.Gateway.JWTRoleClaim, "", "")
	// Apply JWT configuration to AuthService's provider

	cliSessionSvc := NewCLISessionService(db, logger)
	operatorSessionSvc := NewOperatorSessionService(db, logger)
	webSessionSvc := NewWebSessionService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, &cfg.Gateway)
	passkey, _ := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})

	suspendedTxService := setupSuspendedTxService(t, dbDir)

	mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
		Logger:           logger,
		Responder:        resp,
		SuspendedStore:   suspendedTxService,
		ScrubbingService: nil,
		MaxPayloadBytes:  cfg.Gateway.MaxPayloadBytes,
		Posture:          string(cfg.Gateway.Posture),
	})
	if err != nil {
		t.Fatalf("failed to create MCP gateway: %v", err)
	}

	mockEnvProc := &mockEnvelopeProcessor{}
	mcpGateway.SetDependencies(mockEnvProc, nil, nil, "", "")

	h, err := newHTTPHandler(HTTPHandlerDependencies{
		Cfg:                cfg,
		Logger:             logger,
		DB:                 db,
		Pubsub:             pubsub,
		Auth:               auth,
		PKI:                pki,
		CLISessionSvc:      cliSessionSvc,
		OperatorSessionSvc: operatorSessionSvc,
		WebSessionSvc:      webSessionSvc,
		Reg:                reg,
		Passkey:            passkey,
		UserSvc:            userSvc,
		Responder:          resp,
		MCPGateway:         mcpGateway,
		IsReady:            func() bool { return true },
		IsGovernanceReady:  func() bool { return true },
	})
	if err != nil {
		t.Fatalf("failed to create HTTP handler: %v", err)
	}

	cfg.Gateway.RateLimitRPS = 1000
	cfg.Gateway.RateLimitBurst = 1000
	ts := httptest.NewServer(h.buildPublicRouter())
	t.Cleanup(func() { ts.Close() })

	// Give the AuthService JWKS provider time to fetch or fetch lazily
	// Make a JWT request
	claims := map[string]interface{}{
		"sub":       "user-1234",
		"iss":       "test-idp",
		"exp":       time.Now().Add(1 * time.Hour).Unix(),
		"iat":       time.Now().Unix(),
		"tenant_id": "tenant-abc",
		"roles":     []string{"admin", "security-analyst"},
	}

	token := generateSignedJWT(t, privKey, claims)

	// Call an MCP endpoint that generates an envelope
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{}}}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/mcp/tools/call", bytes.NewBufferString(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(res.Body)
	t.Logf("Response Status: %d, Body: %s", res.StatusCode, buf.String())

	// Wait, the MCP Tools Call endpoint will process the envelope. The response might be 503 if downstream is not set up, or it might return success if mocked right.
	// We really only care that the envelope was captured by the mock processor, or at least passed through `processGatewayTransaction`.

	// Because tools/call waits for the envelope processor to return a receipt, and our mock returns a receipt with hash "mocked-hash",
	// the code might then look for pubsub results. But processGatewayTransaction comes FIRST before DispatchToDownstream.
	// Oh, `DispatchToDownstream` calls `processGatewayTransaction`. Let's check `mockEnvProc.Captured`.

	assert.NotNil(t, mockEnvProc.Captured)
	t.Logf("Captured envelope: %s", string(mockEnvProc.Captured))

	// Decode the captured envelope
	var envelope commonv1.GovernanceEnvelope
	err = protojson.Unmarshal(mockEnvProc.Captured, &envelope)
	require.NoError(t, err)

	assert.Equal(t, "tenant-abc", envelope.TenantId)
	assert.Equal(t, "admin", envelope.BindingPersona, "Should map admin to admin persona")

	// Validate JIT user provisioning
	user, err := userSvc.GetByID("user-1234")
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, "user-1234", user.ID)
	assert.True(t, user.IsActive())
}

func TestGateway_JITPasskeyBootstrap(t *testing.T) {
	privKey, idpServer := setupTestIdP(t)

	cfg := testutil.NewTestConfig(t)
	cfg.Gateway.JWKSURL = idpServer.URL + "/.well-known/jwks.json"
	cfg.Gateway.JWTRoleClaim = "roles"

	logger := testutil.NewTestLogger()

	dbDir := tempDir(t)
	pkiDir := tempDir(t)
	secretsDir := tempDir(t)
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, constants.VaultDirname), logger, true, "", false, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	os.RemoveAll(secretsDir)
	os.MkdirAll(secretsDir, 0755)

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	backend, err := keystore.NewTestBackend()
	require.NoError(t, err)
	ks, err := keystore.NewWithBackend(tempDir(t), logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.EnforcePermissions())
	sm := &SecretManager{
		db:         db.db,
		secretsDir: secretsDir,
		logger:     logger,
		keystore:   ks,
	}

	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	err = pki.InitializePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	// Initialize default personas
	for _, persona := range DefaultPersonaDefinitions() {
		existing, err := personaSvc.GetByID(persona.ID)
		require.NoError(t, err)
		if existing == nil {
			require.NoError(t, personaSvc.CreatePersona(&persona))
		}
	}

	// Create an invitation for JIT provisioning
	_, err = userSvc.CreateInvitation("tenant-abc", "jit-user-001", "bootstrap", []string{"admin"}, 24*time.Hour)
	require.NoError(t, err)
	resp := response.NewWriter(logger)
	auth := NewAuthService(db, pki, logger, userSvc, personaSvc, resp, secretsDir, NewJWKSProvider(cfg.Gateway.JWKSURL), cfg.Gateway.JWTRoleClaim, "", "")

	cliSessionSvc := NewCLISessionService(db, logger)
	operatorSessionSvc := NewOperatorSessionService(db, logger)
	webSessionSvc := NewWebSessionService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, &cfg.Gateway)
	passkey, _ := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})

	suspendedTxService := setupSuspendedTxService(t, dbDir)

	mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
		Logger:           logger,
		Responder:        resp,
		SuspendedStore:   suspendedTxService,
		ScrubbingService: nil,
		MaxPayloadBytes:  cfg.Gateway.MaxPayloadBytes,
		Posture:          string(cfg.Gateway.Posture),
	})
	if err != nil {
		t.Fatalf("failed to create MCP gateway: %v", err)
	}

	mockEnvProc := &mockEnvelopeProcessor{}
	mcpGateway.SetDependencies(mockEnvProc, nil, nil, "", "")

	h, err := newHTTPHandler(HTTPHandlerDependencies{
		Cfg:                cfg,
		Logger:             logger,
		DB:                 db,
		Pubsub:             pubsub,
		Auth:               auth,
		PKI:                pki,
		CLISessionSvc:      cliSessionSvc,
		OperatorSessionSvc: operatorSessionSvc,
		WebSessionSvc:      webSessionSvc,
		Reg:                reg,
		Passkey:            passkey,
		UserSvc:            userSvc,
		Responder:          resp,
		MCPGateway:         mcpGateway,
		IsReady:            func() bool { return true },
		IsGovernanceReady:  func() bool { return true },
	})
	if err != nil {
		t.Fatalf("failed to create HTTP handler: %v", err)
	}

	cfg.Gateway.RateLimitRPS = 1000
	cfg.Gateway.RateLimitBurst = 1000
	ts := httptest.NewServer(h.buildPublicRouter())
	t.Cleanup(func() { ts.Close() })

	// Generate a valid JWT for the JIT user
	claims := map[string]interface{}{
		"sub":       "jit-user-001",
		"iss":       "test-idp",
		"exp":       time.Now().Add(1 * time.Hour).Unix(),
		"iat":       time.Now().Unix(),
		"tenant_id": "tenant-abc",
		"roles":     []string{"admin"},
	}
	token := generateSignedJWT(t, privKey, claims)

	t.Run("JIT user with zero credentials can complete register-challenge with valid JWT", func(t *testing.T) {
		reqBody := `{"user_id":"jit-user-001","user_name":"JIT User"}`
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/passkeys/jit-register/challenge", bytes.NewBufferString(reqBody))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		res, err := client.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode, "Should allow first-credential registration via JWT")

		var respBody map[string]interface{}
		json.NewDecoder(res.Body).Decode(&respBody)
		assert.True(t, respBody["success"].(bool))
		assert.NotNil(t, respBody["options"])
	})

	t.Run("JIT user with zero credentials rejected with expired JWT", func(t *testing.T) {
		expiredClaims := map[string]interface{}{
			"sub":       "jit-user-001",
			"iss":       "test-idp",
			"exp":       time.Now().Add(-1 * time.Hour).Unix(),
			"iat":       time.Now().Add(-2 * time.Hour).Unix(),
			"tenant_id": "tenant-abc",
			"roles":     []string{"admin"},
		}
		expiredToken := generateSignedJWT(t, privKey, expiredClaims)

		reqBody := `{"user_id":"jit-user-001","user_name":"JIT User"}`
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/passkeys/jit-register/challenge", bytes.NewBufferString(reqBody))
		req.Header.Set("Authorization", "Bearer "+expiredToken)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		res, err := client.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, res.StatusCode, "Should reject expired JWT")
	})

	t.Run("JIT user with zero credentials rejected with no JWT", func(t *testing.T) {
		reqBody := `{"user_id":"jit-user-001","user_name":"JIT User"}`
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/passkeys/jit-register/challenge", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		res, err := client.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, res.StatusCode, "Should reject request without JWT")
	})
}

func TestGateway_JITPasskeyStepUpRequired(t *testing.T) {
	privKey, idpServer := setupTestIdP(t)

	cfg := testutil.NewTestConfig(t)
	cfg.Gateway.JWKSURL = idpServer.URL + "/.well-known/jwks.json"
	cfg.Gateway.JWTRoleClaim = "roles"

	logger := testutil.NewTestLogger()

	dbDir := tempDir(t)
	pkiDir := tempDir(t)
	secretsDir := tempDir(t)
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, constants.VaultDirname), logger, true, "", false, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	os.RemoveAll(secretsDir)
	os.MkdirAll(secretsDir, 0755)

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	backend, err := keystore.NewTestBackend()
	require.NoError(t, err)
	ks, err := keystore.NewWithBackend(tempDir(t), logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.EnforcePermissions())
	sm := &SecretManager{
		db:         db.db,
		secretsDir: secretsDir,
		logger:     logger,
		keystore:   ks,
	}

	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	err = pki.InitializePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	// Initialize default personas
	for _, persona := range DefaultPersonaDefinitions() {
		existing, err := personaSvc.GetByID(persona.ID)
		require.NoError(t, err)
		if existing == nil {
			require.NoError(t, personaSvc.CreatePersona(&persona))
		}
	}

	// Create an invitation for JIT provisioning
	_, err = userSvc.CreateInvitation("tenant-abc", "stepup-user-001", "bootstrap", []string{"admin"}, 24*time.Hour)
	require.NoError(t, err)
	resp := response.NewWriter(logger)
	auth := NewAuthService(db, pki, logger, userSvc, personaSvc, resp, secretsDir, NewJWKSProvider(cfg.Gateway.JWKSURL), cfg.Gateway.JWTRoleClaim, "", "")

	cliSessionSvc := NewCLISessionService(db, logger)
	operatorSessionSvc := NewOperatorSessionService(db, logger)
	webSessionSvc := NewWebSessionService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, &cfg.Gateway)
	passkey, _ := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})

	suspendedTxService := setupSuspendedTxService(t, dbDir)

	mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
		Logger:           logger,
		Responder:        resp,
		SuspendedStore:   suspendedTxService,
		ScrubbingService: nil,
		MaxPayloadBytes:  cfg.Gateway.MaxPayloadBytes,
		Posture:          string(cfg.Gateway.Posture),
	})
	if err != nil {
		t.Fatalf("failed to create MCP gateway: %v", err)
	}

	mockEnvProc := &mockEnvelopeProcessor{}
	mcpGateway.SetDependencies(mockEnvProc, nil, nil, "", "")

	h, err := newHTTPHandler(HTTPHandlerDependencies{
		Cfg:                cfg,
		Logger:             logger,
		DB:                 db,
		Pubsub:             pubsub,
		Auth:               auth,
		PKI:                pki,
		CLISessionSvc:      cliSessionSvc,
		OperatorSessionSvc: operatorSessionSvc,
		WebSessionSvc:      webSessionSvc,
		Reg:                reg,
		Passkey:            passkey,
		UserSvc:            userSvc,
		Responder:          resp,
		MCPGateway:         mcpGateway,
		IsReady:            func() bool { return true },
		IsGovernanceReady:  func() bool { return true },
	})
	if err != nil {
		t.Fatalf("failed to create HTTP handler: %v", err)
	}

	cfg.Gateway.RateLimitRPS = 1000
	cfg.Gateway.RateLimitBurst = 1000
	ts := httptest.NewServer(h.buildPublicRouter())
	t.Cleanup(func() { ts.Close() })

	// First, JIT provision the user via JWT
	claims := map[string]interface{}{
		"sub":       "stepup-user-001",
		"iss":       "test-idp",
		"exp":       time.Now().Add(1 * time.Hour).Unix(),
		"iat":       time.Now().Unix(),
		"tenant_id": "tenant-abc",
		"roles":     []string{"admin"},
	}
	token := generateSignedJWT(t, privKey, claims)

	// Add a mock passkey credential directly to the user to simulate first credential already registered
	user, err := userSvc.GetBySub("stepup-user-001")
	require.NoError(t, err)
	if user == nil {
		// Create user if doesn't exist
		invitation, err := userSvc.FindActiveInvitationBySub("stepup-user-001")
		require.NoError(t, err)
		require.NotNil(t, invitation)
		user, err = userSvc.CreateUserFromInvitation("stepup-user-001", invitation)
		require.NoError(t, err)
	}
	require.NotNil(t, user)

	// Manually add a credential to simulate having already registered one
	credentials := []models.PasskeyCredential{
		{
			ID:              []byte("mock-credential-id"),
			PublicKey:       []byte("mock-public-key"),
			AttestationType: "none",
			Authenticator: models.Authenticator{
				AAGUID:       []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
				SignCount:    0,
				CloneWarning: false,
			},
		},
	}
	err = userSvc.UpdatePasskeyCredentials(user.ID, credentials)
	require.NoError(t, err)

	t.Run("After one credential exists, JWT-only path is rejected and step-up required", func(t *testing.T) {
		reqBody := `{"user_id":"stepup-user-001","user_name":"Stepup User"}`
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/passkeys/jit-register/challenge", bytes.NewBufferString(reqBody))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		res, err := client.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()

		assert.Equal(t, http.StatusForbidden, res.StatusCode, "Should reject JWT-only registration when user already has credentials")

		var respBody map[string]interface{}
		json.NewDecoder(res.Body).Decode(&respBody)
		assert.Contains(t, respBody["error"], "first-credential registration only")
	})
}

func TestGateway_JWTValidation_IssuerAudienceNbf(t *testing.T) {
	t.Parallel()
	privKey, idpServer := setupTestIdP(t)
	defer idpServer.Close()

	cfg := testutil.NewTestConfig(t)
	cfg.Gateway.JWKSURL = idpServer.URL + "/.well-known/jwks.json"
	cfg.Gateway.JWTIssuer = "https://test-idp.example.com"
	cfg.Gateway.JWTAudience = "g8e-gateway"

	logger := testutil.NewTestLogger()

	dbDir := tempDir(t)
	pkiDir := tempDir(t)
	secretsDir := tempDir(t)
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, constants.VaultDirname), logger, true, "", false, nil)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	os.RemoveAll(secretsDir)
	os.MkdirAll(secretsDir, 0755)

	backend, err := keystore.NewTestBackend()
	require.NoError(t, err)
	ks, err := keystore.NewWithBackend(tempDir(t), logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.EnforcePermissions())

	sm, err := NewSecretManager(db.db, secretsDir, logger)
	require.NoError(t, err)

	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	err = pki.InitializePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	// Initialize default personas
	for _, persona := range DefaultPersonaDefinitions() {
		existing, err := personaSvc.GetByID(persona.ID)
		require.NoError(t, err)
		if existing == nil {
			require.NoError(t, personaSvc.CreatePersona(&persona))
		}
	}

	resp := response.NewWriter(logger)
	auth := NewAuthService(db, pki, logger, userSvc, personaSvc, resp, secretsDir, NewJWKSProvider(cfg.Gateway.JWKSURL), cfg.Gateway.JWTRoleClaim, cfg.Gateway.JWTIssuer, cfg.Gateway.JWTAudience)

	t.Run("Token with wrong aud is rejected", func(t *testing.T) {
		claims := map[string]interface{}{
			"sub": "user-123",
			"iss": "https://test-idp.example.com",
			"aud": "wrong-audience",
			"exp": time.Now().Add(1 * time.Hour).Unix(),
			"iat": time.Now().Unix(),
		}
		token := generateSignedJWT(t, privKey, claims)

		_, err := ParseAndVerifyJWT(context.Background(), token, auth.jwks, auth.jwtRole, auth.jwtIssuer, auth.jwtAudience)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "audience mismatch")
	})

	t.Run("Token with wrong iss is rejected", func(t *testing.T) {
		claims := map[string]interface{}{
			"sub": "user-123",
			"iss": "https://wrong-issuer.example.com",
			"aud": "g8e-gateway",
			"exp": time.Now().Add(1 * time.Hour).Unix(),
			"iat": time.Now().Unix(),
		}
		token := generateSignedJWT(t, privKey, claims)

		_, err := ParseAndVerifyJWT(context.Background(), token, auth.jwks, auth.jwtRole, auth.jwtIssuer, auth.jwtAudience)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "issuer mismatch")
	})

	t.Run("Token with nbf in the future is rejected", func(t *testing.T) {
		claims := map[string]interface{}{
			"sub": "user-123",
			"iss": "https://test-idp.example.com",
			"aud": "g8e-gateway",
			"exp": time.Now().Add(1 * time.Hour).Unix(),
			"nbf": time.Now().Add(5 * time.Minute).Unix(),
			"iat": time.Now().Unix(),
		}
		token := generateSignedJWT(t, privKey, claims)

		_, err := ParseAndVerifyJWT(context.Background(), token, auth.jwks, auth.jwtRole, auth.jwtIssuer, auth.jwtAudience)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not yet valid")
	})

	t.Run("Token with correct iss, aud, and nbf is accepted", func(t *testing.T) {
		claims := map[string]interface{}{
			"sub": "user-123",
			"iss": "https://test-idp.example.com",
			"aud": "g8e-gateway",
			"exp": time.Now().Add(1 * time.Hour).Unix(),
			"nbf": time.Now().Add(-1 * time.Minute).Unix(),
			"iat": time.Now().Unix(),
		}
		token := generateSignedJWT(t, privKey, claims)

		jwt, err := ParseAndVerifyJWT(context.Background(), token, auth.jwks, auth.jwtRole, auth.jwtIssuer, auth.jwtAudience)
		require.NoError(t, err)
		assert.Equal(t, "user-123", jwt.Claims.Sub)
		assert.Equal(t, "https://test-idp.example.com", jwt.Claims.Iss)
		assert.Equal(t, "g8e-gateway", jwt.Claims.Aud)
	})

	t.Run("Token without iss/aud/nbf is accepted when not configured", func(t *testing.T) {
		authNoValidation := NewAuthService(db, pki, testutil.NewTestLogger(), userSvc, personaSvc, resp, secretsDir, NewJWKSProvider(cfg.Gateway.JWKSURL), cfg.Gateway.JWTRoleClaim, "", "")

		claims := map[string]interface{}{
			"sub": "user-123",
			"exp": time.Now().Add(1 * time.Hour).Unix(),
			"iat": time.Now().Unix(),
		}
		token := generateSignedJWT(t, privKey, claims)

		jwt, err := ParseAndVerifyJWT(context.Background(), token, authNoValidation.jwks, authNoValidation.jwtRole, authNoValidation.jwtIssuer, authNoValidation.jwtAudience)
		require.NoError(t, err)
		assert.Equal(t, "user-123", jwt.Claims.Sub)
	})
}
