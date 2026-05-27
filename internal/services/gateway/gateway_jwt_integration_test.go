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
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/g8e-ai/g8e/internal/services/keystore"
	"github.com/g8e-ai/g8e/internal/services/mcp"
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

func TestGateway_JWTIntegration(t *testing.T) {
	privKey, idpServer := setupTestIdP(t)

	cfg := testutil.NewTestConfig(t)
	cfg.Gateway.JWKSURL = idpServer.URL + "/.well-known/jwks.json"
	cfg.Gateway.JWTRoleClaim = "roles"

	logger := testutil.NewTestLogger()

	dbDir := t.TempDir()
	pkiDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	os.RemoveAll(secretsDir)
	os.MkdirAll(secretsDir, 0755)

	pubsub := NewPubSubBroker(logger)
	t.Cleanup(func() { pubsub.Close() })

	backend, err := keystore.NewTestBackend()
	require.NoError(t, err)
	ks, err := keystore.NewWithBackend(t.TempDir(), logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.EnsurePermissions())
	sm := &SecretManager{
		db:         db.db,
		secretsDir: secretsDir,
		logger:     logger,
		keystore:   ks,
	}

	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	err = pki.EnsurePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	require.NoError(t, personaSvc.GetOrCreateDefaultPersonas())

	// Create an invitation for JIT provisioning
	_, err = userSvc.CreateInvitation("tenant-abc", "user-1234", "bootstrap", []string{"admin"}, 24*time.Hour)
	require.NoError(t, err)
	resp := responder.New(logger)
	auth := NewAuthService(db, pki, logger, userSvc, personaSvc, resp, secretsDir, NewJWKSProvider(cfg.Gateway.JWKSURL), cfg.Gateway.JWTRoleClaim)
	// Apply JWT configuration to AuthService's provider

	sessionSvc := NewSessionService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc, sessionSvc, &cfg.Gateway)
	passkey, _ := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})

	mcpGateway := mcp.NewGatewayService(mcp.Dependencies{
		Logger:          logger,
		Responder:       resp,
		SuspendedStore:  db,
		MaxPayloadBytes: cfg.Gateway.MaxPayloadBytes,
	})

	mockEnvProc := &mockEnvelopeProcessor{}
	mcpGateway.SetDependencies(mockEnvProc, nil, nil, "", "")

	h := newHTTPHandler(HTTPHandlerDependencies{
		Cfg:               cfg,
		Logger:            logger,
		DB:                db,
		Pubsub:            pubsub,
		Auth:              auth,
		PKI:               pki,
		SessionSvc:        sessionSvc,
		Reg:               reg,
		Passkey:           passkey,
		UserSvc:           userSvc,
		Responder:         resp,
		MCPGateway:        mcpGateway,
		IsReady:           func() bool { return true },
		IsGovernanceReady: func() bool { return true },
	})

	cfg.Gateway.RateLimitRPS = 1000
	cfg.Gateway.RateLimitBurst = 1000
	ts := httptest.NewServer(h.buildRouter())
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
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/mcp/v1/tools/call", bytes.NewBufferString(reqBody))
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
